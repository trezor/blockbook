package dogecoin

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"bytes"
	"fmt"
	"io"

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

// minTxPayload is the minimum payload size for a transaction.  Note
// that any realistically usable transaction must have at least one
// input or output, but that is a rule enforced at a higher layer, so
// it is intentionally not included here.
// Version 4 bytes + Varint number of transaction inputs 1 byte + Varint
// number of transaction outputs 1 byte + LockTime 4 bytes + min input
// payload + min output payload.
const minTxPayload = 10

// maxTxPerBlock is the maximum number of transactions that could
// possibly fit into a block.
const maxTxPerBlock = (wire.MaxBlockPayload / minTxPayload) + 1

const versionAuxpow = (1 << 8)

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
	if (h.Version & versionAuxpow) != 0 {
		// skip Auxpow part of the block
		// https://github.com/dogecoin/dogecoin/blob/master/src/auxpow.h#L130
		// CMerkleTx CTransaction
		tx := wire.MsgTx{}
		err = tx.BtcDecode(r, 0, wire.WitnessEncoding)
		if err != nil {
			return nil, err
		}
		// CMerkleTx uint256 hashBlock
		_, err = r.Seek(32, io.SeekCurrent)
		if err != nil {
			return nil, err
		}
		// CMerkleTx std::vector<uint256> vMerkleBranch
		size, err := wire.ReadVarInt(r, 0)
		if err != nil {
			return nil, err
		}
		_, err = r.Seek(int64(size)*32, io.SeekCurrent)
		if err != nil {
			return nil, err
		}
		// CMerkleTx int nIndex
		_, err = r.Seek(4, io.SeekCurrent)
		if err != nil {
			return nil, err
		}
		// CAuxPow std::vector<uint256> vChainMerkleBranch;
		size, err = wire.ReadVarInt(r, 0)
		if err != nil {
			return nil, err
		}
		_, err = r.Seek(int64(size)*32, io.SeekCurrent)
		if err != nil {
			return nil, err
		}
		// CAuxPow int nChainIndex;
		_, err = r.Seek(4, io.SeekCurrent)
		if err != nil {
			return nil, err
		}
		// CAuxPow CPureBlockHeader parentBlock;
		ph := wire.BlockHeader{}
		err = ph.Deserialize(r)
		if err != nil {
			return nil, err
		}
	}

	err = decodeTransactions(r, 0, wire.WitnessEncoding, &w)
	if err != nil {
		return nil, err
	}

	txs := make([]bchain.Tx, len(w.Transactions))
	for ti, t := range w.Transactions {
		txs[ti] = p.TxFromMsgTx(t, false)
	}

	return &bchain.Block{Txs: txs}, nil
}

func decodeTransactions(r io.Reader, pver uint32, enc wire.MessageEncoding, blk *wire.MsgBlock) error {
	txCount, err := wire.ReadVarInt(r, pver)
	if err != nil {
		return err
	}

	// Prevent more transactions than could possibly fit into a block.
	// It would be possible to cause memory exhaustion and panics without
	// a sane upper bound on this count.
	if txCount > maxTxPerBlock {
		str := fmt.Sprintf("too many transactions to fit into a block "+
			"[count %d, max %d]", txCount, maxTxPerBlock)
		return &wire.MessageError{Func: "btg.decodeTransactions", Description: str}
	}

	blk.Transactions = make([]*wire.MsgTx, 0, txCount)
	for i := uint64(0); i < txCount; i++ {
		tx := wire.MsgTx{}
		err := tx.BtcDecode(r, pver, enc)
		if err != nil {
			return err
		}
		blk.Transactions = append(blk.Transactions, &tx)
	}

	return nil
}
