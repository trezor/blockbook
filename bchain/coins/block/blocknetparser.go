package block

import (
	"bytes"
	"io"

	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
	"github.com/trezor/blockbook/bchain/coins/utils"
)

// constants used to indicate the message blocknet network.
const (
	MainnetMagic wire.BitcoinNet = 0xa3a2a0a1
	TestnetMagic wire.BitcoinNet = 0xbb657645
	RegtestMagic wire.BitcoinNet = 0xac7ecfa1
)

// chain parameters
var (
	MainNetParams chaincfg.Params
	TestNetParams chaincfg.Params
	RegtestParams chaincfg.Params
)

func init() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic
	MainNetParams.PubKeyHashAddrID = []byte{26} // base58 prefix: B
	MainNetParams.ScriptHashAddrID = []byte{28} // base58 prefix: C
	MainNetParams.Bech32HRPSegwit = "block"

	TestNetParams = chaincfg.TestNet3Params
	TestNetParams.Net = TestnetMagic
	TestNetParams.PubKeyHashAddrID = []byte{139} // base58 prefix: x or y
	TestNetParams.ScriptHashAddrID = []byte{19}  // base58 prefix: 8 or 9
	TestNetParams.Bech32HRPSegwit = "tblock"

	RegtestParams = chaincfg.RegressionNetParams
	RegtestParams.Net = RegtestMagic
	RegtestParams.PubKeyHashAddrID = []byte{139} // base58 prefix: x or y
	RegtestParams.ScriptHashAddrID = []byte{19}  // base58 prefix: 8 or 9
	RegtestParams.Bech32HRPSegwit = "blockrt"
}

// BlocknetParser handle
type BlocknetParser struct {
	*btc.BitcoinLikeParser
}

// NewBlocknetParser returns new BlocknetParser instance
func NewBlocknetParser(params *chaincfg.Params, c *btc.Configuration) *BlocknetParser {
	return &BlocknetParser{BitcoinLikeParser: btc.NewBitcoinLikeParser(params, c)}
}

// GetChainParams contains network parameters for the main Blocknet network,
// the regression test Blocknet network, the test Blocknet network and
// the simulation test Blocknet network
func GetChainParams(chain string) *chaincfg.Params {
	if !chaincfg.IsRegistered(&MainNetParams) {
		err := chaincfg.Register(&MainNetParams)
		if err == nil {
			err = chaincfg.Register(&TestNetParams)
		}
		if err == nil {
			err = chaincfg.Register(&RegtestParams)
		}
		if err != nil {
			panic(err)
		}
	}
	switch chain {
	case "test":
		return &TestNetParams
	case "regtest":
		return &RegtestParams
	default:
		return &MainNetParams
	}
}

func parseBlockHeader(r io.Reader) (*wire.BlockHeader, error) {
	h := &wire.BlockHeader{}
	err := h.Deserialize(r)
	if err != nil {
		return nil, err
	}

	// Blocknet specific header elements
	// uint256 hashStake;
	// uint32_t nStakeIndex;
	// int64_t nStakeAmount;
	// uint256 hashStakeBlock;
	buf := make([]byte, 76)
	_, err = io.ReadFull(r, buf)
	if err != nil {
		return nil, err
	}

	return h, err
}

func (p *BlocknetParser) ParseBlock(b []byte) (*bchain.Block, error) {
	r := bytes.NewReader(b)
	w := wire.MsgBlock{}

	h, err := parseBlockHeader(r)
	if err != nil {
		return nil, err
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
