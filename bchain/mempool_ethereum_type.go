package bchain

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
)

const mempoolTimeoutRunPeriod = 10 * time.Minute

// MempoolEthereumType is mempool handle of EthereumType chains
type MempoolEthereumType struct {
	BaseMempool
	mempoolTimeoutTime   time.Duration
	queryBackendOnResync bool
	nextTimeoutRun       time.Time
}

// NewMempoolEthereumType creates new mempool handler.
func NewMempoolEthereumType(chain BlockChain, mempoolTimeoutTime time.Duration, queryBackendOnResync bool) *MempoolEthereumType {
	return &MempoolEthereumType{
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

func appendAddress(io []addrIndex, i int32, a string, parser BlockChainParser) ([]addrIndex, AddressDescriptor) {
	var addrDesc AddressDescriptor
	var err error
	if len(a) > 0 {
		addrDesc, err = parser.GetAddrDescFromAddress(a)
		if err != nil {
			glog.Error("error in input addrDesc in ", a, ": ", err)
			return io, nil
		}
		io = append(io, addrIndex{string(addrDesc), i})
	}
	return io, addrDesc
}

// hasCalldata reports whether the transaction calls a contract, i.e. carries input data.
func hasCalldata(tx *Tx) bool {
	csd, ok := tx.CoinSpecificData.(EthereumSpecificData)
	return ok && csd.Tx != nil && len(strings.TrimPrefix(csd.Tx.Payload, "0x")) > 0
}

// createTxEntry builds the address index of a mempool transaction and announces it through OnNewTx.
// A private relay send is indexed under its sender and its plain counterparties only - the recipient
// of a value transfer and the parties of a parsed token transfer - never under a contract it calls:
// the relay was chosen so the call is not public before inclusion, and a subscription to a router
// address would otherwise hand out every such call as it is accepted.
func (m *MempoolEthereumType) createTxEntry(txid string, txTime uint32, private bool) (txEntry, bool) {
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
	if !private || !hasCalldata(tx) {
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
	}
	for j := range mtx.Vin {
		input := &mtx.Vin[j]
		for i, a := range input.Addresses {
			addrIndexes, input.AddrDesc = appendAddress(addrIndexes, ^int32(i), a, parser)
		}
	}
	t, err := parser.EthereumTypeGetTokenTransfersFromTx(tx)
	if err != nil {
		glog.Error("GetGetTokenTransfersFromTx for tx ", txid, ", ", err)
	} else {
		mtx.TokenTransfers = t
		for i := range t {
			addrIndexes, _ = appendAddress(addrIndexes, ^int32(i+1), t[i].From, parser)
			addrIndexes, _ = appendAddress(addrIndexes, int32(i+1), t[i].To, parser)
		}
	}
	if private {
		// the notification goes exactly where the index does
		mtx.Private = true
		mtx.NotifyAddrDescs = make([]AddressDescriptor, 0, len(addrIndexes))
		seen := make(map[string]struct{}, len(addrIndexes))
		for _, ai := range addrIndexes {
			if _, ok := seen[ai.addrDesc]; !ok {
				seen[ai.addrDesc] = struct{}{}
				mtx.NotifyAddrDescs = append(mtx.NotifyAddrDescs, AddressDescriptor(ai.addrDesc))
			}
		}
	}
	if m.OnNewTx != nil {
		m.OnNewTx(mtx)
	}
	entry := txEntry{addrIndexes: addrIndexes, time: txTime, private: private}
	if csd, ok := tx.CoinSpecificData.(EthereumSpecificData); ok && csd.Tx != nil && len(mtx.Vin) > 0 {
		nonce, err := strconv.ParseUint(strings.TrimPrefix(csd.Tx.AccountNonce, "0x"), 16, 64)
		if err != nil {
			glog.Warning("cannot parse nonce ", csd.Tx.AccountNonce, " of tx ", txid, ": ", err)
		} else {
			entry.from = string(mtx.Vin[0].AddrDesc)
			entry.nonce = nonce
		}
	}
	return entry, true
}

// RemoveSenderTransactionsUpToNonce retires every entry sent by from with a nonce at or below the
// mined one - no such transaction can ever mine, whatever the backend says about its pool presence.
// Returns the removed txids.
func (m *MempoolEthereumType) RemoveSenderTransactionsUpToNonce(from AddressDescriptor, nonce uint64) []string {
	m.mux.Lock()
	defer m.mux.Unlock()
	// collect first - removeEntryFromMempool compacts the outpoint slice being scanned
	var candidates []string
	for _, o := range m.addrDescToTx[string(from)] {
		if entry, ok := m.txEntries[o.Txid]; ok && entry.from != "" && entry.from == string(from) && entry.nonce <= nonce {
			candidates = append(candidates, o.Txid)
		}
	}
	var removed []string
	for _, txid := range candidates {
		// a self-transfer lists the txid twice under the same address
		if entry, ok := m.txEntries[txid]; ok {
			m.removeEntryFromMempool(txid, entry)
			removed = append(removed, txid)
		}
	}
	return removed
}

// Resync ethereum type removes timed out transactions and returns number of transactions in mempool.
// Transactions are added/removed by AddTransactionToMempool/RemoveTransactionFromMempool methods
func (m *MempoolEthereumType) Resync() (int, error) {
	start := time.Now()
	processedTxs := 0
	backendRemoved := 0
	if m.queryBackendOnResync {
		backendSnapshotTime := uint32(time.Now().Unix())
		txs, err := m.chain.GetMempoolTransactions()
		if err != nil {
			return 0, err
		}
		processedTxs = len(txs)
		backendTxs := make(map[string]struct{}, len(txs))
		for _, txid := range txs {
			backendTxs[txid] = struct{}{}
			m.AddTransactionToMempool(txid)
		}
		backendRemoved = m.removeTransactionsMissingFromBackend(backendTxs, backendSnapshotTime)
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
	duration := time.Since(start)
	durationRounded := duration.Round(time.Millisecond)
	if durationRounded == 0 {
		durationRounded = duration
	}
	if m.queryBackendOnResync {
		throughput := 0.0
		if seconds := duration.Seconds(); seconds > 0 {
			throughput = float64(processedTxs) / seconds
		}
		glog.Infof("Mempool: resync complete, mempool size %d txs, processed %d txs, removed %d stale txs, duration %s, throughput %.2f tx/s", entries, processedTxs, backendRemoved, durationRounded, throughput)
	} else {
		glog.Infof("Mempool: resync complete, mempool size %d txs, duration %s", entries, durationRounded)
	}
	return entries, nil
}

func (m *MempoolEthereumType) removeTransactionsMissingFromBackend(backendTxs map[string]struct{}, backendSnapshotTime uint32) int {
	removed := 0
	m.mux.Lock()
	defer m.mux.Unlock()
	for txid, entry := range m.txEntries {
		if _, exists := backendTxs[txid]; exists || entry.time >= backendSnapshotTime {
			continue
		}
		m.removeEntryFromMempool(txid, entry)
		removed++
	}
	return removed
}

// AddTransactionToMempool adds transactions to mempool, returns true if tx added to mempool, false if not added (for example duplicate call).
// An existing entry keeps its original time - the resync path re-adds every backend tx and the timeout
// sweep depends on the first-seen stamp; use AddOrRefreshTransactionInMempool to age from the newest add.
func (m *MempoolEthereumType) AddTransactionToMempool(txid string) bool {
	return m.addTransactionToMempool(txid, false)
}

func (m *MempoolEthereumType) addTransactionToMempool(txid string, private bool) bool {
	m.mux.Lock()
	_, exists := m.txEntries[txid]
	m.mux.Unlock()
	if glog.V(1) {
		glog.Info("AddTransactionToMempool ", txid, ", existed ", exists, ", private ", private)
	}
	if !exists {
		entry, ok := m.createTxEntry(txid, uint32(time.Now().Unix()), private)
		if !ok {
			return false
		}
		m.mux.Lock()
		m.txEntries[txid] = entry
		for _, si := range entry.addrIndexes {
			m.addrDescToTx[si.addrDesc] = append(m.addrDescToTx[si.addrDesc], Outpoint{txid, si.n})
		}
		m.mux.Unlock()
	}
	return !exists
}

// AddOrRefreshTransactionInMempool is AddTransactionToMempool for a caller whose own store ages from
// the newest submission (the alternative-provider cache): an existing entry is restamped in place, so
// the timeout sweep cannot drop it while that store still serves the transaction as pending.
func (m *MempoolEthereumType) AddOrRefreshTransactionInMempool(txid string) bool {
	return m.addOrRefreshTransactionInMempool(txid, false)
}

// AddOrRefreshPrivateTransactionInMempool is AddOrRefreshTransactionInMempool for a send accepted by
// the private relay: indexed under its sender and plain counterparties only (see createTxEntry) and
// left out of the mempool listing. An entry the public backend already surfaced stays public.
func (m *MempoolEthereumType) AddOrRefreshPrivateTransactionInMempool(txid string) bool {
	return m.addOrRefreshTransactionInMempool(txid, true)
}

func (m *MempoolEthereumType) addOrRefreshTransactionInMempool(txid string, private bool) bool {
	m.mux.Lock()
	entry, exists := m.txEntries[txid]
	if exists {
		entry.time = uint32(time.Now().Unix())
		m.txEntries[txid] = entry
	}
	m.mux.Unlock()
	if exists {
		return true
	}
	return m.addTransactionToMempool(txid, private)
}

// RemoveTransactionFromMempool removes transaction from mempool
func (m *MempoolEthereumType) RemoveTransactionFromMempool(txid string) {
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

// GetAllEntries returns the mempool entries sorted by first seen time in descending order, without
// private relay sends: the listing is public and the relay serves a body to anyone holding the hash.
func (m *MempoolEthereumType) GetAllEntries() MempoolTxidEntries {
	m.mux.Lock()
	entries := make(MempoolTxidEntries, 0, len(m.txEntries))
	for txid, entry := range m.txEntries {
		if entry.private {
			continue
		}
		entries = append(entries, MempoolTxidEntry{Txid: txid, Time: entry.time})
	}
	m.mux.Unlock()
	sort.Sort(entries)
	return entries
}

// GetTxidFilterEntries returns all mempool entries with golomb filter from
func (m *MempoolEthereumType) GetTxidFilterEntries(filterScripts string, fromTimestamp uint32) (MempoolTxidFilterEntries, error) {
	return MempoolTxidFilterEntries{}, errors.New("Not supported")
}
