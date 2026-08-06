package eth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
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

// TestAlternativeSendTxProviderReconcileBacksOffByAge pins the probe pacing that keeps a long cache
// retention from turning into one relay round-trip per entry per minute for hours. The tick is
// unchanged; what changes is which entries it re-asks about.
func TestAlternativeSendTxProviderReconcileBacksOffByAge(t *testing.T) {
	for _, tt := range []struct {
		name       string
		age        time.Duration
		sinceProbe time.Duration
		wantProbes int
		wantAction string
	}{
		{name: "young entry is probed every cycle", age: 5 * time.Minute, sinceProbe: 90 * time.Second, wantProbes: 1, wantAction: "provider_missing_pending"},
		{name: "waiting entry is not re-asked within its interval", age: 30 * time.Minute, sinceProbe: 90 * time.Second, wantProbes: 0, wantAction: "skipped_backoff"},
		{name: "waiting entry is re-asked once its interval elapses", age: 30 * time.Minute, sinceProbe: 6 * time.Minute, wantProbes: 1, wantAction: "provider_missing_pending"},
		{name: "hour-old entry backs off further", age: 2 * time.Hour, sinceProbe: 6 * time.Minute, wantProbes: 0, wantAction: "skipped_backoff"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := newMethodAwareTxProviderTestServer(t, map[string]string{
				"eth_getTransactionByHash": `{"jsonrpc":"2.0","id":1,"result":null}`,
			})
			var removed string
			provider := newTestAlternativeSendTxProvider(server.URL, &removed)
			provider.mempoolTxsTimeout = 3 * time.Hour
			provider.metrics = newReconcileTestMetrics()
			entry := provider.mempoolTxs[testAlternativeTxID]
			entry.time = uint32(time.Now().Add(-tt.age).Unix())
			entry.lastProbe = uint32(time.Now().Add(-tt.sinceProbe).Unix())
			provider.mempoolTxs[testAlternativeTxID] = entry

			provider.reconcileMempoolTxs()

			if got := server.callCount("eth_getTransactionByHash"); got != tt.wantProbes {
				t.Errorf("relay probes = %d, want %d", got, tt.wantProbes)
			}
			if got := labeledCounterValue(t, provider.metrics.EthAlternativeMempoolEvents, "action", tt.wantAction); got != 1 {
				t.Errorf("reconciliation_events{action=%s} = %v, want 1", tt.wantAction, got)
			}
			if _, found := provider.mempoolTxs[testAlternativeTxID]; !found {
				t.Error("entry evicted although it is neither timed out nor superseded")
			}
		})
	}
}

// TestAlternativeSendTxProviderReconcileEvictsTimedOutEntryDespiteBackoff pins that the backoff paces
// only the relay round-trip: the cache timeout is a local decision and must still fire on the cycle it
// comes due, or a backed-off entry would outlive the retention by up to a probe interval.
func TestAlternativeSendTxProviderReconcileEvictsTimedOutEntryDespiteBackoff(t *testing.T) {
	server := newMethodAwareTxProviderTestServer(t, map[string]string{
		"eth_getTransactionByHash": `{"jsonrpc":"2.0","id":1,"result":null}`,
	})
	var removed string
	provider := newTestAlternativeSendTxProvider(server.URL, &removed)
	provider.metrics = newReconcileTestMetrics()
	entry := provider.mempoolTxs[testAlternativeTxID]
	entry.time = uint32(time.Now().Add(-2 * time.Hour).Unix()) // past the 1h test retention
	entry.lastProbe = uint32(time.Now().Add(-time.Second).Unix())
	provider.mempoolTxs[testAlternativeTxID] = entry

	provider.reconcileMempoolTxs()

	if _, found := provider.mempoolTxs[testAlternativeTxID]; found {
		t.Error("timed-out entry survived because its probe was backed off")
	}
	if removed != testAlternativeTxID {
		t.Errorf("removed txid = %q, want %q", removed, testAlternativeTxID)
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

	return newHookedMethodAwareTxProviderTestServer(t, responses, nil)
}

// newHookedMethodAwareTxProviderTestServer is newMethodAwareTxProviderTestServer with a hook that runs
// before the response is written. Blocking in the hook lets a test hold a specific RPC method in
// flight - which is how the send path's concurrency and its independence from the fetch-back are
// asserted exactly, without wall-clock margins.
func newHookedMethodAwareTxProviderTestServer(t *testing.T, responses map[string]string, before func(method string)) *methodAwareServer {
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

		if before != nil {
			before(req.Method)
		}

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
		EthAlternativeMempoolOldestAge: prometheus.NewGauge(
			prometheus.GaugeOpts{Name: "test_alt_mempool_oldest_age_seconds"}),
		EthAlternativeSendNotSurfaced: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "test_alt_send_not_surfaced_total"}, []string{"reason"}),
	}
}

// The readers below narrow gatherMetric (alternativesendtx_metrics_test.go) to the shapes the
// reconcile tests assert on, so a test can read metric values without pulling in the
// prometheus/testutil dependency (and its transitive modules). A series that was never touched
// reads as zero, which is what these call sites assert against.

// gaugeValue reads the current value of a single gauge.
func gaugeValue(t *testing.T, g prometheus.Gauge) float64 {
	t.Helper()
	return gatherMetric(t, g, nil).GetGauge().GetValue()
}

// counterVecValue reads the counter series carrying label=value.
func counterVecValue(t *testing.T, cv *prometheus.CounterVec, label, value string) float64 {
	t.Helper()
	return gatherMetric(t, cv, map[string]string{label: value}).GetCounter().GetValue()
}

// residenceSampleCount reports how many residence observations were recorded under action=action.
func residenceSampleCount(t *testing.T, h *prometheus.HistogramVec, action string) uint64 {
	t.Helper()
	return gatherMetric(t, h, map[string]string{"action": action}).GetHistogram().GetSampleCount()
}

// counterValue reads the value of the counter series carrying action=action.
func counterValue(t *testing.T, cv *prometheus.CounterVec, action string) float64 {
	t.Helper()
	return counterVecValue(t, cv, "action", action)
}

