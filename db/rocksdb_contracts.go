package db

import (
	vlq "github.com/bsm/go-vlq"
	"github.com/juju/errors"
	"github.com/linxGnu/grocksdb"
	"github.com/trezor/blockbook/bchain"
)

var cachedContracts = newContractInfoLRU(cachedContractsLRUMaxSize)

func packContractInfo(contractInfo *bchain.ContractInfo) []byte {
	buf := packString(contractInfo.Name)
	buf = append(buf, packString(contractInfo.Symbol)...)
	buf = append(buf, packString(string(contractInfo.Standard))...)
	varBuf := make([]byte, vlq.MaxLen64)
	l := packVaruint(uint(contractInfo.Decimals), varBuf)
	buf = append(buf, varBuf[:l]...)
	l = packVaruint(uint(contractInfo.CreatedInBlock), varBuf)
	buf = append(buf, varBuf[:l]...)
	l = packVaruint(uint(contractInfo.DestructedInBlock), varBuf)
	buf = append(buf, varBuf[:l]...)
	return buf
}

func unpackContractInfo(buf []byte) (*bchain.ContractInfo, error) {
	var contractInfo bchain.ContractInfo
	var s string
	var l int
	var ui uint
	contractInfo.Name, l = unpackString(buf)
	buf = buf[l:]
	contractInfo.Symbol, l = unpackString(buf)
	buf = buf[l:]
	s, l = unpackString(buf)
	contractInfo.Standard = bchain.TokenStandardName(s)
	contractInfo.Type = bchain.TokenStandardName(s)
	buf = buf[l:]
	ui, l = unpackVaruint(buf)
	contractInfo.Decimals = int(ui)
	buf = buf[l:]
	ui, l = unpackVaruint(buf)
	contractInfo.CreatedInBlock = uint32(ui)
	buf = buf[l:]
	ui, _ = unpackVaruint(buf)
	contractInfo.DestructedInBlock = uint32(ui)
	return &contractInfo, nil
}

func unpackVaruintSafe(buf []byte) (uint, int, bool) {
	if len(buf) == 0 {
		return 0, 0, false
	}
	ui, l := unpackVaruint(buf)
	if l <= 0 || l > len(buf) {
		return 0, 0, false
	}
	return ui, l, true
}

func unpackStringSafe(buf []byte) (string, int, bool) {
	if len(buf) == 0 {
		return "", 0, false
	}
	sl, l, ok := unpackVaruintSafe(buf)
	if !ok {
		return "", 0, false
	}
	so := l + int(sl)
	if so < l || so > len(buf) {
		return "", 0, false
	}
	return string(buf[l:so]), so, true
}

func (d *RocksDB) GetContractInfoForAddress(address string) (*bchain.ContractInfo, error) {
	contract, err := d.chainParser.GetAddrDescFromAddress(address)
	if err != nil || contract == nil {
		return nil, err
	}
	return d.GetContractInfo(contract, "")
}

