package bchain

import (
	"sync"
	"time"

	"github.com/golang/glog"
)

// MempoolEthereumType is mempool handle of EthereumType chains
type MempoolEthereumType struct {
	chain        BlockChain
	mux          sync.Mutex
	txEntries    map[string]txEntry
	addrDescToTx map[string][]Outpoint
	OnNewTxAddr  OnNewTxAddrFunc
}

// NewMempoolEthereumType creates new mempool handler.
func NewMempoolEthereumType(chain BlockChain) *MempoolEthereumType {
	return &MempoolEthereumType{chain: chain}
}

// GetTransactions returns slice of mempool transactions for given address
func (m *MempoolEthereumType) GetTransactions(address string) ([]Outpoint, error) {
	parser := m.chain.GetChainParser()
	addrDesc, err := parser.GetAddrDescFromAddress(address)
	if err != nil {
		return nil, err
	}
	return m.GetAddrDescTransactions(addrDesc)
}

// GetAddrDescTransactions returns slice of mempool transactions for given address descriptor
func (m *MempoolEthereumType) GetAddrDescTransactions(addrDesc AddressDescriptor) ([]Outpoint, error) {
	m.mux.Lock()
	defer m.mux.Unlock()
	return append([]Outpoint(nil), m.addrDescToTx[string(addrDesc)]...), nil
}

func (m *MempoolEthereumType) updateMappings(newTxEntries map[string]txEntry, newAddrDescToTx map[string][]Outpoint) {
	m.mux.Lock()
	defer m.mux.Unlock()
	m.txEntries = newTxEntries
	m.addrDescToTx = newAddrDescToTx
}

func appendAddress(io []addrIndex, i int32, a string, parser BlockChainParser) []addrIndex {
	if len(a) > 0 {
		addrDesc, err := parser.GetAddrDescFromAddress(a)
		if err != nil {
			glog.Error("error in input addrDesc in ", a, ": ", err)
			return io
		}
		io = append(io, addrIndex{string(addrDesc), i})
	}
	return io
}

func (m *MempoolEthereumType) createTxEntry(txid string, txTime uint32) (txEntry, bool) {
	tx, err := m.chain.GetTransactionForMempool(txid)
	if err != nil {
		if err != ErrTxNotFound {
			glog.Warning("cannot get transaction ", txid, ": ", err)
		}
		return txEntry{}, false
	}
	parser := m.chain.GetChainParser()
	addrIndexes := make([]addrIndex, 0, len(tx.Vout)+len(tx.Vin))
	for _, output := range tx.Vout {
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
	for _, input := range tx.Vin {
		for i, a := range input.Addresses {
			addrIndexes = appendAddress(addrIndexes, ^int32(i), a, parser)
		}
	}
	t, err := parser.EthereumTypeGetErc20FromTx(tx)
	if err != nil {
		glog.Error("GetErc20FromTx for tx ", txid, ", ", err)
	} else {
		for i := range t {
			addrIndexes = appendAddress(addrIndexes, ^int32(i+1), t[i].From, parser)
			addrIndexes = appendAddress(addrIndexes, int32(i+1), t[i].To, parser)
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
	return txEntry{addrIndexes: addrIndexes, time: txTime}, true
}

// Resync gets mempool transactions and maps outputs to transactions.
// Resync is not reentrant, it should be called from a single thread.
// Read operations (GetTransactions) are safe.
func (m *MempoolEthereumType) Resync() (int, error) {
	start := time.Now()
	glog.V(1).Info("Mempool: resync")
	txs, err := m.chain.GetMempoolTransactions()
	if err != nil {
		return 0, err
	}
	// allocate slightly larger capacity of the maps
	newTxEntries := make(map[string]txEntry, len(m.txEntries)+5)
	newAddrDescToTx := make(map[string][]Outpoint, len(m.addrDescToTx)+5)
	txTime := uint32(time.Now().Unix())
	var ok bool
	for _, txid := range txs {
		entry, exists := m.txEntries[txid]
		if !exists {
			entry, ok = m.createTxEntry(txid, txTime)
			if !ok {
				continue
			}
		}
		newTxEntries[txid] = entry
		for _, si := range entry.addrIndexes {
			newAddrDescToTx[si.addrDesc] = append(newAddrDescToTx[si.addrDesc], Outpoint{txid, si.n})
		}
	}
	m.updateMappings(newTxEntries, newAddrDescToTx)
	glog.Info("Mempool: resync finished in ", time.Since(start), ", ", len(m.txEntries), " transactions in mempool")
	return len(m.txEntries), nil
}

// GetAllEntries returns all mempool entries sorted by fist seen time in descending order
func (m *MempoolEthereumType) GetAllEntries() MempoolTxidEntries {
	return getAllEntries(m.txEntries)
}
