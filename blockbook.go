package main

import (
	"context"
	"encoding/hex"
	"flag"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"blockbook/bchain"
	"blockbook/db"
	"blockbook/server"

	"github.com/golang/glog"
	"github.com/pkg/profile"
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
	rpcURL     = flag.String("rpcurl", "http://localhost:8332", "url of bitcoin RPC service")
	rpcUser    = flag.String("rpcuser", "rpc", "rpc username")
	rpcPass    = flag.String("rpcpass", "rpc", "rpc password")
	rpcTimeout = flag.Uint("rpctimeout", 25, "rpc timeout in seconds")

	dbPath = flag.String("path", "./data", "path to address index directory")

	blockHeight    = flag.Int("blockheight", -1, "height of the starting block")
	blockUntil     = flag.Int("blockuntil", -1, "height of the final block")
	rollbackHeight = flag.Int("rollback", -1, "rollback to the given height and quit")

	queryAddress = flag.String("address", "", "query contents of this address")

	synchronize = flag.Bool("sync", false, "synchronizes until tip, if together with zeromq, keeps index synchronized")
	repair      = flag.Bool("repair", false, "repair the database")
	prof        = flag.Bool("prof", false, "profile program execution")

	syncChunk          = flag.Int("chunk", 100, "block chunk size for processing")
	syncWorkers        = flag.Int("workers", 8, "number of workers to process blocks")
	dryRun             = flag.Bool("dryrun", false, "do not index blocks, only download")
	parse              = flag.Bool("parse", false, "use in-process block parsing")
	compactDBTriggerMB = flag.Int64("compact", -1, "invoke compaction when db size exceeds value in MB, default no compaction")

	httpServerBinding = flag.String("httpserver", "", "http server binding [address]:port, if missing no http server")

	zeroMQBinding = flag.String("zeromq", "", "binding to zeromq, if missing no zeromq connection")
)

var (
	chanSyncIndex       = make(chan struct{})
	chanSyncMempool     = make(chan struct{})
	chanSyncIndexDone   = make(chan struct{})
	chanSyncMempoolDone = make(chan struct{})
	chain               *bchain.BitcoinRPC
	mempool             *bchain.Mempool
	index               *db.RocksDB
)

func main() {
	flag.Parse()

	// override setting for glog to log only to stderr, to match the http handler
	flag.Lookup("logtostderr").Value.Set("true")

	defer glog.Flush()

	if *prof {
		defer profile.Start().Stop()
	}

	if *repair {
		if err := db.RepairRocksDB(*dbPath); err != nil {
			glog.Fatalf("RepairRocksDB %s: %v", *dbPath, err)
		}
		return
	}

	chain = bchain.NewBitcoinRPC(
		*rpcURL,
		*rpcUser,
		*rpcPass,
		time.Duration(*rpcTimeout)*time.Second)

	if *parse {
		chain.Parser = &bchain.BitcoinBlockParser{
			Params: bchain.GetChainParams()[0],
		}
	}

	mempool = bchain.NewMempool(chain)

	var err error
	index, err = db.NewRocksDB(*dbPath)
	if err != nil {
		glog.Fatalf("NewRocksDB %v", err)
	}
	defer index.Close()

	if *rollbackHeight >= 0 {
		bestHeight, _, err := index.GetBestBlock()
		if err != nil {
			glog.Fatalf("rollbackHeight: %v", err)
		}
		if uint32(*rollbackHeight) > bestHeight {
			glog.Infof("nothing to rollback, rollbackHeight %d, bestHeight: %d", *rollbackHeight, bestHeight)
		} else {
			err = index.DisconnectBlocks(uint32(*rollbackHeight), bestHeight)
			if err != nil {
				glog.Fatalf("rollbackHeight: %v", err)
			}
		}
		return
	}

	if *synchronize {
		if err := resyncIndex(true); err != nil {
			glog.Fatal("resyncIndex ", err)
		}
		go syncIndexLoop()
		go syncMempoolLoop()
		chanSyncMempool <- struct{}{}
	}

	var httpServer *server.HTTPServer
	if *httpServerBinding != "" {
		httpServer, err = server.NewHTTPServer(*httpServerBinding, index, mempool)
		if err != nil {
			glog.Fatal("https: ", err)
		}
		go func() {
			err = httpServer.Run()
			if err != nil {
				if err.Error() == "http: Server closed" {
					glog.Info(err)
				} else {
					glog.Fatal(err)
				}
			}
		}()
	}

	var mq *bchain.MQ
	if *zeroMQBinding != "" {
		if !*synchronize {
			glog.Error("zeromq connection without synchronization does not make sense, ignoring zeromq parameter")
		} else {
			mq, err = bchain.NewMQ(*zeroMQBinding, mqHandler)
			if err != nil {
				glog.Fatal("mq: ", err)
			}
		}
	}

	if *blockHeight >= 0 {
		if *blockUntil < 0 {
			*blockUntil = *blockHeight
		}
		height := uint32(*blockHeight)
		until := uint32(*blockUntil)
		address := *queryAddress

		if address != "" {
			script, err := bchain.AddressToOutputScript(address)
			if err != nil {
				glog.Fatalf("GetTransactions %v", err)
			}
			if err = index.GetTransactions(script, height, until, printResult); err != nil {
				glog.Fatalf("GetTransactions %v", err)
			}
		} else if !*synchronize {
			if err = connectBlocksParallelInChunks(
				height,
				until,
				*syncChunk,
				*syncWorkers,
			); err != nil {
				glog.Fatalf("connectBlocksParallelInChunks %v", err)
			}
		}
	}

	if httpServer != nil || mq != nil {
		waitForSignalAndShutdown(httpServer, mq, 5*time.Second)
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
		if err := resyncIndex(false); err != nil {
			glog.Error("syncIndexLoop", err)
		}
	})
	glog.Info("syncIndexLoop stopped")
}

