package zec

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
)

const (
	MainnetMagic wire.BitcoinNet = 0x6427e924
	TestnetMagic wire.BitcoinNet = 0xbff91afa
	RegtestMagic wire.BitcoinNet = 0x5f3fe8aa
)

// ZCashParser handle
type ZCashParser struct {
	*bchain.BaseParser
}

// NewZCAshParser returns new ZCAshParser instance
func NewZCashParser(c *btc.Configuration) *ZCashParser {
	return &ZCashParser{
		&bchain.BaseParser{
			AddressFactory:       bchain.NewBaseAddress,
			BlockAddressesToKeep: c.BlockAddressesToKeep,
		},
	}
}

// GetChainParams contains network parameters for the main ZCash network,
// the regression test ZCash network, the test ZCash network and
// the simulation test ZCash network, in this order
func GetChainParams(chain string) *chaincfg.Params {
	var params *chaincfg.Params
	switch chain {
	case "test":
		params = &chaincfg.TestNet3Params
		params.Net = TestnetMagic
	case "regtest":
		params = &chaincfg.RegressionNetParams
		params.Net = RegtestMagic
	default:
		params = &chaincfg.MainNetParams
		params.Net = MainnetMagic
	}

	return params
}

// GetAddrIDFromVout returns internal address representation of given transaction output
func (p *ZCashParser) GetAddrIDFromVout(output *bchain.Vout) ([]byte, error) {
	if len(output.ScriptPubKey.Addresses) != 1 {
		return nil, nil
	}
	hash, _, err := CheckDecode(output.ScriptPubKey.Addresses[0])
	return hash, err
}

// GetAddrIDFromAddress returns internal address representation of given address
func (p *ZCashParser) GetAddrIDFromAddress(address string) ([]byte, error) {
	hash, _, err := CheckDecode(address)
	return hash, err
}
