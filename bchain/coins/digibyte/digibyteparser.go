package digibyte

import (
	"blockbook/bchain/coins/btc"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
)

const (
	MainnetMagic wire.BitcoinNet = 0xdab6c3fa
	TestnetMagic wire.BitcoinNet = 0xdab6c3fa 
	RegtestMagic wire.BitcoinNet = 0xdab5bffc
)

var (
	MainNetParams chaincfg.Params
	TestNetParams chaincfg.Params
)

func init() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic
	MainNetParams.PubKeyHashAddrID = 30
	MainNetParams.ScriptHashAddrID = 5
	MainNetParams.Bech32HRPSegwit = "dgb"

	TestNetParams = chaincfg.TestNet3Params
	TestNetParams.Net = TestnetMagic
	// ToDo: Check whether there is a dgb testnet and update the pubkeyhash and scripthash
	TestNetParams.PubKeyHashAddrID = 74 
	TestNetParams.ScriptHashAddrID = 196
	TestNetParams.Bech32HRPSegwit = "tdgb"

	err := chaincfg.Register(&MainNetParams)
	if err == nil {
		err = chaincfg.Register(&TestNetParams)
	}
	if err != nil {
		panic(err)
	}
}

// DigibyteParser handle
type DigiByteParser struct {
	*btc.BitcoinParser
}

// NewDigiByteParser returns new VertcoinParser instance
func NewDigiByteParser(params *chaincfg.Params, c *btc.Configuration) *DigiByteParser {
	return &DigiByteParser{BitcoinParser: btc.NewBitcoinParser(params, c)}
}

// GetChainParams contains network parameters for the main DigiByte network
func GetChainParams(chain string) *chaincfg.Params {
		return &MainNetParams
	}
