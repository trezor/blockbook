package db

import (
	"bytes"

	vlq "github.com/bsm/go-vlq"
	"github.com/golang/glog"
	"github.com/linxGnu/grocksdb"
	"github.com/trezor/blockbook/bchain"
)

// Per-protocol metadata for contracts is held in cfErcProtocols, decoupled
// from the sync-owned cfContracts row. Each protocol-specific detection writes
// independently keyed rows so writes from one protocol never conflict with
// another and the sync write path is unaffected.
//
// Two prefixes share the column family:
//
//	0x00 || protocolID(1B) || addrDesc                          → packVaruint(persistHeight) || payload
//	0x01 || protocolID(1B) || packUint32(persistHeight) || addrDesc → (empty)
//
// The byContract prefix is the read path; the byHeight prefix is the secondary
// index used by disconnect to revert rows whose persist-height fell into a
// reorged range.
const (
	contractProtocolKeyByContract byte = 0x00
	contractProtocolKeyByHeight   byte = 0x01

	// ContractProtocolErc4626 marks a contract as a confirmed ERC4626 vault.
	// New protocols append the next free byte; 0x00 is reserved.
	ContractProtocolErc4626 byte = 0x01
)

func protocolByContractKey(protocolID byte, addrDesc bchain.AddressDescriptor) []byte {
	buf := make([]byte, 0, 2+len(addrDesc))
	buf = append(buf, contractProtocolKeyByContract, protocolID)
	buf = append(buf, addrDesc...)
	return buf
}

func protocolByHeightKey(protocolID byte, height uint32, addrDesc bchain.AddressDescriptor) []byte {
	buf := make([]byte, 0, 2+4+len(addrDesc))
	buf = append(buf, contractProtocolKeyByHeight, protocolID)
	buf = append(buf, packUint(height)...)
	buf = append(buf, addrDesc...)
	return buf
}

func packContractProtocolValue(persistHeight uint32, payload []byte) []byte {
	varBuf := make([]byte, vlq.MaxLen64)
	l := packVaruint(uint(persistHeight), varBuf)
	out := make([]byte, 0, l+len(payload))
	out = append(out, varBuf[:l]...)
	out = append(out, payload...)
	return out
}

func unpackContractProtocolValue(buf []byte) (persistHeight uint32, payload []byte, ok bool) {
	h, l, ok := unpackVaruintSafe(buf)
	if !ok {
		return 0, nil, false
	}
	return uint32(h), buf[l:], true
}

