package creamcoin

import (
	"github.com/trezor/blockbook/bchain/coins/btc"

	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil/chaincfg"
)

// magic numbers
const (
	MainnetMagic wire.BitcoinNet = 0xeeef2cc1
	TestnetMagic wire.BitcoinNet = 0x0709110b
)

// chain parameters
var (
	MainNetParams chaincfg.Params
	TestNetParams chaincfg.Params
)

func init() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic
	MainNetParams.PubKeyHashAddrID = []byte{28}
	MainNetParams.ScriptHashAddrID = []byte{6}
	MainNetParams.Bech32HRPSegwit = "crm"

	TestNetParams = chaincfg.TestNet3Params
	TestNetParams.Net = TestnetMagic
	TestNetParams.PubKeyHashAddrID = []byte{199}
	TestNetParams.ScriptHashAddrID = []byte{198}
	TestNetParams.Bech32HRPSegwit = "crm"
}

// CreamCoinParser handle
type CreamCoinParser struct {
	*btc.BitcoinParser
}

// NewCreamCoinParser returns new CreamCoinParser instance
func NewCreamCoinParser(params *chaincfg.Params, c *btc.Configuration) *CreamCoinParser {
	return &CreamCoinParser{BitcoinParser: btc.NewBitcoinParser(params, c)}
}

// GetChainParams contains network parameters for the main CreamCoin network,
// and the test CreamCoin network
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