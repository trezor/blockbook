package db

import (
	"bytes"
	"encoding/hex"
	"math/big"

	"github.com/flier/gorocksdb"
	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
)

const InternalTxIndexOffset = 1
const ContractIndexOffset = 2

// AddrContract is Contract address with number of transactions done by given address
type AddrContract struct {
	Type     bchain.TokenTransferType
	Contract bchain.AddressDescriptor
	Txs      uint
	Value    big.Int                       // single value of ERC20
	Ids      []big.Int                     // multiple ERC721 tokens
	IdValues []bchain.TokenTransferIdValue // multiple ERC1155 tokens
}

// AddrContracts contains number of transactions and contracts for an address
type AddrContracts struct {
	TotalTxs       uint
	NonContractTxs uint
	InternalTxs    uint
	Contracts      []AddrContract
}

// packAddrContract packs AddrContracts into a byte buffer
func packAddrContracts(acs *AddrContracts) []byte {
	buf := make([]byte, 0, 128)
	varBuf := make([]byte, maxPackedBigintBytes)
	l := packVaruint(acs.TotalTxs, varBuf)
	buf = append(buf, varBuf[:l]...)
	l = packVaruint(acs.NonContractTxs, varBuf)
	buf = append(buf, varBuf[:l]...)
	l = packVaruint(acs.InternalTxs, varBuf)
	buf = append(buf, varBuf[:l]...)
	for _, ac := range acs.Contracts {
		buf = append(buf, ac.Contract...)
		l = packVaruint(uint(ac.Type)+ac.Txs<<2, varBuf)
		buf = append(buf, varBuf[:l]...)
		if ac.Type == bchain.ERC20 {
			l = packBigint(&ac.Value, varBuf)
			buf = append(buf, varBuf[:l]...)
		} else if ac.Type == bchain.ERC721 {
			l = packVaruint(uint(len(ac.Ids)), varBuf)
			buf = append(buf, varBuf[:l]...)
			for i := range ac.Ids {
				l = packBigint(&ac.Ids[i], varBuf)
				buf = append(buf, varBuf[:l]...)
			}
		} else { // bchain.ERC1155
			l = packVaruint(uint(len(ac.IdValues)), varBuf)
			buf = append(buf, varBuf[:l]...)
			for i := range ac.IdValues {
				l = packBigint(&ac.IdValues[i].Id, varBuf)
				buf = append(buf, varBuf[:l]...)
				l = packBigint(&ac.IdValues[i].Value, varBuf)
				buf = append(buf, varBuf[:l]...)
			}
		}
	}
	return buf
}

func unpackAddrContracts(buf []byte, addrDesc bchain.AddressDescriptor) (*AddrContracts, error) {
	tt, l := unpackVaruint(buf)
	buf = buf[l:]
	nct, l := unpackVaruint(buf)
	buf = buf[l:]
	ict, l := unpackVaruint(buf)
	buf = buf[l:]
	c := make([]AddrContract, 0, 4)
	for len(buf) > 0 {
		if len(buf) < eth.EthereumTypeAddressDescriptorLen {
			return nil, errors.New("Invalid data stored in cfAddressContracts for AddrDesc " + addrDesc.String())
		}
		contract := append(bchain.AddressDescriptor(nil), buf[:eth.EthereumTypeAddressDescriptorLen]...)
		txs, l := unpackVaruint(buf[eth.EthereumTypeAddressDescriptorLen:])
		buf = buf[eth.EthereumTypeAddressDescriptorLen+l:]
		ttt := bchain.TokenTransferType(txs & 3)
		txs >>= 2
		ac := AddrContract{
			Type:     ttt,
			Contract: contract,
			Txs:      txs,
		}
		if ttt == bchain.ERC20 {
			b, ll := unpackBigint(buf)
			buf = buf[ll:]
			ac.Value = b
		} else {
			len, ll := unpackVaruint(buf)
			buf = buf[ll:]
			if ttt == bchain.ERC721 {
				ac.Ids = make([]big.Int, len)
				for i := uint(0); i < len; i++ {
					b, ll := unpackBigint(buf)
					buf = buf[ll:]
					ac.Ids[i] = b
				}
			} else {
				ac.IdValues = make([]bchain.TokenTransferIdValue, len)
				for i := uint(0); i < len; i++ {
					b, ll := unpackBigint(buf)
					buf = buf[ll:]
					ac.IdValues[i].Id = b
					b, ll = unpackBigint(buf)
					buf = buf[ll:]
					ac.IdValues[i].Value = b
				}
			}
		}
		c = append(c, ac)
	}
	return &AddrContracts{
		TotalTxs:       tt,
		NonContractTxs: nct,
		InternalTxs:    ict,
		Contracts:      c,
	}, nil
}

