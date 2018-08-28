package zec

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"blockbook/bchain/coins/utils"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
	"github.com/juju/errors"
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

// NewZCashParser returns new ZCashParser instance
func NewZCashParser(c *btc.Configuration) *ZCashParser {
	return &ZCashParser{
		&bchain.BaseParser{
			BlockAddressesToKeep: c.BlockAddressesToKeep,
			AmountDecimalPoint:   8,
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

// GetAddrDescFromVout returns internal address representation of given transaction output
func (p *ZCashParser) GetAddrDescFromVout(output *bchain.Vout) ([]byte, error) {
	if len(output.ScriptPubKey.Addresses) != 1 {
		return nil, nil
	}
	hash, _, err := utils.CheckDecode(output.ScriptPubKey.Addresses[0])
	return hash, err
}

// GetAddrDescFromAddress returns internal address representation of given address
func (p *ZCashParser) GetAddrDescFromAddress(address string) ([]byte, error) {
	hash, _, err := utils.CheckDecode(address)
	return hash, err
}

// GetAddressesFromAddrDesc returns addresses for given address descriptor with flag if the addresses are searchable
func (p *ZCashParser) GetAddressesFromAddrDesc(addrDesc []byte) ([]string, bool, error) {
	// TODO implement
	return nil, false, errors.New("GetAddressesFromAddrDesc: not implemented")
}

// GetScriptFromAddrDesc returns output script for given address descriptor
func (p *ZCashParser) GetScriptFromAddrDesc(addrDesc []byte) ([]byte, error) {
	// TODO implement
	return nil, errors.New("GetScriptFromAddrDesc: not implemented")
}
