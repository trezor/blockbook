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

// UTXOMempool is mempool handle.
type UTXOMempool struct {
	chain           BlockChain
	mux             sync.Mutex
	txToInputOutput map[string][]addrIndex
	addrIDToTx      map[string][]outpoint
}

// NewUTXOMempool creates new mempool handler.
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
	txs := make([]string, 0, len(outpoints))
	for _, o := range outpoints {
		txs = append(txs, o.txid)
	}
	return txs, nil
}

func (m *UTXOMempool) updateMappings(newTxToInputOutput map[string][]addrIndex, newAddrIDToTx map[string][]outpoint) {
	m.mux.Lock()
	defer m.mux.Unlock()
	m.txToInputOutput = newTxToInputOutput
	m.addrIDToTx = newAddrIDToTx
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
	// allocate slightly larger capacity of the maps
	newTxToInputOutput := make(map[string][]addrIndex, len(m.txToInputOutput)+5)
	newAddrIDToTx := make(map[string][]outpoint, len(m.addrIDToTx)+5)
	for _, txid := range txs {
		io, exists := m.txToInputOutput[txid]
		if !exists {
			tx, err := m.chain.GetTransaction(txid)
			if err != nil {
				glog.Error("cannot get transaction ", txid, ": ", err)
				continue
			}
			io = make([]addrIndex, 0, len(tx.Vout)+len(tx.Vin))
			for _, output := range tx.Vout {
				addrID, err := parser.GetAddrIDFromVout(&output)
				if err != nil {
					glog.Error("error in addrID in ", txid, " ", output.N, ": ", err)
					continue
				}
				if len(addrID) > 0 {
					io = append(io, addrIndex{string(addrID), int32(output.N)})
				}
				if onNewTxAddr != nil && len(output.ScriptPubKey.Addresses) == 1 {
					onNewTxAddr(tx.Txid, output.ScriptPubKey.Addresses[0])
				}
			}
			for _, input := range tx.Vin {
				if input.Coinbase != "" {
					continue
				}
				// TODO - possibly get from DB unspenttxs - however some output txs can be in mempool only
				itx, err := m.chain.GetTransaction(input.Txid)
				if err != nil {
					glog.Error("cannot get transaction ", input.Txid, ": ", err)
					continue
				}
				if int(input.Vout) >= len(itx.Vout) {
					glog.Error("Vout len in transaction ", input.Txid, " ", len(itx.Vout), " input.Vout=", input.Vout)
					continue
				}
				addrID, err := parser.GetAddrIDFromVout(&itx.Vout[input.Vout])
				if err != nil {
					glog.Error("error in addrID in ", input.Txid, " ", input.Vout, ": ", err)
					continue
				}
				io = append(io, addrIndex{string(addrID), int32(^input.Vout)})
			}
		}
		newTxToInputOutput[txid] = io
		for _, si := range io {
			newAddrIDToTx[si.addrID] = append(newAddrIDToTx[si.addrID], outpoint{txid, si.n})
		}
	}
	m.updateMappings(newTxToInputOutput, newAddrIDToTx)
	glog.Info("Mempool: resync finished in ", time.Since(start), ", ", len(m.txToInputOutput), " transactions in mempool")
	return nil
}
