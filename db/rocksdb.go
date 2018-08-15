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
	cfBlockTxids
	cfTransactions
)

var cfNames = []string{"default", "height", "addresses", "txAddresses", "addressBalance", "blockTxids", "transactions"}

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
		if err := d.writeAddressesUTXO(wb, block); err != nil {
			return err
		}
	} else {
		if err := d.writeAddressesNonUTXO(wb, block, op); err != nil {
			return err
		}
	}

	return d.db.Write(d.wo, wb)
}

// Addresses index

type outpoint struct {
	btxID []byte
	index int32
}

type txInput struct {
	addrID   []byte
	vout     uint32
	valueSat big.Int
}

type txOutput struct {
	addrID   []byte
	spent    bool
	valueSat big.Int
}

type txAddresses struct {
	inputs  []txInput
	outputs []txOutput
}

type addrBalance struct {
	txs        uint32
	sentSat    big.Int
	balanceSat big.Int
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

func (d *RocksDB) writeAddressesUTXO(wb *gorocksdb.WriteBatch, block *bchain.Block) error {
	addresses := make(map[string][]outpoint)
	blockTxids := make([][]byte, len(block.Txs))
	txAddressesMap := make(map[string]*txAddresses)
	balances := make(map[string]*addrBalance)
	// first process all outputs so that inputs can point to txs in this block
	for txi, tx := range block.Txs {
		btxID, err := d.chainParser.PackTxid(tx.Txid)
		if err != nil {
			return err
		}
		blockTxids[txi] = btxID
		ta := txAddresses{}
		ta.outputs = make([]txOutput, len(tx.Vout))
		txAddressesMap[string(btxID)] = &ta
		for i, output := range tx.Vout {
			tao := &ta.outputs[i]
			tao.valueSat = output.ValueSat
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
				ab, err = d.getAddressBalance(addrID)
				if err != nil {
					return err
				}
				if ab == nil {
					ab = &addrBalance{}
				}
				balances[strAddrID] = ab
			}
			// add number of trx in balance only once, address can be multiple times in tx
			if !processed {
				ab.txs++
			}
			ab.balanceSat.Add(&ab.balanceSat, &output.ValueSat)
		}
	}
	// process inputs
	for txi, tx := range block.Txs {
		spendingTxid := blockTxids[txi]
		ta := txAddressesMap[string(spendingTxid)]
		ta.inputs = make([]txInput, len(tx.Vin))
		for i, input := range tx.Vin {
			tai := &ta.inputs[i]
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
			if len(ita.outputs) <= int(input.Vout) {
				glog.Warningf("rocksdb: height %d, tx %v, input tx %v vout %v is out of bounds of stored tx", block.Height, tx.Txid, input.Txid, input.Vout)
				continue
			}
			ot := &ita.outputs[int(input.Vout)]
			if ot.spent {
				glog.Warningf("rocksdb: height %d, tx %v, input tx %v vout %v is double spend", block.Height, tx.Txid, input.Txid, input.Vout)
				continue
			}
			tai.addrID = ot.addrID
			tai.vout = input.Vout
			tai.valueSat = ot.valueSat
			// mark the output as spent in tx
			ot.spent = true
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
				ab, err = d.getAddressBalance(ot.addrID)
				if err != nil {
					return err
				}
				if ab == nil {
					ab = &addrBalance{}
				}
				balances[strAddrID] = ab
			}
			// add number of trx in balance only once, address can be multiple times in tx
			if !processed {
				ab.txs++
			}
			ab.balanceSat.Sub(&ab.balanceSat, &ot.valueSat)
			if ab.balanceSat.Sign() < 0 {
				d.resetValueSatToZero(&ab.balanceSat, ot.addrID, "balance")
			}
			ab.sentSat.Add(&ab.sentSat, &ot.valueSat)
		}
	}
	if err := d.storeAddresses(wb, block, addresses); err != nil {
		return err
	}
	if err := d.storeTxAddresses(wb, txAddressesMap); err != nil {
		return err
	}
	if err := d.storeBalances(wb, balances); err != nil {
		return err
	}
	return d.storeAndCleanupBlockTxids(wb, block, blockTxids)
}

