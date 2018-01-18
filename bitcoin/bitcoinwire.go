package bitcoin

import (
	"bytes"
	"encoding/hex"

	"github.com/btcsuite/btcd/blockchain"

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

func (p *BitcoinBlockParser) parseOutputScript(b []byte) ([]string, error) {
	_, addresses, _, err := txscript.ExtractPkScriptAddrs(b, p.Params)
	if err != nil {
		return nil, err
	}
	addrs := make([]string, len(addresses))
	for i, a := range addresses {
		addrs[i] = a.EncodeAddress()
	}
	return addrs, nil
}

func (p *BitcoinBlockParser) ParseBlock(b []byte) (*Block, error) {
	w := wire.MsgBlock{}
	r := bytes.NewReader(b)

	if err := w.Deserialize(r); err != nil {
		return nil, err
	}

	txs := make([]Tx, len(w.Transactions))
	for ti, t := range w.Transactions {
		vin := make([]Vin, len(t.TxIn))
		for i, in := range t.TxIn {
			if blockchain.IsCoinBaseTx(t) {
				vin[i] = Vin{
					Coinbase: hex.EncodeToString(in.SignatureScript),
					Sequence: in.Sequence,
				}
				break
			}
			s := ScriptSig{
				Hex: hex.EncodeToString(in.SignatureScript),
				// missing: Asm,
			}
			vin[i] = Vin{
				Txid:      in.PreviousOutPoint.Hash.String(),
				Vout:      in.PreviousOutPoint.Index,
				Sequence:  in.Sequence,
				ScriptSig: s,
			}
		}
		vout := make([]Vout, len(t.TxOut))
		for i, out := range t.TxOut {
			addrs, err := p.parseOutputScript(out.PkScript)
			if err != nil {
				addrs = []string{}
			}
			s := ScriptPubKey{
				Hex:       hex.EncodeToString(out.PkScript),
				Addresses: addrs,
				// missing: Asm,
				// missing: Type,
			}
			vout[i] = Vout{
				Value:        float64(out.Value),
				N:            uint32(i),
				ScriptPubKey: s,
			}
		}
		txs[ti] = Tx{
			Txid:     t.TxHash().String(),
			Version:  t.Version,
			LockTime: t.LockTime,
			Vin:      vin,
			Vout:     vout,
			// missing: BlockHash,
			// missing: Confirmations,
			// missing: Time,
			// missing: Blocktime,
		}
	}

	return &Block{Txs: txs}, nil
}
