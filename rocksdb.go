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

func (d *RocksDB) GetAddressTransactions(address string, lower uint32, higher uint32, fn func(txids []string) error) (err error) {
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

func (d *RocksDB) IndexBlock(block *Block, txids map[string][]string) error {
	wb := gorocksdb.NewWriteBatch()
	defer wb.Destroy()

	if err := d.writeHeight(wb, block); err != nil {
		return err
	}
	if err := d.writeOutpoints(wb, block); err != nil {
		return err
	}
	if err := d.writeAddresses(wb, block, txids); err != nil {
		return err
	}

	return d.db.Write(d.wo, wb)
}

// Address Index

func (d *RocksDB) writeAddresses(wb *gorocksdb.WriteBatch, block *Block, txids map[string][]string) error {
	log.Printf("rocksdb: address put %d in %d %s", len(txids), block.Height, block.Hash)

	for addr, txids := range txids {
		k, err := packAddressKey(block.Height, addr)
		if err != nil {
			return err
		}
		v, err := packAddressVal(txids)
		if err != nil {
			return err
		}
		wb.Put(k, v)
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

const transactionIDLen = 32

func unpackAddressVal(b []byte) (txids []string, err error) {
	for i := 0; i < len(b); i += transactionIDLen {
		t, err := unpackTxid(b[i : i+transactionIDLen])
		if err != nil {
			return nil, err
		}
		txids = append(txids, t)
	}
	return
}

// Outpoint index

func (d *RocksDB) writeOutpoints(wb *gorocksdb.WriteBatch, block *Block) error {
	log.Printf("rocksdb: outpoints put %d in %d %s", len(block.Txs), block.Height, block.Hash)

	for _, tx := range block.Txs {
		for _, vout := range tx.Vout {
			k, err := packOutpointKey(block.Height, tx.Txid, vout.N)
			if err != nil {
				return err
			}
			v, err := packOutpointValue(vout.ScriptPubKey.Addresses)
			if err != nil {
				return err
			}
			wb.Put(k, v)
		}
	}
	return nil
}

func packOutpointKey(height uint32, txid string, vout uint32) (b []byte, err error) {
	h := packUint(height)
	t, err := packTxid(txid)
	if err != nil {
		return nil, err
	}
	v := packVarint(vout)
	b = append(b, h...)
	b = append(b, t...)
	b = append(b, v...)
	return
}

func packOutpointValue(addrs []string) (b []byte, err error) {
	for _, addr := range addrs {
		a, err := packAddress(addr)
		if err != nil {
			return nil, err
		}
		i := packVarint(uint32(len(a)))
		b = append(b, i...)
		b = append(b, a...)
	}
	return
}

func unpackOutpointValue(b []byte) (addrs []string, err error) {
	r := bytes.NewReader(b)
	for r.Len() > 0 {
		alen, err := vlq.ReadUint(r)
		if err != nil {
			return nil, err
		}
		abuf := make([]byte, alen)
		_, err = r.Read(abuf)
		if err != nil {
			return nil, err
		}
		addr, err := unpackAddress(abuf)
		if err != nil {
			return nil, err
		}
		addrs = append(addrs, addr)
	}
	return
}

// Block index

func (d *RocksDB) writeHeight(wb *gorocksdb.WriteBatch, block *Block) error {
	log.Printf("rocksdb: height put %d %s", block.Height, block.Hash)

	bv, err := packBlockValue(block.Hash)
	if err != nil {
		return err
	}
	bk := packUint(block.Height)
	wb.Put(bk, bv)

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

func packAddress(s string) (b []byte, err error) {
	b = base58.Decode(s)
	if len(b) > 4 {
		b = b[:len(b)-4]
	} else {
		err = ErrInvalidAddress
	}
	return
}

func unpackAddress(b []byte) (s string, err error) {
	if len(b) > 1 {
		s = base58.CheckEncode(b[1:], b[0])
	} else {
		err = ErrInvalidAddress
	}
	return
}

func packTxid(s string) (b []byte, err error) {
	return hex.DecodeString(s)
}

func unpackTxid(b []byte) (s string, err error) {
	return hex.EncodeToString(b), nil
}

func packBlockValue(hash string) ([]byte, error) {
	return hex.DecodeString(hash)
}
