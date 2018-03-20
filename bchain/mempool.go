package bchain

import (
	"blockbook/common"
	"sync"
	"time"

	"github.com/golang/glog"
)

// TODO rename
type scriptIndex struct {
	script string
	n      uint32
}

type outpoint struct {
	txid string
	vout uint32
}

type inputOutput struct {
	outputs []scriptIndex
	inputs  []outpoint
}

// Mempool is mempool handle.
type Mempool struct {
	chain           BlockChain
	chainParser     BlockChainParser
	mux             sync.Mutex
	txToInputOutput map[string]inputOutput
	scriptToTx      map[string][]outpoint // TODO rename all occurences
	inputs          map[outpoint]string
	metrics         *common.Metrics
}

// NewMempool creates new mempool handler.
func NewMempool(chain BlockChain, metrics *common.Metrics) *Mempool {
	return &Mempool{chain: chain, chainParser: chain.GetChainParser(), metrics: metrics}
}

// GetTransactions returns slice of mempool transactions for given output script.
func (m *Mempool) GetTransactions(address string) ([]string, error) {
	m.mux.Lock()
	defer m.mux.Unlock()
	buf, err := m.chainParser.GetUIDFromAddress(address)
	if err != nil {
		return nil, err
	}
	outid := m.chainParser.UnpackUID(buf)
	outpoints := m.scriptToTx[outid]
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

// GetSpentOutput returns transaction which spends given outpoint
func (m *Mempool) GetSpentOutput(outputTxid string, vout uint32) string {
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
		m.metrics.MempoolResyncErrors.With(common.Labels{"error": err.Error()}).Inc()
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
				m.metrics.MempoolResyncErrors.With(common.Labels{"error": err.Error()}).Inc()
				glog.Error("cannot get transaction ", txid, ": ", err)
				continue
			}
			io.outputs = make([]scriptIndex, 0, len(tx.Vout))
			for _, output := range tx.Vout {
				outid := m.chainParser.GetUIDFromVout(&output)
				if outid != "" {
					io.outputs = append(io.outputs, scriptIndex{outid, output.N})
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
		for _, si := range io.outputs {
			newScriptToTx[si.script] = append(newScriptToTx[si.script], outpoint{txid, si.n})
		}
		for _, i := range io.inputs {
			newInputs[i] = txid
		}
	}
	m.updateMappings(newTxToInputOutput, newScriptToTx, newInputs)
	d := time.Since(start)
	glog.Info("Mempool: resync finished in ", d, ", ", len(m.txToInputOutput), " transactions in mempool")
	m.metrics.MempoolResyncDuration.Observe(float64(d) / 1e6) // in milliseconds
	return nil
}
