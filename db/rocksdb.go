package db

import (
	"blockbook/bchain"
	"blockbook/common"
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
const refreshIterator = 5000000
const packedHeightBytes = 4

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
func NewRocksDB(path string, parser bchain.BlockChainParser, metrics *common.Metrics) (d *RocksDB, err error) {
	glog.Infof("rocksdb: open %s", path)
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
			if o.vout < 0 {
				vout = uint32(^o.vout)
				isOutput = false
			} else {
				vout = uint32(o.vout)
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
	btxID []byte
	vout  int32
}

func (d *RocksDB) packBlockAddress(addrID []byte, spentTxs map[string][]outpoint) []byte {
	vBuf := make([]byte, vlq.MaxLen32)
	vl := packVarint(int32(len(addrID)), vBuf)
	blockAddress := append([]byte(nil), vBuf[:vl]...)
	blockAddress = append(blockAddress, addrID...)
	if spentTxs == nil {
	} else {
		addrUnspentTxs := spentTxs[string(addrID)]
		vl = packVarint(int32(len(addrUnspentTxs)), vBuf)
		blockAddress = append(blockAddress, vBuf[:vl]...)
		buf := d.packOutpoints(addrUnspentTxs)
		blockAddress = append(blockAddress, buf...)
	}
	return blockAddress
}

func (d *RocksDB) writeAddressRecords(wb *gorocksdb.WriteBatch, block *bchain.Block, op int, addresses map[string][]outpoint, spentTxs map[string][]outpoint) error {
	keep := d.chainParser.KeepBlockAddresses()
	blockAddresses := make([]byte, 0)
	for addrID, outpoints := range addresses {
		baddrID := []byte(addrID)
		key := packAddressKey(baddrID, block.Height)
		switch op {
		case opInsert:
			val := d.packOutpoints(outpoints)
			wb.PutCF(d.cfh[cfAddresses], key, val)
			if keep > 0 {
				// collect all addresses be stored in blockaddresses
				// they are used in disconnect blocks
				blockAddress := d.packBlockAddress(baddrID, spentTxs)
				blockAddresses = append(blockAddresses, blockAddress...)
			}
		case opDelete:
			wb.DeleteCF(d.cfh[cfAddresses], key)
		}
	}
	if keep > 0 && op == opInsert {
		// write new block address and txs spent in this block
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
				btxID: btxid,
				vout:  vout,
			})
			if op == opDelete {
				// remove transactions from cache
				d.internalDeleteTx(wb, btxid)
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

func findAndRemoveUnspentAddr(unspentAddrs []byte, vout uint32) ([]byte, []byte) {
	// the addresses are packed as lenaddrID addrID vout, where lenaddrID and vout are varints
	for i := 0; i < len(unspentAddrs); {
		l, lv1 := unpackVarint(unspentAddrs[i:])
		// index of vout of address in unspentAddrs
		j := i + int(l) + lv1
		if j >= len(unspentAddrs) {
			glog.Error("rocksdb: Inconsistent data in unspentAddrs ", hex.EncodeToString(unspentAddrs), ", ", vout)
			return nil, unspentAddrs
		}
		n, lv2 := unpackVarint(unspentAddrs[j:])
		if uint32(n) == vout {
			addrID := append([]byte(nil), unspentAddrs[i+lv1:j]...)
			unspentAddrs = append(unspentAddrs[:i], unspentAddrs[j+lv2:]...)
			return addrID, unspentAddrs
		}
		i = j + lv2
	}
	return nil, unspentAddrs
}

func (d *RocksDB) writeAddressesUTXO(wb *gorocksdb.WriteBatch, block *bchain.Block, op int) error {
	if op == opDelete {
		// block does not contain mapping tx-> input address, which is necessary to recreate
		// unspentTxs; therefore it is not possible to DisconnectBlocks this way
		return errors.New("DisconnectBlock is not supported for UTXO chains")
	}
	addresses := make(map[string][]outpoint)
	unspentTxs := make(map[string][]byte)
	thisBlockTxs := make(map[string]struct{})
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
		stxID := string(btxID)
		unspentTxs[stxID] = txAddrs
		thisBlockTxs[stxID] = struct{}{}
	}
	// locate addresses spent by this tx and remove them from unspent addresses
	// keep them so that they be stored for DisconnectBlock functionality
	spentTxs := make(map[string][]outpoint)
	for txi, tx := range block.Txs {
		spendingTxid := btxIDs[txi]
		for i, input := range tx.Vin {
			btxID, err := d.chainParser.PackTxid(input.Txid)
			if err != nil {
				// do not process inputs without input txid
				if err == bchain.ErrTxidMissing {
					continue
				}
				return err
			}
			// find the tx in current block or already processed
			stxID := string(btxID)
			unspentAddrs, exists := unspentTxs[stxID]
			if !exists {
				// else find it in previous blocks
				unspentAddrs, err = d.getUnspentTx(btxID)
				if err != nil {
					return err
				}
				if unspentAddrs == nil {
					glog.Warningf("rocksdb: height %d, tx %v, input tx %v vin %v %v missing in unspentTxs", block.Height, tx.Txid, input.Txid, input.Vout, i)
					continue
				}
			}
			var addrID []byte
			addrID, unspentAddrs = findAndRemoveUnspentAddr(unspentAddrs, input.Vout)
			if addrID == nil {
				glog.Warningf("rocksdb: height %d, tx %v, input tx %v vin %v %v not found in unspentAddrs", block.Height, tx.Txid, input.Txid, input.Vout, i)
				continue
			}
			// record what was spent in this tx
			// skip transactions that were created in this block
			if _, exists := thisBlockTxs[stxID]; !exists {
				saddrID := string(addrID)
				rut := spentTxs[saddrID]
				rut = append(rut, outpoint{btxID, int32(input.Vout)})
				spentTxs[saddrID] = rut
			}
			err = d.addAddrIDToRecords(op, wb, addresses, addrID, spendingTxid, int32(^i), block.Height)
			if err != nil {
				return err
			}
			unspentTxs[stxID] = unspentAddrs
		}
	}
	if err := d.writeAddressRecords(wb, block, op, addresses, spentTxs); err != nil {
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
	return d.writeAddressRecords(wb, block, op, addresses, nil)
}

func (d *RocksDB) unpackBlockAddresses(buf []byte) ([][]byte, [][]outpoint, error) {
	addresses := make([][]byte, 0)
	outpointsArray := make([][]outpoint, 0)
	// the addresses are packed as lenaddrID addrID vout, where lenaddrID and vout are varints
	for i := 0; i < len(buf); {
		l, lv := unpackVarint(buf[i:])
		j := i + int(l) + lv
		if j > len(buf) {
			glog.Error("rocksdb: Inconsistent data in blockAddresses ", hex.EncodeToString(buf))
			return nil, nil, errors.New("Inconsistent data in blockAddresses")
		}
		addrID := append([]byte(nil), buf[i+lv:j]...)
		outpoints, ol, err := d.unpackNOutpoints(buf[j:])
		if err != nil {
			glog.Error("rocksdb: Inconsistent data in blockAddresses ", hex.EncodeToString(buf))
			return nil, nil, errors.New("Inconsistent data in blockAddresses")
		}
		addresses = append(addresses, addrID)
		outpointsArray = append(outpointsArray, outpoints)
		i = j + ol
	}
	return addresses, outpointsArray, nil
}

func (d *RocksDB) packOutpoints(outpoints []outpoint) []byte {
	buf := make([]byte, 0)
	bvout := make([]byte, vlq.MaxLen32)
	for _, o := range outpoints {
		l := packVarint(o.vout, bvout)
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
		vout, voutLen := unpackVarint(buf[i:])
		i += voutLen
		outpoints = append(outpoints, outpoint{
			btxID: btxID,
			vout:  vout,
		})
	}
	return outpoints, nil
}

func (d *RocksDB) unpackNOutpoints(buf []byte) ([]outpoint, int, error) {
	txidUnpackedLen := d.chainParser.PackedTxidLen()
	n, p := unpackVarint(buf)
	outpoints := make([]outpoint, n)
	for i := int32(0); i < n; i++ {
		if p+txidUnpackedLen >= len(buf) {
			return nil, 0, errors.New("Inconsistent data in unpackNOutpoints")
		}
		btxID := append([]byte(nil), buf[p:p+txidUnpackedLen]...)
		p += txidUnpackedLen
		vout, voutLen := unpackVarint(buf[p:])
		p += voutLen
		outpoints[i] = outpoint{
			btxID: btxID,
			vout:  vout,
		}
	}
	return outpoints, p, nil
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

func (d *RocksDB) getBlockAddresses(key []byte) ([][]byte, [][]outpoint, error) {
	b, err := d.db.GetCF(d.ro, d.cfh[cfBlockAddresses], key)
	if err != nil {
		return nil, nil, err
	}
	defer b.Free()
	// block is missing in DB
	if b.Data() == nil {
		return nil, nil, errors.New("Block addresses missing")
	}
	return d.unpackBlockAddresses(b.Data())
}

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

// DisconnectBlockRange removes all data belonging to blocks in range lower-higher
// it finds the data in blockaddresses column if available,
// otherwise by doing quite slow full scan of addresses column
func (d *RocksDB) DisconnectBlockRange(lower uint32, higher uint32) error {
	glog.Infof("db: disconnecting blocks %d-%d", lower, higher)
	addrKeys := [][]byte{}
	addrOutpoints := [][]byte{}
	addrUnspentOutpoints := [][]outpoint{}
	keep := d.chainParser.KeepBlockAddresses()
	var err error
	if keep > 0 {
		for height := lower; height <= higher; height++ {
			addresses, unspentOutpoints, err := d.getBlockAddresses(packUint(height))
			if err != nil {
				glog.Error(err)
				return err
			}
			for i, addrID := range addresses {
				addrKey := packAddressKey(addrID, height)
				val, err := d.db.GetCF(d.ro, d.cfh[cfAddresses], addrKey)
				if err != nil {
					glog.Error(err)
					return err
				}
				addrKeys = append(addrKeys, addrKey)
				av := append([]byte(nil), val.Data()...)
				val.Free()
				addrOutpoints = append(addrOutpoints, av)
				addrUnspentOutpoints = append(addrUnspentOutpoints, unspentOutpoints[i])
			}
		}
	} else {
		addrKeys, addrOutpoints, err = d.allAddressesScan(lower, higher)
		if err != nil {
			return err
		}
	}

	glog.Infof("rocksdb: about to disconnect %d addresses ", len(addrKeys))
	wb := gorocksdb.NewWriteBatch()
	defer wb.Destroy()
	unspentTxs := make(map[string][]byte)
	for addrIndex, addrKey := range addrKeys {
		if glog.V(2) {
			glog.Info("address ", hex.EncodeToString(addrKey))
		}
		// delete address:height from the index
		wb.DeleteCF(d.cfh[cfAddresses], addrKey)
		addrID, _, err := unpackAddressKey(addrKey)
		if err != nil {
			return err
		}
		// recreate unspentTxs, which were spent by this block (that is being disconnected)
		for _, o := range addrUnspentOutpoints[addrIndex] {
			stxID := string(o.btxID)
			txAddrs, exists := unspentTxs[stxID]
			if !exists {
				txAddrs, err = d.getUnspentTx(o.btxID)
				if err != nil {
					return err
				}
			}
			txAddrs = appendPackedAddrID(txAddrs, addrID, uint32(o.vout), 1)
			unspentTxs[stxID] = txAddrs
		}
		// delete unspentTxs from this block
		outpoints, err := d.unpackOutpoints(addrOutpoints[addrIndex])
		if err != nil {
			return err
		}
		for _, o := range outpoints {
			wb.DeleteCF(d.cfh[cfUnspentTxs], o.btxID)
			d.internalDeleteTx(wb, o.btxID)
		}
	}
	for key, val := range unspentTxs {
		wb.PutCF(d.cfh[cfUnspentTxs], []byte(key), val)
	}
	for height := lower; height <= higher; height++ {
		if glog.V(2) {
			glog.Info("height ", height)
		}
		key := packUint(height)
		if keep > 0 {
			wb.DeleteCF(d.cfh[cfBlockAddresses], key)
		}
		wb.DeleteCF(d.cfh[cfHeight], key)
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
		for j := 0; j < len(sc); j++ {
			if sc[j].Name == nc[i].Name {
				nc[i].Version = sc[j].Version
				nc[i].Rows = sc[j].Rows
				nc[i].KeyBytes = sc[j].KeyBytes
				nc[i].ValueBytes = sc[j].ValueBytes
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
	rows, keyBytes, valueBytes := d.is.GetDBColumnStats(cfTransactions)
	d.metrics.DbColumnRows.With(common.Labels{"column": cfNames[cfTransactions]}).Set(float64(rows))
	d.metrics.DbColumnSize.With(common.Labels{"column": cfNames[cfTransactions]}).Set(float64(keyBytes + valueBytes))
	buf, err := is.Pack()
	if err != nil {
		return err
	}
	return d.db.PutCF(d.wo, d.cfh[cfDefault], []byte(internalStateKey), buf)
}

func (d *RocksDB) computeColumnSize(col int) (int64, int64, int64, error) {
	var rows, keysSum, valuesSum int64
	var seekKey []byte
	for {
		var key []byte
		it := d.db.NewIteratorCF(d.ro, d.cfh[col])
		if rows == 0 {
			it.SeekToFirst()
		} else {
			glog.Info("Column ", cfNames[col], ": rows ", rows, ", key bytes ", keysSum, ", value bytes ", valuesSum, ", in progress...")
			it.Seek(seekKey)
			it.Next()
		}
		for count := 0; it.Valid() && count < refreshIterator; it.Next() {
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
func (d *RocksDB) ComputeInternalStateColumnStats() error {
	for c := 0; c < len(cfNames); c++ {
		rows, keysSum, valuesSum, err := d.computeColumnSize(c)
		if err != nil {
			return err
		}
		d.is.SetDBColumnStats(c, rows, keysSum, valuesSum)
		glog.Info("Column ", cfNames[c], ": rows ", rows, ", key bytes ", keysSum, ", value bytes ", valuesSum)
	}
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

func packVarint(i int32, buf []byte) int {
	return vlq.PutInt(buf, int64(i))
}

func unpackVarint(buf []byte) (int32, int) {
	i, ofs := vlq.Int(buf)
	return int32(i), ofs
}
