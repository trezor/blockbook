package main

import (
	"context"
	"encoding/hex"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/juju/errors"

	"blockbook/bchain"
	"blockbook/bchain/coins"
	"blockbook/common"
	"blockbook/db"
	"blockbook/server"

	"github.com/golang/glog"

	"net/http"
	_ "net/http/pprof"
)

// resync index at least each resyncIndexPeriodMs (could be more often if invoked by message from ZeroMQ)
const resyncIndexPeriodMs = 935093

// debounce too close requests for resync
const debounceResyncIndexMs = 1009

// resync mempool at least each resyncIndexPeriodMs (could be more often if invoked by message from ZeroMQ)
const resyncMempoolPeriodMs = 60017

// debounce too close requests for resync mempool (ZeroMQ sends message for each tx, when new block there are many transactions)
const debounceResyncMempoolMs = 1009

var (
	rpcURL     = flag.String("rpcurl", "http://localhost:8332", "url of blockchain RPC service")
	rpcUser    = flag.String("rpcuser", "rpc", "rpc username")
	rpcPass    = flag.String("rpcpass", "rpc", "rpc password")
	rpcTimeout = flag.Uint("rpctimeout", 25, "rpc timeout in seconds")

	dbPath = flag.String("path", "./data", "path to address index directory")

	blockFrom      = flag.Int("blockheight", -1, "height of the starting block")
	blockUntil     = flag.Int("blockuntil", -1, "height of the final block")
	rollbackHeight = flag.Int("rollback", -1, "rollback to the given height and quit")

	queryAddress = flag.String("address", "", "query contents of this address")

	synchronize = flag.Bool("sync", false, "synchronizes until tip, if together with zeromq, keeps index synchronized")
	repair      = flag.Bool("repair", false, "repair the database")
	prof        = flag.String("prof", "", "http server binding [address]:port of the interface to profiling data /debug/pprof/ (default no profiling)")

	syncChunk   = flag.Int("chunk", 100, "block chunk size for processing")
	syncWorkers = flag.Int("workers", 8, "number of workers to process blocks")
	dryRun      = flag.Bool("dryrun", false, "do not index blocks, only download")
	parse       = flag.Bool("parse", false, "use in-process block parsing")

	httpServerBinding = flag.String("httpserver", "", "http server binding [address]:port, (default no http server)")

	socketIoBinding = flag.String("socketio", "", "socketio server binding [address]:port[/path], (default no socket.io server)")

	certFiles = flag.String("certfile", "", "to enable SSL specify path to certificate files without extension, expecting <certfile>.crt and <certfile>.key, (default no SSL)")

	zeroMQBinding = flag.String("zeromq", "", "binding to zeromq, if missing no zeromq connection")

	explorerURL = flag.String("explorer", "", "address of blockchain explorer")

	coin = flag.String("coin", "btc", "coin name (default btc)")
)

var (
	chanSyncIndex           = make(chan struct{})
	chanSyncMempool         = make(chan struct{})
	chanSyncIndexDone       = make(chan struct{})
	chanSyncMempoolDone     = make(chan struct{})
	chain                   bchain.BlockChain
	index                   *db.RocksDB
	txCache                 *db.TxCache
	syncWorker              *db.SyncWorker
	callbacksOnNewBlockHash []func(hash string)
	callbacksOnNewTxAddr    []func(txid string, addr string)
	chanOsSignal            chan os.Signal
)

