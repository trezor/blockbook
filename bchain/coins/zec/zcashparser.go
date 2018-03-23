package zec

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"

	"github.com/btcsuite/btcd/chaincfg"
)

// bitcoinwire parsing

type ZCashBlockParser struct {
	btc.BitcoinBlockParser
}

// getChainParams contains network parameters for the main Bitcoin network,
// the regression test Bitcoin network, the test Bitcoin network and
// the simulation test Bitcoin network, in this order
func GetChainParams(chain string) *chaincfg.Params {
	switch chain {
	case "test":
		return &chaincfg.TestNet3Params
	case "regtest":
		return &chaincfg.RegressionNetParams
	}
	return &chaincfg.MainNetParams
}

func (p *ZCashBlockParser) GetAddrIDFromVout(output *bchain.Vout) ([]byte, error) {
	if len(output.ScriptPubKey.Addresses) != 1 {
		return nil, nil
	}
	return []byte(output.ScriptPubKey.Addresses[0]), nil
}

func (p *ZCashBlockParser) GetAddrIDFromAddress(address string) ([]byte, error) {
	return []byte(address), nil
}
