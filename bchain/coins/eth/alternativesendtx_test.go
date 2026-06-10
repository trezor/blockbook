package eth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/trezor/blockbook/bchain"
)

const testAlternativeTxID = "0x1111111111111111111111111111111111111111111111111111111111111111"

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