func (d *RocksDB) storeAddressContracts(wb *gorocksdb.WriteBatch, acm map[string]*AddrContracts) error {
	for addrDesc, acs := range acm {
		// address with 0 contracts is removed from db - happens on disconnect
		if acs == nil || (acs.NonContractTxs == 0 && acs.InternalTxs == 0 && len(acs.Contracts) == 0) {
			wb.DeleteCF(d.cfh[cfAddressContracts], bchain.AddressDescriptor(addrDesc))
		} else {
			buf := packAddrContracts(acs)
			wb.PutCF(d.cfh[cfAddressContracts], bchain.AddressDescriptor(addrDesc), buf)
		}
	}
	return nil
}

// GetAddrDescContracts returns AddrContracts for given addrDesc
func (d *RocksDB) GetAddrDescContracts(addrDesc bchain.AddressDescriptor) (*AddrContracts, error) {
	val, err := d.db.GetCF(d.ro, d.cfh[cfAddressContracts], addrDesc)
	if err != nil {
		return nil, err
	}
	defer val.Free()
	buf := val.Data()
	if len(buf) == 0 {
		return nil, nil
	}
	return unpackAddrContracts(buf, addrDesc)
}

func findContractInAddressContracts(contract bchain.AddressDescriptor, contracts []AddrContract) (int, bool) {
	for i := range contracts {
		if bytes.Equal(contract, contracts[i].Contract) {
			return i, true
		}
	}
	return 0, false
}

func isZeroAddress(addrDesc bchain.AddressDescriptor) bool {
	for _, b := range addrDesc {
		if b != 0 {
			return false
		}
	}
	return true
}

const transferTo = int32(0)
const transferFrom = ^int32(0)
const internalTransferTo = int32(1)
const internalTransferFrom = ^int32(1)

// addToAddressesMapEthereumType maintains mapping between addresses and transactions in one block
// it ensures that each index is there only once, there can be for example multiple internal transactions of the same address
// the return value is true if the tx was processed before, to not to count the tx multiple times
func addToAddressesMapEthereumType(addresses addressesMap, strAddrDesc string, btxID []byte, index int32) bool {
	// check that the address was already processed in this block
	// if not found, it has certainly not been counted
	at, found := addresses[strAddrDesc]
	if found {
		// if the tx is already in the slice, append the index to the array of indexes
		for i, t := range at {
			if bytes.Equal(btxID, t.btxID) {
				for _, existing := range t.indexes {
					if existing == index {
						return true
					}
				}
				at[i].indexes = append(t.indexes, index)
				return true
			}
		}
	}
	addresses[strAddrDesc] = append(at, txIndexes{
		btxID:   btxID,
		indexes: []int32{index},
	})
	return false
}

func addToContract(c *AddrContract, contractIndex int, index int32, contract bchain.AddressDescriptor, transfer *bchain.TokenTransfer, addTxCount bool) int32 {
	var aggregate func(*big.Int, *big.Int)
	// index 0 is for ETH transfers, index 1 (InternalTxIndexOffset) is for internal transfers, contract indexes start with 2 (ContractIndexOffset)
	if index < 0 {
		index = ^int32(contractIndex + ContractIndexOffset)
		aggregate = func(s, v *big.Int) {
			s.Sub(s, v)
			if s.Sign() < 0 {
				// glog.Warningf("rocksdb: addToContracts: contract %s, from %s, negative aggregate", transfer.Contract, transfer.From)
				s.SetUint64(0)
			}
		}
	} else {
		index = int32(contractIndex + ContractIndexOffset)
		aggregate = func(s, v *big.Int) {
			s.Add(s, v)
		}
	}
	if transfer.Type == bchain.ERC20 {
		aggregate(&c.Value, &transfer.Value)
	} else if transfer.Type == bchain.ERC721 {
		if index < 0 {
			// remove token from the list
			for i := range c.Ids {
				if c.Ids[i].Cmp(&transfer.Value) == 0 {
					c.Ids = append(c.Ids[:i], c.Ids[i+1:]...)
					break
				}
			}
		} else {
			// add token to the list
			c.Ids = append(c.Ids, transfer.Value)
		}
	} else { // bchain.ERC1155
		for _, t := range transfer.IdValues {
			for i := range c.IdValues {
				// find the token in the list
				if c.IdValues[i].Id.Cmp(&t.Id) == 0 {
					aggregate(&c.IdValues[i].Value, &t.Value)
					// if transfer from, remove if the value is zero
					if index < 0 && len(c.IdValues[i].Value.Bits()) == 0 {
						c.IdValues = append(c.IdValues[:i], c.IdValues[i+1:]...)
					}
					goto nextTransfer
				}
			}
			// if not found and transfer to, add to the list
			// it is necessary to add a copy of the value so that subsequent calls to addToContract do not change the transfer value
			if index >= 0 {
				c.IdValues = append(c.IdValues, bchain.TokenTransferIdValue{
					Id:    t.Id,
					Value: *new(big.Int).Set(&t.Value),
				})
			}
		nextTransfer:
		}
	}
	if addTxCount {
		c.Txs++
	}
	return index
}

