package db

import (
	"bytes"

	vlq "github.com/bsm/go-vlq"
	"github.com/golang/glog"
	"github.com/linxGnu/grocksdb"
	"github.com/trezor/blockbook/bchain"
)

// Per-protocol contract metadata in cfErcProtocols, decoupled from the
// sync-owned cfContracts row. Two prefixes share the CF:
//
//	0x00 || protocolID(1B) || addrDesc                          → packVaruint(persistHeight) || payload
//	0x01 || protocolID(1B) || packUint32(persistHeight) || addrDesc → (empty)
//
// byContract is the read path; byHeight is the secondary index disconnect uses
// to revert rows whose persist-height fell into a reorged range.
const (
	ercProtocolKeyByContract byte = 0x00
	ercProtocolKeyByHeight   byte = 0x01

	// ErcProtocolErc4626 marks a confirmed ERC4626 vault. New protocols
	// take the next free byte; 0x00 is reserved.
	ErcProtocolErc4626 byte = 0x01
)

func protocolByContractKey(protocolID byte, addrDesc bchain.AddressDescriptor) []byte {
	buf := make([]byte, 0, 2+len(addrDesc))
	buf = append(buf, ercProtocolKeyByContract, protocolID)
	buf = append(buf, addrDesc...)
	return buf
}

func protocolByHeightKey(protocolID byte, height uint32, addrDesc bchain.AddressDescriptor) []byte {
	buf := make([]byte, 0, 2+4+len(addrDesc))
	buf = append(buf, ercProtocolKeyByHeight, protocolID)
	buf = append(buf, packUint(height)...)
	buf = append(buf, addrDesc...)
	return buf
}

func packErcProtocolValue(persistHeight uint32, payload []byte) []byte {
	varBuf := make([]byte, vlq.MaxLen64)
	l := packVaruint(uint(persistHeight), varBuf)
	out := make([]byte, 0, l+len(payload))
	out = append(out, varBuf[:l]...)
	out = append(out, payload...)
	return out
}

func unpackErcProtocolValue(buf []byte) (persistHeight uint32, payload []byte, ok bool) {
	h, l, ok := unpackVaruintSafe(buf)
	if !ok {
		return 0, nil, false
	}
	return uint32(h), buf[l:], true
}

// SetErcProtocol persists a per-protocol detection record anchored to
// persistHeight (the API request's bestHeight, i.e. the multicall's pinned
// height — proxy upgrades make deploy-height an unreliable provenance). A
// future disconnect of that range deletes the row via the byHeight index.
//
// observedBlockHash and observedReorgGen are sampled before the multicall.
// Under connectBlockMux we refuse the write if either has shifted, closing
// the race where a reorg disconnects persistHeight while the multicall is in
// flight. False positives cost one re-probe.
//
// persistHeight==0 is refused defensively (no realistic disconnect range
// would clean it up). On payload conflict we refuse and warn rather than
// overwrite.
func (d *RocksDB) SetErcProtocol(addrDesc bchain.AddressDescriptor, protocolID byte, payload []byte, persistHeight uint32, observedBlockHash string, observedReorgGen uint64) error {
	if len(addrDesc) == 0 || persistHeight == 0 {
		return nil
	}

	d.connectBlockMux.Lock()
	defer d.connectBlockMux.Unlock()

	if d.reorgGen.Load() != observedReorgGen {
		// Reorg ran during the request; drop, next request re-probes.
		return nil
	}
	if observedBlockHash != "" {
		currentHash, err := d.GetBlockHash(persistHeight)
		if err != nil {
			return err
		}
		if currentHash != observedBlockHash {
			// Observed height replaced since the multicall; drop.
			return nil
		}
	}

	byContract := protocolByContractKey(protocolID, addrDesc)
	val, err := d.db.GetCF(d.ro, d.cfh[cfErcProtocols], byContract)
	if err != nil {
		return err
	}
	defer val.Free()
	if buf := val.Data(); len(buf) > 0 {
		_, existingPayload, ok := unpackErcProtocolValue(buf)
		if ok {
			// Drop any cachedContracts entry that may have been populated
			// before the existing row landed and still carries
			// IsErc4626=false. Applies on both the idempotent path and the
			// conflict-refusal path — neither writes, but both must clear
			// stale negatives so the next reader sees the persisted row.
			cachedContracts.delete(string(addrDesc))
			if !bytes.Equal(existingPayload, payload) {
				glog.Warningf("SetErcProtocol: refusing to overwrite protocol %d row for %x: stored payload differs", protocolID, addrDesc)
			}
			return nil
		}
	}

	wb := grocksdb.NewWriteBatch()
	defer wb.Destroy()
	wb.PutCF(d.cfh[cfErcProtocols], byContract, packErcProtocolValue(persistHeight, payload))
	wb.PutCF(d.cfh[cfErcProtocols], protocolByHeightKey(protocolID, persistHeight, addrDesc), nil)
	if err := d.WriteBatch(wb); err != nil {
		return err
	}
	// Bump before the cache delete: a concurrent GetContractInfo that already
	// sampled the old protocolGen and is about to add a stale-negative entry
	// will mismatch on the next read and miss, even if its add lands after our
	// delete clears the slot.
	d.protocolGen.Add(1)
	cachedContracts.delete(string(addrDesc))
	return nil
}

