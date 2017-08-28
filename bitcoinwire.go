package main

import (
	"bytes"
	"encoding/hex"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

func GetChainParams() []*chaincfg.Params {
	return []*chaincfg.Params{
		&chaincfg.MainNetParams,
		&chaincfg.RegressionNetParams,
		&chaincfg.TestNet3Params,
		&chaincfg.SimNetParams,
	}
}

type BitcoinBlockParser struct {
	Params *chaincfg.Params
}

func (p *BitcoinBlockParser) parseOutputScript(b []byte) (addrs []string, err error) {
	_, addresses, _, err := txscript.ExtractPkScriptAddrs(b, p.Params)
	for _, a := range addresses {
		addr := a.EncodeAddress()
		addrs = append(addrs, addr)
	}
	return addrs, err
}

func (p *BitcoinBlockParser) ParseBlock(b []byte) (*Block, error) {
	w := wire.MsgBlock{}
	r := bytes.NewReader(b)
	if err := w.DeserializeNoWitness(r); err != nil {
		return nil, err
	}

	block := &Block{}
	for _, t := range w.Transactions {
		tx := &Tx{
			Txid:     t.TxHash().String(),
			Version:  t.Version,
			LockTime: t.LockTime,
			Vin:      make([]Vin, len(t.TxIn)),
			Vout:     make([]Vout, len(t.TxOut)),
			// missing: BlockHash,
			// missing: Confirmations,
			// missing: Time,
			// missing: Blocktime,
		}
		for i, in := range t.TxIn {
			s := ScriptSig{
				Hex: hex.EncodeToString(in.SignatureScript),
				// missing: Asm,
			}
			tx.Vin[i] = Vin{
				Coinbase:  "_",
				Txid:      in.PreviousOutPoint.Hash.String(),
				Vout:      in.PreviousOutPoint.Index,
				Sequence:  in.Sequence,
				ScriptSig: s,
			}
		}
		for i, out := range t.TxOut {
			addrs, err := p.parseOutputScript(out.PkScript)
			if err != nil {
				return nil, err
			}
			s := ScriptPubKey{
				Hex:       hex.EncodeToString(out.PkScript),
				Addresses: addrs,
				// missing: Asm,
				// missing: Type,
			}
			tx.Vout[i] = Vout{
				Value:        float64(out.Value),
				N:            uint32(i),
				ScriptPubKey: s,
			}
		}
		block.Txs = append(block.Txs, tx)
	}

	return block, nil
}
