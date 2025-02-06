package bchain

import (
	"sort"
	"sync"
	"time"
)

type addrIndex struct {
	addrDesc string
	n        int32
}

type txEntry struct {
	addrIndexes []addrIndex
	time        uint32
	filter      string
}

type txidio struct {
	txid   string
	io     []addrIndex
	filter string
}

// BaseMempool is mempool base handle
type BaseMempool struct {
	chain        BlockChain
	mux          sync.Mutex
	txEntries    map[string]txEntry
	addrDescToTx map[string][]Outpoint
	OnNewTxAddr  OnNewTxAddrFunc
	OnNewTx      OnNewTxFunc
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

// removeEntryFromMempool removes entry from mempool structs. The caller is responsible for locking!
func (m *BaseMempool) removeEntryFromMempool(txid string, entry txEntry) {
	delete(m.txEntries, txid)
	// store already processed addrDesc - it can appear multiple times as a different outpoint
	processedAddrDesc := make(map[string]struct{})
	for _, si := range entry.addrIndexes {
		outpoints, found := m.addrDescToTx[si.addrDesc]
		if found {
			_, processed := processedAddrDesc[si.addrDesc]
			if !processed {
				processedAddrDesc[si.addrDesc] = struct{}{}
				j := 0
				for i := 0; i < len(outpoints); i++ {
					if outpoints[i].Txid != txid {
						outpoints[j] = outpoints[i]
						j++
					}
				}
				outpoints = outpoints[:j]
				if len(outpoints) > 0 {
					m.addrDescToTx[si.addrDesc] = outpoints
				} else {
					delete(m.addrDescToTx, si.addrDesc)
				}
			}
		}
	}
}

// GetAllEntries returns all mempool entries sorted by fist seen time in descending order
func (m *BaseMempool) GetAllEntries() MempoolTxidEntries {
	i := 0
	m.mux.Lock()
	entries := make(MempoolTxidEntries, len(m.txEntries))
	for txid, entry := range m.txEntries {
		entries[i] = MempoolTxidEntry{
			Txid: txid,
			Time: entry.time,
		}
		i++
	}
	m.mux.Unlock()
	sort.Sort(entries)
	return entries
}

// GetTransactionTime returns first seen time of a transaction
func (m *BaseMempool) GetTransactionTime(txid string) uint32 {
	m.mux.Lock()
	e, found := m.txEntries[txid]
	m.mux.Unlock()
	if !found {
		return 0
	}
	return e.time
}

func (m *BaseMempool) txToMempoolTx(tx *Tx) *MempoolTx {
	mtx := MempoolTx{
		Hex:              tx.Hex,
		Blocktime:        time.Now().Unix(),
		LockTime:         tx.LockTime,
		Txid:             tx.Txid,
		VSize:            tx.VSize,
		Version:          tx.Version,
		Vout:             tx.Vout,
		CoinSpecificData: tx.CoinSpecificData,
	}
	mtx.Vin = make([]MempoolVin, len(tx.Vin))
	for i, vin := range tx.Vin {
		mtx.Vin[i] = MempoolVin{
			Vin: vin,
		}
	}
	return &mtx
}
