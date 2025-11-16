package db

import (
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
)

// SyncWorker is handle to SyncWorker
type SyncWorker struct {
	db                     *RocksDB
	chain                  bchain.BlockChain
	syncWorkers, syncChunk int
	dryRun                 bool
	startHeight            uint32
	startHash              string
	chanOsSignal           chan os.Signal
	metrics                *common.Metrics
	is                     *common.InternalState
}

// NewSyncWorker creates new SyncWorker and returns its handle
func NewSyncWorker(db *RocksDB, chain bchain.BlockChain, syncWorkers, syncChunk int, minStartHeight int, dryRun bool, chanOsSignal chan os.Signal, metrics *common.Metrics, is *common.InternalState) (*SyncWorker, error) {
	if minStartHeight < 0 {
		minStartHeight = 0
	}
	return &SyncWorker{
		db:           db,
		chain:        chain,
		syncWorkers:  syncWorkers,
		syncChunk:    syncChunk,
		dryRun:       dryRun,
		startHeight:  uint32(minStartHeight),
		chanOsSignal: chanOsSignal,
		metrics:      metrics,
		is:           is,
	}, nil
}

var errSynced = errors.New("synced")
var errFork = errors.New("fork")

// ErrOperationInterrupted is returned when operation is interrupted by OS signal
var ErrOperationInterrupted = errors.New("ErrOperationInterrupted")

func (w *SyncWorker) updateBackendInfo() {
	ci, err := w.chain.GetChainInfo()
	var backendError string
	if err != nil {
		glog.Error("GetChainInfo error ", err)
		backendError = errors.Annotatef(err, "GetChainInfo").Error()
		ci = &bchain.ChainInfo{}
	}
	w.is.SetBackendInfo(&common.BackendInfo{
		BackendError:     backendError,
		BestBlockHash:    ci.Bestblockhash,
		Blocks:           ci.Blocks,
		Chain:            ci.Chain,
		Difficulty:       ci.Difficulty,
		Headers:          ci.Headers,
		ProtocolVersion:  ci.ProtocolVersion,
		SizeOnDisk:       ci.SizeOnDisk,
		Subversion:       ci.Subversion,
		Timeoffset:       ci.Timeoffset,
		Version:          ci.Version,
		Warnings:         ci.Warnings,
		ConsensusVersion: ci.ConsensusVersion,
		Consensus:        ci.Consensus,
	})
}

// ResyncIndex synchronizes index to the top of the blockchain
// onNewBlock is called when new block is connected, but not in initial parallel sync
func (w *SyncWorker) ResyncIndex(onNewBlock bchain.OnNewBlockFunc, initialSync bool) error {
	start := time.Now()
	w.is.StartedSync()

	err := w.resyncIndex(onNewBlock, initialSync)

	// update backend info after each resync
	w.updateBackendInfo()

	switch err {
	case nil:
		d := time.Since(start)
		glog.Info("resync: finished in ", d)
		w.metrics.IndexResyncDuration.Observe(float64(d) / 1e6) // in milliseconds
		w.metrics.IndexDBSize.Set(float64(w.db.DatabaseSizeOnDisk()))
		bh, _, err := w.db.GetBestBlock()
		if err == nil {
			w.is.FinishedSync(bh)
		}
		w.metrics.BackendBestHeight.Set(float64(w.is.BackendInfo.Blocks))
		w.metrics.BlockbookBestHeight.Set(float64(bh))
		return err
	case errSynced:
		// this is not actually error but flag that resync wasn't necessary
		w.is.FinishedSyncNoChange()
		w.metrics.IndexDBSize.Set(float64(w.db.DatabaseSizeOnDisk()))
		if initialSync {
			d := time.Since(start)
			glog.Info("resync: finished in ", d)
		}
		return nil
	}

	w.metrics.IndexResyncErrors.With(common.Labels{"error": "failure"}).Inc()

	return err
}

