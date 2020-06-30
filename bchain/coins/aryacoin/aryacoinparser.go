package aryacoin

import (
	"github.com/trezor/blockbook/bchain/coins/btc"

	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil/chaincfg"
)

// magic numbers
const (
	MainnetMagic wire.BitcoinNet = 0xe7eae188
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
	MainNetParams.PubKeyHashAddrID = []byte{23}
	MainNetParams.ScriptHashAddrID = []byte{5}
	MainNetParams.Bech32HRPSegwit = "arya"

	TestNetParams = chaincfg.TestNet3Params
	TestNetParams.Net = TestnetMagic
	TestNetParams.PubKeyHashAddrID = []byte{111}
	TestNetParams.ScriptHashAddrID = []byte{196}
	TestNetParams.Bech32HRPSegwit = "arya"
}

// AryaCoinParser handle
type AryaCoinParser struct {
	*btc.BitcoinParser
}

// NewAryaCoinParser returns new AryaCoinParser instance
func NewAryaCoinParser(params *chaincfg.Params, c *btc.Configuration) *AryaCoinParser {
	return &AryaCoinParser{BitcoinParser: btc.NewBitcoinParser(params, c)}
}

// GetChainParams contains network parameters for the main AryaCoin network,
// and the test AryaCoin network
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