func processedInTx(o []outpoint, btxID []byte) bool {
	for _, op := range o {
		if bytes.Equal(btxID, op.btxID) {
			return true
		}
	}
	return false
}

func (d *RocksDB) storeAddresses(wb *gorocksdb.WriteBatch, block *bchain.Block, addresses map[string][]outpoint) error {
	for addrID, outpoints := range addresses {
		ba := []byte(addrID)
		key := packAddressKey(ba, block.Height)
		val := d.packOutpoints(outpoints)
		wb.PutCF(d.cfh[cfAddresses], key, val)
	}
	return nil
}

func (d *RocksDB) storeTxAddresses(wb *gorocksdb.WriteBatch, am map[string]*txAddresses) error {
	varBuf := make([]byte, maxPackedBigintBytes)
	buf := make([]byte, 1024)
	for txID, ta := range am {
		buf = packTxAddresses(ta, buf, varBuf)
		wb.PutCF(d.cfh[cfTxAddresses], []byte(txID), buf)
	}
	return nil
}

func (d *RocksDB) storeBalances(wb *gorocksdb.WriteBatch, abm map[string]*addrBalance) error {
	// allocate buffer big enough for number of txs + 2 bigints
	buf := make([]byte, vlq.MaxLen32+2*maxPackedBigintBytes)
	for addrID, ab := range abm {
		// balance with 0 transactions is removed from db - happens in disconnect
		if ab == nil || ab.txs <= 0 {
			wb.DeleteCF(d.cfh[cfAddressBalance], []byte(addrID))
		} else {
			l := packVaruint(uint(ab.txs), buf)
			ll := packBigint(&ab.sentSat, buf[l:])
			l += ll
			ll = packBigint(&ab.balanceSat, buf[l:])
			l += ll
			wb.PutCF(d.cfh[cfAddressBalance], []byte(addrID), buf[:l])
		}
	}
	return nil
}

func (d *RocksDB) storeAndCleanupBlockTxids(wb *gorocksdb.WriteBatch, block *bchain.Block, txids [][]byte) error {
	pl := d.chainParser.PackedTxidLen()
	buf := make([]byte, pl*len(txids))
	i := 0
	for _, txid := range txids {
		copy(buf[i:], txid)
		i += pl
	}
	key := packUint(block.Height)
	wb.PutCF(d.cfh[cfBlockTxids], key, buf)
	keep := d.chainParser.KeepBlockAddresses()
	// cleanup old block address
	if block.Height > uint32(keep) {
		for rh := block.Height - uint32(keep); rh < block.Height; rh-- {
			key = packUint(rh)
			val, err := d.db.GetCF(d.ro, d.cfh[cfBlockTxids], key)
			if err != nil {
				return err
			}
			if val.Size() == 0 {
				break
			}
			val.Free()
			d.db.DeleteCF(d.wo, d.cfh[cfBlockTxids], key)
		}
	}
	return nil
}

func (d *RocksDB) getBlockTxids(height uint32) ([][]byte, error) {
	pl := d.chainParser.PackedTxidLen()
	val, err := d.db.GetCF(d.ro, d.cfh[cfBlockTxids], packUint(height))
	if err != nil {
		return nil, err
	}
	defer val.Free()
	buf := val.Data()
	txids := make([][]byte, len(buf)/pl)
	for i := 0; i < len(txids); i++ {
		txid := make([]byte, pl)
		copy(txid, buf[pl*i:])
	}
	return txids, nil
}

func (d *RocksDB) getAddressBalance(addrID []byte) (*addrBalance, error) {
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
	return &addrBalance{
		txs:        uint32(txs),
		sentSat:    sentSat,
		balanceSat: balanceSat,
	}, nil
}

func (d *RocksDB) getTxAddresses(btxID []byte) (*txAddresses, error) {
	val, err := d.db.GetCF(d.ro, d.cfh[cfTxAddresses], btxID)
	if err != nil {
		return nil, err
	}
	defer val.Free()
	buf := val.Data()
	// 2 is minimum length of addrBalance - 1 byte inputs len, 1 byte outputs len
	if len(buf) < 2 {
		return nil, nil
	}
	return unpackTxAddresses(buf)
}