func (d *RocksDB) addToAddressesAndContractsEthereumType(addrDesc bchain.AddressDescriptor, btxID []byte, index int32, contract bchain.AddressDescriptor, transfer *bchain.TokenTransfer, addTxCount bool, addresses addressesMap, addressContracts map[string]*AddrContracts) error {
	var err error
	strAddrDesc := string(addrDesc)
	ac, e := addressContracts[strAddrDesc]
	if !e {
		ac, err = d.GetAddrDescContracts(addrDesc)
		if err != nil {
			return err
		}
		if ac == nil {
			ac = &AddrContracts{}
		}
		addressContracts[strAddrDesc] = ac
		d.cbs.balancesMiss++
	} else {
		d.cbs.balancesHit++
	}
	if contract == nil {
		if addTxCount {
			if index == internalTransferFrom || index == internalTransferTo {
				ac.InternalTxs++
			} else {
				ac.NonContractTxs++
			}
		}
	} else {
		// do not store contracts for 0x0000000000000000000000000000000000000000 address
		if !isZeroAddress(addrDesc) {
			// locate the contract and set i to the index in the array of contracts
			contractIndex, found := findContractInAddressContracts(contract, ac.Contracts)
			if !found {
				contractIndex = len(ac.Contracts)
				ac.Contracts = append(ac.Contracts, AddrContract{
					Contract: contract,
					Type:     transfer.Type,
				})
			}
			c := &ac.Contracts[contractIndex]
			index = addToContract(c, contractIndex, index, contract, transfer, addTxCount)
		} else {
			if index < 0 {
				index = transferFrom
			} else {
				index = transferTo
			}
		}
	}
	counted := addToAddressesMapEthereumType(addresses, strAddrDesc, btxID, index)
	if !counted {
		ac.TotalTxs++
	}
	return nil
}

type ethBlockTxContract struct {
	from, to, contract bchain.AddressDescriptor
	transferType       bchain.TokenTransferType
	value              big.Int
	idValues           []bchain.TokenTransferIdValue
}

type ethInternalTransfer struct {
	internalType bchain.EthereumInternalTransactionType
	from, to     bchain.AddressDescriptor
	value        big.Int
}

type ethInternalData struct {
	internalType bchain.EthereumInternalTransactionType
	contract     bchain.AddressDescriptor
	transfers    []ethInternalTransfer
	errorMsg     string
}

type ethBlockTx struct {
	btxID        []byte
	from, to     bchain.AddressDescriptor
	contracts    []ethBlockTxContract
	internalData *ethInternalData
}

func (d *RocksDB) processBaseTxData(blockTx *ethBlockTx, tx *bchain.Tx, addresses addressesMap, addressContracts map[string]*AddrContracts) error {
	var from, to bchain.AddressDescriptor
	var err error
	// there is only one output address in EthereumType transaction, store it in format txid 0
	if len(tx.Vout) == 1 && len(tx.Vout[0].ScriptPubKey.Addresses) == 1 {
		to, err = d.chainParser.GetAddrDescFromAddress(tx.Vout[0].ScriptPubKey.Addresses[0])
		if err != nil {
			// do not log ErrAddressMissing, transactions can be without to address (for example eth contracts)
			if err != bchain.ErrAddressMissing {
				glog.Warningf("rocksdb: processBaseTxData: %v, tx %v, output", err, tx.Txid)
			}
		} else {
			if err = d.addToAddressesAndContractsEthereumType(to, blockTx.btxID, transferTo, nil, nil, true, addresses, addressContracts); err != nil {
				return err
			}
			blockTx.to = to
		}
	}
	// there is only one input address in EthereumType transaction, store it in format txid ^0
	if len(tx.Vin) == 1 && len(tx.Vin[0].Addresses) == 1 {
		from, err = d.chainParser.GetAddrDescFromAddress(tx.Vin[0].Addresses[0])
		if err != nil {
			if err != bchain.ErrAddressMissing {
				glog.Warningf("rocksdb: processBaseTxData: %v, tx %v, input", err, tx.Txid)
			}
		} else {
			if err = d.addToAddressesAndContractsEthereumType(from, blockTx.btxID, transferFrom, nil, nil, !bytes.Equal(from, to), addresses, addressContracts); err != nil {
				return err
			}
			blockTx.from = from
		}
	}
	return nil
}

