package db

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/eth"
	"bytes"
	"encoding/hex"

	"github.com/bsm/go-vlq"
	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/tecbot/gorocksdb"
)

// AddrContract is Contract address with number of transactions done by given address
type AddrContract struct {
	Contract bchain.AddressDescriptor
	Txs      uint
}

// AddrContracts is array of contracts with
type AddrContracts struct {
	EthTxs    uint
	Contracts []AddrContract
}

func (d *RocksDB) storeAddressContracts(wb *gorocksdb.WriteBatch, acm map[string]*AddrContracts) error {
	buf := make([]byte, 64)
	varBuf := make([]byte, vlq.MaxLen64)
	for addrDesc, acs := range acm {
		// address with 0 contracts is removed from db - happens on disconnect
		if acs == nil || (acs.EthTxs == 0 && len(acs.Contracts) == 0) {
			wb.DeleteCF(d.cfh[cfAddressContracts], bchain.AddressDescriptor(addrDesc))
		} else {
			buf = buf[:0]
			l := packVaruint(acs.EthTxs, varBuf)
			buf = append(buf, varBuf[:l]...)
			for _, ac := range acs.Contracts {
				buf = append(buf, ac.Contract...)
				l = packVaruint(ac.Txs, varBuf)
				buf = append(buf, varBuf[:l]...)
			}
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
	c := make([]AddrContract, 0, 4)
	et, l := unpackVaruint(buf)
	buf = buf[l:]
	for len(buf) > 0 {
		if len(buf) < eth.EthereumTypeAddressDescriptorLen {
			return nil, errors.New("Invalid data stored in cfAddressContracts for AddrDesc " + addrDesc.String())
		}
		txs, l := unpackVaruint(buf[eth.EthereumTypeAddressDescriptorLen:])
		contract := make(bchain.AddressDescriptor, eth.EthereumTypeAddressDescriptorLen)
		copy(contract, buf[:eth.EthereumTypeAddressDescriptorLen])
		c = append(c, AddrContract{
			Contract: contract,
			Txs:      txs,
		})
		buf = buf[eth.EthereumTypeAddressDescriptorLen+l:]
	}
	return &AddrContracts{
		EthTxs:    et,
		Contracts: c}, nil
}

func (d *RocksDB) addToAddressesAndContractsEthereumType(addrDesc bchain.AddressDescriptor, btxID []byte, index int32, contract bchain.AddressDescriptor, addresses map[string][]outpoint, addressContracts map[string]*AddrContracts) error {
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
		ac.EthTxs++
	} else {
		// locate the contract and set i to the index in the array of contracts
		var i int
		var found bool
		for i = range ac.Contracts {
			if bytes.Equal(contract, ac.Contracts[i].Contract) {
				found = true
				break
			}
		}
		if !found {
			i = len(ac.Contracts)
			ac.Contracts = append(ac.Contracts, AddrContract{Contract: contract})
		}
		// index 0 is for ETH transfers, contract indexes start with 1
		if index < 0 {
			index = ^int32(i + 1)
		} else {
			index = int32(i + 1)
		}
		ac.Contracts[i].Txs++
	}
	addresses[strAddrDesc] = append(addresses[strAddrDesc], outpoint{
		btxID: btxID,
		index: index,
	})
	return nil
}

type ethBlockTxContract struct {
	addr, contract bchain.AddressDescriptor
}

type ethBlockTx struct {
	btxID     []byte
	from, to  bchain.AddressDescriptor
	contracts []ethBlockTxContract
}

func (d *RocksDB) processAddressesEthereumType(block *bchain.Block, addresses map[string][]outpoint, addressContracts map[string]*AddrContracts) ([]ethBlockTx, error) {
	blockTxs := make([]ethBlockTx, len(block.Txs))
	for txi, tx := range block.Txs {
		btxID, err := d.chainParser.PackTxid(tx.Txid)
		if err != nil {
			return nil, err
		}
		blockTx := &blockTxs[txi]
		blockTx.btxID = btxID
		// there is only one output address in EthereumType transaction, store it in format txid 0
		if len(tx.Vout) == 1 && len(tx.Vout[0].ScriptPubKey.Addresses) == 1 {
			addrDesc, err := d.chainParser.GetAddrDescFromAddress(tx.Vout[0].ScriptPubKey.Addresses[0])
			if err != nil {
				// do not log ErrAddressMissing, transactions can be without to address (for example eth contracts)
				if err != bchain.ErrAddressMissing {
					glog.Warningf("rocksdb: addrDesc: %v - height %d, tx %v, output", err, block.Height, tx.Txid)
				}
				continue
			}
			if err = d.addToAddressesAndContractsEthereumType(addrDesc, btxID, 0, nil, addresses, addressContracts); err != nil {
				return nil, err
			}
			blockTx.to = addrDesc
		}
		// there is only one input address in EthereumType transaction, store it in format txid ^0
		if len(tx.Vin) == 1 && len(tx.Vin[0].Addresses) == 1 {
			addrDesc, err := d.chainParser.GetAddrDescFromAddress(tx.Vin[0].Addresses[0])
			if err != nil {
				if err != bchain.ErrAddressMissing {
					glog.Warningf("rocksdb: addrDesc: %v - height %d, tx %v, input", err, block.Height, tx.Txid)
				}
				continue
			}
			if err = d.addToAddressesAndContractsEthereumType(addrDesc, btxID, ^int32(0), nil, addresses, addressContracts); err != nil {
				return nil, err
			}
			blockTx.from = addrDesc
		}
		// store erc20 transfers
		erc20, err := eth.GetErc20FromTx(&tx)
		if err != nil {
			glog.Warningf("rocksdb: GetErc20FromTx %v - height %d, tx %v", err, block.Height, tx.Txid)
		}
		blockTx.contracts = make([]ethBlockTxContract, len(erc20)*2)
		for i, t := range erc20 {
			var contract, from, to bchain.AddressDescriptor
			contract, err = d.chainParser.GetAddrDescFromAddress(t.Contract)
			if err == nil {
				from, err = d.chainParser.GetAddrDescFromAddress(t.From)
				if err == nil {
					to, err = d.chainParser.GetAddrDescFromAddress(t.To)
				}
			}
			if err != nil {
				glog.Warningf("rocksdb: GetErc20FromTx %v - height %d, tx %v, transfer %v", err, block.Height, tx.Txid, t)
				continue
			}
			if err = d.addToAddressesAndContractsEthereumType(from, btxID, ^int32(i), contract, addresses, addressContracts); err != nil {
				return nil, err
			}
			bc := &blockTx.contracts[i*2]
			bc.addr = from
			bc.contract = contract
			if err = d.addToAddressesAndContractsEthereumType(to, btxID, int32(i), contract, addresses, addressContracts); err != nil {
				return nil, err
			}
			bc = &blockTx.contracts[i*2+1]
			bc.addr = to
			bc.contract = contract
		}
	}
	return blockTxs, nil
}

func (d *RocksDB) storeAndCleanupBlockTxsEthereumType(wb *gorocksdb.WriteBatch, block *bchain.Block, blockTxs []ethBlockTx) error {
	pl := d.chainParser.PackedTxidLen()
	buf := make([]byte, 0, (pl+2*eth.EthereumTypeAddressDescriptorLen)*len(blockTxs))
	varBuf := make([]byte, vlq.MaxLen64)
	zeroAddress := make([]byte, eth.EthereumTypeAddressDescriptorLen)
	appendAddress := func(a bchain.AddressDescriptor) {
		if len(a) != eth.EthereumTypeAddressDescriptorLen {
			buf = append(buf, zeroAddress...)
		} else {
			buf = append(buf, a...)
		}
	}
	for i := range blockTxs {
		blockTx := &blockTxs[i]
		buf = append(buf, blockTx.btxID...)
		appendAddress(blockTx.from)
		appendAddress(blockTx.to)
		l := packVaruint(uint(len(blockTx.contracts)), varBuf)
		buf = append(buf, varBuf[:l]...)
		for j := range blockTx.contracts {
			c := &blockTx.contracts[j]
			appendAddress(c.addr)
			appendAddress(c.contract)
		}
	}
	key := packUint(block.Height)
	wb.PutCF(d.cfh[cfBlockTxs], key, buf)
	return d.cleanupBlockTxs(wb, block)
}

// DisconnectBlockRangeNonUTXO performs full range scan to remove a range of blocks
// it is very slow operation
func (d *RocksDB) DisconnectBlockRangeNonUTXO(lower uint32, higher uint32) error {
	glog.Infof("db: disconnecting blocks %d-%d", lower, higher)
	addrKeys, _, err := d.allAddressesScan(lower, higher)
	if err != nil {
		return err
	}
	glog.Infof("rocksdb: about to disconnect %d addresses ", len(addrKeys))
	wb := gorocksdb.NewWriteBatch()
	defer wb.Destroy()
	for _, addrKey := range addrKeys {
		if glog.V(2) {
			glog.Info("address ", hex.EncodeToString(addrKey))
		}
		// delete address:height from the index
		wb.DeleteCF(d.cfh[cfAddresses], addrKey)
	}
	for height := lower; height <= higher; height++ {
		if glog.V(2) {
			glog.Info("height ", height)
		}
		wb.DeleteCF(d.cfh[cfHeight], packUint(height))
	}
	err = d.db.Write(d.wo, wb)
	if err == nil {
		glog.Infof("rocksdb: blocks %d-%d disconnected", lower, higher)
	}
	return err
}
