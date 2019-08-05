package dcr

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"

	dcrcfg "github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/txscript"
	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil/base58"
	"github.com/martinboehm/btcutil/chaincfg"
)

const (
	// MainnetMagic is mainnet network constant
	MainnetMagic wire.BitcoinNet = 0xd9b400f9
	// TestnetMagic is testnet network constant
	TestnetMagic wire.BitcoinNet = 0xb194aa75
)

var (
	// MainNetParams are parser parameters for mainnet
	MainNetParams chaincfg.Params
	// TestNet3Params are parser parameters for testnet
	TestNet3Params chaincfg.Params
)

func init() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic
	MainNetParams.PubKeyHashAddrID = []byte{0x07, 0x3f}
	MainNetParams.ScriptHashAddrID = []byte{0x07, 0x1a}
	MainNetParams.Base58CksumHasher = base58.Blake256D
	MainNetParams.AddressMagicLen = 2

	TestNet3Params = chaincfg.TestNet3Params
	TestNet3Params.Net = TestnetMagic
	TestNet3Params.PubKeyHashAddrID = []byte{0x0f, 0x21}
	TestNet3Params.ScriptHashAddrID = []byte{0x0e, 0xfc}
	TestNet3Params.Base58CksumHasher = base58.Blake256D
	TestNet3Params.AddressMagicLen = 2
}

// DecredParser handle
type DecredParser struct {
	*btc.BitcoinParser
	baseParser *bchain.BaseParser
}

// NewDecredParser returns new DecredParser instance
func NewDecredParser(params *chaincfg.Params, c *btc.Configuration) *DecredParser {
	p := DecredParser{
		BitcoinParser: btc.NewBitcoinParser(params, c),
		baseParser:    &bchain.BaseParser{},
	}
	p.OutputScriptToAddressesFunc = p.outputScriptToAddresses

	return &p
}

// GetChainParams contains network parameters for the main Decred network,
// and the test Decred network.
func GetChainParams(chain string) *chaincfg.Params {
	var param *chaincfg.Params

	switch chain {
	case "testnet3":
		param = &TestNet3Params
	default:
		param = &MainNetParams
	}

	if !chaincfg.IsRegistered(param) {
		if err := chaincfg.Register(param); err != nil {
			panic(err)
		}
	}
	return param
}

// PackTx packs transaction to byte array using protobuf
func (p *DecredParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
	return p.baseParser.PackTx(tx, height, blockTime)
}

// UnpackTx unpacks transaction from protobuf byte array
func (p *DecredParser) UnpackTx(buf []byte) (*bchain.Tx, uint32, error) {
	return p.baseParser.UnpackTx(buf)
}

// outputScriptToAddresses converts ScriptPubKey to addresses with a flag that the addresses are searchable
func (p *DecredParser) outputScriptToAddresses(script []byte) ([]string, bool, error) {
	var params dcrcfg.Params
	if p.Params.Name == "mainnet" {
		params = dcrcfg.MainNetParams
	} else {
		params = dcrcfg.TestNet3Params
	}
	sc, addresses, _, err := txscript.ExtractPkScriptAddrs(0, script, &params)
	if err != nil {
		return nil, false, err
	}
	rv := make([]string, 0, len(addresses))
	for _, a := range addresses {
		rv = append(rv, a.EncodeAddress())
	}
	var s bool
	switch sc {
	case txscript.PubKeyHashTy, txscript.ScriptHashTy:
		s = true
	}
	return rv, s, nil
}
