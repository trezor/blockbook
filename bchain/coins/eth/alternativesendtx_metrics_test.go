package eth

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
)

func newSendTxMetrics() *common.Metrics {
	return &common.Metrics{
		EthAlternativeSendTx: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "test_alt_sendtx_total"}, []string{"provider", "result", "reason"}),
		EthAlternativeSendTxDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{Name: "test_alt_sendtx_duration_seconds", Buckets: []float64{1}}, []string{"provider", "result"}),
		EthAlternativeMempoolEvents: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "test_alt_mempool_events_total"}, []string{"action"}),
	}
}

// gatherMetric returns the single sample of c whose labels match want, or nil when that series was
// never touched - an absent series and a zero-valued one are different assertions here.
func gatherMetric(t *testing.T, c prometheus.Collector, want map[string]string) *dto.Metric {
	t.Helper()
	reg := prometheus.NewRegistry()
	if err := reg.Register(c); err != nil {
		t.Fatalf("register collector: %v", err)
	}
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, mf := range families {
		for _, mm := range mf.GetMetric() {
			labels := make(map[string]string, len(mm.GetLabel()))
			for _, lp := range mm.GetLabel() {
				labels[lp.GetName()] = lp.GetValue()
			}
			match := true
			for k, v := range want {
				if labels[k] != v {
					match = false
					break
				}
			}
			if match {
				return mm
			}
		}
	}
	return nil
}

// TestAlternativeSendTxProviderSendObservesProvider asserts a broadcast attempt is counted and
// timed per provider. A send succeeds if any provider accepts it, so a provider that is down shows
// up here and nowhere else.
func TestAlternativeSendTxProviderSendObservesProvider(t *testing.T) {
	rawTx, _ := signedTestTx(t)
	m := newSendTxMetrics()
	provider := &AlternativeSendTxProvider{
		urls:              []string{"http://127.0.0.1:1"},
		mempoolTxsTimeout: time.Hour,
		rpcTimeout:        time.Second,
		metrics:           m,
	}

	if _, err := provider.SendRawTransaction(rawTx); err == nil {
		t.Fatal("expected error from unreachable provider")
	}

	// the reason label is not pinned here - the dial error text is platform dependent, and the
	// mapping from message to class is asserted in bchain.TestClassifySendTxError
	failed := gatherMetric(t, m.EthAlternativeSendTx, map[string]string{"provider": "127.0.0.1:1", "result": "error"})
	if failed == nil || failed.GetCounter().GetValue() != 1 {
		t.Errorf("unreachable provider counter = %v, want 1", failed)
	}
	timed := gatherMetric(t, m.EthAlternativeSendTxDuration, map[string]string{"provider": "127.0.0.1:1", "result": "error"})
	if timed == nil || timed.GetHistogram().GetSampleCount() != 1 {
		t.Errorf("unreachable provider duration samples = %v, want 1 - a timing out provider is exactly what the histogram must show", timed)
	}
}

// TestAlternativeSendTxProviderObserveSendTxAccepted asserts an accepted broadcast is labeled
// success/ok under the provider's host rather than its full URL, which carries the API key.
func TestAlternativeSendTxProviderObserveSendTxAccepted(t *testing.T) {
	m := newSendTxMetrics()
	provider := &AlternativeSendTxProvider{metrics: m}

	provider.observeSendTx("https://relay.example.com/v1/SECRETKEY", 250*time.Millisecond, nil)

	accepted := gatherMetric(t, m.EthAlternativeSendTx, map[string]string{"provider": "relay.example.com", "result": "success", "reason": bchain.ReasonOK})
	if accepted == nil || accepted.GetCounter().GetValue() != 1 {
		t.Errorf("accepted send counter = %v, want 1", accepted)
	}
	timed := gatherMetric(t, m.EthAlternativeSendTxDuration, map[string]string{"provider": "relay.example.com", "result": "success"})
	if timed == nil || timed.GetHistogram().GetSampleCount() != 1 {
		t.Errorf("accepted send duration samples = %v, want 1", timed)
	}
}

