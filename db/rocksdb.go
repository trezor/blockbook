package db

import (
	"blockbook/bchain"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"

	"github.com/bsm/go-vlq"
	"github.com/golang/glog"

	"github.com/tecbot/gorocksdb"
)

func RepairRocksDB(name string) error {
	glog.Infof("rocksdb: repair")
	opts := gorocksdb.NewDefaultOptions()
	return gorocksdb.RepairDb(name, opts)
}

// RocksDB handle
type RocksDB struct {
	path string
	db   *gorocksdb.DB
	wo   *gorocksdb.WriteOptions
	ro   *gorocksdb.ReadOptions
	cfh  []*gorocksdb.ColumnFamilyHandle
}

const (
	cfDefault = iota
	cfHeight
	cfOutputs
	cfInputs
)

var cfNames = []string{"default", "height", "outputs", "inputs"}

func openDB(path string, bulk bool) (*gorocksdb.DB, []*gorocksdb.ColumnFamilyHandle, error) {
	fp := gorocksdb.NewBloomFilter(10)
	bbto := gorocksdb.NewDefaultBlockBasedTableOptions()
	bbto.SetBlockSize(16 << 10)                        // 16kb
	bbto.SetBlockCache(gorocksdb.NewLRUCache(8 << 30)) // 8 gb
	bbto.SetFilterPolicy(fp)

	opts := gorocksdb.NewDefaultOptions()
	opts.SetBlockBasedTableFactory(bbto)
	opts.SetCreateIfMissing(true)
	opts.SetCreateIfMissingColumnFamilies(true)
	opts.SetMaxBackgroundCompactions(4)
	opts.SetMaxBackgroundFlushes(2)
	opts.SetBytesPerSync(1 << 20)    // 1mb
	opts.SetWriteBufferSize(2 << 30) // 2 gb
	opts.SetMaxOpenFiles(25000)

	// opts for outputs are different:
	// no bloom filter - from documentation: If most of your queries are executed using iterators, you shouldn't set bloom filter
	optsOutputs := gorocksdb.NewDefaultOptions()
	optsOutputs.SetCreateIfMissing(true)
	optsOutputs.SetCreateIfMissingColumnFamilies(true)
	optsOutputs.SetMaxBackgroundCompactions(4)
	optsOutputs.SetMaxBackgroundFlushes(2)
	optsOutputs.SetBytesPerSync(1 << 20)    // 1mb
	optsOutputs.SetWriteBufferSize(2 << 30) // 2 gb
	optsOutputs.SetMaxOpenFiles(25000)

	if bulk {
		opts.PrepareForBulkLoad()
		optsOutputs.PrepareForBulkLoad()
	}

	fcOptions := []*gorocksdb.Options{opts, opts, optsOutputs, opts}

	db, cfh, err := gorocksdb.OpenDbColumnFamilies(opts, path, cfNames, fcOptions)
	if err != nil {
		return nil, nil, err
	}
	return db, cfh, nil
}

