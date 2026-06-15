package eth

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/trezor/blockbook/bchain"
)

const testAlternativeTxID = "0x1111111111111111111111111111111111111111111111111111111111111111"
const testAlternativeSecondTxID = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// testAlternativeKnownTxResponse is an eth_getTransactionByHash result for a pending (not mined)
// transaction from the sender used in newTestAlternativeSendTxProvider.
const testAlternativeKnownTxResponse = `{"jsonrpc":"2.0","id":1,"result":{"hash":"` + testAlternativeTxID + `","from":"0x2222222222222222222222222222222222222222","nonce":"0x1","gas":"0x5208","value":"0x0","input":"0x","to":"0x3333333333333333333333333333333333333333"}}`

func newAlternativeTxProviderTestServer(t *testing.T, response string) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// the handler runs in a different goroutine, t.Fatalf must not be called from here
		if _, err := w.Write([]byte(response)); err != nil {
			t.Errorf("Write() error = %v", err)
		}
	}))
	t.Cleanup(server.Close)

	return server
}

func newTestAlternativeSendTxProvider(url string, removed *string) *AlternativeSendTxProvider {
	provider := &AlternativeSendTxProvider{
		urls:              []string{url},
		fetchMempoolTx:    true,
		mempoolTxsTimeout: time.Hour,
		rpcTimeout:        time.Second,
		mempoolTxs: map[string]storedTx{
			testAlternativeTxID: {
				tx: &bchain.RpcTransaction{
					Hash:         testAlternativeTxID,
					From:         "0x2222222222222222222222222222222222222222",
					AccountNonce: "0x1",
				},
				// older than the reconcile grace period so reconcileMempoolTxs checks it
				time: uint32(time.Now().Add(-2 * alternativeMempoolTxCheckPeriod).Unix()),
			},
		},
	}
	provider.removeTransactionFromMempool = func(txid string) {
		*removed = txid
		provider.RemoveTransaction(txid)
	}
	return provider
}

func TestAlternativeSendTxProviderReconcileRemovesDroppedTransaction(t *testing.T) {
	server := newAlternativeTxProviderTestServer(t, `{"jsonrpc":"2.0","id":1,"result":null}`)
	var removed string
	provider := newTestAlternativeSendTxProvider(server.URL, &removed)

	provider.reconcileMempoolTxs()

	if removed != testAlternativeTxID {
		t.Fatalf("removed txid = %q, want %q", removed, testAlternativeTxID)
	}
	if _, found := provider.mempoolTxs[testAlternativeTxID]; found {
		t.Fatal("dropped transaction remained in alternative mempool cache")
	}
}

func TestAlternativeSendTxProviderReconcileSkipsFreshTransaction(t *testing.T) {
	server := newAlternativeTxProviderTestServer(t, `{"jsonrpc":"2.0","id":1,"result":null}`)
	var removed string
	provider := newTestAlternativeSendTxProvider(server.URL, &removed)
	tx := provider.mempoolTxs[testAlternativeTxID]
	tx.time = uint32(time.Now().Unix())
	provider.mempoolTxs[testAlternativeTxID] = tx

	provider.reconcileMempoolTxs()

	if removed != "" {
		t.Fatalf("removed txid = %q, want none", removed)
	}
	if _, found := provider.mempoolTxs[testAlternativeTxID]; !found {
		t.Fatal("freshly submitted transaction was removed from alternative mempool cache")
	}
}

func TestAlternativeSendTxProviderReconcileKeepsKnownTransaction(t *testing.T) {
	server := newAlternativeTxProviderTestServer(t, `{"jsonrpc":"2.0","id":1,"result":{"hash":"`+testAlternativeTxID+`","from":"0x2222222222222222222222222222222222222222","nonce":"0x1","gas":"0x5208","value":"0x0","input":"0x","to":"0x3333333333333333333333333333333333333333"}}`)
	var removed string
	provider := newTestAlternativeSendTxProvider(server.URL, &removed)

	provider.reconcileMempoolTxs()

	if removed != "" {
		t.Fatalf("removed txid = %q, want none", removed)
	}
	if _, found := provider.mempoolTxs[testAlternativeTxID]; !found {
		t.Fatal("known transaction was removed from alternative mempool cache")
	}
}

