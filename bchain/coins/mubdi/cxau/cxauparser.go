package cxau

import (
	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
)

const (
	// MainnetMagic is mainnet network constant
	MainnetMagic wire.BitcoinNet = 0x010a707c
	// TestnetMagic is testnet network constant
	TestnetMagic wire.BitcoinNet = 0x01b35ec4
	// RegtestMagic is regtest network constant
	RegtestMagic wire.BitcoinNet = 0x01139b4b
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
	MainNetParams.PubKeyHashAddrID = []byte{88} // base58 prefix: C
	MainNetParams.ScriptHashAddrID = []byte{23} // base58 prefix: A

	TestNetParams = chaincfg.TestNet3Params
	TestNetParams.Net = TestnetMagic

	// Address encoding magics
	TestNetParams.PubKeyHashAddrID = []byte{90} // base58 prefix: d
	TestNetParams.ScriptHashAddrID = []byte{41}  // base58 prefix: H

	RegtestParams = chaincfg.RegressionNetParams
	RegtestParams.Net = RegtestMagic

	// Address encoding magics
	RegtestParams.PubKeyHashAddrID = []byte{90} // base58 prefix: d
	RegtestParams.ScriptHashAddrID = []byte{41}  // base58 prefix: H
}

// cXAUParser handle
type cXAUParser struct {
	*btc.BitcoinLikeParser
	baseparser *bchain.BaseParser
}

// NewcXAUParser returns new cXAUParser instance
func NewcXAUParser(params *chaincfg.Params, c *btc.Configuration) *cXAUParser {
	return &cXAUParser{
		BitcoinLikeParser: btc.NewBitcoinLikeParser(params, c),
		baseparser:        &bchain.BaseParser{},
	}
}

// GetChainParams contains network parameters for the main MuBdI network,
// the regression test MuBdI network, the test MuBdI network and
// the simulation test MuBdI network, in this order
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
func (p *cXAUParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
	return p.baseparser.PackTx(tx, height, blockTime)
}

// UnpackTx unpacks transaction from protobuf byte array
func (p *cXAUParser) UnpackTx(buf []byte) (*bchain.Tx, uint32, error) {
	return p.baseparser.UnpackTx(buf)
}