func packTxAddresses(ta *txAddresses, buf []byte, varBuf []byte) []byte {
	buf = buf[:0]
	l := packVaruint(uint(len(ta.inputs)), varBuf)
	buf = append(buf, varBuf[:l]...)
	for i := range ta.inputs {
		buf = appendTxInput(&ta.inputs[i], buf, varBuf)
	}
	l = packVaruint(uint(len(ta.outputs)), varBuf)
	buf = append(buf, varBuf[:l]...)
	for i := range ta.outputs {
		buf = appendTxOutput(&ta.outputs[i], buf, varBuf)
	}
	return buf
}

func appendTxInput(txi *txInput, buf []byte, varBuf []byte) []byte {
	la := len(txi.addrID)
	l := packVarint(la, varBuf)
	buf = append(buf, varBuf[:l]...)
	buf = append(buf, txi.addrID...)
	l = packBigint(&txi.valueSat, varBuf)
	buf = append(buf, varBuf[:l]...)
	l = packVaruint(uint(txi.vout), varBuf)
	buf = append(buf, varBuf[:l]...)
	return buf
}

func appendTxOutput(txo *txOutput, buf []byte, varBuf []byte) []byte {
	la := len(txo.addrID)
	if txo.spent {
		la = ^la
	}
	l := packVarint(la, varBuf)
	buf = append(buf, varBuf[:l]...)
	buf = append(buf, txo.addrID...)
	l = packBigint(&txo.valueSat, varBuf)
	buf = append(buf, varBuf[:l]...)
	return buf
}

func unpackTxAddresses(buf []byte) (*txAddresses, error) {
	ta := txAddresses{}
	inputs, l := unpackVaruint(buf)
	ta.inputs = make([]txInput, inputs)
	for i := uint(0); i < inputs; i++ {
		l += unpackTxInput(&ta.inputs[i], buf[l:])
	}
	outputs, ll := unpackVaruint(buf[l:])
	l += ll
	ta.outputs = make([]txOutput, outputs)
	for i := uint(0); i < outputs; i++ {
		l += unpackTxOutput(&ta.outputs[i], buf[l:])
	}
	return &ta, nil
}

func unpackTxInput(ti *txInput, buf []byte) int {
	al, l := unpackVarint(buf)
	ti.addrID = make([]byte, al)
	copy(ti.addrID, buf[l:l+al])
	al += l
	ti.valueSat, l = unpackBigint(buf[al:])
	al += l
	v, l := unpackVaruint(buf[al:])
	ti.vout = uint32(v)
	return l + al
}

