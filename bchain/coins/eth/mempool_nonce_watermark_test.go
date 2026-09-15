package eth

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/trezor/blockbook/bchain"
)

// pendingTxRPC serves eth_getTransactionByHash from a map and answers null for anything else,
// including every receipt - the shape of a backend that only holds pending transactions.
type pendingTxRPC struct {
	txs map[string]*bchain.RpcTransaction
}

func (m *pendingTxRPC) EthSubscribe(ctx context.Context, channel interface{}, args ...interface{}) (bchain.EVMClientSubscription, error) {
	return nil, errors.New("not implemented")
}

func (m *pendingTxRPC) Close() {}

func (m *pendingTxRPC) CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error {
	raw := []byte("null")
	if method == "eth_getTransactionByHash" {
		if tx, ok := m.txs[args[0].(interface{ String() string }).String()]; ok {
			raw, _ = json.Marshal(tx)
		}
	}
	return json.Unmarshal(raw, result)
}

const (
	watermarkSender    = "0x1111111111111111111111111111111111111111"
	watermarkRecipient = "0x2222222222222222222222222222222222222222"
)

func pendingTx(hash, from, to, nonce string) *bchain.RpcTransaction {
	return &bchain.RpcTransaction{Hash: hash, From: from, To: to, AccountNonce: nonce, Value: "0x0", GasLimit: "0x5208", GasPrice: "0x1"}
}

// newWatermarkTestRPC wires a real MempoolEthereumType over the fake backend and indexes every
// transaction the backend serves, as the pending-tx subscription would.
func newWatermarkTestRPC(t *testing.T, txs ...*bchain.RpcTransaction) (*EthereumRPC, *pendingTxRPC) {
	t.Helper()
	rpc := &pendingTxRPC{txs: map[string]*bchain.RpcTransaction{}}
	b := &EthereumRPC{RPC: rpc, Parser: NewEthereumParser(1, false), Timeout: time.Second, mempoolInitialized: true}
	b.Mempool = bchain.NewMempoolEthereumType(b, time.Hour, false)
	for _, tx := range txs {
		rpc.txs[tx.Hash] = tx
		if !b.Mempool.AddTransactionToMempool(tx.Hash) {
			t.Fatalf("AddTransactionToMempool(%s) = false", tx.Hash)
		}
	}
	return b, rpc
}

func mempoolTxids(t *testing.T, b *EthereumRPC, address string) []string {
	t.Helper()
	outpoints, err := b.Mempool.GetTransactions(address)
	if err != nil {
		t.Fatalf("GetTransactions(%s) error = %v", address, err)
	}
	txids := make([]string, 0, len(outpoints))
	for _, o := range outpoints {
		txids = append(txids, o.Txid)
	}
	return txids
}

func sameTxids(got []string, want ...string) bool {
	if len(got) != len(want) {
		return false
	}
	set := map[string]struct{}{}
	for _, txid := range got {
		set[txid] = struct{}{}
	}
	for _, txid := range want {
		if _, ok := set[txid]; !ok {
			return false
		}
	}
	return true
}

// A null eth_getTransactionByHash is not proof the transaction is gone, so GetTransaction must report
// ErrTxNotFound without touching the mempool index (#1709).
func TestGetTransactionNullKeepsMempoolEntry(t *testing.T) {
	const hash = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	b, rpc := newWatermarkTestRPC(t, pendingTx(hash, watermarkSender, watermarkRecipient, "0x5"))
	delete(rpc.txs, hash)

	if _, err := b.GetTransaction(hash); err != bchain.ErrTxNotFound {
		t.Fatalf("GetTransaction() error = %v, want ErrTxNotFound", err)
	}
	if got := mempoolTxids(t, b, watermarkSender); len(got) != 1 || got[0] != hash {
		t.Fatalf("sender mempool txids = %v, want [%s] (a null answer must not evict)", got, hash)
	}
}

// A mined transaction retires the sender's pending entries at or below its nonce - including the
// alternative provider's copy - and leaves higher nonces and other senders alone.
func TestRemoveSupersededFromMempoolRetiresLowerNonces(t *testing.T) {
	const (
		stale    = "0x0000000000000000000000000000000000000000000000000000000000000005"
		replaced = "0x0000000000000000000000000000000000000000000000000000000000000006"
		next     = "0x0000000000000000000000000000000000000000000000000000000000000007"
		incoming = "0x0000000000000000000000000000000000000000000000000000000000000106"
	)
	b, _ := newWatermarkTestRPC(t,
		pendingTx(stale, watermarkSender, watermarkRecipient, "0x5"),
		pendingTx(replaced, watermarkSender, watermarkRecipient, "0x6"),
		pendingTx(next, watermarkSender, watermarkRecipient, "0x7"),
		pendingTx(incoming, watermarkRecipient, watermarkSender, "0x6"),
	)
	b.alternativeSendTxProvider = &AlternativeSendTxProvider{
		fetchMempoolTx: true,
		mempoolTxs:     map[string]storedTx{stale: {tx: pendingTx(stale, watermarkSender, watermarkRecipient, "0x5")}},
	}

	// the mined replacement of nonce 6 is a different hash - the RBF/cancel case
	b.removeSupersededFromMempool(pendingTx("0xfeed", watermarkSender, watermarkRecipient, "0x6"))

	if got := mempoolTxids(t, b, watermarkSender); !sameTxids(got, next, incoming) {
		t.Fatalf("sender mempool txids = %v, want next and incoming", got)
	}
	if got := mempoolTxids(t, b, watermarkRecipient); len(got) != 2 {
		t.Fatalf("recipient mempool txids = %v, want next and incoming", got)
	}
	if _, found := b.alternativeSendTxProvider.mempoolTxs[stale]; found {
		t.Fatal("alternative provider still caches the retired transaction")
	}
}
