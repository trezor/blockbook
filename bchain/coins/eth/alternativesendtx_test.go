package eth

import (
	"bytes"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
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

// assertReconcileOutcome checks whether the single cached test transaction was evicted (and reported
// through the removeTransactionFromMempool callback) or kept after a reconcile cycle.
func assertReconcileOutcome(t *testing.T, provider *AlternativeSendTxProvider, removed string, wantRemoved bool) {
	t.Helper()
	_, found := provider.mempoolTxs[testAlternativeTxID]
	if wantRemoved {
		if removed != testAlternativeTxID {
			t.Fatalf("removed txid = %q, want %q", removed, testAlternativeTxID)
		}
		if found {
			t.Fatal("transaction remained in alternative mempool cache, want removed")
		}
		return
	}
	if removed != "" {
		t.Fatalf("removed txid = %q, want none", removed)
	}
	if !found {
		t.Fatal("transaction was removed from alternative mempool cache, want kept")
	}
}

func TestAlternativeSendTxProviderReconcileLivenessOutcomes(t *testing.T) {
	const minedTxResponse = `{"jsonrpc":"2.0","id":1,"result":{"hash":"` + testAlternativeTxID + `","from":"0x2222222222222222222222222222222222222222","nonce":"0x1","gas":"0x5208","value":"0x0","input":"0x","to":"0x3333333333333333333333333333333333333333","blockNumber":"0x1"}}`
	tests := []struct {
		name        string
		response    string
		wantRemoved bool
	}{
		// A provider that returns empty is NOT authoritative proof the tx is gone (Blink-style
		// private/MEV relays stop surfacing a still-pending tx via eth_getTransactionByHash). The tx
		// is kept until the cache timeout - here mempoolTxsTimeout is time.Hour, so it stays.
		{"empty provider result is kept until timeout", `{"jsonrpc":"2.0","id":1,"result":null}`, false},
		{"mined tx is removed", minedTxResponse, true},
		{"known pending tx is kept", testAlternativeKnownTxResponse, false},
		{"tx is kept on provider error", `{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"temporary failure"}}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newAlternativeTxProviderTestServer(t, tt.response)
			var removed string
			provider := newTestAlternativeSendTxProvider(server.URL, &removed)

			provider.reconcileMempoolTxs()

			assertReconcileOutcome(t, provider, removed, tt.wantRemoved)
		})
	}
}

func TestAlternativeSendTxProviderReconcileTimeoutEviction(t *testing.T) {
	// A tx older than mempoolTxsTimeout must be evicted by the reconcile timeout "safety net", and -
	// like every other eviction path - the eviction must go through removeMempoolTx (the
	// removeTransactionFromMempool callback) so the tx is dropped from BOTH the main mempool and the
	// alternative cache, not only the cache. assertReconcileOutcome checks the callback fired.
	tests := []struct {
		name      string
		serverURL func(t *testing.T) string
	}{
		{
			name: "provider error and timed out is removed",
			serverURL: func(t *testing.T) string {
				return newAlternativeTxProviderTestServer(t, `{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"temporary failure"}}`).URL
			},
		},
		{
			// an empty provider result is kept while fresh (see ReconcileLivenessOutcomes) but the
			// timeout safety net still evicts it once mempoolTxsTimeout has elapsed
			name: "empty provider result and timed out is removed",
			serverURL: func(t *testing.T) string {
				return newAlternativeTxProviderTestServer(t, `{"jsonrpc":"2.0","id":1,"result":null}`).URL
			},
		},
		{
			name: "still pending, nonce not superseded and timed out is removed",
			serverURL: func(t *testing.T) string {
				return newMethodAwareTxProviderTestServer(t, map[string]string{
					"eth_getTransactionByHash": testAlternativeKnownTxResponse,
					// confirmed nonce equals the tx nonce (0x1): not superseded, so only the timeout evicts it
					"eth_getTransactionCount": nonceCountResponse("0x1"),
				}).URL
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var removed string
			provider := newTestAlternativeSendTxProvider(tt.serverURL(t), &removed)
			// the cached tx is timestamped ~2 check periods ago; a tiny timeout makes it timed out
			provider.mempoolTxsTimeout = time.Nanosecond

			provider.reconcileMempoolTxs()

			assertReconcileOutcome(t, provider, removed, true)
		})
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

func TestAlternativeSendTxProviderReconcileKeepsTransactionKnownByAnyProvider(t *testing.T) {
	droppedServer := newAlternativeTxProviderTestServer(t, `{"jsonrpc":"2.0","id":1,"result":null}`)
	knownServer := newAlternativeTxProviderTestServer(t, testAlternativeKnownTxResponse)
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

func TestAlternativeSendTxProviderHandleMempoolTransactionFetchesFromAnyProvider(t *testing.T) {
	droppedServer := newAlternativeTxProviderTestServer(t, `{"jsonrpc":"2.0","id":1,"result":null}`)
	knownServer := newAlternativeTxProviderTestServer(t, testAlternativeKnownTxResponse)
	var removed string
	provider := newTestAlternativeSendTxProvider(droppedServer.URL, &removed)
	provider.mempoolTxs = make(map[string]storedTx)
	provider.urls = append(provider.urls, knownServer.URL)

	if _, err := provider.handleMempoolTransaction(testAlternativeTxID, 0); err != nil {
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

	if _, err := provider.handleMempoolTransaction(testAlternativeTxID, 0); err == nil {
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

	if _, err := provider.handleMempoolTransaction(testAlternativeTxID, 0); err == nil {
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

func TestAlternativeSendTxProviderReconcileNonceOutcomes(t *testing.T) {
	// the cached tx has nonce 0x1 and the provider still reports it as pending; only the confirmed
	// account nonce returned by eth_getTransactionCount("latest") decides the outcome.
	tests := []struct {
		name            string
		txCountResponse string
		wantRemoved     bool
	}{
		{"nonce below confirmed nonce is superseded and removed", nonceCountResponse("0x2"), true},
		{"nonce equal to confirmed nonce is kept (next mineable)", nonceCountResponse("0x1"), false},
		{"nonce above confirmed nonce is kept (gap, not evicted)", nonceCountResponse("0x0"), false},
		{"unparsable confirmed nonce keeps the tx", nonceCountResponse("0xZZ"), false},
		{"failed confirmed-nonce lookup keeps the tx", `{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"temporary failure"}}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newMethodAwareTxProviderTestServer(t, map[string]string{
				"eth_getTransactionByHash": testAlternativeKnownTxResponse,
				"eth_getTransactionCount":  tt.txCountResponse,
			})
			var removed string
			provider := newTestAlternativeSendTxProvider(server.URL, &removed)

			provider.reconcileMempoolTxs()

			assertReconcileOutcome(t, provider, removed, tt.wantRemoved)
		})
	}
}

