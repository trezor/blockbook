package btc

import (
	"github.com/trezor/blockbook/bchain"
	"bytes"
	"encoding/binary"
	"encoding/hex"

	vlq "github.com/bsm/go-vlq"
	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
)

// OutputScriptToAddressesFunc converts ScriptPubKey to bitcoin addresses
type OutputScriptToAddressesFunc func(script []byte, params *chaincfg.Params) ([]string, error)

// BitcoinParser handle
type BitcoinParser struct {
	*bchain.BaseParser
	Params                      *chaincfg.Params
	OutputScriptToAddressesFunc OutputScriptToAddressesFunc
}

// NewBitcoinParser returns new BitcoinParser instance
func NewBitcoinParser(params *chaincfg.Params, c *Configuration) *BitcoinParser {
	return &BitcoinParser{
		&bchain.BaseParser{
			AddressFactory:       bchain.NewBaseAddress,
			BlockAddressesToKeep: c.BlockAddressesToKeep,
		},
		params,
		outputScriptToAddresses,
	}
}

// GetChainParams contains network parameters for the main Bitcoin network,
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

// GetAddrIDFromVout returns internal address representation of given transaction output
func (p *BitcoinParser) GetAddrIDFromVout(output *bchain.Vout) ([]byte, error) {
	return hex.DecodeString(output.ScriptPubKey.Hex)
}

// GetAddrIDFromAddress returns internal address representation of given address
func (p *BitcoinParser) GetAddrIDFromAddress(address string) ([]byte, error) {
	return p.AddressToOutputScript(address)
}

// AddressToOutputScript converts bitcoin address to ScriptPubKey
func (p *BitcoinParser) AddressToOutputScript(address string) ([]byte, error) {
	da, err := btcutil.DecodeAddress(address, p.Params)
	if err != nil {
		return nil, err
	}
	script, err := txscript.PayToAddrScript(da)
	if err != nil {
		return nil, err
	}
	return script, nil
}

// OutputScriptToAddresses converts ScriptPubKey to bitcoin addresses
func (p *BitcoinParser) OutputScriptToAddresses(script []byte) ([]string, error) {
	return p.OutputScriptToAddressesFunc(script, p.Params)
}

// outputScriptToAddresses converts ScriptPubKey to bitcoin addresses
func outputScriptToAddresses(script []byte, params *chaincfg.Params) ([]string, error) {
	_, addresses, _, err := txscript.ExtractPkScriptAddrs(script, params)
	if err != nil {
		return nil, err
	}
	rv := make([]string, len(addresses))
	for i, a := range addresses {
		rv[i] = a.EncodeAddress()
	}
	return rv, nil
}

func (p *BitcoinParser) TxFromMsgTx(t *wire.MsgTx, parseAddresses bool) bchain.Tx {
	vin := make([]bchain.Vin, len(t.TxIn))
	for i, in := range t.TxIn {
		if blockchain.IsCoinBaseTx(t) {
			vin[i] = bchain.Vin{
				Coinbase: hex.EncodeToString(in.SignatureScript),
				Sequence: in.Sequence,
			}
			break
		}
		s := bchain.ScriptSig{
			Hex: hex.EncodeToString(in.SignatureScript),
			// missing: Asm,
		}
		vin[i] = bchain.Vin{
			Txid:      in.PreviousOutPoint.Hash.String(),
			Vout:      in.PreviousOutPoint.Index,
			Sequence:  in.Sequence,
			ScriptSig: s,
		}
	}
	vout := make([]bchain.Vout, len(t.TxOut))
	for i, out := range t.TxOut {
		addrs := []string{}
		if parseAddresses {
			addrs, _ = p.OutputScriptToAddresses(out.PkScript)
		}
		s := bchain.ScriptPubKey{
			Hex:       hex.EncodeToString(out.PkScript),
			Addresses: addrs,
			// missing: Asm,
			// missing: Type,
		}
		vout[i] = bchain.Vout{
			Value:        float64(out.Value) / 1E8,
			N:            uint32(i),
			ScriptPubKey: s,
		}
	}
	tx := bchain.Tx{
		Txid:     t.TxHash().String(),
		Version:  t.Version,
		LockTime: t.LockTime,
		Vin:      vin,
		Vout:     vout,
		// skip: BlockHash,
		// skip: Confirmations,
		// skip: Time,
		// skip: Blocktime,
	}
	return tx
}

// ParseTx parses byte array containing transaction and returns Tx struct
func (p *BitcoinParser) ParseTx(b []byte) (*bchain.Tx, error) {
	t := wire.MsgTx{}
	r := bytes.NewReader(b)
	if err := t.Deserialize(r); err != nil {
		return nil, err
	}
	tx := p.TxFromMsgTx(&t, true)
	tx.Hex = hex.EncodeToString(b)

	for i, vout := range tx.Vout {
		if len(vout.ScriptPubKey.Addresses) == 1 {
			a, err := p.AddressFactory(vout.ScriptPubKey.Addresses[0])
			if err != nil {
				return nil, err
			}
			tx.Vout[i].Address = a
		}
	}

	return &tx, nil
}

// ParseBlock parses raw block to our Block struct
func (p *BitcoinParser) ParseBlock(b []byte) (*bchain.Block, error) {
	w := wire.MsgBlock{}
	r := bytes.NewReader(b)

	if err := w.Deserialize(r); err != nil {
		return nil, err
	}

	txs := make([]bchain.Tx, len(w.Transactions))
	for ti, t := range w.Transactions {
		txs[ti] = p.TxFromMsgTx(t, false)
	}

	return &bchain.Block{Txs: txs}, nil
}

// PackTx packs transaction to byte array
func (p *BitcoinParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
	buf := make([]byte, 4+vlq.MaxLen64+len(tx.Hex)/2)
	binary.BigEndian.PutUint32(buf[0:4], height)
	vl := vlq.PutInt(buf[4:4+vlq.MaxLen64], blockTime)
	hl, err := hex.Decode(buf[4+vl:], []byte(tx.Hex))
	return buf[0 : 4+vl+hl], err
}

// UnpackTx unpacks transaction from byte array
func (p *BitcoinParser) UnpackTx(buf []byte) (*bchain.Tx, uint32, error) {
	height := binary.BigEndian.Uint32(buf)
	bt, l := vlq.Int(buf[4:])
	tx, err := p.ParseTx(buf[4+l:])
	if err != nil {
		return nil, 0, err
	}
	tx.Blocktime = bt

	return tx, height, nil
}
