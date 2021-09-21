package zec

import (
	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
)

const (
	// MainnetMagic is mainnet network constant
	MainnetMagic wire.BitcoinNet = 0x6427e924
	// TestnetMagic is testnet network constant
	TestnetMagic wire.BitcoinNet = 0xbff91afa
	// RegtestMagic is regtest network constant
	RegtestMagic wire.BitcoinNet = 0x5f3fe8aa
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
	MainNetParams.AddressMagicLen = 2
	MainNetParams.PubKeyHashAddrID = []byte{0x1C, 0xB8} // base58 prefix: t1
	MainNetParams.ScriptHashAddrID = []byte{0x1C, 0xBD} // base58 prefix: t3

	TestNetParams = chaincfg.TestNet3Params
	TestNetParams.Net = TestnetMagic

	// Address encoding magics
	TestNetParams.AddressMagicLen = 2
	TestNetParams.PubKeyHashAddrID = []byte{0x1D, 0x25} // base58 prefix: tm
	TestNetParams.ScriptHashAddrID = []byte{0x1C, 0xBA} // base58 prefix: t2

	RegtestParams = chaincfg.RegressionNetParams
	RegtestParams.Net = RegtestMagic
}

// ZCashParser handle
type ZCashParser struct {
	*btc.BitcoinLikeParser
	baseparser *bchain.BaseParser
}

// NewZCashParser returns new ZCashParser instance
func NewZCashParser(params *chaincfg.Params, c *btc.Configuration) *ZCashParser {
	return &ZCashParser{
		BitcoinLikeParser: btc.NewBitcoinLikeParser(params, c),
		baseparser:        &bchain.BaseParser{},
	}
}

// GetChainParams contains network parameters for the main ZCash network,
// the regression test ZCash network, the test ZCash network and
// the simulation test ZCash network, in this order
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
func (p *ZCashParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
	return p.baseparser.PackTx(tx, height, blockTime)
}

// UnpackTx unpacks transaction from protobuf byte array
func (p *ZCashParser) UnpackTx(buf []byte) (*bchain.Tx, uint32, error) {
	return p.baseparser.UnpackTx(buf)
}
