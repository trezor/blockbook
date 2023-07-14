package nuls

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"

	vlq "github.com/bsm/go-vlq"
	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil/base58"
	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/martinboehm/btcutil/hdkeychain"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
)

// magic numbers
const (
	MainnetMagic wire.BitcoinNet = 0xbd6b0cbf
	TestnetMagic wire.BitcoinNet = 0xffcae2ce
	RegtestMagic wire.BitcoinNet = 0xdcb7c1fc

	AddressHashLength = 24
)

// chain parameters
var (
	MainNetParams chaincfg.Params
	TestNetParams chaincfg.Params
	RegtestParams chaincfg.Params
)

func init() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic

	// Address encoding magics
	MainNetParams.AddressMagicLen = 3

	// Address encoding magics
	MainNetParams.PubKeyHashAddrID = []byte{4, 35, 1} // base58 prefix: Ns
	MainNetParams.ScriptHashAddrID = []byte{4, 35, 1} // base58 prefix: Ns

	TestNetParams = chaincfg.TestNet3Params
	TestNetParams.Net = TestnetMagic

	// Address encoding magics
	TestNetParams.PubKeyHashAddrID = []byte{140} // base58 prefix: y
	TestNetParams.ScriptHashAddrID = []byte{19}  // base58 prefix: 8 or 9

	RegtestParams = chaincfg.RegressionNetParams
	RegtestParams.Net = RegtestMagic

	// Address encoding magics
	RegtestParams.PubKeyHashAddrID = []byte{140} // base58 prefix: y
	RegtestParams.ScriptHashAddrID = []byte{19}  // base58 prefix: 8 or 9
}

// NulsParser handle
type NulsParser struct {
	*btc.BitcoinLikeParser
}

// NewNulsParser returns new NulsParser instance
func NewNulsParser(params *chaincfg.Params, c *btc.Configuration) *NulsParser {
	return &NulsParser{BitcoinLikeParser: btc.NewBitcoinLikeParser(params, c)}
}

// GetChainParams contains network parameters for the main Gincoin network,
// the regression test Gincoin network, the test Gincoin network and
// the simulation test Gincoin network, in this order
func GetChainParams(chain string) *chaincfg.Params {
	if !chaincfg.IsRegistered(&MainNetParams) {
		err := chaincfg.Register(&MainNetParams)
		if err == nil {
			err = chaincfg.Register(&TestNetParams)
		}
		if err == nil {
			err = chaincfg.Register(&RegtestParams)
		}
		if err != nil {
			panic(err)
		}
	}
	switch chain {
	case "test":
		return &TestNetParams
	case "regtest":
		return &RegtestParams
	default:
		return &MainNetParams
	}
}

// PackedTxidLen returns length in bytes of packed txid
func (p *NulsParser) PackedTxidLen() int {
	return 34
}

// GetAddrDescFromAddress returns internal address representation (descriptor) of given address
func (p *NulsParser) GetAddrDescFromAddress(address string) (bchain.AddressDescriptor, error) {
	addressByte := base58.Decode(address)
	return bchain.AddressDescriptor(addressByte), nil
}

// GetAddrDescFromVout returns internal address representation (descriptor) of given transaction output
func (p *NulsParser) GetAddrDescFromVout(output *bchain.Vout) (bchain.AddressDescriptor, error) {
	addressStr := output.ScriptPubKey.Hex
	addressByte := base58.Decode(addressStr)
	return bchain.AddressDescriptor(addressByte), nil
}

// GetAddressesFromAddrDesc returns addresses for given address descriptor with flag if the addresses are searchable
func (p *NulsParser) GetAddressesFromAddrDesc(addrDesc bchain.AddressDescriptor) ([]string, bool, error) {
	var addrs []string

	if addrDesc != nil {
		addrs = append(addrs, base58.Encode(addrDesc))
	}

	return addrs, true, nil
}

// PackTx packs transaction to byte array
func (p *NulsParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
	txBytes, error := json.Marshal(tx)
	if error != nil {
		return nil, error
	}

	buf := make([]byte, 4+vlq.MaxLen64)
	binary.BigEndian.PutUint32(buf[0:4], height)
	vlq.PutInt(buf[4:4+vlq.MaxLen64], blockTime)
	resByes := bytes.Join([][]byte{buf, txBytes}, []byte(""))
	return resByes, nil
}

// UnpackTx unpacks transaction from byte array
func (p *NulsParser) UnpackTx(buf []byte) (*bchain.Tx, uint32, error) {
	height := binary.BigEndian.Uint32(buf)
	bt, _ := vlq.Int(buf[4 : 4+vlq.MaxLen64])
	tx, err := p.ParseTx(buf[4+vlq.MaxLen64:])
	if err != nil {
		return nil, 0, err
	}
	tx.Blocktime = bt

	return tx, height, nil
}

// ParseTx parses tx from blob
func (p *NulsParser) ParseTx(b []byte) (*bchain.Tx, error) {
	tx := bchain.Tx{}
	err := json.Unmarshal(b, &tx)

	if err != nil {
		return nil, err
	}
	return &tx, err
}

// DeriveAddressDescriptorsFromTo derives address descriptors from given xpub for addresses in index range
func (p *NulsParser) DeriveAddressDescriptorsFromTo(descriptor *bchain.XpubDescriptor, change uint32, fromIndex uint32, toIndex uint32) ([]bchain.AddressDescriptor, error) {
	if toIndex <= fromIndex {
		return nil, errors.New("toIndex<=fromIndex")
	}
	changeExtKey, err := descriptor.ExtKey.(*hdkeychain.ExtendedKey).Derive(change)
	if err != nil {
		return nil, err
	}
	ad := make([]bchain.AddressDescriptor, toIndex-fromIndex)
	for index := fromIndex; index < toIndex; index++ {
		indexExtKey, err := changeExtKey.Derive(index)
		if err != nil {
			return nil, err
		}
		s, err := indexExtKey.Address(p.Params)

		if err != nil && indexExtKey != nil {
			return nil, err
		}
		addHashs := make([]byte, AddressHashLength)
		copy(addHashs[0:3], p.Params.PubKeyHashAddrID)
		copy(addHashs[3:], s.ScriptAddress())
		copy(addHashs[23:], []byte{p.xor(addHashs[0:23])})

		//addressStr := base58.Encode(addHashs)
		ad[index-fromIndex] = addHashs
	}
	return ad, nil
}

func (p *NulsParser) xor(body []byte) byte {
	var xor byte = 0x00
	for i := 0; i < len(body); i++ {
		xor ^= body[i]
	}
	return xor
}
