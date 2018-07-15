package groestlcoin

import (
	"blockbook/bchain/coins/btc"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
)

const (
	// MainnetMagic wire.BitcoinNet = 0xd9b4bef9
	// TestnetMagic wire.BitcoinNet = 0x0709110b
	// Groestlcoin magic number are the same as in BTC, so we use fake ones to
	// avoid clash.  Blockbook does not seem to need real values here, just
	// unique.
	MainnetMagic wire.BitcoinNet = 0xd9b4bef8
	TestnetMagic wire.BitcoinNet = 0x0709110a
)

var (
	MainNetParams chaincfg.Params
	TestNetParams chaincfg.Params
)

func init() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic
	MainNetParams.PubKeyHashAddrID = 36
	MainNetParams.ScriptHashAddrID = 5
	MainNetParams.Bech32HRPSegwit = "grs"

	TestNetParams = chaincfg.TestNet3Params
	TestNetParams.Net = TestnetMagic
	TestNetParams.PubKeyHashAddrID = 111
	TestNetParams.ScriptHashAddrID = 196
	TestNetParams.Bech32HRPSegwit = "tgrs"

	err := chaincfg.Register(&MainNetParams)
	if err == nil {
		err = chaincfg.Register(&TestNetParams)
	}
	if err != nil {
		panic(err)
	}
}

// GroestlcoinParser handle
type GroestlcoinParser struct {
	*btc.BitcoinParser
}

// NewGroestlcoinParser returns new GroestlcoinParser instance
func NewGroestlcoinParser(params *chaincfg.Params, c *btc.Configuration) *GroestlcoinParser {
	return &GroestlcoinParser{BitcoinParser: btc.NewBitcoinParser(params, c)}
}

// GetChainParams contains network parameters for the main Groestlcoin network,
// and the test Groestlcoin network
func GetChainParams(chain string) *chaincfg.Params {
	switch chain {
	case "test":
		return &TestNetParams
	default:
		return &MainNetParams
	}
}
