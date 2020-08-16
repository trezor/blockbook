package bchain

import (
	"time"

	"github.com/golang/glog"
)

// MempoolBscType is mempool handle of BSC chains
type MempoolBscType struct {
	BaseMempool
	mempoolTimeoutTime   time.Duration
	queryBackendOnResync bool
	nextTimeoutRun       time.Time
}

// NewMempoolEthereumType creates new mempool handler.
func NewMempoolBscType(chain BlockChain, mempoolTxTimeoutHours int, queryBackendOnResync bool) *MempoolBscType {
	mempoolTimeoutTime := time.Duration(mempoolTxTimeoutHours) * time.Hour
	return &MempoolBscType{
		BaseMempool: BaseMempool{
			chain:        chain,
			txEntries:    make(map[string]txEntry),
			addrDescToTx: make(map[string][]Outpoint),
		},
		mempoolTimeoutTime:   mempoolTimeoutTime,
		queryBackendOnResync: queryBackendOnResync,
		nextTimeoutRun:       time.Now().Add(mempoolTimeoutTime),
	}
}

func (m *MempoolBscType) createTxEntry(txid string, txTime uint32) (txEntry, bool) {
	tx, err := m.chain.GetTransactionForMempool(txid)
	if err != nil {
		if err != ErrTxNotFound {
			glog.Warning("cannot get transaction ", txid, ": ", err)
		}
		return txEntry{}, false
	}
	mtx := m.txToMempoolTx(tx)
	parser := m.chain.GetChainParser()
	addrIndexes := make([]addrIndex, 0, len(mtx.Vout)+len(mtx.Vin))
	for _, output := range mtx.Vout {
		addrDesc, err := parser.GetAddrDescFromVout(&output)
		if err != nil {
			if err != ErrAddressMissing {
				glog.Error("error in output addrDesc in ", txid, " ", output.N, ": ", err)
			}
			continue
		}
		if len(addrDesc) > 0 {
			addrIndexes = append(addrIndexes, addrIndex{string(addrDesc), int32(output.N)})
		}
	}
	for j := range mtx.Vin {
		input := &mtx.Vin[j]
		for i, a := range input.Addresses {
			addrIndexes, input.AddrDesc = appendAddress(addrIndexes, ^int32(i), a, parser)
		}
	}
	//t, err := parser.EthereumTypeGetErc20FromTx(tx)
	th, err := m.chain.BscTypeGetTokenHub()
	if err != nil {
		glog.Error(err)
	}
	t, err := parser.BscTypeGetBEP20FromTx(tx, th)
	if err != nil {
		glog.Error("GetErc20FromTx for tx ", txid, ", ", err)
	} else {
		mtx.Erc20 = t
		for i := range t {
			addrIndexes, _ = appendAddress(addrIndexes, ^int32(i+1), t[i].From, parser)
			addrIndexes, _ = appendAddress(addrIndexes, int32(i+1), t[i].To, parser)
		}
	}
	if m.OnNewTxAddr != nil {
		sent := make(map[string]struct{})
		for _, si := range addrIndexes {
			if _, found := sent[si.addrDesc]; !found {
				m.OnNewTxAddr(tx, AddressDescriptor(si.addrDesc))
				sent[si.addrDesc] = struct{}{}
			}
		}
	}
	if m.OnNewTx != nil {
		m.OnNewTx(mtx)
	}
	return txEntry{addrIndexes: addrIndexes, time: txTime}, true
}

// Resync ethereum type removes timed out transactions and returns number of transactions in mempool.
// Transactions are added/removed by AddTransactionToMempool/RemoveTransactionFromMempool methods
func (m *MempoolBscType) Resync() (int, error) {
	if m.queryBackendOnResync {
		txs, err := m.chain.GetMempoolTransactions()
		if err != nil {
			return 0, err
		}
		for _, txid := range txs {
			m.AddTransactionToMempool(txid)
		}
	}
	m.mux.Lock()
	entries := len(m.txEntries)
	now := time.Now()
	if m.nextTimeoutRun.Before(now) {
		threshold := now.Add(-m.mempoolTimeoutTime)
		for txid, entry := range m.txEntries {
			if time.Unix(int64(entry.time), 0).Before(threshold) {
				m.removeEntryFromMempool(txid, entry)
			}
		}
		removed := entries - len(m.txEntries)
		entries = len(m.txEntries)
		glog.Info("Mempool: cleanup, removed ", removed, " transactions from mempool")
		m.nextTimeoutRun = now.Add(mempoolTimeoutRunPeriod)
	}
	m.mux.Unlock()
	glog.Info("Mempool: resync ", entries, " transactions in mempool")
	return entries, nil
}

// AddTransactionToMempool adds transactions to mempool
func (m *MempoolBscType) AddTransactionToMempool(txid string) {
	m.mux.Lock()
	_, exists := m.txEntries[txid]
	m.mux.Unlock()
	if glog.V(1) {
		glog.Info("AddTransactionToMempool ", txid, ", existed ", exists)
	}
	if !exists {
		entry, ok := m.createTxEntry(txid, uint32(time.Now().Unix()))
		if !ok {
			return
		}
		m.mux.Lock()
		m.txEntries[txid] = entry
		for _, si := range entry.addrIndexes {
			m.addrDescToTx[si.addrDesc] = append(m.addrDescToTx[si.addrDesc], Outpoint{txid, si.n})
		}
		m.mux.Unlock()
	}
}

// RemoveTransactionFromMempool removes transaction from mempool
func (m *MempoolBscType) RemoveTransactionFromMempool(txid string) {
	m.mux.Lock()
	entry, exists := m.txEntries[txid]
	if glog.V(1) {
		glog.Info("RemoveTransactionFromMempool ", txid, ", existed ", exists)
	}
	if exists {
		m.removeEntryFromMempool(txid, entry)
	}
	m.mux.Unlock()
}
