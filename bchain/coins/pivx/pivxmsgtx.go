package pivx

import (
	"bytes"
	"io"

	"github.com/martinboehm/btcd/chaincfg/chainhash"
	"github.com/martinboehm/btcd/wire"
)

// PivxMsgTx encapsulate pivx tx and extra
type PivxMsgTx struct {
	wire.MsgTx
	Extra []byte
}

// TxHash calculate hash of transaction
func (msg *PivxMsgTx) TxHash() chainhash.Hash {
	buf := bytes.NewBuffer(make([]byte, 0,
		msg.SerializeSizeStripped()+len(msg.Extra)))
	_ = msg.SerializeNoWitness(buf)

	if len(msg.Extra) != 0 {
		buf.Write(msg.Extra)
	}

	return chainhash.DoubleHashH(buf.Bytes())
}

// PivxDecode to decode bitcoin tx and extra
func (msg *PivxMsgTx) PivxDecode(r *bytes.Reader, pver uint32, enc wire.MessageEncoding) error {
	if err := msg.MsgTx.BtcDecode(r, pver, enc); err != nil {
		return err
	}

	// extra
	version := uint32(msg.MsgTx.Version)
	txVersion := version & 0xffff

	var sectionBuf bytes.Buffer
	tee := io.TeeReader(r, &sectionBuf)

	if txVersion >= 3 {
		// valueBalance (9 bytes)
		buf := make([]byte, 9)
		if _, err := io.ReadFull(tee, buf); err != nil {
			return err
		}
		
		// vShieldedSpend
		vShieldedSpend, err := wire.ReadVarInt(tee, 0)
		if err != nil {
			return err
		}
		
		if vShieldedSpend > 0 {
			spendBytes := make([]byte, vShieldedSpend*384)
			if _, err := io.ReadFull(tee, spendBytes); err != nil {
				return err
			}
		}
		
		// vShieldOutput
		vShieldOutput, err := wire.ReadVarInt(tee, 0)
		if err != nil {
			return err
		}
		
		if vShieldOutput > 0 {
			outputBytes := make([]byte, vShieldOutput*948)
			if _, err := io.ReadFull(tee, outputBytes); err != nil {
				return err
			}
		}
		
		// bindingSig (64 bytes)
		sigBytes := make([]byte, 64)
		if _, err := io.ReadFull(tee, sigBytes); err != nil {
			return err
		}
		
		msg.Extra = append(msg.Extra, sectionBuf.Bytes()...)
	}

	return nil
}
