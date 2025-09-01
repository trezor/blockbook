package db

import (
	"bytes"
	"encoding/hex"
	"math/big"
	"os"
	"sort"
	"sync"
	"time"

	vlq "github.com/bsm/go-vlq"
	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/linxGnu/grocksdb"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
)

const InternalTxIndexOffset = 1
const ContractIndexOffset = 2

type AggregateFn = func(*big.Int, *big.Int)

type Ids []big.Int

func (s *Ids) sort() bool {
	sorted := false
	sort.Slice(*s, func(i, j int) bool {
		isLess := (*s)[i].CmpAbs(&(*s)[j]) == -1
		if isLess == (i > j) { // it is necessary to swap - (id[i]<id[j] and i>j) or (id[i]>id[j] and i<j)
			sorted = true
		}
		return isLess
	})
	return sorted
}

func (s *Ids) search(id big.Int) int {
	// attempt to find id using a binary search
	return sort.Search(len(*s), func(i int) bool {
		return (*s)[i].CmpAbs(&id) >= 0
	})
}

// insert id in ascending order
func (s *Ids) insert(id big.Int) {
	i := s.search(id)
	if i == len(*s) {
		*s = append(*s, id)
	} else {
		*s = append((*s)[:i+1], (*s)[i:]...)
		(*s)[i] = id
	}
}

func (s *Ids) remove(id big.Int) {
	i := s.search(id)
	// remove id if found
	if i < len(*s) && (*s)[i].CmpAbs(&id) == 0 {
		*s = append((*s)[:i], (*s)[i+1:]...)
	}
}

type MultiTokenValues []bchain.MultiTokenValue

func (s *MultiTokenValues) sort() bool {
	sorted := false
	sort.Slice(*s, func(i, j int) bool {
		isLess := (*s)[i].Id.CmpAbs(&(*s)[j].Id) == -1
		if isLess == (i > j) { // it is necessary to swap - (id[i]<id[j] and i>j) or (id[i]>id[j] and i<j)
			sorted = true
		}
		return isLess
	})
	return sorted
}

// search for multi token value using a binary seach on id
func (s *MultiTokenValues) search(m bchain.MultiTokenValue) int {
	return sort.Search(len(*s), func(i int) bool {
		return (*s)[i].Id.CmpAbs(&m.Id) >= 0
	})
}

func (s *MultiTokenValues) upsert(m bchain.MultiTokenValue, index int32, aggregate AggregateFn) {
	i := s.search(m)
	if i < len(*s) && (*s)[i].Id.CmpAbs(&m.Id) == 0 {
		aggregate(&(*s)[i].Value, &m.Value)
		// if transfer from, remove if the value is zero
		if index < 0 && len((*s)[i].Value.Bits()) == 0 {
			*s = append((*s)[:i], (*s)[i+1:]...)
		}
		return
	}
	if index >= 0 {
		elem := bchain.MultiTokenValue{
			Id:    m.Id,
			Value: *new(big.Int).Set(&m.Value),
		}
		if i == len(*s) {
			*s = append(*s, elem)
		} else {
			*s = append((*s)[:i+1], (*s)[i:]...)
			(*s)[i] = elem
		}
	}
}

// AddrContract is Contract address with number of transactions done by given address
type AddrContract struct {
	Standard         bchain.TokenStandard
	Contract         bchain.AddressDescriptor
	Txs              uint
	Value            big.Int          // single value of ERC20
	Ids              Ids              // multiple ERC721 tokens
	MultiTokenValues MultiTokenValues // multiple ERC1155 tokens
}

// AddrContracts contains number of transactions and contracts for an address
type AddrContracts struct {
	TotalTxs       uint
	NonContractTxs uint
	InternalTxs    uint
	Contracts      []AddrContract
}

// packAddrContracts packs AddrContracts into a byte buffer
func packAddrContractsV6(acs *AddrContracts) []byte {
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
		l = packVaruint(uint(ac.Standard)+ac.Txs<<2, varBuf)
		buf = append(buf, varBuf[:l]...)
		if ac.Standard == bchain.FungibleToken {
			l = packBigint(&ac.Value, varBuf)
			buf = append(buf, varBuf[:l]...)
		} else if ac.Standard == bchain.NonFungibleToken {
			l = packVaruint(uint(len(ac.Ids)), varBuf)
			buf = append(buf, varBuf[:l]...)
			for i := range ac.Ids {
				l = packBigint(&ac.Ids[i], varBuf)
				buf = append(buf, varBuf[:l]...)
			}
		} else { // bchain.ERC1155
			l = packVaruint(uint(len(ac.MultiTokenValues)), varBuf)
			buf = append(buf, varBuf[:l]...)
			for i := range ac.MultiTokenValues {
				l = packBigint(&ac.MultiTokenValues[i].Id, varBuf)
				buf = append(buf, varBuf[:l]...)
				l = packBigint(&ac.MultiTokenValues[i].Value, varBuf)
				buf = append(buf, varBuf[:l]...)
			}
		}
	}
	return buf
}

