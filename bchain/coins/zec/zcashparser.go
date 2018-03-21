package zec

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"bytes"
	"encoding/binary"
	"encoding/gob"

	"github.com/btcsuite/btcd/chaincfg"
)

// bitcoinwire parsing

type ZCashBlockParser struct {
	btc.BitcoinBlockParser
}

// getChainParams contains network parameters for the main Bitcoin network,
// the regression test Bitcoin network, the test Bitcoin network and
// the simulation test Bitcoin network, in this order
func GetChainParams(chain string) *chaincfg.Params {
	switch chain {
	case "test":
		return &chaincfg.TestNet3Params
	case "regtest":
		return &chaincfg.RegressionNetParams
	}
	return &chaincfg.MainNetParams
}

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
