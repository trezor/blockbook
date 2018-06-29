package viacoin

import (
	"blockbook/bchain/coins/btc"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
)

const (
	MainnetMagic wire.BitcoinNet = 0xcbc6680f
	Regtest      wire.BitcoinNet = 0x377b972d
)

var (
	MainNetParams chaincfg.Params
	RegtestParams chaincfg.Params
)

func init() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic
	MainNetParams.PubKeyHashAddrID = 47
	MainNetParams.ScriptHashAddrID = 21
	MainNetParams.Bech32HRPSegwit = "via"

	err := chaincfg.Register(&MainNetParams)
	if err != nil {
		panic(err)
	}
}

// ViacoinParser handle
type ViacoinParser struct {
	*btc.BitcoinParser
}

// NewViacoinParser returns new ViacoinParser instance
func NewViacoinParser(params *chaincfg.Params, c *btc.Configuration) *ViacoinParser {
	return &ViacoinParser{BitcoinParser: btc.NewBitcoinParser(params, c)}
}

func GetChainParams(chain string) *chaincfg.Params {
	switch chain {
	case "regtest":
		return &RegtestParams
	default:
		return &MainNetParams
	}
}

// handle Auxpow blocks TODO