func (w *SyncWorker) resyncIndex(onNewBlock bchain.OnNewBlockFunc, initialSync bool) error {
	remoteBestHash, err := w.chain.GetBestBlockHash()
	if err != nil {
		return err
	}
	localBestHeight, localBestHash, err := w.db.GetBestBlock()
	if err != nil {
		return err
	}
	// If the locally indexed block is the same as the best block on the network, we're done.
	if localBestHash == remoteBestHash {
		glog.Infof("resync: synced at %d %s", localBestHeight, localBestHash)
		return errSynced
	}
	if localBestHash != "" {
		remoteHash, err := w.chain.GetBlockHash(localBestHeight)
		// for some coins (eth) remote can be at lower best height after rollback
		if err != nil && err != bchain.ErrBlockNotFound {
			return err
		}
		if remoteHash != localBestHash {
			// forked - the remote hash differs from the local hash at the same height
			glog.Info("resync: local is forked at height ", localBestHeight, ", local hash ", localBestHash, ", remote hash ", remoteHash)
			return w.handleFork(localBestHeight, localBestHash, onNewBlock, initialSync)
		}
		glog.Info("resync: local at ", localBestHeight, " is behind")
		w.startHeight = localBestHeight + 1
	} else {
		// database is empty, start genesis
		glog.Info("resync: genesis from block ", w.startHeight)
	}
	w.startHash, err = w.chain.GetBlockHash(w.startHeight)
	if err != nil {
		return err
	}
	// if parallel operation is enabled and the number of blocks to be connected is large,
	// use parallel routine to load majority of blocks
	// use parallel sync only in case of initial sync because it puts the db to inconsistent state
	// or in case of ChainEthereumType if the tip is farther
	if w.syncWorkers > 1 && (initialSync || w.chain.GetChainParser().GetChainType() == bchain.ChainEthereumType) {
		remoteBestHeight, err := w.chain.GetBestBlockHeight()
		if err != nil {
			return err
		}
		if remoteBestHeight < w.startHeight {
			glog.Error("resync: error - remote best height ", remoteBestHeight, " less than sync start height ", w.startHeight)
			return errors.New("resync: remote best height error")
		}
		if initialSync {
			if remoteBestHeight-w.startHeight > uint32(w.syncChunk) {
				glog.Infof("resync: bulk sync of blocks %d-%d, using %d workers", w.startHeight, remoteBestHeight, w.syncWorkers)
				err = w.BulkConnectBlocks(w.startHeight, remoteBestHeight)
				if err != nil {
					return err
				}
				// after parallel load finish the sync using standard way,
				// new blocks may have been created in the meantime
				return w.resyncIndex(onNewBlock, initialSync)
			}
		}
		if w.chain.GetChainParser().GetChainType() == bchain.ChainEthereumType {
			syncWorkers := uint32(4)
			if remoteBestHeight-w.startHeight >= syncWorkers {
				glog.Infof("resync: parallel sync of blocks %d-%d, using %d workers", w.startHeight, remoteBestHeight, syncWorkers)
				err = w.ParallelConnectBlocks(onNewBlock, w.startHeight, remoteBestHeight, syncWorkers)
				if err != nil {
					return err
				}
				// after parallel load finish the sync using standard way,
				// new blocks may have been created in the meantime
				return w.resyncIndex(onNewBlock, initialSync)
			}
		}
	}
	err = w.connectBlocks(onNewBlock, initialSync)
	if err == errFork {
		return w.resyncIndex(onNewBlock, initialSync)
	}
	return err
}

func (w *SyncWorker) handleFork(localBestHeight uint32, localBestHash string, onNewBlock bchain.OnNewBlockFunc, initialSync bool) error {
	// find forked blocks, disconnect them and then synchronize again
	var height uint32
	hashes := []string{localBestHash}
	for height = localBestHeight - 1; ; height-- {
		local, err := w.db.GetBlockHash(height)
		if err != nil {
			return err
		}
		if local == "" {
			break
		}
		remote, err := w.chain.GetBlockHash(height)
		// for some coins (eth) remote can be at lower best height after rollback
		if err != nil && err != bchain.ErrBlockNotFound {
			return err
		}
		if local == remote {
			break
		}
		hashes = append(hashes, local)
	}
	if err := w.DisconnectBlocks(height+1, localBestHeight, hashes); err != nil {
		return err
	}
	return w.resyncIndex(onNewBlock, initialSync)
}

