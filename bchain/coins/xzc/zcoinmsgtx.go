package xzc

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"log"

	"github.com/martinboehm/btcd/chaincfg/chainhash"
	"github.com/martinboehm/btcd/wire"
)

const (
	minTxInPayload          = 9 + chainhash.HashSize
	maxTxInPerMessage       = (wire.MaxMessagePayload / minTxInPayload) + 1
	maxWitnessItemsPerInput = 500000
)

type ZcoinMsgTx struct {
	wire.MsgTx
	Extra []byte
}

func (msg *ZcoinMsgTx) TxHash() chainhash.Hash {
	extraSize := uint64(len(msg.Extra))
	sizeOfExtraSize := wire.VarIntSerializeSize(extraSize)

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

func (msg *ZcoinMsgTx) XzcDecode(r io.Reader, pver uint32, enc wire.MessageEncoding) error {

	var version uint32 = 0
	if err := binary.Read(r, binary.LittleEndian, &version); err != nil {
		return err
	}

	msg.Version = int32(version)

	count, err := wire.ReadVarInt(r, 0)
	if err != nil {
		return err
	}

	var flag [1]byte
	if count == 0 && enc == wire.WitnessEncoding {
		if _, err = io.ReadFull(r, flag[:]); err != nil {
			return err
		}
	}

	// parsing inputs/outputs
	if flag[0]&1 != 0 {
		count, err = wire.ReadVarInt(r, 0)
		if err != nil {
			return err
		}
	}

	if count > uint64(maxTxInPerMessage) {
		return errors.New("too many input transactions")
	}

	msg.TxIn = make([]*wire.TxIn, count)
	txIn := make([]wire.TxIn, count)

	for i := uint64(0); i != count; i++ {
		ti := txIn[i]
		msg.TxIn[i] = &ti

		// outpoint
		if _, err = io.ReadFull(r, ti.PreviousOutPoint.Hash[:]); err != nil {
			return err
		}

		if err = binary.Read(r, binary.LittleEndian, &ti.PreviousOutPoint.Index); err != nil {
			return err
		}

		// script
		if ti.SignatureScript, err = wire.ReadVarBytes(r, 0, wire.MaxMessagePayload,
			"transaction input signature script"); err != nil {
			return err
		}

		// sequence
		if err = binary.Read(r, binary.LittleEndian, &ti.Sequence); err != nil {
			return err
		}
	}

	if !(count == 0 && enc == wire.WitnessEncoding) {
		outputs, err := wire.ReadVarInt(r, 0)
		if err != nil {
			return err
		}

		if outputs > uint64(maxTxInPerMessage) {
			return errors.New("too many output transactions")
		}

		msg.TxOut = make([]*wire.TxOut, outputs)
		txOut := make([]wire.TxOut, outputs)
		for i := 0; i != len(txOut); i++ {
			to := &txOut[i]
			msg.TxOut[i] = to

			if err = binary.Read(r, binary.LittleEndian, &to.Value); err != nil {
				return err
			}

			if to.PkScript, err = wire.ReadVarBytes(r, 0, wire.MaxMessagePayload,
				"transaction output public key script"); err != nil {
				return err
			}
		}
	}

	if (flag[0]&1 != 0) && enc == wire.WitnessEncoding {
		flag[0] ^= 1
		for _, txin := range msg.TxIn {
			witCount, err := wire.ReadVarInt(r, 0)
			if err != nil {
				return err
			}

			if witCount > maxWitnessItemsPerInput {
				return errors.New("too many witness to fit into max message size")
			}

			txin.Witness = make([][]byte, witCount)
			for i := uint64(0); i != witCount; i++ {
				txin.Witness[i], err = wire.ReadVarBytes(r, 0, maxWitnessItemsPerInput, "witness")
				if err != nil {
					return err
				}
			}
		}
	}

	if flag[0] != 0 {
		return errors.New("flag field contain unknown")
	}

	if err := binary.Read(r, binary.LittleEndian, &msg.LockTime); err != nil {
		return err
	}

	// extra
	txVersion := version & 0xffff
	txType := (version >> 16) & 0xffff
	if txVersion == 3 && txType != 0 {

		extraSize, err := wire.ReadVarInt(r, 0)
		if err != nil {
			return err
		}
		log.Printf("Size %d", extraSize)

		msg.Extra = make([]byte, extraSize)
		_, err = io.ReadFull(r, msg.Extra[:])
		if err != nil {
			return err
		}
	}

	return nil
}
