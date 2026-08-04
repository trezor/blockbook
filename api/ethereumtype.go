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

// InternalDataErrorRetryLoop drains the internal data error queue without operator
// action. The first pass runs shortly after startup, while the backend cache is still
// warm from the failed sync fetches, then hourly. RefetchInternalData is a no-op while
// a pass is already running or the queue is empty.
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
		// keep the retry cap here so a permanently unfetchable block is not hammered
		// forever; only the operator's admin button resets it
		if err := w.RefetchInternalData(false); err != nil {
			glog.Errorf("InternalDataErrorRetryLoop: %v", err)
		}
	}
}

// RefetchInternalData starts a pass over the internal data error queue unless one is
// already running. resetRetries clears exhausted retry counts - set by the admin
// "Retry fetch" button so abandoned blocks can be revived, false for the auto-retry.
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

// storeBlockInternalDataError enqueues a block for retry, or updates its retry count.
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
				if blockSpecificData != nil {
					w.storeHealedContracts(blockSpecificData.Contracts)
				}
				glog.Infof("Refetching internal data for %d %s, success", ie.Height, ie.Hash)
			}
		}
	}
	refetchInternalDataMux.Lock()
	refetchingInternalData = false
	refetchInternalDataMux.Unlock()
}

// storeHealedContracts registers the contracts a healed block created or destroyed.
// The failed fetch is what produced them, so sync stored none, and the reconnect stores
// internal data only - without this the block regains its internal data but its
// contracts stay unregistered.
func (w *Worker) storeHealedContracts(contracts []bchain.ContractInfo) {
	for i := range contracts {
		ci := &contracts[i]
		if err := w.db.BackfillContractInfo(ci); err != nil {
			glog.Errorf("storeHealedContracts: BackfillContractInfo %s: %v", ci.Contract, err)
		}
	}
}

// getBlockRetryInternalData fetches a block, retrying once when the internal data are
// missing or errored - the second attempt usually succeeds, apparently because the
// first warms the backend cache. The returned block may still carry an
// InternalDataError; the caller decides how to handle that.
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
