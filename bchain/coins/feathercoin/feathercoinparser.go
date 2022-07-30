package feathercoin

import (
	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
)

// magic numbers
const (
	MainnetMagic wire.BitcoinNet = 0x211a1541
)

// chain parameters
var (
	MainNetParams chaincfg.Params
)

func init() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic
	MainNetParams.PubKeyHashAddrID = []byte{14}
	MainNetParams.ScriptHashAddrID = []byte{5}
	MainNetParams.Bech32HRPSegwit = "fc"
}

// FeathercoinParser handle
type FeathercoinParser struct {
	*btc.BitcoinLikeParser
	baseparser *bchain.BaseParser
}

// NewFeathercoinParser returns new FeathercoinParser instance
func NewFeathercoinParser(params *chaincfg.Params, c *btc.Configuration) *FeathercoinParser {
	return &FeathercoinParser{
		BitcoinLikeParser: btc.NewBitcoinLikeParser(params, c),
		baseparser:        &bchain.BaseParser{},
	}
}

// GetChainParams contains network parameters for the main Feathercoin network,
// and the test Feathercoin network
func GetChainParams(chain string) *chaincfg.Params {
	if !chaincfg.IsRegistered(&chaincfg.MainNetParams) {
		chaincfg.RegisterBitcoinParams()
	}
	if !chaincfg.IsRegistered(&MainNetParams) {
		err := chaincfg.Register(&MainNetParams)
		if err != nil {
			panic(err)
		}
	}
	return &MainNetParams
}

// PackTx packs transaction to byte array using protobuf
func (p *FeathercoinParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
	return p.baseparser.PackTx(tx, height, blockTime)
}

// UnpackTx unpacks transaction from protobuf byte array
func (p *FeathercoinParser) UnpackTx(buf []byte) (*bchain.Tx, uint32, error) {
	return p.baseparser.UnpackTx(buf)
}
