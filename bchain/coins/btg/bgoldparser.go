package btg

import (
	"bytes"
	"encoding/binary"
	"io"

	"github.com/martinboehm/btcd/chaincfg/chainhash"
	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
	"github.com/trezor/blockbook/bchain/coins/utils"
)

const (
	// MainnetMagic is mainnet network constant
	MainnetMagic wire.BitcoinNet = 0x446d47e1
	// TestnetMagic is testnet network constant
	TestnetMagic wire.BitcoinNet = 0x456e48e2
)

var (
	// MainNetParams are parser parameters for mainnet
	MainNetParams chaincfg.Params
	// TestNetParams are parser parameters for testnet
	TestNetParams chaincfg.Params
)

func init() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic

	// Address encoding magics
	MainNetParams.PubKeyHashAddrID = []byte{38} // base58 prefix: G
	MainNetParams.ScriptHashAddrID = []byte{23} // base58 prefix: A

	TestNetParams = chaincfg.TestNet3Params
	TestNetParams.Net = TestnetMagic

	// Human-readable part for Bech32 encoded segwit addresses, as defined in
	// BIP 173.
	// see https://github.com/satoshilabs/slips/blob/master/slip-0173.md
	MainNetParams.Bech32HRPSegwit = "btg"
	TestNetParams.Bech32HRPSegwit = "tbtg"
}

// BGoldParser handle
type BGoldParser struct {
	*btc.BitcoinLikeParser
}

// NewBGoldParser returns new BGoldParser instance
func NewBGoldParser(params *chaincfg.Params, c *btc.Configuration) *BGoldParser {
	p := &BGoldParser{BitcoinLikeParser: btc.NewBitcoinLikeParser(params, c)}
	p.VSizeSupport = true
	return p
}

// GetChainParams contains network parameters for the main Bitcoin Cash network,
// the regression test Bitcoin Cash network, the test Bitcoin Cash network and
// the simulation test Bitcoin Cash network, in this order
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
	case "regtest":
		return &chaincfg.RegressionNetParams
	default:
		return &MainNetParams
	}
}

// headerFixedLength is the length of fixed fields of a block (i.e. without solution)
// see https://github.com/BTCGPU/BTCGPU/wiki/Technical-Spec#block-header
const headerFixedLength = 44 + (chainhash.HashSize * 3)
const timestampOffset = 100
const timestampLength = 4

// ParseBlock parses raw block to our Block struct
func (p *BGoldParser) ParseBlock(b []byte) (*bchain.Block, error) {
	r := bytes.NewReader(b)
	time, err := getTimestampAndSkipHeader(r, 0)
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

	return &bchain.Block{
		BlockHeader: bchain.BlockHeader{
			Size: len(b),
			Time: time,
		},
		Txs: txs,
	}, nil
}

func getTimestampAndSkipHeader(r io.ReadSeeker, pver uint32) (int64, error) {
	_, err := r.Seek(timestampOffset, io.SeekStart)
	if err != nil {
		return 0, err
	}

	buf := make([]byte, timestampLength)
	if _, err = io.ReadFull(r, buf); err != nil {
		return 0, err
	}
	time := binary.LittleEndian.Uint32(buf)

	_, err = r.Seek(headerFixedLength-timestampOffset-timestampLength, io.SeekCurrent)
	if err != nil {
		return 0, err
	}

	size, err := wire.ReadVarInt(r, pver)
	if err != nil {
		return 0, err
	}

	_, err = r.Seek(int64(size), io.SeekCurrent)
	if err != nil {
		return 0, err
	}

	return int64(time), nil
}
