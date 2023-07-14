package deeponion

import (
	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
)

// magic numbers
const (
	MainnetMagic wire.BitcoinNet = 0xf2dbf1d1
)

// chain parameters
var (
	MainNetParams chaincfg.Params
)

func init() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic
	MainNetParams.PubKeyHashAddrID = []byte{31}
	MainNetParams.ScriptHashAddrID = []byte{78}
	MainNetParams.Bech32HRPSegwit = "dpn"
}

// DeepOnionParser handle
type DeepOnionParser struct {
	*btc.BitcoinLikeParser
	baseparser *bchain.BaseParser
}

// NewDeepOnionParser returns new DeepOnionParser instance
func NewDeepOnionParser(params *chaincfg.Params, c *btc.Configuration) *DeepOnionParser {
	return &DeepOnionParser{
		BitcoinLikeParser: btc.NewBitcoinLikeParser(params, c),
		baseparser:        &bchain.BaseParser{},
	}
}

// GetChainParams contains network parameters for the main DeepOnion network,
func GetChainParams(chain string) *chaincfg.Params {
	// register bitcoin parameters in addition to deeponion parameters
	// deeponion has dual standard of addresses and we want to be able to
	// parse both standards
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
func (p *DeepOnionParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
	return p.baseparser.PackTx(tx, height, blockTime)
}

// UnpackTx unpacks transaction from protobuf byte array
func (p *DeepOnionParser) UnpackTx(buf []byte) (*bchain.Tx, uint32, error) {
	return p.baseparser.UnpackTx(buf)
}