// NewRocksDB opens an internal handle to RocksDB environment.  Close
// needs to be called to release it.
func NewRocksDB(path string) (d *RocksDB, err error) {
	glog.Infof("rocksdb: open %s", path)
	db, cfh, err := openDB(path, false)
	wo := gorocksdb.NewDefaultWriteOptions()
	ro := gorocksdb.NewDefaultReadOptions()
	ro.SetFillCache(false)
	return &RocksDB{path, db, wo, ro, cfh}, nil
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

// GetTransactions finds all input/output transactions for address specified by outputScript.
// Transaction are passed to callback function.
func (d *RocksDB) GetTransactions(outputScript []byte, lower uint32, higher uint32, fn func(txid string, vout uint32, isOutput bool) error) (err error) {
	if glog.V(1) {
		glog.Infof("rocksdb: address get %s %d-%d ", unpackOutputScript(outputScript), lower, higher)
	}

	kstart, err := packOutputKey(outputScript, lower)
	if err != nil {
		return err
	}
	kstop, err := packOutputKey(outputScript, higher)
	if err != nil {
		return err
	}

	it := d.db.NewIteratorCF(d.ro, d.cfh[cfOutputs])
	defer it.Close()

	for it.Seek(kstart); it.Valid(); it.Next() {
		key := it.Key().Data()
		val := it.Value().Data()
		if bytes.Compare(key, kstop) > 0 {
			break
		}
		outpoints, err := unpackOutputValue(val)
		if err != nil {
			return err
		}
		if glog.V(2) {
			glog.Infof("rocksdb: output %s: %s", hex.EncodeToString(key), hex.EncodeToString(val))
		}
		for _, o := range outpoints {
			if err := fn(o.txid, o.vout, true); err != nil {
				return err
			}
			boutpoint, err := packOutpoint(o.txid, o.vout)
			if err != nil {
				return err
			}
			input, err := d.getInput(boutpoint)
			if err != nil {
				return err
			}
			if input != nil {
				if glog.V(2) {
					glog.Infof("rocksdb: input %s: %s", hex.EncodeToString(boutpoint), hex.EncodeToString(input))
				}
				inpoints, err := unpackOutputValue(input)
				if err != nil {
					return err
				}
				for _, i := range inpoints {
					if err := fn(i.txid, i.vout, false); err != nil {
						return err
					}
				}
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

	if err := d.writeHeight(wb, block, op); err != nil {
		return err
	}
	if err := d.writeOutputs(wb, block, op); err != nil {
		return err
	}
	if err := d.writeInputs(wb, block, op); err != nil {
		return err
	}

	return d.db.Write(d.wo, wb)
}

// Output Index

type outpoint struct {
	txid string
	vout uint32
}

func (d *RocksDB) writeOutputs(
	wb *gorocksdb.WriteBatch,
	block *bchain.Block,
	op int,
) error {
	records := make(map[string][]outpoint)

	for _, tx := range block.Txs {
		for _, output := range tx.Vout {
			outputScript := output.ScriptPubKey.Hex
			if outputScript != "" {
				if len(outputScript) > 1024 {
					glog.Infof("block %d, skipping outputScript of length %d", block.Height, len(outputScript)/2)
				} else {
					records[outputScript] = append(records[outputScript], outpoint{
						txid: tx.Txid,
						vout: output.N,
					})
				}
			}
		}
	}

	for outputScript, outpoints := range records {
		bOutputScript, err := packOutputScript(outputScript)
		if err != nil {
			glog.Warningf("rocksdb: packOutputScript: %v - %d %s", err, block.Height, outputScript)
			continue
		}
		key, err := packOutputKey(bOutputScript, block.Height)
		if err != nil {
			glog.Warningf("rocksdb: packOutputKey: %v - %d %s", err, block.Height, outputScript)
			continue
		}
		val, err := packOutputValue(outpoints)
		if err != nil {
			glog.Warningf("rocksdb: packOutputValue: %v", err)
			continue
		}

		switch op {
		case opInsert:
			wb.PutCF(d.cfh[cfOutputs], key, val)
		case opDelete:
			wb.DeleteCF(d.cfh[cfOutputs], key)
		}
	}

	return nil
}

func packOutputKey(outputScript []byte, height uint32) ([]byte, error) {
	bheight := packUint(height)
	buf := make([]byte, 0, len(outputScript)+len(bheight))
	buf = append(buf, outputScript...)
	buf = append(buf, bheight...)
	return buf, nil
}

func packOutputValue(outpoints []outpoint) ([]byte, error) {
	buf := make([]byte, 0)
	for _, o := range outpoints {
		btxid, err := packTxid(o.txid)
		if err != nil {
			return nil, err
		}
		bvout := packVaruint(o.vout)
		buf = append(buf, btxid...)
		buf = append(buf, bvout...)
	}
	return buf, nil
}

func unpackOutputValue(buf []byte) ([]outpoint, error) {
	outpoints := make([]outpoint, 0)
	for i := 0; i < len(buf); {
		txid, err := unpackTxid(buf[i : i+txIdUnpackedLen])
		if err != nil {
			return nil, err
		}
		i += txIdUnpackedLen
		vout, voutLen := unpackVaruint(buf[i:])
		i += voutLen
		outpoints = append(outpoints, outpoint{
			txid: txid,
			vout: vout,
		})
	}
	return outpoints, nil
}

// Input index

func (d *RocksDB) writeInputs(
	wb *gorocksdb.WriteBatch,
	block *bchain.Block,
	op int,
) error {
	for _, tx := range block.Txs {
		for i, input := range tx.Vin {
			if input.Coinbase != "" {
				continue
			}
			key, err := packOutpoint(input.Txid, input.Vout)
			if err != nil {
				return err
			}
			val, err := packOutpoint(tx.Txid, uint32(i))
			if err != nil {
				return err
			}
			switch op {
			case opInsert:
				wb.PutCF(d.cfh[cfInputs], key, val)
			case opDelete:
				wb.DeleteCF(d.cfh[cfInputs], key)
			}
		}
	}
	return nil
}

func packOutpoint(txid string, vout uint32) ([]byte, error) {
	btxid, err := packTxid(txid)
	if err != nil {
		return nil, err
	}
	bvout := packVaruint(vout)
	buf := make([]byte, 0, len(btxid)+len(bvout))
	buf = append(buf, btxid...)
	buf = append(buf, bvout...)
	return buf, nil
}

// Block index

// GetBestBlock returns the block hash of the block with highest height in the db
func (d *RocksDB) GetBestBlock() (uint32, string, error) {
	it := d.db.NewIteratorCF(d.ro, d.cfh[cfHeight])
	defer it.Close()
	if it.SeekToLast(); it.Valid() {
		bestHeight := unpackUint(it.Key().Data())
		val, err := unpackBlockValue(it.Value().Data())
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
	return unpackBlockValue(val.Data())
}

func (d *RocksDB) getInput(key []byte) ([]byte, error) {
	val, err := d.db.GetCF(d.ro, d.cfh[cfInputs], key)
	if err != nil {
		return nil, err
	}
	defer val.Free()
	return val.Data(), nil
}

func (d *RocksDB) writeHeight(
	wb *gorocksdb.WriteBatch,
	block *bchain.Block,
	op int,
) error {
	key := packUint(block.Height)

	switch op {
	case opInsert:
		val, err := packBlockValue(block.Hash)
		if err != nil {
			return err
		}
		wb.PutCF(d.cfh[cfHeight], key, val)
	case opDelete:
		wb.DeleteCF(d.cfh[cfHeight], key)
	}

	return nil
}

// DisconnectBlocks removes all data belonging to blocks in range lower-higher
func (d *RocksDB) DisconnectBlocks(
	lower uint32,
	higher uint32,
) error {
	glog.Infof("rocksdb: disconnecting blocks %d-%d", lower, higher)
	it := d.db.NewIteratorCF(d.ro, d.cfh[cfOutputs])
	defer it.Close()
	outputKeys := [][]byte{}
	outputValues := [][]byte{}
	var totalOutputs uint64
	for it.SeekToFirst(); it.Valid(); it.Next() {
		totalOutputs++
		key := it.Key().Data()
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
	glog.Infof("rocksdb: about to disconnect %d outputs from %d", len(outputKeys), totalOutputs)
	wb := gorocksdb.NewWriteBatch()
	defer wb.Destroy()
	for i := 0; i < len(outputKeys); i++ {
		if glog.V(2) {
			glog.Info("output ", hex.EncodeToString(outputKeys[i]))
		}
		wb.DeleteCF(d.cfh[cfOutputs], outputKeys[i])
		outpoints, err := unpackOutputValue(outputValues[i])
		if err != nil {
			return err
		}
		for _, o := range outpoints {
			boutpoint, err := packOutpoint(o.txid, o.vout)
			if err != nil {
				return err
			}
			if glog.V(2) {
				glog.Info("input ", hex.EncodeToString(boutpoint))
			}
			wb.DeleteCF(d.cfh[cfInputs], boutpoint)
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

// CompactDatabase compacts the database
// After unsuccessful experiment with CompactRange method (slow and actually fragmenting the db without compacting)
// the method now closes the db instance and opens it again.
// This means that during compact nobody can access the dababase!
func (d *RocksDB) CompactDatabase(bulk bool) error {
	size, _ := dirSize(d.path)
	glog.Info("Compacting database, db size ", size)
	if err := d.ReopenWithBulk(bulk); err != nil {
		return err
	}
	size, _ = dirSize(d.path)
	glog.Info("Compacting database finished, db size ", size)
	return nil
}

// ReopenWithBulk reopens the database with different settings:
// if bulk==true, reopens for bulk load
// if bulk==false, reopens for normal operation
// It closes and reopens db, nobody can access the database during the operation!
func (d *RocksDB) ReopenWithBulk(bulk bool) error {
	err := d.closeDB()
	if err != nil {
		return err
	}
	d.db = nil
	db, cfh, err := openDB(d.path, bulk)
	if err != nil {
		return err
	}
	d.db, d.cfh = db, cfh
	return nil
}

// Helpers

const txIdUnpackedLen = 32

var ErrInvalidAddress = errors.New("invalid address")

func packUint(i uint32) []byte {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, i)
	return buf
}

func unpackUint(buf []byte) uint32 {
	return binary.BigEndian.Uint32(buf)
}

func packVaruint(i uint32) []byte {
	buf := make([]byte, vlq.MaxLen32)
	ofs := vlq.PutUint(buf, uint64(i))
	return buf[:ofs]
}

func unpackVaruint(buf []byte) (uint32, int) {
	i, ofs := vlq.Uint(buf)
	return uint32(i), ofs
}

func packTxid(txid string) ([]byte, error) {
	return hex.DecodeString(txid)
}

func unpackTxid(buf []byte) (string, error) {
	return hex.EncodeToString(buf), nil
}

func packBlockValue(hash string) ([]byte, error) {
	return hex.DecodeString(hash)
}

func unpackBlockValue(buf []byte) (string, error) {
	return hex.EncodeToString(buf), nil
}

func packOutputScript(script string) ([]byte, error) {
	return hex.DecodeString(script)
}

func unpackOutputScript(buf []byte) string {
	return hex.EncodeToString(buf)
}