func syncMempoolLoop() {
	defer close(chanSyncMempoolDone)
	glog.Info("syncMempoolLoop starting")
	// resync mempool about every minute if there are no chanSyncMempool requests, with debounce 1 second
	tickAndDebounce(resyncMempoolPeriodMs*time.Millisecond, debounceResyncMempoolMs*time.Millisecond, chanSyncMempool, func() {
		if err := mempool.Resync(); err != nil {
			glog.Error("syncMempoolLoop", err)
		}
	})
	glog.Info("syncMempoolLoop stopped")
}

func mqHandler(m *bchain.MQMessage) {
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

func waitForSignalAndShutdown(s *server.HTTPServer, mq *bchain.MQ, timeout time.Duration) {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)

	sig := <-stop

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	glog.Infof("Shutdown: %v", sig)

	if mq != nil {
		if err := mq.Shutdown(); err != nil {
			glog.Error("MQ.Shutdown error: ", err)
		}
	}

	if s != nil {
		if err := s.Shutdown(ctx); err != nil {
			glog.Error("HttpServer.Shutdown error: ", err)
		}
	}
}

func printResult(txid string, vout uint32, isOutput bool) error {
	glog.Info(txid, vout, isOutput)
	return nil
}

func resyncIndex(bulk bool) error {
	remote, err := chain.GetBestBlockHash()
	if err != nil {
		return err
	}
	localBestHeight, local, err := index.GetBestBlock()
	if err != nil {
		local = ""
	}

	// If the locally indexed block is the same as the best block on the
	// network, we're done.
	if local == remote {
		glog.Infof("resync: synced on %d %s", localBestHeight, local)
		return nil
	}

	var header *bchain.BlockHeader
	if local != "" {
		// Is local tip on the best chain?
		header, err = chain.GetBlockHeader(local)
		forked := false
		if err != nil {
			if e, ok := err.(*bchain.RPCError); ok && e.Message == "Block not found" {
				forked = true
			} else {
				return err
			}
		} else {
			if header.Confirmations < 0 {
				forked = true
			}
		}

		if forked {
			// find and disconnect forked blocks and then synchronize again
			glog.Info("resync: local is forked")
			var height uint32
			for height = localBestHeight - 1; height >= 0; height-- {
				local, err = index.GetBlockHash(height)
				if err != nil {
					return err
				}
				remote, err = chain.GetBlockHash(height)
				if err != nil {
					return err
				}
				if local == remote {
					break
				}
			}
			err = index.DisconnectBlocks(height+1, localBestHeight)
			if err != nil {
				return err
			}
			return resyncIndex(false)
		}
	}

	startHeight := uint32(0)
	var hash string
	if header != nil {
		glog.Info("resync: local is behind")
		hash = header.Next
		startHeight = localBestHeight
		// bulk load is allowed only for empty db, otherwise we could get rocksdb "error db has more levels than options.num_levels"
		bulk = false
	} else {
		// If the local block is missing, we're indexing from the genesis block
		// or from the start block specified by flags
		if *blockHeight > 0 {
			startHeight = uint32(*blockHeight)
		}
		glog.Info("resync: genesis from block ", startHeight)
		hash, err = chain.GetBlockHash(startHeight)
		if err != nil {
			return err
		}
	}

	// if parallel operation is enabled and the number of blocks to be connected is large,
	// use parallel routine to load majority of blocks
	if *syncWorkers > 1 {
		chainBestHeight, err := chain.GetBestBlockHeight()
		if err != nil {
			return err
		}
		if chainBestHeight-startHeight > uint32(*syncChunk) {
			glog.Infof("resync: parallel sync of blocks %d-%d, using %d workers", startHeight, chainBestHeight, *syncWorkers)
			err = connectBlocksParallel(
				startHeight,
				chainBestHeight,
				*syncWorkers,
				bulk,
			)
			if err != nil {
				return err
			}
			// after parallel load finish the sync using standard way,
			// new blocks may have been created in the meantime
			return resyncIndex(false)
		}
	}

	return connectBlocks(hash)
}