func unpackTxOutput(to *txOutput, buf []byte) int {
	al, l := unpackVarint(buf)
	if al < 0 {
		to.spent = true
		al = ^al
	}
	to.addrID = make([]byte, al)
	copy(to.addrID, buf[l:l+al])
	al += l
	to.valueSat, l = unpackBigint(buf[al:])
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

// get all transactions of the given address and match it to input to find spent output
func (d *RocksDB) findSpentTx(ti *txInput) ([]byte, *txAddresses, error) {
	start := packAddressKey(ti.addrID, 0)
	stop := packAddressKey(ti.addrID, ^uint32(0))
	it := d.db.NewIteratorCF(d.ro, d.cfh[cfAddresses])
	defer it.Close()
	for it.Seek(start); it.Valid(); it.Next() {
		key := it.Key().Data()
		val := it.Value().Data()
		if bytes.Compare(key, stop) > 0 {
			break
		}
		outpoints, err := d.unpackOutpoints(val)
		if err != nil {
			return nil, nil, err
		}
		for _, o := range outpoints {
			// process only outputs that match
			if o.index >= 0 && uint32(o.index) == ti.vout {
				a, err := d.getTxAddresses(o.btxID)
				if err != nil {
					return nil, nil, err
				}
				if bytes.Equal(a.outputs[o.index].addrID, ti.addrID) {
					return o.btxID, a, nil
				}
			}
		}
	}
	return nil, nil, nil
}

func (d *RocksDB) disconnectTxAddresses(wb *gorocksdb.WriteBatch, height uint32, txid string, txa *txAddresses,
	txAddressesToUpdate map[string]*txAddresses, balances map[string]*addrBalance) error {
	addresses := make(map[string]struct{})
	getAddressBalance := func(addrID []byte) (*addrBalance, error) {
		var err error
		s := string(addrID)
		b, fb := balances[s]
		if !fb {
			b, err = d.getAddressBalance(addrID)
			if err != nil {
				return nil, err
			}
			balances[s] = b
		}
		return b, nil
	}
	for _, t := range txa.inputs {
		s := string(t.addrID)
		_, fa := addresses[s]
		if !fa {
			addresses[s] = struct{}{}
		}
		b, err := getAddressBalance(t.addrID)
		if err != nil {
			return err
		}
		if b != nil {
			// subtract number of txs only once
			if !fa {
				b.txs--
			}
			b.sentSat.Sub(&b.sentSat, &t.valueSat)
			if b.sentSat.Sign() < 0 {
				d.resetValueSatToZero(&b.sentSat, t.addrID, "sent amount")
			}
			b.balanceSat.Add(&b.balanceSat, &t.valueSat)
		}
	}
	for _, t := range txa.outputs {
		s := string(t.addrID)
		_, fa := addresses[s]
		if !fa {
			addresses[s] = struct{}{}
		}
		b, err := getAddressBalance(t.addrID)
		if err != nil {
			return err
		}
		if b != nil {
			// subtract number of txs only once
			if !fa {
				b.txs--
			}
			b.balanceSat.Sub(&b.balanceSat, &t.valueSat)
			if b.balanceSat.Sign() < 0 {
				d.resetValueSatToZero(&b.balanceSat, t.addrID, "balance")
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
// if they are in the range kept in the cfBlockTxs column
func (d *RocksDB) DisconnectBlockRangeUTXO(lower uint32, higher uint32) error {
	glog.Infof("db: disconnecting blocks %d-%d", lower, higher)
	blocksTxids := make([][][]byte, higher-lower+1)
	for height := lower; height <= higher; height++ {
		blockTxids, err := d.getBlockTxids(height)
		if err != nil {
			return err
		}
		if len(blockTxids) == 0 {
			return errors.Errorf("Cannot disconnect blocks with height %v and lower. It is necessary to rebuild index.", height)
		}
		blocksTxids[height-lower] = blockTxids
	}
	wb := gorocksdb.NewWriteBatch()
	defer wb.Destroy()
	txAddressesToUpdate := make(map[string]*txAddresses)
	txsToDelete := make(map[string]struct{})
	balances := make(map[string]*addrBalance)
	for height := higher; height >= lower; height-- {
		blockTxids := blocksTxids[height-lower]
		for _, txid := range blockTxids {
			txa, err := d.getTxAddresses(txid)
			if err != nil {
				return err
			}
			s := string(txid)
			txsToDelete[s] = struct{}{}
			if err := d.disconnectTxAddresses(wb, height, s, txa, txAddressesToUpdate, balances); err != nil {
				return err
			}
		}
		key := packUint(height)
		wb.DeleteCF(d.cfh[cfBlockTxids], key)
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

// SetInternalState sets the InternalState to be used by db to collect internal state
func (d *RocksDB) SetInternalState(is *common.InternalState) {
	d.is = is
}

// StoreInternalState stores the internal state to db
func (d *RocksDB) StoreInternalState(is *common.InternalState) error {
	for c := 0; c < len(cfNames); c++ {
		rows, keyBytes, valueBytes := d.is.GetDBColumnStatValues(c)
		d.metrics.DbColumnRows.With(common.Labels{"column": cfNames[c]}).Set(float64(rows))
		d.metrics.DbColumnSize.With(common.Labels{"column": cfNames[c]}).Set(float64(keyBytes + valueBytes))
	}
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
