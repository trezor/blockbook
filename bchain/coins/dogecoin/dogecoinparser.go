package dogecoin

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"blockbook/bchain/coins/utils"
	"bytes"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
)

const (
	MainnetMagic wire.BitcoinNet = 0xc0c0c0c0
)

var (
	MainNetParams chaincfg.Params
)

func init() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic
	MainNetParams.PubKeyHashAddrID = 30
	MainNetParams.ScriptHashAddrID = 22

	err := chaincfg.Register(&MainNetParams)
	if err != nil {
		panic(err)
	}
}

// DogecoinParser handle
type DogecoinParser struct {
	*btc.BitcoinParser
}

// NewDogecoinParser returns new DogecoinParser instance
func NewDogecoinParser(params *chaincfg.Params, c *btc.Configuration) *DogecoinParser {
	return &DogecoinParser{BitcoinParser: btc.NewBitcoinParser(params, c)}
}

// GetChainParams contains network parameters for the main Dogecoin network,
// and the test Dogecoin network
func GetChainParams(chain string) *chaincfg.Params {
	switch chain {
	default:
		return &MainNetParams
	}
}

// ParseBlock parses raw block to our Block struct
// it has special handling for Auxpow blocks that cannot be parsed by standard btc wire parser
func (p *DogecoinParser) ParseBlock(b []byte) (*bchain.Block, error) {
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
