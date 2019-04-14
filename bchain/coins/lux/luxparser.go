package lux

 import (
	"blockbook/bchain/coins/btc"
        "github.com/martinboehm/btcd/wire"
        "github.com/martinboehm/btcutil/chaincfg"
)

 const (
	MainnetMagic wire.BitcoinNet = 0xa9c8b36a
	TestnetMagic wire.BitcoinNet = 0xab516754
)

 var (
	MainNetParams chaincfg.Params
	TestNetParams chaincfg.Params
)

 func init() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic
	MainNetParams.PubKeyHashAddrID = []byte{48}
	MainNetParams.ScriptHashAddrID = []byte{63}
	MainNetParams.Bech32HRPSegwit = "bc"

 	TestNetParams = chaincfg.TestNet3Params
	TestNetParams.Net = TestnetMagic
	TestNetParams.PubKeyHashAddrID = []byte{48}
	TestNetParams.ScriptHashAddrID = []byte{63}
	TestNetParams.Bech32HRPSegwit = "bc"
}

 // LuxParser handle
type LuxParser struct {
	*btc.BitcoinParser
}

 // NewLuxParser returns new DashParser instance
func NewLuxParser(params *chaincfg.Params, c *btc.Configuration) *LuxParser {
	return &LuxParser{
		BitcoinParser: btc.NewBitcoinParser(params, c),
	}
}

 // GetChainParams contains network parameters for the main Lux network,
// the regression test Lux network, the test Lux network and
// the simulation test Lux network, in this order
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