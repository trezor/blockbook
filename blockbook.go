package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"math/rand"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/api"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins"
	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/db"
	"github.com/trezor/blockbook/fiat"
	"github.com/trezor/blockbook/fourbyte"
	"github.com/trezor/blockbook/server"
)

// debounce too close requests for resync
const debounceResyncIndexMs = 1009

// debounce too close requests for resync mempool (ZeroMQ sends message for each tx, when new block there are many transactions)
const debounceResyncMempoolMs = 1009

// store internal state about once every minute
const storeInternalStatePeriodMs = 59699

// exit codes from the main function
const exitCodeOK = 0
const exitCodeFatal = 255

var (
	configFile = flag.String("blockchaincfg", "", "path to blockchain RPC service configuration json file")

	dbPath         = flag.String("datadir", "./data", "path to database directory")
	dbCache        = flag.Int("dbcache", 1<<29, "size of the rocksdb cache")
	dbMaxOpenFiles = flag.Int("dbmaxopenfiles", 1<<14, "max open files by rocksdb")

	blockFrom      = flag.Int("blockheight", -1, "height of the starting block")
	blockUntil     = flag.Int("blockuntil", -1, "height of the final block")
	rollbackHeight = flag.Int("rollback", -1, "rollback to the given height and quit")

	synchronize = flag.Bool("sync", false, "synchronizes until tip, if together with zeromq, keeps index synchronized")
	repair      = flag.Bool("repair", false, "repair the database")
	fixUtxo     = flag.Bool("fixutxo", false, "check and fix utxo db and exit")
	prof        = flag.String("prof", "", "http server binding [address]:port of the interface to profiling data /debug/pprof/ (default no profiling)")

	syncChunk   = flag.Int("chunk", 100, "block chunk size for processing in bulk mode")
	syncWorkers = flag.Int("workers", 8, "number of workers to process blocks in bulk mode")
	dryRun      = flag.Bool("dryrun", false, "do not index blocks, only download")

	debugMode = flag.Bool("debug", false, "debug mode, return more verbose errors, reload templates on each request")

	internalBinding = flag.String("internal", "", "internal http server binding [address]:port, (default no internal server)")

	publicBinding = flag.String("public", "", "public http server binding [address]:port[/path] (default no public server)")

	certFiles = flag.String("certfile", "", "to enable SSL specify path to certificate files without extension, expecting <certfile>.crt and <certfile>.key (default no SSL)")

	explorerURL = flag.String("explorer", "", "address of blockchain explorer")

	noTxCache = flag.Bool("notxcache", false, "disable tx cache")

	enableSubNewTx = flag.Bool("enablesubnewtx", false, "enable support for subscribing to all new transactions")

	computeColumnStats  = flag.Bool("computedbstats", false, "compute column stats and exit")
	computeFeeStatsFlag = flag.Bool("computefeestats", false, "compute fee stats for blocks in blockheight-blockuntil range and exit")
	dbStatsPeriodHours  = flag.Int("dbstatsperiod", 24, "period of db stats collection in hours, 0 disables stats collection")

	// resync index at least each resyncIndexPeriodMs (could be more often if invoked by message from ZeroMQ)
	resyncIndexPeriodMs = flag.Int("resyncindexperiod", 935093, "resync index period in milliseconds")

	// resync mempool at least each resyncMempoolPeriodMs (could be more often if invoked by message from ZeroMQ)
	resyncMempoolPeriodMs = flag.Int("resyncmempoolperiod", 60017, "resync mempool period in milliseconds")

	extendedIndex = flag.Bool("extendedindex", false, "if true, create index of input txids and spending transactions")
)

var (
	chanSyncIndex                 = make(chan struct{})
	chanSyncMempool               = make(chan struct{})
	chanStoreInternalState        = make(chan struct{})
	chanSyncIndexDone             = make(chan struct{})
	chanSyncMempoolDone           = make(chan struct{})
	chanStoreInternalStateDone    = make(chan struct{})
	chain                         bchain.BlockChain
	mempool                       bchain.Mempool
	index                         *db.RocksDB
	txCache                       *db.TxCache
	metrics                       *common.Metrics
	syncWorker                    *db.SyncWorker
	internalState                 *common.InternalState
	fiatRates                     *fiat.FiatRates
	callbacksOnNewBlock           []bchain.OnNewBlockFunc
	callbacksOnNewTxAddr          []bchain.OnNewTxAddrFunc
	callbacksOnNewTx              []bchain.OnNewTxFunc
	callbacksOnNewFiatRatesTicker []fiat.OnNewFiatRatesTicker
	chanOsSignal                  chan os.Signal
)

