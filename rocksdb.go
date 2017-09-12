package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"log"

	"github.com/bsm/go-vlq"
	"github.com/btcsuite/btcutil/base58"

	"github.com/tecbot/gorocksdb"
)

func RepairRocksDB(name string) error {
	log.Printf("rocksdb: repair")
	opts := gorocksdb.NewDefaultOptions()
	return gorocksdb.RepairDb(name, opts)
}

type RocksDB struct {
	db *gorocksdb.DB
	wo *gorocksdb.WriteOptions
	ro *gorocksdb.ReadOptions
}

// NewRocksDB opens an internal handle to RocksDB environment.  Close
// needs to be called to release it.
func NewRocksDB(path string) (d *RocksDB, err error) {
	log.Printf("rocksdb: open %s", path)

	fp := gorocksdb.NewBloomFilter(10)
	bbto := gorocksdb.NewDefaultBlockBasedTableOptions()
	bbto.SetBlockCache(gorocksdb.NewLRUCache(3 << 30))
	bbto.SetFilterPolicy(fp)

	opts := gorocksdb.NewDefaultOptions()
	opts.SetBlockBasedTableFactory(bbto)
	opts.SetCreateIfMissing(true)
	opts.SetMaxBackgroundCompactions(4)

	db, err := gorocksdb.OpenDb(opts, path)
	if err != nil {
		return
	}

	wo := gorocksdb.NewDefaultWriteOptions()
	ro := gorocksdb.NewDefaultReadOptions()
	ro.SetFillCache(false)

	return &RocksDB{db, wo, ro}, nil
}

// Close releases the RocksDB environment opened in NewRocksDB.
func (d *RocksDB) Close() error {
	log.Printf("rocksdb: close")
	d.wo.Destroy()
	d.ro.Destroy()
	d.db.Close()
	return nil
}

func (d *RocksDB) GetAddress(txid string, vout uint32) (string, error) {
	log.Printf("rocksdb: outpoint get %s:%d", txid, vout)
	k, err := packOutpointKey(txid, vout)
	if err != nil {
		return "", err
	}
	v, err := d.db.Get(d.ro, k)
	if err != nil {
		return "", err
	}
	if v.Size() == 0 {
		return "", ErrNotFound
	}
	defer v.Free()
	return unpackAddress(v.Data())
}

func (d *RocksDB) GetTransactions(address string, lower uint32, higher uint32, fn func(txids []string) error) (err error) {
	log.Printf("rocksdb: address get %d:%d %s", lower, higher, address)

	kstart, err := packAddressKey(lower, address)
	if err != nil {
		return err
	}
	kstop, err := packAddressKey(higher, address)
	if err != nil {
		return err
	}

	it := d.db.NewIterator(d.ro)
	defer it.Close()

	for it.Seek(kstart); it.Valid(); it.Next() {
		k := it.Key()
		v := it.Value()
		if bytes.Compare(k.Data(), kstop) > 0 {
			break
		}
		txids, err := unpackAddressVal(v.Data())
		if err != nil {
			return err
		}
		if err := fn(txids); err != nil {
			return err
		}
	}
	return nil
}

func (d *RocksDB) ConnectBlock(block *Block, txids map[string][]string) error {
	return d.writeBlock(block, txids, false /* delete */)
}

func (d *RocksDB) DisconnectBlock(block *Block, txids map[string][]string) error {
	return d.writeBlock(block, txids, true /* delete */)
}

func (d *RocksDB) writeBlock(
	block *Block,
	txids map[string][]string,
	delete bool,
) error {
	wb := gorocksdb.NewWriteBatch()
	defer wb.Destroy()

	if err := d.writeHeight(wb, block, delete); err != nil {
		return err
	}
	if err := d.writeOutpoints(wb, block, delete); err != nil {
		return err
	}
	if err := d.writeAddresses(wb, block, txids, delete); err != nil {
		return err
	}

	return d.db.Write(d.wo, wb)
}

// Address Index

func (d *RocksDB) writeAddresses(
	wb *gorocksdb.WriteBatch,
	block *Block,
	txids map[string][]string,
	delete bool,
) error {
	if delete {
		log.Printf("rocksdb: address delete %d in %d %s", len(txids), block.Height, block.Hash)
	} else {
		log.Printf("rocksdb: address put %d in %d %s", len(txids), block.Height, block.Hash)
	}

	for addr, txids := range txids {
		k, err := packAddressKey(block.Height, addr)
		if err != nil {
			return err
		}
		v, err := packAddressVal(txids)
		if err != nil {
			return err
		}
		if delete {
			wb.Delete(k)
		} else {
			wb.Put(k, v)
		}

	}
	return nil
}

