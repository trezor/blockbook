package db

import (
	"blockbook/bchain"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"os"
	"path/filepath"

	"github.com/bsm/go-vlq"
	"github.com/golang/glog"
	"github.com/juju/errors"

	"github.com/tecbot/gorocksdb"
)

// iterator creates snapshot, which takes lots of resources
// when doing huge scan, it is better to close it and reopen from time to time to free the resources
const disconnectBlocksRefreshIterator = uint64(1000000)

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
}

const (
	cfDefault = iota
	cfHeight
	cfAddresses
	cfUnspentTxs
	cfTransactions
	cfBlockAddresses
)

var cfNames = []string{"default", "height", "addresses", "unspenttxs", "transactions", "blockaddresses"}

func openDB(path string) (*gorocksdb.DB, []*gorocksdb.ColumnFamilyHandle, error) {
	c := gorocksdb.NewLRUCache(8 << 30) // 8GB
	fp := gorocksdb.NewBloomFilter(10)
	bbto := gorocksdb.NewDefaultBlockBasedTableOptions()
	bbto.SetBlockSize(16 << 10) // 16kB
	bbto.SetBlockCache(c)
	bbto.SetFilterPolicy(fp)

	opts := gorocksdb.NewDefaultOptions()
	opts.SetBlockBasedTableFactory(bbto)
	opts.SetCreateIfMissing(true)
	opts.SetCreateIfMissingColumnFamilies(true)
	opts.SetMaxBackgroundCompactions(4)
	opts.SetMaxBackgroundFlushes(2)
	opts.SetBytesPerSync(1 << 20)    // 1MB
	opts.SetWriteBufferSize(1 << 27) // 128MB
	opts.SetMaxOpenFiles(25000)
	opts.SetCompression(gorocksdb.NoCompression)

	// opts for outputs are different:
	// no bloom filter - from documentation: If most of your queries are executed using iterators, you shouldn't set bloom filter
	bbtoOutputs := gorocksdb.NewDefaultBlockBasedTableOptions()
	bbtoOutputs.SetBlockSize(16 << 10) // 16kB
	bbtoOutputs.SetBlockCache(c)       // 8GB

	optsOutputs := gorocksdb.NewDefaultOptions()
	optsOutputs.SetBlockBasedTableFactory(bbtoOutputs)
	optsOutputs.SetCreateIfMissing(true)
	optsOutputs.SetCreateIfMissingColumnFamilies(true)
	optsOutputs.SetMaxBackgroundCompactions(4)
	optsOutputs.SetMaxBackgroundFlushes(2)
	optsOutputs.SetBytesPerSync(1 << 20)    // 1MB
	optsOutputs.SetWriteBufferSize(1 << 27) // 128MB
	optsOutputs.SetMaxOpenFiles(25000)
	optsOutputs.SetCompression(gorocksdb.NoCompression)

	fcOptions := []*gorocksdb.Options{opts, opts, optsOutputs, opts, opts, opts}

	db, cfh, err := gorocksdb.OpenDbColumnFamilies(opts, path, cfNames, fcOptions)
	if err != nil {
		return nil, nil, err
	}
	return db, cfh, nil
}

// NewRocksDB opens an internal handle to RocksDB environment.  Close
// needs to be called to release it.
func NewRocksDB(path string, parser bchain.BlockChainParser) (d *RocksDB, err error) {
	glog.Infof("rocksdb: open %s", path)
	db, cfh, err := openDB(path)
	wo := gorocksdb.NewDefaultWriteOptions()
	ro := gorocksdb.NewDefaultReadOptions()
	ro.SetFillCache(false)
	return &RocksDB{path, db, wo, ro, cfh, parser}, nil
}

func (d *RocksDB) closeDB() error {
	for _, h := range d.cfh {
		h.Destroy()
	}
	d.db.Close()
	return nil
}

