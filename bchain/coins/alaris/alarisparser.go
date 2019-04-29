package alaris

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"

	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil/chaincfg"
)

const (
	MainnetMagic wire.BitcoinNet = 0xfbcfccd4
	TestnetMagic wire.BitcoinNet = 0xfbcdccd3
	RegtestMagic wire.BitcoinNet = 0xfabfb5da
)

var (
	MainNetParams chaincfg.Params
	TestNetParams chaincfg.Params
)

func init() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic
	MainNetParams.PubKeyHashAddrID = []byte{23}
	MainNetParams.ScriptHashAddrID = []byte{30}
	MainNetParams.Bech32HRPSegwit = "ala"

	TestNetParams = chaincfg.TestNet3Params
	TestNetParams.Net = TestnetMagic
	TestNetParams.PubKeyHashAddrID = []byte{33}
	TestNetParams.ScriptHashAddrID = []byte{55}
	TestNetParams.Bech32HRPSegwit = "tala"
}

// AlarisParser handle
type AlarisParser struct {
	*btc.BitcoinParser
	baseparser *bchain.BaseParser
}

// NewAlarisParser returns new AlarisParser instance
func NewAlarisParser(params *chaincfg.Params, c *btc.Configuration) *AlarisParser {
	return &AlarisParser{
	BitcoinParser: btc.NewBitcoinParser(params, c),
	baseparser:    &bchain.BaseParser{},
	}
}

// GetChainParams contains network parameters for the main Alaris network,
// and the test Alaris network
func GetChainParams(chain string) *chaincfg.Params {
	// register bitcoin parameters in addition to alaris parameters
	// alaris has dual standard of addresses and we want to be able to
	// parse both standards
	if !chaincfg.IsRegistered(&chaincfg.MainNetParams) {
		chaincfg.RegisterBitcoinParams()
	}
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

// PackTx packs transaction to byte array using protobuf
func (p *AlarisParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
	return p.baseparser.PackTx(tx, height, blockTime)
}

// UnpackTx unpacks transaction from protobuf byte array
func (p *AlarisParser) UnpackTx(buf []byte) (*bchain.Tx, uint32, error) {
	return p.baseparser.UnpackTx(buf)
}
