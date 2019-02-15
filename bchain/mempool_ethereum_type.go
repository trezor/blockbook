package bchain

import (
	"sync"
	"time"

	"github.com/golang/glog"
)

// MempoolEthereumType is mempool handle of EthereumType chains
type MempoolEthereumType struct {
	chain           BlockChain
	mux             sync.Mutex
	txToInputOutput map[string][]addrIndex
	addrDescToTx    map[string][]Outpoint
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

func (m *MempoolEthereumType) updateMappings(newTxToInputOutput map[string][]addrIndex, newAddrDescToTx map[string][]Outpoint) {
	m.mux.Lock()
	defer m.mux.Unlock()
	m.txToInputOutput = newTxToInputOutput
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

// Resync gets mempool transactions and maps outputs to transactions.
// Resync is not reentrant, it should be called from a single thread.
// Read operations (GetTransactions) are safe.
func (m *MempoolEthereumType) Resync(onNewTxAddr OnNewTxAddrFunc) (int, error) {
	start := time.Now()
	glog.V(1).Info("Mempool: resync")
	txs, err := m.chain.GetMempool()
	if err != nil {
		return 0, err
	}
	parser := m.chain.GetChainParser()
	// allocate slightly larger capacity of the maps
	newTxToInputOutput := make(map[string][]addrIndex, len(m.txToInputOutput)+5)
	newAddrDescToTx := make(map[string][]Outpoint, len(m.addrDescToTx)+5)
	for _, txid := range txs {
		io, exists := m.txToInputOutput[txid]
		if !exists {
			tx, err := m.chain.GetTransactionForMempool(txid)
			if err != nil {
				if err != ErrTxNotFound {
					glog.Warning("cannot get transaction ", txid, ": ", err)
				}
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
			}
			for _, input := range tx.Vin {
				for i, a := range input.Addresses {
					appendAddress(io, ^int32(i), a, parser)
				}
			}
			t, err := parser.EthereumTypeGetErc20FromTx(tx)
			if err != nil {
				glog.Error("GetErc20FromTx for tx ", txid, ", ", err)
			} else {
				for i := range t {
					io = appendAddress(io, ^int32(i+1), t[i].From, parser)
					io = appendAddress(io, int32(i+1), t[i].To, parser)
				}
			}
			if onNewTxAddr != nil {
				sent := make(map[string]struct{})
				for _, si := range io {
					if _, found := sent[si.addrDesc]; !found {
						onNewTxAddr(tx, AddressDescriptor(si.addrDesc))
						sent[si.addrDesc] = struct{}{}
					}
				}
			}
		}
		newTxToInputOutput[txid] = io
		for _, si := range io {
			newAddrDescToTx[si.addrDesc] = append(newAddrDescToTx[si.addrDesc], Outpoint{txid, si.n})
		}
	}
	m.updateMappings(newTxToInputOutput, newAddrDescToTx)
	glog.Info("Mempool: resync finished in ", time.Since(start), ", ", len(m.txToInputOutput), " transactions in mempool")
	return len(m.txToInputOutput), nil
}
