package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
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

// resync mempool at least each resyncMempoolPeriodMs (could be more often if invoked by message from ZeroMQ)
const resyncMempoolPeriodMs = 60017

// debounce too close requests for resync mempool (ZeroMQ sends message for each tx, when new block there are many transactions)
const debounceResyncMempoolMs = 1009

// store internal state about once every minute
const storeInternalStatePeriodMs = 59699

var (
	blockchain = flag.String("blockchaincfg", "", "path to blockchain RPC service configuration json file")

	dbPath = flag.String("datadir", "./data", "path to database directory")

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

	httpServerBinding = flag.String("httpserver", "", "http server binding [address]:port, (default no http server)")

	socketIoBinding = flag.String("socketio", "", "socketio server binding [address]:port[/path], (default no socket.io server)")

	certFiles = flag.String("certfile", "", "to enable SSL specify path to certificate files without extension, expecting <certfile>.crt and <certfile>.key, (default no SSL)")

	explorerURL = flag.String("explorer", "", "address of blockchain explorer")

	coin = flag.String("coin", "btc", "coin name")

	noTxCache = flag.Bool("notxcache", false, "disable tx cache")

	computeColumnStats = flag.Bool("computedbstats", false, "compute column stats and exit")
)

var (
	chanSyncIndex              = make(chan struct{})
	chanSyncMempool            = make(chan struct{})
	chanStoreInternalState     = make(chan struct{})
	chanSyncIndexDone          = make(chan struct{})
	chanSyncMempoolDone        = make(chan struct{})
	chanStoreInternalStateDone = make(chan struct{})
	chain                      bchain.BlockChain
	index                      *db.RocksDB
	txCache                    *db.TxCache
	syncWorker                 *db.SyncWorker
	internalState              *common.InternalState
	callbacksOnNewBlockHash    []func(hash string)
	callbacksOnNewTxAddr       []func(txid string, addr string)
	chanOsSignal               chan os.Signal
	inShutdown                 int32
)

func init() {
	glog.MaxSize = 1024 * 1024 * 8
	glog.CopyStandardLogTo("INFO")
}

func getBlockChainWithRetry(coin string, configfile string, pushHandler func(bchain.NotificationType), metrics *common.Metrics, seconds int) (bchain.BlockChain, error) {
	var chain bchain.BlockChain
	var err error
	timer := time.NewTimer(time.Second)
	for i := 0; ; i++ {
		if chain, err = coins.NewBlockChain(coin, configfile, pushHandler, metrics); err != nil {
			if i < seconds {
				glog.Error("rpc: ", err, " Retrying...")
				select {
				case <-chanOsSignal:
					return nil, errors.New("Interrupted")
				case <-timer.C:
					timer.Reset(time.Second)
					continue
				}
			} else {
				return nil, err
			}
		}
		return chain, nil
	}
}

