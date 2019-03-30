package iocoin

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"

	"github.com/martinboehm/btcd/wire"
//	"bytes"
	//"github.com/golang/glog"
	"github.com/martinboehm/btcutil/chaincfg"
)

const (
	MainnetMagic wire.BitcoinNet =  0xfec3bade
	TestnetMagic wire.BitcoinNet =  0xffc4bbdf
)

var (
	MainNetParams chaincfg.Params
	TestNetParams chaincfg.Params
)

func init() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic
	MainNetParams.PubKeyHashAddrID = []byte{103}
	MainNetParams.ScriptHashAddrID = []byte{85}

	TestNetParams = chaincfg.TestNet3Params
	TestNetParams.Net = TestnetMagic
	TestNetParams.PubKeyHashAddrID = []byte{111} // starting with 'x' or 'y'
	TestNetParams.ScriptHashAddrID = []byte{96}
}

// IocoinParser handle
type IocoinParser struct {
	*btc.BitcoinParser
	baseparser                         *bchain.BaseParser
}

// NewIocoinParser returns new IocoinParser instance
func NewIocoinParser(params *chaincfg.Params, c *btc.Configuration) *IocoinParser {
	p := &IocoinParser{
		BitcoinParser: btc.NewBitcoinParser(params, c),
		baseparser:    &bchain.BaseParser{},
	}
  return p
}

// GetChainParams contains network parameters for the main Iocoin network,
// and the test Iocoin network
func GetChainParams(chain string) *chaincfg.Params {
	if !chaincfg.IsRegistered(&MainNetParams) {
		err := chaincfg.Register(&MainNetParams)
		if err == nil {
			err = chaincfg.Register(&TestNetParams)
		}
		if err != nil {
			panic(err)
		}
	}
	switch chain {
	case "test":
		return &TestNetParams
	default:
		return &MainNetParams
	}
}
func (p *IocoinParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
	return p.baseparser.PackTx(tx, height, blockTime)
}

// UnpackTx unpacks transaction from protobuf byte array
func (p *IocoinParser) UnpackTx(buf []byte) (*bchain.Tx, uint32, error) {
	return p.baseparser.UnpackTx(buf)
}
