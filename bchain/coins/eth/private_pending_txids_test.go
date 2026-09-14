package eth

import (
	"context"
	"testing"
	"time"

	"github.com/trezor/blockbook/bchain"
)

const (
	declaredSender    = "0x3333333333333333333333333333333333333333"
	declaredRecipient = "0x4444444444444444444444444444444444444444"
	declaredTxid      = "0x00000000000000000000000000000000000000000000000000000000000000aa"
)

// newDeclaredTestRPC wires a real MempoolEthereumType over a backend that serves only the given
// transactions, without indexing any of them - the shape of an instance that never saw the send.
func newDeclaredTestRPC(txs ...*bchain.RpcTransaction) (*EthereumRPC, *countingRPC) {
	rpc := &countingRPC{pendingTxRPC: pendingTxRPC{txs: map[string]*bchain.RpcTransaction{}}, calls: map[string]int{}}
	b := &EthereumRPC{RPC: rpc, Parser: NewEthereumParser(1, false), Timeout: time.Second, mempoolInitialized: true}
	b.Mempool = bchain.NewMempoolEthereumType(b, time.Hour, false)
	for _, tx := range txs {
		rpc.txs[tx.Hash] = tx
	}
	return b, rpc
}

// countingRPC counts the backend calls per method, so a test can assert that an already indexed
// txid costs nothing and that a mined one costs no receipt lookup.
type countingRPC struct {
	pendingTxRPC
	calls map[string]int
}

func (m *countingRPC) CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error {
	m.calls[method]++
	return m.pendingTxRPC.CallContext(ctx, result, method, args...)
}

func addrDescOf(t *testing.T, b *EthereumRPC, address string) bchain.AddressDescriptor {
	t.Helper()
	addrDesc, err := b.Parser.GetAddrDescFromAddress(address)
	if err != nil {
		t.Fatalf("GetAddrDescFromAddress(%s) error = %v", address, err)
	}
	return addrDesc
}

// A declared txid this instance never saw is fetched once and indexed as pending, so the address
// page that triggered the declaration already lists it (#1773).
func TestAddPendingTransactionsIndexesUnknownPendingTx(t *testing.T) {
	b, rpc := newDeclaredTestRPC(pendingTx(declaredTxid, declaredSender, declaredRecipient, "0x5"))
	var notified int
	b.Mempool.OnNewTx = func(*bchain.MempoolTx) { notified++ }

	added, err := b.EthereumTypeAddPendingTransactions(addrDescOf(t, b, declaredSender), []string{declaredTxid})
	if err != nil || added != 1 {
		t.Fatalf("EthereumTypeAddPendingTransactions() = (%d, %v), want (1, nil)", added, err)
	}
	if got := mempoolTxids(t, b, declaredSender); !sameTxids(got, declaredTxid) {
		t.Fatalf("sender mempool txids = %v, want [%s]", got, declaredTxid)
	}
	if got := mempoolTxids(t, b, declaredRecipient); !sameTxids(got, declaredTxid) {
		t.Fatalf("recipient mempool txids = %v, want [%s]", got, declaredTxid)
	}
	if notified != 1 {
		t.Fatalf("OnNewTx fired %d times, want 1 (address subscribers must be notified)", notified)
	}
	if rpc.calls["eth_getTransactionByHash"] != 1 || rpc.calls["eth_getTransactionReceipt"] != 0 {
		t.Fatalf("backend calls = %v, want exactly one eth_getTransactionByHash", rpc.calls)
	}
}

// An already indexed txid is answered from the index: no backend call, and the first-seen time is
// kept so a wallet that keeps declaring it cannot postpone the mempool timeout indefinitely.
func TestAddPendingTransactionsSkipsKnownTxidWithoutRpc(t *testing.T) {
	b, rpc := newDeclaredTestRPC(pendingTx(declaredTxid, declaredSender, declaredRecipient, "0x5"))
	if !b.Mempool.AddTransactionToMempool(declaredTxid) {
		t.Fatal("AddTransactionToMempool() = false, want the entry indexed")
	}
	firstSeen := b.Mempool.GetTransactionTime(declaredTxid)
	rpc.calls = map[string]int{}

	added, err := b.EthereumTypeAddPendingTransactions(addrDescOf(t, b, declaredSender), []string{declaredTxid})
	if err != nil || added != 0 {
		t.Fatalf("EthereumTypeAddPendingTransactions() = (%d, %v), want (0, nil)", added, err)
	}
	if len(rpc.calls) != 0 {
		t.Fatalf("backend calls = %v, want none for an already indexed txid", rpc.calls)
	}
	if got := b.Mempool.GetTransactionTime(declaredTxid); got != firstSeen {
		t.Fatalf("entry time = %d, want the first-seen %d (a declaration must not restamp)", got, firstSeen)
	}
}

