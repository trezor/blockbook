package db

import (
	"blockbook/bchain"

	"github.com/golang/glog"
)

// TxCache is handle to TxCacheServer
type TxCache struct {
	db    *RocksDB
	chain bchain.BlockChain
}

// NewTxCache creates new TxCache interface and returns its handle
func NewTxCache(db *RocksDB, chain bchain.BlockChain) (*TxCache, error) {
	return &TxCache{
		db:    db,
		chain: chain,
	}, nil
}

// GetTransaction returns transaction either from RocksDB or if not present from blockchain
// it the transaction is confirmed, it is stored in the RocksDB
func (c *TxCache) GetTransaction(txid string, bestheight uint32) (*bchain.Tx, error) {
	tx, h, err := c.db.GetTx(txid)
	if err != nil {
		return nil, err
	}
	if tx != nil {
		tx.Confirmations = bestheight - h
		return tx, nil
	}
	tx, err = c.chain.GetTransaction(txid)
	if err != nil {
		return nil, err
	}
	// do not cache mempool transactions
	if tx.Confirmations > 0 {
		err = c.db.PutTx(tx, bestheight-tx.Confirmations, tx.Blocktime)
		// do not return caching error, only log it
		if err != nil {
			glog.Error("PutTx error ", err)
		}
	}
	return tx, nil
}
