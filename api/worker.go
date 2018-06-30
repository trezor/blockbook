package api

import (
	"blockbook/bchain"
	"blockbook/common"
	"blockbook/db"

	"github.com/golang/glog"
)

// Worker is handle to api worker
type Worker struct {
	db          *db.RocksDB
	txCache     *db.TxCache
	chain       bchain.BlockChain
	chainParser bchain.BlockChainParser
	is          *common.InternalState
}

// NewWorker creates new api worker
func NewWorker(db *db.RocksDB, chain bchain.BlockChain, txCache *db.TxCache, is *common.InternalState) (*Worker, error) {
	w := &Worker{
		db:          db,
		txCache:     txCache,
		chain:       chain,
		chainParser: chain.GetChainParser(),
		is:          is,
	}
	return w, nil
}

// GetTransaction reads transaction data from txid
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
		//  bchainVin.Txid=="" is coinbase transaction
		if bchainVin.Txid != "" {
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
	// for coinbase transactions valIn is 0
	fees = valIn - valOut
	if fees < 0 {
		fees = 0
	}
	// for now do not return size, we would have to compute vsize of segwit transactions
	// size:=len(bchainTx.Hex) / 2
	r := &Tx{
		Blockhash:     blockhash,
		Blockheight:   int(height),
		Blocktime:     bchainTx.Blocktime,
		Confirmations: bchainTx.Confirmations,
		Fees:          fees,
		Locktime:      bchainTx.LockTime,
		WithSpends:    spendingTx,
		Time:          bchainTx.Time,
		Txid:          bchainTx.Txid,
		ValueIn:       valIn,
		ValueOut:      valOut,
		Version:       bchainTx.Version,
		Vin:           vins,
		Vout:          vouts,
	}
	return r, nil
}

func (s *Worker) getAddressTxids(address string, mempool bool) ([]string, error) {
	var err error
	txids := make([]string, 0)
	if !mempool {
		err = s.db.GetTransactions(address, 0, ^uint32(0), func(txid string, vout uint32, isOutput bool) error {
			txids = append(txids, txid)
			return nil
		})
		if err != nil {
			return nil, err
		}
	} else {
		m, err := s.chain.GetMempoolTransactions(address)
		if err != nil {
			return nil, err
		}
		txids = append(txids, m...)
	}
	return txids, nil
}

func (t *Tx) getAddrVoutValue(addrID string) float64 {
	var val float64
	for _, vout := range t.Vout {
		for _, a := range vout.ScriptPubKey.Addresses {
			if a == addrID {
				val += vout.Value
			}
		}
	}
	return val
}

func (t *Tx) getAddrVinValue(addrID string) float64 {
	var val float64
	for _, vin := range t.Vin {
		if vin.Addr == addrID {
			val += vin.Value
		}
	}
	return val
}

// GetAddress computes address value and gets transactions for given address
func (w *Worker) GetAddress(addrID string) (*Address, error) {
	glog.Info(addrID, " start")
	txc, err := w.getAddressTxids(addrID, false)
	if err != nil {
		return nil, err
	}
	txm, err := w.getAddressTxids(addrID, true)
	if err != nil {
		return nil, err
	}
	bestheight, _, err := w.db.GetBestBlock()
	if err != nil {
		return nil, err
	}
	txs := make([]*Tx, len(txc)+len(txm))
	txi := 0
	var uBal, bal, totRecv, totSent float64
	for _, tx := range txm {
		tx, err := w.GetTransaction(tx, bestheight, false)
		// mempool transaction may fail
		if err != nil {
			glog.Error("GetTransaction ", tx, ": ", err)
		} else {
			txs[txi] = tx
			uBal = tx.getAddrVoutValue(addrID) - tx.getAddrVinValue(addrID)
			txi++
		}
	}
	for i := len(txc) - 1; i >= 0; i-- {
		tx, err := w.GetTransaction(txc[i], bestheight, false)
		if err != nil {
			return nil, err
		} else {
			txs[txi] = tx
			totRecv += tx.getAddrVoutValue(addrID)
			totSent += tx.getAddrVinValue(addrID)
			txi++
		}
	}
	bal = totRecv - totSent
	r := &Address{
		AddrStr:                 addrID,
		Balance:                 bal,
		BalanceSat:              int64(bal*1E8 + 0.5),
		TotalReceived:           totRecv,
		TotalReceivedSat:        int64(totRecv*1E8 + 0.5),
		TotalSent:               totSent,
		TotalSentSat:            int64(totSent*1E8 + 0.5),
		Transactions:            txs[:txi],
		TxApperances:            len(txc),
		UnconfirmedBalance:      uBal,
		UnconfirmedTxApperances: len(txm),
	}
	glog.Info(addrID, " finished")
	return r, nil
}
