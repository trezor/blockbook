package api

import (
	"sync"

	"github.com/golang/glog"
	"github.com/linxGnu/grocksdb"
	"github.com/trezor/blockbook/bchain"
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

func (w *Worker) RefetchInternalData() error {
	refetchInternalDataMux.Lock()
	defer refetchInternalDataMux.Unlock()
	if !refetchingInternalData {
		refetchingInternalData = true
		go w.RefetchInternalDataRoutine()
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

func (w *Worker) RefetchInternalDataRoutine() {
	internalErrors, err := w.db.GetBlockInternalDataErrorsEthereumType()
	if err == nil {
		for i := range internalErrors {
			ie := &internalErrors[i]
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