// GetContractInfo gets contract from cache or DB and possibly updates the standard from standardFromContext
// it is hard to guess the standard of the contract using API, it is easier to set it the first time the contract is processed in a tx
func (d *RocksDB) GetContractInfo(contract bchain.AddressDescriptor, standardFromContext bchain.TokenStandardName) (*bchain.ContractInfo, error) {
	cacheKey := string(contract)
	// Sample both counters before the CF reads. If a disconnect bumps reorgGen
	// (populate-after-delete race) or a SetErcProtocol bumps protocolGen
	// (populate-after-write race) during this call, the stamped entry will
	// mismatch on the next get and miss.
	reorgGen := d.reorgGen.Load()
	protocolGen := d.protocolGen.Load()
	contractInfo, found := cachedContracts.get(cacheKey, reorgGen, protocolGen)
	if !found {
		val, err := d.db.GetCF(d.ro, d.cfh[cfContracts], contract)
		if err != nil {
			return nil, err
		}
		defer val.Free()
		buf := val.Data()
		if len(buf) == 0 {
			return nil, nil
		}
		contractInfo, _ = unpackContractInfo(buf)
		addresses, _, _ := d.chainParser.GetAddressesFromAddrDesc(contract)
		if len(addresses) > 0 {
			contractInfo.Contract = addresses[0]
		}
		// if the standard is specified and stored contractInfo has unknown standard, set and store it
		if standardFromContext != bchain.UnknownTokenStandard && contractInfo.Standard == bchain.UnknownTokenStandard {
			contractInfo.Standard = standardFromContext
			contractInfo.Type = standardFromContext
			err = d.db.PutCF(d.wo, d.cfh[cfContracts], contract, packContractInfo(contractInfo))
			if err != nil {
				return nil, err
			}
		}
		// Merge ERC4626 detection from the per-protocol CF.
		if assetContract, ok, err := d.GetContractInfoErc4626Vault(contract); err != nil {
			return nil, err
		} else if ok {
			contractInfo.IsErc4626 = true
			contractInfo.Erc4626AssetContract = assetContract
		}
		cachedContracts.add(cacheKey, contractInfo, reorgGen, protocolGen)
	}
	return contractInfo, nil
}

// SetContractInfoErc4626Vault persists a detected vault's asset() address to
// the per-protocol CF. See SetErcProtocol for the persistHeight /
// observedBlockHash / observedReorgGen race rationale and refusal policy.
func (d *RocksDB) SetContractInfoErc4626Vault(address, assetContract string, persistHeight uint32, observedBlockHash string, observedReorgGen uint64) error {
	contract, err := d.chainParser.GetAddrDescFromAddress(address)
	if err != nil || contract == nil {
		return err
	}
	return d.SetErcProtocol(contract, ErcProtocolErc4626, packString(assetContract), persistHeight, observedBlockHash, observedReorgGen)
}

// GetContractInfoErc4626Vault returns the persisted asset() address, if any.
func (d *RocksDB) GetContractInfoErc4626Vault(contract bchain.AddressDescriptor) (assetContract string, ok bool, err error) {
	payload, _, ok, err := d.GetErcProtocol(contract, ErcProtocolErc4626)
	if err != nil || !ok {
		return "", ok, err
	}
	asset, _, ok := unpackStringSafe(payload)
	if !ok {
		return "", false, nil
	}
	return asset, true, nil
}

// StoreContractInfo stores contractInfo in DB
// if CreatedInBlock==0 and DestructedInBlock!=0, it is evaluated as a destruction of a contract, the contract info is updated
// in all other cases the contractInfo overwrites previously stored data in DB (however it should not really happen as contract is created only once)
func (d *RocksDB) StoreContractInfo(contractInfo *bchain.ContractInfo) error {
	wb := grocksdb.NewWriteBatch()
	defer wb.Destroy()
	if err := d.storeContractInfo(wb, contractInfo); err != nil {
		return err
	}
	return d.WriteBatch(wb)
}

func (d *RocksDB) storeContractInfo(wb *grocksdb.WriteBatch, contractInfo *bchain.ContractInfo) error {
	if contractInfo.Contract != "" {
		key, err := d.chainParser.GetAddrDescFromAddress(contractInfo.Contract)
		if err != nil {
			return err
		}
		if contractInfo.CreatedInBlock == 0 && contractInfo.DestructedInBlock != 0 {
			storedCI, err := d.GetContractInfo(key, "")
			if err != nil {
				return err
			}
			if storedCI == nil {
				return nil
			}
			storedCI.DestructedInBlock = contractInfo.DestructedInBlock
			contractInfo = storedCI
		}
		wb.PutCF(d.cfh[cfContracts], key, packContractInfo(contractInfo))
		cacheKey := string(key)
		cachedContracts.delete(cacheKey)
	}
	return nil
}

