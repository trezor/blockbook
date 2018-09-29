package monacoin

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"

	"github.com/btcsuite/btcd/wire"
	"github.com/jakm/btcutil/chaincfg"
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
	MainNetParams.PubKeyHashAddrID = []byte{50}
	MainNetParams.ScriptHashAddrID = []byte{55}
	MainNetParams.Bech32HRPSegwit = "mona"
	MonaMainParams = monacoinCfg.MainNetParams
	MonaMainParams.Net = MonaMainMagic
	MonaMainParams.PubKeyHashAddrID = 50
	MonaMainParams.ScriptHashAddrID = 55
	MonaMainParams.Bech32HRPSegwit = "mona"

	TestNetParams = chaincfg.TestNet3Params
	TestNetParams.Net = TestnetMagic
	TestNetParams.PubKeyHashAddrID = []byte{111}
	TestNetParams.ScriptHashAddrID = []byte{117}
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
	p := &MonacoinParser{BitcoinParser: btc.NewBitcoinParser(params, c)}
	p.OutputScriptToAddressesFunc = p.outputScriptToAddresses
	return p
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

// GetAddrDescFromAddress returns internal address representation (descriptor) of given address
func (p *MonacoinParser) GetAddrDescFromAddress(address string) (bchain.AddressDescriptor, error) {
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

// GetAddressesFromAddrDesc returns addresses for given address descriptor with flag if the addresses are searchable
func (p *MonacoinParser) GetAddressesFromAddrDesc(addrDesc bchain.AddressDescriptor) ([]string, bool, error) {
	return p.OutputScriptToAddressesFunc(addrDesc)
}

// outputScriptToAddresses converts ScriptPubKey to bitcoin addresses
func (p *MonacoinParser) outputScriptToAddresses(script []byte) ([]string, bool, error) {
	switch p.Params.Net {
	case MainnetMagic:
		sc, addresses, _, err := txscript.ExtractPkScriptAddrs(script, &MonaMainParams)
		if err != nil {
			return nil, false, err
		}
		rv := make([]string, len(addresses))
		for i, a := range addresses {
			rv[i] = a.EncodeAddress()
		}
		var s bool
		if sc != txscript.NonStandardTy && sc != txscript.NullDataTy {
			s = true
		} else if len(rv) == 0 {
			or := btc.TryParseOPReturn(script)
			if or != "" {
				rv = []string{or}
			}
		}
		return rv, s, nil
	default:
		sc, addresses, _, err := txscript.ExtractPkScriptAddrs(script, &MonaTestParams)
		if err != nil {
			return nil, false, err
		}
		rv := make([]string, len(addresses))
		for i, a := range addresses {
			rv[i] = a.EncodeAddress()
		}
		var s bool
		if sc != txscript.NonStandardTy && sc != txscript.NullDataTy {
			s = true
		} else if len(rv) == 0 {
			or := btc.TryParseOPReturn(script)
			if or != "" {
				rv = []string{or}
			}
		}
		return rv, s, nil
	}
}
