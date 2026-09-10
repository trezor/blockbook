package eth

import (
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/juju/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
)

// TestInitAlternativeFeeProviderFailFast verifies that when a coin config
// explicitly selects an EVM alternative fee provider whose required API-key env
// var is missing, initialization fails fast instead of silently reverting to
// default fee estimation. An unset provider stays a no-op.
func TestInitAlternativeFeeProviderFailFast(t *testing.T) {
	const infuraParams = `{"url":"https://gas.api.infura.io/v3/${api_key}/networks/1/suggestedGasFees","periodSeconds":10}`
	const oneInchParams = `{"url":"https://api.1inch.dev/gas-price/v1.5/1","periodSeconds":10}`

	tests := []struct {
		name        string
		feeProvider string
		params      string
		wantErr     bool
	}{
		{name: "infura without INFURA_API_KEY fails fast", feeProvider: "infura", params: infuraParams, wantErr: true},
		{name: "1inch without ONE_INCH_API_KEY fails fast", feeProvider: "1inch", params: oneInchParams, wantErr: true},
		{name: "no provider configured is a no-op", feeProvider: "", params: "", wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Ensure the required env vars are unset for this test.
			t.Setenv("INFURA_API_KEY", "")
			t.Setenv("ONE_INCH_API_KEY", "")

			b := &EthereumRPC{ChainConfig: &Configuration{
				AlternativeEstimateFee:       tt.feeProvider,
				AlternativeEstimateFeeParams: tt.params,
			}}

			err := b.initAlternativeFeeProvider()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected initialization to fail fast, got nil error")
				}
				if b.alternativeFeeProvider != nil {
					t.Fatal("alternativeFeeProvider should be nil after a failed init")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

const infuraTestFeesResponse = `{
	"estimatedBaseFee": "10",
	"low": {"suggestedMaxPriorityFeePerGas": "1", "suggestedMaxFeePerGas": "11"},
	"medium": {"suggestedMaxPriorityFeePerGas": "2", "suggestedMaxFeePerGas": "12"},
	"high": {"suggestedMaxPriorityFeePerGas": "3", "suggestedMaxFeePerGas": "13"}
}`

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// stubFeeTransport routes feeHTTPClient through rt for the test, so the real
// getData/decode path runs without sockets (the sandbox forbids httptest binds).
func stubFeeTransport(t *testing.T, rt roundTripFunc) {
	orig := feeHTTPClient.Transport
	feeHTTPClient.Transport = rt
	t.Cleanup(func() { feeHTTPClient.Transport = orig })
}

func infuraTestFeesRoundTrip() (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(infuraTestFeesResponse)),
	}, nil
}

// newTestInfuraProvider builds an infuraFeeProvider directly, bypassing the
// constructor so no API key env vars or warm-up fetch are involved.
func newTestInfuraProvider(ttl time.Duration) *infuraFeeProvider {
	p := &infuraFeeProvider{
		alternativeFeeProvider: &alternativeFeeProvider{
			ttl:               ttl,
			staleSyncDuration: feeStaleDuration(int(ttl/time.Second), 0),
		},
		params: feeProviderParams{URL: "http://fees.test/"},
	}
	p.fetch = p.fetchFees
	return p
}

func TestGetEip1559FeesCoalescesConcurrentRequests(t *testing.T) {
	var upstreamRequests int32
	release := make(chan struct{})
	stubFeeTransport(t, func(r *http.Request) (*http.Response, error) {
		atomic.AddInt32(&upstreamRequests, 1)
		<-release
		return infuraTestFeesRoundTrip()
	})

	provider := newTestInfuraProvider(time.Minute)
	// unregistered collector: GetMetrics registers globally and would collide across tests
	cache := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "test_fee_cache"}, []string{"provider", "result"})
	provider.metrics = &common.Metrics{AlternativeFeeProviderCache: cache}

	const n = 20
	var wg sync.WaitGroup
	results := make([]*bchain.Eip1559Fees, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], _ = provider.GetEip1559Fees()
		}(i)
	}
	// let the goroutines pile up on the single in-flight fetch before releasing it
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	if got := atomic.LoadInt32(&upstreamRequests); got != 1 {
		t.Fatalf("upstream requests = %d, want 1 (requests must coalesce)", got)
	}
	for i, fees := range results {
		if fees == nil {
			t.Fatalf("request %d got nil fees", i)
		}
		if fees.Medium.MaxPriorityFeePerGas.Cmp(big.NewInt(2e9)) != 0 {
			t.Fatalf("request %d got medium priority fee %s, want 2 gwei", i, fees.Medium.MaxPriorityFeePerGas)
		}
	}
	// exactly the leader is labelled fetched; singleflight's shared flag would
	// have labelled it coalesced too, hiding the upstream fetch from the metric
	if m := gatherMetric(t, cache, map[string]string{"result": "fetched"}); m == nil || m.GetCounter().GetValue() != 1 {
		t.Fatalf("cache result fetched = %v, want 1", m.GetCounter().GetValue())
	}
	if m := gatherMetric(t, cache, map[string]string{"result": "coalesced"}); m == nil || m.GetCounter().GetValue() != n-1 {
		t.Fatalf("cache result coalesced = %v, want %d", m.GetCounter().GetValue(), n-1)
	}
}

