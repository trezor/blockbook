package fujicoin

import (
	"blockbook/bchain/coins/btc"

	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil/chaincfg"
)

const (
	MainnetMagic wire.BitcoinNet = 0x696a7566
	TestnetMagic wire.BitcoinNet = 0x66756a69
	RegtestMagic wire.BitcoinNet = 0x66756a69
)

var (
	MainNetParams chaincfg.Params
	TestNetParams chaincfg.Params
)

func init() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic
	MainNetParams.PubKeyHashAddrID = []byte{36}
	MainNetParams.ScriptHashAddrID = []byte{16}
	MainNetParams.Bech32HRPSegwit = "fc"

	TestNetParams = chaincfg.TestNet3Params
	TestNetParams.Net = TestnetMagic
	TestNetParams.PubKeyHashAddrID = []byte{74}
	TestNetParams.ScriptHashAddrID = []byte{196}
	TestNetParams.Bech32HRPSegwit = "tfc"
}

// FujicoinParser handle
type FujicoinParser struct {
	*btc.BitcoinParser
}

// NewFujicoinParser returns new FujicoinParser instance
func NewFujicoinParser(params *chaincfg.Params, c *btc.Configuration) *FujicoinParser {
	return &FujicoinParser{BitcoinParser: btc.NewBitcoinParser(params, c)}
}

// GetChainParams contains network parameters for the main Fujicoin network,
// and the test Fujicoin network
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
