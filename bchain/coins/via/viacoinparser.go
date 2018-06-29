package viacoinPubke

import (
	"blockbook/bchain/coins/btc"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
)

const (
	MainnetMagic wire.BitcoinNet = 0xcbc6680f
	TestnetMagic wire.BitcoinNet = 0x92efc5a9
	RegtestMagic wire.BitcoinNet = 0x377b972d
)

var (
	MainNetParams chaincfg.Params
	TestNetParams chaincfg.Params
)

func init() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic
	MainNetParams.PubKeyHashAddrID = 71
	MainNetParams.ScriptHashAddrID = 33
	MainNetParams.Bech32HRPSegwit = "via"

	TestNetParams = chaincfg.TestNet3Params
	TestNetParams.Net = TestnetMagic
	TestNetParams.PubKeyHashAddrID = 127
	TestNetParams.ScriptHashAddrID = 196
	TestNetParams.Bech32HRPSegwit = "tvia"

	err := chaincfg.Register(&MainNetParams)
	if err == nil {
		err = chaincfg.Register(&TestNetParams)
	}
	if err != nil {
		panic(err)
	}
}

// ViacoinParser handle
type ViacoinParser struct {
	*btc.BitcoinParser
}

// NewViacoinParser returns new ViacoinParser instance
func NewViacoinParser(params *chaincfg.Params, c *btc.Configuration) *ViacoinParser {
	return &ViacoinParser{BitcoinParser: btc.NewBitcoinParser(params, c)}
}

// GetChainParams contains network parameters for the main Viacoin network,
// and the test Viacoin network
func GetChainParams(chain string) *chaincfg.Params {
	switch chain {
	case "test":
		return &TestNetParams
	default:
		return &MainNetParams
	}
}