func (d *RocksDB) processInternalData(blockTx *ethBlockTx, tx *bchain.Tx, id *bchain.EthereumInternalData, addresses addressesMap, addressContracts map[string]*AddrContracts) error {
	blockTx.internalData = &ethInternalData{
		internalType: id.Type,
		errorMsg:     id.Error,
	}
	// index contract creation
	if id.Type == bchain.CREATE {
		to, err := d.chainParser.GetAddrDescFromAddress(id.Contract)
		if err != nil {
			if err != bchain.ErrAddressMissing {
				glog.Warningf("rocksdb: processInternalData: %v, tx %v, create contract", err, tx.Txid)
			}
			// set the internalType to CALL if incorrect contract so that it is not breaking the packing of data to DB
			blockTx.internalData.internalType = bchain.CALL
		} else {
			blockTx.internalData.contract = to
			if err = d.addToAddressesAndContractsEthereumType(to, blockTx.btxID, internalTransferTo, nil, nil, true, addresses, addressContracts); err != nil {
				return err
			}
		}
	}
	// index internal transfers
	if len(id.Transfers) > 0 {
		blockTx.internalData.transfers = make([]ethInternalTransfer, len(id.Transfers))
		for i := range id.Transfers {
			iti := &id.Transfers[i]
			ito := &blockTx.internalData.transfers[i]
			to, err := d.chainParser.GetAddrDescFromAddress(iti.To)
			if err != nil {
				// do not log ErrAddressMissing, transactions can be without to address (for example eth contracts)
				if err != bchain.ErrAddressMissing {
					glog.Warningf("rocksdb: processInternalData: %v, tx %v, internal transfer %d to", err, tx.Txid, i)
				}
			} else {
				if err = d.addToAddressesAndContractsEthereumType(to, blockTx.btxID, internalTransferTo, nil, nil, true, addresses, addressContracts); err != nil {
					return err
				}
				ito.to = to
			}
			from, err := d.chainParser.GetAddrDescFromAddress(iti.From)
			if err != nil {
				if err != bchain.ErrAddressMissing {
					glog.Warningf("rocksdb: processInternalData: %v, tx %v, internal transfer %d from", err, tx.Txid, i)
				}
			} else {
				if err = d.addToAddressesAndContractsEthereumType(from, blockTx.btxID, internalTransferFrom, nil, nil, !bytes.Equal(from, to), addresses, addressContracts); err != nil {
					return err
				}
				ito.from = from
			}
			ito.internalType = iti.Type
			ito.value = iti.Value
		}
	}
	return nil
}

func (d *RocksDB) processContractTransfers(blockTx *ethBlockTx, tx *bchain.Tx, addresses addressesMap, addressContracts map[string]*AddrContracts) error {
	tokenTransfers, err := d.chainParser.EthereumTypeGetTokenTransfersFromTx(tx)
	if err != nil {
		glog.Warningf("rocksdb: processContractTransfers %v, tx %v", err, tx.Txid)
	}
	blockTx.contracts = make([]ethBlockTxContract, len(tokenTransfers))
	for i, t := range tokenTransfers {
		var contract, from, to bchain.AddressDescriptor
		contract, err = d.chainParser.GetAddrDescFromAddress(t.Contract)
		if err == nil {
			from, err = d.chainParser.GetAddrDescFromAddress(t.From)
			if err == nil {
				to, err = d.chainParser.GetAddrDescFromAddress(t.To)
			}
		}
		if err != nil {
			glog.Warningf("rocksdb: processContractTransfers %v, tx %v, transfer %v", err, tx.Txid, t)
			continue
		}
		if err = d.addToAddressesAndContractsEthereumType(to, blockTx.btxID, int32(i), contract, t, true, addresses, addressContracts); err != nil {
			return err
		}
		eq := bytes.Equal(from, to)
		if err = d.addToAddressesAndContractsEthereumType(from, blockTx.btxID, ^int32(i), contract, t, !eq, addresses, addressContracts); err != nil {
			return err
		}
		bc := &blockTx.contracts[i]
		bc.transferType = t.Type
		bc.from = from
		bc.to = to
		bc.contract = contract
		bc.value = t.Value
		bc.idValues = t.IdValues
	}
	return nil
}

func (d *RocksDB) processAddressesEthereumType(block *bchain.Block, addresses addressesMap, addressContracts map[string]*AddrContracts) ([]ethBlockTx, error) {
	blockTxs := make([]ethBlockTx, len(block.Txs))
	for txi := range block.Txs {
		tx := &block.Txs[txi]
		btxID, err := d.chainParser.PackTxid(tx.Txid)
		if err != nil {
			return nil, err
		}
		blockTx := &blockTxs[txi]
		blockTx.btxID = btxID
		if err = d.processBaseTxData(blockTx, tx, addresses, addressContracts); err != nil {
			return nil, err
		}
		// process internal data
		eid, _ := tx.CoinSpecificData.(bchain.EthereumSpecificData)
		if eid.InternalData != nil {
			if err = d.processInternalData(blockTx, tx, eid.InternalData, addresses, addressContracts); err != nil {
				return nil, err
			}
		}
		// store contract transfers
		if err = d.processContractTransfers(blockTx, tx, addresses, addressContracts); err != nil {
			return nil, err
		}
	}
	return blockTxs, nil
}

var ethZeroAddress []byte = make([]byte, eth.EthereumTypeAddressDescriptorLen)

func appendAddress(buf []byte, a bchain.AddressDescriptor) []byte {
	if len(a) != eth.EthereumTypeAddressDescriptorLen {
		buf = append(buf, ethZeroAddress...)
	} else {
		buf = append(buf, a...)
	}
	return buf
}

