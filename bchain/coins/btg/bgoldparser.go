package bch

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"fmt"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcutil"
	"github.com/cpacia/bchutil"
	"github.com/schancel/cashaddr-converter/address"
)

const (
	MainnetMagic wire.BitcoinNet = 0x446d47e1
	TestnetMagic wire.BitcoinNet = 0x456e48e2
	RegtestMagic wire.BitcoinNet = 0xdab5bffa
)

// BGoldParser handle
type BGoldParser struct {
	*btc.BitcoinParser
}

// NewBCashParser returns new BGoldParser instance
func NewBGoldParser(params *chaincfg.Params, c *btc.Configuration) *BGoldParser {
	return BGoldParser{BitcoinParser: btc.NewBitcoinParser(params, c)}
}

// GetChainParams contains network parameters for the main Bitcoin Cash network,
// the regression test Bitcoin Cash network, the test Bitcoin Cash network and
// the simulation test Bitcoin Cash network, in this order
func GetChainParams(chain string) *chaincfg.Params {
	var params *chaincfg.Params
	switch chain {
	case "test":
		params = &chaincfg.TestNet3Params
		params.Net = TestnetMagic
	case "regtest":
		params = &chaincfg.RegressionNetParams
		params.Net = Regtestmagic
	default:
		params = &chaincfg.MainNetParams
		params.Net = MainnetMagic
	}

	return params
}