// SetContractProtocol persists a per-protocol detection record for addrDesc.
// persistHeight is the chain height at which the protocol fact was observed;
// it anchors the row to that height so a future disconnect of that range
// removes the row via the byHeight secondary index. Callers pass the API
// request's bestHeight here (i.e. the height the multicall was pinned to),
// not the contract's deploy height — proxy upgrades make the deploy height
// an unreliable provenance for the protocol fact.
//
// observedBlockHash and observedReorgGen identify the canonical chain state
// sampled at the start of the API request, before the multicall was issued.
// They close a race where a reorg disconnects persistHeight while the
// multicall is in flight: the disconnect bumps reorgGen, finishes its byHeight
// scan (finding no row to remove because we haven't written yet), and releases
// the mutex. By the time we acquire the mutex here, either the generation has
// advanced or the block hash at persistHeight no longer matches, so we refuse
// the write rather than persist a row anchored to a non-canonical height that
// no future disconnect would catch. False positives (an unrelated disconnect
// during the request) cost one re-probe on the next request, which is fine.
//
// The read-modify-write is serialized with the disconnect path via
// connectBlockMux so the byContract / byHeight pair is observed atomically:
// neither a concurrent ConnectBlock nor DisconnectBlockRangeEthereumType can
// interleave between the existence check and the write batch.
//
// Defensive refusal on persistHeight == 0: blockbook can be asked to fetch
// contract metadata via direct RPC where CreatedInBlock isn't known and ends
// up zero. We do not expect bestHeight==0 either (the chain is always synced
// to at least height 1 before serving), but guarding here keeps an
// uninitialized caller from writing a row that no realistic reorg range
// would ever clean up.
//
// On conflict (an existing row with a *different* payload), the write is
// refused with a warning rather than overwriting. This is a discrepancy that
// should surface, not be papered over.
func (d *RocksDB) SetContractProtocol(addrDesc bchain.AddressDescriptor, protocolID byte, payload []byte, persistHeight uint32, observedBlockHash string, observedReorgGen uint64) error {
	if len(addrDesc) == 0 || persistHeight == 0 {
		return nil
	}

	d.connectBlockMux.Lock()
	defer d.connectBlockMux.Unlock()

	if d.reorgGen.Load() != observedReorgGen {
		// A disconnect ran between observation and this write. Our multicall
		// result is from the previous canonical chain; persisting it would
		// anchor a row to a height the disconnect path has already passed.
		// Drop silently — the next request will re-probe under the new
		// generation.
		return nil
	}
	if observedBlockHash != "" {
		currentHash, err := d.GetBlockHash(persistHeight)
		if err != nil {
			return err
		}
		if currentHash != observedBlockHash {
			// The observed height has been disconnected or replaced since the
			// multicall was issued. Drop the stale observation; the next request
			// can re-probe against the current canonical block.
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
		_, existingPayload, ok := unpackContractProtocolValue(buf)
		if ok && bytes.Equal(existingPayload, payload) {
			return nil
		}
		if ok {
			glog.Warningf("SetContractProtocol: refusing to overwrite protocol %d row for %x: stored payload differs", protocolID, addrDesc)
			return nil
		}
	}

	wb := grocksdb.NewWriteBatch()
	defer wb.Destroy()
	wb.PutCF(d.cfh[cfErcProtocols], byContract, packContractProtocolValue(persistHeight, payload))
	wb.PutCF(d.cfh[cfErcProtocols], protocolByHeightKey(protocolID, persistHeight, addrDesc), nil)
	if err := d.WriteBatch(wb); err != nil {
		return err
	}
	cachedContracts.delete(string(addrDesc))
	return nil
}

// disconnectContractProtocols removes per-protocol rows whose persist-height
// falls into [lower, higher]. Called from DisconnectBlockRangeEthereumType
// after the rest of the block-disconnect batch has been built.
//
// The byHeight secondary index is laid out so a single range scan over
// [0x01||protocolID||packUint(lower), 0x01||protocolID||packUint(higher+1))
// yields exactly the affected rows. With the chain-time confirmation gate
// upstream this is virtually always empty in practice; it's the safety net
// for pathological deep reorgs.
//
// All known protocolIDs are scanned (currently just ERC4626).
func (d *RocksDB) disconnectContractProtocols(wb *grocksdb.WriteBatch, lower, higher uint32) error {
	for _, protocolID := range []byte{ContractProtocolErc4626} {
		if err := d.disconnectContractProtocolRange(wb, protocolID, lower, higher); err != nil {
			return err
		}
	}
	return nil
}

func (d *RocksDB) disconnectContractProtocolRange(wb *grocksdb.WriteBatch, protocolID byte, lower, higher uint32) error {
	startKey := []byte{contractProtocolKeyByHeight, protocolID}
	startKey = append(startKey, packUint(lower)...)
	endKey := []byte{contractProtocolKeyByHeight, protocolID}
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
		heightBytes := append([]byte(nil), key[2:headerLen]...)
		// reconstruct byContract key for the same (protocolID, addrDesc)
		wb.DeleteCF(d.cfh[cfErcProtocols], protocolByContractKey(protocolID, addrDesc))
		// delete the byHeight key itself; copy because the iterator owns the buffer
		byHeightKey := make([]byte, 0, headerLen+len(addrDesc))
		byHeightKey = append(byHeightKey, contractProtocolKeyByHeight, protocolID)
		byHeightKey = append(byHeightKey, heightBytes...)
		byHeightKey = append(byHeightKey, addrDesc...)
		wb.DeleteCF(d.cfh[cfErcProtocols], byHeightKey)
		cachedContracts.delete(string(addrDesc))
		it.Key().Free()
	}
	if err := it.Err(); err != nil {
		return err
	}
	return nil
}

// GetContractProtocol returns the persisted payload for (addrDesc, protocolID)
// if present. ok=false with err=nil means the row is absent.
func (d *RocksDB) GetContractProtocol(addrDesc bchain.AddressDescriptor, protocolID byte) (payload []byte, persistHeight uint32, ok bool, err error) {
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
	h, p, ok := unpackContractProtocolValue(buf)
	if !ok {
		return nil, 0, false, nil
	}
	out := make([]byte, len(p))
	copy(out, p)
	return out, h, true, nil
}