func packEthInternalData(data *ethInternalData) []byte {
	// allocate enough for type+contract+all transfers with bigint value
	buf := make([]byte, 0, (2*len(data.transfers)+1)*(eth.EthereumTypeAddressDescriptorLen+16))
	varBuf := make([]byte, maxPackedBigintBytes)

	// internalType is one bit (CALL|CREATE), it is joined with count of internal transfers*2
	l := packVaruint(uint(data.internalType)&1+uint(len(data.transfers))<<1, varBuf)
	buf = append(buf, varBuf[:l]...)
	if data.internalType == bchain.CREATE {
		buf = appendAddress(buf, data.contract)
	}
	for i := range data.transfers {
		t := &data.transfers[i]
		buf = append(buf, byte(t.internalType))
		buf = appendAddress(buf, t.from)
		buf = appendAddress(buf, t.to)
		l = packBigint(&t.value, varBuf)
		buf = append(buf, varBuf[:l]...)
	}
	if len(data.errorMsg) > 0 {
		buf = append(buf, []byte(data.errorMsg)...)
	}
	return buf
}

func (d *RocksDB) unpackEthInternalData(buf []byte) (*bchain.EthereumInternalData, error) {
	id := bchain.EthereumInternalData{}
	v, l := unpackVaruint(buf)
	id.Type = bchain.EthereumInternalTransactionType(v & 1)
	id.Transfers = make([]bchain.EthereumInternalTransfer, v>>1)
	if id.Type == bchain.CREATE {
		addresses, _, _ := d.chainParser.GetAddressesFromAddrDesc(buf[l : l+eth.EthereumTypeAddressDescriptorLen])
		l += eth.EthereumTypeAddressDescriptorLen
		if len(addresses) > 0 {
			id.Contract = addresses[0]
		}
	}
	var ll int
	for i := range id.Transfers {
		t := &id.Transfers[i]
		t.Type = bchain.EthereumInternalTransactionType(buf[l])
		l++
		addresses, _, _ := d.chainParser.GetAddressesFromAddrDesc(buf[l : l+eth.EthereumTypeAddressDescriptorLen])
		l += eth.EthereumTypeAddressDescriptorLen
		if len(addresses) > 0 {
			t.From = addresses[0]
		}
		addresses, _, _ = d.chainParser.GetAddressesFromAddrDesc(buf[l : l+eth.EthereumTypeAddressDescriptorLen])
		l += eth.EthereumTypeAddressDescriptorLen
		if len(addresses) > 0 {
			t.To = addresses[0]
		}
		t.Value, ll = unpackBigint(buf[l:])
		l += ll
	}
	id.Error = eth.UnpackInternalTransactionError(buf[l:])
	return &id, nil
}

type FourByteSignature struct {
	Name       string
	Parameters []string
}

func packFourByteKey(fourBytes uint32, id uint32) []byte {
	key := make([]byte, 0, 8)
	key = append(key, packUint(fourBytes)...)
	key = append(key, packUint(id)...)
	return key
}

func packFourByteSignature(signature *FourByteSignature) []byte {
	buf := packString(signature.Name)
	for i := range signature.Parameters {
		buf = append(buf, packString(signature.Parameters[i])...)
	}
	return buf
}

func unpackFourByteSignature(buf []byte) (*FourByteSignature, error) {
	var signature FourByteSignature
	var l int
	signature.Name, l = unpackString(buf)
	for l < len(buf) {
		s, ll := unpackString(buf[l:])
		signature.Parameters = append(signature.Parameters, s)
		l += ll
	}
	return &signature, nil
}

func (d *RocksDB) GetFourByteSignature(fourBytes uint32, id uint32) (*FourByteSignature, error) {
	key := packFourByteKey(fourBytes, id)
	val, err := d.db.GetCF(d.ro, d.cfh[cfFunctionSignatures], key)
	if err != nil {
		return nil, err
	}
	defer val.Free()
	buf := val.Data()
	if len(buf) == 0 {
		return nil, nil
	}
	return unpackFourByteSignature(buf)
}

func (d *RocksDB) StoreFourByteSignature(wb *gorocksdb.WriteBatch, fourBytes uint32, id uint32, signature *FourByteSignature) error {
	key := packFourByteKey(fourBytes, id)
	wb.PutCF(d.cfh[cfFunctionSignatures], key, packFourByteSignature(signature))
	return nil
}

func (d *RocksDB) GetEthereumInternalData(txid string) (*bchain.EthereumInternalData, error) {
	btxID, err := d.chainParser.PackTxid(txid)
	if err != nil {
		return nil, err
	}
	return d.getEthereumInternalData(btxID)
}

func (d *RocksDB) getEthereumInternalData(btxID []byte) (*bchain.EthereumInternalData, error) {
	val, err := d.db.GetCF(d.ro, d.cfh[cfInternalData], btxID)
	if err != nil {
		return nil, err
	}
	defer val.Free()
	buf := val.Data()
	if len(buf) == 0 {
		return nil, nil
	}
	return d.unpackEthInternalData(buf)
}

