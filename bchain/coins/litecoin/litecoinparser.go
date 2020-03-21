package litecoin

import (
	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/trezor/blockbook/bchain/coins/btc"
)

// magic numbers
const (
	MainnetMagic wire.BitcoinNet = 0xdbb6c0fb
	TestnetMagic wire.BitcoinNet = 0xf1c8d2fd
	RegtestMagic wire.BitcoinNet = 0xdab5bffa
)

// chain parameters
var (
	MainNetParams chaincfg.Params
	TestNetParams chaincfg.Params
)

func init() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic
	MainNetParams.PubKeyHashAddrID = []byte{48}
	MainNetParams.ScriptHashAddrID = []byte{50}
	MainNetParams.Bech32HRPSegwit = "ltc"

	TestNetParams = chaincfg.TestNet3Params
	TestNetParams.Net = TestnetMagic
	TestNetParams.PubKeyHashAddrID = []byte{111}
	TestNetParams.ScriptHashAddrID = []byte{58}
	TestNetParams.Bech32HRPSegwit = "tltc"
}

// LitecoinParser handle
type LitecoinParser struct {
	*btc.BitcoinParser
}

// NewLitecoinParser returns new LitecoinParser instance
func NewLitecoinParser(params *chaincfg.Params, c *btc.Configuration) *LitecoinParser {
	return &LitecoinParser{BitcoinParser: btc.NewBitcoinParser(params, c)}
}

// GetChainParams contains network parameters for the main Litecoin network,
// and the test Litecoin network
func GetChainParams(chain string) *chaincfg.Params {
	// register bitcoin parameters in addition to litecoin parameters
	// litecoin has dual standard of addresses and we want to be able to
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
