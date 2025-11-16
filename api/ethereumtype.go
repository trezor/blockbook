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

func (w *Worker) incrementRefetchInternalDataRetryCount(ie *db.BlockInternalDataError) {
	wb := grocksdb.NewWriteBatch()
	defer wb.Destroy()
	err := w.db.StoreBlockInternalDataErrorEthereumType(wb, &bchain.Block{
		BlockHeader: bchain.BlockHeader{
			Hash:   ie.Hash,
			Height: ie.Height,
		},
	}, ie.ErrorMessage, ie.Retries+1)
	if err != nil {
		glog.Errorf("StoreBlockInternalDataErrorEthereumType %d %s, error %v", ie.Height, ie.Hash, err)
	} else {
		w.db.WriteBatch(wb)
	}
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
			block, err := w.chain.GetBlock(ie.Hash, ie.Height)
			var blockSpecificData *bchain.EthereumBlockSpecificData
			if block != nil {
				blockSpecificData, _ = block.CoinSpecificData.(*bchain.EthereumBlockSpecificData)
			}
			if err != nil || block == nil || (blockSpecificData != nil && blockSpecificData.InternalDataError != "") {
				glog.Errorf("Refetching internal data for %d %s, error %v, retrying", ie.Height, ie.Hash, err)
				// try for second time to fetch the data - the 2nd attempt after the first unsuccessful has many times higher probability of success
				// probably something to do with data preloaded to cache on the backend
				block, err = w.chain.GetBlock(ie.Hash, ie.Height)
				if err != nil || block == nil {
					glog.Errorf("Refetching internal data for %d %s, error %v", ie.Height, ie.Hash, err)
					continue
				}
			}
			blockSpecificData, _ = block.CoinSpecificData.(*bchain.EthereumBlockSpecificData)
			if blockSpecificData != nil && blockSpecificData.InternalDataError != "" {
				glog.Errorf("Refetching internal data for %d %s, internal data error %v", ie.Height, ie.Hash, blockSpecificData.InternalDataError)
				w.incrementRefetchInternalDataRetryCount(ie)
			} else {
				err = w.db.ReconnectInternalDataToBlockEthereumType(block)
				if err != nil {
					glog.Errorf("ReconnectInternalDataToBlockEthereumType %d %s, error %v", ie.Height, ie.Hash, err)
				} else {
					glog.Infof("Refetching internal data for %d %s, success", ie.Height, ie.Hash)
				}
			}
		}
	}
	refetchInternalDataMux.Lock()
	refetchingInternalData = false
	refetchInternalDataMux.Unlock()
}
