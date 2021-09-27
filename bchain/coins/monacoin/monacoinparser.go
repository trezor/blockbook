package monacoin

import (
	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/trezor/blockbook/bchain/coins/btc"
)

// magic numbers
const (
	MainnetMagic wire.BitcoinNet = 0xdbb6c0fb
	TestnetMagic wire.BitcoinNet = 0xf1c8d2fd
)

// chain parameters
var (
	MainNetParams chaincfg.Params
	TestNetParams chaincfg.Params
)

func init() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic
	MainNetParams.PubKeyHashAddrID = []byte{50}
	MainNetParams.ScriptHashAddrID = []byte{55}
	MainNetParams.Bech32HRPSegwit = "mona"

	TestNetParams = chaincfg.TestNet3Params
	TestNetParams.Net = TestnetMagic
	TestNetParams.PubKeyHashAddrID = []byte{111}
	TestNetParams.ScriptHashAddrID = []byte{117}
	TestNetParams.Bech32HRPSegwit = "tmona"
}

// MonacoinParser handle
type MonacoinParser struct {
	*btc.BitcoinLikeParser
}

// NewMonacoinParser returns new MonacoinParser instance
func NewMonacoinParser(params *chaincfg.Params, c *btc.Configuration) *MonacoinParser {
	return &MonacoinParser{BitcoinLikeParser: btc.NewBitcoinLikeParser(params, c)}
}

// GetChainParams contains network parameters for the main Monacoin network,
// and the test Monacoin network
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