func (w *SyncWorker) connectBlocks(onNewBlock bchain.OnNewBlockFunc, initialSync bool) error {
	bch := make(chan blockResult, 8)
	done := make(chan struct{})
	defer close(done)

	go w.getBlockChain(bch, done)

	var lastRes, empty blockResult

	connect := func(res blockResult) error {
		lastRes = res
		if res.err != nil {
			return res.err
		}
		err := w.db.ConnectBlock(res.block)
		if err != nil {
			return err
		}
		if onNewBlock != nil {
			onNewBlock(res.block.Hash, res.block.Height)
		}
		w.metrics.BlockbookBestHeight.Set(float64(res.block.Height))
		if res.block.Height > 0 && res.block.Height%1000 == 0 {
			glog.Info("connected block ", res.block.Height, " ", res.block.Hash)
		}

		return nil
	}

	if initialSync {
	ConnectLoop:
		for {
			select {
			case <-w.chanOsSignal:
				glog.Info("connectBlocks interrupted at height ", lastRes.block.Height)
				return ErrOperationInterrupted
			case res := <-bch:
				if res == empty {
					break ConnectLoop
				}
				err := connect(res)
				if err != nil {
					return err
				}
			}
		}
	} else {
		// while regular sync, OS sig is handled by waitForSignalAndShutdown
		for res := range bch {
			err := connect(res)
			if err != nil {
				return err
			}
		}
	}

	if lastRes.block != nil {
		glog.Infof("resync: synced at %d %s", lastRes.block.Height, lastRes.block.Hash)
	}

	return nil
}

type hashHeight struct {
	hash   string
	height uint32
}

// ParallelConnectBlocks uses parallel goroutines to get data from blockchain daemon but keeps Blockbook in
func (w *SyncWorker) ParallelConnectBlocks(onNewBlock bchain.OnNewBlockFunc, lower, higher uint32, syncWorkers uint32) error {
	var err error
	var wg sync.WaitGroup
	bch := make([]chan *bchain.Block, syncWorkers)
	for i := 0; i < int(syncWorkers); i++ {
		bch[i] = make(chan *bchain.Block)
	}
	hch := make(chan hashHeight, syncWorkers)
	hchClosed := atomic.Value{}
	hchClosed.Store(false)
	writeBlockDone := make(chan struct{})
	terminating := make(chan struct{})
	writeBlockWorker := func() {
		defer close(writeBlockDone)
		lastBlock := lower - 1
	WriteBlockLoop:
		for {
			select {
			case b := <-bch[(lastBlock+1)%syncWorkers]:
				if b == nil {
					// channel is closed and empty - work is done
					break WriteBlockLoop
				}
				if b.Height != lastBlock+1 {
					glog.Fatal("writeBlockWorker skipped block, expected block ", lastBlock+1, ", new block ", b.Height)
				}
				err := w.db.ConnectBlock(b)
				if err != nil {
					glog.Fatal("writeBlockWorker ", b.Height, " ", b.Hash, " error ", err)
				}

				if onNewBlock != nil {
					onNewBlock(b.Hash, b.Height)
				}
				w.metrics.BlockbookBestHeight.Set(float64(b.Height))

				if b.Height > 0 && b.Height%1000 == 0 {
					glog.Info("connected block ", b.Height, " ", b.Hash)
				}

				lastBlock = b.Height
			case <-terminating:
				break WriteBlockLoop
			}
		}
		if err != nil {
			glog.Error("sync: ParallelConnectBlocks.Close error ", err)
		}
		glog.Info("WriteBlock exiting...")
	}
	for i := 0; i < int(syncWorkers); i++ {
		wg.Add(1)
		go w.getBlockWorker(i, syncWorkers, &wg, hch, bch, &hchClosed, terminating)
	}
	go writeBlockWorker()
	var hash string
ConnectLoop:
	for h := lower; h <= higher; {
		select {
		case <-w.chanOsSignal:
			glog.Info("connectBlocksParallel interrupted at height ", h)
			err = ErrOperationInterrupted
			// signal all workers to terminate their loops (error loops are interrupted below)
			close(terminating)
			break ConnectLoop
		default:
			hash, err = w.chain.GetBlockHash(h)
			if err != nil {
				glog.Error("GetBlockHash error ", err)
				w.metrics.IndexResyncErrors.With(common.Labels{"error": "failure"}).Inc()
				time.Sleep(time.Millisecond * 500)
				continue
			}
			hch <- hashHeight{hash, h}
			h++
		}
	}
	close(hch)
	// signal stop to workers that are in a error loop
	hchClosed.Store(true)
	// wait for workers and close bch that will stop writer loop
	wg.Wait()
	for i := 0; i < int(syncWorkers); i++ {
		close(bch[i])
	}
	<-writeBlockDone
	return err
}