// The mempool does not check whether a body is mined, so the mined guard lives here: a confirmed
// transaction must never be indexed as pending.
func TestAddPendingTransactionsIgnoresMinedTx(t *testing.T) {
	mined := pendingTx(declaredTxid, declaredSender, declaredRecipient, "0x5")
	mined.BlockNumber = "0x42"
	b, rpc := newDeclaredTestRPC(mined)

	added, err := b.EthereumTypeAddPendingTransactions(addrDescOf(t, b, declaredSender), []string{declaredTxid})
	if err != nil || added != 0 {
		t.Fatalf("EthereumTypeAddPendingTransactions() = (%d, %v), want (0, nil)", added, err)
	}
	if got := mempoolTxids(t, b, declaredSender); len(got) != 0 {
		t.Fatalf("sender mempool txids = %v, want none (a mined tx must not be indexed)", got)
	}
	if rpc.calls["eth_getTransactionReceipt"] != 0 {
		t.Fatalf("backend calls = %v, want no receipt lookup on the mined branch", rpc.calls)
	}
}

// A hash no backend knows costs one lookup and changes nothing; the wallet stops declaring it once
// its own page no longer lists it.
func TestAddPendingTransactionsIgnoresUnknownTx(t *testing.T) {
	b, rpc := newDeclaredTestRPC()

	added, err := b.EthereumTypeAddPendingTransactions(addrDescOf(t, b, declaredSender), []string{declaredTxid})
	if err != nil || added != 0 {
		t.Fatalf("EthereumTypeAddPendingTransactions() = (%d, %v), want (0, nil)", added, err)
	}
	if got := mempoolTxids(t, b, declaredSender); len(got) != 0 {
		t.Fatalf("sender mempool txids = %v, want none", got)
	}
	if rpc.calls["eth_getTransactionByHash"] != 1 {
		t.Fatalf("backend calls = %v, want exactly one lookup", rpc.calls)
	}
}

// The declaration names the wallet's own sends, so a pending transaction sent by somebody else is
// not indexed - a client can name a transaction, never inject one into a stranger's history.
func TestAddPendingTransactionsIgnoresForeignSender(t *testing.T) {
	b, _ := newDeclaredTestRPC(pendingTx(declaredTxid, declaredSender, declaredRecipient, "0x5"))

	added, err := b.EthereumTypeAddPendingTransactions(addrDescOf(t, b, declaredRecipient), []string{declaredTxid})
	if err != nil || added != 0 {
		t.Fatalf("EthereumTypeAddPendingTransactions() = (%d, %v), want (0, nil)", added, err)
	}
	if got := mempoolTxids(t, b, declaredSender); len(got) != 0 {
		t.Fatalf("sender mempool txids = %v, want none", got)
	}
}

// A transaction pending only in the relay's pool is served from the provider's cache, so the
// instance that accepted the send indexes it without asking the public node at all.
func TestAddPendingTransactionsUsesAlternativeProviderCache(t *testing.T) {
	b, rpc := newDeclaredTestRPC()
	b.alternativeSendTxProvider = &AlternativeSendTxProvider{
		fetchMempoolTx:    true,
		mempoolTxsTimeout: time.Hour,
		mempoolTxs: map[string]storedTx{
			declaredTxid: {tx: pendingTx(declaredTxid, declaredSender, declaredRecipient, "0x5"), time: uint32(time.Now().Unix())},
		},
	}

	added, err := b.EthereumTypeAddPendingTransactions(addrDescOf(t, b, declaredSender), []string{declaredTxid})
	if err != nil || added != 1 {
		t.Fatalf("EthereumTypeAddPendingTransactions() = (%d, %v), want (1, nil)", added, err)
	}
	if len(rpc.calls) != 0 {
		t.Fatalf("backend calls = %v, want none - the relay cache holds the body", rpc.calls)
	}
}

// Before the mempool exists there is nothing to index into, and the declaration must not fail the
// request that carried it.
func TestAddPendingTransactionsBeforeMempoolInitialized(t *testing.T) {
	b, rpc := newDeclaredTestRPC(pendingTx(declaredTxid, declaredSender, declaredRecipient, "0x5"))
	b.mempoolInitialized = false

	added, err := b.EthereumTypeAddPendingTransactions(addrDescOf(t, b, declaredSender), []string{declaredTxid})
	if err != nil || added != 0 {
		t.Fatalf("EthereumTypeAddPendingTransactions() = (%d, %v), want (0, nil)", added, err)
	}
	if len(rpc.calls) != 0 {
		t.Fatalf("backend calls = %v, want none before the mempool is initialized", rpc.calls)
	}
}

// A body the parser rejects is skipped, not propagated: the account request must still be answered.
func TestAddPendingTransactionsSkipsUndecodableBody(t *testing.T) {
	broken := pendingTx(declaredTxid, declaredSender, declaredRecipient, "0x5")
	broken.Value = "not-a-number"
	b, _ := newDeclaredTestRPC(broken)

	added, err := b.EthereumTypeAddPendingTransactions(addrDescOf(t, b, declaredSender), []string{declaredTxid})
	if err != nil || added != 0 {
		t.Fatalf("EthereumTypeAddPendingTransactions() = (%d, %v), want (0, nil)", added, err)
	}
	if got := mempoolTxids(t, b, declaredSender); len(got) != 0 {
		t.Fatalf("sender mempool txids = %v, want none", got)
	}
}
