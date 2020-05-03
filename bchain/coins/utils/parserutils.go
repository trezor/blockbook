package utils

import (
	"fmt"
	"io"

	"github.com/martinboehm/btcd/wire"
)

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

// DecodeTransactions decodes transactions from input stream using wire
func DecodeTransactions(r io.Reader, pver uint32, enc wire.MessageEncoding, blk *wire.MsgBlock) error {
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
		return &wire.MessageError{Func: "utils.decodeTransactions", Description: str}
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

// VersionAuxpow marks that block contains Auxpow
const VersionAuxpow = (1 << 8)

// SkipAuxpow skips Auxpow data in block
func SkipAuxpow(r io.ReadSeeker) error {
	// skip Auxpow part of the block
	// https://github.com/dogecoin/dogecoin/blob/master/src/auxpow.h#L130
	// CMerkleTx CTransaction
	tx := wire.MsgTx{}
	err := tx.BtcDecode(r, 0, wire.WitnessEncoding)
	if err != nil {
		return err
	}
	// CMerkleTx uint256 hashBlock
	_, err = r.Seek(32, io.SeekCurrent)
	if err != nil {
		return err
	}
	// CMerkleTx std::vector<uint256> vMerkleBranch
	size, err := wire.ReadVarInt(r, 0)
	if err != nil {
		return err
	}
	_, err = r.Seek(int64(size)*32, io.SeekCurrent)
	if err != nil {
		return err
	}
	// CMerkleTx int nIndex
	_, err = r.Seek(4, io.SeekCurrent)
	if err != nil {
		return err
	}
	// CAuxPow std::vector<uint256> vChainMerkleBranch;
	size, err = wire.ReadVarInt(r, 0)
	if err != nil {
		return err
	}
	_, err = r.Seek(int64(size)*32, io.SeekCurrent)
	if err != nil {
		return err
	}
	// CAuxPow int nChainIndex;
	_, err = r.Seek(4, io.SeekCurrent)
	if err != nil {
		return err
	}
	// CAuxPow CPureBlockHeader parentBlock;
	ph := wire.BlockHeader{}
	err = ph.Deserialize(r)
	if err != nil {
		return err
	}
	return nil
}
