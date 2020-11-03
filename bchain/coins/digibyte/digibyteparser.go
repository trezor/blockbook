package digibyte

import (
	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/trezor/blockbook/bchain/coins/btc"
)

const (
	// MainnetMagic is mainnet network constant
	MainnetMagic wire.BitcoinNet = 0xdab6c3fa
	TestnetMagic wire.BitcoinNet = 0xddbdc8fd
)

var (
	// MainNetParams are parser parameters for mainnet
	MainNetParams chaincfg.Params
	TestNetParams chaincfg.Params
)

func init() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic
	MainNetParams.PubKeyHashAddrID = []byte{30}
	MainNetParams.ScriptHashAddrID = []byte{63}
	MainNetParams.Bech32HRPSegwit = "dgb"

	TestNetParams = chaincfg.TestNet3Params
	TestNetParams.Net = TestnetMagic
	TestNetParams.PubKeyHashAddrID = []byte{126}
	TestNetParams.ScriptHashAddrID = []byte{140}
	TestNetParams.Bech32HRPSegwit = "dgbt"
}

// DigiByteParser handle
type DigiByteParser struct {
	*btc.BitcoinParser
}

// NewDigiByteParser returns new DigiByteParser instance
func NewDigiByteParser(params *chaincfg.Params, c *btc.Configuration) *DigiByteParser {
	return &DigiByteParser{BitcoinParser: btc.NewBitcoinParser(params, c)}
}

// GetChainParams contains network parameters for the main DigiByte network
// and the DigiByte Testnet network
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
