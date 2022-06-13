package dash

import (
	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
)

const (
	// MainnetMagic is mainnet network constant
	MainnetMagic wire.BitcoinNet = 0xbd6b0cbf
	// TestnetMagic is testnet network constant
	TestnetMagic wire.BitcoinNet = 0xffcae2ce
	// RegtestMagic is regtest network constant
	RegtestMagic wire.BitcoinNet = 0xdcb7c1fc
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

// DashParser handle
type DashParser struct {
	*btc.BitcoinLikeParser
	baseparser *bchain.BaseParser
}

// NewDashParser returns new DashParser instance
func NewDashParser(params *chaincfg.Params, c *btc.Configuration) *DashParser {
	return &DashParser{
		BitcoinLikeParser: btc.NewBitcoinLikeParser(params, c),
		baseparser:        &bchain.BaseParser{},
	}
}

// GetChainParams contains network parameters for the main Dash network,
// the regression test Dash network, the test Dash network and
// the simulation test Dash network, in this order
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
func (p *DashParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
	return p.baseparser.PackTx(tx, height, blockTime)
}

// UnpackTx unpacks transaction from protobuf byte array
func (p *DashParser) UnpackTx(buf []byte) (*bchain.Tx, uint32, error) {
	return p.baseparser.UnpackTx(buf)
}