func TestAlternativeSendTxProviderReconcileEvictsSupersededMissingTx(t *testing.T) {
	// The provider no longer surfaces the tx via eth_getTransactionByHash (returns empty), so it is
	// not evicted on the "missing" path alone. But the confirmed account nonce (0x2) is strictly
	// above the cached tx nonce (0x1): the nonce is spent on-chain, so the tx can never be mined and
	// is evicted deterministically even though it is well within the (1h) cache timeout.
	server := newMethodAwareTxProviderTestServer(t, map[string]string{
		"eth_getTransactionByHash": `{"jsonrpc":"2.0","id":1,"result":null}`,
		"eth_getTransactionCount":  nonceCountResponse("0x2"),
	})
	var removed string
	provider := newTestAlternativeSendTxProvider(server.URL, &removed)

	provider.reconcileMempoolTxs()

	assertReconcileOutcome(t, provider, removed, true)
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

// newReconcileTestMetrics builds a common.Metrics holding only the collectors reconcileMempoolTxs
// touches, left unregistered so each test owns fresh collectors and testutil can read them directly.
func newReconcileTestMetrics() *common.Metrics {
	return &common.Metrics{
		EthAlternativeMempoolEvents: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "test_alt_mempool_events_total"}, []string{"action"}),
		EthAlternativeMempoolTxResidence: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{Name: "test_alt_mempool_tx_residence_seconds", Buckets: []float64{30, 60, 120, 300, 600}}, []string{"action"}),
		EthAlternativeMempoolCacheSize: prometheus.NewGauge(
			prometheus.GaugeOpts{Name: "test_alt_mempool_cache_size"}),
	}
}

// The readers below register a collector in a throwaway registry and gather it, so a test can read
// metric values without pulling in the prometheus/testutil dependency (and its transitive modules).

// gaugeValue reads the current value of a single gauge.
func gaugeValue(t *testing.T, g prometheus.Gauge) float64 {
	t.Helper()
	reg := prometheus.NewRegistry()
	if err := reg.Register(g); err != nil {
		t.Fatalf("register gauge: %v", err)
	}
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather gauge: %v", err)
	}
	for _, mf := range families {
		for _, m := range mf.GetMetric() {
			return m.GetGauge().GetValue()
		}
	}
	return 0
}

// counterValue reads the value of the counter series carrying action=action.
func counterValue(t *testing.T, cv *prometheus.CounterVec, action string) float64 {
	t.Helper()
	reg := prometheus.NewRegistry()
	if err := reg.Register(cv); err != nil {
		t.Fatalf("register counter: %v", err)
	}
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather counter: %v", err)
	}
	for _, mf := range families {
		for _, m := range mf.GetMetric() {
			for _, lp := range m.GetLabel() {
				if lp.GetName() == "action" && lp.GetValue() == action {
					return m.GetCounter().GetValue()
				}
			}
		}
	}
	return 0
}