func init() {
	glog.MaxSize = 1024 * 1024 * 8
	glog.CopyStandardLogTo("INFO")
}

func main() {
	defer func() {
		if e := recover(); e != nil {
			glog.Error("main recovered from panic: ", e)
			debug.PrintStack()
			os.Exit(-1)
		}
	}()
	os.Exit(mainWithExitCode())
}

// allow deferred functions to run even in case of fatal error
func mainWithExitCode() int {
	flag.Parse()

	defer glog.Flush()

	rand.Seed(time.Now().UTC().UnixNano())

	chanOsSignal = make(chan os.Signal, 1)
	signal.Notify(chanOsSignal, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)

	glog.Infof("Blockbook: %+v, debug mode %v", common.GetVersionInfo(), *debugMode)

	if *prof != "" {
		go func() {
			log.Println(http.ListenAndServe(*prof, nil))
		}()
	}

	if *repair {
		if err := db.RepairRocksDB(*dbPath); err != nil {
			glog.Errorf("RepairRocksDB %s: %v", *dbPath, err)
			return exitCodeFatal
		}
		return exitCodeOK
	}

	if *configFile == "" {
		glog.Error("Missing blockchaincfg configuration parameter")
		return exitCodeFatal
	}

	configFileContent, err := os.ReadFile(*configFile)
	if err != nil {
		glog.Errorf("Error reading file %v, %v", configFile, err)
		return exitCodeFatal
	}

	coin, coinShortcut, coinLabel, err := coins.GetCoinNameFromConfig(configFileContent)
	if err != nil {
		glog.Error("config: ", err)
		return exitCodeFatal
	}

	metrics, err = common.GetMetrics(coin)
	if err != nil {
		glog.Error("metrics: ", err)
		return exitCodeFatal
	}

	if chain, mempool, err = getBlockChainWithRetry(coin, *configFile, pushSynchronizationHandler, metrics, 120); err != nil {
		glog.Error("rpc: ", err)
		return exitCodeFatal
	}

	index, err = db.NewRocksDB(*dbPath, *dbCache, *dbMaxOpenFiles, chain.GetChainParser(), metrics, *extendedIndex)
	if err != nil {
		glog.Error("rocksDB: ", err)
		return exitCodeFatal
	}
	defer index.Close()

	internalState, err = newInternalState(coin, coinShortcut, coinLabel, index, *enableSubNewTx)
	if err != nil {
		glog.Error("internalState: ", err)
		return exitCodeFatal
	}

	// fix possible inconsistencies in the UTXO index
	if *fixUtxo || !internalState.UtxoChecked {
		err = index.FixUtxos(chanOsSignal)
		if err != nil {
			glog.Error("fixUtxos: ", err)
			return exitCodeFatal
		}
		internalState.UtxoChecked = true
	}

	// sort addressContracts if necessary
	if !internalState.SortedAddressContracts {
		err = index.SortAddressContracts(chanOsSignal)
		if err != nil {
			glog.Error("sortAddressContracts: ", err)
			return exitCodeFatal
		}
		internalState.SortedAddressContracts = true
	}

	index.SetInternalState(internalState)
	if *fixUtxo {
		err = index.StoreInternalState(internalState)
		if err != nil {
			glog.Error("StoreInternalState: ", err)
			return exitCodeFatal
		}
		return exitCodeOK
	}

	if internalState.DbState != common.DbStateClosed {
		if internalState.DbState == common.DbStateInconsistent {
			glog.Error("internalState: database is in inconsistent state and cannot be used")
			return exitCodeFatal
		}
		glog.Warning("internalState: database was left in open state, possibly previous ungraceful shutdown")
	}

	if *computeFeeStatsFlag {
		internalState.DbState = common.DbStateOpen
		err = computeFeeStats(chanOsSignal, *blockFrom, *blockUntil, index, chain, txCache, internalState, metrics)
		if err != nil && err != db.ErrOperationInterrupted {
			glog.Error("computeFeeStats: ", err)
			return exitCodeFatal
		}
		return exitCodeOK
	}

	if *computeColumnStats {
		internalState.DbState = common.DbStateOpen
		err = index.ComputeInternalStateColumnStats(chanOsSignal)
		if err != nil {
			glog.Error("internalState: ", err)
			return exitCodeFatal
		}
		glog.Info("DB size on disk: ", index.DatabaseSizeOnDisk(), ", DB size as computed: ", internalState.DBSizeTotal())
		return exitCodeOK
	}

	syncWorker, err = db.NewSyncWorker(index, chain, *syncWorkers, *syncChunk, *blockFrom, *dryRun, chanOsSignal, metrics, internalState)
	if err != nil {
		glog.Errorf("NewSyncWorker %v", err)
		return exitCodeFatal
	}

	// set the DbState to open at this moment, after all important workers are initialized
	internalState.DbState = common.DbStateOpen
	err = index.StoreInternalState(internalState)
	if err != nil {
		glog.Error("internalState: ", err)
		return exitCodeFatal
	}

	if *rollbackHeight >= 0 {
		err = performRollback()
		if err != nil {
			return exitCodeFatal
		}
		return exitCodeOK
	}

	if txCache, err = db.NewTxCache(index, chain, metrics, internalState, !*noTxCache); err != nil {
		glog.Error("txCache ", err)
		return exitCodeFatal
	}

	if fiatRates, err = fiat.NewFiatRates(index, configFileContent, metrics, onNewFiatRatesTicker); err != nil {
		glog.Error("fiatRates ", err)
		return exitCodeFatal
	}

	// report BlockbookAppInfo metric, only log possible error
	if err = blockbookAppInfoMetric(index, chain, txCache, internalState, metrics); err != nil {
		glog.Error("blockbookAppInfoMetric ", err)
	}

	var internalServer *server.InternalServer
	if *internalBinding != "" {
		internalServer, err = startInternalServer()
		if err != nil {
			glog.Error("internal server: ", err)
			return exitCodeFatal
		}
	}

	var publicServer *server.PublicServer
	if *publicBinding != "" {
		publicServer, err = startPublicServer()
		if err != nil {
			glog.Error("public server: ", err)
			return exitCodeFatal
		}
	}

	if *synchronize {
		internalState.SyncMode = true
		internalState.InitialSync = true
		if err := syncWorker.ResyncIndex(nil, true); err != nil {
			if err != db.ErrOperationInterrupted {
				glog.Error("resyncIndex ", err)
				return exitCodeFatal
			}
			return exitCodeOK
		}
		// initialize mempool after the initial sync is complete
		var addrDescForOutpoint bchain.AddrDescForOutpointFunc
		if chain.GetChainParser().GetChainType() == bchain.ChainBitcoinType {
			addrDescForOutpoint = index.AddrDescForOutpoint
		}
		err = chain.InitializeMempool(addrDescForOutpoint, onNewTxAddr, onNewTx)
		if err != nil {
			glog.Error("initializeMempool ", err)
			return exitCodeFatal
		}
		var mempoolCount int
		if mempoolCount, err = mempool.Resync(); err != nil {
			glog.Error("resyncMempool ", err)
			return exitCodeFatal
		}
		internalState.FinishedMempoolSync(mempoolCount)
		go syncIndexLoop()
		go syncMempoolLoop()
		internalState.InitialSync = false
	}
	go storeInternalStateLoop()

	if publicServer != nil {
		// start full public interface
		callbacksOnNewBlock = append(callbacksOnNewBlock, publicServer.OnNewBlock)
		callbacksOnNewTxAddr = append(callbacksOnNewTxAddr, publicServer.OnNewTxAddr)
		callbacksOnNewTx = append(callbacksOnNewTx, publicServer.OnNewTx)
		callbacksOnNewFiatRatesTicker = append(callbacksOnNewFiatRatesTicker, publicServer.OnNewFiatRatesTicker)
		publicServer.ConnectFullPublicInterface()
	}

	if *blockFrom >= 0 {
		if *blockUntil < 0 {
			*blockUntil = *blockFrom
		}
		height := uint32(*blockFrom)
		until := uint32(*blockUntil)

		if !*synchronize {
			if err = syncWorker.ConnectBlocksParallel(height, until); err != nil {
				if err != db.ErrOperationInterrupted {
					glog.Error("connectBlocksParallel ", err)
					return exitCodeFatal
				}
				return exitCodeOK
			}
		}
	}

	if internalServer != nil || publicServer != nil || chain != nil {
		// start fiat rates downloader only if not shutting down immediately
		initDownloaders(index, chain, configFileContent)
		waitForSignalAndShutdown(internalServer, publicServer, chain, 10*time.Second)
	}

	if *synchronize {
		close(chanSyncIndex)
		close(chanSyncMempool)
		close(chanStoreInternalState)
		<-chanSyncIndexDone
		<-chanSyncMempoolDone
		<-chanStoreInternalStateDone
	}
	return exitCodeOK
}

