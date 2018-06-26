package api

import (
	"blockbook/bchain"
	"blockbook/db"
)

// Worker is handle to api worker
type Worker struct {
	db          *db.RocksDB
	txCache     *db.TxCache
	chain       bchain.BlockChain
	chainParser bchain.BlockChainParser
}

// NewWorker creates new api worker
func NewWorker(db *db.RocksDB, chain bchain.BlockChain, txCache *db.TxCache) (*Worker, error) {
	w := &Worker{
		db:          db,
		txCache:     txCache,
		chain:       chain,
		chainParser: chain.GetChainParser(),
	}
	return w, nil
}

func (w *Worker) GetTransaction(txid string, bestheight uint32, spendingTx bool) (*Tx, error) {
	bchainTx, height, err := w.txCache.GetTransaction(txid, bestheight)
	if err != nil {
		return nil, err
	}
	var blockhash string
	if bchainTx.Confirmations > 0 {
		blockhash, err = w.db.GetBlockHash(height)
		if err != nil {
			return nil, err
		}
	}
	var valIn, valOut, fees float64
	vins := make([]Vin, len(bchainTx.Vin))
	for i := range bchainTx.Vin {
		bchainVin := &bchainTx.Vin[i]
		vin := &vins[i]
		vin.Txid = bchainVin.Txid
		vin.N = i
		vin.Vout = bchainVin.Vout
		vin.ScriptSig.Hex = bchainVin.ScriptSig.Hex
		otx, _, err := w.txCache.GetTransaction(bchainVin.Txid, bestheight)
		if err != nil {
			return nil, err
		}
		if len(otx.Vout) > int(vin.Vout) {
			vout := &otx.Vout[vin.Vout]
			vin.Value = vout.Value
			valIn += vout.Value
			vin.ValueSat = int64(vout.Value*1E8 + 0.5)
			if vout.Address != nil {
				a := vout.Address.String()
				vin.Addr = a
			}
		}
	}
	vouts := make([]Vout, len(bchainTx.Vout))
	for i := range bchainTx.Vout {
		bchainVout := &bchainTx.Vout[i]
		vout := &vouts[i]
		vout.N = i
		vout.Value = bchainVout.Value
		valOut += bchainVout.Value
		vout.ScriptPubKey.Hex = bchainVout.ScriptPubKey.Hex
		vout.ScriptPubKey.Addresses = bchainVout.ScriptPubKey.Addresses
		if spendingTx {
			// TODO
		}
	}
	// for now do not return size, we would have to compute vsize of segwit transactions
	// size:=len(bchainTx.Hex) / 2
	fees = valIn - valOut
	r := &Tx{
		Blockhash:     blockhash,
		Blockheight:   int(height),
		Blocktime:     bchainTx.Blocktime,
		Confirmations: bchainTx.Confirmations,
		Fees:          fees,
		Locktime:      bchainTx.LockTime,
		Time:          bchainTx.Time,
		Txid:          txid,
		ValueIn:       valIn,
		ValueOut:      valOut,
		Version:       bchainTx.Version,
		Vin:           vins,
		Vout:          vouts,
	}
	return r, nil
}