// residenceSampleCount reports how many residence observations were recorded under action=action.
func residenceSampleCount(t *testing.T, h *prometheus.HistogramVec, action string) uint64 {
	t.Helper()
	reg := prometheus.NewRegistry()
	if err := reg.Register(h); err != nil {
		t.Fatalf("register residence histogram: %v", err)
	}
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather residence histogram: %v", err)
	}
	for _, mf := range families {
		for _, m := range mf.GetMetric() {
			for _, lp := range m.GetLabel() {
				if lp.GetName() == "action" && lp.GetValue() == action {
					return m.GetHistogram().GetSampleCount()
				}
			}
		}
	}
	return 0
}

// TestAlternativeSendTxProviderReconcileObservesMetrics asserts the reconcile flow feeds the two
// metrics added to make it transparent: the per-action tx-lifetime histogram (observed only on
// eviction, under the same action label as the decision counter) and the cache-depth gauge.
func TestAlternativeSendTxProviderReconcileObservesMetrics(t *testing.T) {
	const minedTxResponse = `{"jsonrpc":"2.0","id":1,"result":{"hash":"` + testAlternativeTxID + `","from":"0x2222222222222222222222222222222222222222","nonce":"0x1","gas":"0x5208","value":"0x0","input":"0x","to":"0x3333333333333333333333333333333333333333","blockNumber":"0x1"}}`

	t.Run("eviction records residence and zeroes the cache-depth gauge", func(t *testing.T) {
		server := newAlternativeTxProviderTestServer(t, minedTxResponse)
		var removed string
		provider := newTestAlternativeSendTxProvider(server.URL, &removed)
		provider.metrics = newReconcileTestMetrics()

		provider.reconcileMempoolTxs()

		if got := counterValue(t, provider.metrics.EthAlternativeMempoolEvents, "mined"); got != 1 {
			t.Errorf("mined reconciliation events = %v, want 1", got)
		}
		// the lifetime histogram records exactly one sample, under the same action label as the counter
		if got := residenceSampleCount(t, provider.metrics.EthAlternativeMempoolTxResidence, "mined"); got != 1 {
			t.Errorf("mined residence sample count = %d, want 1", got)
		}
		// the only cached tx was evicted, so the depth gauge settles at 0
		if got := gaugeValue(t, provider.metrics.EthAlternativeMempoolCacheSize); got != 0 {
			t.Errorf("cache depth gauge = %v, want 0 after eviction", got)
		}
	})

	t.Run("a kept tx records no residence and keeps the gauge at one", func(t *testing.T) {
		server := newAlternativeTxProviderTestServer(t, testAlternativeKnownTxResponse)
		var removed string
		provider := newTestAlternativeSendTxProvider(server.URL, &removed)
		provider.metrics = newReconcileTestMetrics()

		provider.reconcileMempoolTxs()

		if got := counterValue(t, provider.metrics.EthAlternativeMempoolEvents, "kept"); got != 1 {
			t.Errorf("kept reconciliation events = %v, want 1", got)
		}
		// nothing was evicted, so no lifetime sample must be recorded for any terminal action
		for _, action := range []string{"mined", "nonce_superseded", "provider_missing", "timeout"} {
			if got := residenceSampleCount(t, provider.metrics.EthAlternativeMempoolTxResidence, action); got != 0 {
				t.Errorf("residence sample count for %q = %d, want 0 when nothing is evicted", action, got)
			}
		}
		if got := gaugeValue(t, provider.metrics.EthAlternativeMempoolCacheSize); got != 1 {
			t.Errorf("cache depth gauge = %v, want 1 with one tx retained", got)
		}
	})
}

func TestAlternativeSendTxProviderGetTransactionTimeoutObservesMetrics(t *testing.T) {
	// a cached entry past mempoolTxsTimeout, read before the reconcile loop reaches it, is evicted on
	// the read path - and that eviction must be counted and have its residence observed just like the
	// reconcile-loop timeout, otherwise the timeout series is undercounted.
	provider := &AlternativeSendTxProvider{
		fetchMempoolTx:    true,
		mempoolTxsTimeout: time.Minute,
		mempoolTxs: map[string]storedTx{
			testAlternativeTxID: {
				tx:   &bchain.RpcTransaction{Hash: testAlternativeTxID},
				time: uint32(time.Now().Add(-2 * time.Minute).Unix()),
			},
		},
		metrics: newReconcileTestMetrics(),
	}

	if tx, found := provider.GetTransaction(testAlternativeTxID); found || tx != nil {
		t.Fatalf("timed-out tx: got (tx=%v found=%v), want (nil false)", tx, found)
	}
	if _, stillCached := provider.mempoolTxs[testAlternativeTxID]; stillCached {
		t.Fatal("timed-out tx was not evicted from the cache")
	}
	if got := counterValue(t, provider.metrics.EthAlternativeMempoolEvents, "timeout"); got != 1 {
		t.Errorf("timeout reconciliation events = %v, want 1", got)
	}
	if got := residenceSampleCount(t, provider.metrics.EthAlternativeMempoolTxResidence, "timeout"); got != 1 {
		t.Errorf("timeout residence sample count = %d, want 1", got)
	}
}