func connectBlocks(
	hash string,
) error {
	bch := make(chan blockResult, 8)
	done := make(chan struct{})
	defer close(done)

	go getBlockChain(hash, bch, done)

	var lastRes blockResult
	for res := range bch {
		lastRes = res
		if res.err != nil {
			return res.err
		}
		err := index.ConnectBlock(res.block)
		if err != nil {
			return err
		}
	}

	if lastRes.block != nil {
		glog.Infof("resync: synced on %d %s", lastRes.block.Height, lastRes.block.Hash)
	}

	return nil
}

func connectBlocksParallel(
	lower uint32,
	higher uint32,
	numWorkers int,
	bulk bool,
) error {
	var err error
	if bulk {
		err = index.ReopenWithBulk(true)
		if err != nil {
			return err
		}
	}

	var wg sync.WaitGroup
	hch := make(chan string, numWorkers)
	running := make([]bool, numWorkers)
	work := func(i int) {
		defer wg.Done()
		for hash := range hch {
			running[i] = true
			block, err := chain.GetBlock(hash)
			if err != nil {
				glog.Error("Connect block ", hash, " error ", err)
				running[i] = false
				continue
			}
			if *dryRun {
				running[i] = false
				continue
			}
			err = index.ConnectBlock(block)
			if err != nil {
				glog.Error("Connect block ", hash, " error ", err)
			}
			running[i] = false
		}
	}
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go work(i)
	}
	var hash string

	for h := lower; h <= higher; h++ {
		hash, err = chain.GetBlockHash(h)
		if err != nil {
			break
		}
		hch <- hash
		if h > 0 && h%1000 == 0 {
			glog.Info("connecting block ", h, " ", hash)
			if bulk && *compactDBTriggerMB > 0 {
				size, err := index.DatabaseSizeOnDisk()
				if err != nil {
					break
				}
				if size > *compactDBTriggerMB*1048576 {
					// wait for the workers to finish block
				WaitAgain:
					for {
						for _, r := range running {
							if r {
								glog.Info("Waiting ", running)
								time.Sleep(time.Millisecond * 500)
								continue WaitAgain
							}
						}
						break
					}
					if err = index.CompactDatabase(bulk); err != nil {
						break
					}
				}
			}
		}
	}
	close(hch)
	wg.Wait()

	if err == nil && bulk {
		err = index.ReopenWithBulk(false)
	}

	return err
}

func connectBlocksParallelInChunks(
	lower uint32,
	higher uint32,
	chunkSize int,
	numWorkers int,
) error {
	var wg sync.WaitGroup

	work := func(i int) {
		defer wg.Done()

		offset := uint32(chunkSize * i)
		stride := uint32(chunkSize * numWorkers)

		for low := lower + offset; low <= higher; low += stride {
			high := low + uint32(chunkSize-1)
			if high > higher {
				high = higher
			}
			err := connectBlockChunk(low, high)
			if err != nil {
				if e, ok := err.(*bchain.RPCError); ok && (e.Message == "Block height out of range" || e.Message == "Block not found") {
					break
				}
				glog.Fatalf("connectBlocksParallel %d-%d %v", low, high, err)
			}
		}
	}
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go work(i)
	}
	wg.Wait()

	return nil
}

func connectBlockChunk(
	lower uint32,
	higher uint32,
) error {
	connected, err := isBlockConnected(higher)
	if err != nil || connected {
		// if higher is over the best block, continue with lower block, otherwise return error
		if e, ok := err.(*bchain.RPCError); !ok || e.Message != "Block height out of range" {
			return err
		}
	}

	height := lower
	hash, err := chain.GetBlockHash(lower)
	if err != nil {
		return err
	}

	for height <= higher {
		block, err := chain.GetBlock(hash)
		if err != nil {
			return err
		}
		hash = block.Next
		height = block.Height + 1
		if *dryRun {
			continue
		}
		err = index.ConnectBlock(block)
		if err != nil {
			return err
		}
		if block.Height%1000 == 0 {
			glog.Info("connected block ", block.Height, " ", block.Hash)
		}
	}

	return nil
}

func isBlockConnected(
	height uint32,
) (bool, error) {
	local, err := index.GetBlockHash(height)
	if err != nil {
		return false, err
	}
	remote, err := chain.GetBlockHash(height)
	if err != nil {
		return false, err
	}
	if local != remote {
		return false, nil
	}
	return true, nil
}

type blockResult struct {
	block *bchain.Block
	err   error
}

func getBlockChain(
	hash string,
	out chan blockResult,
	done chan struct{},
) {
	defer close(out)

	for hash != "" {
		select {
		case <-done:
			return
		default:
		}
		block, err := chain.GetBlock(hash)
		if err != nil {
			out <- blockResult{err: err}
			return
		}
		hash = block.Next
		out <- blockResult{block: block}
	}
}