func (d *RocksDB) storeInternalDataEthereumType(wb *gorocksdb.WriteBatch, blockTxs []ethBlockTx) error {
	for i := range blockTxs {
		blockTx := &blockTxs[i]
		if blockTx.internalData != nil {
			wb.PutCF(d.cfh[cfInternalData], blockTx.btxID, packEthInternalData(blockTx.internalData))
		}
	}
	return nil
}

func packBlockTx(buf []byte, blockTx *ethBlockTx) []byte {
	varBuf := make([]byte, maxPackedBigintBytes)
	buf = append(buf, blockTx.btxID...)
	buf = appendAddress(buf, blockTx.from)
	buf = appendAddress(buf, blockTx.to)
	// internal data are not stored in blockTx, they are fetched on disconnect directly from the cfInternalData column
	// contracts - store the number of address pairs
	l := packVaruint(uint(len(blockTx.contracts)), varBuf)
	buf = append(buf, varBuf[:l]...)
	for j := range blockTx.contracts {
		c := &blockTx.contracts[j]
		buf = appendAddress(buf, c.from)
		buf = appendAddress(buf, c.to)
		buf = appendAddress(buf, c.contract)
		l = packVaruint(uint(c.transferType), varBuf)
		buf = append(buf, varBuf[:l]...)
		if c.transferType == bchain.ERC1155 {
			l = packVaruint(uint(len(c.idValues)), varBuf)
			buf = append(buf, varBuf[:l]...)
			for i := range c.idValues {
				l = packBigint(&c.idValues[i].Id, varBuf)
				buf = append(buf, varBuf[:l]...)
				l = packBigint(&c.idValues[i].Value, varBuf)
				buf = append(buf, varBuf[:l]...)
			}
		} else { // ERC20, ERC721
			l = packBigint(&c.value, varBuf)
			buf = append(buf, varBuf[:l]...)
		}
	}
	return buf
}

func (d *RocksDB) storeAndCleanupBlockTxsEthereumType(wb *gorocksdb.WriteBatch, block *bchain.Block, blockTxs []ethBlockTx) error {
	pl := d.chainParser.PackedTxidLen()
	buf := make([]byte, 0, (pl+2*eth.EthereumTypeAddressDescriptorLen)*len(blockTxs))
	for i := range blockTxs {
		buf = packBlockTx(buf, &blockTxs[i])
	}
	key := packUint(block.Height)
	wb.PutCF(d.cfh[cfBlockTxs], key, buf)
	return d.cleanupBlockTxs(wb, block)
}

func (d *RocksDB) storeBlockInternalDataErrorEthereumType(wb *gorocksdb.WriteBatch, block *bchain.Block, message string) error {
	key := packUint(block.Height)
	txid, err := d.chainParser.PackTxid(block.Hash)
	if err != nil {
		return err
	}
	m := []byte(message)
	buf := make([]byte, 0, len(txid)+len(m)+1)
	// the stored structure is txid+retry count (1 byte)+error message
	buf = append(buf, txid...)
	buf = append(buf, 0)
	buf = append(buf, m...)
	wb.PutCF(d.cfh[cfBlockInternalDataErrors], key, buf)
	return nil
}

func (d *RocksDB) storeBlockSpecificDataEthereumType(wb *gorocksdb.WriteBatch, block *bchain.Block) error {
	blockSpecificData, _ := block.CoinSpecificData.(*bchain.EthereumBlockSpecificData)
	if blockSpecificData != nil && blockSpecificData.InternalDataError != "" {
		glog.Info("storeBlockSpecificDataEthereumType ", block.Height, ": ", blockSpecificData.InternalDataError)
		if err := d.storeBlockInternalDataErrorEthereumType(wb, block, blockSpecificData.InternalDataError); err != nil {
			return err
		}
	}
	return nil
}

