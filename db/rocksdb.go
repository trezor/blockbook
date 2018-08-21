package db

import (
	"blockbook/bchain"
	"blockbook/common"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/bsm/go-vlq"
	"github.com/golang/glog"
	"github.com/juju/errors"

	"github.com/tecbot/gorocksdb"
)

// iterator creates snapshot, which takes lots of resources
// when doing huge scan, it is better to close it and reopen from time to time to free the resources
const refreshIterator = 5000000
const packedHeightBytes = 4
const dbVersion = 3
const maxAddrIDLen = 1024

// RepairRocksDB calls RocksDb db repair function
func RepairRocksDB(name string) error {
	glog.Infof("rocksdb: repair")
	opts := gorocksdb.NewDefaultOptions()
	return gorocksdb.RepairDb(name, opts)
}

// RocksDB handle
type RocksDB struct {
	path        string
	db          *gorocksdb.DB
	wo          *gorocksdb.WriteOptions
	ro          *gorocksdb.ReadOptions
	cfh         []*gorocksdb.ColumnFamilyHandle
	chainParser bchain.BlockChainParser
	is          *common.InternalState
	metrics     *common.Metrics
}

const (
	cfDefault = iota
	cfHeight
	cfAddresses
	cfTxAddresses
	cfAddressBalance
	cfBlockTxs
	cfTransactions
)

var cfNames = []string{"default", "height", "addresses", "txAddresses", "addressBalance", "blockTxs", "transactions"}

func openDB(path string) (*gorocksdb.DB, []*gorocksdb.ColumnFamilyHandle, error) {
	c := gorocksdb.NewLRUCache(8 << 30) // 8GB
	fp := gorocksdb.NewBloomFilter(10)
	bbto := gorocksdb.NewDefaultBlockBasedTableOptions()
	bbto.SetBlockSize(16 << 10) // 16kB
	bbto.SetBlockCache(c)
	bbto.SetFilterPolicy(fp)

	optsNoCompression := gorocksdb.NewDefaultOptions()
	optsNoCompression.SetBlockBasedTableFactory(bbto)
	optsNoCompression.SetCreateIfMissing(true)
	optsNoCompression.SetCreateIfMissingColumnFamilies(true)
	optsNoCompression.SetMaxBackgroundCompactions(4)
	optsNoCompression.SetMaxBackgroundFlushes(2)
	optsNoCompression.SetBytesPerSync(1 << 20)    // 1MB
	optsNoCompression.SetWriteBufferSize(1 << 27) // 128MB
	optsNoCompression.SetMaxOpenFiles(25000)
	optsNoCompression.SetCompression(gorocksdb.NoCompression)

	optsLZ4 := gorocksdb.NewDefaultOptions()
	optsLZ4.SetBlockBasedTableFactory(bbto)
	optsLZ4.SetCreateIfMissing(true)
	optsLZ4.SetCreateIfMissingColumnFamilies(true)
	optsLZ4.SetMaxBackgroundCompactions(4)
	optsLZ4.SetMaxBackgroundFlushes(2)
	optsLZ4.SetBytesPerSync(1 << 20)    // 1MB
	optsLZ4.SetWriteBufferSize(1 << 27) // 128MB
	optsLZ4.SetMaxOpenFiles(25000)
	optsLZ4.SetCompression(gorocksdb.LZ4HCCompression)

	// opts for addresses are different:
	// no bloom filter - from documentation: If most of your queries are executed using iterators, you shouldn't set bloom filter
	bbtoAddresses := gorocksdb.NewDefaultBlockBasedTableOptions()
	bbtoAddresses.SetBlockSize(16 << 10) // 16kB
	bbtoAddresses.SetBlockCache(c)       // 8GB

	optsAddresses := gorocksdb.NewDefaultOptions()
	optsAddresses.SetBlockBasedTableFactory(bbtoAddresses)
	optsAddresses.SetCreateIfMissing(true)
	optsAddresses.SetCreateIfMissingColumnFamilies(true)
	optsAddresses.SetMaxBackgroundCompactions(4)
	optsAddresses.SetMaxBackgroundFlushes(2)
	optsAddresses.SetBytesPerSync(1 << 20)    // 1MB
	optsAddresses.SetWriteBufferSize(1 << 27) // 128MB
	optsAddresses.SetMaxOpenFiles(25000)
	optsAddresses.SetCompression(gorocksdb.LZ4HCCompression)

	// default, height, addresses, txAddresses, addressBalance, blockTxids, transactions
	fcOptions := []*gorocksdb.Options{optsLZ4, optsLZ4, optsAddresses, optsLZ4, optsLZ4, optsLZ4, optsLZ4}

	db, cfh, err := gorocksdb.OpenDbColumnFamilies(optsNoCompression, path, cfNames, fcOptions)
	if err != nil {
		return nil, nil, err
	}
	return db, cfh, nil
}

// NewRocksDB opens an internal handle to RocksDB environment.  Close
// needs to be called to release it.
func NewRocksDB(path string, parser bchain.BlockChainParser, metrics *common.Metrics) (d *RocksDB, err error) {
	glog.Infof("rocksdb: open %s, version %v", path, dbVersion)
	db, cfh, err := openDB(path)
	wo := gorocksdb.NewDefaultWriteOptions()
	ro := gorocksdb.NewDefaultReadOptions()
	ro.SetFillCache(false)
	return &RocksDB{path, db, wo, ro, cfh, parser, nil, metrics}, nil
}

func (d *RocksDB) closeDB() error {
	for _, h := range d.cfh {
		h.Destroy()
	}
	d.db.Close()
	d.db = nil
	return nil
}

// Close releases the RocksDB environment opened in NewRocksDB.
func (d *RocksDB) Close() error {
	if d.db != nil {
		// store the internal state of the app
		if d.is != nil && d.is.DbState == common.DbStateOpen {
			d.is.DbState = common.DbStateClosed
			if err := d.StoreInternalState(d.is); err != nil {
				glog.Infof("internalState: ", err)
			}
		}
		glog.Infof("rocksdb: close")
		d.closeDB()
		d.wo.Destroy()
		d.ro.Destroy()
	}
	return nil
}

// Reopen reopens the database
// It closes and reopens db, nobody can access the database during the operation!
func (d *RocksDB) Reopen() error {
	err := d.closeDB()
	if err != nil {
		return err
	}
	d.db = nil
	db, cfh, err := openDB(d.path)
	if err != nil {
		return err
	}
	d.db, d.cfh = db, cfh
	return nil
}