func TestAlternativeSendTxProviderReconcileKeepsTransactionKnownByAnyProvider(t *testing.T) {
	droppedServer := newAlternativeTxProviderTestServer(t, `{"jsonrpc":"2.0","id":1,"result":null}`)
	knownServer := newAlternativeTxProviderTestServer(t, `{"jsonrpc":"2.0","id":1,"result":{"hash":"`+testAlternativeTxID+`","from":"0x2222222222222222222222222222222222222222","nonce":"0x1","gas":"0x5208","value":"0x0","input":"0x","to":"0x3333333333333333333333333333333333333333"}}`)
	var removed string
	provider := newTestAlternativeSendTxProvider(droppedServer.URL, &removed)
	provider.urls = append(provider.urls, knownServer.URL)

	provider.reconcileMempoolTxs()

	if removed != "" {
		t.Fatalf("removed txid = %q, want none", removed)
	}
	if _, found := provider.mempoolTxs[testAlternativeTxID]; !found {
		t.Fatal("transaction known by a provider was removed from alternative mempool cache")
	}
}

func TestAlternativeSendTxProviderReconcileRemovesMinedTransaction(t *testing.T) {
	server := newAlternativeTxProviderTestServer(t, `{"jsonrpc":"2.0","id":1,"result":{"hash":"`+testAlternativeTxID+`","from":"0x2222222222222222222222222222222222222222","nonce":"0x1","gas":"0x5208","value":"0x0","input":"0x","to":"0x3333333333333333333333333333333333333333","blockNumber":"0x1"}}`)
	var removed string
	provider := newTestAlternativeSendTxProvider(server.URL, &removed)

	provider.reconcileMempoolTxs()

	if removed != testAlternativeTxID {
		t.Fatalf("removed txid = %q, want %q", removed, testAlternativeTxID)
	}
	if _, found := provider.mempoolTxs[testAlternativeTxID]; found {
		t.Fatal("mined transaction remained in alternative mempool cache")
	}
}

func TestAlternativeSendTxProviderReconcileKeepsTransactionOnProviderError(t *testing.T) {
	server := newAlternativeTxProviderTestServer(t, `{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"temporary failure"}}`)
	var removed string
	provider := newTestAlternativeSendTxProvider(server.URL, &removed)

	provider.reconcileMempoolTxs()

	if removed != "" {
		t.Fatalf("removed txid = %q, want none", removed)
	}
	if _, found := provider.mempoolTxs[testAlternativeTxID]; !found {
		t.Fatal("transaction was removed after provider error")
	}
}

func TestAlternativeSendTxProviderHandleMempoolTransactionFetchesFromAnyProvider(t *testing.T) {
	droppedServer := newAlternativeTxProviderTestServer(t, `{"jsonrpc":"2.0","id":1,"result":null}`)
	knownServer := newAlternativeTxProviderTestServer(t, `{"jsonrpc":"2.0","id":1,"result":{"hash":"`+testAlternativeTxID+`","from":"0x2222222222222222222222222222222222222222","nonce":"0x1","gas":"0x5208","value":"0x0","input":"0x","to":"0x3333333333333333333333333333333333333333"}}`)
	var removed string
	provider := newTestAlternativeSendTxProvider(droppedServer.URL, &removed)
	provider.mempoolTxs = make(map[string]storedTx)
	provider.urls = append(provider.urls, knownServer.URL)

	if _, err := provider.handleMempoolTransaction(testAlternativeTxID); err != nil {
		t.Fatalf("handleMempoolTransaction() error = %v", err)
	}
	if _, found := provider.mempoolTxs[testAlternativeTxID]; !found {
		t.Fatal("known transaction was not stored in alternative mempool cache")
	}
}

