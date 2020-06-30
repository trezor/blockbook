package kmd

import (
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"

	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil/chaincfg"
)

const (
	// MainnetMagic is mainnet network constant
	MainnetMagic wire.BitcoinNet = 0x8de4eef9
	// TestnetMagic is testnet network constant
	TestnetMagic wire.BitcoinNet = 0x627e1f5a
	// RegtestMagic is regtest network constant
	RegtestMagic wire.BitcoinNet = 0xf5f38eaa
)

var (
	// MainNetParams are parser parameters for mainnet
	MainNetParams chaincfg.Params
	// TestNetParams are parser parameters for testnet
	TestNetParams chaincfg.Params
	// RegtestParams are parser parameters for regtest
	RegtestParams chaincfg.Params
)

func init() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic

	// Address encoding magics

	MainNetParams.PubKeyHashAddrID = []byte{0x3C}
	MainNetParams.ScriptHashAddrID = []byte{0x55}

	TestNetParams = chaincfg.TestNet3Params
	TestNetParams.Net = TestnetMagic

	// Address encoding magics
	TestNetParams.PubKeyHashAddrID = []byte{0x0} // base58 prefix: tm
	TestNetParams.ScriptHashAddrID = []byte{0x5} // base58 prefix: t2

	RegtestParams = chaincfg.RegressionNetParams
	RegtestParams.Net = RegtestMagic
}

// KmdParser handle
type KmdParser struct {
	*btc.BitcoinParser
	baseparser *bchain.BaseParser
}

// NewKmdParser returns new KmdParser instance
func NewKmdParser(params *chaincfg.Params, c *btc.Configuration) *KmdParser {
	return &KmdParser{
		BitcoinParser: btc.NewBitcoinParser(params, c),
		baseparser:    &bchain.BaseParser{},
	}
}

// GetChainParams contains network parameters for the main Kmd network,
// the regression test Kmd network, the test Kmd network and
// the simulation test Kmd network, in this order
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

// PackTx packs transaction to byte array using protobuf
func (p *KmdParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
	return p.baseparser.PackTx(tx, height, blockTime)
}

// UnpackTx unpacks transaction from protobuf byte array
func (p *KmdParser) UnpackTx(buf []byte) (*bchain.Tx, uint32, error) {
	return p.baseparser.UnpackTx(buf)
}