// labeledCounterValue reads the counter series carrying label==value (counterValue is fixed to
// "action"); an alias of counterVecValue kept for the call sites this series added.
func labeledCounterValue(t *testing.T, cv *prometheus.CounterVec, label, value string) float64 {
	t.Helper()
	return counterVecValue(t, cv, label, value)
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

		if got := counterVecValue(t, provider.metrics.EthAlternativeMempoolEvents, "action", "mined"); got != 1 {
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

		if got := counterVecValue(t, provider.metrics.EthAlternativeMempoolEvents, "action", "kept"); got != 1 {
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
	if got := counterVecValue(t, provider.metrics.EthAlternativeMempoolEvents, "action", "timeout"); got != 1 {
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
	if got := counterVecValue(t, provider.metrics.EthAlternativeMempoolEvents, "action", "rbf_replaced"); got != 1 {
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
	to := ethcommon.HexToAddress("0x3333333333333333333333333333333333333333")
	raw, sender, _ := signTestTx(t, &types.LegacyTx{
		Nonce:    1,
		GasPrice: big.NewInt(1),
		Gas:      21000,
		To:       &to,
		Value:    big.NewInt(0),
	})
	return raw, sender
}

// signedTestTxWithHash is signedTestTx plus the transaction's true hash, for tests whose relay mock
// must echo - and surface - the same txid the signed bytes hash to, which is the txid the send path
// caches under.
func signedTestTxWithHash(t *testing.T) (string, ethcommon.Address, string) {
	t.Helper()
	to := ethcommon.HexToAddress("0x3333333333333333333333333333333333333333")
	raw, sender, tx := signTestTx(t, &types.LegacyTx{
		Nonce:    1,
		GasPrice: big.NewInt(1),
		Gas:      21000,
		To:       &to,
		Value:    big.NewInt(0),
	})
	return raw, sender, tx.Hash().Hex()
}

// signTestTx signs inner with the fixed test key on chain id 1 and returns its raw hex, the sender
// address and the signed transaction itself (so a test can assert against its true hash).
func signTestTx(t *testing.T, inner types.TxData) (string, ethcommon.Address, *types.Transaction) {
	t.Helper()
	key, err := crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
	if err != nil {
		t.Fatalf("HexToECDSA() error = %v", err)
	}
	tx, err := types.SignNewTx(key, types.LatestSignerForChainID(big.NewInt(1)), inner)
	if err != nil {
		t.Fatalf("SignNewTx() error = %v", err)
	}
	raw, err := tx.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() error = %v", err)
	}
	return hexutil.Encode(raw), crypto.PubkeyToAddress(key.PublicKey), tx
}

// TestDecodeAlternativeSendTx checks that the pending-transaction body derived from the signed bytes
// carries the same fields, in the same encoding, that eth_getTransactionByHash would return for the
// same unmined transaction - that equivalence is what lets the send path cache a relay-accepted
// transaction without the relay having to surface it.
func TestDecodeAlternativeSendTx(t *testing.T) {
	to := ethcommon.HexToAddress("0x3333333333333333333333333333333333333333")

	t.Run("legacy transaction", func(t *testing.T) {
		raw, sender, tx := signTestTx(t, &types.LegacyTx{
			Nonce:    7,
			GasPrice: big.NewInt(1000000000),
			Gas:      21000,
			To:       &to,
			Value:    big.NewInt(12345),
		})
		sent, err := decodeAlternativeSendTx(raw)
		if err != nil {
			t.Fatalf("decodeAlternativeSendTx() error = %v", err)
		}
		if sent.from != sender || sent.nonce != 7 || sent.txid != tx.Hash().Hex() {
			t.Fatalf("got (from=%s nonce=%d txid=%s), want (%s 7 %s)", sent.from.Hex(), sent.nonce, sent.txid, sender.Hex(), tx.Hash().Hex())
		}
		want := bchain.RpcTransaction{
			AccountNonce: "0x7",
			GasPrice:     "0x3b9aca00",
			GasLimit:     "0x5208",
			To:           strings.ToLower(to.Hex()),
			Value:        "0x3039",
			Payload:      "0x",
			Hash:         tx.Hash().Hex(),
			From:         strings.ToLower(sender.Hex()),
		}
		if *sent.body != want {
			t.Errorf("body = %+v, want %+v", *sent.body, want)
		}
	})

	t.Run("dynamic fee transaction with payload", func(t *testing.T) {
		raw, sender, tx := signTestTx(t, &types.DynamicFeeTx{
			ChainID:   big.NewInt(1),
			Nonce:     3,
			GasTipCap: big.NewInt(2000000000),
			GasFeeCap: big.NewInt(30000000000),
			Gas:       60000,
			To:        &to,
			Value:     big.NewInt(0),
			Data:      ethcommon.FromHex("0xa9059cbb"),
		})
		sent, err := decodeAlternativeSendTx(raw)
		if err != nil {
			t.Fatalf("decodeAlternativeSendTx() error = %v", err)
		}
		want := bchain.RpcTransaction{
			AccountNonce: "0x3",
			// a pending EIP-1559 transaction reports its fee cap as gasPrice
			GasPrice:             "0x6fc23ac00",
			MaxFeePerGas:         "0x6fc23ac00",
			MaxPriorityFeePerGas: "0x77359400",
			GasLimit:             "0xea60",
			To:                   strings.ToLower(to.Hex()),
			Value:                "0x0",
			Payload:              "0xa9059cbb",
			Hash:                 tx.Hash().Hex(),
			From:                 strings.ToLower(sender.Hex()),
		}
		if *sent.body != want {
			t.Errorf("body = %+v, want %+v", *sent.body, want)
		}
	})

	t.Run("contract creation has no recipient", func(t *testing.T) {
		raw, _, _ := signTestTx(t, &types.LegacyTx{
			Nonce:    0,
			GasPrice: big.NewInt(1),
			Gas:      100000,
			Value:    big.NewInt(0),
			Data:     ethcommon.FromHex("0x60006000"),
		})
		sent, err := decodeAlternativeSendTx(raw)
		if err != nil {
			t.Fatalf("decodeAlternativeSendTx() error = %v", err)
		}
		if sent.body.To != "" {
			t.Errorf("To = %q, want empty for a contract creation", sent.body.To)
		}
	})

	t.Run("access list transaction has no fee-market fields", func(t *testing.T) {
		// type 1 is the other half of the condition that adds maxFeePerGas/maxPriorityFeePerGas, and
		// geth reports neither for it - coverage cannot see this, the branch is shared with legacy
		raw, _, tx := signTestTx(t, &types.AccessListTx{
			ChainID:  big.NewInt(1),
			Nonce:    2,
			GasPrice: big.NewInt(7),
			Gas:      30000,
			To:       &to,
			Value:    big.NewInt(0),
		})
		sent, err := decodeAlternativeSendTx(raw)
		if err != nil {
			t.Fatalf("decodeAlternativeSendTx() error = %v", err)
		}
		if sent.body.MaxFeePerGas != "" || sent.body.MaxPriorityFeePerGas != "" {
			t.Errorf("access list body carries fee-market fields: maxFeePerGas=%q maxPriorityFeePerGas=%q", sent.body.MaxFeePerGas, sent.body.MaxPriorityFeePerGas)
		}
		if sent.body.GasPrice != "0x7" || sent.txid != tx.Hash().Hex() {
			t.Errorf("got (gasPrice=%s txid=%s), want (0x7 %s)", sent.body.GasPrice, sent.txid, tx.Hash().Hex())
		}
	})

	t.Run("unprotected pre-EIP-155 transaction", func(t *testing.T) {
		// ChainId() is a non-nil zero for these, which the latest signer rejects by PANICKING - and a
		// panic here would abort the answer to a wallet whose transaction the relay already accepted
		const unprotected = "0xf85f010182520894333333333333333333333333333333333333333301801ba0e993c688e0fefda43299dc6dffd9142da3d1135e31e7e637f4a479b1a139762ca01d30e00a368007ad7d4cc0102d95f156c3766590ced509d71fc8994153c745a4"
		sent, err := decodeAlternativeSendTx(unprotected)
		if err != nil {
			t.Fatalf("decodeAlternativeSendTx() error = %v", err)
		}
		if want := "0x71562b71999873db5b286df957af199ec94617f7"; sent.body.From != want {
			t.Errorf("from = %q, want %q recovered with the Homestead signer", sent.body.From, want)
		}
	})

	t.Run("undecodable raw hex", func(t *testing.T) {
		if _, err := decodeAlternativeSendTx("0xdeadbeef"); err == nil {
			t.Fatal("decodeAlternativeSendTx() error = nil, want decode failure")
		}
	})

	t.Run("unrecoverable signature", func(t *testing.T) {
		// decodes as RLP but the signature recovers to nothing, so no sender and no (from, nonce) slot
		bogus, err := types.NewTx(&types.LegacyTx{
			Nonce: 1, GasPrice: big.NewInt(1), Gas: 21000, To: &to, Value: big.NewInt(0),
			V: big.NewInt(27), R: big.NewInt(0), S: big.NewInt(1),
		}).MarshalBinary()
		if err != nil {
			t.Fatalf("MarshalBinary() error = %v", err)
		}
		if _, err := decodeAlternativeSendTx(hexutil.Encode(bogus)); err == nil {
			t.Fatal("decodeAlternativeSendTx() error = nil, want sender recovery failure")
		}
	})
}

// TestAlternativeSendTxProviderAcceptedSendCachedWithoutFetchBack covers the send-not-surfaced
// hole: a relay that ACKs the send but never returns the transaction from eth_getTransactionByHash
// used to leave it cached and indexed nowhere - not served as pending and not raising the
// pending-nonce floor, so the next send could reuse its nonce. The send path now caches it from its
// own signed bytes, so the failed fetch-back only costs the metric.
func TestAlternativeSendTxProviderAcceptedSendCachedWithoutFetchBack(t *testing.T) {
	to := ethcommon.HexToAddress("0x3333333333333333333333333333333333333333")
	rawTx, sender, tx := signTestTx(t, &types.LegacyTx{
		Nonce:    4,
		GasPrice: big.NewInt(1),
		Gas:      21000,
		To:       &to,
		Value:    big.NewInt(1),
	})
	server := newMethodAwareTxProviderTestServer(t, map[string]string{
		"eth_sendRawTransaction": `{"jsonrpc":"2.0","id":1,"result":"` + tx.Hash().Hex() + `"}`,
		// the relay accepted the transaction but does not surface it
		"eth_getTransactionByHash": `{"jsonrpc":"2.0","id":1,"result":null}`,
	})
	metrics := newReconcileTestMetrics()
	provider := &AlternativeSendTxProvider{
		urls:              []string{server.URL},
		onlyAlternative:   true,
		fetchMempoolTx:    true,
		mempoolTxs:        map[string]storedTx{},
		mempoolTxsTimeout: time.Hour,
		rpcTimeout:        time.Second,
		metrics:           metrics,
	}

	txid, err := provider.SendRawTransaction(rawTx)
	if err != nil {
		t.Fatalf("SendRawTransaction() error = %v", err)
	}
	provider.waitForRefreshes()
	if txid != tx.Hash().Hex() {
		t.Fatalf("txid = %q, want %q", txid, tx.Hash().Hex())
	}
	if server.callCount("eth_getTransactionByHash") == 0 {
		t.Error("fetch-back was not attempted")
	}

	cached, found := provider.GetTransaction(txid)
	if !found {
		t.Fatal("accepted transaction is not served as pending after a failed fetch-back")
	}
	if cached.From != strings.ToLower(sender.Hex()) || cached.AccountNonce != "0x4" {
		t.Errorf("cached body = (from=%s nonce=%s), want (%s 0x4)", cached.From, cached.AccountNonce, strings.ToLower(sender.Hex()))
	}
	if floor, stranded := provider.raiseToPendingFloor(sender, 4); floor != 5 || stranded {
		t.Errorf("raiseToPendingFloor(4) = (%d, %v), want (5, false)", floor, stranded)
	}
	if got := counterVecValue(t, metrics.EthAlternativeSendNotSurfaced, "reason", "not_found"); got != 1 {
		t.Errorf("send_not_surfaced{not_found} = %v, want 1", got)
	}
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

// TestAlternativeSendTxProviderRoutingHorizonIsIndependentOfRetention pins the split the long cache
// retention forces: a sender stops being routed to the relay well before its transaction stops being
// exposed as pending. Routing is a rate-quota decision (eth_estimateGas rides the same gate, once per
// send-form keystroke - #1629); keeping the nonce reserved is the cache's job, and the floor answers
// from the primary RPC just as well.
func TestAlternativeSendTxProviderRoutingHorizonIsIndependentOfRetention(t *testing.T) {
	const senderHex = "0x2222222222222222222222222222222222222222"
	sender := ethcommon.HexToAddress(senderHex)
	provider := &AlternativeSendTxProvider{
		mempoolTxsTimeout: 3 * time.Hour,
		recentSenders: map[ethcommon.Address]recentSender{
			sender: {time: time.Now().Add(-30 * time.Minute)},
		},
		acceptedSlots: map[nonceSlot]acceptedSlot{
			{addr: sender, nonce: 4}: {gen: 1, time: time.Now().Add(-30 * time.Minute)},
		},
		mempoolTxs: map[string]storedTx{
			testAlternativeTxID: {tx: &bchain.RpcTransaction{From: senderHex, AccountNonce: "0x4"}, gen: 1},
		},
	}

	if provider.useForNonces(sender) {
		t.Error("sender still routed to the relay 30 min after its send, twice the routing horizon")
	}
	if floor, stranded := provider.raiseToPendingFloor(sender, 4); floor != 5 || stranded {
		t.Errorf("raiseToPendingFloor(4) = (%d, %v), want (5, false) - the cached tx must keep its nonce reserved after routing lapsed", floor, stranded)
	}
	// the acceptance must outlive routing: it is what retires a predecessor whose replacement the
	// relay never surfaces (#1573), and a predecessor can land for as long as the cache holds it
	if !provider.slotSupersededBy(sender, 4, 0) {
		t.Error("accepted nonce slot swept on the routing horizon instead of the cache retention")
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

// TestAlternativeSendTxProviderRaiseToPendingFloor pins the contiguity clamp of #1675: the floor
// advances the backend's own pending answer across the run of cached nonces that starts at it, and
// never across a hole. The pre-#1675 rule was a blind max(cached)+1, which answered 8 for the gap
// case below and queued every later send behind a nonce nothing was filling.
func TestAlternativeSendTxProviderRaiseToPendingFloor(t *testing.T) {
	const senderHex = "0x2222222222222222222222222222222222222222"
	sender := ethcommon.HexToAddress(senderHex)
	cached := func(nonces ...string) map[string]storedTx {
		txs := make(map[string]storedTx, len(nonces)+1)
		for i, nonce := range nonces {
			txs[fmt.Sprintf("0x%02d", i)] = storedTx{tx: &bchain.RpcTransaction{From: senderHex, AccountNonce: nonce}}
		}
		// another sender's pending tx must never move this one's floor
		txs["0xff"] = storedTx{tx: &bchain.RpcTransaction{From: "0x3333333333333333333333333333333333333333", AccountNonce: "0x9"}}

		return txs
	}

	tests := []struct {
		name         string
		addr         ethcommon.Address
		cached       map[string]storedTx
		pending      uint64
		wantFloor    uint64
		wantStranded bool
	}{
		{name: "no cached tx: the backend answer stands", addr: sender, cached: cached(), pending: 4, wantFloor: 4},
		{name: "cached at the pending nonce", addr: sender, cached: cached("0x4"), pending: 4, wantFloor: 5},
		{name: "contiguous run", addr: sender, cached: cached("0x4", "0x5", "0x6"), pending: 4, wantFloor: 7},
		{name: "gap: nothing fills the pending nonce", addr: sender, cached: cached("0x7"), pending: 4, wantFloor: 4, wantStranded: true},
		{name: "run then island", addr: sender, cached: cached("0x4", "0x5", "0x8"), pending: 4, wantFloor: 6, wantStranded: true},
		{name: "cached below the pending nonce is already consumed", addr: sender, cached: cached("0x2", "0x3"), pending: 4, wantFloor: 4},
		{name: "unparsable nonce is skipped", addr: sender, cached: cached("0x4", "0xZZ"), pending: 4, wantFloor: 5},
		{name: "duplicate nonce across two txids counts once", addr: sender, cached: cached("0x4", "0x4"), pending: 4, wantFloor: 5},
		{name: "another sender's cache is invisible", addr: ethcommon.HexToAddress("0x4444444444444444444444444444444444444444"), cached: cached("0x4"), pending: 4, wantFloor: 4},
		{name: "first send of an account", addr: sender, cached: cached("0x0"), pending: 0, wantFloor: 1},
		{name: "the walk cannot wrap below the backend answer", addr: sender, cached: cached(hexutil.EncodeUint64(math.MaxUint64)), pending: math.MaxUint64, wantFloor: math.MaxUint64},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &AlternativeSendTxProvider{mempoolTxs: tt.cached}
			floor, stranded := provider.raiseToPendingFloor(tt.addr, tt.pending)
			if floor != tt.wantFloor || stranded != tt.wantStranded {
				t.Errorf("raiseToPendingFloor(%d) = (%d, %v), want (%d, %v)", tt.pending, floor, stranded, tt.wantFloor, tt.wantStranded)
			}
			if floor < tt.pending {
				t.Errorf("floor %d fell below the backend's pending answer %d", floor, tt.pending)
			}
		})
	}
}

// cachedPredecessor builds a cache entry for a transaction from `from` with the given hex nonce,
// aged past the reconcile grace period, so a test can assert it is retired by a same-nonce
// replacement rather than by any reconcile timing.
func cachedPredecessor(txid string, from ethcommon.Address, nonceHex string) storedTx {
	return storedTx{
		tx: &bchain.RpcTransaction{
			Hash:         txid,
			From:         from.Hex(),
			AccountNonce: nonceHex,
		},
		time: uint32(time.Now().Add(-3 * time.Minute).Unix()),
	}
}

// TestAlternativeSendTxProviderAcceptedSendRetiresUnsurfacedPredecessor is the core of #1573: a
// replacement or drop-mode cancel accepted by the relay retires the cached predecessor sharing its
// (from, nonce) immediately, even when the relay never surfaces the replacement via
// eth_getTransactionByHash. Before the fix the predecessor lingered as "Unconfirmed" until the
// cache timeout because eviction was gated behind a successful fetch-back.
func TestAlternativeSendTxProviderAcceptedSendRetiresUnsurfacedPredecessor(t *testing.T) {
	rawTx, sender, sentTxID := signedTestTxWithHash(t) // nonce 1
	// relay accepts the send but never surfaces it (drop mode): getTransactionByHash returns null
	server := newMethodAwareTxProviderTestServer(t, map[string]string{
		"eth_sendRawTransaction":   `{"jsonrpc":"2.0","id":1,"result":"` + sentTxID + `"}`,
		"eth_getTransactionByHash": `{"jsonrpc":"2.0","id":1,"result":null}`,
	})
	provider := &AlternativeSendTxProvider{
		urls:              []string{server.URL},
		fetchMempoolTx:    true,
		onlyAlternative:   true,
		rpcTimeout:        time.Second,
		mempoolTxsTimeout: time.Hour,
		mempoolTxs: map[string]storedTx{
			testAlternativeSecondTxID: cachedPredecessor(testAlternativeSecondTxID, sender, "0x1"),
		},
		metrics: newReconcileTestMetrics(),
	}
	var removed string
	provider.removeTransactionFromMempool = func(txid string) {
		removed = txid
		provider.RemoveTransaction(txid)
	}

	if _, err := provider.SendRawTransaction(rawTx); err != nil {
		t.Fatalf("SendRawTransaction() error = %v", err)
	}
	// checked before waiting for the background fetch-back: the retirement must have happened by the
	// time the wallet is answered, not merely by the time the relay has been asked about the send
	if _, found := provider.GetTransaction(testAlternativeSecondTxID); found {
		t.Fatal("predecessor still served as pending when the send returned")
	}
	provider.waitForRefreshes()

	if removed != testAlternativeSecondTxID {
		t.Fatalf("removed txid = %q, want %q (predecessor must be retired on acceptance)", removed, testAlternativeSecondTxID)
	}
	if _, found := provider.mempoolTxs[testAlternativeSecondTxID]; found {
		t.Fatal("predecessor remained in cache after an accepted same-nonce replacement")
	}
	if got := counterVecValue(t, provider.metrics.EthAlternativeMempoolEvents, "action", "rbf_replaced"); got != 1 {
		t.Errorf("rbf_replaced events = %v, want 1", got)
	}
	if got := residenceSampleCount(t, provider.metrics.EthAlternativeMempoolTxResidence, "rbf_replaced"); got != 1 {
		t.Errorf("rbf_replaced residence sample count = %d, want 1", got)
	}
}

// TestAlternativeSendTxProviderAcceptedSendCachesAndRetiresSurfacedReplacement covers the ordinary
// surfaced RBF path through SendRawTransaction: the replacement is fetched back and cached, the
// predecessor is retired, and the fee-replacement exit is counted exactly once (not doubled by the
// acceptance-time and handleMempoolTransaction removals both firing).
func TestAlternativeSendTxProviderAcceptedSendCachesAndRetiresSurfacedReplacement(t *testing.T) {
	rawTx, sender, replacementTxID := signedTestTxWithHash(t) // nonce 1
	// the surfaced replacement shares the sender and nonce of the predecessor
	surfaced := `{"jsonrpc":"2.0","id":1,"result":{"hash":"` + replacementTxID + `","from":"` + sender.Hex() + `","nonce":"0x1","gas":"0x5208","value":"0x0","input":"0x","to":"0x3333333333333333333333333333333333333333"}}`
	server := newMethodAwareTxProviderTestServer(t, map[string]string{
		"eth_sendRawTransaction":   `{"jsonrpc":"2.0","id":1,"result":"` + replacementTxID + `"}`,
		"eth_getTransactionByHash": surfaced,
	})
	provider := &AlternativeSendTxProvider{
		urls:              []string{server.URL},
		fetchMempoolTx:    true,
		onlyAlternative:   true,
		rpcTimeout:        time.Second,
		mempoolTxsTimeout: time.Hour,
		mempoolTxs: map[string]storedTx{
			testAlternativeSecondTxID: cachedPredecessor(testAlternativeSecondTxID, sender, "0x1"),
		},
		metrics: newReconcileTestMetrics(),
	}
	provider.removeTransactionFromMempool = func(txid string) { provider.RemoveTransaction(txid) }

	if _, err := provider.SendRawTransaction(rawTx); err != nil {
		t.Fatalf("SendRawTransaction() error = %v", err)
	}
	provider.waitForRefreshes()

	if _, found := provider.mempoolTxs[testAlternativeSecondTxID]; found {
		t.Fatal("predecessor remained in cache after a surfaced same-nonce replacement")
	}
	cached, found := provider.mempoolTxs[replacementTxID]
	if !found {
		t.Fatal("surfaced replacement was not cached")
	}
	// the entry is the one derived from the signed bytes; the fetch-back only probes, and it did run
	if cached.tx.GasPrice == "" {
		t.Error("cached body is not the one derived from the signed bytes")
	}
	if got := server.callCount("eth_getTransactionByHash"); got != 1 {
		t.Errorf("eth_getTransactionByHash calls = %d, want 1", got)
	}
	if got := counterVecValue(t, provider.metrics.EthAlternativeMempoolEvents, "action", "rbf_replaced"); got != 1 {
		t.Errorf("rbf_replaced events = %v, want 1 (must not double-count)", got)
	}
}

// TestAlternativeSendTxProviderAcceptedSendKeepsDifferentNonce confirms the acceptance-time eviction
// is scoped to the same nonce slot: a cached transaction with a different nonce is a distinct,
// still-mineable in-flight tx and must be left in the cache.
func TestAlternativeSendTxProviderAcceptedSendKeepsDifferentNonce(t *testing.T) {
	rawTx, sender, sentTxID := signedTestTxWithHash(t) // nonce 1
	server := newMethodAwareTxProviderTestServer(t, map[string]string{
		"eth_sendRawTransaction":   `{"jsonrpc":"2.0","id":1,"result":"` + sentTxID + `"}`,
		"eth_getTransactionByHash": `{"jsonrpc":"2.0","id":1,"result":null}`,
	})
	provider := &AlternativeSendTxProvider{
		urls:              []string{server.URL},
		fetchMempoolTx:    true,
		onlyAlternative:   true,
		rpcTimeout:        time.Second,
		mempoolTxsTimeout: time.Hour,
		mempoolTxs: map[string]storedTx{
			// same sender but nonce 2 - not the slot the accepted nonce-1 tx fills
			testAlternativeSecondTxID: cachedPredecessor(testAlternativeSecondTxID, sender, "0x2"),
		},
		metrics: newReconcileTestMetrics(),
	}
	var removed string
	provider.removeTransactionFromMempool = func(txid string) {
		removed = txid
		provider.RemoveTransaction(txid)
	}

	if _, err := provider.SendRawTransaction(rawTx); err != nil {
		t.Fatalf("SendRawTransaction() error = %v", err)
	}
	provider.waitForRefreshes()

	if removed != "" {
		t.Fatalf("removed txid = %q, want none (different-nonce tx must be kept)", removed)
	}
	if _, found := provider.mempoolTxs[testAlternativeSecondTxID]; !found {
		t.Fatal("different-nonce transaction was evicted, want kept")
	}
}

// TestAlternativeSendTxProviderRejectedSendSkipsMempoolFetch verifies that when every relay rejects
// the broadcast (no acceptedURL, empty txid) the provider does not run the mempool-cache path:
// no eth_getTransactionByHash fan-out for the zero hash and no spurious error log (#1629 follow-up).
func TestAlternativeSendTxProviderRejectedSendSkipsMempoolFetch(t *testing.T) {
	rawTx, _ := signedTestTx(t)
	server := newMethodAwareTxProviderTestServer(t, map[string]string{
		"eth_sendRawTransaction": `{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"nonce too low"}}`,
	})
	provider := &AlternativeSendTxProvider{
		urls:              []string{server.URL},
		fetchMempoolTx:    true,
		onlyAlternative:   true,
		rpcTimeout:        time.Second,
		mempoolTxsTimeout: time.Hour,
		mempoolTxs:        map[string]storedTx{},
	}

	if _, err := provider.SendRawTransaction(rawTx); err == nil {
		t.Fatal("SendRawTransaction() error = nil, want rejection error")
	}
	provider.waitForRefreshes()
	if got := server.callCount("eth_getTransactionByHash"); got != 0 {
		t.Fatalf("eth_getTransactionByHash calls = %d, want 0 (rejected send must not touch the mempool cache path)", got)
	}
}

// TestAlternativeSendTxProviderEvictionMetricsGatedOnRemoval verifies the lifecycle metrics are
// attributed to the single eviction that actually removed a cache entry. A second eviction of the
// same txid (e.g. reconcile working off a stale snapshot after the read-path already evicted the
// entry) must not record another event or residence sample under a different action.
func TestAlternativeSendTxProviderEvictionMetricsGatedOnRemoval(t *testing.T) {
	added := uint32(time.Now().Add(-3 * time.Minute).Unix())
	provider := &AlternativeSendTxProvider{
		fetchMempoolTx:    true,
		mempoolTxsTimeout: time.Hour,
		mempoolTxs: map[string]storedTx{
			testAlternativeTxID: {
				tx:   &bchain.RpcTransaction{Hash: testAlternativeTxID, From: "0x2222222222222222222222222222222222222222", AccountNonce: "0x1"},
				time: added,
			},
		},
		metrics: newReconcileTestMetrics(),
	}
	provider.removeTransactionFromMempool = func(txid string) { provider.RemoveTransaction(txid) }

	// first eviction removes the entry and is metered
	provider.evictMempoolTx("timeout", testAlternativeTxID, added)
	// second eviction targets the already-gone entry under a different action and must be a no-op
	provider.evictMempoolTx("mined", testAlternativeTxID, added)

	if got := counterValue(t, provider.metrics.EthAlternativeMempoolEvents, "timeout"); got != 1 {
		t.Errorf("timeout events = %v, want 1", got)
	}
	if got := counterValue(t, provider.metrics.EthAlternativeMempoolEvents, "mined"); got != 0 {
		t.Errorf("mined events = %v, want 0 (entry already removed, must not be re-counted)", got)
	}
	if got := residenceSampleCount(t, provider.metrics.EthAlternativeMempoolTxResidence, "timeout"); got != 1 {
		t.Errorf("timeout residence samples = %d, want 1", got)
	}
	if got := residenceSampleCount(t, provider.metrics.EthAlternativeMempoolTxResidence, "mined"); got != 0 {
		t.Errorf("mined residence samples = %d, want 0", got)
	}
}

// TestAlternativeSendTxProviderEvictReplacedByNonceRespectsGeneration verifies the RBF eviction is
// generation-ordered: it retires a strictly-older predecessor for the nonce slot but leaves a
// strictly-newer replacement intact, so an older submission's late fetch-back cannot drop the newer
// tx that will actually mine (#1573 follow-up / finding #1).
func TestAlternativeSendTxProviderEvictReplacedByNonceRespectsGeneration(t *testing.T) {
	sender := ethcommon.HexToAddress("0x2222222222222222222222222222222222222222")
	entry := func(txid string, gen uint64) storedTx {
		return storedTx{
			tx:   &bchain.RpcTransaction{Hash: txid, From: sender.Hex(), AccountNonce: "0x1"},
			time: uint32(time.Now().Unix()),
			gen:  gen,
		}
	}

	t.Run("retires every victim for the slot", func(t *testing.T) {
		const thirdTxID = "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		otherNonce := entry(thirdTxID, 2)
		otherNonce.tx.AccountNonce = "0x2"
		var removed []string
		provider := &AlternativeSendTxProvider{
			fetchMempoolTx: true,
			mempoolTxs: map[string]storedTx{
				// two entries for one slot: reachable because insertMempoolTx refuses an insert only
				// when a STRICTLY newer entry already holds it, so a send whose scan ran first still lands
				testAlternativeTxID:       entry(testAlternativeTxID, 1),
				testAlternativeSecondTxID: entry(testAlternativeSecondTxID, 2),
				thirdTxID:                 otherNonce, // different nonce, must survive
			},
			metrics: newReconcileTestMetrics(),
		}
		provider.removeTransactionFromMempool = func(txid string) {
			removed = append(removed, txid)
			provider.RemoveTransaction(txid)
		}

		provider.evictReplacedByNonce(sender, 1, "0xkeep", 3)

		if _, found := provider.mempoolTxs[testAlternativeTxID]; found {
			t.Error("gen-1 predecessor was kept")
		}
		if _, found := provider.mempoolTxs[testAlternativeSecondTxID]; found {
			t.Error("gen-2 predecessor was kept: stopping after the first victim leaves two entries for one nonce slot")
		}
		if _, found := provider.mempoolTxs[thirdTxID]; !found {
			t.Error("an entry for a different nonce was evicted")
		}
		if len(removed) != 2 {
			t.Errorf("removeTransactionFromMempool fired %d times, want 2 (both stores must be cleared per victim)", len(removed))
		}
		if got := counterValue(t, provider.metrics.EthAlternativeMempoolEvents, "rbf_replaced"); got != 2 {
			t.Errorf("rbf_replaced events = %v, want 2", got)
		}
		if got := residenceSampleCount(t, provider.metrics.EthAlternativeMempoolTxResidence, "rbf_replaced"); got != 2 {
			t.Errorf("rbf_replaced residence samples = %d, want 2", got)
		}
	})

	t.Run("keeps a strictly newer replacement", func(t *testing.T) {
		provider := &AlternativeSendTxProvider{
			fetchMempoolTx: true,
			mempoolTxs:     map[string]storedTx{testAlternativeTxID: entry(testAlternativeTxID, 2)},
		}
		provider.removeTransactionFromMempool = func(txid string) { provider.RemoveTransaction(txid) }
		// an older send (keepGen 1) reconciling its own tx must not evict the newer gen-2 entry
		provider.evictReplacedByNonce(sender, 1, "0xolder", 1)
		if _, found := provider.mempoolTxs[testAlternativeTxID]; !found {
			t.Fatal("strictly newer replacement was evicted by an older send")
		}
	})

	t.Run("evicts a strictly older predecessor", func(t *testing.T) {
		provider := &AlternativeSendTxProvider{
			fetchMempoolTx: true,
			mempoolTxs:     map[string]storedTx{testAlternativeSecondTxID: entry(testAlternativeSecondTxID, 1)},
		}
		provider.removeTransactionFromMempool = func(txid string) { provider.RemoveTransaction(txid) }
		provider.evictReplacedByNonce(sender, 1, testAlternativeTxID, 2)
		if _, found := provider.mempoolTxs[testAlternativeSecondTxID]; found {
			t.Fatal("older predecessor was not evicted by a newer replacement")
		}
	})

	t.Run("unknown keeper generation keeps a generation-carrying replacement", func(t *testing.T) {
		// A keeper whose own send order is unknown (keepGen 0 - raw-hex sender recovery failed at
		// send time, though the fetched tx still decodes) must NOT evict a cached replacement that
		// carries a real generation. Evicting it dropped the newer tx that will actually mine and
		// surfaced the stale one (#1573 follow-up / finding #2).
		provider := &AlternativeSendTxProvider{
			fetchMempoolTx: true,
			mempoolTxs:     map[string]storedTx{testAlternativeTxID: entry(testAlternativeTxID, 2)},
		}
		provider.removeTransactionFromMempool = func(txid string) { provider.RemoveTransaction(txid) }
		provider.evictReplacedByNonce(sender, 1, "0xunordered", 0)
		if _, found := provider.mempoolTxs[testAlternativeTxID]; !found {
			t.Fatal("generation-carrying replacement was evicted by an unknown-generation keeper")
		}
	})

	t.Run("unknown keeper generation still evicts an unordered predecessor", func(t *testing.T) {
		// When neither side carries a generation (both 0) the order is genuinely unknown; the keeper
		// is the just-accepted replacement, so retiring the equally-unordered predecessor keeps the
		// #1573 acceptance-time eviction working (and the rbf_replaced metric balanced) for that path.
		provider := &AlternativeSendTxProvider{
			fetchMempoolTx: true,
			mempoolTxs:     map[string]storedTx{testAlternativeSecondTxID: entry(testAlternativeSecondTxID, 0)},
		}
		provider.removeTransactionFromMempool = func(txid string) { provider.RemoveTransaction(txid) }
		provider.evictReplacedByNonce(sender, 1, testAlternativeTxID, 0)
		if _, found := provider.mempoolTxs[testAlternativeSecondTxID]; found {
			t.Fatal("unordered predecessor was not evicted by an unknown-generation keeper")
		}
	})
}

// TestAlternativeSendTxProviderHandleMempoolTransactionSkipsStaleFetchBack verifies that a slow
// fetch-back for an older submission does not cache itself or evict the newer same-nonce
// replacement a concurrent higher-generation send already cached (#1573 follow-up / finding #1).
func TestAlternativeSendTxProviderHandleMempoolTransactionSkipsStaleFetchBack(t *testing.T) {
	sender := ethcommon.HexToAddress("0x2222222222222222222222222222222222222222")
	// getTransactionByHash returns the older tx A (testAlternativeTxID), same sender+nonce as the
	// newer cached replacement B (testAlternativeSecondTxID, gen 2)
	server := newAlternativeTxProviderTestServer(t, `{"jsonrpc":"2.0","id":1,"result":{"hash":"`+testAlternativeTxID+`","from":"`+sender.Hex()+`","nonce":"0x1","gas":"0x5208","value":"0x0","input":"0x","to":"0x3333333333333333333333333333333333333333"}}`)
	provider := &AlternativeSendTxProvider{
		urls:              []string{server.URL},
		fetchMempoolTx:    true,
		onlyAlternative:   true,
		rpcTimeout:        time.Second,
		mempoolTxsTimeout: time.Hour,
		mempoolTxs: map[string]storedTx{
			testAlternativeSecondTxID: {
				tx:   &bchain.RpcTransaction{Hash: testAlternativeSecondTxID, From: sender.Hex(), AccountNonce: "0x1"},
				time: uint32(time.Now().Unix()),
				gen:  2,
			},
		},
	}
	provider.removeTransactionFromMempool = func(txid string) { provider.RemoveTransaction(txid) }

	// A is gen 1 - older than the cached gen-2 replacement B
	if _, err := provider.handleMempoolTransaction(testAlternativeTxID, 1); err != nil {
		t.Fatalf("handleMempoolTransaction() error = %v", err)
	}
	if _, found := provider.mempoolTxs[testAlternativeTxID]; found {
		t.Fatal("stale (older-generation) fetch-back was cached, should be skipped")
	}
	if _, found := provider.mempoolTxs[testAlternativeSecondTxID]; !found {
		t.Fatal("newer replacement was evicted by a stale older-generation fetch-back")
	}
}

// TestAlternativeSendTxProviderSyncRemovalIsMetered verifies the removal path carrying no reconcile
// decision - block sync or the read path, via EthereumRPC.removeTransactionFromMempool - records its
// exit. evictMempoolTx meters only the goroutine whose delete removed the entry, and block sync
// almost always beats the next reconcile probe, so this exit was counted nowhere.
func TestAlternativeSendTxProviderSyncRemovalIsMetered(t *testing.T) {
	added := uint32(time.Now().Add(-3 * time.Minute).Unix())
	newProvider := func() *AlternativeSendTxProvider {
		provider := &AlternativeSendTxProvider{
			fetchMempoolTx:    true,
			mempoolTxsTimeout: time.Hour,
			mempoolTxs: map[string]storedTx{
				testAlternativeTxID: {
					tx:   &bchain.RpcTransaction{Hash: testAlternativeTxID, From: "0x2222222222222222222222222222222222222222", AccountNonce: "0x1"},
					time: added,
				},
			},
			metrics: newReconcileTestMetrics(),
		}
		provider.removeTransactionFromMempool = func(txid string) { provider.RemoveTransaction(txid) }
		return provider
	}

	t.Run("block sync removal is counted as sync_removed", func(t *testing.T) {
		provider := newProvider()
		if !provider.RemoveTransaction(testAlternativeTxID) {
			t.Fatal("RemoveTransaction() = false, want true")
		}
		if got := counterValue(t, provider.metrics.EthAlternativeMempoolEvents, "sync_removed"); got != 1 {
			t.Errorf("sync_removed events = %v, want 1", got)
		}
		if got := residenceSampleCount(t, provider.metrics.EthAlternativeMempoolTxResidence, "sync_removed"); got != 1 {
			t.Errorf("sync_removed residence samples = %d, want 1", got)
		}
		// a later reconcile off a stale snapshot must not re-count it
		provider.evictMempoolTx("mined", testAlternativeTxID, added)
		if got := counterValue(t, provider.metrics.EthAlternativeMempoolEvents, "mined"); got != 0 {
			t.Errorf("mined events = %v, want 0 (entry already removed and metered)", got)
		}
	})

	t.Run("a reconcile eviction is not also counted as sync_removed", func(t *testing.T) {
		// removeMempoolTx deletes the entry, then the delegate re-enters RemoveTransaction, which must
		// find nothing and meter nothing
		provider := newProvider()
		provider.evictMempoolTx("mined", testAlternativeTxID, added)
		if got := counterValue(t, provider.metrics.EthAlternativeMempoolEvents, "mined"); got != 1 {
			t.Errorf("mined events = %v, want 1", got)
		}
		if got := counterValue(t, provider.metrics.EthAlternativeMempoolEvents, "sync_removed"); got != 0 {
			t.Errorf("sync_removed events = %v, want 0 (reconcile exit must not be double-counted)", got)
		}
		if got := residenceSampleCount(t, provider.metrics.EthAlternativeMempoolTxResidence, "sync_removed"); got != 0 {
			t.Errorf("sync_removed residence samples = %d, want 0", got)
		}
	})
}

// TestAlternativeSendTxProviderAcceptedSlotRetiresLateFetchBack verifies the acceptance-time
// retirement survives a fetch-back that produces no cache entry: the relay ACK records the
// (sender, nonce) slot, so a slower submission for that slot is dropped on arrival even though the
// newer send left nothing cached to order it against (Blink drop mode, #1573).
func TestAlternativeSendTxProviderAcceptedSlotRetiresLateFetchBack(t *testing.T) {
	sender := ethcommon.HexToAddress("0x2222222222222222222222222222222222222222")
	// getTransactionByHash surfaces the older submission A, sender+nonce 0x1
	response := `{"jsonrpc":"2.0","id":1,"result":{"hash":"` + testAlternativeTxID + `","from":"` + sender.Hex() + `","nonce":"0x1","gas":"0x5208","value":"0x0","input":"0x","to":"0x3333333333333333333333333333333333333333"}}`

	newProvider := func(t *testing.T, slot nonceSlot, slotGen uint64) *AlternativeSendTxProvider {
		t.Helper()
		server := newAlternativeTxProviderTestServer(t, response)
		provider := &AlternativeSendTxProvider{
			urls:              []string{server.URL},
			fetchMempoolTx:    true,
			onlyAlternative:   true,
			rpcTimeout:        time.Second,
			mempoolTxsTimeout: time.Hour,
			mempoolTxs:        map[string]storedTx{},
			// a newer send was accepted for this slot but never produced a cache entry
			acceptedSlots: map[nonceSlot]acceptedSlot{slot: {gen: slotGen, time: time.Now()}},
		}
		provider.removeTransactionFromMempool = func(txid string) { provider.RemoveTransaction(txid) }
		return provider
	}

	t.Run("drops a submission older than the accepted slot", func(t *testing.T) {
		provider := newProvider(t, nonceSlot{addr: sender, nonce: 1}, 2)
		if _, err := provider.handleMempoolTransaction(testAlternativeTxID, 1); err != nil {
			t.Fatalf("handleMempoolTransaction() error = %v", err)
		}
		if _, found := provider.mempoolTxs[testAlternativeTxID]; found {
			t.Fatal("submission superseded by a newer accepted send was cached")
		}
	})

	t.Run("caches the submission that owns the accepted slot", func(t *testing.T) {
		// the slot generation is this submission's own - it must not retire itself
		provider := newProvider(t, nonceSlot{addr: sender, nonce: 1}, 2)
		if _, err := provider.handleMempoolTransaction(testAlternativeTxID, 2); err != nil {
			t.Fatalf("handleMempoolTransaction() error = %v", err)
		}
		if _, found := provider.mempoolTxs[testAlternativeTxID]; !found {
			t.Fatal("submission owning the accepted slot was not cached")
		}
	})

	t.Run("ignores an accepted slot for a different nonce", func(t *testing.T) {
		provider := newProvider(t, nonceSlot{addr: sender, nonce: 2}, 2)
		if _, err := provider.handleMempoolTransaction(testAlternativeTxID, 1); err != nil {
			t.Fatalf("handleMempoolTransaction() error = %v", err)
		}
		if _, found := provider.mempoolTxs[testAlternativeTxID]; !found {
			t.Fatal("submission for an unrelated nonce slot was dropped")
		}
	})
}

// TestAlternativeSendTxProviderSendRecordsAcceptedSlot verifies SendRawTransaction records the
// (sender, nonce) slot it filled even when the relay never surfaces the transaction.
func TestAlternativeSendTxProviderSendRecordsAcceptedSlot(t *testing.T) {
	rawTx, sender, sentTxID := signedTestTxWithHash(t) // nonce 1
	server := newMethodAwareTxProviderTestServer(t, map[string]string{
		"eth_sendRawTransaction":   `{"jsonrpc":"2.0","id":1,"result":"` + sentTxID + `"}`,
		"eth_getTransactionByHash": `{"jsonrpc":"2.0","id":1,"result":null}`,
	})
	provider := &AlternativeSendTxProvider{
		urls:              []string{server.URL},
		fetchMempoolTx:    true,
		onlyAlternative:   true,
		rpcTimeout:        time.Second,
		mempoolTxsTimeout: time.Hour,
		mempoolTxs:        map[string]storedTx{},
	}
	provider.removeTransactionFromMempool = func(txid string) { provider.RemoveTransaction(txid) }

	if _, err := provider.SendRawTransaction(rawTx); err != nil {
		t.Fatalf("SendRawTransaction() error = %v", err)
	}
	provider.waitForRefreshes()

	slot, found := provider.acceptedSlots[nonceSlot{addr: sender, nonce: 1}]
	if !found {
		t.Fatal("accepted (sender, nonce) slot was not recorded")
	}
	if slot.gen == 0 {
		t.Error("accepted slot generation = 0, want the send generation")
	}
}

// TestAlternativeSendTxProviderNotSurfacedMetered verifies that a relay-accepted send whose
// fetch-back never surfaces is counted under eth_alternative_send_not_surfaced_total - the
// observable signal that a relay does not surface what it accepted (#1638 review). The transaction
// itself is cached from its own signed bytes, so the counter no longer implies the cache is empty
// (see TestAlternativeSendTxProviderAcceptedSendCachedWithoutFetchBack).
func TestAlternativeSendTxProviderNotSurfacedMetered(t *testing.T) {
	rawTx, _, sentTxID := signedTestTxWithHash(t)
	server := newMethodAwareTxProviderTestServer(t, map[string]string{
		"eth_sendRawTransaction":   `{"jsonrpc":"2.0","id":1,"result":"` + sentTxID + `"}`,
		"eth_getTransactionByHash": `{"jsonrpc":"2.0","id":1,"result":null}`, // accepted but never surfaced
	})
	provider := &AlternativeSendTxProvider{
		urls:              []string{server.URL},
		fetchMempoolTx:    true,
		onlyAlternative:   true,
		rpcTimeout:        time.Second,
		mempoolTxsTimeout: time.Hour,
		mempoolTxs:        map[string]storedTx{},
		metrics:           newReconcileTestMetrics(),
	}
	provider.removeTransactionFromMempool = func(txid string) { provider.RemoveTransaction(txid) }

	if _, err := provider.SendRawTransaction(rawTx); err != nil {
		t.Fatalf("SendRawTransaction() error = %v", err)
	}
	provider.waitForRefreshes()

	if got := counterVecValue(t, provider.metrics.EthAlternativeSendNotSurfaced, "reason", "not_found"); got != 1 {
		t.Errorf("send_not_surfaced{reason=not_found} = %v, want 1", got)
	}
	// the accepted send is still exposed: cached from the signed bytes, under its true hash
	if len(provider.mempoolTxs) != 1 {
		t.Errorf("cache size = %d, want 1 (accepted send cached from its signed bytes)", len(provider.mempoolTxs))
	}
}

// TestAlternativeSendTxProviderSendBroadcastsConcurrently checks that the broadcast to several relay
// URLs costs the slowest single URL rather than their sum. Sequentially, one unresponsive relay held
// up the wallet's answer for its full timeout before the next URL was even tried, and a wallet that
// gives up before Blockbook answers reports a failure for a transaction that is on its way.
func TestAlternativeSendTxProviderSendBroadcastsConcurrently(t *testing.T) {
	rawTx, _, txID := signedTestTxWithHash(t)
	sendResponse := `{"jsonrpc":"2.0","id":1,"result":"` + txID + `"}`

	// Every relay holds its response until ALL of them have the broadcast in flight, so the send can
	// only complete if the broadcasts really are simultaneous. A sequential broadcast (or one capped
	// below len(urls) concurrency) never reaches the barrier and fails the test on the fallback,
	// rather than on a wall-clock margin that a partial regression could still fit inside.
	const parties = 3
	barrier := newTestBarrier(parties)
	servers := make([]*methodAwareServer, parties)
	urls := make([]string, len(servers))
	for i := range servers {
		servers[i] = newHookedMethodAwareTxProviderTestServer(t,
			map[string]string{"eth_sendRawTransaction": sendResponse},
			func(method string) {
				if method == "eth_sendRawTransaction" {
					barrier.arrive(t)
				}
			})
		urls[i] = servers[i].URL
	}
	provider := &AlternativeSendTxProvider{
		urls:              urls,
		mempoolTxsTimeout: time.Hour,
		rpcTimeout:        10 * time.Second,
	}

	if _, err := provider.SendRawTransaction(rawTx); err != nil {
		t.Fatalf("SendRawTransaction() error = %v", err)
	}

	if !barrier.reached() {
		t.Errorf("the %d broadcasts were not in flight simultaneously, want a concurrent fan-out", parties)
	}
	for i, s := range servers {
		if got := s.callCount("eth_sendRawTransaction"); got != 1 {
			t.Errorf("url %d received %d broadcasts, want 1", i, got)
		}
	}
}

// testBarrier releases every arriving caller only once `parties` of them are waiting, so a test can
// assert that N calls really overlap. The fallback timeout keeps a regression a failed assertion
// instead of a hung test.
type testBarrier struct {
	mu      sync.Mutex
	arrived int
	parties int
	open    chan struct{}
	all     bool
}

func newTestBarrier(parties int) *testBarrier {
	return &testBarrier{parties: parties, open: make(chan struct{})}
}

func (b *testBarrier) arrive(t *testing.T) {
	t.Helper()
	b.mu.Lock()
	b.arrived++
	if b.arrived == b.parties {
		b.all = true
		close(b.open)
	}
	b.mu.Unlock()
	select {
	case <-b.open:
	case <-time.After(3 * time.Second):
		// the handler runs in a different goroutine, t.Fatalf must not be called from here
		t.Error("barrier not reached: calls did not overlap")
	}
}

func (b *testBarrier) reached() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.all
}

// TestAlternativeSendTxProviderSendAggregatesResultsInURLOrder pins the outcome aggregation of the
// concurrent broadcast: any accepting URL makes the send successful, the first accepting URL in
// configuration order is the one recorded for the sender's nonce routing, and a later failure never
// turns an accepted send into an error.
func TestAlternativeSendTxProviderSendAggregatesResultsInURLOrder(t *testing.T) {
	rawTx, sender, txID := signedTestTxWithHash(t)
	sendResponse := `{"jsonrpc":"2.0","id":1,"result":"` + txID + `"}`
	rejection := `{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"replacement transaction underpriced"}}`

	tests := []struct {
		name        string
		responses   []string
		wantAccepts int // index of the url expected to be recorded as the accepting one
	}{
		{name: "first url accepts", responses: []string{sendResponse, rejection}, wantAccepts: 0},
		{name: "second url accepts", responses: []string{rejection, sendResponse}, wantAccepts: 1},
		{name: "both accept", responses: []string{sendResponse, sendResponse}, wantAccepts: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			urls := make([]string, len(tt.responses))
			for i, response := range tt.responses {
				urls[i] = newMethodAwareTxProviderTestServer(t, map[string]string{"eth_sendRawTransaction": response}).URL
			}
			provider := &AlternativeSendTxProvider{urls: urls, mempoolTxsTimeout: time.Hour, rpcTimeout: time.Second}

			got, err := provider.SendRawTransaction(rawTx)
			if err != nil {
				t.Fatalf("SendRawTransaction() error = %v", err)
			}
			provider.waitForRefreshes()
			if got != txID {
				t.Errorf("txid = %q, want %q", got, txID)
			}
			if s := provider.recentSenders[sender]; s.url != urls[tt.wantAccepts] {
				t.Errorf("recorded accepting url = %q, want url %d (%q)", s.url, tt.wantAccepts, urls[tt.wantAccepts])
			}
		})
	}

	t.Run("unresponsive url does not fail or stall the send", func(t *testing.T) {
		// The case the concurrent fan-out exists for: sequentially, each unresponsive relay burned its
		// full rpcTimeout before the next URL was even tried, so two of them pushed the answer past
		// any wallet deadline. Cleanup order matters - the hanging handlers must be released before
		// httptest.Server.Close, which waits for outstanding requests (t.Cleanup runs LIFO).
		hang := make(chan struct{})
		hold := func(method string) { <-hang }
		urls := []string{
			newHookedMethodAwareTxProviderTestServer(t, nil, hold).URL,
			newHookedMethodAwareTxProviderTestServer(t, nil, hold).URL,
			newMethodAwareTxProviderTestServer(t, map[string]string{"eth_sendRawTransaction": sendResponse}).URL,
		}
		t.Cleanup(func() { close(hang) })
		const rpcTimeout = 400 * time.Millisecond
		provider := &AlternativeSendTxProvider{urls: urls, mempoolTxsTimeout: time.Hour, rpcTimeout: rpcTimeout}

		start := time.Now()
		got, err := provider.SendRawTransaction(rawTx)
		elapsed := time.Since(start)

		if err != nil {
			t.Fatalf("SendRawTransaction() error = %v, want the accepting url to decide the outcome", err)
		}
		if got != txID {
			t.Errorf("txid = %q, want %q", got, txID)
		}
		if s := provider.recentSenders[sender]; s.url != urls[2] {
			t.Errorf("recorded accepting url = %q, want %q", s.url, urls[2])
		}
		// two unresponsive urls cost 2*rpcTimeout sequentially and one rpcTimeout concurrently
		if elapsed >= 2*rpcTimeout {
			t.Errorf("send took %s with 2 unresponsive urls, want about one rpcTimeout (%s)", elapsed, rpcTimeout)
		}
	})

	t.Run("no url accepts", func(t *testing.T) {
		urls := []string{
			newMethodAwareTxProviderTestServer(t, map[string]string{"eth_sendRawTransaction": rejection}).URL,
			newMethodAwareTxProviderTestServer(t, map[string]string{"eth_sendRawTransaction": rejection}).URL,
		}
		provider := &AlternativeSendTxProvider{urls: urls, mempoolTxsTimeout: time.Hour, rpcTimeout: time.Second}

		if _, err := provider.SendRawTransaction(rawTx); err == nil {
			t.Fatal("SendRawTransaction() error = nil, want the relay rejection")
		}
		provider.waitForRefreshes()
		if len(provider.recentSenders) != 0 {
			t.Errorf("recentSenders has %d entries, want 0 when no url accepted", len(provider.recentSenders))
		}
	})
}

// newBlockedFetchBackProvider wires a provider whose relay accepts the send immediately but holds its
// eth_getTransactionByHash response until the returned release channel is closed, so a test can make
// assertions while the fetch-back is provably still in flight. The surfaced body differs from the one
// derived from the signed bytes by transactionIndex, an artificial marker: a real pending transaction
// has none, which is exactly why it identifies the relay's view unambiguously.
func newBlockedFetchBackProvider(t *testing.T, rawTx string, sender ethcommon.Address, txID string) (*AlternativeSendTxProvider, chan struct{}, *methodAwareServer) {
	t.Helper()
	return newBlockedFetchBackProviderWithBody(t, rawTx, txID,
		`{"jsonrpc":"2.0","id":1,"result":{"hash":"`+txID+`","from":"`+sender.Hex()+`","nonce":"0x1","gas":"0x5208","value":"0x0","input":"0x","to":"0x3333333333333333333333333333333333333333","transactionIndex":"0x7"}}`)
}

func newBlockedFetchBackProviderWithBody(t *testing.T, rawTx, txID, fetchBackResponse string) (*AlternativeSendTxProvider, chan struct{}, *methodAwareServer) {
	t.Helper()
	release := make(chan struct{})
	server := newHookedMethodAwareTxProviderTestServer(t,
		map[string]string{
			"eth_sendRawTransaction":   `{"jsonrpc":"2.0","id":1,"result":"` + txID + `"}`,
			"eth_getTransactionByHash": fetchBackResponse,
		},
		func(method string) {
			if method == "eth_getTransactionByHash" {
				<-release
			}
		})
	provider := &AlternativeSendTxProvider{
		urls:              []string{server.URL},
		fetchMempoolTx:    true,
		onlyAlternative:   true,
		rpcTimeout:        10 * time.Second,
		mempoolTxsTimeout: time.Hour,
		mempoolTxs:        map[string]storedTx{},
		metrics:           newReconcileTestMetrics(),
	}
	provider.removeTransactionFromMempool = func(txid string) { provider.RemoveTransaction(txid) }
	return provider, release, server
}

// sendWhileFetchBackBlocked runs the send and returns once it has answered, failing the test if it
// waits for the blocked fetch-back.
func sendWhileFetchBackBlocked(t *testing.T, provider *AlternativeSendTxProvider, rawTx string) string {
	t.Helper()
	var txid string
	var err error
	done := make(chan struct{})
	go func() {
		defer close(done)
		txid, err = provider.SendRawTransaction(rawTx)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("SendRawTransaction() did not answer while the fetch-back was still in flight")
	}
	if err != nil {
		t.Fatalf("SendRawTransaction() error = %v", err)
	}
	return txid
}

// TestAlternativeSendTxProviderSendDoesNotWaitForFetchBack checks that the post-send fetch-back does
// not delay the answer to the wallet: the transaction is already cached from its signed bytes when the
// send returns, and the fetch-back only replaces that body with the relay's own view of it.
func TestAlternativeSendTxProviderSendDoesNotWaitForFetchBack(t *testing.T) {
	rawTx, sender, txID := signedTestTxWithHash(t)
	provider, release, server := newBlockedFetchBackProvider(t, rawTx, sender, txID)

	sendWhileFetchBackBlocked(t, provider, rawTx)

	// the transaction is already exposed as pending, from the signed bytes
	cached, found := provider.GetTransaction(txID)
	if !found {
		t.Fatal("accepted transaction is not served as pending before the fetch-back completes")
	}
	if cached.TransactionIndex != "" {
		t.Error("cached body came from the relay, want the one derived from the signed bytes")
	}

	close(release)
	provider.waitForRefreshes()
	if got := server.callCount("eth_getTransactionByHash"); got != 1 {
		t.Errorf("eth_getTransactionByHash calls = %d, want exactly 1 (in the background)", got)
	}
	// and it stays the derived body afterwards: the fetch-back only probes, so the relay's view - which
	// could carry a different hash, `to` or `value` - never becomes what Blockbook serves
	after, found := provider.GetTransaction(txID)
	if !found {
		t.Fatal("transaction left the cache after the fetch-back")
	}
	if after.TransactionIndex != "" {
		t.Errorf("the fetch-back adopted the relay's body: transactionIndex = %q, want it untouched", after.TransactionIndex)
	}
}

// TestAlternativeSendTxProviderFetchBackDoesNotResurrectRemoval is why the fetch-back never writes the
// cache. It answers up to rpcTimeout per URL after the wallet was told the send succeeded, so the
// transaction may have mined and block sync may have cleared it by then - and a relay is free to keep
// reporting it as pending. Re-inserting it would flip a confirmed transaction back to Unconfirmed.
func TestAlternativeSendTxProviderFetchBackDoesNotResurrectRemoval(t *testing.T) {
	rawTx, sender, txID := signedTestTxWithHash(t)
	provider, release, _ := newBlockedFetchBackProvider(t, rawTx, sender, txID)

	sendWhileFetchBackBlocked(t, provider, rawTx)

	// block sync indexing the mined transaction, while the fetch-back is still in flight
	if !provider.RemoveTransaction(txID) {
		t.Fatal("transaction was not cached by the send")
	}

	close(release)
	provider.waitForRefreshes()

	if _, found := provider.mempoolTxs[txID]; found {
		t.Fatal("the fetch-back re-inserted a transaction that had already left the cache")
	}
	if floor, stranded := provider.raiseToPendingFloor(sender, 1); floor != 1 || stranded {
		t.Errorf("raiseToPendingFloor(1) = (%d, %v), want the backend answer (1, false) after the entry was removed", floor, stranded)
	}
}

// TestAlternativeSendTxProviderFetchBackKeepsDerivedBody covers the body a relay disagrees about: it
// must stay the one derived from the signed bytes, which is what keeps the entry visible to
// pendingNonceFloor, to releaseRecentSender and to the nonce-superseded reconcile check.
func TestAlternativeSendTxProviderFetchBackKeepsDerivedBody(t *testing.T) {
	rawTx, sender, txID := signedTestTxWithHash(t)
	// surfaced without `from`, so it identifies no nonce slot at all
	provider, release, _ := newBlockedFetchBackProviderWithBody(t, rawTx, txID,
		`{"jsonrpc":"2.0","id":1,"result":{"hash":"`+txID+`","nonce":"0x1","gas":"0x5208","value":"0x0","input":"0x","to":"0x3333333333333333333333333333333333333333"}}`)

	sendWhileFetchBackBlocked(t, provider, rawTx)
	close(release)
	provider.waitForRefreshes()

	cached, found := provider.GetTransaction(txID)
	if !found {
		t.Fatal("transaction left the cache after the fetch-back")
	}
	if cached.From != strings.ToLower(sender.Hex()) {
		t.Errorf("cached From = %q, want the derived %q kept", cached.From, strings.ToLower(sender.Hex()))
	}
	if floor, stranded := provider.raiseToPendingFloor(sender, 1); floor != 2 || stranded {
		t.Errorf("raiseToPendingFloor(1) = (%d, %v), want (2, false)", floor, stranded)
	}
}

// TestAlternativeSendTxProviderFetchBackKeepsMinedTransactionForBlockSync pins that the fetch-back does
// not evict when the relay already reports the transaction mined: the relay can see the block before
// Blockbook indexes it, and eviction clears the address index too, so the transaction would briefly be
// in neither store. Block sync removes it as sync_removed instead.
func TestAlternativeSendTxProviderFetchBackKeepsMinedTransactionForBlockSync(t *testing.T) {
	rawTx, sender, txID := signedTestTxWithHash(t)
	provider, release, _ := newBlockedFetchBackProviderWithBody(t, rawTx, txID,
		`{"jsonrpc":"2.0","id":1,"result":{"hash":"`+txID+`","from":"`+sender.Hex()+`","nonce":"0x1","gas":"0x5208","value":"0x0","input":"0x","to":"0x3333333333333333333333333333333333333333","blockNumber":"0x10"}}`)

	sendWhileFetchBackBlocked(t, provider, rawTx)
	close(release)
	provider.waitForRefreshes()

	if _, found := provider.mempoolTxs[txID]; !found {
		t.Fatal("the fetch-back evicted a mined transaction before block sync could index its block")
	}
	if got := counterValue(t, provider.metrics.EthAlternativeMempoolEvents, "mined"); got != 0 {
		t.Errorf("mined events = %v, want 0 from the fetch-back", got)
	}
}

// TestAlternativeSendTxProviderUndecodableSendUsesFetchBack covers the fallback: when the accepted raw
// hex does not decode, nothing can be derived from it, so the fetch-back keeps its create semantics
// and the relay's echoed txid is the only key available.
func TestAlternativeSendTxProviderUndecodableSendUsesFetchBack(t *testing.T) {
	server := newMethodAwareTxProviderTestServer(t, map[string]string{
		"eth_sendRawTransaction":   `{"jsonrpc":"2.0","id":1,"result":"` + testAlternativeTxID + `"}`,
		"eth_getTransactionByHash": testAlternativeKnownTxResponse,
	})
	provider := &AlternativeSendTxProvider{
		urls:              []string{server.URL},
		fetchMempoolTx:    true,
		onlyAlternative:   true,
		rpcTimeout:        time.Second,
		mempoolTxsTimeout: time.Hour,
		mempoolTxs:        map[string]storedTx{},
		metrics:           newReconcileTestMetrics(),
	}

	txid, err := provider.SendRawTransaction("0xdeadbeef")
	if err != nil {
		t.Fatalf("SendRawTransaction() error = %v", err)
	}
	provider.waitForRefreshes()

	if txid != testAlternativeTxID {
		t.Errorf("txid = %q, want the relay echo %q when the raw hex does not decode", txid, testAlternativeTxID)
	}
	cached, found := provider.mempoolTxs[testAlternativeTxID]
	if !found {
		t.Fatal("the fetch-back did not cache the transaction when the raw hex could not be decoded")
	}
	if cached.gen != 0 {
		t.Errorf("cached gen = %d, want 0 (the send order is unknown)", cached.gen)
	}
	if len(provider.recentSenders) != 0 {
		t.Errorf("recentSenders has %d entries, want 0 after an undecodable raw tx", len(provider.recentSenders))
	}
}

// TestAlternativeSendTxProviderDroppedFetchBackMetered covers the one drop that loses a transaction: on
// the raw-hex-decode-failure path the fetch-back is the only thing that can expose the send, so a
// refusal leaves it served nowhere, indexed nowhere and raising no pending-nonce floor - the
// nonce-reuse precursor, and the only variant of it the dashboard could not see.
func TestAlternativeSendTxProviderDroppedFetchBackMetered(t *testing.T) {
	server := newMethodAwareTxProviderTestServer(t, map[string]string{
		"eth_sendRawTransaction":   `{"jsonrpc":"2.0","id":1,"result":"` + testAlternativeTxID + `"}`,
		"eth_getTransactionByHash": testAlternativeKnownTxResponse,
	})
	provider := &AlternativeSendTxProvider{
		urls:              []string{server.URL},
		fetchMempoolTx:    true,
		onlyAlternative:   true,
		rpcTimeout:        time.Second,
		mempoolTxsTimeout: time.Hour,
		mempoolTxs:        map[string]storedTx{},
		metrics:           newReconcileTestMetrics(),
		// every slot of the expose allowance taken, so the fetch-back below is refused
		exposeCount: maxExposeFetchBacks,
	}

	// undecodable raw hex: nothing can be derived from it, so the refused fetch-back loses the send
	if _, err := provider.SendRawTransaction("0xdeadbeef"); err != nil {
		t.Fatalf("SendRawTransaction() error = %v", err)
	}
	// no waitForRefreshes: the slots are held by hand, so the counter never drains

	if got := labeledCounterValue(t, provider.metrics.EthAlternativeSendNotSurfaced, "reason", "dropped"); got != 1 {
		t.Errorf("send_not_surfaced{reason=dropped} = %v, want 1", got)
	}
	if got := server.callCount("eth_getTransactionByHash"); got != 0 {
		t.Errorf("eth_getTransactionByHash calls = %d, want 0 (the fetch-back was refused)", got)
	}
	if len(provider.mempoolTxs) != 0 {
		t.Errorf("cache size = %d, want 0 (nothing could be derived or fetched)", len(provider.mempoolTxs))
	}
}

// TestAlternativeSendTxProviderDroppedRefreshNotMetered is the other half: a refused refresh must NOT
// be reported as a not-surfaced send. The transaction is cached and indexed from its signed bytes, so
// counting it would raise a false alarm on the metric documented as the nonce-reuse precursor.
func TestAlternativeSendTxProviderDroppedRefreshNotMetered(t *testing.T) {
	rawTx, sender, txID := signedTestTxWithHash(t)
	server := newMethodAwareTxProviderTestServer(t, map[string]string{
		"eth_sendRawTransaction": `{"jsonrpc":"2.0","id":1,"result":"` + txID + `"}`,
	})
	provider := &AlternativeSendTxProvider{
		urls:              []string{server.URL},
		fetchMempoolTx:    true,
		onlyAlternative:   true,
		rpcTimeout:        time.Second,
		mempoolTxsTimeout: time.Hour,
		mempoolTxs:        map[string]storedTx{},
		metrics:           newReconcileTestMetrics(),
		backgroundCount:   maxBackgroundFetchBacks,
	}

	if _, err := provider.SendRawTransaction(rawTx); err != nil {
		t.Fatalf("SendRawTransaction() error = %v", err)
	}
	// no waitForRefreshes: the slots are held by hand, so the counter never drains

	if got := labeledCounterValue(t, provider.metrics.EthAlternativeSendNotSurfaced, "reason", "dropped"); got != 0 {
		t.Errorf("send_not_surfaced{reason=dropped} = %v, want 0 for a refused refresh", got)
	}
	// the send is exposed regardless of the refresh, which is why the drop is not worth reporting
	if _, found := provider.GetTransaction(txID); !found {
		t.Error("accepted send is not served as pending")
	}
	if floor, stranded := provider.raiseToPendingFloor(sender, 1); floor != 2 || stranded {
		t.Errorf("raiseToPendingFloor(1) = (%d, %v), want (2, false)", floor, stranded)
	}
}

// TestAlternativeSendTxProviderShutdownDropNotMetered keeps a fetch-back declined at shutdown out of
// the counter: the process is going down, and an alert on the nonce-reuse precursor must not fire on
// every restart that catches a send in flight.
func TestAlternativeSendTxProviderShutdownDropNotMetered(t *testing.T) {
	server := newMethodAwareTxProviderTestServer(t, map[string]string{
		"eth_sendRawTransaction": `{"jsonrpc":"2.0","id":1,"result":"` + testAlternativeTxID + `"}`,
	})
	provider := &AlternativeSendTxProvider{
		urls:              []string{server.URL},
		fetchMempoolTx:    true,
		onlyAlternative:   true,
		rpcTimeout:        time.Second,
		mempoolTxsTimeout: time.Hour,
		mempoolTxs:        map[string]storedTx{},
		metrics:           newReconcileTestMetrics(),
		stop:              make(chan struct{}),
	}
	provider.shutdown()

	if _, err := provider.SendRawTransaction("0xdeadbeef"); err != nil {
		t.Fatalf("SendRawTransaction() error = %v", err)
	}
	provider.waitForRefreshes()

	if got := labeledCounterValue(t, provider.metrics.EthAlternativeSendNotSurfaced, "reason", "dropped"); got != 0 {
		t.Errorf("send_not_surfaced{reason=dropped} = %v, want 0 at shutdown", got)
	}
}

// TestAlternativeSendTxProviderNotSurfacedErrorMetered covers the error label of
// eth_alternative_send_not_surfaced_total, and that a relay failing the fetch-back leaves the
// transaction cached from its signed bytes all the same.
func TestAlternativeSendTxProviderNotSurfacedErrorMetered(t *testing.T) {
	rawTx, sender, txID := signedTestTxWithHash(t)
	server := newMethodAwareTxProviderTestServer(t, map[string]string{
		"eth_sendRawTransaction":   `{"jsonrpc":"2.0","id":1,"result":"` + txID + `"}`,
		"eth_getTransactionByHash": `{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"rate limited"}}`,
	})
	provider := &AlternativeSendTxProvider{
		urls:              []string{server.URL},
		fetchMempoolTx:    true,
		onlyAlternative:   true,
		rpcTimeout:        time.Second,
		mempoolTxsTimeout: time.Hour,
		mempoolTxs:        map[string]storedTx{},
		metrics:           newReconcileTestMetrics(),
	}

	if _, err := provider.SendRawTransaction(rawTx); err != nil {
		t.Fatalf("SendRawTransaction() error = %v", err)
	}
	provider.waitForRefreshes()

	if got := counterVecValue(t, provider.metrics.EthAlternativeSendNotSurfaced, "reason", "error"); got != 1 {
		t.Errorf("send_not_surfaced{reason=error} = %v, want 1", got)
	}
	if _, found := provider.GetTransaction(txID); !found {
		t.Error("transaction is not served as pending after a failed fetch-back")
	}
	if floor, stranded := provider.raiseToPendingFloor(sender, 1); floor != 2 || stranded {
		t.Errorf("raiseToPendingFloor(1) = (%d, %v), want (2, false)", floor, stranded)
	}
}

// TestAlternativeSendTxProviderSendKeysOnSignedBytesHash pins the decision that the txid comes from
// the signed bytes rather than the relay's echo: the echo is what a wallet would poll and what its own
// optimistic pending entry is keyed on, so a relay echoing anything else must not decide where the
// transaction lives.
func TestAlternativeSendTxProviderSendKeysOnSignedBytesHash(t *testing.T) {
	rawTx, sender, txID := signedTestTxWithHash(t)
	server := newMethodAwareTxProviderTestServer(t, map[string]string{
		// the relay echoes a different hash than the bytes it was given
		"eth_sendRawTransaction":   `{"jsonrpc":"2.0","id":1,"result":"` + testAlternativeSecondTxID + `"}`,
		"eth_getTransactionByHash": `{"jsonrpc":"2.0","id":1,"result":null}`,
	})
	provider := &AlternativeSendTxProvider{
		urls:              []string{server.URL},
		fetchMempoolTx:    true,
		onlyAlternative:   true,
		rpcTimeout:        time.Second,
		mempoolTxsTimeout: time.Hour,
		mempoolTxs:        map[string]storedTx{},
		metrics:           newReconcileTestMetrics(),
	}

	got, err := provider.SendRawTransaction(rawTx)
	if err != nil {
		t.Fatalf("SendRawTransaction() error = %v", err)
	}
	provider.waitForRefreshes()

	if got != txID {
		t.Errorf("returned txid = %q, want the signed bytes hash %q", got, txID)
	}
	if _, found := provider.mempoolTxs[txID]; !found {
		t.Error("transaction was not cached under the hash of its signed bytes")
	}
	if _, found := provider.mempoolTxs[testAlternativeSecondTxID]; found {
		t.Error("transaction was cached under the relay's echoed hash")
	}
	if floor, stranded := provider.raiseToPendingFloor(sender, 1); floor != 2 || stranded {
		t.Errorf("raiseToPendingFloor(1) = (%d, %v), want (2, false)", floor, stranded)
	}
}

// TestAlternativeSendTxProviderOldestAgeGauge verifies setMempoolOldestAge reports the age of the
// oldest cached entry and zeroes on an empty cache - the live stuck-tx signal (#1638 review).
func TestAlternativeSendTxProviderOldestAgeGauge(t *testing.T) {
	provider := &AlternativeSendTxProvider{fetchMempoolTx: true, metrics: newReconcileTestMetrics()}

	provider.setMempoolOldestAge(uint32(time.Now().Add(-120 * time.Second).Unix()))
	if got := gaugeValue(t, provider.metrics.EthAlternativeMempoolOldestAge); got < 115 || got > 125 {
		t.Errorf("oldest age gauge = %v, want ~120", got)
	}

	provider.setMempoolOldestAge(0) // empty cache
	if got := gaugeValue(t, provider.metrics.EthAlternativeMempoolOldestAge); got != 0 {
		t.Errorf("oldest age gauge on empty cache = %v, want 0", got)
	}
}

// TestAlternativeSendTxProviderHandleMempoolTransactionUnorderedFetchBackSkipsItself verifies a
// fetch-back of unknown send order (gen 0) skips itself when a generation-carrying entry holds the
// nonce slot. evictReplacedByNonce protects that entry from a gen-0 keeper, so an asymmetric check
// here left BOTH transactions cached for one slot.
func TestAlternativeSendTxProviderHandleMempoolTransactionUnorderedFetchBackSkipsItself(t *testing.T) {
	sender := ethcommon.HexToAddress("0x2222222222222222222222222222222222222222")
	// getTransactionByHash surfaces A (testAlternativeTxID), same sender+nonce as the cached gen-2 B
	server := newAlternativeTxProviderTestServer(t, `{"jsonrpc":"2.0","id":1,"result":{"hash":"`+testAlternativeTxID+`","from":"`+sender.Hex()+`","nonce":"0x1","gas":"0x5208","value":"0x0","input":"0x","to":"0x3333333333333333333333333333333333333333"}}`)
	provider := &AlternativeSendTxProvider{
		urls:              []string{server.URL},
		fetchMempoolTx:    true,
		onlyAlternative:   true,
		rpcTimeout:        time.Second,
		mempoolTxsTimeout: time.Hour,
		mempoolTxs: map[string]storedTx{
			testAlternativeSecondTxID: {
				tx:   &bchain.RpcTransaction{Hash: testAlternativeSecondTxID, From: sender.Hex(), AccountNonce: "0x1"},
				time: uint32(time.Now().Unix()),
				gen:  2,
			},
		},
	}
	provider.removeTransactionFromMempool = func(txid string) { provider.RemoveTransaction(txid) }

	// gen 0 - this submission's send order is unknown
	if _, err := provider.handleMempoolTransaction(testAlternativeTxID, 0); err != nil {
		t.Fatalf("handleMempoolTransaction() error = %v", err)
	}

	if _, found := provider.mempoolTxs[testAlternativeTxID]; found {
		t.Fatal("unordered fetch-back was cached alongside a generation-carrying entry for the same nonce slot")
	}
	if _, found := provider.mempoolTxs[testAlternativeSecondTxID]; !found {
		t.Fatal("generation-carrying entry was evicted by an unordered fetch-back")
	}
	if len(provider.mempoolTxs) != 1 {
		t.Fatalf("cached entries for one nonce slot = %d, want 1", len(provider.mempoolTxs))
	}
}

// TestAlternativeSendTxProviderGetTransactionReturnsCopy pins that the cached body never leaves the
// provider: its only caller hands the result to EthTxToTx with fixEIP55=true, which rewrites From and
// To in place holding no lock. Deterministic on purpose - mutating the result and finding the cache
// unchanged proves ownership without depending on the race detector catching a schedule.
func TestAlternativeSendTxProviderGetTransactionReturnsCopy(t *testing.T) {
	const from = "0x2222222222222222222222222222222222222222"
	provider := &AlternativeSendTxProvider{
		fetchMempoolTx:    true,
		mempoolTxsTimeout: time.Hour,
		mempoolTxs: map[string]storedTx{
			testAlternativeTxID: {
				tx:   &bchain.RpcTransaction{Hash: testAlternativeTxID, From: from, To: "0x3333333333333333333333333333333333333333", AccountNonce: "0x1"},
				time: uint32(time.Now().Unix()),
			},
		},
	}

	body, found := provider.GetTransaction(testAlternativeTxID)
	if !found {
		t.Fatal("cached tx not found")
	}
	if body == provider.mempoolTxs[testAlternativeTxID].tx {
		t.Fatal("GetTransaction handed out the cached body itself")
	}

	// stands in for EthTxToTx's in-place EIP-55 rewrite of From/To
	body.From = strings.ToUpper(from)
	body.To = ""

	cached := provider.mempoolTxs[testAlternativeTxID].tx
	if cached.From != from {
		t.Errorf("cached From = %q, want the untouched %q", cached.From, from)
	}
	if cached.To == "" {
		t.Error("caller cleared the cached To")
	}
}

// TestAlternativeSendTxProviderGetTransactionNilBody covers the entry whose body is nil: every other
// reader of the cache guards against it, so the read path must too rather than dereference it.
func TestAlternativeSendTxProviderGetTransactionNilBody(t *testing.T) {
	provider := &AlternativeSendTxProvider{
		fetchMempoolTx:    true,
		mempoolTxsTimeout: time.Hour,
		mempoolTxs:        map[string]storedTx{testAlternativeTxID: {time: uint32(time.Now().Unix())}},
	}

	if tx, found := provider.GetTransaction(testAlternativeTxID); found || tx != nil {
		t.Fatalf("GetTransaction() = (%v, %v), want (nil, false) for an entry with no body", tx, found)
	}
}

// TestAlternativeSendTxProviderGetTransactionTimeoutCleansWrappedMempool verifies the read-path
// staleness eviction routes through the removeTransactionFromMempool delegate (which clears the
// wrapped Blockbook mempool's address index), not the cache-only RemoveTransaction. Before the fix
// an expired private tx lingered in the wrapped mempool whenever the caller's own primary-RPC
// lookup errored instead of returning null (finding #3).
func TestAlternativeSendTxProviderGetTransactionTimeoutCleansWrappedMempool(t *testing.T) {
	var removed string
	provider := &AlternativeSendTxProvider{
		fetchMempoolTx:    true,
		mempoolTxsTimeout: time.Minute,
		mempoolTxs: map[string]storedTx{
			testAlternativeTxID: {
				tx:   &bchain.RpcTransaction{Hash: testAlternativeTxID, From: "0x2222222222222222222222222222222222222222", AccountNonce: "0x1"},
				time: uint32(time.Now().Add(-2 * time.Minute).Unix()), // already past mempoolTxsTimeout
			},
		},
		metrics: newReconcileTestMetrics(),
	}
	// stands in for EthereumRPC.removeTransactionFromMempool, which clears b.Mempool too
	provider.removeTransactionFromMempool = func(txid string) { removed = txid; provider.RemoveTransaction(txid) }

	if _, found := provider.GetTransaction(testAlternativeTxID); found {
		t.Fatal("expired tx returned as found")
	}
	if removed != testAlternativeTxID {
		t.Fatalf("removeTransactionFromMempool delegate not invoked on read-path timeout; removed=%q, want %q", removed, testAlternativeTxID)
	}
	if _, found := provider.mempoolTxs[testAlternativeTxID]; found {
		t.Fatal("expired tx remained in provider cache")
	}
}