func TestAlternativeSendTxProviderHandleMempoolTransactionSkipsEmptyTransaction(t *testing.T) {
	server := newAlternativeTxProviderTestServer(t, `{"jsonrpc":"2.0","id":1,"result":null}`)
	var removed string
	provider := newTestAlternativeSendTxProvider(server.URL, &removed)
	provider.mempoolTxs = make(map[string]storedTx)

	if _, err := provider.handleMempoolTransaction(testAlternativeTxID); err == nil {
		t.Fatal("handleMempoolTransaction() error = nil, want ErrTxNotFound")
	}
	if _, found := provider.mempoolTxs[testAlternativeTxID]; found {
		t.Fatal("empty transaction was stored in alternative mempool cache")
	}
}

func TestAlternativeSendTxProviderHandleMempoolTransactionSkipsTransactionWithoutHash(t *testing.T) {
	server := newAlternativeTxProviderTestServer(t, `{"jsonrpc":"2.0","id":1,"result":{"from":"0x2222222222222222222222222222222222222222","nonce":"0x1","gas":"0x5208","value":"0x0","input":"0x","to":"0x3333333333333333333333333333333333333333"}}`)
	var removed string
	provider := newTestAlternativeSendTxProvider(server.URL, &removed)
	provider.mempoolTxs = make(map[string]storedTx)

	if _, err := provider.handleMempoolTransaction(testAlternativeTxID); err == nil {
		t.Fatal("handleMempoolTransaction() error = nil, want ErrTxNotFound")
	}
	if _, found := provider.mempoolTxs[testAlternativeTxID]; found {
		t.Fatal("transaction without hash was stored in alternative mempool cache")
	}
}

// methodAwareServer is a JSON-RPC test server that returns a different response per RPC method and
// records how many times each method was called.
type methodAwareServer struct {
	*httptest.Server
	mu    sync.Mutex
	calls map[string]int
}

func (s *methodAwareServer) callCount(method string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls[method]
}

func newMethodAwareTxProviderTestServer(t *testing.T, responses map[string]string) *methodAwareServer {
	t.Helper()

	s := &methodAwareServer{calls: make(map[string]int)}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Method string `json:"method"`
		}
		_ = json.Unmarshal(body, &req)

		s.mu.Lock()
		s.calls[req.Method]++
		s.mu.Unlock()

		resp, ok := responses[req.Method]
		if !ok {
			resp = `{"jsonrpc":"2.0","id":1,"result":null}`
		}
		w.Header().Set("Content-Type", "application/json")
		// the handler runs in a different goroutine, t.Fatalf must not be called from here
		if _, err := w.Write([]byte(resp)); err != nil {
			t.Errorf("Write() error = %v", err)
		}
	}))
	t.Cleanup(s.Server.Close)

	return s
}

func nonceCountResponse(hexNonce string) string {
	return `{"jsonrpc":"2.0","id":1,"result":"` + hexNonce + `"}`
}

func TestAlternativeSendTxProviderReconcileRemovesNonceSupersededTransaction(t *testing.T) {
	// provider still reports the tx as pending, but its nonce (0x1) is below the confirmed account
	// nonce (0x2): a different transaction already consumed it, so it can never be mined.
	server := newMethodAwareTxProviderTestServer(t, map[string]string{
		"eth_getTransactionByHash": testAlternativeKnownTxResponse,
		"eth_getTransactionCount":  nonceCountResponse("0x2"),
	})
	var removed string
	provider := newTestAlternativeSendTxProvider(server.URL, &removed)

	provider.reconcileMempoolTxs()

	if removed != testAlternativeTxID {
		t.Fatalf("removed txid = %q, want %q", removed, testAlternativeTxID)
	}
	if _, found := provider.mempoolTxs[testAlternativeTxID]; found {
		t.Fatal("nonce-superseded transaction remained in alternative mempool cache")
	}
}

