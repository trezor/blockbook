package bchain

import (
	"sync"
	"time"

	"github.com/golang/glog"
)

// NonUTXOMempool is mempool handle of non UTXO chains
type NonUTXOMempool struct {
	chain           BlockChain
	mux             sync.Mutex
	txToInputOutput map[string][]addrIndex
	addrDescToTx      map[string][]outpoint
}

// NewNonUTXOMempool creates new mempool handler.
func NewNonUTXOMempool(chain BlockChain) *NonUTXOMempool {
	return &NonUTXOMempool{chain: chain}
}

// GetTransactions returns slice of mempool transactions for given address
func (m *NonUTXOMempool) GetTransactions(address string) ([]string, error) {
	parser := m.chain.GetChainParser()
	addrDesc, err := parser.GetAddrDescFromAddress(address)
	if err != nil {
		return nil, err
	}
	m.mux.Lock()
	defer m.mux.Unlock()
	outpoints := m.addrDescToTx[string(addrDesc)]
	txs := make([]string, 0, len(outpoints))
	for _, o := range outpoints {
		txs = append(txs, o.txid)
	}
	return txs, nil
}

func (m *NonUTXOMempool) updateMappings(newTxToInputOutput map[string][]addrIndex, newAddrDescToTx map[string][]outpoint) {
	m.mux.Lock()
	defer m.mux.Unlock()
	m.txToInputOutput = newTxToInputOutput
	m.addrDescToTx = newAddrDescToTx
}

// Resync gets mempool transactions and maps outputs to transactions.
// Resync is not reentrant, it should be called from a single thread.
// Read operations (GetTransactions) are safe.
func (m *NonUTXOMempool) Resync(onNewTxAddr func(txid string, addr string)) (int, error) {
	start := time.Now()
	glog.V(1).Info("Mempool: resync")
	txs, err := m.chain.GetMempool()
	if err != nil {
		return 0, err
	}
	parser := m.chain.GetChainParser()
	// allocate slightly larger capacity of the maps
	newTxToInputOutput := make(map[string][]addrIndex, len(m.txToInputOutput)+5)
	newAddrDescToTx := make(map[string][]outpoint, len(m.addrDescToTx)+5)
	for _, txid := range txs {
		io, exists := m.txToInputOutput[txid]
		if !exists {
			tx, err := m.chain.GetTransactionForMempool(txid)
			if err != nil {
				glog.Error("cannot get transaction ", txid, ": ", err)
				continue
			}
			io = make([]addrIndex, 0, len(tx.Vout)+len(tx.Vin))
			for _, output := range tx.Vout {
				addrDesc, err := parser.GetAddrDescFromVout(&output)
				if err != nil {
					if err != ErrAddressMissing {
						glog.Error("error in output addrDesc in ", txid, " ", output.N, ": ", err)
					}
					continue
				}
				if len(addrDesc) > 0 {
					io = append(io, addrIndex{string(addrDesc), int32(output.N)})
				}
				if onNewTxAddr != nil && len(output.ScriptPubKey.Addresses) == 1 {
					onNewTxAddr(tx.Txid, output.ScriptPubKey.Addresses[0])
				}
			}
			for _, input := range tx.Vin {
				for i, a := range input.Addresses {
					if len(a) > 0 {
						addrDesc, err := parser.GetAddrDescFromAddress(a)
						if err != nil {
							glog.Error("error in input addrDesc in ", txid, " ", a, ": ", err)
							continue
						}
						io = append(io, addrIndex{string(addrDesc), int32(^i)})
					}
				}
			}
		}
		newTxToInputOutput[txid] = io
		for _, si := range io {
			newAddrDescToTx[si.addrDesc] = append(newAddrDescToTx[si.addrDesc], outpoint{txid, si.n})
		}
	}
	m.updateMappings(newTxToInputOutput, newAddrDescToTx)
	glog.Info("Mempool: resync finished in ", time.Since(start), ", ", len(m.txToInputOutput), " transactions in mempool")
	return len(m.txToInputOutput), nil
}