// TestAlternativeSendTxProviderFetchBackObservesError asserts a failed post-send fetch-back is
// counted: it leaves the just-broadcast transaction out of the cache, so the wallet that sent it
// sees no pending transaction at all - previously visible only as a log line.
func TestAlternativeSendTxProviderFetchBackObservesError(t *testing.T) {
	m := newSendTxMetrics()
	provider := &AlternativeSendTxProvider{
		urls:              []string{"http://127.0.0.1:1"},
		fetchMempoolTx:    true,
		mempoolTxs:        make(map[string]storedTx),
		mempoolTxsTimeout: time.Hour,
		rpcTimeout:        time.Second,
		metrics:           m,
	}

	if _, err := provider.handleMempoolTransaction(testAlternativeTxID, 1); err == nil {
		t.Fatal("expected error from unreachable provider")
	}

	failed := gatherMetric(t, m.EthAlternativeMempoolEvents, map[string]string{"action": "fetchback_error"})
	if failed == nil || failed.GetCounter().GetValue() != 1 {
		t.Errorf("fetchback_error counter = %v, want 1", failed)
	}
}

// TestProviderLabel asserts a provider's metric label and log identity carries no secret -
// configured provider URLs commonly embed an API key in the path or query.
func TestProviderLabel(t *testing.T) {
	tests := []struct{ url, want string }{
		{"https://relay.example.com/v1/SECRETKEY", "relay.example.com"},
		{"https://relay.example.com:8545/?apikey=SECRETKEY", "relay.example.com:8545"},
		{"not a url at all", "unknown"},
	}
	for _, tt := range tests {
		if got := providerLabel(tt.url); got != tt.want {
			t.Errorf("providerLabel(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

// TestSendRawTransactionErrorCarriesNoAPIKey reproduces the leak this pair of helpers exists to
// close: the provider URL embeds an API key, the dial fails, and the resulting *url.Error message
// is both logged and handed back to the API client. The key must survive in neither.
func TestSendRawTransactionErrorCarriesNoAPIKey(t *testing.T) {
	const apiKey = "SECRET_API_KEY_ABCDEF"
	rawTx, _ := signedTestTx(t)
	provider := &AlternativeSendTxProvider{
		urls:              []string{"http://127.0.0.1:1/v3/" + apiKey},
		mempoolTxsTimeout: time.Hour,
		rpcTimeout:        time.Second,
	}

	_, err := provider.SendRawTransaction(rawTx)
	if err == nil {
		t.Fatal("expected error from unreachable provider")
	}
	if strings.Contains(err.Error(), apiKey) {
		t.Errorf("error returned to the caller leaks the api key: %s", err)
	}
	if got := scrubProviderURLs(err.Error()); strings.Contains(got, apiKey) {
		t.Errorf("scrubbed message still leaks the api key: %s", got)
	}
}

// TestScrubProviderURLs covers the message shapes an API key actually arrives in.
func TestScrubProviderURLs(t *testing.T) {
	tests := []struct{ in, want string }{
		{`Post "http://127.0.0.1:1/v3/SECRET": dial tcp 127.0.0.1:1: connect: connection refused`, `Post "127.0.0.1:1": dial tcp 127.0.0.1:1: connect: connection refused`},
		{"https://relay.example.com/rpc?apikey=SECRET eth_sendRawTransaction : failed", "relay.example.com eth_sendRawTransaction : failed"},
		// a plain JSON-RPC rejection carries no url and must pass through untouched, so error
		// classification keeps seeing the backend's own wording
		{"nonce too low: next nonce 5, tx nonce 3", "nonce too low: next nonce 5, tx nonce 3"},
	}
	for _, tt := range tests {
		if got := scrubProviderURLs(tt.in); got != tt.want {
			t.Errorf("scrubProviderURLs(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