func TestAlternativeSendTxProviderRBFReplacementObservesMetrics(t *testing.T) {
	// handleMempoolTransaction replaces a cached entry sharing the incoming tx's sender+nonce. The
	// replaced entry leaves the cache by fee-replacement, so that exit must be counted and its residence
	// observed, not silently dropped.
	server := newAlternativeTxProviderTestServer(t, testAlternativeKnownTxResponse)
	provider := &AlternativeSendTxProvider{
		urls:              []string{server.URL},
		fetchMempoolTx:    true,
		onlyAlternative:   true,
		rpcTimeout:        time.Second,
		mempoolTxsTimeout: time.Hour,
		mempoolTxs: map[string]storedTx{
			// the older tx that the incoming testAlternativeTxID (same sender 0x2222, nonce 0x1) replaces
			testAlternativeSecondTxID: {
				tx: &bchain.RpcTransaction{
					Hash:         testAlternativeSecondTxID,
					From:         "0x2222222222222222222222222222222222222222",
					AccountNonce: "0x1",
				},
				time: uint32(time.Now().Add(-3 * time.Minute).Unix()),
			},
		},
		metrics: newReconcileTestMetrics(),
	}
	var removed string
	provider.removeTransactionFromMempool = func(txid string) {
		removed = txid
		provider.RemoveTransaction(txid)
	}

	if _, err := provider.handleMempoolTransaction(testAlternativeTxID, 0); err != nil {
		t.Fatalf("handleMempoolTransaction: %v", err)
	}

	if removed != testAlternativeSecondTxID {
		t.Fatalf("replaced txid = %q, want %q", removed, testAlternativeSecondTxID)
	}
	if got := counterValue(t, provider.metrics.EthAlternativeMempoolEvents, "rbf_replaced"); got != 1 {
		t.Errorf("rbf_replaced events = %v, want 1", got)
	}
	if got := residenceSampleCount(t, provider.metrics.EthAlternativeMempoolTxResidence, "rbf_replaced"); got != 1 {
		t.Errorf("rbf_replaced residence sample count = %d, want 1", got)
	}
}

func TestAlternativeSendTxProviderShutdownStopsWatchLoop(t *testing.T) {
	provider := &AlternativeSendTxProvider{
		fetchMempoolTx: true,
		mempoolTxs:     make(map[string]storedTx),
		stop:           make(chan struct{}),
	}

	done := make(chan struct{})
	go func() {
		provider.watchMempoolTxs()
		close(done)
	}()

	provider.shutdown()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("watchMempoolTxs did not return after shutdown")
	}

	// shutdown must be idempotent and must not panic when called again
	provider.shutdown()
}

func TestAlternativeSendTxProviderShutdownNilSafe(t *testing.T) {
	// no alternative provider configured leaves a nil *AlternativeSendTxProvider; Shutdown must not panic
	var provider *AlternativeSendTxProvider
	provider.shutdown()
}

// jsonRPCReq is the subset of a JSON-RPC request the nonce test server inspects.
type jsonRPCReq struct {
	ID     json.RawMessage `json:"id"`
	Params []interface{}   `json:"params"`
}

// nonceRPCServer is a JSON-RPC test server for eth_getTransactionCount. It serves a per-block-tag
// hex result (or a per-tag JSON-RPC error) and supports both single and batched requests, so it can
// drive AlternativeSendTxProvider.getNonces over a real rpc.Client round-trip (the batched path uses
// BatchCallContext, which a plain method-keyed mock cannot exercise). It records how many times each
// block tag was queried so a test can assert the "latest" call is skipped when not requested.
type nonceRPCServer struct {
	*httptest.Server
	mu      sync.Mutex
	results map[string]string // tag -> hex result
	errs    map[string]bool   // tag -> return a JSON-RPC error instead of a result
	calls   map[string]int    // tag -> query count
}

func (s *nonceRPCServer) callCount(tag string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls[tag]
}

