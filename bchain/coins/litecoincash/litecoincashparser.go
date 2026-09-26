package litecoincash

import (
	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/trezor/blockbook/bchain/coins/btc"
)

const (
	MainnetMagic wire.BitcoinNet = 0xf8bae4c7
	TestnetMagic wire.BitcoinNet = 0xcfd3f5b6
	RegtestMagic wire.BitcoinNet = 0xdab5bffa
)

var (
	MainNetParams chaincfg.Params
	TestNetParams chaincfg.Params
	RegtestParams chaincfg.Params
)

func init() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic
	MainNetParams.PubKeyHashAddrID = []byte{28}
	MainNetParams.ScriptHashAddrID = []byte{50}
	MainNetParams.Bech32HRPSegwit = "lcc"

	TestNetParams = chaincfg.TestNet3Params
	TestNetParams.Net = TestnetMagic
	TestNetParams.PubKeyHashAddrID = []byte{127}
	TestNetParams.ScriptHashAddrID = []byte{58}
	TestNetParams.Bech32HRPSegwit = "tlcc"

	RegtestParams = chaincfg.RegressionNetParams
	RegtestParams.Net = RegtestMagic
	RegtestParams.PubKeyHashAddrID = []byte{111}
	RegtestParams.ScriptHashAddrID = []byte{58}
	RegtestParams.Bech32HRPSegwit = "rlcc"
}

type LitecoinCashParser struct {
	*btc.BitcoinLikeParser
}

func NewLitecoinCashParser(params *chaincfg.Params, c *btc.Configuration) *LitecoinCashParser {
	p := &LitecoinCashParser{
		BitcoinLikeParser: btc.NewBitcoinLikeParser(params, c),
	}
	p.AmountDecimalPoint = 7
	return p
}

func GetChainParams(chain string) *chaincfg.Params {
	if !chaincfg.IsRegistered(&MainNetParams) {
		err := chaincfg.Register(&MainNetParams)
		if err == nil {
			err = chaincfg.Register(&TestNetParams)
		}
		if err == nil {
			err = chaincfg.Register(&RegtestParams)
		}
		if err != nil {
			panic(err)
		}
	}

	switch chain {
	case "test":
		return &TestNetParams
	case "regtest":
		return &RegtestParams
	default:
		return &MainNetParams
	}
}