func (w *SyncWorker) getBlockWorker(i int, syncWorkers uint32, wg *sync.WaitGroup, hch chan hashHeight, bch []chan *bchain.Block, hchClosed *atomic.Value, terminating chan struct{}) {
	defer wg.Done()
	var err error
	var block *bchain.Block
GetBlockLoop:
	for hh := range hch {
		for {
			block, err = w.chain.GetBlock(hh.hash, hh.height)
			if err != nil {
				// signal came while looping in the error loop
				if hchClosed.Load() == true {
					glog.Error("getBlockWorker ", i, " connect block error ", err, ". Exiting...")
					return
				}
				if err == bchain.ErrBlockNotFound {
					glog.Error("getBlockWorker ", i, " connect block ", hh.height, " ", hh.hash, " error ", err, ". Retrying...")
				} else {
					glog.Error("getBlockWorker ", i, " connect block error ", err, ". Retrying...")
				}
				w.metrics.IndexResyncErrors.With(common.Labels{"error": "failure"}).Inc()
				time.Sleep(time.Millisecond * 500)
			} else {
				break
			}
		}
		if w.dryRun {
			continue
		}
		select {
		case bch[hh.height%syncWorkers] <- block:
		case <-terminating:
			break GetBlockLoop
		}
	}
	glog.Info("getBlockWorker ", i, " exiting...")
}