func main() {
	flag.Parse()

	defer glog.Flush()

	chanOsSignal = make(chan os.Signal, 1)
	signal.Notify(chanOsSignal, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)

	glog.Infof("Blockbook: %+v", common.GetVersionInfo())

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

	metrics, err := common.GetMetrics(*coin)
	if err != nil {
		glog.Fatal("GetMetrics: ", err)
	}

	if *blockchain == "" {
		glog.Fatal("Missing blockchaincfg configuration parameter")
	}

	if chain, err = getBlockChainWithRetry(*coin, *blockchain, pushSynchronizationHandler, metrics, 60); err != nil {
		glog.Fatal("rpc: ", err)
	}

	index, err = db.NewRocksDB(*dbPath, chain.GetChainParser(), metrics)
	if err != nil {
		glog.Fatal("rocksDB: ", err)
	}
	defer index.Close()

	internalState, err = newInternalState(*coin, index)
	if err != nil {
		glog.Error("internalState: ", err)
		return
	}
	index.SetInternalState(internalState)
	if internalState.DbState != common.DbStateClosed {
		glog.Warning("internalState: database in not closed state ", internalState.DbState, ", possibly previous ungraceful shutdown")
	}

	if *computeColumnStats {
		internalState.DbState = common.DbStateOpen
		err = index.ComputeInternalStateColumnStats()
		if err != nil {
			glog.Error("internalState: ", err)
		}
		glog.Info("DB size on disk: ", index.DatabaseSizeOnDisk(), ", DB size as computed: ", internalState.DBSizeTotal())
		return
	}

	syncWorker, err = db.NewSyncWorker(index, chain, *syncWorkers, *syncChunk, *blockFrom, *dryRun, chanOsSignal, metrics, internalState)
	if err != nil {
		glog.Fatalf("NewSyncWorker %v", err)
	}

	// set the DbState to open at this moment, after all important workers are initialized
	internalState.DbState = common.DbStateOpen
	err = index.StoreInternalState(internalState)
	if err != nil {
		glog.Fatal("internalState: ", err)
	}

	if *rollbackHeight >= 0 {
		bestHeight, bestHash, err := index.GetBestBlock()
		if err != nil {
			glog.Error("rollbackHeight: ", err)
			return
		}
		if uint32(*rollbackHeight) > bestHeight {
			glog.Infof("nothing to rollback, rollbackHeight %d, bestHeight: %d", *rollbackHeight, bestHeight)
		} else {
			hashes := []string{bestHash}
			for height := bestHeight - 1; height >= uint32(*rollbackHeight); height-- {
				hash, err := index.GetBlockHash(height)
				if err != nil {
					glog.Error("rollbackHeight: ", err)
					return
				}
				hashes = append(hashes, hash)
			}
			err = syncWorker.DisconnectBlocks(uint32(*rollbackHeight), bestHeight, hashes)
			if err != nil {
				glog.Error("rollbackHeight: ", err)
				return
			}
		}
		return
	}

	if txCache, err = db.NewTxCache(index, chain, metrics, !*noTxCache); err != nil {
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

	if *synchronize {
		if err := syncWorker.ResyncIndex(nil); err != nil {
			glog.Error("resyncIndex ", err)
			return
		}
		if _, err = chain.ResyncMempool(nil); err != nil {
			glog.Error("resyncMempool ", err)
			return
		}
	}

	var socketIoServer *server.SocketIoServer
	if *socketIoBinding != "" {
		socketIoServer, err = server.NewSocketIoServer(
			*socketIoBinding, *certFiles, index, chain, txCache, *explorerURL, metrics, internalState)
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
		go storeInternalStateLoop()
	}

	if *blockFrom >= 0 {
		if *blockUntil < 0 {
			*blockUntil = *blockFrom
		}
		height := uint32(*blockFrom)
		until := uint32(*blockUntil)
		address := *queryAddress

		if address != "" {
			if err = index.GetTransactions(address, height, until, printResult); err != nil {
				glog.Error("GetTransactions ", err)
				return
			}
		} else if !*synchronize {
			if err = syncWorker.ConnectBlocksParallel(height, until); err != nil {
				glog.Error("connectBlocksParallel ", err)
				return
			}
		}
	}

	if httpServer != nil || socketIoServer != nil || chain != nil {
		waitForSignalAndShutdown(httpServer, socketIoServer, chain, 10*time.Second)
	}

	if *synchronize {
		close(chanSyncIndex)
		close(chanSyncMempool)
		close(chanStoreInternalState)
		<-chanSyncIndexDone
		<-chanSyncMempoolDone
		<-chanStoreInternalState
	}
}

func newInternalState(coin string, d *db.RocksDB) (*common.InternalState, error) {
	is, err := d.LoadInternalState(coin)
	if err != nil {
		return nil, err
	}
	name, err := os.Hostname()
	if err != nil {
		glog.Error("get hostname ", err)
	} else {
		if i := strings.IndexByte(name, '.'); i > 0 {
			name = name[:i]
		}
		is.Host = name
	}
	return is, nil
}

func tickAndDebounce(tickTime time.Duration, debounceTime time.Duration, input chan struct{}, f func()) {
	timer := time.NewTimer(tickTime)
	var firstDebounce time.Time
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
			if firstDebounce.IsZero() {
				firstDebounce = time.Now()
			}
			// debounce for up to debounceTime period
			// afterwards execute immediately
			if firstDebounce.Add(debounceTime).After(time.Now()) {
				timer.Reset(debounceTime)
			} else {
				timer.Reset(0)
			}
		case <-timer.C:
			// do the action, if not in shutdown, then start the loop again
			if atomic.LoadInt32(&inShutdown) == 0 {
				f()
			}
			timer.Reset(tickTime)
			firstDebounce = time.Time{}
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
		internalState.StartedMempoolSync()
		if count, err := chain.ResyncMempool(onNewTxAddr); err != nil {
			glog.Error("syncMempoolLoop ", errors.ErrorStack(err))
		} else {
			internalState.FinishedMempoolSync(count)

		}
	})
	glog.Info("syncMempoolLoop stopped")
}

func storeInternalStateLoop() {
	defer close(chanStoreInternalStateDone)
	glog.Info("storeInternalStateLoop starting")
	tickAndDebounce(storeInternalStatePeriodMs*time.Millisecond, (storeInternalStatePeriodMs-1)*time.Millisecond, chanStoreInternalState, func() {
		if err := index.StoreInternalState(internalState); err != nil {
			glog.Error("storeInternalStateLoop ", errors.ErrorStack(err))
		}
	})
	glog.Info("storeInternalStateLoop stopped")
}

func onNewTxAddr(txid string, addr string) {
	for _, c := range callbacksOnNewTxAddr {
		c(txid, addr)
	}
}

func pushSynchronizationHandler(nt bchain.NotificationType) {
	if atomic.LoadInt32(&inShutdown) != 0 {
		return
	}
	glog.V(1).Infof("MQ: notification ", nt)
	if nt == bchain.NotificationNewBlock {
		chanSyncIndex <- struct{}{}
	} else if nt == bchain.NotificationNewTx {
		chanSyncMempool <- struct{}{}
	} else {
		glog.Error("MQ: unknown notification sent")
	}
}

func waitForSignalAndShutdown(https *server.HTTPServer, socketio *server.SocketIoServer, chain bchain.BlockChain, timeout time.Duration) {
	sig := <-chanOsSignal
	atomic.StoreInt32(&inShutdown, 1)
	glog.Infof("Shutdown: %v", sig)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

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

	if chain != nil {
		if err := chain.Shutdown(ctx); err != nil {
			glog.Error("BlockChain.Shutdown error: ", err)
		}
	}
}

func printResult(txid string, vout uint32, isOutput bool) error {
	glog.Info(txid, vout, isOutput)
	return nil
}