// ListContractInfos returns up to limit stored contract records ordered by
// address descriptor, starting at the optional from address (inclusive), and
// the address to pass as from to fetch the next page ("" when the listing is
// complete). A page limit is mandatory: the rows are sync-populated (every
// contract creation when internal data processing is enabled), so the full
// set can run into millions on a busy chain.
func (d *RocksDB) ListContractInfos(from string, limit int) ([]bchain.ContractInfo, string, error) {
	var start bchain.AddressDescriptor
	if from != "" {
		var err error
		start, err = d.chainParser.GetAddrDescFromAddress(from)
		if err != nil {
			return nil, "", err
		}
		if start == nil {
			return nil, "", errors.Errorf("invalid address %s", from)
		}
	}
	it := d.db.NewIteratorCF(d.ro, d.cfh[cfContracts])
	defer it.Close()
	if start != nil {
		it.Seek(start)
	} else {
		it.SeekToFirst()
	}
	contracts := make([]bchain.ContractInfo, 0, limit)
	for ; it.Valid(); it.Next() {
		// The key is the contract's address descriptor. A key that fails to
		// decode to an address is a corrupt row: fail loudly (as the
		// unpackContractInfo error below does) rather than fall through to
		// return next="", which a caller cannot tell apart from a completed
		// listing and which would silently drop the boundary row's cursor —
		// or return an in-page record with an empty Contract.
		addresses, _, err := d.chainParser.GetAddressesFromAddrDesc(it.Key().Data())
		if err != nil {
			return nil, "", err
		}
		if len(addresses) == 0 {
			return nil, "", errors.Errorf("no address for contract descriptor %x", it.Key().Data())
		}
		if len(contracts) == limit {
			// one more row exists — its address is the cursor of the next page
			return contracts, addresses[0], nil
		}
		contractInfo, err := unpackContractInfo(it.Value().Data())
		if err != nil {
			return nil, "", err
		}
		contractInfo.Contract = addresses[0]
		contracts = append(contracts, *contractInfo)
	}
	return contracts, "", nil
}

// DeleteContractInfoForAddress removes the stored contract metadata for the given
// address (and its in-memory cache entry) so the next read re-fetches it from the
// backend node. It returns the purged record (nil when no row was stored): the
// whole row is discarded, including the sync-owned CreatedInBlock and
// DestructedInBlock, which a backend re-fetch cannot restore — only a reindex or
// storing the returned record back can. ERC-4626 protocol-detection rows in
// cfErcProtocols are kept: they live in their own column family with their own
// lifecycle and are merged on read.
func (d *RocksDB) DeleteContractInfoForAddress(address string) (*bchain.ContractInfo, error) {
	contract, err := d.chainParser.GetAddrDescFromAddress(address)
	if err != nil {
		return nil, err
	}
	if contract == nil {
		return nil, errors.Errorf("invalid address %s", address)
	}
	val, err := d.db.GetCF(d.ro, d.cfh[cfContracts], contract)
	if err != nil {
		return nil, err
	}
	buf := val.Data()
	var purged *bchain.ContractInfo
	if len(buf) > 0 {
		purged, _ = unpackContractInfo(buf)
		addresses, _, _ := d.chainParser.GetAddressesFromAddrDesc(contract)
		if len(addresses) > 0 {
			purged.Contract = addresses[0]
		}
	}
	val.Free()
	if purged == nil {
		return nil, nil
	}
	if err := d.db.DeleteCF(d.wo, d.cfh[cfContracts], contract); err != nil {
		return nil, err
	}
	// Bump before the cache delete: a concurrent GetContractInfo that already
	// sampled the old protocolGen and is about to re-add the just-deleted row
	// will mismatch on the next read and miss, even if its add lands after our
	// delete clears the slot (same idiom as SetErcProtocol).
	d.protocolGen.Add(1)
	cachedContracts.delete(string(contract))
	return purged, nil
}
