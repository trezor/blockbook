package vertcoin

import (
	"blockbook/bchain/coins/btc"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
)

const (
	MainnetMagic wire.BitcoinNet = 0xdab5bffb
	TestnetMagic wire.BitcoinNet = 0x74726576 // "vert" word
	RegtestMagic wire.BitcoinNet = 0xdab5bffc
)

var (
	MainNetParams chaincfg.Params
	TestNetParams chaincfg.Params
)

func init() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic
	MainNetParams.PubKeyHashAddrID = 71
	MainNetParams.ScriptHashAddrID = 5
	MainNetParams.Bech32HRPSegwit = "vtc"

	TestNetParams = chaincfg.TestNet3Params
	TestNetParams.Net = TestnetMagic
	TestNetParams.PubKeyHashAddrID = 74
	TestNetParams.ScriptHashAddrID = 196
	TestNetParams.Bech32HRPSegwit = "tvtc"

	err := chaincfg.Register(&MainNetParams)
	if err == nil {
		err = chaincfg.Register(&TestNetParams)
	}
	if err != nil {
		panic(err)
	}
}

// VertcoinParser handle
type VertcoinParser struct {
	*btc.BitcoinParser
}

// NewVertcoinParser returns new VertcoinParser instance
func NewVertcoinParser(params *chaincfg.Params, c *btc.Configuration) *VertcoinParser {
	return &VertcoinParser{BitcoinParser: btc.NewBitcoinParser(params, c)}
}

// GetChainParams contains network parameters for the main Vertcoin network,
// and the test Vertcoin network
func GetChainParams(chain string) *chaincfg.Params {
	switch chain {
	case "test":
		return &TestNetParams
	default:
		return &MainNetParams
	}
}