// GetTransactions finds all input/output transactions for address
// Transaction are passed to callback function.
func (d *RocksDB) GetTransactions(address string, lower uint32, higher uint32, fn func(txid string, vout uint32, isOutput bool) error) (err error) {
	if glog.V(1) {
		glog.Infof("rocksdb: address get %s %d-%d ", address, lower, higher)
	}
	addrID, err := d.chainParser.GetAddrIDFromAddress(address)
	if err != nil {
		return err
	}

	kstart := packAddressKey(addrID, lower)
	kstop := packAddressKey(addrID, higher)

	it := d.db.NewIteratorCF(d.ro, d.cfh[cfAddresses])
	defer it.Close()

	for it.Seek(kstart); it.Valid(); it.Next() {
		key := it.Key().Data()
		val := it.Value().Data()
		if bytes.Compare(key, kstop) > 0 {
			break
		}
		outpoints, err := d.unpackOutpoints(val)
		if err != nil {
			return err
		}
		if glog.V(2) {
			glog.Infof("rocksdb: output %s: %s", hex.EncodeToString(key), hex.EncodeToString(val))
		}
		for _, o := range outpoints {
			var vout uint32
			var isOutput bool
			if o.index < 0 {
				vout = uint32(^o.index)
				isOutput = false
			} else {
				vout = uint32(o.index)
				isOutput = true
			}
			tx, err := d.chainParser.UnpackTxid(o.btxID)
			if err != nil {
				return err
			}
			if err := fn(tx, vout, isOutput); err != nil {
				return err
			}
		}
	}
	return nil
}

const (
	opInsert = 0
	opDelete = 1
)

// ConnectBlock indexes addresses in the block and stores them in db
func (d *RocksDB) ConnectBlock(block *bchain.Block) error {
	return d.writeBlock(block, opInsert)
}

// DisconnectBlock removes addresses in the block from the db
func (d *RocksDB) DisconnectBlock(block *bchain.Block) error {
	return d.writeBlock(block, opDelete)
}

func (d *RocksDB) writeBlock(block *bchain.Block, op int) error {
	wb := gorocksdb.NewWriteBatch()
	defer wb.Destroy()

	if glog.V(2) {
		switch op {
		case opInsert:
			glog.Infof("rocksdb: insert %d %s", block.Height, block.Hash)
		case opDelete:
			glog.Infof("rocksdb: delete %d %s", block.Height, block.Hash)
		}
	}

	isUTXO := d.chainParser.IsUTXOChain()

	if err := d.writeHeight(wb, block, op); err != nil {
		return err
	}
	if isUTXO {
		if op == opDelete {
			// block does not contain mapping tx-> input address, which is necessary to recreate
			// unspentTxs; therefore it is not possible to DisconnectBlocks this way
			return errors.New("DisconnectBlock is not supported for UTXO chains")
		}
		addresses := make(map[string][]outpoint)
		txAddressesMap := make(map[string]*TxAddresses)
		balances := make(map[string]*AddrBalance)
		if err := d.processAddressesUTXO(block, addresses, txAddressesMap, balances); err != nil {
			return err
		}
		if err := d.storeAddresses(wb, block.Height, addresses); err != nil {
			return err
		}
		if err := d.storeTxAddresses(wb, txAddressesMap); err != nil {
			return err
		}
		if err := d.storeBalances(wb, balances); err != nil {
			return err
		}
		if err := d.storeAndCleanupBlockTxs(wb, block); err != nil {
			return err
		}
	} else {
		if err := d.writeAddressesNonUTXO(wb, block, op); err != nil {
			return err
		}
	}

	return d.db.Write(d.wo, wb)
}

// BulkConnect is used to connect blocks in bulk, faster but if interrupted inconsistent way
type bulkAddresses struct {
	height    uint32
	addresses map[string][]outpoint
}

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
	maxBulkAddresses      = 400000
	maxBulkTxAddresses    = 2000000
	partialStoreAddresses = maxBulkTxAddresses / 10
	maxBulkBalances       = 2500000
	partialStoreBalances  = maxBulkBalances / 10
)

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

func (b *BulkConnect) storeTxAddresses(c chan error, all bool) {
	defer close(c)
	start := time.Now()
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
	wb := gorocksdb.NewWriteBatch()
	defer wb.Destroy()
	if err := b.d.storeTxAddresses(wb, txm); err != nil {
		c <- err
	} else {
		if err := b.d.db.Write(b.d.wo, wb); err != nil {
			c <- err
		}
	}
	glog.Info("rocksdb: height ", b.height, ", stored ", len(txm), " (", sp, " spent) txAddresses, ", len(b.txAddressesMap), " remaining, done in ", time.Since(start))
}

func (b *BulkConnect) storeBalances(c chan error, all bool) {
	defer close(c)
	start := time.Now()
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
	wb := gorocksdb.NewWriteBatch()
	defer wb.Destroy()
	if err := b.d.storeBalances(wb, bal); err != nil {
		c <- err
	} else {
		if err := b.d.db.Write(b.d.wo, wb); err != nil {
			c <- err
		}
	}
	glog.Info("rocksdb: height ", b.height, ", stored ", len(bal), " balances, ", len(b.balances), " remaining, done in ", time.Since(start))
}

func (b *BulkConnect) storeBulkAddresses(wb *gorocksdb.WriteBatch) error {
	for _, ba := range b.bulkAddresses {
		if err := b.d.storeAddresses(wb, ba.height, ba.addresses); err != nil {
			return err
		}
	}
	b.bulkAddressesCount = 0
	b.bulkAddresses = b.bulkAddresses[:0]
	return nil
}