func (s *nonceRPCServer) respond(req jsonRPCReq) string {
	tag := ""
	if len(req.Params) >= 2 {
		tag, _ = req.Params[1].(string)
	}
	s.mu.Lock()
	s.calls[tag]++
	s.mu.Unlock()
	id := string(req.ID)
	if id == "" {
		id = "null"
	}
	if s.errs[tag] {
		return `{"jsonrpc":"2.0","id":` + id + `,"error":{"code":-32000,"message":"temporary failure"}}`
	}
	return `{"jsonrpc":"2.0","id":` + id + `,"result":"` + s.results[tag] + `"}`
}

func newNonceRPCServer(t *testing.T, results map[string]string, errs map[string]bool) *nonceRPCServer {
	t.Helper()

	s := &nonceRPCServer{results: results, errs: errs, calls: make(map[string]int)}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		// the handler runs in a different goroutine, t.Fatalf must not be called from here
		var out string
		if trimmed := bytes.TrimSpace(body); len(trimmed) > 0 && trimmed[0] == '[' {
			var reqs []jsonRPCReq
			if err := json.Unmarshal(body, &reqs); err != nil {
				t.Errorf("Unmarshal batch request: %v", err)
				return
			}
			parts := make([]string, 0, len(reqs))
			for _, req := range reqs {
				parts = append(parts, s.respond(req))
			}
			out = "[" + strings.Join(parts, ",") + "]"
		} else {
			var req jsonRPCReq
			if err := json.Unmarshal(body, &req); err != nil {
				t.Errorf("Unmarshal request: %v", err)
				return
			}
			out = s.respond(req)
		}
		if _, err := w.Write([]byte(out)); err != nil {
			t.Errorf("Write() error = %v", err)
		}
	}))
	t.Cleanup(s.Server.Close)

	return s
}

