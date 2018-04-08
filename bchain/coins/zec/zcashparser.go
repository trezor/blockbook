package zec

import (
	"blockbook/bchain"
	"bytes"
	"encoding/binary"
	"encoding/gob"
)

// ZCashParser handle
type ZCashParser struct {
	*bchain.BaseParser
}

// GetAddrIDFromVout returns internal address representation of given transaction output
func (p *ZCashParser) GetAddrIDFromVout(output *bchain.Vout) ([]byte, error) {
	if len(output.ScriptPubKey.Addresses) != 1 {
		return nil, nil
	}
	hash, _, err := CheckDecode(output.ScriptPubKey.Addresses[0])
	return hash, err
}

// GetAddrIDFromAddress returns internal address representation of given address
func (p *ZCashParser) GetAddrIDFromAddress(address string) ([]byte, error) {
	hash, _, err := CheckDecode(address)
	return hash, err
}

// PackTx packs transaction to byte array
func (p *ZCashParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, height)
	buf, err := encodeTx(buf, tx)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

func encodeTx(b []byte, tx *bchain.Tx) ([]byte, error) {
	buf := bytes.NewBuffer(b)
	enc := gob.NewEncoder(buf)
	err := enc.Encode(tx)
	return buf.Bytes(), err
}

// UnpackTx unpacks transaction from byte array
func (p *ZCashParser) UnpackTx(buf []byte) (*bchain.Tx, uint32, error) {
	height := binary.BigEndian.Uint32(buf)
	tx, err := decodeTx(buf[4:])
	if err != nil {
		return nil, 0, err
	}
	return tx, height, nil
}

func decodeTx(buf []byte) (*bchain.Tx, error) {
	tx := new(bchain.Tx)
	dec := gob.NewDecoder(bytes.NewBuffer(buf))
	err := dec.Decode(tx)
	if err != nil {
		return nil, err
	}
	return tx, nil
}