func (b *BulkConnect) ConnectBlock(block *bchain.Block, storeBlockTxs bool) error {
	b.height = block.Height
	if !b.isUTXO {
		return b.d.ConnectBlock(block)
	}
	wb := gorocksdb.NewWriteBatch()
	defer wb.Destroy()
	addresses := make(map[string][]outpoint)
	if err := b.d.processAddressesUTXO(block, addresses, b.txAddressesMap, b.balances); err != nil {
		return err
	}
	start := time.Now()
	var sa bool
	var storeAddressesChan, storeBalancesChan chan error
	if len(b.txAddressesMap) > maxBulkTxAddresses || len(b.balances) > maxBulkBalances {
		sa = true
		if len(b.txAddressesMap)+partialStoreAddresses > maxBulkTxAddresses {
			storeAddressesChan = make(chan error)
			go b.storeTxAddresses(storeAddressesChan, false)
		}
		if len(b.balances)+partialStoreBalances > maxBulkBalances {
			storeBalancesChan = make(chan error)
			go b.storeBalances(storeBalancesChan, false)
		}
	}
	b.bulkAddresses = append(b.bulkAddresses, bulkAddresses{
		height:    block.Height,
		addresses: addresses,
	})
	b.bulkAddressesCount += len(addresses)
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
	if err := b.d.writeHeight(wb, block, opInsert); err != nil {
		return err
	}
	if err := b.d.db.Write(b.d.wo, wb); err != nil {
		return err
	}
	if bac > b.bulkAddressesCount {
		glog.Info("rocksdb: height ", b.height, ", stored ", bac, " addresses, done in ", time.Since(start))
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

func (b *BulkConnect) Close() error {
	glog.Info("rocksdb: bulk connect closing")
	start := time.Now()
	storeAddressesChan := make(chan error)
	go b.storeTxAddresses(storeAddressesChan, true)
	storeBalancesChan := make(chan error)
	go b.storeBalances(storeBalancesChan, true)
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

// Addresses index

type outpoint struct {
	btxID []byte
	index int32
}

type TxInput struct {
	addrID   []byte
	ValueSat big.Int
}

func (ti *TxInput) Addresses(p bchain.BlockChainParser) ([]string, error) {
	// TODO - we will need AddressesFromAddrID parser method, this will not work for ZCash
	return p.OutputScriptToAddresses(ti.addrID)
}

type TxOutput struct {
	addrID   []byte
	Spent    bool
	ValueSat big.Int
}

func (to *TxOutput) Addresses(p bchain.BlockChainParser) ([]string, error) {
	// TODO - we will need AddressesFromAddrID parser method, this will not work for ZCash
	return p.OutputScriptToAddresses(to.addrID)
}

type TxAddresses struct {
	Height  uint32
	Inputs  []TxInput
	Outputs []TxOutput
}

type AddrBalance struct {
	Txs        uint32
	SentSat    big.Int
	BalanceSat big.Int
}

func (ab *AddrBalance) ReceivedSat() *big.Int {
	var r big.Int
	r.Add(&ab.BalanceSat, &ab.SentSat)
	return &r
}

type blockTxs struct {
	btxID  []byte
	inputs []outpoint
}

func (d *RocksDB) resetValueSatToZero(valueSat *big.Int, addrID []byte, logText string) {
	ad, err := d.chainParser.OutputScriptToAddresses(addrID)
	had := hex.EncodeToString(addrID)
	if err != nil {
		glog.Warningf("rocksdb: unparsable address hex '%v' reached negative %s %v, resetting to 0. Parser error %v", had, logText, valueSat.String(), err)
	} else {
		glog.Warningf("rocksdb: address %v hex '%v' reached negative %s %v, resetting to 0", ad, had, logText, valueSat.String())
	}
	valueSat.SetInt64(0)
}

func (d *RocksDB) processAddressesUTXO(block *bchain.Block, addresses map[string][]outpoint, txAddressesMap map[string]*TxAddresses, balances map[string]*AddrBalance) error {
	blockTxIDs := make([][]byte, len(block.Txs))
	blockTxAddresses := make([]*TxAddresses, len(block.Txs))
	// first process all outputs so that inputs can point to txs in this block
	for txi := range block.Txs {
		tx := &block.Txs[txi]
		btxID, err := d.chainParser.PackTxid(tx.Txid)
		if err != nil {
			return err
		}
		blockTxIDs[txi] = btxID
		ta := TxAddresses{Height: block.Height}
		ta.Outputs = make([]TxOutput, len(tx.Vout))
		txAddressesMap[string(btxID)] = &ta
		blockTxAddresses[txi] = &ta
		for i, output := range tx.Vout {
			tao := &ta.Outputs[i]
			tao.ValueSat = output.ValueSat
			addrID, err := d.chainParser.GetAddrIDFromVout(&output)
			if err != nil || len(addrID) == 0 || len(addrID) > maxAddrIDLen {
				if err != nil {
					// do not log ErrAddressMissing, transactions can be without to address (for example eth contracts)
					if err != bchain.ErrAddressMissing {
						glog.Warningf("rocksdb: addrID: %v - height %d, tx %v, output %v", err, block.Height, tx.Txid, output)
					}
				} else {
					glog.Infof("rocksdb: height %d, tx %v, vout %v, skipping addrID of length %d", block.Height, tx.Txid, i, len(addrID))
				}
				continue
			}
			tao.addrID = addrID
			strAddrID := string(addrID)
			// check that the address was used already in this block
			o, processed := addresses[strAddrID]
			if processed {
				// check that the address was already used in this tx
				processed = processedInTx(o, btxID)
			}
			addresses[strAddrID] = append(o, outpoint{
				btxID: btxID,
				index: int32(i),
			})
			ab, e := balances[strAddrID]
			if !e {
				ab, err = d.getAddrIDBalance(addrID)
				if err != nil {
					return err
				}
				if ab == nil {
					ab = &AddrBalance{}
				}
				balances[strAddrID] = ab
			}
			// add number of trx in balance only once, address can be multiple times in tx
			if !processed {
				ab.Txs++
			}
			ab.BalanceSat.Add(&ab.BalanceSat, &output.ValueSat)
		}
	}
	// process inputs
	for txi := range block.Txs {
		tx := &block.Txs[txi]
		spendingTxid := blockTxIDs[txi]
		ta := blockTxAddresses[txi]
		ta.Inputs = make([]TxInput, len(tx.Vin))
		for i, input := range tx.Vin {
			tai := &ta.Inputs[i]
			btxID, err := d.chainParser.PackTxid(input.Txid)
			if err != nil {
				// do not process inputs without input txid
				if err == bchain.ErrTxidMissing {
					continue
				}
				return err
			}
			stxID := string(btxID)
			ita, e := txAddressesMap[stxID]
			if !e {
				ita, err = d.getTxAddresses(btxID)
				if err != nil {
					return err
				}
				if ita == nil {
					glog.Warningf("rocksdb: height %d, tx %v, input tx %v not found in txAddresses", block.Height, tx.Txid, input.Txid)
					continue
				}
				txAddressesMap[stxID] = ita
			}
			if len(ita.Outputs) <= int(input.Vout) {
				glog.Warningf("rocksdb: height %d, tx %v, input tx %v vout %v is out of bounds of stored tx", block.Height, tx.Txid, input.Txid, input.Vout)
				continue
			}
			ot := &ita.Outputs[int(input.Vout)]
			if ot.Spent {
				glog.Warningf("rocksdb: height %d, tx %v, input tx %v vout %v is double spend", block.Height, tx.Txid, input.Txid, input.Vout)
			}
			tai.addrID = ot.addrID
			tai.ValueSat = ot.ValueSat
			// mark the output as spent in tx
			ot.Spent = true
			if len(ot.addrID) == 0 {
				glog.Warningf("rocksdb: height %d, tx %v, input tx %v vout %v skipping empty address", block.Height, tx.Txid, input.Txid, input.Vout)
				continue
			}
			strAddrID := string(ot.addrID)
			// check that the address was used already in this block
			o, processed := addresses[strAddrID]
			if processed {
				// check that the address was already used in this tx
				processed = processedInTx(o, spendingTxid)
			}
			addresses[strAddrID] = append(o, outpoint{
				btxID: spendingTxid,
				index: ^int32(i),
			})
			ab, e := balances[strAddrID]
			if !e {
				ab, err = d.getAddrIDBalance(ot.addrID)
				if err != nil {
					return err
				}
				if ab == nil {
					ab = &AddrBalance{}
				}
				balances[strAddrID] = ab
			}
			// add number of trx in balance only once, address can be multiple times in tx
			if !processed {
				ab.Txs++
			}
			ab.BalanceSat.Sub(&ab.BalanceSat, &ot.ValueSat)
			if ab.BalanceSat.Sign() < 0 {
				d.resetValueSatToZero(&ab.BalanceSat, ot.addrID, "balance")
			}
			ab.SentSat.Add(&ab.SentSat, &ot.ValueSat)
		}
	}
	return nil
}

func processedInTx(o []outpoint, btxID []byte) bool {
	for _, op := range o {
		if bytes.Equal(btxID, op.btxID) {
			return true
		}
	}
	return false
}

func (d *RocksDB) storeAddresses(wb *gorocksdb.WriteBatch, height uint32, addresses map[string][]outpoint) error {
	for addrID, outpoints := range addresses {
		ba := []byte(addrID)
		key := packAddressKey(ba, height)
		val := d.packOutpoints(outpoints)
		wb.PutCF(d.cfh[cfAddresses], key, val)
	}
	return nil
}

func (d *RocksDB) storeTxAddresses(wb *gorocksdb.WriteBatch, am map[string]*TxAddresses) error {
	varBuf := make([]byte, maxPackedBigintBytes)
	buf := make([]byte, 1024)
	for txID, ta := range am {
		buf = packTxAddresses(ta, buf, varBuf)
		wb.PutCF(d.cfh[cfTxAddresses], []byte(txID), buf)
	}
	return nil
}

func (d *RocksDB) storeBalances(wb *gorocksdb.WriteBatch, abm map[string]*AddrBalance) error {
	// allocate buffer big enough for number of txs + 2 bigints
	buf := make([]byte, vlq.MaxLen32+2*maxPackedBigintBytes)
	for addrID, ab := range abm {
		// balance with 0 transactions is removed from db - happens in disconnect
		if ab == nil || ab.Txs <= 0 {
			wb.DeleteCF(d.cfh[cfAddressBalance], []byte(addrID))
		} else {
			l := packVaruint(uint(ab.Txs), buf)
			ll := packBigint(&ab.SentSat, buf[l:])
			l += ll
			ll = packBigint(&ab.BalanceSat, buf[l:])
			l += ll
			wb.PutCF(d.cfh[cfAddressBalance], []byte(addrID), buf[:l])
		}
	}
	return nil
}

func (d *RocksDB) storeAndCleanupBlockTxs(wb *gorocksdb.WriteBatch, block *bchain.Block) error {
	pl := d.chainParser.PackedTxidLen()
	buf := make([]byte, 0, pl*len(block.Txs))
	varBuf := make([]byte, vlq.MaxLen64)
	zeroTx := make([]byte, pl)
	for i := range block.Txs {
		tx := &block.Txs[i]
		o := make([]outpoint, len(tx.Vin))
		for v := range tx.Vin {
			vin := &tx.Vin[v]
			btxID, err := d.chainParser.PackTxid(vin.Txid)
			if err != nil {
				// do not process inputs without input txid
				if err == bchain.ErrTxidMissing {
					btxID = zeroTx
				} else {
					return err
				}
			}
			o[v].btxID = btxID
			o[v].index = int32(vin.Vout)
		}
		btxID, err := d.chainParser.PackTxid(tx.Txid)
		if err != nil {
			return err
		}
		buf = append(buf, btxID...)
		l := packVaruint(uint(len(o)), varBuf)
		buf = append(buf, varBuf[:l]...)
		buf = append(buf, d.packOutpoints(o)...)
	}
	key := packUint(block.Height)
	wb.PutCF(d.cfh[cfBlockTxs], key, buf)
	keep := d.chainParser.KeepBlockAddresses()
	// cleanup old block address
	if block.Height > uint32(keep) {
		for rh := block.Height - uint32(keep); rh < block.Height; rh-- {
			key = packUint(rh)
			val, err := d.db.GetCF(d.ro, d.cfh[cfBlockTxs], key)
			if err != nil {
				return err
			}
			if val.Size() == 0 {
				break
			}
			val.Free()
			d.db.DeleteCF(d.wo, d.cfh[cfBlockTxs], key)
		}
	}
	return nil
}

func (d *RocksDB) getBlockTxs(height uint32) ([]blockTxs, error) {
	pl := d.chainParser.PackedTxidLen()
	val, err := d.db.GetCF(d.ro, d.cfh[cfBlockTxs], packUint(height))
	if err != nil {
		return nil, err
	}
	defer val.Free()
	buf := val.Data()
	bt := make([]blockTxs, 0)
	for i := 0; i < len(buf); {
		if len(buf)-i < pl {
			glog.Error("rocksdb: Inconsistent data in blockTxs ", hex.EncodeToString(buf))
			return nil, errors.New("Inconsistent data in blockTxs")
		}
		txid := make([]byte, pl)
		copy(txid, buf[i:])
		i += pl
		o, ol, err := d.unpackNOutpoints(buf[i:])
		if err != nil {
			glog.Error("rocksdb: Inconsistent data in blockTxs ", hex.EncodeToString(buf))
			return nil, errors.New("Inconsistent data in blockTxs")
		}
		bt = append(bt, blockTxs{
			btxID:  txid,
			inputs: o,
		})
		i += ol
	}
	return bt, nil
}

func (d *RocksDB) getAddrIDBalance(addrID []byte) (*AddrBalance, error) {
	val, err := d.db.GetCF(d.ro, d.cfh[cfAddressBalance], addrID)
	if err != nil {
		return nil, err
	}
	defer val.Free()
	buf := val.Data()
	// 3 is minimum length of addrBalance - 1 byte txs, 1 byte sent, 1 byte balance
	if len(buf) < 3 {
		return nil, nil
	}
	txs, l := unpackVaruint(buf)
	sentSat, sl := unpackBigint(buf[l:])
	balanceSat, _ := unpackBigint(buf[l+sl:])
	return &AddrBalance{
		Txs:        uint32(txs),
		SentSat:    sentSat,
		BalanceSat: balanceSat,
	}, nil
}

// GetAddressBalance returns address balance for an address or nil if address not found
func (d *RocksDB) GetAddressBalance(address string) (*AddrBalance, error) {
	addrID, err := d.chainParser.GetAddrIDFromAddress(address)
	if err != nil {
		return nil, err
	}
	return d.getAddrIDBalance(addrID)
}

func (d *RocksDB) getTxAddresses(btxID []byte) (*TxAddresses, error) {
	val, err := d.db.GetCF(d.ro, d.cfh[cfTxAddresses], btxID)
	if err != nil {
		return nil, err
	}
	defer val.Free()
	buf := val.Data()
	// 2 is minimum length of addrBalance - 1 byte height, 1 byte inputs len, 1 byte outputs len
	if len(buf) < 3 {
		return nil, nil
	}
	return unpackTxAddresses(buf)
}

// GetTxAddresses returns TxAddresses for given txid or nil if not found
func (d *RocksDB) GetTxAddresses(txid string) (*TxAddresses, error) {
	btxID, err := d.chainParser.PackTxid(txid)
	if err != nil {
		return nil, err
	}
	return d.getTxAddresses(btxID)
}

func packTxAddresses(ta *TxAddresses, buf []byte, varBuf []byte) []byte {
	buf = buf[:0]
	l := packVaruint(uint(ta.Height), varBuf)
	buf = append(buf, varBuf[:l]...)
	l = packVaruint(uint(len(ta.Inputs)), varBuf)
	buf = append(buf, varBuf[:l]...)
	for i := range ta.Inputs {
		buf = appendTxInput(&ta.Inputs[i], buf, varBuf)
	}
	l = packVaruint(uint(len(ta.Outputs)), varBuf)
	buf = append(buf, varBuf[:l]...)
	for i := range ta.Outputs {
		buf = appendTxOutput(&ta.Outputs[i], buf, varBuf)
	}
	return buf
}

func appendTxInput(txi *TxInput, buf []byte, varBuf []byte) []byte {
	la := len(txi.addrID)
	l := packVaruint(uint(la), varBuf)
	buf = append(buf, varBuf[:l]...)
	buf = append(buf, txi.addrID...)
	l = packBigint(&txi.ValueSat, varBuf)
	buf = append(buf, varBuf[:l]...)
	return buf
}

func appendTxOutput(txo *TxOutput, buf []byte, varBuf []byte) []byte {
	la := len(txo.addrID)
	if txo.Spent {
		la = ^la
	}
	l := packVarint(la, varBuf)
	buf = append(buf, varBuf[:l]...)
	buf = append(buf, txo.addrID...)
	l = packBigint(&txo.ValueSat, varBuf)
	buf = append(buf, varBuf[:l]...)
	return buf
}

func unpackTxAddresses(buf []byte) (*TxAddresses, error) {
	ta := TxAddresses{}
	height, l := unpackVaruint(buf)
	ta.Height = uint32(height)
	inputs, ll := unpackVaruint(buf[l:])
	l += ll
	ta.Inputs = make([]TxInput, inputs)
	for i := uint(0); i < inputs; i++ {
		l += unpackTxInput(&ta.Inputs[i], buf[l:])
	}
	outputs, ll := unpackVaruint(buf[l:])
	l += ll
	ta.Outputs = make([]TxOutput, outputs)
	for i := uint(0); i < outputs; i++ {
		l += unpackTxOutput(&ta.Outputs[i], buf[l:])
	}
	return &ta, nil
}

func unpackTxInput(ti *TxInput, buf []byte) int {
	al, l := unpackVaruint(buf)
	ti.addrID = make([]byte, al)
	copy(ti.addrID, buf[l:l+int(al)])
	al += uint(l)
	ti.ValueSat, l = unpackBigint(buf[al:])
	return l + int(al)
}

func unpackTxOutput(to *TxOutput, buf []byte) int {
	al, l := unpackVarint(buf)
	if al < 0 {
		to.Spent = true
		al = ^al
	}
	to.addrID = make([]byte, al)
	copy(to.addrID, buf[l:l+al])
	al += l
	to.ValueSat, l = unpackBigint(buf[al:])
	return l + al
}

func (d *RocksDB) packOutpoints(outpoints []outpoint) []byte {
	buf := make([]byte, 0)
	bvout := make([]byte, vlq.MaxLen32)
	for _, o := range outpoints {
		l := packVarint32(o.index, bvout)
		buf = append(buf, []byte(o.btxID)...)
		buf = append(buf, bvout[:l]...)
	}
	return buf
}

func (d *RocksDB) unpackOutpoints(buf []byte) ([]outpoint, error) {
	txidUnpackedLen := d.chainParser.PackedTxidLen()
	outpoints := make([]outpoint, 0)
	for i := 0; i < len(buf); {
		btxID := append([]byte(nil), buf[i:i+txidUnpackedLen]...)
		i += txidUnpackedLen
		vout, voutLen := unpackVarint32(buf[i:])
		i += voutLen
		outpoints = append(outpoints, outpoint{
			btxID: btxID,
			index: vout,
		})
	}
	return outpoints, nil
}

func (d *RocksDB) unpackNOutpoints(buf []byte) ([]outpoint, int, error) {
	txidUnpackedLen := d.chainParser.PackedTxidLen()
	n, p := unpackVaruint(buf)
	outpoints := make([]outpoint, n)
	for i := uint(0); i < n; i++ {
		if p+txidUnpackedLen >= len(buf) {
			return nil, 0, errors.New("Inconsistent data in unpackNOutpoints")
		}
		btxID := append([]byte(nil), buf[p:p+txidUnpackedLen]...)
		p += txidUnpackedLen
		vout, voutLen := unpackVarint32(buf[p:])
		p += voutLen
		outpoints[i] = outpoint{
			btxID: btxID,
			index: vout,
		}
	}
	return outpoints, p, nil
}

func (d *RocksDB) addAddrIDToRecords(op int, wb *gorocksdb.WriteBatch, records map[string][]outpoint, addrID []byte, btxid []byte, vout int32, bh uint32) error {
	if len(addrID) > 0 {
		if len(addrID) > maxAddrIDLen {
			glog.Infof("rocksdb: block %d, skipping addrID of length %d", bh, len(addrID))
		} else {
			strAddrID := string(addrID)
			records[strAddrID] = append(records[strAddrID], outpoint{
				btxID: btxid,
				index: vout,
			})
			if op == opDelete {
				// remove transactions from cache
				d.internalDeleteTx(wb, btxid)
			}
		}
	}
	return nil
}

func (d *RocksDB) writeAddressesNonUTXO(wb *gorocksdb.WriteBatch, block *bchain.Block, op int) error {
	addresses := make(map[string][]outpoint)
	for _, tx := range block.Txs {
		btxID, err := d.chainParser.PackTxid(tx.Txid)
		if err != nil {
			return err
		}
		for _, output := range tx.Vout {
			addrID, err := d.chainParser.GetAddrIDFromVout(&output)
			if err != nil {
				// do not log ErrAddressMissing, transactions can be without to address (for example eth contracts)
				if err != bchain.ErrAddressMissing {
					glog.Warningf("rocksdb: addrID: %v - height %d, tx %v, output %v", err, block.Height, tx.Txid, output)
				}
				continue
			}
			err = d.addAddrIDToRecords(op, wb, addresses, addrID, btxID, int32(output.N), block.Height)
			if err != nil {
				return err
			}
		}
		// store inputs in format txid ^index
		for _, input := range tx.Vin {
			for i, a := range input.Addresses {
				addrID, err := d.chainParser.GetAddrIDFromAddress(a)
				if err != nil {
					glog.Warningf("rocksdb: addrID: %v - %d %s", err, block.Height, addrID)
					continue
				}
				err = d.addAddrIDToRecords(op, wb, addresses, addrID, btxID, int32(^i), block.Height)
				if err != nil {
					return err
				}
			}
		}
	}
	for addrID, outpoints := range addresses {
		key := packAddressKey([]byte(addrID), block.Height)
		switch op {
		case opInsert:
			val := d.packOutpoints(outpoints)
			wb.PutCF(d.cfh[cfAddresses], key, val)
		case opDelete:
			wb.DeleteCF(d.cfh[cfAddresses], key)
		}
	}
	return nil
}

// Block index

// GetBestBlock returns the block hash of the block with highest height in the db
func (d *RocksDB) GetBestBlock() (uint32, string, error) {
	it := d.db.NewIteratorCF(d.ro, d.cfh[cfHeight])
	defer it.Close()
	if it.SeekToLast(); it.Valid() {
		bestHeight := unpackUint(it.Key().Data())
		val, err := d.chainParser.UnpackBlockHash(it.Value().Data())
		if glog.V(1) {
			glog.Infof("rocksdb: bestblock %d %s", bestHeight, val)
		}
		return bestHeight, val, err
	}
	return 0, "", nil
}

// GetBlockHash returns block hash at given height or empty string if not found
func (d *RocksDB) GetBlockHash(height uint32) (string, error) {
	key := packUint(height)
	val, err := d.db.GetCF(d.ro, d.cfh[cfHeight], key)
	if err != nil {
		return "", err
	}
	defer val.Free()
	return d.chainParser.UnpackBlockHash(val.Data())
}

func (d *RocksDB) writeHeight(wb *gorocksdb.WriteBatch, block *bchain.Block, op int) error {
	key := packUint(block.Height)

	switch op {
	case opInsert:
		val, err := d.chainParser.PackBlockHash(block.Hash)
		if err != nil {
			return err
		}
		wb.PutCF(d.cfh[cfHeight], key, val)
	case opDelete:
		wb.DeleteCF(d.cfh[cfHeight], key)
	}
	return nil
}

// Disconnect blocks

func (d *RocksDB) allAddressesScan(lower uint32, higher uint32) ([][]byte, [][]byte, error) {
	glog.Infof("db: doing full scan of addresses column")
	addrKeys := [][]byte{}
	addrValues := [][]byte{}
	var totalOutputs, count uint64
	var seekKey []byte
	for {
		var key []byte
		it := d.db.NewIteratorCF(d.ro, d.cfh[cfAddresses])
		if totalOutputs == 0 {
			it.SeekToFirst()
		} else {
			it.Seek(seekKey)
			it.Next()
		}
		for count = 0; it.Valid() && count < refreshIterator; it.Next() {
			totalOutputs++
			count++
			key = it.Key().Data()
			l := len(key)
			if l > packedHeightBytes {
				height := unpackUint(key[l-packedHeightBytes : l])
				if height >= lower && height <= higher {
					addrKey := make([]byte, len(key))
					copy(addrKey, key)
					addrKeys = append(addrKeys, addrKey)
					value := it.Value().Data()
					addrValue := make([]byte, len(value))
					copy(addrValue, value)
					addrValues = append(addrValues, addrValue)
				}
			}
		}
		seekKey = make([]byte, len(key))
		copy(seekKey, key)
		valid := it.Valid()
		it.Close()
		if !valid {
			break
		}
	}
	glog.Infof("rocksdb: scanned %d addresses, found %d to disconnect", totalOutputs, len(addrKeys))
	return addrKeys, addrValues, nil
}

func (d *RocksDB) disconnectTxAddresses(wb *gorocksdb.WriteBatch, height uint32, txid string, inputs []outpoint, txa *TxAddresses,
	txAddressesToUpdate map[string]*TxAddresses, balances map[string]*AddrBalance) error {
	addresses := make(map[string]struct{})
	getAddressBalance := func(addrID []byte) (*AddrBalance, error) {
		var err error
		s := string(addrID)
		b, fb := balances[s]
		if !fb {
			b, err = d.getAddrIDBalance(addrID)
			if err != nil {
				return nil, err
			}
			balances[s] = b
		}
		return b, nil
	}
	for i, t := range txa.Inputs {
		if len(t.addrID) > 0 {
			s := string(t.addrID)
			_, exist := addresses[s]
			if !exist {
				addresses[s] = struct{}{}
			}
			b, err := getAddressBalance(t.addrID)
			if err != nil {
				return err
			}
			if b != nil {
				// subtract number of txs only once
				if !exist {
					b.Txs--
				}
				b.SentSat.Sub(&b.SentSat, &t.ValueSat)
				if b.SentSat.Sign() < 0 {
					d.resetValueSatToZero(&b.SentSat, t.addrID, "sent amount")
				}
				b.BalanceSat.Add(&b.BalanceSat, &t.ValueSat)
			} else {
				ad, _ := d.chainParser.OutputScriptToAddresses(t.addrID)
				had := hex.EncodeToString(t.addrID)
				glog.Warningf("Balance for address %s (%s) not found", ad, had)
			}
			s = string(inputs[i].btxID)
			sa, exist := txAddressesToUpdate[s]
			if !exist {
				sa, err = d.getTxAddresses(inputs[i].btxID)
				if err != nil {
					return err
				}
				txAddressesToUpdate[s] = sa
			}
			sa.Outputs[inputs[i].index].Spent = false
		}
	}
	for _, t := range txa.Outputs {
		if len(t.addrID) > 0 {
			s := string(t.addrID)
			_, exist := addresses[s]
			if !exist {
				addresses[s] = struct{}{}
			}
			b, err := getAddressBalance(t.addrID)
			if err != nil {
				return err
			}
			if b != nil {
				// subtract number of txs only once
				if !exist {
					b.Txs--
				}
				b.BalanceSat.Sub(&b.BalanceSat, &t.ValueSat)
				if b.BalanceSat.Sign() < 0 {
					d.resetValueSatToZero(&b.BalanceSat, t.addrID, "balance")
				}
			} else {
				ad, _ := d.chainParser.OutputScriptToAddresses(t.addrID)
				had := hex.EncodeToString(t.addrID)
				glog.Warningf("Balance for address %s (%s) not found", ad, had)
			}
		}
	}
	for a := range addresses {
		key := packAddressKey([]byte(a), height)
		wb.DeleteCF(d.cfh[cfAddresses], key)
	}
	return nil
}

// DisconnectBlockRangeUTXO removes all data belonging to blocks in range lower-higher
// if they are in the range kept in the cfBlockTxids column
func (d *RocksDB) DisconnectBlockRangeUTXO(lower uint32, higher uint32) error {
	glog.Infof("db: disconnecting blocks %d-%d", lower, higher)
	blocks := make([][]blockTxs, higher-lower+1)
	for height := lower; height <= higher; height++ {
		blockTxs, err := d.getBlockTxs(height)
		if err != nil {
			return err
		}
		if len(blockTxs) == 0 {
			return errors.Errorf("Cannot disconnect blocks with height %v and lower. It is necessary to rebuild index.", height)
		}
		blocks[height-lower] = blockTxs
	}
	wb := gorocksdb.NewWriteBatch()
	defer wb.Destroy()
	txAddressesToUpdate := make(map[string]*TxAddresses)
	txsToDelete := make(map[string]struct{})
	balances := make(map[string]*AddrBalance)
	for height := higher; height >= lower; height-- {
		blockTxs := blocks[height-lower]
		glog.Info("Disconnecting block ", height, " containing ", len(blockTxs), " transactions")
		// go backwards to avoid interim negative balance
		// when connecting block, amount is first in tx on the output side, then in another tx on the input side
		// when disconnecting, it must be done backwards
		for i := len(blockTxs) - 1; i >= 0; i-- {
			txid := blockTxs[i].btxID
			s := string(txid)
			txsToDelete[s] = struct{}{}
			txa, err := d.getTxAddresses(txid)
			if err != nil {
				return err
			}
			if txa == nil {
				ut, _ := d.chainParser.UnpackTxid(txid)
				glog.Warning("TxAddress for txid ", ut, " not found")
				continue
			}
			if err := d.disconnectTxAddresses(wb, height, s, blockTxs[i].inputs, txa, txAddressesToUpdate, balances); err != nil {
				return err
			}
		}
		key := packUint(height)
		wb.DeleteCF(d.cfh[cfBlockTxs], key)
		wb.DeleteCF(d.cfh[cfHeight], key)
	}
	d.storeTxAddresses(wb, txAddressesToUpdate)
	d.storeBalances(wb, balances)
	for s := range txsToDelete {
		b := []byte(s)
		wb.DeleteCF(d.cfh[cfTransactions], b)
		wb.DeleteCF(d.cfh[cfTxAddresses], b)
	}
	err := d.db.Write(d.wo, wb)
	if err == nil {
		glog.Infof("rocksdb: blocks %d-%d disconnected", lower, higher)
	}
	return err
}

// DisconnectBlockRangeNonUTXO performs full range scan to remove a range of blocks
// it is very slow operation
func (d *RocksDB) DisconnectBlockRangeNonUTXO(lower uint32, higher uint32) error {
	glog.Infof("db: disconnecting blocks %d-%d", lower, higher)
	addrKeys, _, err := d.allAddressesScan(lower, higher)
	if err != nil {
		return err
	}
	glog.Infof("rocksdb: about to disconnect %d addresses ", len(addrKeys))
	wb := gorocksdb.NewWriteBatch()
	defer wb.Destroy()
	for _, addrKey := range addrKeys {
		if glog.V(2) {
			glog.Info("address ", hex.EncodeToString(addrKey))
		}
		// delete address:height from the index
		wb.DeleteCF(d.cfh[cfAddresses], addrKey)
	}
	for height := lower; height <= higher; height++ {
		if glog.V(2) {
			glog.Info("height ", height)
		}
		wb.DeleteCF(d.cfh[cfHeight], packUint(height))
	}
	err = d.db.Write(d.wo, wb)
	if err == nil {
		glog.Infof("rocksdb: blocks %d-%d disconnected", lower, higher)
	}
	return err
}

func dirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			size += info.Size()
		}
		return err
	})
	return size, err
}

