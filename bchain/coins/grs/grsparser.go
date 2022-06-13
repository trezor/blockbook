package grs

import (
	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil/base58"
	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
)

// magic numbers
const (
	MainnetMagic wire.BitcoinNet = 0xd4b4bef9
	TestnetMagic wire.BitcoinNet = 0x0709110b
	RegtestMagic wire.BitcoinNet = 0xdab5bffa
	SignetMagic  wire.BitcoinNet = 0x7696b422
)

// chain parameters
var (
	MainNetParams chaincfg.Params
	TestNetParams chaincfg.Params
	RegTestParams chaincfg.Params
	SigNetParams  chaincfg.Params
)

func init() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic

	// Address encoding magics
	MainNetParams.PubKeyHashAddrID = []byte{36}
	MainNetParams.ScriptHashAddrID = []byte{5}
	MainNetParams.Bech32HRPSegwit = "grs"
	MainNetParams.Base58CksumHasher = base58.Groestl512D

	TestNetParams = chaincfg.TestNet3Params
	TestNetParams.Net = TestnetMagic

	// Address encoding magics
	TestNetParams.PubKeyHashAddrID = []byte{111}
	TestNetParams.ScriptHashAddrID = []byte{196}
	TestNetParams.Bech32HRPSegwit = "tgrs"
	TestNetParams.Base58CksumHasher = base58.Groestl512D

	RegTestParams = chaincfg.RegressionNetParams
	RegTestParams.Net = RegtestMagic

	// Address encoding magics
	RegTestParams.PubKeyHashAddrID = []byte{111}
	RegTestParams.ScriptHashAddrID = []byte{196}
	RegTestParams.Bech32HRPSegwit = "grsrt"
	RegTestParams.Base58CksumHasher = base58.Groestl512D

	SigNetParams = chaincfg.SigNetParams
	SigNetParams.Net = SignetMagic

	// Address encoding magics
	SigNetParams.PubKeyHashAddrID = []byte{111}
	SigNetParams.ScriptHashAddrID = []byte{196}
	SigNetParams.Bech32HRPSegwit = "tgrs"
	SigNetParams.Base58CksumHasher = base58.Groestl512D
}

// GroestlcoinParser handle
type GroestlcoinParser struct {
	*btc.BitcoinParser
	baseparser *bchain.BaseParser
}

// NewGroestlcoinParser returns new GroestlcoinParser instance
func NewGroestlcoinParser(params *chaincfg.Params, c *btc.Configuration) *GroestlcoinParser {
	return &GroestlcoinParser{
		BitcoinParser: btc.NewBitcoinParser(params, c),
		baseparser:    &bchain.BaseParser{},
	}
}

// GetChainParams contains network parameters for the main Groestlcoin network,
// the regression test Groestlcoin network, the test Groestlcoin network and
// the simulation test Groestlcoin network, in this order
func GetChainParams(chain string) *chaincfg.Params {
	if !chaincfg.IsRegistered(&MainNetParams) {
		err := chaincfg.Register(&MainNetParams)
		if err == nil {
			err = chaincfg.Register(&TestNetParams)
		}
		if err != nil {
			panic(err)
		}
	}
	switch chain {
	case "test":
		return &TestNetParams
	case "regtest":
		return &RegTestParams
	case "signet":
		return &SigNetParams
	default:
		return &MainNetParams
	}
}

// PackTx packs transaction to byte array using protobuf
func (p *GroestlcoinParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
	return p.baseparser.PackTx(tx, height, blockTime)
}

// UnpackTx unpacks transaction from protobuf byte array
func (p *GroestlcoinParser) UnpackTx(buf []byte) (*bchain.Tx, uint32, error) {
	return p.baseparser.UnpackTx(buf)
}
