package satoxcoin

import (
	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
)

// magic numbers
const (
	MainnetMagic wire.BitcoinNet = 0x53415458 // S A T X (unique for Satoxcoin)
)

// chain parameters
var (
	MainNetParams chaincfg.Params
)

func init() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic
	MainNetParams.PubKeyHashAddrID = []byte{63} // 'S' prefix
	MainNetParams.ScriptHashAddrID = []byte{122}
	MainNetParams.HDCoinType = 9007 // SLIP44 for Satoxcoin
}

// SatoxcoinParser represents Satoxcoin parser.
type SatoxcoinParser struct {
	*btc.BitcoinLikeParser
	baseparser *bchain.BaseParser
}

// NewSatoxcoinParser returns new SatoxcoinParser instance.
func NewSatoxcoinParser(params *chaincfg.Params, c *btc.Configuration) *SatoxcoinParser {
	return &SatoxcoinParser{
		BitcoinLikeParser: btc.NewBitcoinLikeParser(params, c),
		baseparser:        &bchain.BaseParser{},
	}
}

// GetChainParams contains network parameters
func GetChainParams(chain string) *chaincfg.Params {
	if !chaincfg.IsRegistered(&MainNetParams) {
		err := chaincfg.Register(&MainNetParams)
		if err != nil {
			// If registration fails, it might already be registered, so just return the params
			return &MainNetParams
		}
	}
	return &MainNetParams
}

// PackTx packs transaction to byte array using protobuf
func (p *SatoxcoinParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
	return p.baseparser.PackTx(tx, height, blockTime)
}

// UnpackTx unpacks transaction from protobuf byte array
func (p *SatoxcoinParser) UnpackTx(buf []byte) (*bchain.Tx, uint32, error) {
	return p.baseparser.UnpackTx(buf)
}
