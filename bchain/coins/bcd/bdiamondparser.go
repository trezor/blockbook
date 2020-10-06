package bcd

import (
	"fmt"

	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"

	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil/chaincfg"
)

const (
	// MainnetMagic is mainnet network constant
	MainnetMagic wire.BitcoinNet = 0xbddeb4d9
	// TestnetMagic is testnet network constant
	TestnetMagic wire.BitcoinNet = 0x0bcd2018
	// RegtestMagic is regtest network constant
	RegtestMagic wire.BitcoinNet = 0xfabfb5da
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
	MainNetParams.AddressMagicLen = 1
	MainNetParams.PubKeyHashAddrID = []byte{0x00} // base58 prefix: 1
	MainNetParams.ScriptHashAddrID = []byte{0x05} // base58 prefix: 3
	MainNetParams.PrivateKeyID = []byte{128}

	TestNetParams = chaincfg.TestNet3Params
	TestNetParams.Net = TestnetMagic

	// Address encoding magics
	TestNetParams.AddressMagicLen = 1
	TestNetParams.PubKeyHashAddrID = []byte{111}
	TestNetParams.ScriptHashAddrID = []byte{196}
	TestNetParams.PrivateKeyID = []byte{239}

	RegtestParams = chaincfg.RegressionNetParams
	RegtestParams.Net = RegtestMagic
}

// BdiamondParser handle
type BdiamondParser struct {
	*btc.BitcoinParser
	baseparser *bchain.BaseParser
}

func NewBdiamondParser(params *chaincfg.Params, c *btc.Configuration) *BdiamondParser {
	p := &BdiamondParser{BitcoinParser: btc.NewBitcoinParser(params, c)}
	p.AmountDecimalPoint = 7
	return p
}

// GetChainParams contains network parameters for the main Bdiamond network,
// the regression test Bdiamond network, the test Bdiamond network and
// the simulation test Bdiamond network, in this order
func GetChainParams(chain string) *chaincfg.Params {
	fmt.Println("GetChainParams. Chain:", chain)
	fmt.Println("GetChainParams.IsRegistered MainNetParams:", chaincfg.IsRegistered(&MainNetParams))
	if !chaincfg.IsRegistered(&MainNetParams) {
		fmt.Println("GetChainParams chaincfg.Register(&MainNetParams)")
		err := chaincfg.Register(&MainNetParams)
		fmt.Println("GetChainParams err:", err)
		if err == nil {
			fmt.Println("GetChainParams chaincfg.Register(&TestNetParams)")
			err = chaincfg.Register(&TestNetParams)
		}
		if err == nil {
			fmt.Println("GetChainParams chaincfg.Register(&RegtestParams)")
			err = chaincfg.Register(&RegtestParams)
		}
		if err != nil {
			panic(err)
		}
	}

	switch chain {
	case "test":
		fmt.Println("GetChainParams case chain: test")
		return &TestNetParams
	case "regtest":
		fmt.Println("GetChainParams case chain: regtest")
		return &RegtestParams
	default:
		fmt.Println("GetChainParams case chain: mainnet")
		return &MainNetParams
	}
}

// PackTx packs transaction to byte array using protobuf
func (p *BdiamondParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
	fmt.Println("p.baseparser.PackTx")
	return p.baseparser.PackTx(tx, height, blockTime)
}

// UnpackTx unpacks transaction from protobuf byte array
func (p *BdiamondParser) UnpackTx(buf []byte) (*bchain.Tx, uint32, error) {
	fmt.Println("p.baseparser.UnpackTx")
	return p.baseparser.UnpackTx(buf)
}