// packAddrContracts packs AddrContracts into a byte buffer
func packAddrContracts(acs *AddrContracts) []byte {
	buf := make([]byte, 0, 8+len(acs.Contracts)*(eth.EthereumTypeAddressDescriptorLen+12))
	varBuf := make([]byte, maxPackedBigintBytes)
	l := packVaruint(acs.TotalTxs, varBuf)
	buf = append(buf, varBuf[:l]...)
	l = packVaruint(acs.NonContractTxs, varBuf)
	buf = append(buf, varBuf[:l]...)
	l = packVaruint(acs.InternalTxs, varBuf)
	buf = append(buf, varBuf[:l]...)
	l = packVaruint(uint(len(acs.Contracts)), varBuf)
	buf = append(buf, varBuf[:l]...)
	for _, ac := range acs.Contracts {
		buf = append(buf, ac.Contract...)
		l = packVaruint(uint(ac.Standard)+ac.Txs<<2, varBuf)
		buf = append(buf, varBuf[:l]...)
		if ac.Standard == bchain.FungibleToken {
			l = packBigint(&ac.Value, varBuf)
			buf = append(buf, varBuf[:l]...)
		} else if ac.Standard == bchain.NonFungibleToken {
			l = packVaruint(uint(len(ac.Ids)), varBuf)
			buf = append(buf, varBuf[:l]...)
			for i := range ac.Ids {
				l = packBigint(&ac.Ids[i], varBuf)
				buf = append(buf, varBuf[:l]...)
			}
		} else { // bchain.ERC1155
			l = packVaruint(uint(len(ac.MultiTokenValues)), varBuf)
			buf = append(buf, varBuf[:l]...)
			for i := range ac.MultiTokenValues {
				l = packBigint(&ac.MultiTokenValues[i].Id, varBuf)
				buf = append(buf, varBuf[:l]...)
				l = packBigint(&ac.MultiTokenValues[i].Value, varBuf)
				buf = append(buf, varBuf[:l]...)
			}
		}
	}
	return buf
}

