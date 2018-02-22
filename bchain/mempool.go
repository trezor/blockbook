package bchain

import (
	"encoding/hex"
	"sync"
	"time"

	"github.com/golang/glog"
)

type scriptIndex struct {
	script string
	n      uint32
}

type outpoint struct {
	txid string
	vout uint32
}

type inputOutput struct {
	outputScripts []scriptIndex
	inputs        []outpoint
}

// Mempool is mempool handle.
type Mempool struct {
	chain           *BitcoinRPC
	mux             sync.Mutex
	txToInputOutput map[string]inputOutput
	scriptToTx      map[string][]outpoint
	inputs          map[outpoint]string
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
	outpoints := m.scriptToTx[scriptHex]
	txs := make([]string, 0, len(outpoints)+len(outpoints)/2)
	for _, o := range outpoints {
		txs = append(txs, o.txid)
		i := m.inputs[o]
		if i != "" {
			txs = append(txs, i)
		}
	}
	return txs, nil
}

// GetInput returns transaction which spends given outpoint
func (m *Mempool) GetInput(outputTxid string, vout uint32) string {
	o := outpoint{txid: outputTxid, vout: vout}
	return m.inputs[o]
}

func (m *Mempool) updateMappings(newTxToInputOutput map[string]inputOutput, newScriptToTx map[string][]outpoint, newInputs map[outpoint]string) {
	m.mux.Lock()
	defer m.mux.Unlock()
	m.txToInputOutput = newTxToInputOutput
	m.scriptToTx = newScriptToTx
	m.inputs = newInputs
}

// Resync gets mempool transactions and maps output scripts to transactions.
// Resync is not reentrant, it should be called from a single thread.
// Read operations (GetTransactions) are safe.
func (m *Mempool) Resync(onNewTxAddr func(txid string, addr string)) error {
	start := time.Now()
	glog.V(1).Info("Mempool: resync")
	txs, err := m.chain.GetMempool()
	if err != nil {
		return err
	}
	newTxToInputOutput := make(map[string]inputOutput, len(m.txToInputOutput)+1)
	newScriptToTx := make(map[string][]outpoint, len(m.scriptToTx)+1)
	newInputs := make(map[outpoint]string, len(m.inputs)+1)
	for _, txid := range txs {
		io, exists := m.txToInputOutput[txid]
		if !exists {
			tx, err := m.chain.GetTransaction(txid)
			if err != nil {
				glog.Error("cannot get transaction ", txid, ": ", err)
				continue
			}
			io.outputScripts = make([]scriptIndex, 0, len(tx.Vout))
			for _, output := range tx.Vout {
				outputScript := output.ScriptPubKey.Hex
				if outputScript != "" {
					io.outputScripts = append(io.outputScripts, scriptIndex{outputScript, output.N})
				}
				if onNewTxAddr != nil && len(output.ScriptPubKey.Addresses) == 1 {
					onNewTxAddr(tx.Txid, output.ScriptPubKey.Addresses[0])
				}
			}
			io.inputs = make([]outpoint, 0, len(tx.Vin))
			for _, input := range tx.Vin {
				if input.Coinbase != "" {
					continue
				}
				io.inputs = append(io.inputs, outpoint{input.Txid, input.Vout})
			}
		}
		newTxToInputOutput[txid] = io
		for _, si := range io.outputScripts {
			newScriptToTx[si.script] = append(newScriptToTx[si.script], outpoint{txid, si.n})
		}
		for _, i := range io.inputs {
			newInputs[i] = txid
		}
	}
	m.updateMappings(newTxToInputOutput, newScriptToTx, newInputs)
	glog.Info("Mempool: resync finished in ", time.Since(start), ", ", len(m.txToInputOutput), " transactions in mempool")
	return nil
}
