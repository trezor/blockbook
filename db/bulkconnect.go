package db

import (
	"blockbook/bchain"
	"time"

	"github.com/golang/glog"
	"github.com/tecbot/gorocksdb"
)

// bulk connect
// in bulk mode the data are cached and stored to db in batches
// it speeds up the import in two ways:
// 1) balances and txAddresses are modified several times during the import, there is a chance that the modifications are done before write to DB
// 2) rocksdb seems to handle better fewer larger batches than continuous stream of smaller batches

type bulkAddresses struct {
	bi        BlockInfo
	addresses map[string][]outpoint
}

// BulkConnect is used to connect blocks in bulk, faster but if interrupted inconsistent way
type BulkConnect struct {
	d                  *RocksDB
	isUTXO             bool
	bulkAddresses      []bulkAddresses
	bulkAddressesCount int
	txAddressesMap     map[string]*TxAddresses
	balances           map[string]*AddrBalance
	height             uint32
}

const (
	maxBulkAddresses      = 200000
	maxBulkTxAddresses    = 500000
	partialStoreAddresses = maxBulkTxAddresses / 10
	maxBulkBalances       = 800000
	partialStoreBalances  = maxBulkBalances / 10
)

// InitBulkConnect initializes bulk connect and switches DB to inconsistent state
func (d *RocksDB) InitBulkConnect() (*BulkConnect, error) {
	bc := &BulkConnect{
		d:              d,
		isUTXO:         d.chainParser.IsUTXOChain(),
		txAddressesMap: make(map[string]*TxAddresses),
		balances:       make(map[string]*AddrBalance),
	}
	if err := d.SetInconsistentState(true); err != nil {
		return nil, err
	}
	glog.Info("rocksdb: bulk connect init, db set to inconsistent state")
	return bc, nil
}

func (b *BulkConnect) storeTxAddresses(wb *gorocksdb.WriteBatch, all bool) (int, int, error) {
	var txm map[string]*TxAddresses
	var sp int
	if all {
		txm = b.txAddressesMap
		b.txAddressesMap = make(map[string]*TxAddresses)
	} else {
		txm = make(map[string]*TxAddresses)
		for k, a := range b.txAddressesMap {
			// store all completely spent transactions, they will not be modified again
			r := true
			for _, o := range a.Outputs {
				if o.Spent == false {
					r = false
					break
				}
			}
			if r {
				txm[k] = a
				delete(b.txAddressesMap, k)
			}
		}
		sp = len(txm)
		// store some other random transactions if necessary
		if len(txm) < partialStoreAddresses {
			for k, a := range b.txAddressesMap {
				txm[k] = a
				delete(b.txAddressesMap, k)
				if len(txm) >= partialStoreAddresses {
					break
				}
			}
		}
	}
	if err := b.d.storeTxAddresses(wb, txm); err != nil {
		return 0, 0, err
	}
	return len(txm), sp, nil
}

func (b *BulkConnect) parallelStoreTxAddresses(c chan error, all bool) {
	defer close(c)
	start := time.Now()
	wb := gorocksdb.NewWriteBatch()
	defer wb.Destroy()
	count, sp, err := b.storeTxAddresses(wb, all)
	if err != nil {
		c <- err
		return
	}
	if err := b.d.db.Write(b.d.wo, wb); err != nil {
		c <- err
		return
	}
	glog.Info("rocksdb: height ", b.height, ", stored ", count, " (", sp, " spent) txAddresses, ", len(b.txAddressesMap), " remaining, done in ", time.Since(start))
	c <- nil
}

func (b *BulkConnect) storeBalances(wb *gorocksdb.WriteBatch, all bool) (int, error) {
	var bal map[string]*AddrBalance
	if all {
		bal = b.balances
		b.balances = make(map[string]*AddrBalance)
	} else {
		bal = make(map[string]*AddrBalance)
		// store some random balances
		for k, a := range b.balances {
			bal[k] = a
			delete(b.balances, k)
			if len(bal) >= partialStoreBalances {
				break
			}
		}
	}
	if err := b.d.storeBalances(wb, bal); err != nil {
		return 0, err
	}
	return len(bal), nil
}

func (b *BulkConnect) parallelStoreBalances(c chan error, all bool) {
	defer close(c)
	start := time.Now()
	wb := gorocksdb.NewWriteBatch()
	defer wb.Destroy()
	count, err := b.storeBalances(wb, all)
	if err != nil {
		c <- err
		return
	}
	if err := b.d.db.Write(b.d.wo, wb); err != nil {
		c <- err
		return
	}
	glog.Info("rocksdb: height ", b.height, ", stored ", count, " balances, ", len(b.balances), " remaining, done in ", time.Since(start))
	c <- nil
}