// BulkConnectBlocks uses parallel goroutines to get data from blockchain daemon
func (w *SyncWorker) BulkConnectBlocks(lower, higher uint32) error {
	var err error
	var wg sync.WaitGroup
	bch := make([]chan *bchain.Block, w.syncWorkers)
	for i := 0; i < w.syncWorkers; i++ {
		bch[i] = make(chan *bchain.Block)
	}
	hch := make(chan hashHeight, w.syncWorkers)
	hchClosed := atomic.Value{}
	hchClosed.Store(false)
	writeBlockDone := make(chan struct{})
	terminating := make(chan struct{})
	writeBlockWorker := func() {
		defer close(writeBlockDone)
		bc, err := w.db.InitBulkConnect()
		if err != nil {
			glog.Error("sync: InitBulkConnect error ", err)
		}
		lastBlock := lower - 1
		keep := uint32(w.chain.GetChainParser().KeepBlockAddresses())
	WriteBlockLoop:
		for {
			select {
			case b := <-bch[(lastBlock+1)%uint32(w.syncWorkers)]:
				if b == nil {
					// channel is closed and empty - work is done
					break WriteBlockLoop
				}
				if b.Height != lastBlock+1 {
					glog.Fatal("writeBlockWorker skipped block, expected block ", lastBlock+1, ", new block ", b.Height)
				}
				err := bc.ConnectBlock(b, b.Height+keep > higher)
				if err != nil {
					glog.Fatal("writeBlockWorker ", b.Height, " ", b.Hash, " error ", err)
				}
				lastBlock = b.Height
			case <-terminating:
				break WriteBlockLoop
			}
		}
		err = bc.Close()
		if err != nil {
			glog.Error("sync: bulkconnect.Close error ", err)
		}
		glog.Info("WriteBlock exiting...")
	}
	for i := 0; i < w.syncWorkers; i++ {
		wg.Add(1)
		go w.getBlockWorker(i, uint32(w.syncWorkers), &wg, hch, bch, &hchClosed, terminating)
	}
	go writeBlockWorker()
	var hash string
	start := time.Now()
	msTime := time.Now().Add(1 * time.Minute)
ConnectLoop:
	for h := lower; h <= higher; {
		select {
		case <-w.chanOsSignal:
			glog.Info("connectBlocksParallel interrupted at height ", h)
			err = ErrOperationInterrupted
			// signal all workers to terminate their loops (error loops are interrupted below)
			close(terminating)
			break ConnectLoop
		default:
			hash, err = w.chain.GetBlockHash(h)
			if err != nil {
				glog.Error("GetBlockHash error ", err)
				w.metrics.IndexResyncErrors.With(common.Labels{"error": "failure"}).Inc()
				time.Sleep(time.Millisecond * 500)
				continue
			}
			hch <- hashHeight{hash, h}
			if h > 0 && h%1000 == 0 {
				w.metrics.BlockbookBestHeight.Set(float64(h))
				glog.Info("connecting block ", h, " ", hash, ", elapsed ", time.Since(start), " ", w.db.GetAndResetConnectBlockStats())
				start = time.Now()
			}
			if msTime.Before(time.Now()) {
				if glog.V(1) {
					glog.Info(w.db.GetMemoryStats())
				}
				w.metrics.IndexDBSize.Set(float64(w.db.DatabaseSizeOnDisk()))
				msTime = time.Now().Add(10 * time.Minute)
			}
			h++
		}
	}
	close(hch)
	// signal stop to workers that are in a error loop
	hchClosed.Store(true)
	// wait for workers and close bch that will stop writer loop
	wg.Wait()
	for i := 0; i < w.syncWorkers; i++ {
		close(bch[i])
	}
	<-writeBlockDone
	return err
}

type blockResult struct {
	block *bchain.Block
	err   error
}

func (w *SyncWorker) getBlockChain(out chan blockResult, done chan struct{}) {
	defer close(out)

	hash := w.startHash
	height := w.startHeight
	prevHash := ""
	// loop until error ErrBlockNotFound
	for {
		select {
		case <-done:
			return
		default:
		}
		block, err := w.chain.GetBlock(hash, height)
		if err != nil {
			if err == bchain.ErrBlockNotFound {
				break
			}
			out <- blockResult{err: err}
			return
		}
		if block.Prev != "" && prevHash != "" && prevHash != block.Prev {
			glog.Infof("sync: fork detected at height %d %s, local prevHash %s, remote prevHash %s", height, block.Hash, prevHash, block.Prev)
			out <- blockResult{err: errFork}
			return
		}
		prevHash = block.Hash
		hash = block.Next
		height++
		out <- blockResult{block: block}
	}
}

// DisconnectBlocks removes all data belonging to blocks in range lower-higher,
func (w *SyncWorker) DisconnectBlocks(lower uint32, higher uint32, hashes []string) error {
	glog.Infof("sync: disconnecting blocks %d-%d", lower, higher)
	ct := w.chain.GetChainParser().GetChainType()
	if ct == bchain.ChainBitcoinType {
		return w.db.DisconnectBlockRangeBitcoinType(lower, higher)
	} else if ct == bchain.ChainEthereumType {
		return w.db.DisconnectBlockRangeEthereumType(lower, higher)
	}
	return errors.New("Unknown chain type")
}