// DatabaseSizeOnDisk returns size of the database in bytes
func (d *RocksDB) DatabaseSizeOnDisk() int64 {
	size, err := dirSize(d.path)
	if err != nil {
		glog.Error("rocksdb: DatabaseSizeOnDisk: ", err)
		return 0
	}
	return size
}

// GetTx returns transaction stored in db and height of the block containing it
func (d *RocksDB) GetTx(txid string) (*bchain.Tx, uint32, error) {
	key, err := d.chainParser.PackTxid(txid)
	if err != nil {
		return nil, 0, err
	}
	val, err := d.db.GetCF(d.ro, d.cfh[cfTransactions], key)
	if err != nil {
		return nil, 0, err
	}
	defer val.Free()
	data := val.Data()
	if len(data) > 4 {
		return d.chainParser.UnpackTx(data)
	}
	return nil, 0, nil
}

// PutTx stores transactions in db
func (d *RocksDB) PutTx(tx *bchain.Tx, height uint32, blockTime int64) error {
	key, err := d.chainParser.PackTxid(tx.Txid)
	if err != nil {
		return nil
	}
	buf, err := d.chainParser.PackTx(tx, height, blockTime)
	if err != nil {
		return err
	}
	err = d.db.PutCF(d.wo, d.cfh[cfTransactions], key, buf)
	if err == nil {
		d.is.AddDBColumnStats(cfTransactions, 1, int64(len(key)), int64(len(buf)))
	}
	return err
}

