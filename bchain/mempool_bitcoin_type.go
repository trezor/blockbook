package bchain

import (
	"sync"
	"time"

	"github.com/golang/glog"
)

type addrIndex struct {
	addrDesc string
	n        int32
}

type txidio struct {
	txid string
	io   []addrIndex
}

// MempoolBitcoinType is mempool handle.
type MempoolBitcoinType struct {
	chain           BlockChain
	mux             sync.Mutex
	txToInputOutput map[string][]addrIndex
	addrDescToTx    map[string][]Outpoint
	chanTxid        chan string
	chanAddrIndex   chan txidio
	onNewTxAddr     OnNewTxAddrFunc
}

// NewMempoolBitcoinType creates new mempool handler.
// For now there is no cleanup of sync routines, the expectation is that the mempool is created only once per process
func NewMempoolBitcoinType(chain BlockChain, workers int, subworkers int) *MempoolBitcoinType {
	m := &MempoolBitcoinType{
		chain:         chain,
		chanTxid:      make(chan string, 1),
		chanAddrIndex: make(chan txidio, 1),
	}
	for i := 0; i < workers; i++ {
		go func(i int) {
			chanInput := make(chan Outpoint, 1)
			chanResult := make(chan *addrIndex, 1)
			for j := 0; j < subworkers; j++ {
				go func(j int) {
					for input := range chanInput {
						ai := m.getInputAddress(input)
						chanResult <- ai
					}
				}(j)
			}
			for txid := range m.chanTxid {
				io, ok := m.getTxAddrs(txid, chanInput, chanResult)
				if !ok {
					io = []addrIndex{}
				}
				m.chanAddrIndex <- txidio{txid, io}
			}
		}(i)
	}
	glog.Info("mempool: starting with ", workers, "*", subworkers, " sync workers")
	return m
}

// GetTransactions returns slice of mempool transactions for given address
func (m *MempoolBitcoinType) GetTransactions(address string) ([]Outpoint, error) {
	parser := m.chain.GetChainParser()
	addrDesc, err := parser.GetAddrDescFromAddress(address)
	if err != nil {
		return nil, err
	}
	return m.GetAddrDescTransactions(addrDesc)
}

// GetAddrDescTransactions returns slice of mempool transactions for given address descriptor
func (m *MempoolBitcoinType) GetAddrDescTransactions(addrDesc AddressDescriptor) ([]Outpoint, error) {
	m.mux.Lock()
	defer m.mux.Unlock()
	return append([]Outpoint(nil), m.addrDescToTx[string(addrDesc)]...), nil
}

func (m *MempoolBitcoinType) updateMappings(newTxToInputOutput map[string][]addrIndex, newAddrDescToTx map[string][]Outpoint) {
	m.mux.Lock()
	defer m.mux.Unlock()
	m.txToInputOutput = newTxToInputOutput
	m.addrDescToTx = newAddrDescToTx
}

func (m *MempoolBitcoinType) getInputAddress(input Outpoint) *addrIndex {
	itx, err := m.chain.GetTransactionForMempool(input.Txid)
	if err != nil {
		glog.Error("cannot get transaction ", input.Txid, ": ", err)
		return nil
	}
	if int(input.Vout) >= len(itx.Vout) {
		glog.Error("Vout len in transaction ", input.Txid, " ", len(itx.Vout), " input.Vout=", input.Vout)
		return nil
	}
	addrDesc, err := m.chain.GetChainParser().GetAddrDescFromVout(&itx.Vout[input.Vout])
	if err != nil {
		glog.Error("error in addrDesc in ", input.Txid, " ", input.Vout, ": ", err)
		return nil
	}
	return &addrIndex{string(addrDesc), ^input.Vout}

}

func (m *MempoolBitcoinType) getTxAddrs(txid string, chanInput chan Outpoint, chanResult chan *addrIndex) ([]addrIndex, bool) {
	tx, err := m.chain.GetTransactionForMempool(txid)
	if err != nil {
		glog.Error("cannot get transaction ", txid, ": ", err)
		return nil, false
	}
	glog.V(2).Info("mempool: gettxaddrs ", txid, ", ", len(tx.Vin), " inputs")
	io := make([]addrIndex, 0, len(tx.Vout)+len(tx.Vin))
	for _, output := range tx.Vout {
		addrDesc, err := m.chain.GetChainParser().GetAddrDescFromVout(&output)
		if err != nil {
			glog.Error("error in addrDesc in ", txid, " ", output.N, ": ", err)
			continue
		}
		if len(addrDesc) > 0 {
			io = append(io, addrIndex{string(addrDesc), int32(output.N)})
		}
		if m.onNewTxAddr != nil {
			m.onNewTxAddr(tx, addrDesc)
		}
	}
	dispatched := 0
	for _, input := range tx.Vin {
		if input.Coinbase != "" {
			continue
		}
		o := Outpoint{input.Txid, int32(input.Vout)}
	loop:
		for {
			select {
			// store as many processed results as possible
			case ai := <-chanResult:
				if ai != nil {
					io = append(io, *ai)
				}
				dispatched--
			// send input to be processed
			case chanInput <- o:
				dispatched++
				break loop
			}
		}
	}
	for i := 0; i < dispatched; i++ {
		ai := <-chanResult
		if ai != nil {
			io = append(io, *ai)
		}
	}
	return io, true
}

// Resync gets mempool transactions and maps outputs to transactions.
// Resync is not reentrant, it should be called from a single thread.
// Read operations (GetTransactions) are safe.
func (m *MempoolBitcoinType) Resync(onNewTxAddr OnNewTxAddrFunc) (int, error) {
	start := time.Now()
	glog.V(1).Info("mempool: resync")
	m.onNewTxAddr = onNewTxAddr
	txs, err := m.chain.GetMempool()
	if err != nil {
		return 0, err
	}
	glog.V(2).Info("mempool: resync ", len(txs), " txs")
	// allocate slightly larger capacity of the maps
	newTxToInputOutput := make(map[string][]addrIndex, len(m.txToInputOutput)+5)
	newAddrDescToTx := make(map[string][]Outpoint, len(m.addrDescToTx)+5)
	dispatched := 0
	onNewData := func(txid string, io []addrIndex) {
		if len(io) > 0 {
			newTxToInputOutput[txid] = io
			for _, si := range io {
				newAddrDescToTx[si.addrDesc] = append(newAddrDescToTx[si.addrDesc], Outpoint{txid, si.n})
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
	m.updateMappings(newTxToInputOutput, newAddrDescToTx)
	m.onNewTxAddr = nil
	glog.Info("mempool: resync finished in ", time.Since(start), ", ", len(m.txToInputOutput), " transactions in mempool")
	return len(m.txToInputOutput), nil
}