// Close releases the RocksDB environment opened in NewRocksDB.
func (d *RocksDB) Close() error {
	glog.Infof("rocksdb: close")
	d.closeDB()
	d.wo.Destroy()
	d.ro.Destroy()
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

	kstart, err := packOutputKey(addrID, lower)
	if err != nil {
		return err
	}
	kstop, err := packOutputKey(addrID, higher)
	if err != nil {
		return err
	}

	it := d.db.NewIteratorCF(d.ro, d.cfh[cfAddresses])
	defer it.Close()

	for it.Seek(kstart); it.Valid(); it.Next() {
		key := it.Key().Data()
		val := it.Value().Data()
		if bytes.Compare(key, kstop) > 0 {
			break
		}
		outpoints, err := d.unpackOutputValue(val)
		if err != nil {
			return err
		}
		if glog.V(2) {
			glog.Infof("rocksdb: output %s: %s", hex.EncodeToString(key), hex.EncodeToString(val))
		}
		for _, o := range outpoints {
			var vout uint32
			var isOutput bool
			if o.vout < 0 {
				vout = uint32(^o.vout)
				isOutput = false
			} else {
				vout = uint32(o.vout)
				isOutput = true
			}
			if err := fn(o.txid, vout, isOutput); err != nil {
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

func (d *RocksDB) ConnectBlock(block *bchain.Block) error {
	return d.writeBlock(block, opInsert)
}

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
		if err := d.writeAddressesUTXO(wb, block, op); err != nil {
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
	txid string
	vout int32
}

func (d *RocksDB) writeAddressRecords(wb *gorocksdb.WriteBatch, block *bchain.Block, op int, records map[string][]outpoint) error {
	keep := d.chainParser.KeepBlockAddresses()
	blockAddresses := make([]byte, 0)
	vBuf := make([]byte, vlq.MaxLen32)
	for addrID, outpoints := range records {
		key, err := packOutputKey([]byte(addrID), block.Height)
		if err != nil {
			glog.Warningf("rocksdb: packOutputKey: %v - %d %s", err, block.Height, addrID)
			continue
		}
		switch op {
		case opInsert:
			val, err := d.packOutputValue(outpoints)
			if err != nil {
				glog.Warningf("rocksdb: packOutputValue: %v", err)
				continue
			}
			wb.PutCF(d.cfh[cfAddresses], key, val)
			if keep > 0 {
				// collect all addresses to be stored in blockaddresses
				vl := packVarint(int32(len([]byte(addrID))), vBuf)
				blockAddresses = append(blockAddresses, vBuf[0:vl]...)
				blockAddresses = append(blockAddresses, []byte(addrID)...)
			}
		case opDelete:
			wb.DeleteCF(d.cfh[cfAddresses], key)
		}
	}
	if keep > 0 && op == opInsert {
		// write new block address
		key := packUint(block.Height)
		wb.PutCF(d.cfh[cfBlockAddresses], key, blockAddresses)
		// cleanup old block address
		if block.Height > uint32(keep) {
			for rh := block.Height - uint32(keep); rh < block.Height; rh-- {
				key = packUint(rh)
				val, err := d.db.GetCF(d.ro, d.cfh[cfBlockAddresses], key)
				if err != nil {
					return err
				}
				if val.Size() == 0 {
					break
				}
				val.Free()
				d.db.DeleteCF(d.wo, d.cfh[cfBlockAddresses], key)
			}
		}
	}
	return nil
}

func (d *RocksDB) addAddrIDToRecords(op int, wb *gorocksdb.WriteBatch, records map[string][]outpoint, addrID []byte, btxid []byte, vout int32, bh uint32) error {
	if len(addrID) > 0 {
		if len(addrID) > 1024 {
			glog.Infof("rocksdb: block %d, skipping addrID of length %d", bh, len(addrID))
		} else {
			strAddrID := string(addrID)
			records[strAddrID] = append(records[strAddrID], outpoint{
				txid: string(btxid),
				vout: vout,
			})
			if op == opDelete {
				// remove transactions from cache
				wb.DeleteCF(d.cfh[cfTransactions], btxid)
			}
		}
	}
	return nil
}

func (d *RocksDB) getUnspentTx(btxID []byte) ([]byte, error) {
	// find it in db, in the column cfUnspentTxs
	val, err := d.db.GetCF(d.ro, d.cfh[cfUnspentTxs], btxID)
	if err != nil {
		return nil, err
	}
	defer val.Free()
	data := append([]byte(nil), val.Data()...)
	return data, nil
}

func appendPackedAddrID(txAddrs []byte, addrID []byte, n uint32, remaining int) []byte {
	// resize the addr buffer if necessary by a new estimate
	if cap(txAddrs)-len(txAddrs) < 2*vlq.MaxLen32+len(addrID) {
		txAddrs = append(txAddrs, make([]byte, vlq.MaxLen32+len(addrID)+remaining*32)...)[:len(txAddrs)]
	}
	// addrID is packed as number of bytes of the addrID + bytes of addrID + vout
	lv := packVarint(int32(len(addrID)), txAddrs[len(txAddrs):len(txAddrs)+vlq.MaxLen32])
	txAddrs = txAddrs[:len(txAddrs)+lv]
	txAddrs = append(txAddrs, addrID...)
	lv = packVarint(int32(n), txAddrs[len(txAddrs):len(txAddrs)+vlq.MaxLen32])
	txAddrs = txAddrs[:len(txAddrs)+lv]
	return txAddrs
}

func findAndRemoveUnspentAddr(unspentAddrs []byte, vout uint32) ([]byte, uint32, []byte) {
	// the addresses are packed as lenaddrID addrID vout, where lenaddrID and vout are varints
	for i := 0; i < len(unspentAddrs); {
		l, lv1 := unpackVarint(unspentAddrs[i:])
		// index of vout of address in unspentAddrs
		j := i + int(l) + lv1
		if j >= len(unspentAddrs) {
			glog.Error("rocksdb: Inconsistent data in unspentAddrs")
			return nil, 0, unspentAddrs
		}
		n, lv2 := unpackVarint(unspentAddrs[j:])
		if uint32(n) == vout {
			addrID := append([]byte(nil), unspentAddrs[i+lv1:j]...)
			unspentAddrs = append(unspentAddrs[:i], unspentAddrs[j+lv2:]...)
			return addrID, uint32(n), unspentAddrs
		}
		i += j + lv2
	}
	return nil, 0, unspentAddrs
}

func (d *RocksDB) writeAddressesUTXO(wb *gorocksdb.WriteBatch, block *bchain.Block, op int) error {
	if op == opDelete {
		// block does not contain mapping tx-> input address, which is necessary to recreate
		// unspentTxs; therefore it is not possible to DisconnectBlocks this way
		return errors.New("DisconnectBlock is not supported for UTXO chains")
	}
	addresses := make(map[string][]outpoint)
	unspentTxs := make(map[string][]byte)
	btxIDs := make([][]byte, len(block.Txs))
	// first process all outputs, build mapping of addresses to outpoints and mappings of unspent txs to addresses
	for txi, tx := range block.Txs {
		btxID, err := d.chainParser.PackTxid(tx.Txid)
		if err != nil {
			return err
		}
		btxIDs[txi] = btxID
		// preallocate estimated size of addresses (32 bytes is 1 byte length of addrID, 25 bytes addrID, 1-2 bytes vout and reserve)
		txAddrs := make([]byte, 0, len(tx.Vout)*32)
		for i, output := range tx.Vout {
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
			txAddrs = appendPackedAddrID(txAddrs, addrID, output.N, len(tx.Vout)-i)
		}
		unspentTxs[string(btxID)] = txAddrs
	}
	// locate addresses spent by this tx and add them to addresses map them in format txid ^index
	for txi, tx := range block.Txs {
		spendingTxid := btxIDs[txi]
		for i, input := range tx.Vin {
			btxID, err := d.chainParser.PackTxid(input.Txid)
			if err != nil {
				return err
			}
			// try to find the tx in current block
			unspentAddrs, inThisBlock := unspentTxs[string(btxID)]
			if !inThisBlock {
				unspentAddrs, err = d.getUnspentTx(btxID)
				if err != nil {
					return err
				}
				if unspentAddrs == nil {
					glog.Warningf("rocksdb: height %d, tx %v in inputs but missing in unspentTxs", block.Height, tx.Txid)
					continue
				}
			}
			var addrID []byte
			addrID, _, unspentAddrs = findAndRemoveUnspentAddr(unspentAddrs, input.Vout)
			if addrID == nil {
				glog.Warningf("rocksdb: height %d, tx %v vout %v in inputs but missing in unspentTxs", block.Height, tx.Txid, input.Vout)
				continue
			}
			err = d.addAddrIDToRecords(op, wb, addresses, addrID, spendingTxid, int32(^i), block.Height)
			if err != nil {
				return err
			}
			unspentTxs[string(btxID)] = unspentAddrs
		}
	}
	if err := d.writeAddressRecords(wb, block, op, addresses); err != nil {
		return err
	}
	// save unspent txs from current block
	for tx, val := range unspentTxs {
		if len(val) == 0 {
			wb.DeleteCF(d.cfh[cfUnspentTxs], []byte(tx))
		} else {
			wb.PutCF(d.cfh[cfUnspentTxs], []byte(tx), val)
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
	return d.writeAddressRecords(wb, block, op, addresses)
}

func packOutputKey(outputScript []byte, height uint32) ([]byte, error) {
	bheight := packUint(height)
	buf := make([]byte, 0, len(outputScript)+len(bheight))
	buf = append(buf, outputScript...)
	buf = append(buf, bheight...)
	return buf, nil
}

func (d *RocksDB) packOutputValue(outpoints []outpoint) ([]byte, error) {
	buf := make([]byte, 0)
	for _, o := range outpoints {
		bvout := make([]byte, vlq.MaxLen32)
		l := packVarint(o.vout, bvout)
		buf = append(buf, []byte(o.txid)...)
		buf = append(buf, bvout[:l]...)
	}
	return buf, nil
}

func (d *RocksDB) unpackOutputValue(buf []byte) ([]outpoint, error) {
	txidUnpackedLen := d.chainParser.PackedTxidLen()
	outpoints := make([]outpoint, 0)
	for i := 0; i < len(buf); {
		txid, err := d.chainParser.UnpackTxid(buf[i : i+txidUnpackedLen])
		if err != nil {
			return nil, err
		}
		i += txidUnpackedLen
		vout, voutLen := unpackVarint(buf[i:])
		i += voutLen
		outpoints = append(outpoints, outpoint{
			txid: txid,
			vout: vout,
		})
	}
	return outpoints, nil
}

func (d *RocksDB) packOutpoint(txid string, vout int32) ([]byte, error) {
	btxid, err := d.chainParser.PackTxid(txid)
	if err != nil {
		return nil, err
	}
	bv := make([]byte, vlq.MaxLen32)
	l := packVarint(vout, bv)
	buf := make([]byte, 0, l+len(btxid))
	buf = append(buf, btxid...)
	buf = append(buf, bv[:l]...)
	return buf, nil
}

func (d *RocksDB) unpackOutpoint(buf []byte) (string, int32, int) {
	txidUnpackedLen := d.chainParser.PackedTxidLen()
	txid, _ := d.chainParser.UnpackTxid(buf[:txidUnpackedLen])
	vout, o := unpackVarint(buf[txidUnpackedLen:])
	return txid, vout, txidUnpackedLen + o
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

func (d *RocksDB) writeHeight(
	wb *gorocksdb.WriteBatch,
	block *bchain.Block,
	op int,
) error {
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

// DisconnectBlocksFullScan removes all data belonging to blocks in range lower-higher
// it finds the data by doing full scan of outputs column, therefore it is quite slow
func (d *RocksDB) DisconnectBlocksFullScan(lower uint32, higher uint32) error {
	glog.Infof("db: disconnecting blocks %d-%d using full scan", lower, higher)
	outputKeys := [][]byte{}
	outputValues := [][]byte{}
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
		for count = 0; it.Valid() && count < disconnectBlocksRefreshIterator; it.Next() {
			totalOutputs++
			count++
			key = it.Key().Data()
			l := len(key)
			if l > 4 {
				height := unpackUint(key[l-4 : l])
				if height >= lower && height <= higher {
					outputKey := make([]byte, len(key))
					copy(outputKey, key)
					outputKeys = append(outputKeys, outputKey)
					value := it.Value().Data()
					outputValue := make([]byte, len(value))
					copy(outputValue, value)
					outputValues = append(outputValues, outputValue)
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
	glog.Infof("rocksdb: about to disconnect %d outputs from %d", len(outputKeys), totalOutputs)
	wb := gorocksdb.NewWriteBatch()
	defer wb.Destroy()
	for i := 0; i < len(outputKeys); i++ {
		if glog.V(2) {
			glog.Info("output ", hex.EncodeToString(outputKeys[i]))
		}
		wb.DeleteCF(d.cfh[cfAddresses], outputKeys[i])
		outpoints, err := d.unpackOutputValue(outputValues[i])
		if err != nil {
			return err
		}
		for _, o := range outpoints {
			// delete from inputs
			boutpoint, err := d.packOutpoint(o.txid, o.vout)
			if err != nil {
				return err
			}
			if glog.V(2) {
				glog.Info("input ", hex.EncodeToString(boutpoint))
			}
			wb.DeleteCF(d.cfh[cfUnspentTxs], boutpoint)
			// delete from txCache
			b, err := d.chainParser.PackTxid(o.txid)
			if err != nil {
				return err
			}
			wb.DeleteCF(d.cfh[cfTransactions], b)
		}
	}
	for height := lower; height <= higher; height++ {
		if glog.V(2) {
			glog.Info("height ", height)
		}
		wb.DeleteCF(d.cfh[cfHeight], packUint(height))
	}
	err := d.db.Write(d.wo, wb)
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
	return d.db.PutCF(d.wo, d.cfh[cfTransactions], key, buf)
}

// DeleteTx removes transactions from db
func (d *RocksDB) DeleteTx(txid string) error {
	key, err := d.chainParser.PackTxid(txid)
	if err != nil {
		return nil
	}
	return d.db.DeleteCF(d.wo, d.cfh[cfTransactions], key)
}

// Helpers

func packUint(i uint32) []byte {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, i)
	return buf
}

func unpackUint(buf []byte) uint32 {
	return binary.BigEndian.Uint32(buf)
}

func packVarint(i int32, buf []byte) int {
	return vlq.PutInt(buf, int64(i))
}

func unpackVarint(buf []byte) (int32, int) {
	i, ofs := vlq.Int(buf)
	return int32(i), ofs
}
