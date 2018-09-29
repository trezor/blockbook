package myriad

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"blockbook/bchain/coins/utils"
	"bytes"

	"github.com/jakm/btcutil/chaincfg"
	"github.com/btcsuite/btcd/wire"
)

const (
	MainnetMagic wire.BitcoinNet = 0xee7645af
)

var (
	MainNetParams chaincfg.Params
)

func initParams() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic

	MainNetParams.Bech32HRPSegwit = "my"

	MainNetParams.PubKeyHashAddrID = []byte{50} // 0x32 - starts with M
	MainNetParams.ScriptHashAddrID = []byte{9} // 0x09 - starts with 4
	MainNetParams.PrivateKeyID = []byte{178} // 0xB2
	
	MainNetParams.HDCoinType = 90

	err := chaincfg.Register(&MainNetParams)
	if err != nil {
		panic(err)
	}
}

// MyriadParser handle
type MyriadParser struct {
	*btc.BitcoinParser
}

// NewMyriadParser returns new MyriadParser instance
func NewMyriadParser(params *chaincfg.Params, c *btc.Configuration) *MyriadParser {
	return &MyriadParser{BitcoinParser: btc.NewBitcoinParser(params, c)}
}

// GetChainParams contains network parameters for the main Myriad network
func GetChainParams(chain string) *chaincfg.Params {
	if MainNetParams.Name == "" {
		initParams()
	}
	switch chain {
	default:
		return &MainNetParams
	}
}

// ParseBlock parses raw block to our Block struct
// it has special handling for Auxpow blocks that cannot be parsed by standard btc wire parser
func (p *MyriadParser) ParseBlock(b []byte) (*bchain.Block, error) {
	r := bytes.NewReader(b)
	w := wire.MsgBlock{}
	h := wire.BlockHeader{}
	err := h.Deserialize(r)
	if err != nil {
		return nil, err
	}
	if (h.Version & utils.VersionAuxpow) != 0 {
		if err = utils.SkipAuxpow(r); err != nil {
			return nil, err
		}
	}

	err = utils.DecodeTransactions(r, 0, wire.WitnessEncoding, &w)
	if err != nil {
		return nil, err
	}

	txs := make([]bchain.Tx, len(w.Transactions))
	for ti, t := range w.Transactions {
		txs[ti] = p.TxFromMsgTx(t, false)
	}

	return &bchain.Block{Txs: txs}, nil
}