func packAddressKey(height uint32, address string) (b []byte, err error) {
	b, err = packAddress(address)
	if err != nil {
		return
	}
	h := packUint(height)
	b = append(b, h...)
	return
}

func packAddressVal(txids []string) (b []byte, err error) {
	for _, txid := range txids {
		t, err := packTxid(txid)
		if err != nil {
			return nil, err
		}
		b = append(b, t...)
	}
	return
}

const txidLen = 32

func unpackAddressVal(b []byte) (txids []string, err error) {
	for i := 0; i < len(b); i += txidLen {
		t, err := unpackTxid(b[i : i+txidLen])
		if err != nil {
			return nil, err
		}
		txids = append(txids, t)
	}
	return
}

// Outpoint index

func (d *RocksDB) writeOutpoints(
	wb *gorocksdb.WriteBatch,
	block *Block,
	delete bool,
) error {
	if delete {
		log.Printf("rocksdb: outpoints delete %d in %d %s", len(block.Txs), block.Height, block.Hash)
	} else {
		log.Printf("rocksdb: outpoints put %d in %d %s", len(block.Txs), block.Height, block.Hash)
	}

	for _, tx := range block.Txs {
		for _, vout := range tx.Vout {
			k, err := packOutpointKey(tx.Txid, vout.N)
			if err != nil {
				return err
			}
			v, err := packAddress(vout.GetAddress())
			if err != nil {
				return err
			}
			if delete {
				wb.Delete(k)
			} else {
				if len(v) > 0 {
					wb.Put(k, v)
				}
			}
		}
	}
	return nil
}

func packOutpointKey(txid string, vout uint32) (b []byte, err error) {
	t, err := packTxid(txid)
	if err != nil {
		return nil, err
	}
	v := packVarint(vout)
	b = append(b, t...)
	b = append(b, v...)
	return
}

// Block index

const (
	lastBlockHash = 0x00
)

var (
	lastBlockHashKey = []byte{lastBlockHash}
)

func (d *RocksDB) GetLastBlockHash() (string, error) {
	v, err := d.db.Get(d.ro, lastBlockHashKey)
	if err != nil {
		return "", err
	}
	defer v.Free()
	if v.Size() == 0 {
		return "", ErrNotFound
	}
	return unpackBlockValue(v.Data())
}

func (d *RocksDB) writeHeight(
	wb *gorocksdb.WriteBatch,
	block *Block,
	delete bool,
) error {
	if delete {
		log.Printf("rocksdb: height delete %d %s", block.Height, block.Hash)
	} else {
		log.Printf("rocksdb: height put %d %s", block.Height, block.Hash)
	}

	bk := packUint(block.Height)

	if delete {
		bv, err := packBlockValue(block.Prev)
		if err != nil {
			return err
		}
		wb.Delete(bk)
		wb.Put(lastBlockHashKey, bv)

	} else {
		bv, err := packBlockValue(block.Hash)
		if err != nil {
			return err
		}
		wb.Put(bk, bv)
		wb.Put(lastBlockHashKey, bv)
	}

	return nil
}

// Helpers

func packUint(i uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, i)
	return b
}

func packVarint(i uint32) []byte {
	b := make([]byte, vlq.MaxLen32)
	n := vlq.PutUint(b, uint64(i))
	return b[:n]
}

var (
	ErrInvalidAddress = errors.New("invalid address")
)

func packAddress(s string) ([]byte, error) {
	var b []byte
	if len(s) == 0 {
		return b, nil
	}
	b = base58.Decode(s)
	if len(b) <= 4 {
		return nil, ErrInvalidAddress
	}
	b = b[:len(b)-4] // Slice off the checksum
	return b, nil
}

func unpackAddress(b []byte) (string, error) {
	if len(b) == 0 {
		return "", nil
	}
	if len(b) == 1 {
		return "", ErrInvalidAddress
	}
	return base58.CheckEncode(b[1:], b[0]), nil
}

func packTxid(s string) ([]byte, error) {
	return hex.DecodeString(s)
}

func unpackTxid(b []byte) (string, error) {
	return hex.EncodeToString(b), nil
}

func packBlockValue(hash string) ([]byte, error) {
	return hex.DecodeString(hash)
}

func unpackBlockValue(b []byte) (string, error) {
	return hex.EncodeToString(b), nil
}