func getBlockChainWithRetry(coin string, configFile string, pushHandler func(bchain.NotificationType), metrics *common.Metrics, seconds int) (bchain.BlockChain, bchain.Mempool, error) {
	var chain bchain.BlockChain
	var mempool bchain.Mempool
	var err error
	timer := time.NewTimer(time.Second)
	for i := 0; ; i++ {
		if chain, mempool, err = coins.NewBlockChain(coin, configFile, pushHandler, metrics); err != nil {
			if i < seconds {
				glog.Error("rpc: ", err, " Retrying...")
				select {
				case <-chanOsSignal:
					return nil, nil, errors.New("Interrupted")
				case <-timer.C:
					timer.Reset(time.Second)
					continue
				}
			} else {
				return nil, nil, err
			}
		}
		return chain, mempool, nil
	}
}

func startInternalServer() (*server.InternalServer, error) {
	internalServer, err := server.NewInternalServer(*internalBinding, *certFiles, index, chain, mempool, txCache, metrics, internalState, fiatRates)
	if err != nil {
		return nil, err
	}
	go func() {
		err = internalServer.Run()
		if err != nil {
			if err.Error() == "http: Server closed" {
				glog.Info("internal server: closed")
			} else {
				glog.Error(err)
				return
			}
		}
	}()
	return internalServer, nil
}

