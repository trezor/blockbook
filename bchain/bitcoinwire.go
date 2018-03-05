package bchain

import (
	"bytes"
	"encoding/hex"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcutil"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

// GetChainParams contains network parameters for the main Bitcoin network,
// the regression test Bitcoin network, the test Bitcoin network and
// the simulation test Bitcoin network, in this order
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

// AddressToOutputScript converts bitcoin address to ScriptPubKey
func AddressToOutputScript(address string) ([]byte, error) {
	da, err := btcutil.DecodeAddress(address, GetChainParams()[0])
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
func OutputScriptToAddresses(script []byte) ([]string, error) {
	_, addresses, _, err := txscript.ExtractPkScriptAddrs(script, GetChainParams()[0])
	if err != nil {
		return nil, err
	}
	rv := make([]string, len(addresses))
	for i, a := range addresses {
		rv[i] = a.EncodeAddress()
	}
	return rv, nil
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
			// addrs, err := OutputScriptToAddresses(out.PkScript)
			// if err != nil {
			// 	addrs = []string{}
			// }
			s := ScriptPubKey{
				Hex: hex.EncodeToString(out.PkScript),
				// missing Addresses,
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
			Txid: t.TxHash().String(),
			// skip: Version,
			LockTime: t.LockTime,
			Vin:      vin,
			Vout:     vout,
			// skip: BlockHash,
			// skip: Confirmations,
			// skip: Time,
			// skip: Blocktime,
		}
	}

	return &Block{Txs: txs}, nil
}