func main() {
	flag.Parse()

	// override setting for glog to log only to stderr, to match the http handler
	flag.Lookup("logtostderr").Value.Set("true")

	defer glog.Flush()

	chanOsSignal = make(chan os.Signal, 1)
	signal.Notify(chanOsSignal, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)

	if *prof != "" {
		go func() {
			log.Println(http.ListenAndServe(*prof, nil))
		}()
	}

	if *repair {
		if err := db.RepairRocksDB(*dbPath); err != nil {
			glog.Fatalf("RepairRocksDB %s: %v", *dbPath, err)
		}
		return
	}

	metrics, err := common.GetMetrics()
	if err != nil {
		glog.Fatal("GetMetrics: ", err)
	}

	if chain, err = coins.NewBlockChain(*coin, *rpcURL, *rpcUser, *rpcPass, time.Duration(*rpcTimeout)*time.Second, *parse, metrics); err != nil {
		glog.Fatal("rpc: ", err)
	}

	index, err = db.NewRocksDB(*dbPath, chain.GetChainParser())
	if err != nil {
		glog.Fatal("rocksDB: ", err)
	}
	defer index.Close()

	if *rollbackHeight >= 0 {
		bestHeight, _, err := index.GetBestBlock()
		if err != nil {
			glog.Error("rollbackHeight: ", err)
			return
		}
		if uint32(*rollbackHeight) > bestHeight {
			glog.Infof("nothing to rollback, rollbackHeight %d, bestHeight: %d", *rollbackHeight, bestHeight)
		} else {
			err = index.DisconnectBlocks(uint32(*rollbackHeight), bestHeight)
			if err != nil {
				glog.Error("rollbackHeight: ", err)
				return
			}
		}
		return
	}

	syncWorker, err = db.NewSyncWorker(index, chain, *syncWorkers, *syncChunk, *blockFrom, *dryRun, chanOsSignal, metrics)
	if err != nil {
		glog.Fatalf("NewSyncWorker %v", err)
	}

	if *synchronize {
		if err := syncWorker.ResyncIndex(nil); err != nil {
			glog.Error("resyncIndex ", err)
			return
		}
		if err = chain.ResyncMempool(nil); err != nil {
			glog.Error("resyncMempool ", err)
			return
		}
	}

	if txCache, err = db.NewTxCache(index, chain, metrics); err != nil {
		glog.Error("txCache ", err)
		return
	}

	var httpServer *server.HTTPServer
	if *httpServerBinding != "" {
		httpServer, err = server.NewHTTPServer(*httpServerBinding, *certFiles, index, chain, txCache)
		if err != nil {
			glog.Error("https: ", err)
			return
		}
		go func() {
			err = httpServer.Run()
			if err != nil {
				if err.Error() == "http: Server closed" {
					glog.Info(err)
				} else {
					glog.Error(err)
					return
				}
			}
		}()
	}

	var socketIoServer *server.SocketIoServer
	if *socketIoBinding != "" {
		socketIoServer, err = server.NewSocketIoServer(
			*socketIoBinding, *certFiles, index, chain, txCache, *explorerURL, metrics)
		if err != nil {
			glog.Error("socketio: ", err)
			return
		}
		go func() {
			err = socketIoServer.Run()
			if err != nil {
				if err.Error() == "http: Server closed" {
					glog.Info(err)
				} else {
					glog.Error(err)
					return
				}
			}
		}()
		callbacksOnNewBlockHash = append(callbacksOnNewBlockHash, socketIoServer.OnNewBlockHash)
		callbacksOnNewTxAddr = append(callbacksOnNewTxAddr, socketIoServer.OnNewTxAddr)
	}

	if *synchronize {
		// start the synchronization loops after the server interfaces are started
		go syncIndexLoop()
		go syncMempoolLoop()
	}

	var mq *bchain.MQ
	if *zeroMQBinding != "" {
		if !*synchronize {
			glog.Error("zeromq connection without synchronization does not make sense, ignoring zeromq parameter")
		} else {
			mq, err = bchain.NewMQ(*zeroMQBinding, mqHandler)
			if err != nil {
				glog.Error("mq: ", err)
				return
			}
		}
	}

	if *blockFrom >= 0 {
		if *blockUntil < 0 {
			*blockUntil = *blockFrom
		}
		height := uint32(*blockFrom)
		until := uint32(*blockUntil)
		address := *queryAddress

		if address != "" {
			script, err := chain.GetChainParser().AddressToOutputScript(address)
			if err != nil {
				glog.Error("GetTransactions ", err)
				return
			}
			if err = index.GetTransactions(script, height, until, printResult); err != nil {
				glog.Error("GetTransactions ", err)
				return
			}
		} else if !*synchronize {
			if err = syncWorker.ConnectBlocksParallelInChunks(height, until); err != nil {
				glog.Error("connectBlocksParallelInChunks ", err)
				return
			}
		}
	}

	if httpServer != nil || socketIoServer != nil || mq != nil {
		waitForSignalAndShutdown(httpServer, socketIoServer, mq, 5*time.Second)
	}

	if *synchronize {
		close(chanSyncIndex)
		close(chanSyncMempool)
		<-chanSyncIndexDone
		<-chanSyncMempoolDone
	}
}

