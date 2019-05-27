package nuls

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"bytes"
	"encoding/binary"
	"encoding/json"

	vlq "github.com/bsm/go-vlq"
	"github.com/martinboehm/btcutil/base58"

	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil/chaincfg"
)

// magic numbers
const (
	MainnetMagic wire.BitcoinNet = 0xbd6b0cbf
	TestnetMagic wire.BitcoinNet = 0xffcae2ce
	RegtestMagic wire.BitcoinNet = 0xdcb7c1fc
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
	MainNetParams.PubKeyHashAddrID = []byte{38} // base58 prefix: G
	MainNetParams.ScriptHashAddrID = []byte{10} // base58 prefix: W

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
	*btc.BitcoinParser
}

// NewNulsParser returns new NulsParser instance
func NewNulsParser(params *chaincfg.Params, c *btc.Configuration) *NulsParser {
	return &NulsParser{BitcoinParser: btc.NewBitcoinParser(params, c)}
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
