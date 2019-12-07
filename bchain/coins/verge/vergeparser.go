package verge

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"

	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil/chaincfg"
)

const (
	// MainnetMagic is mainnet network constant
	MainnetMagic wire.BitcoinNet = 0xff7ea7f7
)

var (
	// MainNetParams are parser parameters for mainnet
	MainNetParams chaincfg.Params
)

func init() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic
	MainNetParams.PubKeyHashAddrID = []byte{30}
	MainNetParams.ScriptHashAddrID = []byte{33}
	MainNetParams.Bech32HRPSegwit = "xvg"
}

// VergeParser handle
type VergeParser struct {
	*btc.BitcoinParser
	baseparser *bchain.BaseParser
}

// NewVergeParser returns new VergeParser instance
func NewVergeParser(params *chaincfg.Params, c *btc.Configuration) *VergeParser {
	p := &VergeParser{
		BitcoinParser: btc.NewBitcoinParser(params, c),
		baseparser: &bchain.BaseParser{
			AmountDecimalPoint: 6,
		},
	}

	return p
}

// GetChainParams contains network parameters for the main Verge network
func GetChainParams(chain string) *chaincfg.Params {
	if !chaincfg.IsRegistered(&MainNetParams) {
		err := chaincfg.Register(&MainNetParams)
		if err != nil {
			panic(err)
		}
	}
	return &MainNetParams
}

// PackTx packs transaction to byte array using protobuf
func (p *VergeParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
	return p.baseparser.PackTx(tx, height, blockTime)
}

// UnpackTx unpacks transaction from protobuf byte array
func (p *VergeParser) UnpackTx(buf []byte) (*bchain.Tx, uint32, error) {
	return p.baseparser.UnpackTx(buf)
}
