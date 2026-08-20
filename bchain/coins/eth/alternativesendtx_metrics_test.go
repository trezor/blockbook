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

// newSendTxMetrics extends the reconcile collectors with the send-path ones, so the mempool event
// counter has a single owner - the fetch-back is observed on it from the send path too.
func newSendTxMetrics() *common.Metrics {
	m := newReconcileTestMetrics()
	m.EthAlternativeSendTx = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "test_alt_sendtx_total"}, []string{"provider_host", "status", "reason"})
	m.EthAlternativeSendTxDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{Name: "test_alt_sendtx_duration_seconds", Buckets: []float64{1}}, []string{"provider_host", "status"})
	return m
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
	failed := gatherMetric(t, m.EthAlternativeSendTx, map[string]string{"provider_host": "127.0.0.1:1", "status": bchain.SendTxStatusFailure})
	if failed == nil || failed.GetCounter().GetValue() != 1 {
		t.Errorf("unreachable provider counter = %v, want 1", failed)
	}
	timed := gatherMetric(t, m.EthAlternativeSendTxDuration, map[string]string{"provider_host": "127.0.0.1:1", "status": bchain.SendTxStatusFailure})
	if timed == nil || timed.GetHistogram().GetSampleCount() != 1 {
		t.Errorf("unreachable provider duration samples = %v, want 1 - a timing out provider is exactly what the histogram must show", timed)
	}
}

// TestAlternativeSendTxProviderObserveSendTxAccepted asserts an accepted broadcast is labeled
// success/ok under the provider's host rather than its full URL, which carries the API key.
func TestAlternativeSendTxProviderObserveSendTxAccepted(t *testing.T) {
	m := newSendTxMetrics()
	provider := &AlternativeSendTxProvider{metrics: m}

	provider.observeSendTx(providerLabel("https://relay.example.com/v1/SECRETKEY"), 250*time.Millisecond, nil)

	accepted := gatherMetric(t, m.EthAlternativeSendTx, map[string]string{"provider_host": "relay.example.com", "status": bchain.SendTxStatusSuccess, "reason": bchain.ReasonOK})
	if accepted == nil || accepted.GetCounter().GetValue() != 1 {
		t.Errorf("accepted send counter = %v, want 1", accepted)
	}
	timed := gatherMetric(t, m.EthAlternativeSendTxDuration, map[string]string{"provider_host": "relay.example.com", "status": bchain.SendTxStatusSuccess})
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

// TestAlternativeSendTxProviderErrorRedactedForClients pins the real error shape a failing provider
// call produces against the redaction the API applies before answering a client. The URL enters the
// message from the http client itself (Post "<url>": dial tcp ...), so a key in a provider URL is
// only kept out of REST/WebSocket responses for as long as that shape stays redactable - this test
// fails if either side drifts. The unredacted error is what reaches the logs, by design.
func TestAlternativeSendTxProviderErrorRedactedForClients(t *testing.T) {
	const key = "SECRET_API_KEY_ABCDEF"
	rawTx, _ := signedTestTx(t)
	provider := &AlternativeSendTxProvider{
		urls:              []string{"http://127.0.0.1:1/v3/" + key},
		mempoolTxsTimeout: time.Hour,
		rpcTimeout:        time.Second,
		metrics:           newSendTxMetrics(),
	}

	_, err := provider.SendRawTransaction(rawTx)
	if err == nil {
		t.Fatal("expected error from unreachable provider")
	}
	if !strings.Contains(err.Error(), key) {
		t.Skipf("provider error carries no url on this platform (%v), nothing to redact", err)
	}

	redacted := common.RedactURLs(err.Error())
	if strings.Contains(redacted, key) {
		t.Errorf("api key survives redaction, would be served to clients: %s", redacted)
	}
	if !strings.Contains(redacted, "127.0.0.1:1") {
		t.Errorf("provider host should survive redaction, got: %s", redacted)
	}
}
