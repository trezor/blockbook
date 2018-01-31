package bchain

import (
	"encoding/hex"
	"sync"

	"github.com/golang/glog"
)

// Mempool is mempool handle.
type Mempool struct {
	chain      *BitcoinRPC
	mux        sync.Mutex
	scriptToTx map[string][]string
	txToScript map[string][]string
}

// NewMempool creates new mempool handler.
func NewMempool(chain *BitcoinRPC) *Mempool {
	return &Mempool{chain: chain}
}

// GetTransactions returns slice of mempool transactions for given output script.
func (m *Mempool) GetTransactions(outputScript []byte) ([]string, error) {
	m.mux.Lock()
	defer m.mux.Unlock()
	scriptHex := hex.EncodeToString(outputScript)
	return m.scriptToTx[scriptHex], nil
}

func (m *Mempool) updateMaps(newScriptToTx map[string][]string, newTxToScript map[string][]string) {
	m.mux.Lock()
	defer m.mux.Unlock()
	m.scriptToTx = newScriptToTx
	m.txToScript = newTxToScript
}

// Resync gets mempool transactions and maps output scripts to transactions.
// Resync is not reentrant, it should be called from a single thread.
// Read operations (GetTransactions) are safe.
func (m *Mempool) Resync() error {
	glog.Info("Mempool: resync")
	txs, err := m.chain.GetMempool()
	if err != nil {
		return err
	}
	newScriptToTx := make(map[string][]string)
	newTxToScript := make(map[string][]string)
	for _, txid := range txs {
		scripts := m.txToScript[txid]
		if scripts == nil {
			tx, err := m.chain.GetTransaction(txid)
			if err != nil {
				glog.Error("cannot get transaction ", txid, ": ", err)
				continue
			}
			scripts = make([]string, 0, len(tx.Vout))
			for _, output := range tx.Vout {
				outputScript := output.ScriptPubKey.Hex
				if outputScript != "" {
					scripts = append(scripts, outputScript)
				}
			}
		}
		newTxToScript[txid] = scripts
		for _, script := range scripts {
			newScriptToTx[script] = append(newScriptToTx[script], txid)
		}
	}
	m.updateMaps(newScriptToTx, newTxToScript)
	glog.Info("Mempool: resync finished, ", len(m.txToScript), " transactions in mempool")
	return nil
}
