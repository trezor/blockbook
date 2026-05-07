package db

import (
	vlq "github.com/bsm/go-vlq"
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