// DeleteTx removes transactions from db
func (d *RocksDB) DeleteTx(txid string) error {
	key, err := d.chainParser.PackTxid(txid)
	if err != nil {
		return nil
	}
	// use write batch so that this delete matches other deletes
	wb := gorocksdb.NewWriteBatch()
	defer wb.Destroy()
	d.internalDeleteTx(wb, key)
	return d.db.Write(d.wo, wb)
}

// internalDeleteTx checks if tx is cached and updates internal state accordingly
func (d *RocksDB) internalDeleteTx(wb *gorocksdb.WriteBatch, key []byte) {
	val, err := d.db.GetCF(d.ro, d.cfh[cfTransactions], key)
	// ignore error, it is only for statistics
	if err == nil {
		l := len(val.Data())
		if l > 0 {
			d.is.AddDBColumnStats(cfTransactions, -1, int64(-len(key)), int64(-l))
		}
		defer val.Free()
	}
	wb.DeleteCF(d.cfh[cfTransactions], key)
}

// internal state
const internalStateKey = "internalState"

// LoadInternalState loads from db internal state or initializes a new one if not yet stored
func (d *RocksDB) LoadInternalState(rpcCoin string) (*common.InternalState, error) {
	val, err := d.db.GetCF(d.ro, d.cfh[cfDefault], []byte(internalStateKey))
	if err != nil {
		return nil, err
	}
	defer val.Free()
	data := val.Data()
	var is *common.InternalState
	if len(data) == 0 {
		is = &common.InternalState{Coin: rpcCoin}
	} else {
		is, err = common.UnpackInternalState(data)
		if err != nil {
			return nil, err
		}
		// verify that the rpc coin matches DB coin
		// running it mismatched would corrupt the database
		if is.Coin == "" {
			is.Coin = rpcCoin
		} else if is.Coin != rpcCoin {
			return nil, errors.Errorf("Coins do not match. DB coin %v, RPC coin %v", is.Coin, rpcCoin)
		}
	}
	// make sure that column stats match the columns
	sc := is.DbColumns
	nc := make([]common.InternalStateColumn, len(cfNames))
	for i := 0; i < len(nc); i++ {
		nc[i].Name = cfNames[i]
		nc[i].Version = dbVersion
		for j := 0; j < len(sc); j++ {
			if sc[j].Name == nc[i].Name {
				// check the version of the column, if it does not match, the db is not compatible
				if sc[j].Version != dbVersion {
					return nil, errors.Errorf("DB version %v of column '%v' does not match the required version %v. DB is not compatible.", sc[j].Version, sc[j].Name, dbVersion)
				}
				nc[i].Rows = sc[j].Rows
				nc[i].KeyBytes = sc[j].KeyBytes
				nc[i].ValueBytes = sc[j].ValueBytes
				nc[i].Updated = sc[j].Updated
				break
			}
		}
	}
	is.DbColumns = nc
	return is, nil
}

