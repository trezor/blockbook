package api

import (
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/linxGnu/grocksdb"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/db"
)

// refetch internal data
var refetchingInternalData = false
var refetchInternalDataMux sync.Mutex

func (w *Worker) IsRefetchingInternalData() bool {
	refetchInternalDataMux.Lock()
	defer refetchInternalDataMux.Unlock()
	return refetchingInternalData
}

const internalDataErrorRetryPeriod = time.Hour

// InternalDataErrorRetryLoop keeps the internal data of the index complete without
// operator action: blocks whose internal data could not be fetched while they were
// synced wait in the internal data error queue, and this loop periodically retries
// them. The underlying refetch routine runs at most once at a time, is a no-op on an
// empty queue and gives up on a block after its retry limit, so the periodic trigger
// is safe. The first pass runs shortly after startup to pick up failures from the
// previous run while the backend cache is warm.
func (w *Worker) InternalDataErrorRetryLoop() {
	if w.chainType != bchain.ChainEthereumType || !bchain.ProcessInternalTransactions {
		return
	}
	period := time.Minute
	for {
		time.Sleep(period)
		period = internalDataErrorRetryPeriod
		if common.IsInShutdown() {
			return
		}
		// periodic auto-retry keeps the retry cap so it never hammers a permanently
		// unfetchable block; an operator can force a reset via the admin page
		if err := w.RefetchInternalData(false); err != nil {
			glog.Errorf("InternalDataErrorRetryLoop: %v", err)
		}
	}
}

// RefetchInternalData starts a background pass over the internal data error queue if
// one is not already running. resetRetries requests that blocks which exhausted their
// retry budget be given a fresh start: it is set for an operator-triggered refetch (the
// admin "Retry fetch" button) so abandoned blocks are no longer permanently stuck, and
// left false for the periodic auto-retry so it keeps the cap and cannot hammer a
// permanently unfetchable block forever.
func (w *Worker) RefetchInternalData(resetRetries bool) error {
	refetchInternalDataMux.Lock()
	defer refetchInternalDataMux.Unlock()
	if !refetchingInternalData {
		refetchingInternalData = true
		go w.RefetchInternalDataRoutine(resetRetries)
	}
	return nil
}

const maxNumberOfRetires = 25

// storeBlockInternalDataError enqueues (or updates the retry count of) a block in
// the internal data error table, so it is retried by RefetchInternalDataRoutine and
// shown on the internal data errors admin page.
func (w *Worker) storeBlockInternalDataError(hash string, height uint32, message string, retries uint8) {
	wb := grocksdb.NewWriteBatch()
	defer wb.Destroy()
	if err := w.db.StoreBlockInternalDataErrorEthereumType(wb, &bchain.Block{
		BlockHeader: bchain.BlockHeader{Hash: hash, Height: height},
	}, message, retries); err != nil {
		glog.Errorf("StoreBlockInternalDataErrorEthereumType %d %s, error %v", height, hash, err)
		return
	}
	if err := w.db.WriteBatch(wb); err != nil {
		glog.Errorf("WriteBatch internal data error %d %s, error %v", height, hash, err)
	}
}

func (w *Worker) incrementRefetchInternalDataRetryCount(ie *db.BlockInternalDataError) {
	w.storeBlockInternalDataError(ie.Hash, ie.Height, ie.ErrorMessage, ie.Retries+1)
}

func (w *Worker) RefetchInternalDataRoutine(resetRetries bool) {
	internalErrors, err := w.db.GetBlockInternalDataErrorsEthereumType()
	if err == nil {
		for i := range internalErrors {
			ie := &internalErrors[i]
			// an operator-triggered refetch clears the retry count so a block that had
			// exhausted its budget gets a full fresh set of attempts (via the periodic
			// auto-retry too) instead of staying permanently abandoned
			if resetRetries {
				ie.Retries = 0
			}
			if ie.Retries >= maxNumberOfRetires {
				glog.Infof("Refetching internal data for %d %s, retries exceeded", ie.Height, ie.Hash)
				continue
			}
			glog.Infof("Refetching internal data for %d %s, retries %d", ie.Height, ie.Hash, ie.Retries)
			block, err := w.getBlockRetryInternalData(ie.Hash, ie.Height)
			if err != nil || block == nil {
				glog.Errorf("Refetching internal data for %d %s, error %v", ie.Height, ie.Hash, err)
				continue
			}
			blockSpecificData, _ := block.CoinSpecificData.(*bchain.EthereumBlockSpecificData)
			if blockSpecificData != nil && blockSpecificData.InternalDataError != "" {
				glog.Errorf("Refetching internal data for %d %s, internal data error %v", ie.Height, ie.Hash, blockSpecificData.InternalDataError)
				w.incrementRefetchInternalDataRetryCount(ie)
			} else if err = w.db.ReconnectInternalDataToBlockEthereumType(block); err != nil {
				glog.Errorf("ReconnectInternalDataToBlockEthereumType %d %s, error %v", ie.Height, ie.Hash, err)
			} else {
				glog.Infof("Refetching internal data for %d %s, success", ie.Height, ie.Hash)
			}
		}
	}
	refetchInternalDataMux.Lock()
	refetchingInternalData = false
	refetchInternalDataMux.Unlock()
}

// getBlockRetryInternalData fetches a block together with its internal data,
// retrying the backend fetch once when the first attempt fails or returns an
// internal data error. The second attempt has a much higher probability of
// success, probably because the first (failed) request preloads the data into
// the backend cache. The returned block may still carry an InternalDataError -
// the caller decides how to handle that.
func (w *Worker) getBlockRetryInternalData(hash string, height uint32) (*bchain.Block, error) {
	block, err := w.chain.GetBlock(hash, height)
	if err == nil && block != nil {
		if bsd, _ := block.CoinSpecificData.(*bchain.EthereumBlockSpecificData); bsd == nil || bsd.InternalDataError == "" {
			return block, nil
		}
	}
	glog.Errorf("getBlockRetryInternalData %d %s, first attempt failed (error %v), retrying", height, hash, err)
	return w.chain.GetBlock(hash, height)
}
