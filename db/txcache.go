package db

import (
	"strings"

	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
	"github.com/trezor/blockbook/common"
)

// TxCache is handle to TxCacheServer
type TxCache struct {
	db        *RocksDB
	chain     bchain.BlockChain
	metrics   *common.Metrics
	is        *common.InternalState
	enabled   bool
	chainType bchain.ChainType
}

// NewTxCache creates new TxCache interface and returns its handle
func NewTxCache(db *RocksDB, chain bchain.BlockChain, metrics *common.Metrics, is *common.InternalState, enabled bool) (*TxCache, error) {
	if !enabled {
		glog.Info("txcache: disabled")
	}
	return &TxCache{
		db:        db,
		chain:     chain,
		metrics:   metrics,
		is:        is,
		enabled:   enabled,
		chainType: chain.GetChainParser().GetChainType(),
	}, nil
}

// GetTransaction returns transaction either from RocksDB or if not present from blockchain
// it the transaction is confirmed, it is stored in the RocksDB
func (c *TxCache) GetTransaction(txid string) (*bchain.Tx, int, error) {
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
			_, bestheight, _, _ := c.is.GetSyncState()
			// the best height drops before a disconnect commits, so a cached tx can sit above
			// it for a moment; report the mined minimum instead of underflowing
			if bestheight < h {
				tx.Confirmations = 1
			} else {
				tx.Confirmations = bestheight - h + 1
			}
			c.metrics.TxCacheEfficiency.With(common.Labels{"status": "hit"}).Inc()
			return tx, int(h), nil
		}
	}
	tx, err = c.chain.GetTransaction(txid)
	if err != nil {
		return nil, 0, err
	}
	c.metrics.TxCacheEfficiency.With(common.Labels{"status": "miss"}).Inc()
	// cache only confirmed transactions
	if tx.Confirmations > 0 {
		// only txs of indexed blocks are evicted on reorg, a tx from any other block must not be cached
		var cache bool
		if c.chainType == bchain.ChainBitcoinType {
			ta, err := c.db.GetTxAddresses(txid)
			if err != nil {
				return nil, 0, err
			}
			switch {
			case ta == nil:
				// the transaction may not yet be indexed, in that case:
				if tx.BlockHeight > 0 {
					// Check if the tx height value is set.
					h = tx.BlockHeight
				} else {
					// Get the height from the backend's bestblock.
					h, err = c.chain.GetBestBlockHeight()
					if err != nil {
						return nil, 0, err
					}
				}
			default:
				h = ta.Height
				cache = c.enabled
			}
		} else if c.chainType == bchain.ChainEthereumType {
			h, err = eth.GetHeightFromTx(tx)
			if err != nil {
				return nil, 0, err
			}
			cache = c.enabled && c.inIndexedBlock(tx, txid, h)
		} else {
			return nil, 0, errors.New("Unknown chain type")
		}
		if cache {
			err = c.db.PutTx(tx, h, tx.Blocktime)
			// do not return caching error, only log it
			if err != nil {
				glog.Warning("PutTx ", tx.Txid, ",error ", err)
			} else if !c.inIndexedBlock(tx, txid, h) {
				// a disconnect that landed between the check above and PutTx evicted nothing
				if err = c.db.DeleteTx(txid); err != nil {
					glog.Warning("DeleteTx ", tx.Txid, ",error ", err)
				}
			}
		}
	} else {
		return tx, -1, nil
	}
	return tx, int(h), nil
}

// inIndexedBlock reports whether the index currently holds the tx in the block at height h.
func (c *TxCache) inIndexedBlock(tx *bchain.Tx, txid string, h uint32) bool {
	if c.chainType == bchain.ChainBitcoinType {
		ta, err := c.db.GetTxAddresses(txid)
		return err == nil && ta != nil && ta.Height == h
	}
	indexed, err := c.db.GetBlockHash(h)
	if err != nil || indexed == "" {
		return false
	}
	csd, ok := tx.CoinSpecificData.(bchain.EthereumSpecificData)
	// Tron reports confirmations for unsolidified txs too, their nil receipt is the only pending marker
	if !ok || csd.Tx == nil || csd.Receipt == nil {
		return false
	}
	// Tron txs carry no block hash, the indexed height is the only check available there
	if csd.Tx.BlockHash == "" {
		return true
	}
	return strings.EqualFold(indexed, csd.Tx.BlockHash)
}