func TestAlternativeSendTxProviderReconcileKeepsTransactionWithCurrentNonce(t *testing.T) {
	// cached tx nonce (0x1) equals the confirmed account nonce (0x1): it is the next mineable tx.
	server := newMethodAwareTxProviderTestServer(t, map[string]string{
		"eth_getTransactionByHash": testAlternativeKnownTxResponse,
		"eth_getTransactionCount":  nonceCountResponse("0x1"),
	})
	var removed string
	provider := newTestAlternativeSendTxProvider(server.URL, &removed)

	provider.reconcileMempoolTxs()

	if removed != "" {
		t.Fatalf("removed txid = %q, want none", removed)
	}
	if _, found := provider.mempoolTxs[testAlternativeTxID]; !found {
		t.Fatal("transaction with the current account nonce was removed from alternative mempool cache")
	}
}

func TestAlternativeSendTxProviderReconcileKeepsTransactionWithFutureNonce(t *testing.T) {
	// cached tx nonce (0x1) is ahead of the confirmed account nonce (0x0): a gap that may still be
	// filled. Gap eviction is intentionally not performed; the timeout remains the only safety net.
	server := newMethodAwareTxProviderTestServer(t, map[string]string{
		"eth_getTransactionByHash": testAlternativeKnownTxResponse,
		"eth_getTransactionCount":  nonceCountResponse("0x0"),
	})
	var removed string
	provider := newTestAlternativeSendTxProvider(server.URL, &removed)

	provider.reconcileMempoolTxs()

	if removed != "" {
		t.Fatalf("removed txid = %q, want none", removed)
	}
	if _, found := provider.mempoolTxs[testAlternativeTxID]; !found {
		t.Fatal("transaction ahead of the confirmed nonce (gap) was incorrectly evicted")
	}
}

func TestAlternativeSendTxProviderReconcileKeepsTransactionOnNonceLookupFailure(t *testing.T) {
	// eth_getTransactionCount fails: without a confirmed nonce we cannot prove the tx is superseded,
	// so it must be kept and left to the timeout path.
	server := newMethodAwareTxProviderTestServer(t, map[string]string{
		"eth_getTransactionByHash": testAlternativeKnownTxResponse,
		"eth_getTransactionCount":  `{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"temporary failure"}}`,
	})
	var removed string
	provider := newTestAlternativeSendTxProvider(server.URL, &removed)

	provider.reconcileMempoolTxs()

	if removed != "" {
		t.Fatalf("removed txid = %q, want none", removed)
	}
	if _, found := provider.mempoolTxs[testAlternativeTxID]; !found {
		t.Fatal("transaction was evicted despite a failed confirmed-nonce lookup")
	}
}

func TestAlternativeSendTxProviderReconcileUsesLowestConfirmedNonce(t *testing.T) {
	// one provider claims the nonce is consumed (0x2), another that it is still current (0x1). The
	// conservative minimum (0x1) must win so a still-mineable tx is not evicted by a lagging node.
	highServer := newMethodAwareTxProviderTestServer(t, map[string]string{
		"eth_getTransactionByHash": testAlternativeKnownTxResponse,
		"eth_getTransactionCount":  nonceCountResponse("0x2"),
	})
	lowServer := newMethodAwareTxProviderTestServer(t, map[string]string{
		"eth_getTransactionByHash": testAlternativeKnownTxResponse,
		"eth_getTransactionCount":  nonceCountResponse("0x1"),
	})
	var removed string
	provider := newTestAlternativeSendTxProvider(highServer.URL, &removed)
	provider.urls = append(provider.urls, lowServer.URL)

	provider.reconcileMempoolTxs()

	if removed != "" {
		t.Fatalf("removed txid = %q, want none", removed)
	}
	if _, found := provider.mempoolTxs[testAlternativeTxID]; !found {
		t.Fatal("transaction was evicted using a non-conservative confirmed nonce")
	}
}

