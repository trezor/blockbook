package db

import (
	"bytes"
	"encoding/hex"
	"math/big"
	"os"

	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/linxGnu/grocksdb"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/xcb"
)

// packCoreCoinAddrContract packs AddrContracts into a byte buffer
func packCoreCoinAddrContract(acs *AddrContracts) []byte {
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
		if ac.Type == bchain.FungibleToken {
			l = packBigint(&ac.Value, varBuf)
			buf = append(buf, varBuf[:l]...)
		} else if ac.Type == bchain.NonFungibleToken {
			l = packVaruint(uint(len(ac.Ids)), varBuf)
			buf = append(buf, varBuf[:l]...)
			for i := range ac.Ids {
				l = packBigint(&ac.Ids[i], varBuf)
				buf = append(buf, varBuf[:l]...)
			}
		} else {
			panic("other token types are not implemented")
		}
	}
	return buf
}

func unpackCoreCoinAddrContracts(buf []byte, addrDesc bchain.AddressDescriptor) (*AddrContracts, error) {
	tt, l := unpackVaruint(buf)
	buf = buf[l:]
	nct, l := unpackVaruint(buf)
	buf = buf[l:]
	ict, l := unpackVaruint(buf)
	buf = buf[l:]
	c := make([]AddrContract, 0, 4)
	for len(buf) > 0 {
		if len(buf) < xcb.CoreCoinTypeAddressDescriptorLen {
			return nil, errors.New("Invalid data stored in cfAddressContracts for AddrDesc " + addrDesc.String())
		}
		contract := append(bchain.AddressDescriptor(nil), buf[:xcb.CoreCoinTypeAddressDescriptorLen]...)
		txs, l := unpackVaruint(buf[xcb.CoreCoinTypeAddressDescriptorLen:])
		buf = buf[xcb.CoreCoinTypeAddressDescriptorLen+l:]
		ttt := bchain.TokenType(txs & 3)
		txs >>= 2
		ac := AddrContract{
			Type:     ttt,
			Contract: contract,
			Txs:      txs,
		}
		if ttt == bchain.FungibleToken {
			b, ll := unpackBigint(buf)
			buf = buf[ll:]
			ac.Value = b
		} else {
			len, ll := unpackVaruint(buf)
			buf = buf[ll:]
			if ttt == bchain.NonFungibleToken {
				ac.Ids = make(Ids, len)
				for i := uint(0); i < len; i++ {
					b, ll := unpackBigint(buf)
					buf = buf[ll:]
					ac.Ids[i] = b
				}
			} else {
				panic("other token types are not implemented")
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

// GetCoreCoinAddrDescContracts returns AddrContracts for given addrDesc
func (d *RocksDB) GetCoreCoinAddrDescContracts(addrDesc bchain.AddressDescriptor) (*AddrContracts, error) {
	val, err := d.db.GetCF(d.ro, d.cfh[cfAddressContracts], addrDesc)
	if err != nil {
		return nil, err
	}
	defer val.Free()
	buf := val.Data()
	if len(buf) == 0 {
		return nil, nil
	}
	return unpackCoreCoinAddrContracts(buf, addrDesc)
}

// addToAddressesMapCoreCoinType maintains mapping between addresses and transactions in one block
// it ensures that each index is there only once, there can be for example multiple internal transactions of the same address
// the return value is true if the tx was processed before, to not to count the tx multiple times
func addToAddressesMapCoreCoinType(addresses addressesMap, strAddrDesc string, btxID []byte, index int32) bool {
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

func addToCoreCoinContract(c *AddrContract, contractIndex int, index int32, contract bchain.AddressDescriptor, transfer *bchain.TokenTransfer, addTxCount bool) int32 {
	var aggregate AggregateFn
	// index 0 is for XCB transfers, index 1 (InternalTxIndexOffset) is for internal transfers, contract indexes start with 2 (ContractIndexOffset)
	if index < 0 {
		index = ^int32(contractIndex + ContractIndexOffset)
		aggregate = func(s, v *big.Int) {
			s.Sub(s, v)
			if s.Sign() < 0 {
				// glog.Warningf("rocksdb: addToCoreCoinContract: contract %s, from %s, negative aggregate", transfer.Contract, transfer.From)
				s.SetUint64(0)
			}
		}
	} else {
		index = int32(contractIndex + ContractIndexOffset)
		aggregate = func(s, v *big.Int) {
			s.Add(s, v)
		}
	}
	if transfer.Type == bchain.FungibleToken {
		aggregate(&c.Value, &transfer.Value)
	} else if transfer.Type == bchain.NonFungibleToken {
		if index < 0 {
			c.Ids.remove(transfer.Value)
		} else {
			c.Ids.insert(transfer.Value)
		}
	} else {
		panic("token is not implemented")
	}
	if addTxCount {
		c.Txs++
	}
	return index
}

func (d *RocksDB) addToAddressesAndContractsCoreCoinType(addrDesc bchain.AddressDescriptor, btxID []byte, index int32, contract bchain.AddressDescriptor, transfer *bchain.TokenTransfer, addTxCount bool, addresses addressesMap, addressContracts map[string]*AddrContracts) error {
	var err error
	strAddrDesc := string(addrDesc)
	ac, e := addressContracts[strAddrDesc]
	if !e {
		ac, err = d.GetCoreCoinAddrDescContracts(addrDesc)
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
			index = addToCoreCoinContract(c, contractIndex, index, contract, transfer, addTxCount)
		} else {
			if index < 0 {
				index = transferFrom
			} else {
				index = transferTo
			}
		}
	}
	counted := addToAddressesMapCoreCoinType(addresses, strAddrDesc, btxID, index)
	if !counted {
		ac.TotalTxs++
	}
	return nil
}

type xcbBlockTxContract struct {
	from, to, contract bchain.AddressDescriptor
	transferType       bchain.TokenType
	value              big.Int
	idValues           []bchain.MultiTokenValue
}

type xcbBlockTx struct {
	btxID     []byte
	from, to  bchain.AddressDescriptor
	contracts []xcbBlockTxContract
}

func (d *RocksDB) processBaseCoreCoinTxData(blockTx *xcbBlockTx, tx *bchain.Tx, addresses addressesMap, addressContracts map[string]*AddrContracts) error {
	var from, to bchain.AddressDescriptor
	var err error
	// there is only one output address in CoreCoinType transaction, store it in format txid 0
	if len(tx.Vout) == 1 && len(tx.Vout[0].ScriptPubKey.Addresses) == 1 {
		to, err = d.chainParser.GetAddrDescFromAddress(tx.Vout[0].ScriptPubKey.Addresses[0])
		if err != nil {
			// do not log ErrAddressMissing, transactions can be without to address (for example xcb contracts)
			if err != bchain.ErrAddressMissing {
				glog.Warningf("rocksdb: processBaseTxData: %v, tx %v, output", err, tx.Txid)
			}
		} else {
			if err = d.addToAddressesAndContractsCoreCoinType(to, blockTx.btxID, transferTo, nil, nil, true, addresses, addressContracts); err != nil {
				return err
			}
			blockTx.to = to
		}
	}
	// there is only one input address in CoreCoinType transaction, store it in format txid ^0
	if len(tx.Vin) == 1 && len(tx.Vin[0].Addresses) == 1 {
		from, err = d.chainParser.GetAddrDescFromAddress(tx.Vin[0].Addresses[0])
		if err != nil {
			if err != bchain.ErrAddressMissing {
				glog.Warningf("rocksdb: processBaseTxData: %v, tx %v, input", err, tx.Txid)
			}
		} else {
			if err = d.addToAddressesAndContractsCoreCoinType(from, blockTx.btxID, transferFrom, nil, nil, !bytes.Equal(from, to), addresses, addressContracts); err != nil {
				return err
			}
			blockTx.from = from
		}
	}
	return nil
}

// func (d *RocksDB) setAddressTxIndexesToAddressMap(addrDesc bchain.AddressDescriptor, height uint32, addresses addressesMap) error {
// 	strAddrDesc := string(addrDesc)
// 	_, found := addresses[strAddrDesc]
// 	if !found {
// 		txIndexes, err := d.getTxIndexesForAddressAndBlock(addrDesc, height)
// 		if err != nil {
// 			return err
// 		}
// 		if len(txIndexes) > 0 {
// 			addresses[strAddrDesc] = txIndexes
// 		}
// 	}
// 	return nil
// }

func (d *RocksDB) processCoreCoinContractTransfers(blockTx *xcbBlockTx, tx *bchain.Tx, addresses addressesMap, addressContracts map[string]*AddrContracts) error {
	tokenTransfers, err := d.chainParser.CoreCoinTypeGetTokenTransfersFromTx(tx)
	if err != nil {
		glog.Warningf("rocksdb: processCoreCoinContractTransfers %v, tx %v", err, tx.Txid)
	}
	blockTx.contracts = make([]xcbBlockTxContract, len(tokenTransfers))
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
			glog.Warningf("rocksdb: processCoreCoinContractTransfers %v, tx %v, transfer %v", err, tx.Txid, t)
			continue
		}
		if err = d.addToAddressesAndContractsCoreCoinType(to, blockTx.btxID, int32(i), contract, t, true, addresses, addressContracts); err != nil {
			return err
		}
		eq := bytes.Equal(from, to)
		if err = d.addToAddressesAndContractsCoreCoinType(from, blockTx.btxID, ^int32(i), contract, t, !eq, addresses, addressContracts); err != nil {
			return err
		}
		bc := &blockTx.contracts[i]
		bc.transferType = t.Type
		bc.from = from
		bc.to = to
		bc.contract = contract
		bc.value = t.Value
		bc.idValues = t.MultiTokenValues
	}
	return nil
}

func (d *RocksDB) processAddressesCoreCoinType(block *bchain.Block, addresses addressesMap, addressContracts map[string]*AddrContracts) ([]xcbBlockTx, error) {
	blockTxs := make([]xcbBlockTx, len(block.Txs))
	for txi := range block.Txs {
		tx := &block.Txs[txi]
		btxID, err := d.chainParser.PackTxid(tx.Txid)
		if err != nil {
			return nil, err
		}
		blockTx := &blockTxs[txi]
		blockTx.btxID = btxID
		if err = d.processBaseCoreCoinTxData(blockTx, tx, addresses, addressContracts); err != nil {
			return nil, err
		}
		// store contract transfers
		if err = d.processCoreCoinContractTransfers(blockTx, tx, addresses, addressContracts); err != nil {
			return nil, err
		}
	}
	return blockTxs, nil
}

var xcbZeroAddress []byte = make([]byte, xcb.CoreCoinTypeAddressDescriptorLen)

func appendXcbAddress(buf []byte, a bchain.AddressDescriptor) []byte {
	if len(a) != xcb.CoreCoinTypeAddressDescriptorLen {
		buf = append(buf, xcbZeroAddress...)
	} else {
		buf = append(buf, a...)
	}
	return buf
}

func packCoreCoinBlockTx(buf []byte, blockTx *xcbBlockTx) []byte {
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
		if c.transferType == bchain.FungibleToken || c.transferType == bchain.NonFungibleToken {
			l = packBigint(&c.value, varBuf)
			buf = append(buf, varBuf[:l]...)
		} else {
			panic("token in not implemented")
		}
	}
	return buf
}

func (d *RocksDB) storeAndCleanupBlockTxsCoreCoinType(wb *grocksdb.WriteBatch, block *bchain.Block, blockTxs []xcbBlockTx) error {
	pl := d.chainParser.PackedTxidLen()
	buf := make([]byte, 0, (pl+2*xcb.CoreCoinTypeAddressDescriptorLen)*len(blockTxs))
	for i := range blockTxs {
		buf = packCoreCoinBlockTx(buf, &blockTxs[i])
	}
	key := packUint(block.Height)
	wb.PutCF(d.cfh[cfBlockTxs], key, buf)
	return d.cleanupBlockTxs(wb, block)
}

func (d *RocksDB) storeBlockSpecificDataCoreCoinType(wb *grocksdb.WriteBatch, block *bchain.Block) error {
	blockSpecificData, _ := block.CoinSpecificData.(*xcb.CoreCoinBlockSpecificData)
	if blockSpecificData != nil {
		for i := range blockSpecificData.Contracts {
			if err := d.storeContractInfo(wb, &blockSpecificData.Contracts[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

// unpackCoreCoinBlockTx unpacks xcbBlockTx from buf, starting at position pos
// the position is updated as the data is unpacked and returned to the caller
func unpackCoreCoinBlockTx(buf []byte, pos int) (*xcbBlockTx, int, error) {
	getAddress := func(i int) (bchain.AddressDescriptor, int, error) {
		if len(buf)-i < xcb.CoreCoinTypeAddressDescriptorLen {
			glog.Error("rocksdb: Inconsistent data in blockTxs ", hex.EncodeToString(buf))
			return nil, 0, errors.New("Inconsistent data in blockTxs")
		}
		a := append(bchain.AddressDescriptor(nil), buf[i:i+xcb.CoreCoinTypeAddressDescriptorLen]...)
		return a, i + xcb.CoreCoinTypeAddressDescriptorLen, nil
	}
	var from, to bchain.AddressDescriptor
	var err error
	if len(buf)-pos < xcb.CoreCoinTypeTxidLen {
		glog.Error("rocksdb: Inconsistent data in blockTxs ", hex.EncodeToString(buf))
		return nil, 0, errors.New("Inconsistent data in blockTxs")
	}
	txid := append([]byte(nil), buf[pos:pos+xcb.CoreCoinTypeTxidLen]...)
	pos += xcb.CoreCoinTypeTxidLen
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
	contracts := make([]xcbBlockTxContract, cc)
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
		c.transferType = bchain.TokenType(cc)
		pos += l
		if c.transferType == bchain.FungibleToken || c.transferType == bchain.NonFungibleToken {
			c.value, l = unpackBigint(buf[pos:])
			pos += l
		} else {
			panic("token in not implemented")
		}
	}
	return &xcbBlockTx{
		btxID:     txid,
		from:      from,
		to:        to,
		contracts: contracts,
	}, pos, nil
}

func (d *RocksDB) getBlockTxsCoreCoinType(height uint32) ([]xcbBlockTx, error) {
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
	bt := make([]xcbBlockTx, 0, 16)
	var btx *xcbBlockTx
	for i := 0; i < len(buf); {
		btx, i, err = unpackCoreCoinBlockTx(buf, i)
		if err != nil {
			return nil, err
		}
		bt = append(bt, *btx)
	}
	return bt, nil
}

func (d *RocksDB) disconnectCoreCoinAddress(btxID []byte, internal bool, addrDesc bchain.AddressDescriptor, btxContract *xcbBlockTxContract, addresses map[string]map[string]struct{}, contracts map[string]*AddrContracts) error {
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
		addrContracts, err = d.GetCoreCoinAddrDescContracts(addrDesc)
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
						addToCoreCoinContract(addrContract, contractIndex, index, btxContract.contract, &bchain.TokenTransfer{
							Type:             btxContract.transferType,
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

func (d *RocksDB) disconnectBlockTxsCoreCoinType(wb *grocksdb.WriteBatch, height uint32, blockTxs []xcbBlockTx, contracts map[string]*AddrContracts) error {
	glog.Info("Disconnecting block ", height, " containing ", len(blockTxs), " transactions")
	addresses := make(map[string]map[string]struct{})
	for i := range blockTxs {
		blockTx := &blockTxs[i]
		if err := d.disconnectCoreCoinAddress(blockTx.btxID, false, blockTx.from, nil, addresses, contracts); err != nil {
			return err
		}
		// if from==to, tx is counted only once and does not have to be disconnected again
		if !bytes.Equal(blockTx.from, blockTx.to) {
			if err := d.disconnectCoreCoinAddress(blockTx.btxID, false, blockTx.to, nil, addresses, contracts); err != nil {
				return err
			}
		}
		// contracts
		for j := range blockTx.contracts {
			c := &blockTx.contracts[j]
			if err := d.disconnectCoreCoinAddress(blockTx.btxID, false, c.from, c, addresses, contracts); err != nil {
				return err
			}
			if !bytes.Equal(c.from, c.to) {
				if err := d.disconnectCoreCoinAddress(blockTx.btxID, false, c.to, c, addresses, contracts); err != nil {
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

// DisconnectBlockRangeCoreCoinType removes all data belonging to blocks in range lower-higher
// it is able to disconnect only blocks for which there are data in the blockTxs column
func (d *RocksDB) DisconnectBlockRangeCoreCoinType(lower uint32, higher uint32) error {
	blocks := make([][]xcbBlockTx, higher-lower+1)
	for height := lower; height <= higher; height++ {
		blockTxs, err := d.getBlockTxsCoreCoinType(height)
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
	contracts := make(map[string]*AddrContracts)
	for height := higher; height >= lower; height-- {
		if err := d.disconnectBlockTxsCoreCoinType(wb, height, blocks[height-lower], contracts); err != nil {
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

func (d *RocksDB) SortCoreCoinAddressContracts(stop chan os.Signal) error {
	if d.chainParser.GetChainType() != bchain.ChainCoreCoinType {
		glog.Info("SortCoreCoinAddressContracts: applicable only for corecoin type")
		return nil
	}
	glog.Info("SortCoreCoinAddressContracts: starting")
	// do not use cache
	ro := grocksdb.NewDefaultReadOptions()
	ro.SetFillCache(false)
	it := d.db.NewIteratorCF(ro, d.cfh[cfAddressContracts])
	defer it.Close()
	var rowCount, idsSortedCount, multiTokenValuesSortedCount int
	for it.SeekToFirst(); it.Valid(); it.Next() {
		select {
		case <-stop:
			return errors.New("SortCoreCoinAddressContracts: interrupted")
		default:
		}
		rowCount++
		addrDesc := it.Key().Data()
		buf := it.Value().Data()
		if len(buf) > 0 {
			ca, err := unpackCoreCoinAddrContracts(buf, addrDesc)
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
			glog.Infof("SortCoreCoinAddressContracts: progress - scanned %d rows, sorted %d ids and %d multi token values", rowCount, idsSortedCount, multiTokenValuesSortedCount)
		}
	}
	glog.Infof("SortCoreCoinAddressContracts: finished - scanned %d rows, sorted %d ids and %d multi token value", rowCount, idsSortedCount, multiTokenValuesSortedCount)
	return nil
}
