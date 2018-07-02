package denarius

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"blockbook/bchain/coins/utils"
	"bytes"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
)

const (
	MainnetMagic wire.BitcoinNet = 0xb4eff2fa
)

var (
	MainNetParams chaincfg.Params
)

func init() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic
	MainNetParams.PubKeyHashAddrID = 30
	MainNetParams.ScriptHashAddrID = 90

	err := chaincfg.Register(&MainNetParams)
	if err != nil {
		panic(err)
	}
}

// DenariusParser handle
type DenariusParser struct {
	*btc.BitcoinParser
}

// NewDenariusParser returns new DenariusParser instance
func NewDenariusParser(params *chaincfg.Params, c *btc.Configuration) *DenariusParser {
	return &DenariusParser{BitcoinParser: btc.NewBitcoinParser(params, c)}
}

// GetChainParams contains network parameters for the main Denarius network,
// and the test Denarius network
func GetChainParams(chain string) *chaincfg.Params {
	switch chain {
	default:
		return &MainNetParams
	}
}

// GetAddrIDFromVout returns internal address representation of given transaction output
func (p *DenariusParser) GetAddrIDFromVout(output *bchain.Vout) ([]byte, error) {
	if len(output.ScriptPubKey.Addresses) != 1 {
		return nil, nil
	}
	hash, _, err := utils.CheckDecode(output.ScriptPubKey.Addresses[0])
	return hash, err
}

// GetAddrIDFromAddress returns internal address representation of given address
func (p *DenariusParser) GetAddrIDFromAddress(address string) ([]byte, error) {
	hash, _, err := utils.CheckDecode(address)
	return hash, err
}
