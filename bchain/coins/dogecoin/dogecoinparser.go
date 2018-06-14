package dogecoin

import (
	"blockbook/bchain/coins/btc"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
)

const (
	MainnetMagic wire.BitcoinNet = 0xc0c0c0c0
)

var (
	MainNetParams chaincfg.Params
)

func init() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic
	MainNetParams.PubKeyHashAddrID = 30
	MainNetParams.ScriptHashAddrID = 22

	err := chaincfg.Register(&MainNetParams)
	if err != nil {
		panic(err)
	}
}

// DogecoinParser handle
type DogecoinParser struct {
	*btc.BitcoinParser
}

// NewDogecoinParser returns new DogecoinParser instance
func NewDogecoinParser(params *chaincfg.Params, c *btc.Configuration) *DogecoinParser {
	return &DogecoinParser{BitcoinParser: btc.NewBitcoinParser(params, c)}
}

// GetChainParams contains network parameters for the main Dogecoin network,
// and the test Dogecoin network
func GetChainParams(chain string) *chaincfg.Params {
	switch chain {
	default:
		return &MainNetParams
	}
}
