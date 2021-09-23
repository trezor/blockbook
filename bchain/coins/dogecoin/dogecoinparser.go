package dogecoin

import (
	"bytes"

	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
	"github.com/trezor/blockbook/bchain/coins/utils"
)

// magic numbers
const (
	MainnetMagic wire.BitcoinNet = 0xc0c0c0c0
	TestnetMagic wire.BitcoinNet = 0xfcc1b7dc // See https://github.com/dogecoin/dogecoin/blob/f80bfe9068ac1a0619d48dad0d268894d926941e/qa/rpc-tests/test_framework/mininode.py#L1620
)

// chain parameters
var (
	MainNetParams chaincfg.Params
	TestNetParams chaincfg.Params
)

func init() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic
	MainNetParams.PubKeyHashAddrID = []byte{30}
	MainNetParams.ScriptHashAddrID = []byte{22}

	TestNetParams = chaincfg.TestNet3Params
	TestNetParams.Net = TestnetMagic
	TestNetParams.PubKeyHashAddrID = []byte{113} // See https://github.com/dogecoin/dogecoin/blob/f80bfe9068ac1a0619d48dad0d268894d926941e/contrib/testgen/gen_base58_test_vectors.py#L23
	TestNetParams.ScriptHashAddrID = []byte{196} // See https://github.com/dogecoin/dogecoin/blob/f80bfe9068ac1a0619d48dad0d268894d926941e/contrib/testgen/gen_base58_test_vectors.py#L24
}

// DogecoinParser handle
type DogecoinParser struct {
	*btc.BitcoinLikeParser
}

// NewDogecoinParser returns new DogecoinParser instance
func NewDogecoinParser(params *chaincfg.Params, c *btc.Configuration) *DogecoinParser {
	return &DogecoinParser{BitcoinLikeParser: btc.NewBitcoinLikeParser(params, c)}
}

// GetChainParams contains network parameters for the main Dogecoin network,
// and the test Dogecoin network
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

	return &bchain.Block{
		BlockHeader: bchain.BlockHeader{
			Size: len(b),
			Time: h.Timestamp.Unix(),
		},
		Txs: txs,
	}, nil
}
