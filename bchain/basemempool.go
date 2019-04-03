package bchain

import (
	"sort"
	"sync"
)

type addrIndex struct {
	addrDesc string
	n        int32
}

type txEntry struct {
	addrIndexes []addrIndex
	time        uint32
}

type txidio struct {
	txid string
	io   []addrIndex
}

// BaseMempool is mempool base handle
type BaseMempool struct {
	chain        BlockChain
	mux          sync.Mutex
	txEntries    map[string]txEntry
	addrDescToTx map[string][]Outpoint
	OnNewTxAddr  OnNewTxAddrFunc
}

// GetTransactions returns slice of mempool transactions for given address
func (m *BaseMempool) GetTransactions(address string) ([]Outpoint, error) {
	parser := m.chain.GetChainParser()
	addrDesc, err := parser.GetAddrDescFromAddress(address)
	if err != nil {
		return nil, err
	}
	return m.GetAddrDescTransactions(addrDesc)
}

// GetAddrDescTransactions returns slice of mempool transactions for given address descriptor, in reverse order
func (m *BaseMempool) GetAddrDescTransactions(addrDesc AddressDescriptor) ([]Outpoint, error) {
	m.mux.Lock()
	defer m.mux.Unlock()
	outpoints := m.addrDescToTx[string(addrDesc)]
	rv := make([]Outpoint, len(outpoints))
	for i, j := len(outpoints)-1, 0; i >= 0; i-- {
		rv[j] = outpoints[i]
		j++
	}
	return rv, nil
}

func (a MempoolTxidEntries) Len() int      { return len(a) }
func (a MempoolTxidEntries) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a MempoolTxidEntries) Less(i, j int) bool {
	// if the Time is equal, sort by txid to make the order defined
	hi := a[i].Time
	hj := a[j].Time
	if hi == hj {
		return a[i].Txid > a[j].Txid
	}
	// order in reverse
	return hi > hj
}

func (m *BaseMempool) updateMappings(newTxEntries map[string]txEntry, newAddrDescToTx map[string][]Outpoint) {
	m.mux.Lock()
	defer m.mux.Unlock()
	m.txEntries = newTxEntries
	m.addrDescToTx = newAddrDescToTx
}

func getAllEntries(txEntries map[string]txEntry) MempoolTxidEntries {
	a := make(MempoolTxidEntries, len(txEntries))
	i := 0
	for txid, entry := range txEntries {
		a[i] = MempoolTxidEntry{
			Txid: txid,
			Time: entry.time,
		}
		i++
	}
	sort.Sort(a)
	return a
}

// GetAllEntries returns all mempool entries sorted by fist seen time in descending order
func (m *BaseMempool) GetAllEntries() MempoolTxidEntries {
	return getAllEntries(m.txEntries)
}

// GetTransactionTime returns first seen time of a transaction
func (m *BaseMempool) GetTransactionTime(txid string) uint32 {
	e, found := m.txEntries[txid]
	if !found {
		return 0
	}
	return e.time
}
