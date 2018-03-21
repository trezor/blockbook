package zec

import (
	"blockbook/bchain"
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"errors"
)

type ZCashBlockParser struct{}

func (p *ZCashBlockParser) GetUIDFromVout(output *bchain.Vout) string {
	if len(output.ScriptPubKey.Addresses) != 1 {
		return ""
	}
	return output.ScriptPubKey.Addresses[0]
}

func (p *ZCashBlockParser) GetUIDFromAddress(address string) ([]byte, error) {
	return p.PackUID(address)
}

func (p *ZCashBlockParser) PackUID(str string) ([]byte, error) {
	return []byte(str), nil
}

func (p *ZCashBlockParser) UnpackUID(buf []byte) string {
	return string(buf)
}

// PackTx packs transaction to byte array
func (p *ZCashBlockParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
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
func (p *ZCashBlockParser) UnpackTx(buf []byte) (*bchain.Tx, uint32, error) {
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

func (p *ZCashBlockParser) AddressToOutputScript(address string) ([]byte, error) {
	return nil, errors.New("AddressToOutputScript: not implemented")
}

func (p *ZCashBlockParser) OutputScriptToAddresses(script []byte) ([]string, error) {
	return nil, errors.New("OutputScriptToAddresses: not implemented")
}

func (p *ZCashBlockParser) ParseBlock(b []byte) (*bchain.Block, error) {
	return nil, errors.New("ParseBlock: not implemented")
}

func (p *ZCashBlockParser) ParseTx(b []byte) (*bchain.Tx, error) {
	return nil, errors.New("ParseTx: not implemented")
}
