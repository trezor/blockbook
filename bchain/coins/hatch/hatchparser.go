package hatch

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"

	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil/chaincfg"
)

const (
	// MainnetMagic is mainnet network constant
	MainnetMagic wire.BitcoinNet = 0x2fbc8a6d
	// TestnetMagic is testnet network constant
	TestnetMagic wire.BitcoinNet = 0xc59be37a
	// RegtestMagic is regtest network constant
	RegtestMagic wire.BitcoinNet = 0x3dd8f7a1
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
	MainNetParams.PubKeyHashAddrID = []byte{76} // base58 prefix: X
	MainNetParams.ScriptHashAddrID = []byte{16} // base58 prefix: 7

	TestNetParams = chaincfg.TestNet3Params
	TestNetParams.Net = TestnetMagic

	// Address encoding magics
	TestNetParams.PubKeyHashAddrID = []byte{140} // base58 prefix: y
	TestNetParams.ScriptHashAddrID = []byte{19}  // base58 prefix: 8 or 9

	RegtestParams = chaincfg.RegressionNetParams
	RegtestParams.Net = RegtestMagic

	// Address encoding magics
	RegtestParams.PubKeyHashAddrID = []byte{140} // base58 prefix: y
	RegtestParams.ScriptHashAddrID = []byte{19}  // base58 prefix: 8 or 9
}

// HatchParser handle
type HatchParser struct {
	*btc.BitcoinParser
	baseparser *bchain.BaseParser
}

// NewHatchParser returns new HatchParser instance
func NewHatchParser(params *chaincfg.Params, c *btc.Configuration) *HatchParser {
	return &HatchParser{
		BitcoinParser: btc.NewBitcoinParser(params, c),
		baseparser:    &bchain.BaseParser{},
	}
}

// GetChainParams contains network parameters for the main Hatch network,
// the regression test Hatch network, the test Hatch network and
// the simulation test Hatch network, in this order
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
func (p *HatchParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
	return p.baseparser.PackTx(tx, height, blockTime)
}

// UnpackTx unpacks transaction from protobuf byte array
func (p *HatchParser) UnpackTx(buf []byte) (*bchain.Tx, uint32, error) {
	return p.baseparser.UnpackTx(buf)
}