func (d *RocksDB) SetInconsistentState(inconsistent bool) error {
	if d.is == nil {
		return errors.New("Internal state not created")
	}
	if inconsistent {
		d.is.DbState = common.DbStateInconsistent
	} else {
		d.is.DbState = common.DbStateOpen
	}
	return d.storeState(d.is)
}

// SetInternalState sets the InternalState to be used by db to collect internal state
func (d *RocksDB) SetInternalState(is *common.InternalState) {
	d.is = is
}

// StoreInternalState stores the internal state to db
func (d *RocksDB) StoreInternalState(is *common.InternalState) error {
	if d.metrics != nil {
		for c := 0; c < len(cfNames); c++ {
			rows, keyBytes, valueBytes := d.is.GetDBColumnStatValues(c)
			d.metrics.DbColumnRows.With(common.Labels{"column": cfNames[c]}).Set(float64(rows))
			d.metrics.DbColumnSize.With(common.Labels{"column": cfNames[c]}).Set(float64(keyBytes + valueBytes))
		}
	}
	return d.storeState(is)
}

func (d *RocksDB) storeState(is *common.InternalState) error {
	buf, err := is.Pack()
	if err != nil {
		return err
	}
	return d.db.PutCF(d.wo, d.cfh[cfDefault], []byte(internalStateKey), buf)
}

