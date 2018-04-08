package bchain

import (
	"encoding/hex"
	"errors"
)

// BaseParser implements data parsing/handling functionality base for all other parsers
type BaseParser struct{}

// AddressToOutputScript converts address to ScriptPubKey - currently not implemented
func (p *BaseParser) AddressToOutputScript(address string) ([]byte, error) {
	return nil, errors.New("AddressToOutputScript: not implemented")
}

// OutputScriptToAddresses converts ScriptPubKey to addresses - currently not implemented
func (p *BaseParser) OutputScriptToAddresses(script []byte) ([]string, error) {
	return nil, errors.New("OutputScriptToAddresses: not implemented")
}

// ParseBlock parses raw block to our Block struct - currently not implemented
func (p *BaseParser) ParseBlock(b []byte) (*Block, error) {
	return nil, errors.New("ParseBlock: not implemented")
}

// ParseTx parses byte array containing transaction and returns Tx struct - currently not implemented
func (p *BaseParser) ParseTx(b []byte) (*Tx, error) {
	return nil, errors.New("ParseTx: not implemented")
}

// PackedTxidLen returns length in bytes of packed txid
func (p *BaseParser) PackedTxidLen() int {
	return 32
}

// PackTxid packs txid to byte array
func (p *BaseParser) PackTxid(txid string) ([]byte, error) {
	return hex.DecodeString(txid)
}

// UnpackTxid unpacks byte array to txid
func (p *BaseParser) UnpackTxid(buf []byte) (string, error) {
	return hex.EncodeToString(buf), nil
}

// PackBlockHash packs block hash to byte array
func (p *BaseParser) PackBlockHash(hash string) ([]byte, error) {
	return hex.DecodeString(hash)
}

// UnpackBlockHash unpacks byte array to block hash
func (p *BaseParser) UnpackBlockHash(buf []byte) (string, error) {
	return hex.EncodeToString(buf), nil
}

// IsUTXOChain returns true if the block chain is UTXO type, otherwise false
func (p *BaseParser) IsUTXOChain() bool {
	return true
}
