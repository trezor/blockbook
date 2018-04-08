package zec

import (
	"blockbook/bchain"
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"encoding/hex"
	"errors"
)

// ZCashParser handle
type ZCashParser struct{}

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

// AddressToOutputScript converts address to ScriptPubKey - currently not implemented
func (p *ZCashParser) AddressToOutputScript(address string) ([]byte, error) {
	return nil, errors.New("AddressToOutputScript: not implemented")
}

// OutputScriptToAddresses converts ScriptPubKey to addresses - currently not implemented
func (p *ZCashParser) OutputScriptToAddresses(script []byte) ([]string, error) {
	return nil, errors.New("OutputScriptToAddresses: not implemented")
}

// ParseBlock parses raw block to our Block struct - currently not implemented
func (p *ZCashParser) ParseBlock(b []byte) (*bchain.Block, error) {
	return nil, errors.New("ParseBlock: not implemented")
}

// ParseTx parses byte array containing transaction and returns Tx struct - currently not implemented
func (p *ZCashParser) ParseTx(b []byte) (*bchain.Tx, error) {
	return nil, errors.New("ParseTx: not implemented")
}

// PackedTxidLen returns length in bytes of packed txid
func (p *ZCashParser) PackedTxidLen() int {
	return 32
}

// PackTxid packs txid to byte array
func (p *ZCashParser) PackTxid(txid string) ([]byte, error) {
	return hex.DecodeString(txid)
}

// UnpackTxid unpacks byte array to txid
func (p *ZCashParser) UnpackTxid(buf []byte) (string, error) {
	return hex.EncodeToString(buf), nil
}

// PackBlockHash packs block hash to byte array
func (p *ZCashParser) PackBlockHash(hash string) ([]byte, error) {
	return hex.DecodeString(hash)
}

// UnpackBlockHash unpacks byte array to block hash
func (p *ZCashParser) UnpackBlockHash(buf []byte) (string, error) {
	return hex.EncodeToString(buf), nil
}

// IsUTXOChain returns true if the block chain is UTXO type, otherwise false
func (p *ZCashParser) IsUTXOChain() bool {
	return true
}
