package bchain

import (
	"sync"
	"time"

	"github.com/golang/glog"
)

// addrIndex and outpoint are used also in non utxo mempool
type addrIndex struct {
	addrID string
	n      int32
}

type outpoint struct {
	txid string
	vout int32
}

type inputOutput struct {
	outputs []addrIndex
	inputs  []outpoint
}

// UTXOMempool is mempool handle.
type UTXOMempool struct {
	chain           BlockChain
	mux             sync.Mutex
	txToInputOutput map[string]inputOutput
	addrIDToTx      map[string][]outpoint
	inputs          map[outpoint]string
}

// NewMempool creates new mempool handler.
func NewUTXOMempool(chain BlockChain) *UTXOMempool {
	return &UTXOMempool{chain: chain}
}

// GetTransactions returns slice of mempool transactions for given address
func (m *UTXOMempool) GetTransactions(address string) ([]string, error) {
	parser := m.chain.GetChainParser()
	addrID, err := parser.GetAddrIDFromAddress(address)
	if err != nil {
		return nil, err
	}
	m.mux.Lock()
	defer m.mux.Unlock()
	outpoints := m.addrIDToTx[string(addrID)]
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
func (m *UTXOMempool) GetSpentOutput(outputTxid string, vout uint32) string {
	o := outpoint{txid: outputTxid, vout: int32(vout)}
	return m.inputs[o]
}

func (m *UTXOMempool) updateMappings(newTxToInputOutput map[string]inputOutput, newAddrIDToTx map[string][]outpoint, newInputs map[outpoint]string) {
	m.mux.Lock()
	defer m.mux.Unlock()
	m.txToInputOutput = newTxToInputOutput
	m.addrIDToTx = newAddrIDToTx
	m.inputs = newInputs
}

// Resync gets mempool transactions and maps outputs to transactions.
// Resync is not reentrant, it should be called from a single thread.
// Read operations (GetTransactions) are safe.
func (m *UTXOMempool) Resync(onNewTxAddr func(txid string, addr string)) error {
	start := time.Now()
	glog.V(1).Info("Mempool: resync")
	txs, err := m.chain.GetMempool()
	if err != nil {
		return err
	}
	parser := m.chain.GetChainParser()
	newTxToInputOutput := make(map[string]inputOutput, len(m.txToInputOutput)+1)
	newAddrIDToTx := make(map[string][]outpoint, len(m.addrIDToTx)+1)
	newInputs := make(map[outpoint]string, len(m.inputs)+1)
	for _, txid := range txs {
		io, exists := m.txToInputOutput[txid]
		if !exists {
			tx, err := m.chain.GetTransaction(txid)
			if err != nil {
				glog.Error("cannot get transaction ", txid, ": ", err)
				continue
			}
			io.outputs = make([]addrIndex, 0, len(tx.Vout))
			for _, output := range tx.Vout {
				addrID, err := parser.GetAddrIDFromVout(&output)
				if err != nil {
					glog.Error("error in addrID in ", txid, " ", output.N, ": ", err)
					continue
				}
				if len(addrID) > 0 {
					io.outputs = append(io.outputs, addrIndex{string(addrID), int32(output.N)})
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
				io.inputs = append(io.inputs, outpoint{input.Txid, int32(input.Vout)})
			}
		}
		newTxToInputOutput[txid] = io
		for _, si := range io.outputs {
			newAddrIDToTx[si.addrID] = append(newAddrIDToTx[si.addrID], outpoint{txid, si.n})
		}
		for _, i := range io.inputs {
			newInputs[i] = txid
		}
	}
	m.updateMappings(newTxToInputOutput, newAddrIDToTx, newInputs)
	glog.Info("Mempool: resync finished in ", time.Since(start), ", ", len(m.txToInputOutput), " transactions in mempool")
	return nil
}