func (d *RocksDB) computeColumnSize(col int, stopCompute chan os.Signal) (int64, int64, int64, error) {
	var rows, keysSum, valuesSum int64
	var seekKey []byte
	for {
		var key []byte
		it := d.db.NewIteratorCF(d.ro, d.cfh[col])
		if rows == 0 {
			it.SeekToFirst()
		} else {
			glog.Info("db: Column ", cfNames[col], ": rows ", rows, ", key bytes ", keysSum, ", value bytes ", valuesSum, ", in progress...")
			it.Seek(seekKey)
			it.Next()
		}
		for count := 0; it.Valid() && count < refreshIterator; it.Next() {
			select {
			case <-stopCompute:
				return 0, 0, 0, errors.New("Interrupted")
			default:
			}
			key = it.Key().Data()
			count++
			rows++
			keysSum += int64(len(key))
			valuesSum += int64(len(it.Value().Data()))
		}
		seekKey = append([]byte{}, key...)
		valid := it.Valid()
		it.Close()
		if !valid {
			break
		}
	}
	return rows, keysSum, valuesSum, nil
}

// ComputeInternalStateColumnStats computes stats of all db columns and sets them to internal state
// can be very slow operation
func (d *RocksDB) ComputeInternalStateColumnStats(stopCompute chan os.Signal) error {
	start := time.Now()
	glog.Info("db: ComputeInternalStateColumnStats start")
	for c := 0; c < len(cfNames); c++ {
		rows, keysSum, valuesSum, err := d.computeColumnSize(c, stopCompute)
		if err != nil {
			return err
		}
		d.is.SetDBColumnStats(c, rows, keysSum, valuesSum)
		glog.Info("db: Column ", cfNames[c], ": rows ", rows, ", key bytes ", keysSum, ", value bytes ", valuesSum)
	}
	glog.Info("db: ComputeInternalStateColumnStats finished in ", time.Since(start))
	return nil
}

