package db

import (
	"blockbook/bchain"
	"os"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/juju/errors"
)

// SyncWorker is handle to SyncWorker
type SyncWorker struct {
	db                     *RocksDB
	chain                  *bchain.BitcoinRPC
	syncWorkers, syncChunk int
	dryRun                 bool
	startHeight            uint32
	chanOsSignal           chan os.Signal
}

// NewSyncWorker creates new SyncWorker and returns its handle
func NewSyncWorker(db *RocksDB, chain *bchain.BitcoinRPC, syncWorkers, syncChunk int, minStartHeight uint32, dryRun bool, chanOsSignal chan os.Signal) (*SyncWorker, error) {
	return &SyncWorker{
		db:           db,
		chain:        chain,
		syncWorkers:  syncWorkers,
		syncChunk:    syncChunk,
		dryRun:       dryRun,
		startHeight:  minStartHeight,
		chanOsSignal: chanOsSignal,
	}, nil
}

// ResyncIndex synchronizes index to the top of the blockchain
// onNewBlock is called when new block is connected, but not in initial parallel sync
func (w *SyncWorker) ResyncIndex(onNewBlock func(hash string)) error {
	remote, err := w.chain.GetBestBlockHash()
	if err != nil {
		return err
	}
	localBestHeight, local, err := w.db.GetBestBlock()
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
		header, err = w.chain.GetBlockHeader(local)
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
				local, err = w.db.GetBlockHash(height)
				if err != nil {
					return err
				}
				remote, err = w.chain.GetBlockHash(height)
				if err != nil {
					return err
				}
				if local == remote {
					break
				}
			}
			err = w.db.DisconnectBlocks(height+1, localBestHeight)
			if err != nil {
				return err
			}
			return w.ResyncIndex(onNewBlock)
		}
	}

	var hash string
	if header != nil {
		glog.Info("resync: local is behind")
		hash = header.Next
		w.startHeight = localBestHeight
	} else {
		// If the local block is missing, we're indexing from the genesis block
		// or from the start block specified by flags
		glog.Info("resync: genesis from block ", w.startHeight)
		hash, err = w.chain.GetBlockHash(w.startHeight)
		if err != nil {
			return err
		}
	}

	// if parallel operation is enabled and the number of blocks to be connected is large,
	// use parallel routine to load majority of blocks
	if w.syncWorkers > 1 {
		chainBestHeight, err := w.chain.GetBestBlockHeight()
		if err != nil {
			return err
		}
		if chainBestHeight-w.startHeight > uint32(w.syncChunk) {
			glog.Infof("resync: parallel sync of blocks %d-%d, using %d workers", w.startHeight, chainBestHeight, w.syncWorkers)
			err = w.connectBlocksParallel(w.startHeight, chainBestHeight)
			if err != nil {
				return err
			}
			// after parallel load finish the sync using standard way,
			// new blocks may have been created in the meantime
			return w.ResyncIndex(onNewBlock)
		}
	}

	return w.connectBlocks(hash, onNewBlock)
}

func (w *SyncWorker) connectBlocks(hash string, onNewBlock func(hash string)) error {
	bch := make(chan blockResult, 8)
	done := make(chan struct{})
	defer close(done)

	go w.getBlockChain(hash, bch, done)

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
		glog.Infof("resync: synced on %d %s", lastRes.block.Height, lastRes.block.Hash)
	}

	return nil
}

func (w *SyncWorker) connectBlocksParallel(lower, higher uint32) error {
	type hashHeight struct {
		hash   string
		height uint32
	}
	var err error
	var wg sync.WaitGroup
	hch := make(chan hashHeight, w.syncWorkers)
	running := make([]bool, w.syncWorkers)
	work := func(i int) {
		defer wg.Done()
		var err error
		var block *bchain.Block
		for hh := range hch {
			running[i] = true
			for {
				block, err = w.chain.GetBlockWithoutHeader(hh.hash, hh.height)
				if err != nil {
					glog.Error("Connect block error ", err, ". Retrying...")
					time.Sleep(time.Millisecond * 500)
				} else {
					break
				}
			}
			if w.dryRun {
				running[i] = false
				continue
			}
			err = w.db.ConnectBlock(block)
			if err != nil {
				glog.Error("Connect block ", hh.height, " ", hh.hash, " error ", err)
			}
			running[i] = false
		}
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
			// wait for the workers to finish block
			i := 0
		WaitAgain:
			for ; i < 60; i++ {
				for _, r := range running {
					if r {
						glog.Info("Waiting ", i, "s for workers to finish ", running)
						time.Sleep(time.Millisecond * 1000)
						continue WaitAgain
					}
				}
				break
			}
			err = errors.Errorf("connectBlocksParallel interrupted at height %d", h)
			break ConnectLoop
		default:
			hash, err = w.chain.GetBlockHash(h)
			if err != nil {
				glog.Error("GetBlockHash error ", err)
				time.Sleep(time.Millisecond * 500)
				continue
			}
			hch <- hashHeight{hash, h}
			if h > 0 && h%1000 == 0 {
				glog.Info("connecting block ", h, " ", hash)
			}
			h++
		}
	}
	close(hch)
	wg.Wait()
	return err
}

func (w *SyncWorker) connectBlockChunk(lower, higher uint32) error {
	connected, err := w.isBlockConnected(higher)
	if err != nil || connected {
		// if higher is over the best block, continue with lower block, otherwise return error
		if e, ok := err.(*bchain.RPCError); !ok || e.Message != "Block height out of range" {
			return err
		}
	}

	height := lower
	hash, err := w.chain.GetBlockHash(lower)
	if err != nil {
		return err
	}

	for height <= higher {
		block, err := w.chain.GetBlock(hash)
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
		}
	}

	return nil
}

// ConnectBlocksParallelInChunks connect blocks in chunks
func (w *SyncWorker) ConnectBlocksParallelInChunks(lower, higher uint32) error {
	var wg sync.WaitGroup

	work := func(i int) {
		defer wg.Done()

		offset := uint32(w.syncChunk * i)
		stride := uint32(w.syncChunk * w.syncWorkers)

		for low := lower + offset; low <= higher; low += stride {
			high := low + uint32(w.syncChunk-1)
			if high > higher {
				high = higher
			}
			err := w.connectBlockChunk(low, high)
			if err != nil {
				if e, ok := err.(*bchain.RPCError); ok && (e.Message == "Block height out of range" || e.Message == "Block not found") {
					break
				}
				glog.Fatalf("connectBlocksParallel %d-%d %v", low, high, err)
			}
		}
	}
	for i := 0; i < w.syncWorkers; i++ {
		wg.Add(1)
		go work(i)
	}
	wg.Wait()

	return nil
}

func (w *SyncWorker) isBlockConnected(height uint32) (bool, error) {
	local, err := w.db.GetBlockHash(height)
	if err != nil {
		return false, err
	}
	remote, err := w.db.GetBlockHash(height)
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

func (w *SyncWorker) getBlockChain(hash string, out chan blockResult, done chan struct{}) {
	defer close(out)

	for hash != "" {
		select {
		case <-done:
			return
		default:
		}
		block, err := w.chain.GetBlock(hash)
		if err != nil {
			out <- blockResult{err: err}
			return
		}
		hash = block.Next
		out <- blockResult{block: block}
	}
}
