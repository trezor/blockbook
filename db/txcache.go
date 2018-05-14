package db

import (
	"blockbook/bchain"
	"blockbook/common"

	"github.com/golang/glog"
)

// TxCache is handle to TxCacheServer
type TxCache struct {
	db      *RocksDB
	chain   bchain.BlockChain
	metrics *common.Metrics
	enabled bool
}

// NewTxCache creates new TxCache interface and returns its handle
func NewTxCache(db *RocksDB, chain bchain.BlockChain, metrics *common.Metrics, enabled bool) (*TxCache, error) {
	if !enabled {
		glog.Info("txcache: disabled")
	}
	return &TxCache{
		db:      db,
		chain:   chain,
		metrics: metrics,
		enabled: enabled,
	}, nil
}

// GetTransaction returns transaction either from RocksDB or if not present from blockchain
// it the transaction is confirmed, it is stored in the RocksDB
func (c *TxCache) GetTransaction(txid string, bestheight uint32) (*bchain.Tx, uint32, error) {
	var tx *bchain.Tx
	var h uint32
	var err error
	if c.enabled {
		tx, h, err = c.db.GetTx(txid)
		if err != nil {
			return nil, 0, err
		}
		if tx != nil {
			// number of confirmations is not stored in cache, they change all the time
			tx.Confirmations = bestheight - h + 1
			c.metrics.TxCacheEfficiency.With(common.Labels{"status": "hit"}).Inc()
			return tx, h, nil
		}
	}
	tx, err = c.chain.GetTransaction(txid)
	if err != nil {
		return nil, 0, err
	}
	c.metrics.TxCacheEfficiency.With(common.Labels{"status": "miss"}).Inc()
	// do not cache mempool transactions
	if tx.Confirmations > 0 {
		// the transaction in the currently best block has 1 confirmation
		h = bestheight - tx.Confirmations + 1
		if c.enabled {
			err = c.db.PutTx(tx, h, tx.Blocktime)
			// do not return caching error, only log it
			if err != nil {
				glog.Error("PutTx error ", err)
			}
		}
	} else {
		h = 0
	}
	return tx, h, nil
}