// Helpers

func packAddressKey(addrID []byte, height uint32) []byte {
	bheight := packUint(height)
	buf := make([]byte, 0, len(addrID)+len(bheight))
	buf = append(buf, addrID...)
	buf = append(buf, bheight...)
	return buf
}

func unpackAddressKey(key []byte) ([]byte, uint32, error) {
	i := len(key) - packedHeightBytes
	if i <= 0 {
		return nil, 0, errors.New("Invalid address key")
	}
	return key[:i], unpackUint(key[i : i+packedHeightBytes]), nil
}

func packUint(i uint32) []byte {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, i)
	return buf
}

func unpackUint(buf []byte) uint32 {
	return binary.BigEndian.Uint32(buf)
}

func packVarint32(i int32, buf []byte) int {
	return vlq.PutInt(buf, int64(i))
}

func packVarint(i int, buf []byte) int {
	return vlq.PutInt(buf, int64(i))
}

func packVaruint(i uint, buf []byte) int {
	return vlq.PutUint(buf, uint64(i))
}

func unpackVarint32(buf []byte) (int32, int) {
	i, ofs := vlq.Int(buf)
	return int32(i), ofs
}

func unpackVarint(buf []byte) (int, int) {
	i, ofs := vlq.Int(buf)
	return int(i), ofs
}

func unpackVaruint(buf []byte) (uint, int) {
	i, ofs := vlq.Uint(buf)
	return uint(i), ofs
}

const (
	// number of bits in a big.Word
	wordBits = 32 << (uint64(^big.Word(0)) >> 63)
	// number of bytes in a big.Word
	wordBytes = wordBits / 8
	// max packed bigint words
	maxPackedBigintWords = (256 - wordBytes) / wordBytes
	maxPackedBigintBytes = 249
)

// big int is packed in BigEndian order without memory allocation as 1 byte length followed by bytes of big int
// number of written bytes is returned
// limitation: bigints longer than 248 bytes are truncated to 248 bytes
// caution: buffer must be big enough to hold the packed big int, buffer 249 bytes big is always safe
func packBigint(bi *big.Int, buf []byte) int {
	w := bi.Bits()
	lw := len(w)
	// zero returns only one byte - zero length
	if lw == 0 {
		buf[0] = 0
		return 1
	}
	// pack the most significant word in a special way - skip leading zeros
	w0 := w[lw-1]
	fb := 8
	mask := big.Word(0xff) << (wordBits - 8)
	for w0&mask == 0 {
		fb--
		mask >>= 8
	}
	for i := fb; i > 0; i-- {
		buf[i] = byte(w0)
		w0 >>= 8
	}
	// if the big int is too big (> 2^1984), the number of bytes would not fit to 1 byte
	// in this case, truncate the number, it is not expected to work with this big numbers as amounts
	s := 0
	if lw > maxPackedBigintWords {
		s = lw - maxPackedBigintWords
	}
	// pack the rest of the words in reverse order
	for j := lw - 2; j >= s; j-- {
		d := w[j]
		for i := fb + wordBytes; i > fb; i-- {
			buf[i] = byte(d)
			d >>= 8
		}
		fb += wordBytes
	}
	buf[0] = byte(fb)
	return fb + 1
}

func unpackBigint(buf []byte) (big.Int, int) {
	var r big.Int
	l := int(buf[0]) + 1
	r.SetBytes(buf[1:l])
	return r, l
}
