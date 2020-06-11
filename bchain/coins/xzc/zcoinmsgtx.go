package xzc

import (
	"bytes"
	"io"

	"github.com/martinboehm/btcd/chaincfg/chainhash"
	"github.com/martinboehm/btcd/wire"
)

// ZcoinMsgTx encapsulate zcoin tx and extra
type ZcoinMsgTx struct {
	wire.MsgTx
	Extra []byte
}

// TxHash calculate hash of transaction
func (msg *ZcoinMsgTx) TxHash() chainhash.Hash {
	extraSize := uint64(len(msg.Extra))
	sizeOfExtraSize := 0
	if extraSize != 0 {
		sizeOfExtraSize = wire.VarIntSerializeSize(extraSize)
	}

	// Original payload
	buf := bytes.NewBuffer(make([]byte, 0,
		msg.SerializeSizeStripped()+sizeOfExtraSize+len(msg.Extra)))
	_ = msg.SerializeNoWitness(buf)

	// Extra payload
	if extraSize != 0 {
		wire.WriteVarInt(buf, 0, extraSize)
		buf.Write(msg.Extra)
	}

	return chainhash.DoubleHashH(buf.Bytes())
}

// XzcDecode to decode bitcoin tx and extra
func (msg *ZcoinMsgTx) XzcDecode(r io.Reader, pver uint32, enc wire.MessageEncoding) error {
	if err := msg.MsgTx.BtcDecode(r, pver, enc); err != nil {
		return err
	}

	// extra
	version := uint32(msg.Version)
	txVersion := version & 0xffff
	txType := (version >> 16) & 0xffff
	if txVersion == 3 && txType != 0 {

		extraSize, err := wire.ReadVarInt(r, 0)
		if err != nil {
			return err
		}

		msg.Extra = make([]byte, extraSize)
		_, err = io.ReadFull(r, msg.Extra[:])
		if err != nil {
			return err
		}
	}

	return nil
}
