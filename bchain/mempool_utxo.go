package bchain

import (
	"sync"
	"time"

	"github.com/golang/glog"
)

const numberOfSyncRoutines = 8

// addrIndex and outpoint are used also in non utxo mempool
type addrIndex struct {
	addrID string
	n      int32
}

type outpoint struct {
	txid string
	vout int32
}

type txidio struct {
	txid string
	io   []addrIndex
}

// UTXOMempool is mempool handle.
type UTXOMempool struct {
	chain           BlockChain
	mux             sync.Mutex
	txToInputOutput map[string][]addrIndex
	addrIDToTx      map[string][]outpoint
	chanTxid        chan string
	chanAddrIndex   chan txidio
	onNewTxAddr     func(txid string, addr string)
}

// NewUTXOMempool creates new mempool handler.
// For now there is no cleanup of sync routines, the expectation is that the mempool is created only once per process
func NewUTXOMempool(chain BlockChain) *UTXOMempool {
	m := &UTXOMempool{
		chain:         chain,
		chanTxid:      make(chan string, 1),
		chanAddrIndex: make(chan txidio, 1),
	}
	for i := 0; i < numberOfSyncRoutines; i++ {
		go func(i int) {
			for txid := range m.chanTxid {
				io, ok := m.getMempoolTxAddrs(txid)
				if !ok {
					io = []addrIndex{}
				}
				m.chanAddrIndex <- txidio{txid, io}
			}
		}(i)
	}
	glog.Info("mempool: starting with ", numberOfSyncRoutines, " sync workers")
	return m
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

func (m *UTXOMempool) getMempoolTxAddrs(txid string) ([]addrIndex, bool) {
	parser := m.chain.GetChainParser()
	tx, err := m.chain.GetTransactionForMempool(txid)
	if err != nil {
		glog.Error("cannot get transaction ", txid, ": ", err)
		return nil, false
	}
	io := make([]addrIndex, 0, len(tx.Vout)+len(tx.Vin))
	for _, output := range tx.Vout {
		addrID, err := parser.GetAddrIDFromVout(&output)
		if err != nil {
			glog.Error("error in addrID in ", txid, " ", output.N, ": ", err)
			continue
		}
		if len(addrID) > 0 {
			io = append(io, addrIndex{string(addrID), int32(output.N)})
		}
		if m.onNewTxAddr != nil && len(output.ScriptPubKey.Addresses) == 1 {
			m.onNewTxAddr(tx.Txid, output.ScriptPubKey.Addresses[0])
		}
	}
	for _, input := range tx.Vin {
		if input.Coinbase != "" {
			continue
		}
		// TODO - possibly get from DB unspenttxs - however some output txs can be also in mempool
		itx, err := m.chain.GetTransactionForMempool(input.Txid)
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
	return io, true
}

// Resync gets mempool transactions and maps outputs to transactions.
// Resync is not reentrant, it should be called from a single thread.
// Read operations (GetTransactions) are safe.
func (m *UTXOMempool) Resync(onNewTxAddr func(txid string, addr string)) error {
	start := time.Now()
	glog.V(1).Info("mempool: resync")
	m.onNewTxAddr = onNewTxAddr
	txs, err := m.chain.GetMempool()
	if err != nil {
		return err
	}
	// allocate slightly larger capacity of the maps
	newTxToInputOutput := make(map[string][]addrIndex, len(m.txToInputOutput)+5)
	newAddrIDToTx := make(map[string][]outpoint, len(m.addrIDToTx)+5)
	dispatched := 0
	onNewData := func(txid string, io []addrIndex) {
		if len(io) > 0 {
			newTxToInputOutput[txid] = io
			for _, si := range io {
				newAddrIDToTx[si.addrID] = append(newAddrIDToTx[si.addrID], outpoint{txid, si.n})
			}
		}
	}
	// get transaction in parallel using goroutines created in NewUTXOMempool
	for _, txid := range txs {
		io, exists := m.txToInputOutput[txid]
		if !exists {
		loop:
			for {
				select {
				// store as many processed transactions as possible
				case tio := <-m.chanAddrIndex:
					onNewData(tio.txid, tio.io)
					dispatched--
				// send transaction to be processed
				case m.chanTxid <- txid:
					dispatched++
					break loop
				}
			}
		} else {
			onNewData(txid, io)
		}
	}
	for i := 0; i < dispatched; i++ {
		tio := <-m.chanAddrIndex
		onNewData(tio.txid, tio.io)
	}
	m.updateMappings(newTxToInputOutput, newAddrIDToTx)
	m.onNewTxAddr = nil
	glog.Info("mempool: resync finished in ", time.Since(start), ", ", len(m.txToInputOutput), " transactions in mempool")
	return nil
}
