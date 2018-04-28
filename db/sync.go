package db

import (
	"blockbook/bchain"
	"blockbook/common"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/glog"
	"github.com/juju/errors"
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
}

// NewSyncWorker creates new SyncWorker and returns its handle
func NewSyncWorker(db *RocksDB, chain bchain.BlockChain, syncWorkers, syncChunk int, minStartHeight int, dryRun bool, chanOsSignal chan os.Signal, metrics *common.Metrics) (*SyncWorker, error) {
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
	}, nil
}

var errSynced = errors.New("synced")

// ResyncIndex synchronizes index to the top of the blockchain
// onNewBlock is called when new block is connected, but not in initial parallel sync
func (w *SyncWorker) ResyncIndex(onNewBlock func(hash string)) error {
	start := time.Now()

	err := w.resyncIndex(onNewBlock)

	switch err {
	case nil:
		d := time.Since(start)
		glog.Info("resync: finished in ", d)
		w.metrics.IndexResyncDuration.Observe(float64(d) / 1e6) // in milliseconds
		w.metrics.IndexDBSize.Set(float64(w.db.DatabaseSizeOnDisk()))
		fallthrough
	case errSynced:
		// this is not actually error but flag that resync wasn't necessary
		return nil
	}

	w.metrics.IndexResyncErrors.With(common.Labels{"error": err.Error()}).Inc()

	return err
}

func (w *SyncWorker) resyncIndex(onNewBlock func(hash string)) error {
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
			glog.Info("resync: local is forked at height ", localBestHeight, ", local hash ", localBestHash, ", remote hash", remoteHash)
			return w.handleFork(localBestHeight, localBestHash, onNewBlock)
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
	if w.syncWorkers > 1 {
		remoteBestHeight, err := w.chain.GetBestBlockHeight()
		if err != nil {
			return err
		}
		if remoteBestHeight < w.startHeight {
			glog.Error("resync: error - remote best height ", remoteBestHeight, " less than sync start height ", w.startHeight)
			return errors.New("resync: remote best height error")
		}
		if remoteBestHeight-w.startHeight > uint32(w.syncChunk) {
			glog.Infof("resync: parallel sync of blocks %d-%d, using %d workers", w.startHeight, remoteBestHeight, w.syncWorkers)
			err = w.ConnectBlocksParallel(w.startHeight, remoteBestHeight)
			if err != nil {
				return err
			}
			// after parallel load finish the sync using standard way,
			// new blocks may have been created in the meantime
			return w.resyncIndex(onNewBlock)
		}
	}
	return w.connectBlocks(onNewBlock)
}