func startPublicServer() (*server.PublicServer, error) {
	// start public server in limited functionality, extend it after sync is finished by calling ConnectFullPublicInterface
	publicServer, err := server.NewPublicServer(*publicBinding, *certFiles, index, chain, mempool, txCache, *explorerURL, metrics, internalState, fiatRates, *debugMode)
	if err != nil {
		return nil, err
	}
	go func() {
		err = publicServer.Run()
		if err != nil {
			if err.Error() == "http: Server closed" {
				glog.Info("public server: closed")
			} else {
				glog.Error(err)
				return
			}
		}
	}()
	return publicServer, err
}

func performRollback() error {
	bestHeight, bestHash, err := index.GetBestBlock()
	if err != nil {
		glog.Error("rollbackHeight: ", err)
		return err
	}
	if uint32(*rollbackHeight) > bestHeight {
		glog.Infof("nothing to rollback, rollbackHeight %d, bestHeight: %d", *rollbackHeight, bestHeight)
	} else {
		hashes := []string{bestHash}
		for height := bestHeight - 1; height >= uint32(*rollbackHeight); height-- {
			hash, err := index.GetBlockHash(height)
			if err != nil {
				glog.Error("rollbackHeight: ", err)
				return err
			}
			hashes = append(hashes, hash)
		}
		err = syncWorker.DisconnectBlocks(uint32(*rollbackHeight), bestHeight, hashes)
		if err != nil {
			glog.Error("rollbackHeight: ", err)
			return err
		}
	}
	return nil
}

func blockbookAppInfoMetric(db *db.RocksDB, chain bchain.BlockChain, txCache *db.TxCache, is *common.InternalState, metrics *common.Metrics) error {
	api, err := api.NewWorker(db, chain, mempool, txCache, metrics, is, fiatRates)
	if err != nil {
		return err
	}
	si, err := api.GetSystemInfo(false)
	if err != nil {
		return err
	}
	subversion := si.Backend.Subversion
	if subversion == "" {
		// for coins without subversion (ETH) use ConsensusVersion as subversion in metrics
		subversion = si.Backend.ConsensusVersion
	}

	metrics.BlockbookAppInfo.Reset()
	metrics.BlockbookAppInfo.With(common.Labels{
		"blockbook_version":        si.Blockbook.Version,
		"blockbook_commit":         si.Blockbook.GitCommit,
		"blockbook_buildtime":      si.Blockbook.BuildTime,
		"backend_version":          si.Backend.Version,
		"backend_subversion":       subversion,
		"backend_protocol_version": si.Backend.ProtocolVersion}).Set(float64(0))
	metrics.BackendBestHeight.Set(float64(si.Backend.Blocks))
	metrics.BlockbookBestHeight.Set(float64(si.Blockbook.BestHeight))
	return nil
}