// unpackBlockTx unpacks ethBlockTx from buf, starting at position pos
// the position is updated as the data is unpacked and returned to the caller
func unpackBlockTx(buf []byte, pos int) (*ethBlockTx, int, error) {
	getAddress := func(i int) (bchain.AddressDescriptor, int, error) {
		if len(buf)-i < eth.EthereumTypeAddressDescriptorLen {
			glog.Error("rocksdb: Inconsistent data in blockTxs ", hex.EncodeToString(buf))
			return nil, 0, errors.New("Inconsistent data in blockTxs")
		}
		a := append(bchain.AddressDescriptor(nil), buf[i:i+eth.EthereumTypeAddressDescriptorLen]...)
		return a, i + eth.EthereumTypeAddressDescriptorLen, nil
	}
	var from, to bchain.AddressDescriptor
	var err error
	if len(buf)-pos < eth.EthereumTypeTxidLen {
		glog.Error("rocksdb: Inconsistent data in blockTxs ", hex.EncodeToString(buf))
		return nil, 0, errors.New("Inconsistent data in blockTxs")
	}
	txid := append([]byte(nil), buf[pos:pos+eth.EthereumTypeTxidLen]...)
	pos += eth.EthereumTypeTxidLen
	from, pos, err = getAddress(pos)
	if err != nil {
		return nil, 0, err
	}
	to, pos, err = getAddress(pos)
	if err != nil {
		return nil, 0, err
	}
	// contracts
	cc, l := unpackVaruint(buf[pos:])
	pos += l
	contracts := make([]ethBlockTxContract, cc)
	for j := range contracts {
		c := &contracts[j]
		c.from, pos, err = getAddress(pos)
		if err != nil {
			return nil, 0, err
		}
		c.to, pos, err = getAddress(pos)
		if err != nil {
			return nil, 0, err
		}
		c.contract, pos, err = getAddress(pos)
		if err != nil {
			return nil, 0, err
		}
		cc, l = unpackVaruint(buf[pos:])
		c.transferType = bchain.TokenTransferType(cc)
		pos += l
		if c.transferType == bchain.ERC1155 {
			cc, l = unpackVaruint(buf[pos:])
			pos += l
			c.idValues = make([]bchain.TokenTransferIdValue, cc)
			for i := range c.idValues {
				c.idValues[i].Id, l = unpackBigint(buf[pos:])
				pos += l
				c.idValues[i].Value, l = unpackBigint(buf[pos:])
				pos += l
			}
		} else { // ERC20, ERC721
			c.value, l = unpackBigint(buf[pos:])
			pos += l
		}
	}
	return &ethBlockTx{
		btxID:     txid,
		from:      from,
		to:        to,
		contracts: contracts,
	}, pos, nil
}

func (d *RocksDB) getBlockTxsEthereumType(height uint32) ([]ethBlockTx, error) {
	val, err := d.db.GetCF(d.ro, d.cfh[cfBlockTxs], packUint(height))
	if err != nil {
		return nil, err
	}
	defer val.Free()
	buf := val.Data()
	// nil data means the key was not found in DB
	if buf == nil {
		return nil, nil
	}
	// buf can be empty slice, this means the block did not contain any transactions
	bt := make([]ethBlockTx, 0, 16)
	var btx *ethBlockTx
	for i := 0; i < len(buf); {
		btx, i, err = unpackBlockTx(buf, i)
		if err != nil {
			return nil, err
		}
		bt = append(bt, *btx)
	}
	return bt, nil
}

func (d *RocksDB) disconnectAddress(btxID []byte, internal bool, addrDesc bchain.AddressDescriptor, btxContract *ethBlockTxContract, addresses map[string]map[string]struct{}, contracts map[string]*AddrContracts) error {
	var err error
	// do not process empty address
	if len(addrDesc) == 0 {
		return nil
	}
	s := string(addrDesc)
	txid := string(btxID)
	// find if tx for this address was already encountered
	mtx, ftx := addresses[s]
	if !ftx {
		mtx = make(map[string]struct{})
		mtx[txid] = struct{}{}
		addresses[s] = mtx
	} else {
		_, ftx = mtx[txid]
		if !ftx {
			mtx[txid] = struct{}{}
		}
	}
	addrContracts, fc := contracts[s]
	if !fc {
		addrContracts, err = d.GetAddrDescContracts(addrDesc)
		if err != nil {
			return err
		}
		if addrContracts != nil {
			contracts[s] = addrContracts
		}
	}
	if addrContracts != nil {
		if !ftx {
			addrContracts.TotalTxs--
		}
		if btxContract == nil {
			if internal {
				if addrContracts.InternalTxs > 0 {
					addrContracts.InternalTxs--
				} else {
					glog.Warning("AddressContracts ", addrDesc, ", InternalTxs would be negative, tx ", hex.EncodeToString(btxID))
				}
			} else {
				if addrContracts.NonContractTxs > 0 {
					addrContracts.NonContractTxs--
				} else {
					glog.Warning("AddressContracts ", addrDesc, ", EthTxs would be negative, tx ", hex.EncodeToString(btxID))
				}
			}
		} else {
			contractIndex, found := findContractInAddressContracts(btxContract.contract, addrContracts.Contracts)
			if found {
				addrContract := &addrContracts.Contracts[contractIndex]
				if addrContract.Txs > 0 {
					addrContract.Txs--
					if addrContract.Txs == 0 {
						// no transactions, remove the contract
						addrContracts.Contracts = append(addrContracts.Contracts[:contractIndex], addrContracts.Contracts[contractIndex+1:]...)
					} else {
						// update the values of the contract, reverse the direction
						var index int32
						if bytes.Equal(addrDesc, btxContract.to) {
							index = transferFrom
						} else {
							index = transferTo
						}
						addToContract(addrContract, contractIndex, index, btxContract.contract, &bchain.TokenTransfer{
							Type:     btxContract.transferType,
							Value:    btxContract.value,
							IdValues: btxContract.idValues,
						}, false)
					}
				} else {
					glog.Warning("AddressContracts ", addrDesc, ", contract ", contractIndex, " Txs would be negative, tx ", hex.EncodeToString(btxID))
				}
			} else {
				glog.Warning("AddressContracts ", addrDesc, ", contract ", btxContract.contract, " not found, tx ", hex.EncodeToString(btxID))
			}
		}
	} else {
		if !isZeroAddress(addrDesc) {
			glog.Warning("AddressContracts ", addrDesc, " not found, tx ", hex.EncodeToString(btxID))
		}
	}
	return nil
}

