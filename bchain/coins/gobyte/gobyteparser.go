package gobyte

import (
	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
)

const (
	// MainnetMagic is mainnet network constant
	MainnetMagic wire.BitcoinNet = 0x1ab2c3d4
	// TestnetMagic is testnet network constant
	TestnetMagic wire.BitcoinNet = 0xd12bb37a
	// RegtestMagic is regtest network constant
	RegtestMagic wire.BitcoinNet = 0xa1b3d57b
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
	MainNetParams.PubKeyHashAddrID = []byte{38} // base58 prefix: G
	MainNetParams.ScriptHashAddrID = []byte{10} // base58 prefix: 5

	TestNetParams = chaincfg.TestNet3Params
	TestNetParams.Net = TestnetMagic

	// Address encoding magics
	TestNetParams.PubKeyHashAddrID = []byte{112} // base58 prefix: n
	TestNetParams.ScriptHashAddrID = []byte{20}  // base58 prefix: 9

	RegtestParams = chaincfg.RegressionNetParams
	RegtestParams.Net = RegtestMagic

	// Address encoding magics
	RegtestParams.PubKeyHashAddrID = []byte{112} // base58 prefix: n
	RegtestParams.ScriptHashAddrID = []byte{20}  // base58 prefix: 9
}

// GoByteParser handle
type GoByteParser struct {
	*btc.BitcoinLikeParser
	baseparser *bchain.BaseParser
}

// NewGoByteParser returns new GoByteParser instance
func NewGoByteParser(params *chaincfg.Params, c *btc.Configuration) *GoByteParser {
	return &GoByteParser{
		BitcoinLikeParser: btc.NewBitcoinLikeParser(params, c),
		baseparser:        &bchain.BaseParser{},
	}
}

// GetChainParams contains network parameters for the main GoByte network,
// the regression test GoByte network, the test GoByte network and
// the simulation test GoByte network, in this order
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
func (p *GoByteParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
	return p.baseparser.PackTx(tx, height, blockTime)
}

// UnpackTx unpacks transaction from protobuf byte array
func (p *GoByteParser) UnpackTx(buf []byte) (*bchain.Tx, uint32, error) {
	return p.baseparser.UnpackTx(buf)
}
