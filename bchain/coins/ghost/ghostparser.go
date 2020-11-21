package ghost

import (
	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/martinboehm/btcutil/txscript"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
)

const (
	MainnetMagic wire.BitcoinNet = 0xD9B4BEF9
	TestnetMagic wire.BitcoinNet = 0xDAB5BFFA
)

var (
	MainNetParams chaincfg.Params
	TestNetParams chaincfg.Params
)

func init() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic

	TestNetParams = chaincfg.TestNet3Params
	TestNetParams.Net = TestnetMagic

	err := chaincfg.Register(&MainNetParams)
	if err != nil {
		panic(err)
	}
}

// BitcoreParser handle
type GhostParser struct {
	*btc.BitcoinParser
	baseparser *bchain.BaseParser
}

// NewBitcoreParser returns new BitcoreParser instance
func NewGhostParser(params *chaincfg.Params, c *btc.Configuration) *GhostParser {
	p := &GhostParser{
		BitcoinParser: btc.NewBitcoinParser(params, c),
		baseparser:    &bchain.BaseParser{},
	}
	p.OutputScriptToAddressesFunc = p.outputScriptToAddresses
	return p
}

// GetChainParams contains network parameters for the main Bitcore network,
// and the test Bitcore network
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

func (p *GhostParser) outputScriptToAddresses(script []byte) ([]string, bool, error) {
	sc, addresses, _, err := txscript.ExtractPkScriptAddrs(script, p.Params)
	if err != nil {
		return nil, false, err
	}
	rv := make([]string, len(addresses))
	for i, a := range addresses {
		rv[i] = a.EncodeAddress()
	}
	var s bool
	if sc == txscript.PubKeyHashTy || sc == txscript.WitnessV0PubKeyHashTy || sc == txscript.ScriptHashTy || sc == txscript.WitnessV0ScriptHashTy {
		s = true
	} else if len(rv) == 0 {
		or := p.TryParseOPReturn(script)
		if or != "" {
			rv = []string{or}
		}
	}
	return rv, s, nil
}