// TestAlternativeSendTxProviderGetNonces covers the alternative-provider nonce path that backs the
// confirmedNonce field for private/Blink relay coins. It mirrors the primary-RPC getNoncesRPC tests
// in nonce_test.go: pending-only when gated off, batched pending+confirmed when gated on, best-effort
// confirmed failure, fatal pending failure, and fatal batch transport failure.
func TestAlternativeSendTxProviderGetNonces(t *testing.T) {
	addr := ethcommon.HexToAddress("0x2222222222222222222222222222222222222222")

	t.Run("gated off fetches pending only", func(t *testing.T) {
		server := newNonceRPCServer(t, map[string]string{"pending": "0x4", "latest": "0x2"}, nil)
		provider := &AlternativeSendTxProvider{urls: []string{server.URL}, rpcTimeout: time.Second}

		pending, confirmed, confirmedOK, err := provider.getNonces(addr, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pending != 4 || confirmed != 0 || confirmedOK {
			t.Errorf("got (pending=%d confirmed=%d ok=%v), want (4 0 false)", pending, confirmed, confirmedOK)
		}
		if got := server.callCount("latest"); got != 0 {
			t.Errorf("latest queried %d times, want 0 when confirmed nonce not requested", got)
		}
		if got := server.callCount("pending"); got != 1 {
			t.Errorf("pending queried %d times, want 1", got)
		}
	})

	t.Run("gated on batched success", func(t *testing.T) {
		server := newNonceRPCServer(t, map[string]string{"pending": "0x4", "latest": "0x2"}, nil)
		provider := &AlternativeSendTxProvider{urls: []string{server.URL}, rpcTimeout: time.Second}

		pending, confirmed, confirmedOK, err := provider.getNonces(addr, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pending != 4 || confirmed != 2 || !confirmedOK {
			t.Errorf("got (pending=%d confirmed=%d ok=%v), want (4 2 true)", pending, confirmed, confirmedOK)
		}
	})

	t.Run("gated on confirmed failure is best-effort", func(t *testing.T) {
		// the latest sub-call fails but pending succeeds: pending must still be returned with
		// confirmedOK=false and NO error, so the whole address response survives
		server := newNonceRPCServer(t, map[string]string{"pending": "0x4"}, map[string]bool{"latest": true})
		provider := &AlternativeSendTxProvider{urls: []string{server.URL}, rpcTimeout: time.Second}

		pending, confirmed, confirmedOK, err := provider.getNonces(addr, true)
		if err != nil {
			t.Fatalf("confirmed-nonce failure must not be fatal, got error: %v", err)
		}
		if pending != 4 || confirmed != 0 || confirmedOK {
			t.Errorf("got (pending=%d confirmed=%d ok=%v), want (4 0 false) on best-effort failure", pending, confirmed, confirmedOK)
		}
	})

	t.Run("gated on pending failure is fatal", func(t *testing.T) {
		server := newNonceRPCServer(t, map[string]string{"latest": "0x2"}, map[string]bool{"pending": true})
		provider := &AlternativeSendTxProvider{urls: []string{server.URL}, rpcTimeout: time.Second}

		if _, _, _, err := provider.getNonces(addr, true); err == nil {
			t.Fatal("expected fatal error when the required pending nonce cannot be obtained")
		}
	})

	t.Run("batch transport failure is fatal", func(t *testing.T) {
		// an unreachable provider makes the batch round-trip fail at transport level
		provider := &AlternativeSendTxProvider{urls: []string{"http://127.0.0.1:1"}, rpcTimeout: time.Second}

		if _, _, _, err := provider.getNonces(addr, true); err == nil {
			t.Fatal("expected fatal error on batch transport failure")
		}
	})
}

// signedTestTx builds a signed raw transaction with a throwaway key and returns its hex
// encoding together with the sender address the key derives to.
func signedTestTx(t *testing.T) (string, ethcommon.Address) {
	t.Helper()
	key, err := crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
	if err != nil {
		t.Fatalf("HexToECDSA() error = %v", err)
	}
	to := ethcommon.HexToAddress("0x3333333333333333333333333333333333333333")
	tx, err := types.SignNewTx(key, types.LatestSignerForChainID(big.NewInt(1)), &types.LegacyTx{
		Nonce:    1,
		GasPrice: big.NewInt(1),
		Gas:      21000,
		To:       &to,
		Value:    big.NewInt(0),
	})
	if err != nil {
		t.Fatalf("SignNewTx() error = %v", err)
	}
	raw, err := tx.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() error = %v", err)
	}
	return hexutil.Encode(raw), crypto.PubkeyToAddress(key.PublicKey)
}

func TestAlternativeSendTxProviderUseForNonces(t *testing.T) {
	recent := ethcommon.HexToAddress("0x2222222222222222222222222222222222222222")
	expired := ethcommon.HexToAddress("0x3333333333333333333333333333333333333333")
	unknown := ethcommon.HexToAddress("0x4444444444444444444444444444444444444444")
	provider := &AlternativeSendTxProvider{
		mempoolTxsTimeout: time.Hour,
		recentSenders: map[ethcommon.Address]recentSender{
			recent:  {time: time.Now()},
			expired: {time: time.Now().Add(-2 * time.Hour)},
		},
	}

	if !provider.useForNonces(recent) {
		t.Error("recent sender not routed to the alternative provider")
	}
	if provider.useForNonces(unknown) {
		t.Error("unknown address routed to the alternative provider")
	}
	if provider.useForNonces(expired) {
		t.Error("expired sender routed to the alternative provider")
	}
	if _, found := provider.recentSenders[expired]; found {
		t.Error("expired sender not evicted on lookup")
	}
}

func TestAlternativeSendTxProviderSendRecordsSender(t *testing.T) {
	rawTx, sender := signedTestTx(t)
	// callHttpStringResult dials a fresh client per call, so its first request always has id 1
	sendTxResponse := `{"jsonrpc":"2.0","id":1,"result":"` + testAlternativeTxID + `"}`

	t.Run("successful send records the decoded sender", func(t *testing.T) {
		server := newAlternativeTxProviderTestServer(t, sendTxResponse)
		// recentSenders left nil to also cover the lazy initialization on write
		provider := &AlternativeSendTxProvider{urls: []string{server.URL}, mempoolTxsTimeout: time.Hour, rpcTimeout: time.Second}

		if _, err := provider.SendRawTransaction(rawTx); err != nil {
			t.Fatalf("SendRawTransaction() error = %v", err)
		}
		if !provider.useForNonces(sender) {
			t.Error("sender not routed to the alternative provider after a successful send")
		}
		if s := provider.recentSenders[sender]; s.url != server.URL {
			t.Errorf("recorded accepting url = %q, want %q", s.url, server.URL)
		}
	})

	t.Run("failed send records nothing", func(t *testing.T) {
		provider := &AlternativeSendTxProvider{urls: []string{"http://127.0.0.1:1"}, mempoolTxsTimeout: time.Hour, rpcTimeout: time.Second}

		if _, err := provider.SendRawTransaction(rawTx); err == nil {
			t.Fatal("expected error from unreachable provider")
		}
		if provider.useForNonces(sender) {
			t.Error("sender recorded despite failed send")
		}
	})

	t.Run("undecodable transaction records nothing", func(t *testing.T) {
		server := newAlternativeTxProviderTestServer(t, sendTxResponse)
		provider := &AlternativeSendTxProvider{urls: []string{server.URL}, mempoolTxsTimeout: time.Hour, rpcTimeout: time.Second}

		if _, err := provider.SendRawTransaction("0xdeadbeef"); err != nil {
			t.Fatalf("SendRawTransaction() error = %v", err)
		}
		if len(provider.recentSenders) != 0 {
			t.Errorf("recentSenders has %d entries, want 0 after undecodable raw tx", len(provider.recentSenders))
		}
	})

	t.Run("nonce reads follow the accepting provider", func(t *testing.T) {
		// urls[0] is unreachable: the broadcast succeeds only through the second provider, so
		// nonce reads for the sender must go there too - urls[0] never saw the transaction.
		// The nonce server answers eth_sendRawTransaction via the empty tag and
		// eth_getTransactionCount via the "pending" tag.
		server := newNonceRPCServer(t, map[string]string{"": testAlternativeTxID, "pending": "0x9"}, nil)
		provider := &AlternativeSendTxProvider{
			urls:              []string{"http://127.0.0.1:1", server.URL},
			mempoolTxsTimeout: time.Hour,
			rpcTimeout:        time.Second,
		}

		if _, err := provider.SendRawTransaction(rawTx); err != nil {
			t.Fatalf("SendRawTransaction() error = %v", err)
		}
		pending, _, _, err := provider.getNonces(sender, false)
		if err != nil {
			t.Fatalf("getNonces() error = %v", err)
		}
		if pending != 9 {
			t.Errorf("pending = %d, want 9 from the provider that accepted the send", pending)
		}
		if got := server.callCount("pending"); got != 1 {
			t.Errorf("accepting provider queried %d times for pending, want 1", got)
		}
	})

	t.Run("batched nonce read follows the accepting provider", func(t *testing.T) {
		// withConfirmed=true exercises the batch branch of getNonces, which dials the url
		// itself instead of going through callHttpStringResult - it must pick the accepting
		// provider the same way as the pending-only branch
		server := newNonceRPCServer(t, map[string]string{"": testAlternativeTxID, "pending": "0x9", "latest": "0x5"}, nil)
		provider := &AlternativeSendTxProvider{
			urls:              []string{"http://127.0.0.1:1", server.URL},
			mempoolTxsTimeout: time.Hour,
			rpcTimeout:        time.Second,
		}

		if _, err := provider.SendRawTransaction(rawTx); err != nil {
			t.Fatalf("SendRawTransaction() error = %v", err)
		}
		pending, confirmed, confirmedOK, err := provider.getNonces(sender, true)
		if err != nil {
			t.Fatalf("getNonces() error = %v", err)
		}
		if pending != 9 || confirmed != 5 || !confirmedOK {
			t.Errorf("got (pending=%d confirmed=%d ok=%v), want (9 5 true) from the provider that accepted the send", pending, confirmed, confirmedOK)
		}
	})

	t.Run("send sweeps expired senders", func(t *testing.T) {
		server := newAlternativeTxProviderTestServer(t, sendTxResponse)
		stale := ethcommon.HexToAddress("0x5555555555555555555555555555555555555555")
		provider := &AlternativeSendTxProvider{
			urls:              []string{server.URL},
			mempoolTxsTimeout: time.Hour,
			rpcTimeout:        time.Second,
			recentSenders:     map[ethcommon.Address]recentSender{stale: {time: time.Now().Add(-2 * time.Hour)}},
		}

		if _, err := provider.SendRawTransaction(rawTx); err != nil {
			t.Fatalf("SendRawTransaction() error = %v", err)
		}
		if _, found := provider.recentSenders[stale]; found {
			t.Error("expired sender not swept on send")
		}
		if _, found := provider.recentSenders[sender]; !found {
			t.Error("new sender not recorded")
		}
	})
}

func TestAlternativeSendTxProviderRemoveTransactionReleasesSender(t *testing.T) {
	sender := ethcommon.HexToAddress("0x2222222222222222222222222222222222222222")
	cachedTx := func(nonce string, gen uint64) storedTx {
		return storedTx{
			tx: &bchain.RpcTransaction{
				Hash:         testAlternativeTxID,
				From:         "0x2222222222222222222222222222222222222222",
				AccountNonce: nonce,
			},
			time: uint32(time.Now().Unix()),
			gen:  gen,
		}
	}
	makeProvider := func(senderGen uint64, txs map[string]storedTx) *AlternativeSendTxProvider {
		return &AlternativeSendTxProvider{
			fetchMempoolTx:    true,
			mempoolTxsTimeout: time.Hour,
			mempoolTxs:        txs,
			recentSenders:     map[ethcommon.Address]recentSender{sender: {time: time.Now(), gen: senderGen}},
		}
	}

	t.Run("evicting the last cached tx releases the sender", func(t *testing.T) {
		provider := makeProvider(1, map[string]storedTx{testAlternativeTxID: cachedTx("0x1", 1)})

		provider.RemoveTransaction(testAlternativeTxID)

		if provider.useForNonces(sender) {
			t.Error("sender still routed to the alternative provider after its last cached tx settled")
		}
	})

	t.Run("another cached tx from the sender keeps the entry", func(t *testing.T) {
		provider := makeProvider(2, map[string]storedTx{
			testAlternativeTxID:       cachedTx("0x1", 1),
			testAlternativeSecondTxID: cachedTx("0x2", 2),
		})

		provider.RemoveTransaction(testAlternativeTxID)

		if !provider.useForNonces(sender) {
			t.Error("sender released while another of its txs is still cached")
		}
	})

	t.Run("a newer send since the evicted tx keeps the entry", func(t *testing.T) {
		// the sender submitted again after the evicted tx was cached (possibly without a cache
		// entry of its own, e.g. when the post-send fetch-back failed) - the entry must survive.
		// The generation counter orders the sends precisely, so this holds even when both sends
		// landed within the same wall-clock second.
		provider := makeProvider(2, map[string]storedTx{testAlternativeTxID: cachedTx("0x1", 1)})

		provider.RemoveTransaction(testAlternativeTxID)

		if !provider.useForNonces(sender) {
			t.Error("sender released although a newer private send may still be pending")
		}
	})

	t.Run("unknown txid releases nothing", func(t *testing.T) {
		provider := makeProvider(1, map[string]storedTx{testAlternativeTxID: cachedTx("0x1", 1)})

		provider.RemoveTransaction("0xdoesnotexist")

		if !provider.useForNonces(sender) {
			t.Error("sender released by removal of an unknown txid")
		}
	})
}

func TestAlternativeSendTxProviderHandleMempoolTransactionStampsOwnGeneration(t *testing.T) {
	// The cached entry must carry the generation of ITS OWN submission, not the sender's
	// current generation: the fetch-back is a network round-trip during which a concurrent
	// send can bump the sender's generation. Simulated here: transaction A (generation 1)
	// finishes its slow fetch-back after a concurrent transaction B (generation 2) was
	// registered but left no cache entry because B's own fetch-back failed. Stamping A with
	// generation 2 would make A's eviction release the sender's routing while B is still
	// privately pending.
	server := newAlternativeTxProviderTestServer(t, testAlternativeKnownTxResponse)
	sender := ethcommon.HexToAddress("0x2222222222222222222222222222222222222222")
	provider := &AlternativeSendTxProvider{
		urls:              []string{server.URL},
		fetchMempoolTx:    true,
		mempoolTxsTimeout: time.Hour,
		rpcTimeout:        time.Second,
		mempoolTxs:        map[string]storedTx{},
		recentSenders:     map[ethcommon.Address]recentSender{sender: {time: time.Now(), gen: 2}},
	}

	if _, err := provider.handleMempoolTransaction(testAlternativeTxID, 1); err != nil {
		t.Fatalf("handleMempoolTransaction() error = %v", err)
	}
	if got := provider.mempoolTxs[testAlternativeTxID].gen; got != 1 {
		t.Errorf("cached tx generation = %d, want 1 (the generation of its own submission)", got)
	}

	// evicting A must keep the routing alive for the uncached, possibly still pending B
	provider.RemoveTransaction(testAlternativeTxID)

	if !provider.useForNonces(sender) {
		t.Error("sender routing released although a newer private send (generation 2) may still be pending")
	}
}

func TestAlternativeSendTxProviderPendingNonceFloor(t *testing.T) {
	sender := ethcommon.HexToAddress("0x2222222222222222222222222222222222222222")
	provider := &AlternativeSendTxProvider{
		mempoolTxs: map[string]storedTx{
			"0x01": {tx: &bchain.RpcTransaction{From: "0x2222222222222222222222222222222222222222", AccountNonce: "0x4"}},
			"0x02": {tx: &bchain.RpcTransaction{From: "0x2222222222222222222222222222222222222222", AccountNonce: "0x7"}},
			"0x03": {tx: &bchain.RpcTransaction{From: "0x2222222222222222222222222222222222222222", AccountNonce: "0xZZ"}}, // unparsable, skipped
			"0x04": {tx: &bchain.RpcTransaction{From: "0x3333333333333333333333333333333333333333", AccountNonce: "0x9"}},
		},
	}

	floor, found := provider.pendingNonceFloor(sender)
	if !found {
		t.Fatal("no floor found for sender with cached txs")
	}
	if floor != 8 {
		t.Errorf("floor = %d, want 8 (highest cached nonce 0x7 + 1)", floor)
	}
	if _, found := provider.pendingNonceFloor(ethcommon.HexToAddress("0x4444444444444444444444444444444444444444")); found {
		t.Error("floor found for address without cached txs")
	}
}