func tickAndDebounce(tickTime time.Duration, debounceTime time.Duration, input chan struct{}, f func()) {
	timer := time.NewTimer(tickTime)
Loop:
	for {
		select {
		case _, ok := <-input:
			if !timer.Stop() {
				<-timer.C
			}
			// exit loop on closed input channel
			if !ok {
				break Loop
			}
			// debounce for debounceTime
			timer.Reset(debounceTime)
		case <-timer.C:
			// do the action and start the loop again
			f()
			timer.Reset(tickTime)
		}
	}
}

func syncIndexLoop() {
	defer close(chanSyncIndexDone)
	glog.Info("syncIndexLoop starting")
	// resync index about every 15 minutes if there are no chanSyncIndex requests, with debounce 1 second
	tickAndDebounce(resyncIndexPeriodMs*time.Millisecond, debounceResyncIndexMs*time.Millisecond, chanSyncIndex, func() {
		if err := syncWorker.ResyncIndex(onNewBlockHash); err != nil {
			glog.Error("syncIndexLoop ", errors.ErrorStack(err))
		}
	})
	glog.Info("syncIndexLoop stopped")
}

func onNewBlockHash(hash string) {
	for _, c := range callbacksOnNewBlockHash {
		c(hash)
	}
}

func syncMempoolLoop() {
	defer close(chanSyncMempoolDone)
	glog.Info("syncMempoolLoop starting")
	// resync mempool about every minute if there are no chanSyncMempool requests, with debounce 1 second
	tickAndDebounce(resyncMempoolPeriodMs*time.Millisecond, debounceResyncMempoolMs*time.Millisecond, chanSyncMempool, func() {
		if err := chain.ResyncMempool(onNewTxAddr); err != nil {
			glog.Error("syncMempoolLoop ", errors.ErrorStack(err))
		}
	})
	glog.Info("syncMempoolLoop stopped")
}

func onNewTxAddr(txid string, addr string) {
	for _, c := range callbacksOnNewTxAddr {
		c(txid, addr)
	}
}

func mqHandler(m *bchain.MQMessage) {
	// TODO - is coin specific, item for abstraction
	body := hex.EncodeToString(m.Body)
	glog.V(1).Infof("MQ: %s-%d  %s", m.Topic, m.Sequence, body)
	if m.Topic == "hashblock" {
		chanSyncIndex <- struct{}{}
	} else if m.Topic == "hashtx" {
		chanSyncMempool <- struct{}{}
	} else {
		glog.Errorf("MQ: unknown message %s-%d  %s", m.Topic, m.Sequence, body)
	}
}

func waitForSignalAndShutdown(https *server.HTTPServer, socketio *server.SocketIoServer, mq *bchain.MQ, timeout time.Duration) {
	sig := <-chanOsSignal

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	glog.Infof("Shutdown: %v", sig)

	if mq != nil {
		if err := mq.Shutdown(); err != nil {
			glog.Error("MQ.Shutdown error: ", err)
		}
	}

	if https != nil {
		if err := https.Shutdown(ctx); err != nil {
			glog.Error("HttpServer.Shutdown error: ", err)
		}
	}

	if socketio != nil {
		if err := socketio.Shutdown(ctx); err != nil {
			glog.Error("SocketIo.Shutdown error: ", err)
		}
	}
}

func printResult(txid string, vout uint32, isOutput bool) error {
	glog.Info(txid, vout, isOutput)
	return nil
}