// disconnectErcProtocols deletes per-protocol rows whose persist-height
// falls into [lower, higher] via a byHeight range scan per protocolID.
func (d *RocksDB) disconnectErcProtocols(wb *grocksdb.WriteBatch, lower, higher uint32) error {
	for _, protocolID := range []byte{ErcProtocolErc4626} {
		if err := d.disconnectErcProtocolRange(wb, protocolID, lower, higher); err != nil {
			return err
		}
	}
	return nil
}

func (d *RocksDB) disconnectErcProtocolRange(wb *grocksdb.WriteBatch, protocolID byte, lower, higher uint32) error {
	startKey := []byte{ercProtocolKeyByHeight, protocolID}
	startKey = append(startKey, packUint(lower)...)
	endKey := []byte{ercProtocolKeyByHeight, protocolID}
	endKey = append(endKey, packUint(higher+1)...)

	it := d.db.NewIteratorCF(d.ro, d.cfh[cfErcProtocols])
	defer it.Close()
	for it.Seek(startKey); it.Valid(); it.Next() {
		key := it.Key().Data()
		if bytes.Compare(key, endKey) >= 0 {
			it.Key().Free()
			break
		}
		// key layout: 0x01 || protocolID(1B) || packUint32(height)(4B) || addrDesc
		const headerLen = 2 + 4
		if len(key) <= headerLen {
			it.Key().Free()
			continue
		}
		addrDesc := bchain.AddressDescriptor(append([]byte(nil), key[headerLen:]...))
		byHeightKey := append([]byte(nil), key...) // iterator owns the buffer
		wb.DeleteCF(d.cfh[cfErcProtocols], protocolByContractKey(protocolID, addrDesc))
		wb.DeleteCF(d.cfh[cfErcProtocols], byHeightKey)
		cachedContracts.delete(string(addrDesc))
		it.Key().Free()
	}
	if err := it.Err(); err != nil {
		return err
	}
	return nil
}

// GetErcProtocol returns the persisted payload for (addrDesc, protocolID)
// if present. ok=false with err=nil means the row is absent.
func (d *RocksDB) GetErcProtocol(addrDesc bchain.AddressDescriptor, protocolID byte) (payload []byte, persistHeight uint32, ok bool, err error) {
	if len(addrDesc) == 0 {
		return nil, 0, false, nil
	}
	val, err := d.db.GetCF(d.ro, d.cfh[cfErcProtocols], protocolByContractKey(protocolID, addrDesc))
	if err != nil {
		return nil, 0, false, err
	}
	defer val.Free()
	buf := val.Data()
	if len(buf) == 0 {
		return nil, 0, false, nil
	}
	h, p, ok := unpackErcProtocolValue(buf)
	if !ok {
		return nil, 0, false, nil
	}
	out := make([]byte, len(p))
	copy(out, p)
	return out, h, true, nil
}
