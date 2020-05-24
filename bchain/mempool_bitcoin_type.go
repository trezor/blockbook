package bchain

import (
	"math/big"
	"time"

	"github.com/golang/glog"
)

type chanInputPayload struct {
	tx    *MempoolTx
	index int
}

// MempoolBitcoinType is mempool handle.
type MempoolBitcoinType struct {
	BaseMempool
	chanTxid            chan string
	chanAddrIndex       chan txidio
	AddrDescForOutpoint AddrDescForOutpointFunc
}

// NewMempoolBitcoinType creates new mempool handler.
// For now there is no cleanup of sync routines, the expectation is that the mempool is created only once per process
func NewMempoolBitcoinType(chain BlockChain, workers int, subworkers int) *MempoolBitcoinType {
	m := &MempoolBitcoinType{
		BaseMempool: BaseMempool{
			chain:        chain,
			txEntries:    make(map[string]txEntry),
			addrDescToTx: make(map[string][]Outpoint),
		},
		chanTxid:      make(chan string, 1),
		chanAddrIndex: make(chan txidio, 1),
	}
	for i := 0; i < workers; i++ {
		go func(i int) {
			chanInput := make(chan chanInputPayload, 1)
			chanResult := make(chan *addrIndex, 1)
			for j := 0; j < subworkers; j++ {
				go func(j int) {
					for payload := range chanInput {
						ai := m.getInputAddress(&payload)
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

func (m *MempoolBitcoinType) getInputAddress(payload *chanInputPayload) *addrIndex {
	var addrDesc AddressDescriptor
	var value *big.Int
	vin := &payload.tx.Vin[payload.index]
	if m.AddrDescForOutpoint != nil {
		addrDesc, value = m.AddrDescForOutpoint(Outpoint{vin.Txid, int32(vin.Vout)})
	}
	if addrDesc == nil {
		itx, err := m.chain.GetTransactionForMempool(vin.Txid)
		if err != nil {
			glog.Error("cannot get transaction ", vin.Txid, ": ", err)
			return nil
		}
		if int(vin.Vout) >= len(itx.Vout) {
			glog.Error("Vout len in transaction ", vin.Txid, " ", len(itx.Vout), " input.Vout=", vin.Vout)
			return nil
		}
		addrDesc, err = m.chain.GetChainParser().GetAddrDescFromVout(&itx.Vout[vin.Vout])
		if err != nil {
			glog.Error("error in addrDesc in ", vin.Txid, " ", vin.Vout, ": ", err)
			return nil
		}
		value = &itx.Vout[vin.Vout].ValueSat
	}
	vin.AddrDesc = addrDesc
	vin.ValueSat = *value
	return &addrIndex{string(addrDesc), ^int32(vin.Vout)}

}

func (m *MempoolBitcoinType) getTxAddrs(txid string, chanInput chan chanInputPayload, chanResult chan *addrIndex) ([]addrIndex, bool) {
	tx, err := m.chain.GetTransactionForMempool(txid)
	if err != nil {
		glog.Error("cannot get transaction ", txid, ": ", err)
		return nil, false
	}
	glog.V(2).Info("mempool: gettxaddrs ", txid, ", ", len(tx.Vin), " inputs")
	mtx := m.txToMempoolTx(tx)
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
		if m.OnNewTxAddr != nil {
			m.OnNewTxAddr(tx, addrDesc)
		}
	}
	dispatched := 0
	for i := range tx.Vin {
		input := &tx.Vin[i]
		if input.Coinbase != "" {
			continue
		}
		payload := chanInputPayload{mtx, i}
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
			case chanInput <- payload:
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
	if m.OnNewTx != nil {
		m.OnNewTx(mtx)
	}
	return io, true
}

// Resync gets mempool transactions and maps outputs to transactions.
// Resync is not reentrant, it should be called from a single thread.
// Read operations (GetTransactions) are safe.
func (m *MempoolBitcoinType) Resync() (int, error) {
	start := time.Now()
	glog.V(1).Info("mempool: resync")
	txs, err := m.chain.GetMempoolTransactions()
	if err != nil {
		return 0, err
	}
	glog.V(2).Info("mempool: resync ", len(txs), " txs")
	onNewEntry := func(txid string, entry txEntry) {
		if len(entry.addrIndexes) > 0 {
			m.mux.Lock()
			m.txEntries[txid] = entry
			for _, si := range entry.addrIndexes {
				m.addrDescToTx[si.addrDesc] = append(m.addrDescToTx[si.addrDesc], Outpoint{txid, si.n})
			}
			m.mux.Unlock()
		}
	}
	txsMap := make(map[string]struct{}, len(txs))
	dispatched := 0
	txTime := uint32(time.Now().Unix())
	// get transaction in parallel using goroutines created in NewUTXOMempool
	for _, txid := range txs {
		txsMap[txid] = struct{}{}
		_, exists := m.txEntries[txid]
		if !exists {
		loop:
			for {
				select {
				// store as many processed transactions as possible
				case tio := <-m.chanAddrIndex:
					onNewEntry(tio.txid, txEntry{tio.io, txTime})
					dispatched--
				// send transaction to be processed
				case m.chanTxid <- txid:
					dispatched++
					break loop
				}
			}
		}
	}
	for i := 0; i < dispatched; i++ {
		tio := <-m.chanAddrIndex
		onNewEntry(tio.txid, txEntry{tio.io, txTime})
	}

	for txid, entry := range m.txEntries {
		if _, exists := txsMap[txid]; !exists {
			m.mux.Lock()
			m.removeEntryFromMempool(txid, entry)
			m.mux.Unlock()
		}
	}
	glog.Info("mempool: resync finished in ", time.Since(start), ", ", len(m.txEntries), " transactions in mempool")
	return len(m.txEntries), nil
}