func (b *BulkConnect) storeBulkAddresses(wb *gorocksdb.WriteBatch) error {
	for _, ba := range b.bulkAddresses {
		if err := b.d.storeAddresses(wb, ba.bi.Height, ba.addresses); err != nil {
			return err
		}
		if err := b.d.writeHeight(wb, ba.bi.Height, &ba.bi, opInsert); err != nil {
			return err
		}
	}
	b.bulkAddressesCount = 0
	b.bulkAddresses = b.bulkAddresses[:0]
	return nil
}

// ConnectBlock connects block in bulk mode
func (b *BulkConnect) ConnectBlock(block *bchain.Block, storeBlockTxs bool) error {
	b.height = block.Height
	if !b.isUTXO {
		return b.d.ConnectBlock(block)
	}
	addresses := make(map[string][]outpoint)
	if err := b.d.processAddressesUTXO(block, addresses, b.txAddressesMap, b.balances); err != nil {
		return err
	}
	var storeAddressesChan, storeBalancesChan chan error
	var sa bool
	if len(b.txAddressesMap) > maxBulkTxAddresses || len(b.balances) > maxBulkBalances {
		sa = true
		if len(b.txAddressesMap)+partialStoreAddresses > maxBulkTxAddresses {
			storeAddressesChan = make(chan error)
			go b.parallelStoreTxAddresses(storeAddressesChan, false)
		}
		if len(b.balances)+partialStoreBalances > maxBulkBalances {
			storeBalancesChan = make(chan error)
			go b.parallelStoreBalances(storeBalancesChan, false)
		}
	}
	b.bulkAddresses = append(b.bulkAddresses, bulkAddresses{
		bi: BlockInfo{
			Hash:   block.Hash,
			Time:   block.Time,
			Txs:    uint32(len(block.Txs)),
			Size:   uint32(block.Size),
			Height: block.Height,
		},
		addresses: addresses,
	})
	b.bulkAddressesCount += len(addresses)
	// open WriteBatch only if going to write
	if sa || b.bulkAddressesCount > maxBulkAddresses || storeBlockTxs {
		start := time.Now()
		wb := gorocksdb.NewWriteBatch()
		defer wb.Destroy()
		bac := b.bulkAddressesCount
		if sa || b.bulkAddressesCount > maxBulkAddresses {
			if err := b.storeBulkAddresses(wb); err != nil {
				return err
			}
		}
		if storeBlockTxs {
			if err := b.d.storeAndCleanupBlockTxs(wb, block); err != nil {
				return err
			}
		}
		if err := b.d.db.Write(b.d.wo, wb); err != nil {
			return err
		}
		if bac > b.bulkAddressesCount {
			glog.Info("rocksdb: height ", b.height, ", stored ", bac, " addresses, done in ", time.Since(start))
		}
	}
	if storeAddressesChan != nil {
		if err := <-storeAddressesChan; err != nil {
			return err
		}
	}
	if storeBalancesChan != nil {
		if err := <-storeBalancesChan; err != nil {
			return err
		}
	}
	return nil
}

// Close flushes the cached data and switches DB from inconsistent state open
// after Close, the BulkConnect cannot be used
func (b *BulkConnect) Close() error {
	glog.Info("rocksdb: bulk connect closing")
	start := time.Now()
	storeAddressesChan := make(chan error)
	go b.parallelStoreTxAddresses(storeAddressesChan, true)
	storeBalancesChan := make(chan error)
	go b.parallelStoreBalances(storeBalancesChan, true)
	wb := gorocksdb.NewWriteBatch()
	defer wb.Destroy()
	bac := b.bulkAddressesCount
	if err := b.storeBulkAddresses(wb); err != nil {
		return err
	}
	if err := b.d.db.Write(b.d.wo, wb); err != nil {
		return err
	}
	glog.Info("rocksdb: height ", b.height, ", stored ", bac, " addresses, done in ", time.Since(start))
	if err := <-storeAddressesChan; err != nil {
		return err
	}
	if err := <-storeBalancesChan; err != nil {
		return err
	}
	if err := b.d.SetInconsistentState(false); err != nil {
		return err
	}
	glog.Info("rocksdb: bulk connect closed, db set to open state")
	b.d = nil
	return nil
}