func TestAlternativeSendTxProviderReconcileKeepsTransactionWithUnparsableNonce(t *testing.T) {
	// a cached tx whose own nonce cannot be parsed must never be treated as superseded
	server := newMethodAwareTxProviderTestServer(t, map[string]string{
		"eth_getTransactionByHash": testAlternativeKnownTxResponse,
		"eth_getTransactionCount":  nonceCountResponse("0x2"),
	})
	var removed string
	provider := newTestAlternativeSendTxProvider(server.URL, &removed)
	tx := provider.mempoolTxs[testAlternativeTxID]
	tx.tx.AccountNonce = "not-a-nonce"
	provider.mempoolTxs[testAlternativeTxID] = tx

	provider.reconcileMempoolTxs()

	if removed != "" {
		t.Fatalf("removed txid = %q, want none", removed)
	}
	if _, found := provider.mempoolTxs[testAlternativeTxID]; !found {
		t.Fatal("transaction with an unparsable nonce was incorrectly evicted")
	}
}

func TestAlternativeSendTxProviderReconcileKeepsTransactionWhenConfirmedNonceUnparsable(t *testing.T) {
	// the confirmed-nonce response is malformed: without a usable count we cannot prove supersession
	server := newMethodAwareTxProviderTestServer(t, map[string]string{
		"eth_getTransactionByHash": testAlternativeKnownTxResponse,
		"eth_getTransactionCount":  nonceCountResponse("0xZZ"),
	})
	var removed string
	provider := newTestAlternativeSendTxProvider(server.URL, &removed)

	provider.reconcileMempoolTxs()

	if removed != "" {
		t.Fatalf("removed txid = %q, want none", removed)
	}
	if _, found := provider.mempoolTxs[testAlternativeTxID]; !found {
		t.Fatal("transaction was evicted despite an unparsable confirmed-nonce response")
	}
}

func TestAlternativeSendTxProviderReconcileFailedNonceLookupIsPerSender(t *testing.T) {
	// sender 0x2222 is checked here; a failed lookup for one sender must not suppress eviction for
	// another. Single-provider servers cannot distinguish senders, so this test uses one sender and
	// asserts the failed-memo does not leak into the resolved map.
	server := newMethodAwareTxProviderTestServer(t, map[string]string{
		"eth_getTransactionByHash": testAlternativeKnownTxResponse,
		"eth_getTransactionCount":  nonceCountResponse("0x2"),
	})
	var removed string
	provider := newTestAlternativeSendTxProvider(server.URL, &removed)

	resolved := make(map[string]uint64)
	failed := map[string]bool{"0x9999999999999999999999999999999999999999": true}

	tx := provider.mempoolTxs[testAlternativeTxID]
	if !provider.transactionSupersededByNonce(tx.tx, resolved, failed) {
		t.Fatal("a failed lookup for a different sender suppressed supersession of sender 0x2222")
	}
}

func TestAlternativeSendTxProviderReconcileMemoizesConfirmedNoncePerSender(t *testing.T) {
	server := newMethodAwareTxProviderTestServer(t, map[string]string{
		"eth_getTransactionByHash": testAlternativeKnownTxResponse,
		"eth_getTransactionCount":  nonceCountResponse("0x2"),
	})
	var removed string
	provider := newTestAlternativeSendTxProvider(server.URL, &removed)
	// a second tx from the same sender must reuse the single confirmed-nonce lookup
	provider.mempoolTxs[testAlternativeSecondTxID] = storedTx{
		tx: &bchain.RpcTransaction{
			Hash:         testAlternativeSecondTxID,
			From:         "0x2222222222222222222222222222222222222222",
			AccountNonce: "0x3",
		},
		time: uint32(time.Now().Add(-2 * alternativeMempoolTxCheckPeriod).Unix()),
	}

	provider.reconcileMempoolTxs()

	if got := server.callCount("eth_getTransactionCount"); got != 1 {
		t.Fatalf("eth_getTransactionCount calls = %d, want 1 (memoized per sender)", got)
	}
	// nonce 0x1 < 0x2 is superseded and evicted; nonce 0x3 > 0x2 stays
	if _, found := provider.mempoolTxs[testAlternativeTxID]; found {
		t.Fatal("nonce-superseded transaction remained in alternative mempool cache")
	}
	if _, found := provider.mempoolTxs[testAlternativeSecondTxID]; !found {
		t.Fatal("transaction ahead of the confirmed nonce was incorrectly evicted")
	}
}