func (d *RocksDB) disconnectInternalData(btxID []byte, addresses map[string]map[string]struct{}, contracts map[string]*AddrContracts) error {
	internalData, err := d.getEthereumInternalData(btxID)
	if err != nil {
		return err
	}
	if internalData != nil {
		if internalData.Type == bchain.CREATE {
			contract, err := d.chainParser.GetAddrDescFromAddress(internalData.Contract)
			if err != nil {
				return err
			}
			if err := d.disconnectAddress(btxID, true, contract, nil, addresses, contracts); err != nil {
				return err
			}
		}
		for j := range internalData.Transfers {
			t := &internalData.Transfers[j]
			var from, to bchain.AddressDescriptor
			from, err = d.chainParser.GetAddrDescFromAddress(t.From)
			if err == nil {
				to, err = d.chainParser.GetAddrDescFromAddress(t.To)
			}
			if err != nil {
				return err
			}
			if err := d.disconnectAddress(btxID, true, from, nil, addresses, contracts); err != nil {
				return err
			}
			// if from==to, tx is counted only once and does not have to be disconnected again
			if !bytes.Equal(from, to) {
				if err := d.disconnectAddress(btxID, true, to, nil, addresses, contracts); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (d *RocksDB) disconnectBlockTxsEthereumType(wb *gorocksdb.WriteBatch, height uint32, blockTxs []ethBlockTx, contracts map[string]*AddrContracts) error {
	glog.Info("Disconnecting block ", height, " containing ", len(blockTxs), " transactions")
	addresses := make(map[string]map[string]struct{})
	for i := range blockTxs {
		blockTx := &blockTxs[i]
		if err := d.disconnectAddress(blockTx.btxID, false, blockTx.from, nil, addresses, contracts); err != nil {
			return err
		}
		// if from==to, tx is counted only once and does not have to be disconnected again
		if !bytes.Equal(blockTx.from, blockTx.to) {
			if err := d.disconnectAddress(blockTx.btxID, false, blockTx.to, nil, addresses, contracts); err != nil {
				return err
			}
		}
		// internal data
		err := d.disconnectInternalData(blockTx.btxID, addresses, contracts)
		if err != nil {
			return err

		}
		// contracts
		for j := range blockTx.contracts {
			c := &blockTx.contracts[j]
			if err := d.disconnectAddress(blockTx.btxID, false, c.from, c, addresses, contracts); err != nil {
				return err
			}
			if err := d.disconnectAddress(blockTx.btxID, false, c.to, c, addresses, contracts); err != nil {
				return err
			}
		}
		wb.DeleteCF(d.cfh[cfTransactions], blockTx.btxID)
		wb.DeleteCF(d.cfh[cfInternalData], blockTx.btxID)
	}
	for a := range addresses {
		key := packAddressKey([]byte(a), height)
		wb.DeleteCF(d.cfh[cfAddresses], key)
	}
	return nil
}

// DisconnectBlockRangeEthereumType removes all data belonging to blocks in range lower-higher
// it is able to disconnect only blocks for which there are data in the blockTxs column
func (d *RocksDB) DisconnectBlockRangeEthereumType(lower uint32, higher uint32) error {
	blocks := make([][]ethBlockTx, higher-lower+1)
	for height := lower; height <= higher; height++ {
		blockTxs, err := d.getBlockTxsEthereumType(height)
		if err != nil {
			return err
		}
		// nil blockTxs means blockTxs were not found in db
		if blockTxs == nil {
			return errors.Errorf("Cannot disconnect blocks with height %v and lower. It is necessary to rebuild index.", height)
		}
		blocks[height-lower] = blockTxs
	}
	wb := gorocksdb.NewWriteBatch()
	defer wb.Destroy()
	contracts := make(map[string]*AddrContracts)
	for height := higher; height >= lower; height-- {
		if err := d.disconnectBlockTxsEthereumType(wb, height, blocks[height-lower], contracts); err != nil {
			return err
		}
		key := packUint(height)
		wb.DeleteCF(d.cfh[cfBlockTxs], key)
		wb.DeleteCF(d.cfh[cfHeight], key)
		wb.DeleteCF(d.cfh[cfBlockInternalDataErrors], key)
	}
	d.storeAddressContracts(wb, contracts)
	err := d.WriteBatch(wb)
	if err == nil {
		d.is.RemoveLastBlockTimes(int(higher-lower) + 1)
		glog.Infof("rocksdb: blocks %d-%d disconnected", lower, higher)
	}
	return err
}