func (w *SyncWorker) handleFork(localBestHeight uint32, localBestHash string, onNewBlock func(hash string)) error {
	// find forked blocks, disconnect them and then synchronize again
	var height uint32
	hashes := []string{localBestHash}
	for height = localBestHeight - 1; height >= 0; height-- {
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
	return w.resyncIndex(onNewBlock)
}

func (w *SyncWorker) connectBlocks(onNewBlock func(hash string)) error {
	bch := make(chan blockResult, 8)
	done := make(chan struct{})
	defer close(done)

	go w.getBlockChain(bch, done)

	var lastRes blockResult
	for res := range bch {
		lastRes = res
		if res.err != nil {
			return res.err
		}
		err := w.db.ConnectBlock(res.block)
		if err != nil {
			return err
		}
		if onNewBlock != nil {
			onNewBlock(res.block.Hash)
		}
	}

	if lastRes.block != nil {
		glog.Infof("resync: synced at %d %s", lastRes.block.Height, lastRes.block.Hash)
	}

	return nil
}

func (w *SyncWorker) ConnectBlocksParallel(lower, higher uint32) error {
	type hashHeight struct {
		hash   string
		height uint32
	}
	var err error
	var wg sync.WaitGroup
	hch := make(chan hashHeight, w.syncWorkers)
	hchClosed := atomic.Value{}
	hchClosed.Store(false)
	lastConnectedBlock := int(lower) - 1
	connectedCh := make([]chan struct{}, w.syncWorkers)
	var connectedMux sync.Mutex
	totalWaitDuration := time.Duration(0)
	totalWaitCount := 0
	work := func(i int) {
		defer wg.Done()
		var err error
		var block *bchain.Block
		for hh := range hch {
			for {
				block, err = w.chain.GetBlock(hh.hash, hh.height)
				if err != nil {
					// signal came while looping in the error loop
					if hchClosed.Load() == true {
						glog.Error("Worker ", i, " connect block error ", err, ". Exiting...")
						return
					}
					glog.Error("Worker ", i, " connect block error ", err, ". Retrying...")
					w.metrics.IndexResyncErrors.With(common.Labels{"error": err.Error()}).Inc()
					time.Sleep(time.Millisecond * 500)
				} else {
					break
				}
			}
			if w.dryRun {
				continue
			}
			// check if the block is the next in line to be connected
			// if not, wait for the previous block connect to complete
			chi := int(hh.height) % w.syncWorkers
			waitForBlock := false
			waitDuration := time.Duration(0)
			glog.Info(i, " Going to connect block ", hh.height)
			connectedMux.Lock()
			if uint32(lastConnectedBlock+1) != hh.height {
				if connectedCh[chi] != nil {
					glog.Fatal("Channel ", chi, " is not nil!")
				}
				connectedCh[chi] = make(chan struct{})
				waitForBlock = true
			}
			connectedMux.Unlock()
			if waitForBlock {
				start := time.Now()
				glog.Info(i, " Waiting for block ", hh.height, " ", chi)
				<-connectedCh[chi]
				if hchClosed.Load() == true {
					glog.Error("Worker ", i, " connect block error ", err, ". Exiting...")
					return
				}
				waitDuration = time.Since(start)
				connectedCh[chi] = nil
			}
			err = w.db.ConnectBlock(block)
			if err != nil {
				glog.Error("Worker ", i, " connect block ", hh.height, " ", hh.hash, " error ", err)
			}
			connectedMux.Lock()
			if lastConnectedBlock < int(hh.height) {
				lastConnectedBlock = int(hh.height)
			}
			chi = (chi + 1) % w.syncWorkers
			if connectedCh[chi] != nil {
				glog.Info(i, " closing channel ", chi)
				close(connectedCh[chi])
				connectedCh[chi] = nil
			}
			totalWaitDuration += waitDuration
			if waitDuration > 0 {
				totalWaitCount++
			}
			glog.Info("Connected block ", hh.height)
			connectedMux.Unlock()
		}
		glog.Info("Worker ", i, " exiting...")
	}
	for i := 0; i < w.syncWorkers; i++ {
		wg.Add(1)
		go work(i)
	}

	var hash string
ConnectLoop:
	for h := lower; h <= higher; {
		select {
		case <-w.chanOsSignal:
			err = errors.Errorf("connectBlocksParallel interrupted at height %d", h)
			break ConnectLoop
		default:
			hash, err = w.chain.GetBlockHash(h)
			if err != nil {
				glog.Error("GetBlockHash error ", err)
				w.metrics.IndexResyncErrors.With(common.Labels{"error": err.Error()}).Inc()
				time.Sleep(time.Millisecond * 500)
				continue
			}
			hch <- hashHeight{hash, h}
			if h > 0 && h%1000 == 0 {
				glog.Info("connecting block ", h, " ", hash, " block wait time ", totalWaitDuration, " wait count ", totalWaitCount)
			}
			h++
		}
	}
	close(hch)
	// signal stop to workers that are in w.chain.GetBlockWithoutHeader error loop
	hchClosed.Store(true)
	connectedMux.Lock()
	for _, ch := range connectedCh {
		if ch != nil {
			close(ch)
		}
	}
	connectedMux.Unlock()
	wg.Wait()
	return err
}

func (w *SyncWorker) connectBlockChunk(lower, higher uint32) error {
	connected, err := w.isBlockConnected(higher)
	if err != nil || connected {
		// if higher is over the best block, continue with lower block, otherwise return error
		if err != bchain.ErrBlockNotFound {
			return err
		}
	}

	height := lower
	hash, err := w.chain.GetBlockHash(lower)
	if err != nil {
		return err
	}

	for height <= higher {
		block, err := w.chain.GetBlock(hash, height)
		if err != nil {
			return err
		}
		hash = block.Next
		height = block.Height + 1
		if w.dryRun {
			continue
		}
		err = w.db.ConnectBlock(block)
		if err != nil {
			return err
		}
		if block.Height%1000 == 0 {
			glog.Info("connected block ", block.Height, " ", block.Hash)
			go w.metrics.IndexDBSize.Set(float64(w.db.DatabaseSizeOnDisk()))
		}
	}

	return nil
}

func (w *SyncWorker) isBlockConnected(height uint32) (bool, error) {
	local, err := w.db.GetBlockHash(height)
	if err != nil {
		return false, err
	}
	remote, err := w.chain.GetBlockHash(height)
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

func (w *SyncWorker) getBlockChain(out chan blockResult, done chan struct{}) {
	defer close(out)

	hash := w.startHash
	height := w.startHeight

	// some coins do not return Next hash
	// must loop until error
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
		hash = block.Next
		height++
		out <- blockResult{block: block}
	}
}

// DisconnectBlocks removes all data belonging to blocks in range lower-higher,
// using block data from blockchain, if they are available,
// otherwise doing full scan
func (w *SyncWorker) DisconnectBlocks(lower uint32, higher uint32, hashes []string) error {
	glog.Infof("sync: disconnecting blocks %d-%d", lower, higher)
	// if the chain uses Block to Addresses mapping, always use DisconnectBlockRange
	if w.chain.GetChainParser().KeepBlockAddresses() > 0 {
		return w.db.DisconnectBlockRange(lower, higher)
	}
	blocks := make([]*bchain.Block, len(hashes))
	var err error
	// get all blocks first to see if we can avoid full scan
	for i, hash := range hashes {
		blocks[i], err = w.chain.GetBlock(hash, 0)
		if err != nil {
			// cannot get a block, we must do full range scan
			return w.db.DisconnectBlockRange(lower, higher)
		}
	}
	// then disconnect one after another
	for i, block := range blocks {
		glog.Info("Disconnecting block ", (int(higher) - i), " ", block.Hash)
		if err = w.db.DisconnectBlock(block); err != nil {
			return err
		}
	}
	return nil
}
