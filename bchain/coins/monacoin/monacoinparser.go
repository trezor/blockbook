package monacoin

import (
	"blockbook/bchain/coins/btc"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
	monacoinCfg "github.com/wakiyamap/monad/chaincfg"
	"github.com/wakiyamap/monad/txscript"
	monacoinWire "github.com/wakiyamap/monad/wire"
	"github.com/wakiyamap/monautil"
)

const (
	MainnetMagic  wire.BitcoinNet         = 0x39393939 //dummy. Correct value is 0xdbb6c0fb
	TestnetMagic  wire.BitcoinNet         = 0x69696969 //dummy. Correct value is 0xf1c8d2fd
	MonaMainMagic monacoinWire.BitcoinNet = 0xdbb6c0fb
	MonaTestMagic monacoinWire.BitcoinNet = 0xf1c8d2fd
)

var (
	MainNetParams  chaincfg.Params
	TestNetParams  chaincfg.Params
	MonaMainParams monacoinCfg.Params
	MonaTestParams monacoinCfg.Params
)

func init() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic
	MainNetParams.PubKeyHashAddrID = 50
	MainNetParams.ScriptHashAddrID = 55
	MainNetParams.Bech32HRPSegwit = "mona"
	MonaMainParams = monacoinCfg.MainNetParams
	MonaMainParams.Net = MonaMainMagic
	MonaMainParams.PubKeyHashAddrID = 50
	MonaMainParams.ScriptHashAddrID = 55
	MonaMainParams.Bech32HRPSegwit = "mona"

	TestNetParams = chaincfg.TestNet3Params
	TestNetParams.Net = TestnetMagic
	TestNetParams.PubKeyHashAddrID = 111
	TestNetParams.ScriptHashAddrID = 117
	TestNetParams.Bech32HRPSegwit = "tmona"
	MonaTestParams = monacoinCfg.TestNet4Params
	MonaTestParams.Net = MonaTestMagic
	MonaTestParams.PubKeyHashAddrID = 111
	MonaTestParams.ScriptHashAddrID = 117
	MonaTestParams.Bech32HRPSegwit = "tmona"

	err := chaincfg.Register(&MainNetParams)
	if err == nil {
		err = chaincfg.Register(&TestNetParams)
	}
	if err != nil {
		panic(err)
	}
}

// MonacoinParser handle
type MonacoinParser struct {
	*btc.BitcoinParser
}

// NewMonacoinParser returns new MonacoinParser instance
func NewMonacoinParser(params *chaincfg.Params, c *btc.Configuration) *MonacoinParser {
	return &MonacoinParser{BitcoinParser: btc.NewBitcoinParser(params, c)}
}

// GetChainParams contains network parameters for the main Monacoin network,
// and the test Monacoin network
func GetChainParams(chain string) *chaincfg.Params {
	switch chain {
	case "test":
		return &TestNetParams
	default:
		return &MainNetParams
	}
}

// GetMonaChainParams contains network parameters for the main Monacoin network,
// and the test Monacoin network
func GetMonaChainParams(chain string) *monacoinCfg.Params {
	switch chain {
	case "test":
		return &MonaTestParams
	default:
		return &MonaMainParams
	}
}

// GetAddrIDFromAddress returns internal address representation of given address
func (p *MonacoinParser) GetAddrIDFromAddress(address string) ([]byte, error) {
	return p.addressToOutputScript(address)
}

// addressToOutputScript converts monacoin address to ScriptPubKey
func (p *MonacoinParser) addressToOutputScript(address string) ([]byte, error) {
	switch p.Params.Net {
	case MainnetMagic:
		da, err := monautil.DecodeAddress(address, &MonaMainParams)
		if err != nil {
			return nil, err
		}
		script, err := txscript.PayToAddrScript(da)
		if err != nil {
			return nil, err
		}
		return script, nil
	default:
		da, err := monautil.DecodeAddress(address, &MonaTestParams)
		if err != nil {
			return nil, err
		}
		script, err := txscript.PayToAddrScript(da)
		if err != nil {
			return nil, err
		}
		return script, nil
	}
}
