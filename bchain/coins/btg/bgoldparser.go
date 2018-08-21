package btg

import (
	"bytes"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
	"github.com/trezor/blockbook/bchain/coins/utils"
	"io"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

const (
	MainnetMagic wire.BitcoinNet = 0x446d47e1
	TestnetMagic wire.BitcoinNet = 0x456e48e2
)

var (
	MainNetParams chaincfg.Params
	TestNetParams chaincfg.Params
)

func init() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic

	// Address encoding magics
	MainNetParams.PubKeyHashAddrID = 38 // base58 prefix: G
	MainNetParams.ScriptHashAddrID = 23 // base58 prefix: A

	TestNetParams = chaincfg.TestNet3Params
	TestNetParams.Net = TestnetMagic

	// Human-readable part for Bech32 encoded segwit addresses, as defined in
	// BIP 173.
	// see https://github.com/satoshilabs/slips/blob/master/slip-0173.md
	MainNetParams.Bech32HRPSegwit = "btg"
	TestNetParams.Bech32HRPSegwit = "tbtg"

	err := chaincfg.Register(&MainNetParams)
	if err == nil {
		err = chaincfg.Register(&TestNetParams)
	}
	if err != nil {
		panic(err)
	}
}

// BGoldParser handle
type BGoldParser struct {
	*btc.BitcoinParser
}

// NewBGoldParser returns new BGoldParser instance
func NewBGoldParser(params *chaincfg.Params, c *btc.Configuration) *BGoldParser {
	return &BGoldParser{BitcoinParser: btc.NewBitcoinParser(params, c)}
}

// GetChainParams contains network parameters for the main Bitcoin Cash network,
// the regression test Bitcoin Cash network, the test Bitcoin Cash network and
// the simulation test Bitcoin Cash network, in this order
func GetChainParams(chain string) *chaincfg.Params {
	switch chain {
	case "test":
		return &TestNetParams
	case "regtest":
		return &chaincfg.RegressionNetParams
	default:
		return &MainNetParams
	}
}

// headerFixedLength is the length of fixed fields of a block (i.e. without solution)
// see https://github.com/BTCGPU/BTCGPU/wiki/Technical-Spec#block-header
const headerFixedLength = 44 + (chainhash.HashSize * 3)

// ParseBlock parses raw block to our Block struct
func (p *BGoldParser) ParseBlock(b []byte) (*bchain.Block, error) {
	r := bytes.NewReader(b)
	err := skipHeader(r, 0)
	if err != nil {
		return nil, err
	}

	w := wire.MsgBlock{}
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

func skipHeader(r io.ReadSeeker, pver uint32) error {
	_, err := r.Seek(headerFixedLength, io.SeekStart)
	if err != nil {
		return err
	}

	size, err := wire.ReadVarInt(r, pver)
	if err != nil {
		return err
	}

	_, err = r.Seek(int64(size), io.SeekCurrent)
	if err != nil {
		return err
	}

	return nil
}