func unpackAddrContractsV6(buf []byte, addrDesc bchain.AddressDescriptor) (acs *AddrContracts, err error) {
	tt, l := unpackVaruint(buf)
	buf = buf[l:]
	nct, l := unpackVaruint(buf)
	buf = buf[l:]
	ict, l := unpackVaruint(buf)
	buf = buf[l:]
	c := make([]AddrContract, 0, len(buf)/30+4)
	for len(buf) > 0 {
		if len(buf) < eth.EthereumTypeAddressDescriptorLen {
			return nil, errors.New("Invalid data stored in cfAddressContracts for AddrDesc " + addrDesc.String())
		}
		contract := append(bchain.AddressDescriptor(nil), buf[:eth.EthereumTypeAddressDescriptorLen]...)
		txs, l := unpackVaruint(buf[eth.EthereumTypeAddressDescriptorLen:])
		buf = buf[eth.EthereumTypeAddressDescriptorLen+l:]
		standard := bchain.TokenStandard(txs & 3)
		txs >>= 2
		ac := AddrContract{
			Standard: standard,
			Contract: contract,
			Txs:      txs,
		}
		if standard == bchain.FungibleToken {
			b, ll := unpackBigint(buf)
			buf = buf[ll:]
			ac.Value = b
		} else {
			len, ll := unpackVaruint(buf)
			buf = buf[ll:]
			if standard == bchain.NonFungibleToken {
				ac.Ids = make(Ids, len)
				for i := uint(0); i < len; i++ {
					b, ll := unpackBigint(buf)
					buf = buf[ll:]
					ac.Ids[i] = b
				}
			} else {
				ac.MultiTokenValues = make(MultiTokenValues, len)
				for i := uint(0); i < len; i++ {
					b, ll := unpackBigint(buf)
					buf = buf[ll:]
					ac.MultiTokenValues[i].Id = b
					b, ll = unpackBigint(buf)
					buf = buf[ll:]
					ac.MultiTokenValues[i].Value = b
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

func unpackAddrContracts(buf []byte, addrDesc bchain.AddressDescriptor) (acs *AddrContracts, err error) {
	tt, l := unpackVaruint(buf)
	buf = buf[l:]
	nct, l := unpackVaruint(buf)
	buf = buf[l:]
	ict, l := unpackVaruint(buf)
	buf = buf[l:]
	cl, l := unpackVaruint(buf)
	buf = buf[l:]
	c := make([]AddrContract, 0, cl)
	for len(buf) > 0 {
		if len(buf) < eth.EthereumTypeAddressDescriptorLen {
			return nil, errors.New("Invalid data stored in cfAddressContracts for AddrDesc " + addrDesc.String())
		}
		contract := append(bchain.AddressDescriptor(nil), buf[:eth.EthereumTypeAddressDescriptorLen]...)
		txs, l := unpackVaruint(buf[eth.EthereumTypeAddressDescriptorLen:])
		buf = buf[eth.EthereumTypeAddressDescriptorLen+l:]
		standard := bchain.TokenStandard(txs & 3)
		txs >>= 2
		ac := AddrContract{
			Standard: standard,
			Contract: contract,
			Txs:      txs,
		}
		if standard == bchain.FungibleToken {
			b, ll := unpackBigint(buf)
			buf = buf[ll:]
			ac.Value = b
		} else {
			len, ll := unpackVaruint(buf)
			buf = buf[ll:]
			if standard == bchain.NonFungibleToken {
				ac.Ids = make(Ids, len)
				for i := uint(0); i < len; i++ {
					b, ll := unpackBigint(buf)
					buf = buf[ll:]
					ac.Ids[i] = b
				}
			} else {
				ac.MultiTokenValues = make(MultiTokenValues, len)
				for i := uint(0); i < len; i++ {
					b, ll := unpackBigint(buf)
					buf = buf[ll:]
					ac.MultiTokenValues[i].Id = b
					b, ll = unpackBigint(buf)
					buf = buf[ll:]
					ac.MultiTokenValues[i].Value = b
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

func (d *RocksDB) storeAddressContracts(wb *grocksdb.WriteBatch, acm map[string]*AddrContracts) error {
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

func findContractInAddressContracts(contract bchain.AddressDescriptor, contracts []unpackedAddrContract) (int, bool) {
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

func addToContract(c *unpackedAddrContract, contractIndex int, index int32, contract bchain.AddressDescriptor, transfer *bchain.TokenTransfer, addTxCount bool) int32 {
	var aggregate AggregateFn
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
	if transfer.Standard == bchain.FungibleToken {
		aggregate(c.Value.get(), &transfer.Value)
	} else if transfer.Standard == bchain.NonFungibleToken {
		if index < 0 {
			c.Ids.remove(transfer.Value)
		} else {
			c.Ids.insert(transfer.Value)
		}
	} else { // bchain.ERC1155
		for _, t := range transfer.MultiTokenValues {
			c.MultiTokenValues.upsert(t, index, aggregate)
		}
	}
	if addTxCount {
		c.Txs++
	}
	return index
}

func (d *RocksDB) addToAddressesAndContractsEthereumType(addrDesc bchain.AddressDescriptor, btxID []byte, index int32, contract bchain.AddressDescriptor, transfer *bchain.TokenTransfer, addTxCount bool, addresses addressesMap, addressContracts map[string]*unpackedAddrContracts) error {
	var err error
	strAddrDesc := string(addrDesc)
	ac, e := addressContracts[strAddrDesc]
	if !e {
		ac, err = d.getUnpackedAddrDescContracts(addrDesc)
		if err != nil {
			return err
		}
		if ac == nil {
			ac = &unpackedAddrContracts{}
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
				ac.Contracts = append(ac.Contracts, unpackedAddrContract{
					Contract: contract,
					Standard: transfer.Standard,
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
	transferStandard   bchain.TokenStandard
	value              big.Int
	idValues           []bchain.MultiTokenValue
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

func (d *RocksDB) processBaseTxData(blockTx *ethBlockTx, tx *bchain.Tx, addresses addressesMap, addressContracts map[string]*unpackedAddrContracts) error {
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

func (d *RocksDB) setAddressTxIndexesToAddressMap(addrDesc bchain.AddressDescriptor, height uint32, addresses addressesMap) error {
	strAddrDesc := string(addrDesc)
	_, found := addresses[strAddrDesc]
	if !found {
		txIndexes, err := d.getTxIndexesForAddressAndBlock(addrDesc, height)
		if err != nil {
			return err
		}
		if len(txIndexes) > 0 {
			addresses[strAddrDesc] = txIndexes
		}
	}
	return nil
}

// existingBlock signals that internal data are reconnected to already indexed block after they failed during standard sync
func (d *RocksDB) processInternalData(blockTx *ethBlockTx, tx *bchain.Tx, id *bchain.EthereumInternalData, addresses addressesMap, addressContracts map[string]*unpackedAddrContracts, existingBlock bool) error {
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
			if existingBlock {
				if err = d.setAddressTxIndexesToAddressMap(to, tx.BlockHeight, addresses); err != nil {
					return err
				}
			}
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
				if existingBlock {
					if err = d.setAddressTxIndexesToAddressMap(to, tx.BlockHeight, addresses); err != nil {
						return err
					}
				}
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
				if existingBlock {
					if err = d.setAddressTxIndexesToAddressMap(from, tx.BlockHeight, addresses); err != nil {
						return err
					}
				}
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

func (d *RocksDB) processContractTransfers(blockTx *ethBlockTx, tx *bchain.Tx, addresses addressesMap, addressContracts map[string]*unpackedAddrContracts) error {
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
		bc.transferStandard = t.Standard
		bc.from = from
		bc.to = to
		bc.contract = contract
		bc.value = t.Value
		bc.idValues = t.MultiTokenValues
	}
	return nil
}

func (d *RocksDB) processAddressesEthereumType(block *bchain.Block, addresses addressesMap, addressContracts map[string]*unpackedAddrContracts) ([]ethBlockTx, error) {
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
			if err = d.processInternalData(blockTx, tx, eid.InternalData, addresses, addressContracts, false); err != nil {
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

// ReconnectInternalDataToBlockEthereumType adds missing internal data to the block and stores them in db
func (d *RocksDB) ReconnectInternalDataToBlockEthereumType(block *bchain.Block) error {
	d.connectBlockMux.Lock()
	defer d.connectBlockMux.Unlock()

	wb := grocksdb.NewWriteBatch()
	defer wb.Destroy()
	if d.chainParser.GetChainType() != bchain.ChainEthereumType {
		return errors.New("Unsupported chain type")
	}

	addresses := make(addressesMap)
	addressContracts := make(map[string]*unpackedAddrContracts)

	// process internal data
	blockTxs := make([]ethBlockTx, len(block.Txs))
	for txi := range block.Txs {
		tx := &block.Txs[txi]
		eid, _ := tx.CoinSpecificData.(bchain.EthereumSpecificData)
		if eid.InternalData != nil {
			btxID, err := d.chainParser.PackTxid(tx.Txid)
			if err != nil {
				return err
			}
			blockTx := &blockTxs[txi]
			blockTx.btxID = btxID
			tx.BlockHeight = block.Height
			if err = d.processInternalData(blockTx, tx, eid.InternalData, addresses, addressContracts, true); err != nil {
				return err
			}
		}
	}

	if err := d.storeUnpackedAddressContracts(wb, addressContracts); err != nil {
		return err
	}
	if err := d.storeInternalDataEthereumType(wb, blockTxs); err != nil {
		return err
	}
	if err := d.storeAddresses(wb, block.Height, addresses); err != nil {
		return err
	}
	// remove the block from the internal errors table
	wb.DeleteCF(d.cfh[cfBlockInternalDataErrors], packUint(block.Height))
	if err := d.WriteBatch(wb); err != nil {
		return err
	}
	return nil
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

// FourByteSignature contains 4byte signature of transaction value with parameters
// and parsed parameters (that are not stored in DB)
func packFourByteKey(fourBytes uint32, id uint32) []byte {
	key := make([]byte, 0, 8)
	key = append(key, packUint(fourBytes)...)
	key = append(key, packUint(id)...)
	return key
}

func packFourByteSignature(signature *bchain.FourByteSignature) []byte {
	buf := packString(signature.Name)
	for i := range signature.Parameters {
		buf = append(buf, packString(signature.Parameters[i])...)
	}
	return buf
}

func unpackFourByteSignature(buf []byte) (*bchain.FourByteSignature, error) {
	var signature bchain.FourByteSignature
	var l int
	signature.Name, l = unpackString(buf)
	for l < len(buf) {
		s, ll := unpackString(buf[l:])
		signature.Parameters = append(signature.Parameters, s)
		l += ll
	}
	return &signature, nil
}

// GetFourByteSignature gets all 4byte signature of given fourBytes and id
func (d *RocksDB) GetFourByteSignature(fourBytes uint32, id uint32) (*bchain.FourByteSignature, error) {
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

var cachedByteSignatures = make(map[uint32]*[]bchain.FourByteSignature)
var cachedByteSignaturesMux sync.Mutex

// GetFourByteSignatures gets all 4byte signatures of given fourBytes
// (there may be more than one signature starting with the same four bytes)
func (d *RocksDB) GetFourByteSignatures(fourBytes uint32) (*[]bchain.FourByteSignature, error) {
	cachedByteSignaturesMux.Lock()
	signatures, found := cachedByteSignatures[fourBytes]
	cachedByteSignaturesMux.Unlock()
	if !found {
		retval := []bchain.FourByteSignature{}
		key := packUint(fourBytes)
		it := d.db.NewIteratorCF(d.ro, d.cfh[cfFunctionSignatures])
		defer it.Close()
		for it.Seek(key); it.Valid(); it.Next() {
			current := it.Key().Data()
			if bytes.Compare(current[:4], key) > 0 {
				break
			}
			val := it.Value().Data()
			signature, err := unpackFourByteSignature(val)
			if err != nil {
				return nil, err
			}
			retval = append(retval, *signature)
		}
		cachedByteSignaturesMux.Lock()
		cachedByteSignatures[fourBytes] = &retval
		cachedByteSignaturesMux.Unlock()
		return &retval, nil
	}
	return signatures, nil
}

// StoreFourByteSignature stores 4byte signature in DB
func (d *RocksDB) StoreFourByteSignature(wb *grocksdb.WriteBatch, fourBytes uint32, id uint32, signature *bchain.FourByteSignature) error {
	key := packFourByteKey(fourBytes, id)
	wb.PutCF(d.cfh[cfFunctionSignatures], key, packFourByteSignature(signature))
	cachedByteSignaturesMux.Lock()
	delete(cachedByteSignatures, fourBytes)
	cachedByteSignaturesMux.Unlock()
	return nil
}

// GetEthereumInternalData gets transaction internal data from DB
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

func (d *RocksDB) storeInternalDataEthereumType(wb *grocksdb.WriteBatch, blockTxs []ethBlockTx) error {
	for i := range blockTxs {
		blockTx := &blockTxs[i]
		if blockTx.internalData != nil {
			wb.PutCF(d.cfh[cfInternalData], blockTx.btxID, packEthInternalData(blockTx.internalData))
		}
	}
	return nil
}

var cachedContracts = make(map[string]*bchain.ContractInfo)
var cachedContractsMux sync.Mutex

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
	cachedContractsMux.Lock()
	contractInfo, found := cachedContracts[cacheKey]
	cachedContractsMux.Unlock()
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
		cachedContractsMux.Lock()
		cachedContracts[cacheKey] = contractInfo
		cachedContractsMux.Unlock()
	}
	return contractInfo, nil
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
		cachedContractsMux.Lock()
		delete(cachedContracts, cacheKey)
		cachedContractsMux.Unlock()
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
		l = packVaruint(uint(c.transferStandard), varBuf)
		buf = append(buf, varBuf[:l]...)
		if c.transferStandard == bchain.MultiToken {
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

func (d *RocksDB) storeAndCleanupBlockTxsEthereumType(wb *grocksdb.WriteBatch, block *bchain.Block, blockTxs []ethBlockTx) error {
	pl := d.chainParser.PackedTxidLen()
	buf := make([]byte, 0, (pl+2*eth.EthereumTypeAddressDescriptorLen)*len(blockTxs))
	for i := range blockTxs {
		buf = packBlockTx(buf, &blockTxs[i])
	}
	key := packUint(block.Height)
	wb.PutCF(d.cfh[cfBlockTxs], key, buf)
	return d.cleanupBlockTxs(wb, block)
}

func (d *RocksDB) StoreBlockInternalDataErrorEthereumType(wb *grocksdb.WriteBatch, block *bchain.Block, message string, retryCount uint8) error {
	key := packUint(block.Height)
	// TODO: this supposes that Txid and block hash are the same size
	txid, err := d.chainParser.PackTxid(block.Hash)
	if err != nil {
		return err
	}
	m := []byte(message)
	buf := make([]byte, 0, len(txid)+len(m)+1)
	// the stored structure is txid+retry count (1 byte)+error message
	buf = append(buf, txid...)
	buf = append(buf, retryCount)
	buf = append(buf, m...)
	wb.PutCF(d.cfh[cfBlockInternalDataErrors], key, buf)
	return nil
}

type BlockInternalDataError struct {
	Height       uint32
	Hash         string
	Retries      uint8
	ErrorMessage string
}

func (d *RocksDB) unpackBlockInternalDataError(val []byte) (string, uint8, string, error) {
	txidUnpackedLen := d.chainParser.PackedTxidLen()
	var hash, message string
	var retries uint8
	var err error
	if len(val) > txidUnpackedLen+1 {
		hash, err = d.chainParser.UnpackTxid(val[:txidUnpackedLen])
		if err != nil {
			return "", 0, "", err
		}
		val = val[txidUnpackedLen:]
		retries = val[0]
		message = string(val[1:])
	}
	return hash, retries, message, nil
}

func (d *RocksDB) GetBlockInternalDataErrorsEthereumType() ([]BlockInternalDataError, error) {
	retval := []BlockInternalDataError{}
	if d.chainParser.GetChainType() == bchain.ChainEthereumType {
		it := d.db.NewIteratorCF(d.ro, d.cfh[cfBlockInternalDataErrors])
		defer it.Close()
		for it.SeekToFirst(); it.Valid(); it.Next() {
			height := unpackUint(it.Key().Data())
			val := it.Value().Data()
			hash, retires, message, err := d.unpackBlockInternalDataError(val)
			if err != nil {
				glog.Error("GetBlockInternalDataErrorsEthereumType height ", height, ", unpack error ", err)
				continue
			}
			retval = append(retval, BlockInternalDataError{
				Height:       height,
				Hash:         hash,
				Retries:      retires,
				ErrorMessage: message,
			})
		}
	}
	return retval, nil
}

func (d *RocksDB) storeBlockSpecificDataEthereumType(wb *grocksdb.WriteBatch, block *bchain.Block) error {
	blockSpecificData, _ := block.CoinSpecificData.(*bchain.EthereumBlockSpecificData)
	if blockSpecificData != nil {
		if blockSpecificData.InternalDataError != "" {
			glog.Info("storeBlockSpecificDataEthereumType ", block.Height, ": ", blockSpecificData.InternalDataError)
			if err := d.StoreBlockInternalDataErrorEthereumType(wb, block, blockSpecificData.InternalDataError, 0); err != nil {
				return err
			}
		}
		if len(blockSpecificData.AddressAliasRecords) > 0 {
			if err := d.storeAddressAliasRecords(wb, blockSpecificData.AddressAliasRecords); err != nil {
				return err
			}
		}
		for i := range blockSpecificData.Contracts {
			if err := d.storeContractInfo(wb, &blockSpecificData.Contracts[i]); err != nil {
				return err
			}
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
		c.transferStandard = bchain.TokenStandard(cc)
		pos += l
		if c.transferStandard == bchain.MultiToken {
			cc, l = unpackVaruint(buf[pos:])
			pos += l
			c.idValues = make([]bchain.MultiTokenValue, cc)
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

func (d *RocksDB) disconnectAddress(btxID []byte, internal bool, addrDesc bchain.AddressDescriptor, btxContract *ethBlockTxContract, addresses map[string]map[string]struct{}, contracts map[string]*unpackedAddrContracts) error {
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
		addrContracts, err = d.getUnpackedAddrDescContracts(addrDesc)
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
							Standard:         btxContract.transferStandard,
							Value:            btxContract.value,
							MultiTokenValues: btxContract.idValues,
						}, false)
					}
				} else {
					glog.Warning("AddressContracts ", addrDesc, ", contract ", contractIndex, " Txs would be negative, tx ", hex.EncodeToString(btxID))
				}
			} else {
				if !isZeroAddress(addrDesc) {
					glog.Warning("AddressContracts ", addrDesc, ", contract ", btxContract.contract, " not found, tx ", hex.EncodeToString(btxID))
				}
			}
		}
	} else {
		if !isZeroAddress(addrDesc) {
			glog.Warning("AddressContracts ", addrDesc, " not found, tx ", hex.EncodeToString(btxID))
		}
	}
	return nil
}

func (d *RocksDB) disconnectInternalData(btxID []byte, addresses map[string]map[string]struct{}, contracts map[string]*unpackedAddrContracts) error {
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

func (d *RocksDB) disconnectBlockTxsEthereumType(wb *grocksdb.WriteBatch, height uint32, blockTxs []ethBlockTx, contracts map[string]*unpackedAddrContracts) error {
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
			if !bytes.Equal(c.from, c.to) {
				if err := d.disconnectAddress(blockTx.btxID, false, c.to, c, addresses, contracts); err != nil {
					return err
				}
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
	wb := grocksdb.NewWriteBatch()
	defer wb.Destroy()
	contracts := make(map[string]*unpackedAddrContracts)
	for height := higher; height >= lower; height-- {
		if err := d.disconnectBlockTxsEthereumType(wb, height, blocks[height-lower], contracts); err != nil {
			return err
		}
		key := packUint(height)
		wb.DeleteCF(d.cfh[cfBlockTxs], key)
		wb.DeleteCF(d.cfh[cfHeight], key)
		wb.DeleteCF(d.cfh[cfBlockInternalDataErrors], key)
	}
	d.storeUnpackedAddressContracts(wb, contracts)
	err := d.WriteBatch(wb)
	if err == nil {
		d.is.RemoveLastBlockTimes(int(higher-lower) + 1)
		glog.Infof("rocksdb: blocks %d-%d disconnected", lower, higher)
	}
	return err
}

func (d *RocksDB) SortAddressContracts(stop chan os.Signal) error {
	if d.chainParser.GetChainType() != bchain.ChainEthereumType {
		glog.Info("SortAddressContracts: applicable only for ethereum type coins")
		return nil
	}
	glog.Info("SortAddressContracts: starting")
	// do not use cache
	ro := grocksdb.NewDefaultReadOptions()
	ro.SetFillCache(false)
	it := d.db.NewIteratorCF(ro, d.cfh[cfAddressContracts])
	defer it.Close()
	var rowCount, idsSortedCount, multiTokenValuesSortedCount int
	for it.SeekToFirst(); it.Valid(); it.Next() {
		select {
		case <-stop:
			return errors.New("SortAddressContracts: interrupted")
		default:
		}
		rowCount++
		addrDesc := it.Key().Data()
		buf := it.Value().Data()
		if len(buf) > 0 {
			ca, err := unpackAddrContracts(buf, addrDesc)
			if err != nil {
				glog.Error("failed to unpack AddrContracts for: ", hex.EncodeToString(addrDesc))
			}
			update := false
			for i := range ca.Contracts {
				c := &ca.Contracts[i]
				if sorted := c.Ids.sort(); sorted {
					idsSortedCount++
					update = true
				}
				if sorted := c.MultiTokenValues.sort(); sorted {
					multiTokenValuesSortedCount++
					update = true
				}
			}
			if update {
				if err := func() error {
					wb := grocksdb.NewWriteBatch()
					defer wb.Destroy()
					buf := packAddrContracts(ca)
					wb.PutCF(d.cfh[cfAddressContracts], addrDesc, buf)
					return d.WriteBatch(wb)
				}(); err != nil {
					return errors.Errorf("failed to write cfAddressContracts for: %v: %v", addrDesc, err)
				}
			}
		}
		if rowCount%5000000 == 0 {
			glog.Infof("SortAddressContracts: progress - scanned %d rows, sorted %d ids and %d multi token values", rowCount, idsSortedCount, multiTokenValuesSortedCount)
		}
	}
	glog.Infof("SortAddressContracts: finished - scanned %d rows, sorted %d ids and %d multi token value", rowCount, idsSortedCount, multiTokenValuesSortedCount)
	return nil
}

type unpackedBigInt struct {
	Slice []byte
	Value *big.Int
}
type unpackedIds []unpackedBigInt

type unpackedAddrContract struct {
	Standard         bchain.TokenStandard
	Contract         bchain.AddressDescriptor
	Txs              uint
	Value            unpackedBigInt           // single value of ERC20
	Ids              unpackedIds              // multiple ERC721 tokens
	MultiTokenValues unpackedMultiTokenValues // multiple ERC1155 tokens
}

func (b *unpackedBigInt) get() *big.Int {
	if b.Value == nil {
		if len(b.Slice) == 0 {
			b.Value = big.NewInt(0)
		} else {
			bi, _ := unpackBigint(b.Slice)
			b.Value = &bi
		}
	}
	return b.Value
}

type unpackedAddrContracts struct {
	Packed         []byte
	TotalTxs       uint
	NonContractTxs uint
	InternalTxs    uint
	Contracts      []unpackedAddrContract
}

func (s *unpackedIds) search(id big.Int) int {
	// attempt to find id using a binary search
	return sort.Search(len(*s), func(i int) bool {
		return (*s)[i].get().CmpAbs(&id) >= 0
	})
}

// insert id in ascending order
func (s *unpackedIds) insert(id big.Int) {
	i := s.search(id)
	if i == len(*s) {
		*s = append(*s, unpackedBigInt{Value: &id})
	} else {
		*s = append((*s)[:i+1], (*s)[i:]...)
		(*s)[i] = unpackedBigInt{Value: &id}
	}
}

func (s *unpackedIds) remove(id big.Int) {
	i := s.search(id)
	// remove id if found
	if i < len(*s) && (*s)[i].get().CmpAbs(&id) == 0 {
		*s = append((*s)[:i], (*s)[i+1:]...)
	}
}

type unpackedMultiTokenValue struct {
	Id    unpackedBigInt
	Value unpackedBigInt
}

type unpackedMultiTokenValues []unpackedMultiTokenValue

// search for multi token value using a binary seach on id
func (s *unpackedMultiTokenValues) search(m bchain.MultiTokenValue) int {
	return sort.Search(len(*s), func(i int) bool {
		return (*s)[i].Id.get().CmpAbs(&m.Id) >= 0
	})
}

func (s *unpackedMultiTokenValues) upsert(m bchain.MultiTokenValue, index int32, aggregate AggregateFn) {
	i := s.search(m)
	if i < len(*s) && (*s)[i].Id.get().CmpAbs(&m.Id) == 0 {
		aggregate((*s)[i].Value.get(), &m.Value)
		// if transfer from, remove if the value is zero
		if index < 0 && len((*s)[i].Value.get().Bits()) == 0 {
			*s = append((*s)[:i], (*s)[i+1:]...)
		}
		return
	}
	if index >= 0 {
		elem := unpackedMultiTokenValue{
			Id:    unpackedBigInt{Value: &m.Id},
			Value: unpackedBigInt{Value: new(big.Int).Set(&m.Value)},
		}
		if i == len(*s) {
			*s = append(*s, elem)
		} else {
			*s = append((*s)[:i+1], (*s)[i:]...)
			(*s)[i] = elem
		}
	}
}

// getUnpackedAddrDescContracts returns partially unpacked AddrContracts for given addrDesc
func (d *RocksDB) getUnpackedAddrDescContracts(addrDesc bchain.AddressDescriptor) (*unpackedAddrContracts, error) {
	d.addrContractsCacheMux.Lock()
	rv, found := d.addrContractsCache[string(addrDesc)]
	d.addrContractsCacheMux.Unlock()
	if found && rv != nil {
		return rv, nil
	}
	val, err := d.db.GetCF(d.ro, d.cfh[cfAddressContracts], addrDesc)
	if err != nil {
		return nil, err
	}
	defer val.Free()
	buf := val.Data()
	if len(buf) == 0 {
		return nil, nil
	}
	rv, err = partiallyUnpackAddrContracts(buf)
	if err == nil && rv != nil && len(buf) > addrContractsCacheMinSize {
		d.addrContractsCacheMux.Lock()
		d.addrContractsCache[string(addrDesc)] = rv
		d.addrContractsCacheMux.Unlock()
	}
	return rv, err
}

// to speed up import of blocks, the unpacking of big ints is deferred to time when they are needed
func partiallyUnpackAddrContracts(buf []byte) (acs *unpackedAddrContracts, err error) {
	// make copy of the slice to avoid subsequent allocation of smaller slices
	buf = append([]byte{}, buf...)
	index := 0
	tt, l := unpackVaruint(buf)
	index += l
	nct, l := unpackVaruint(buf[index:])
	index += l
	ict, l := unpackVaruint(buf[index:])
	index += l
	cl, l := unpackVaruint(buf[index:])
	index += l
	c := make([]unpackedAddrContract, 0, cl)
	for index < len(buf) {
		contract := buf[index : index+eth.EthereumTypeAddressDescriptorLen]
		index += eth.EthereumTypeAddressDescriptorLen
		txs, l := unpackVaruint(buf[index:])
		index += l
		standard := bchain.TokenStandard(txs & 3)
		txs >>= 2
		ac := unpackedAddrContract{
			Standard: standard,
			Contract: contract,
			Txs:      txs,
		}
		if standard == bchain.FungibleToken {
			l := packedBigintLen(buf[index:])
			ac.Value = unpackedBigInt{Slice: buf[index : index+l]}
			index += l
		} else {
			len, ll := unpackVaruint(buf[index:])
			index += ll
			if standard == bchain.NonFungibleToken {
				ac.Ids = make(unpackedIds, len)
				for i := uint(0); i < len; i++ {
					ll := packedBigintLen(buf[index:])
					ac.Ids[i] = unpackedBigInt{Slice: buf[index : index+ll]}
					index += ll
				}
			} else {
				ac.MultiTokenValues = make(unpackedMultiTokenValues, len)
				for i := uint(0); i < len; i++ {
					ll := packedBigintLen(buf[index:])
					ac.MultiTokenValues[i].Id = unpackedBigInt{Slice: buf[index : index+ll]}
					index += ll
					ll = packedBigintLen(buf[index:])
					ac.MultiTokenValues[i].Value = unpackedBigInt{Slice: buf[index : index+ll]}
					index += ll
				}
			}
		}
		c = append(c, ac)
	}
	return &unpackedAddrContracts{
		Packed:         buf,
		TotalTxs:       tt,
		NonContractTxs: nct,
		InternalTxs:    ict,
		Contracts:      c,
	}, nil
}

// packUnpackedAddrContracts packs unpackedAddrContracts into a byte buffer
func packUnpackedAddrContracts(acs *unpackedAddrContracts) []byte {
	buf := make([]byte, 0, len(acs.Packed)+eth.EthereumTypeAddressDescriptorLen+12)
	varBuf := make([]byte, maxPackedBigintBytes)
	l := packVaruint(acs.TotalTxs, varBuf)
	buf = append(buf, varBuf[:l]...)
	l = packVaruint(acs.NonContractTxs, varBuf)
	buf = append(buf, varBuf[:l]...)
	l = packVaruint(acs.InternalTxs, varBuf)
	buf = append(buf, varBuf[:l]...)
	l = packVaruint(uint(len(acs.Contracts)), varBuf)
	buf = append(buf, varBuf[:l]...)
	for _, ac := range acs.Contracts {
		buf = append(buf, ac.Contract...)
		l = packVaruint(uint(ac.Standard)+ac.Txs<<2, varBuf)
		buf = append(buf, varBuf[:l]...)
		if ac.Standard == bchain.FungibleToken {
			if ac.Value.Value != nil {
				l = packBigint(ac.Value.Value, varBuf)
				buf = append(buf, varBuf[:l]...)
			} else {
				buf = append(buf, ac.Value.Slice...)
			}
		} else if ac.Standard == bchain.NonFungibleToken {
			l = packVaruint(uint(len(ac.Ids)), varBuf)
			buf = append(buf, varBuf[:l]...)
			for i := range ac.Ids {
				if ac.Ids[i].Value != nil {
					l = packBigint(ac.Ids[i].Value, varBuf)
					buf = append(buf, varBuf[:l]...)
				} else {
					buf = append(buf, ac.Ids[i].Slice...)
				}
			}
		} else { // bchain.ERC1155
			l = packVaruint(uint(len(ac.MultiTokenValues)), varBuf)
			buf = append(buf, varBuf[:l]...)
			for i := range ac.MultiTokenValues {
				if ac.MultiTokenValues[i].Id.Value != nil {
					l = packBigint(ac.MultiTokenValues[i].Id.Value, varBuf)
					buf = append(buf, varBuf[:l]...)
				} else {
					buf = append(buf, ac.MultiTokenValues[i].Id.Slice...)
				}
				if ac.MultiTokenValues[i].Value.Value != nil {
					l = packBigint(ac.MultiTokenValues[i].Value.Value, varBuf)
					buf = append(buf, varBuf[:l]...)
				} else {
					buf = append(buf, ac.MultiTokenValues[i].Value.Slice...)
				}
			}
		}
	}
	return buf
}

func (d *RocksDB) storeUnpackedAddressContracts(wb *grocksdb.WriteBatch, acm map[string]*unpackedAddrContracts) error {
	for addrDesc, acs := range acm {
		// address with 0 contracts is removed from db - happens on disconnect
		if acs == nil || (acs.NonContractTxs == 0 && acs.InternalTxs == 0 && len(acs.Contracts) == 0) {
			wb.DeleteCF(d.cfh[cfAddressContracts], bchain.AddressDescriptor(addrDesc))
		} else {
			// do not store large address contracts found in cache
			if _, found := d.addrContractsCache[addrDesc]; !found {
				buf := packUnpackedAddrContracts(acs)
				wb.PutCF(d.cfh[cfAddressContracts], bchain.AddressDescriptor(addrDesc), buf)
			}
		}
	}
	return nil
}

func (d *RocksDB) writeContractsCache() {
	wb := grocksdb.NewWriteBatch()
	defer wb.Destroy()
	d.addrContractsCacheMux.Lock()
	for addrDesc, acs := range d.addrContractsCache {
		buf := packUnpackedAddrContracts(acs)
		wb.PutCF(d.cfh[cfAddressContracts], bchain.AddressDescriptor(addrDesc), buf)
	}
	d.addrContractsCacheMux.Unlock()
	if err := d.WriteBatch(wb); err != nil {
		glog.Error("writeContractsCache: failed to store addrContractsCache: ", err)
	}
}

func (d *RocksDB) storeAddrContractsCache() {
	start := time.Now()
	if len(d.addrContractsCache) > 0 {
		d.writeContractsCache()
	}
	glog.Info("storeAddrContractsCache: store ", len(d.addrContractsCache), " entries in ", time.Since(start))
}

func (d *RocksDB) periodicStoreAddrContractsCache() {
	period := time.Duration(5) * time.Minute
	timer := time.NewTimer(period)
	for {
		<-timer.C
		timer.Reset(period)
		d.storeAddrContractsCache()
	}
}