func TestGetEip1559FeesRespectsTTL(t *testing.T) {
	var upstreamRequests int32
	stubFeeTransport(t, func(r *http.Request) (*http.Response, error) {
		atomic.AddInt32(&upstreamRequests, 1)
		return infuraTestFeesRoundTrip()
	})

	provider := newTestInfuraProvider(time.Minute)

	for i := 0; i < 3; i++ {
		fees, err := provider.GetEip1559Fees()
		if err != nil || fees == nil {
			t.Fatalf("call %d: fees=%v err=%v", i, fees, err)
		}
	}
	if got := atomic.LoadInt32(&upstreamRequests); got != 1 {
		t.Fatalf("upstream requests = %d, want 1 (calls within the TTL must be served from cache)", got)
	}

	// an expired TTL must trigger exactly one new upstream fetch
	provider.mux.Lock()
	provider.lastSync = time.Now().Add(-2 * time.Minute)
	provider.mux.Unlock()
	if fees, err := provider.GetEip1559Fees(); err != nil || fees == nil {
		t.Fatalf("post-TTL call: fees=%v err=%v", fees, err)
	}
	if got := atomic.LoadInt32(&upstreamRequests); got != 2 {
		t.Fatalf("upstream requests = %d, want 2 after TTL expiry", got)
	}
}

func TestGetEip1559FeesFailurePacingAndOnchainFallback(t *testing.T) {
	var fetchCalls int32
	provider := &alternativeFeeProvider{
		ttl:               time.Minute,
		staleSyncDuration: 10 * time.Minute,
		fetch: func() (*bchain.Eip1559Fees, error) {
			atomic.AddInt32(&fetchCalls, 1)
			return nil, errors.New("provider down")
		},
	}

	// cold start with a failing provider: nil fees and nil error, so the caller
	// falls through to on-chain estimation instead of failing the request
	for i := 0; i < 3; i++ {
		fees, err := provider.GetEip1559Fees()
		if err != nil {
			t.Fatalf("call %d: unexpected error %v", i, err)
		}
		if fees != nil {
			t.Fatalf("call %d: got fees %v from a failing provider with an empty cache", i, fees)
		}
	}
	if got := atomic.LoadInt32(&fetchCalls); got != 1 {
		t.Fatalf("fetch calls = %d, want 1 (failures must be paced, not retried per request)", got)
	}

	// failures are retried at most once per ttl, not once per the 5s floor — the
	// one-request-per-periodSeconds cap must hold during an outage too
	provider.mux.Lock()
	provider.lastFailure = time.Now().Add(-10 * time.Second)
	provider.mux.Unlock()
	provider.GetEip1559Fees()
	if got := atomic.LoadInt32(&fetchCalls); got != 1 {
		t.Fatalf("fetch calls = %d, want 1 (retry before the ttl elapsed since the last failure)", got)
	}

	provider.mux.Lock()
	provider.lastFailure = time.Now().Add(-2 * time.Minute)
	provider.mux.Unlock()
	provider.GetEip1559Fees()
	if got := atomic.LoadInt32(&fetchCalls); got != 2 {
		t.Fatalf("fetch calls = %d, want 2 (retry once the ttl elapsed since the last failure)", got)
	}
}

func TestGetEip1559FeesReturnsCopy(t *testing.T) {
	provider := &alternativeFeeProvider{
		ttl: time.Minute,
		eip1559Fees: &bchain.Eip1559Fees{
			BaseFeePerGas: big.NewInt(10),
			Medium:        &bchain.Eip1559Fee{MaxFeePerGas: big.NewInt(12), MaxPriorityFeePerGas: big.NewInt(2)},
		},
		lastSync: time.Now(),
	}

	fees, err := provider.GetEip1559Fees()
	if err != nil || fees == nil {
		t.Fatalf("GetEip1559Fees() fees=%v err=%v", fees, err)
	}
	fees.Medium.MaxFeePerGas.SetInt64(999)

	fees2, _ := provider.GetEip1559Fees()
	if fees2.Medium.MaxFeePerGas.Cmp(big.NewInt(12)) != 0 {
		t.Fatalf("cache was mutated through a returned value: medium maxFeePerGas = %s, want 12", fees2.Medium.MaxFeePerGas)
	}
}
