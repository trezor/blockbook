package db

import (
	vlq "github.com/bsm/go-vlq"
	"github.com/linxGnu/grocksdb"
	"github.com/trezor/blockbook/bchain"
)

var cachedContracts = newContractInfoLRU(cachedContractsLRUMaxSize)

const (
	// Bit 0 of the ERC4626 protocol payload's flags varuint — IsErc4626.
	contractInfoFlagErc4626 uint = 1 << 0

	// Protocol-extension tail header. Marker bit 15 identifies a versioned
	// extensions block; any trailing varuint without it is treated as junk and
	// skipped, so the core record decodes unaffected.
	contractInfoExtensionsMarker   uint = 1 << 15
	contractInfoExtensionsVersion1 uint = contractInfoExtensionsMarker | 1

	contractInfoProtocolErc4626 uint = 1
)

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
	buf = append(buf, packContractInfoProtocolExtensions(contractInfo)...)
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
	ui, l = unpackVaruint(buf)
	contractInfo.DestructedInBlock = uint32(ui)
	buf = buf[l:]
	if len(buf) == 0 {
		return &contractInfo, nil
	}
	ui, l = unpackVaruint(buf)
	if l == 0 {
		return &contractInfo, nil
	}
	if ui&contractInfoExtensionsMarker != 0 {
		unpackContractInfoProtocolExtensions(&contractInfo, ui, buf[l:])
	}
	return &contractInfo, nil
}

func packContractInfoProtocolExtensions(contractInfo *bchain.ContractInfo) []byte {
	extensionCount := 0
	if contractInfo.IsErc4626 || contractInfo.Erc4626AssetContract != "" {
		extensionCount++
	}
	if extensionCount == 0 {
		return nil
	}
	varBuf := make([]byte, vlq.MaxLen64)
	buf := make([]byte, 0, 64)
	l := packVaruint(contractInfoExtensionsVersion1, varBuf)
	buf = append(buf, varBuf[:l]...)
	l = packVaruint(uint(extensionCount), varBuf)
	buf = append(buf, varBuf[:l]...)
	if contractInfo.IsErc4626 || contractInfo.Erc4626AssetContract != "" {
		payload := packContractInfoErc4626Payload(contractInfo)
		l = packVaruint(contractInfoProtocolErc4626, varBuf)
		buf = append(buf, varBuf[:l]...)
		l = packVaruint(uint(len(payload)), varBuf)
		buf = append(buf, varBuf[:l]...)
		buf = append(buf, payload...)
	}
	return buf
}

func packContractInfoErc4626Payload(contractInfo *bchain.ContractInfo) []byte {
	var flags uint
	if contractInfo.IsErc4626 {
		flags |= contractInfoFlagErc4626
	}
	varBuf := make([]byte, vlq.MaxLen64)
	l := packVaruint(flags, varBuf)
	buf := make([]byte, 0, l+len(contractInfo.Erc4626AssetContract)+4)
	buf = append(buf, varBuf[:l]...)
	buf = append(buf, packString(contractInfo.Erc4626AssetContract)...)
	return buf
}

func unpackContractInfoProtocolExtensions(contractInfo *bchain.ContractInfo, header uint, buf []byte) {
	if header != contractInfoExtensionsVersion1 {
		return
	}
	count, l, ok := unpackVaruintSafe(buf)
	if !ok {
		return
	}
	buf = buf[l:]
	for i := uint(0); i < count && len(buf) > 0; i++ {
		protocolID, ll, ok := unpackVaruintSafe(buf)
		if !ok {
			return
		}
		buf = buf[ll:]
		payloadLen, ll, ok := unpackVaruintSafe(buf)
		if !ok {
			return
		}
		buf = buf[ll:]
		if int(payloadLen) > len(buf) {
			return
		}
		payload := buf[:payloadLen]
		switch protocolID {
		case contractInfoProtocolErc4626:
			unpackContractInfoErc4626Payload(contractInfo, payload)
		}
		buf = buf[payloadLen:]
	}
}

func unpackContractInfoErc4626Payload(contractInfo *bchain.ContractInfo, payload []byte) {
	flags, l, ok := unpackVaruintSafe(payload)
	if !ok {
		return
	}
	contractInfo.IsErc4626 = flags&contractInfoFlagErc4626 != 0
	if l == len(payload) {
		return
	}
	if assetContract, _, ok := unpackStringSafe(payload[l:]); ok {
		contractInfo.Erc4626AssetContract = assetContract
	}
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
	contractInfo, found := cachedContracts.get(cacheKey)
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
		cachedContracts.add(cacheKey, contractInfo)
	}
	return contractInfo, nil
}

// SetContractInfoErc4626Vault persists the cached ERC4626 invariants for a
// detected vault: marks IsErc4626=true and stores the underlying asset address.
// If the row does not yet exist, this is a no-op - the contractInfo path will
// have triggered a lazy ContractInfo fetch separately, so the row will be
// present by the time we reach here in normal flow. Idempotent.
func (d *RocksDB) SetContractInfoErc4626Vault(address string, assetContract string) error {
	contract, err := d.chainParser.GetAddrDescFromAddress(address)
	if err != nil || contract == nil {
		return err
	}
	contractInfo, err := d.GetContractInfo(contract, "")
	if err != nil {
		return err
	}
	if contractInfo == nil {
		return nil
	}
	changed := false
	if !contractInfo.IsErc4626 {
		contractInfo.IsErc4626 = true
		changed = true
	}
	if contractInfo.Erc4626AssetContract != assetContract {
		contractInfo.Erc4626AssetContract = assetContract
		changed = true
	}
	if !changed {
		return nil
	}
	if err := d.db.PutCF(d.wo, d.cfh[cfContracts], contract, packContractInfo(contractInfo)); err != nil {
		return err
	}
	cachedContracts.add(string(contract), contractInfo)
	return nil
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