func newInternalState(coin, coinShortcut, coinLabel string, d *db.RocksDB, enableSubNewTx bool) (*common.InternalState, error) {
	is, err := d.LoadInternalState(coin)
	if err != nil {
		return nil, err
	}
	is.CoinShortcut = coinShortcut
	if coinLabel == "" {
		coinLabel = coin
	}
	is.CoinLabel = coinLabel
	is.EnableSubNewTx = enableSubNewTx
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

func syncIndexLoop() {
	defer close(chanSyncIndexDone)
	glog.Info("syncIndexLoop starting")
	// resync index about every 15 minutes if there are no chanSyncIndex requests, with debounce 1 second
	common.TickAndDebounce(time.Duration(*resyncIndexPeriodMs)*time.Millisecond, debounceResyncIndexMs*time.Millisecond, chanSyncIndex, func() {
		if err := syncWorker.ResyncIndex(onNewBlockHash, false); err != nil {
			glog.Error("syncIndexLoop ", errors.ErrorStack(err), ", will retry...")
			// retry once in case of random network error, after a slight delay
			time.Sleep(time.Millisecond * 2500)
			if err := syncWorker.ResyncIndex(onNewBlockHash, false); err != nil {
				glog.Error("syncIndexLoop ", errors.ErrorStack(err))
			}
		}
	})
	glog.Info("syncIndexLoop stopped")
}

func onNewBlockHash(hash string, height uint32) {
	defer func() {
		if r := recover(); r != nil {
			glog.Error("onNewBlockHash recovered from panic: ", r)
		}
	}()
	for _, c := range callbacksOnNewBlock {
		c(hash, height)
	}
}

func onNewFiatRatesTicker(ticker *common.CurrencyRatesTicker) {
	defer func() {
		if r := recover(); r != nil {
			glog.Error("onNewFiatRatesTicker recovered from panic: ", r)
			debug.PrintStack()
		}
	}()
	for _, c := range callbacksOnNewFiatRatesTicker {
		c(ticker)
	}
}

func syncMempoolLoop() {
	defer close(chanSyncMempoolDone)
	glog.Info("syncMempoolLoop starting")
	// resync mempool about every minute if there are no chanSyncMempool requests, with debounce 1 second
	common.TickAndDebounce(time.Duration(*resyncMempoolPeriodMs)*time.Millisecond, debounceResyncMempoolMs*time.Millisecond, chanSyncMempool, func() {
		internalState.StartedMempoolSync()
		if count, err := mempool.Resync(); err != nil {
			glog.Error("syncMempoolLoop ", errors.ErrorStack(err))
		} else {
			internalState.FinishedMempoolSync(count)

		}
	})
	glog.Info("syncMempoolLoop stopped")
}

func storeInternalStateLoop() {
	stopCompute := make(chan os.Signal)
	defer func() {
		close(stopCompute)
		close(chanStoreInternalStateDone)
	}()
	signal.Notify(stopCompute, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)
	var computeRunning bool
	lastCompute := time.Now()
	lastAppInfo := time.Now()
	logAppInfoPeriod := 15 * time.Minute
	// randomize the duration between ComputeInternalStateColumnStats to avoid peaks after reboot of machine with multiple blockbooks
	computePeriod := time.Duration(*dbStatsPeriodHours)*time.Hour + time.Duration(rand.Float64()*float64((4*time.Hour).Nanoseconds()))
	if (*dbStatsPeriodHours) > 0 {
		glog.Info("storeInternalStateLoop starting with db stats recompute period ", computePeriod)
	} else {
		glog.Info("storeInternalStateLoop starting with db stats compute disabled")
	}
	common.TickAndDebounce(storeInternalStatePeriodMs*time.Millisecond, (storeInternalStatePeriodMs-1)*time.Millisecond, chanStoreInternalState, func() {
		if (*dbStatsPeriodHours) > 0 && !computeRunning && lastCompute.Add(computePeriod).Before(time.Now()) {
			computeRunning = true
			go func() {
				err := index.ComputeInternalStateColumnStats(stopCompute)
				if err != nil {
					glog.Error("computeInternalStateColumnStats error: ", err)
				}
				lastCompute = time.Now()
				computeRunning = false
			}()
		}
		if err := index.StoreInternalState(internalState); err != nil {
			glog.Error("storeInternalStateLoop ", errors.ErrorStack(err))
		}
		if lastAppInfo.Add(logAppInfoPeriod).Before(time.Now()) {
			if glog.V(1) {
				glog.Info(index.GetMemoryStats())
			}
			if err := blockbookAppInfoMetric(index, chain, txCache, internalState, metrics); err != nil {
				glog.Error("blockbookAppInfoMetric ", err)
			}
			lastAppInfo = time.Now()
		}
	})
	glog.Info("storeInternalStateLoop stopped")
}

func onNewTxAddr(tx *bchain.Tx, desc bchain.AddressDescriptor) {
	defer func() {
		if r := recover(); r != nil {
			glog.Error("onNewTxAddr recovered from panic: ", r)
		}
	}()
	for _, c := range callbacksOnNewTxAddr {
		c(tx, desc)
	}
}

func onNewTx(tx *bchain.MempoolTx) {
	defer func() {
		if r := recover(); r != nil {
			glog.Error("onNewTx recovered from panic: ", r)
		}
	}()
	for _, c := range callbacksOnNewTx {
		c(tx)
	}
}

func pushSynchronizationHandler(nt bchain.NotificationType) {
	glog.V(1).Info("MQ: notification ", nt)
	if common.IsInShutdown() {
		return
	}
	if nt == bchain.NotificationNewBlock {
		chanSyncIndex <- struct{}{}
	} else if nt == bchain.NotificationNewTx {
		chanSyncMempool <- struct{}{}
	} else {
		glog.Error("MQ: unknown notification sent")
	}
}

func waitForSignalAndShutdown(internal *server.InternalServer, public *server.PublicServer, chain bchain.BlockChain, timeout time.Duration) {
	sig := <-chanOsSignal
	common.SetInShutdown()
	glog.Infof("shutdown: %v", sig)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if internal != nil {
		if err := internal.Shutdown(ctx); err != nil {
			glog.Error("internal server: shutdown error: ", err)
		}
	}

	if public != nil {
		if err := public.Shutdown(ctx); err != nil {
			glog.Error("public server: shutdown error: ", err)
		}
	}

	if chain != nil {
		if err := chain.Shutdown(ctx); err != nil {
			glog.Error("rpc: shutdown error: ", err)
		}
	}
}

// computeFeeStats computes fee distribution in defined blocks
func computeFeeStats(stopCompute chan os.Signal, blockFrom, blockTo int, db *db.RocksDB, chain bchain.BlockChain, txCache *db.TxCache, is *common.InternalState, metrics *common.Metrics) error {
	start := time.Now()
	glog.Info("computeFeeStats start")
	api, err := api.NewWorker(db, chain, mempool, txCache, metrics, is, fiatRates)
	if err != nil {
		return err
	}
	err = api.ComputeFeeStats(blockFrom, blockTo, stopCompute)
	glog.Info("computeFeeStats finished in ", time.Since(start))
	return err
}

func initDownloaders(db *db.RocksDB, chain bchain.BlockChain, configFileContent []byte) {
	if fiatRates.Enabled {
		go fiatRates.RunDownloader()
	}

	var config struct {
		FourByteSignatures string `json:"fourByteSignatures"`
	}

	err := json.Unmarshal(configFileContent, &config)
	if err != nil {
		glog.Errorf("Error parsing config file %v, %v", *configFile, err)
		return
	}

	if config.FourByteSignatures != "" && chain.GetChainParser().GetChainType() == bchain.ChainEthereumType {
		fbsd, err := fourbyte.NewFourByteSignaturesDownloader(db, config.FourByteSignatures)
		if err != nil {
			glog.Errorf("NewFourByteSignaturesDownloader Init error: %v", err)
		} else {
			glog.Infof("Starting FourByteSignatures downloader...")
			go fbsd.Run()
		}

	}

}
