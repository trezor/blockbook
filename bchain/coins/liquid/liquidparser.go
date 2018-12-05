package liquid

import (
	"blockbook/bchain/coins/btc"

	"github.com/btcsuite/btcd/wire"
	"github.com/jakm/btcutil/chaincfg"
)

const (
	MainnetMagic wire.BitcoinNet = 0xdab5bffa
)

var (
	MainNetParams chaincfg.Params
)

func init() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic
	MainNetParams.PubKeyHashAddrID = []byte{57}
	MainNetParams.ScriptHashAddrID = []byte{39}
	// BLINDED_ADDRESS 12
}

// LiquidParser handle
type LiquidParser struct {
	*btc.BitcoinParser
}

// NewLiquidParser returns new LiquidParser instance
func NewLiquidParser(params *chaincfg.Params, c *btc.Configuration) *LiquidParser {
	return &LiquidParser{BitcoinParser: btc.NewBitcoinParser(params, c)}
}

// GetChainParams contains network parameters for the main GameCredits network,
// and the test GameCredits network
func GetChainParams(chain string) *chaincfg.Params {
	if !chaincfg.IsRegistered(&MainNetParams) {
		err := chaincfg.Register(&MainNetParams)
		if err != nil {
			panic(err)
		}
	}
	switch chain {
	default:
		return &MainNetParams
	}
}
