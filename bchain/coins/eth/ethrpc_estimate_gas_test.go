package eth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

// countingEstimateGasServer answers eth_estimateGas with a fixed hex gas value and counts hits, so a
// test can tell which path - provider or primary - was consulted.
func countingEstimateGasServer(t *testing.T, gasHex string) (*httptest.Server, *int32) {
	t.Helper()
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"` + gasHex + `"}`)); err != nil {
			t.Errorf("Write() error = %v", err)
		}
	}))
	t.Cleanup(server.Close)
	return server, &hits
}

// newEstimateGasTestRPC points the primary Client at primaryURL and the alternative send-tx provider
// at providerURL, so the routing decision shows up as which server receives the hit.
func newEstimateGasTestRPC(t *testing.T, primaryURL, providerURL string) *EthereumRPC {
	t.Helper()
	primaryRPC, err := rpc.DialContext(context.Background(), primaryURL)
	if err != nil {
		t.Fatalf("dial primary: %v", err)
	}
	t.Cleanup(primaryRPC.Close)
	return &EthereumRPC{
		Client:  &EthereumClient{Client: ethclient.NewClient(primaryRPC)},
		Timeout: 2 * time.Second,
		alternativeSendTxProvider: &AlternativeSendTxProvider{
			urls:              []string{providerURL},
			mempoolTxsTimeout: time.Hour,
			rpcTimeout:        2 * time.Second,
			recentSenders:     map[ethcommon.Address]recentSender{},
		},
	}
}

// TestEthereumTypeEstimateGasSkipsProviderForNonRecentSender is the core of #1629: a sender with no
// recent private transaction must not have its estimate routed to the provider, so the hot estimateFee
// endpoint does not burn the provider's rate-limit quota.
func TestEthereumTypeEstimateGasSkipsProviderForNonRecentSender(t *testing.T) {
	primary, primaryHits := countingEstimateGasServer(t, "0x5208")
	provider, providerHits := countingEstimateGasServer(t, "0x9999")
	b := newEstimateGasTestRPC(t, primary.URL, provider.URL)

	gas, err := b.EthereumTypeEstimateGas(map[string]interface{}{
		"from": "0x2222222222222222222222222222222222222222",
		"to":   "0x3333333333333333333333333333333333333333",
	})
	if err != nil {
		t.Fatalf("EthereumTypeEstimateGas() error = %v", err)
	}
	if gas != 0x5208 {
		t.Fatalf("gas = %#x, want 0x5208 (primary backend value)", gas)
	}
	if got := atomic.LoadInt32(providerHits); got != 0 {
		t.Fatalf("provider hits = %d, want 0 (non-recent sender must not touch the provider)", got)
	}
	if got := atomic.LoadInt32(primaryHits); got != 1 {
		t.Fatalf("primary hits = %d, want 1", got)
	}
}

// TestEthereumTypeEstimateGasRoutesRecentSenderToProvider keeps the provider path for the case it
// exists for: a recent private sender goes to the URL that accepted its send (see nonceURL), which
// may know a pending tx the primary does not.
func TestEthereumTypeEstimateGasRoutesRecentSenderToProvider(t *testing.T) {
	primary, primaryHits := countingEstimateGasServer(t, "0x5208")
	provider, providerHits := countingEstimateGasServer(t, "0x9999")
	b := newEstimateGasTestRPC(t, primary.URL, provider.URL)

	sender := ethcommon.HexToAddress("0x2222222222222222222222222222222222222222")
	b.alternativeSendTxProvider.recentSenders[sender] = recentSender{
		time: time.Now(),
		url:  provider.URL,
		gen:  1,
	}

	gas, err := b.EthereumTypeEstimateGas(map[string]interface{}{
		"from": sender.Hex(),
		"to":   "0x3333333333333333333333333333333333333333",
	})
	if err != nil {
		t.Fatalf("EthereumTypeEstimateGas() error = %v", err)
	}
	if gas != 0x9999 {
		t.Fatalf("gas = %#x, want 0x9999 (provider value)", gas)
	}
	if got := atomic.LoadInt32(providerHits); got != 1 {
		t.Fatalf("provider hits = %d, want 1 (recent sender must be routed to the provider)", got)
	}
	if got := atomic.LoadInt32(primaryHits); got != 0 {
		t.Fatalf("primary hits = %d, want 0", got)
	}
}

// TestEthereumTypeEstimateGasFallsBackWhenProviderFails pins that a provider error is not fatal to the
// estimate.
func TestEthereumTypeEstimateGasFallsBackWhenProviderFails(t *testing.T) {
	primary, primaryHits := countingEstimateGasServer(t, "0x5208")
	// a provider server that always errors the JSON-RPC call
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32005,"message":"rate limited"}}`))
	}))
	t.Cleanup(provider.Close)
	b := newEstimateGasTestRPC(t, primary.URL, provider.URL)

	sender := ethcommon.HexToAddress("0x2222222222222222222222222222222222222222")
	b.alternativeSendTxProvider.recentSenders[sender] = recentSender{time: time.Now(), url: provider.URL, gen: 1}

	gas, err := b.EthereumTypeEstimateGas(map[string]interface{}{"from": sender.Hex()})
	if err != nil {
		t.Fatalf("EthereumTypeEstimateGas() error = %v", err)
	}
	if gas != 0x5208 {
		t.Fatalf("gas = %#x, want 0x5208 (primary fallback value)", gas)
	}
	if got := atomic.LoadInt32(primaryHits); got != 1 {
		t.Fatalf("primary hits = %d, want 1 (must fall back after provider error)", got)
	}
}

// TestEthereumTypeEstimateGasFallsBackWhenProviderReturnsMalformedResult treats a successful response
// carrying a non-decodable gas value as a provider failure: fall back, do not surface the decode error.
func TestEthereumTypeEstimateGasFallsBackWhenProviderReturnsMalformedResult(t *testing.T) {
	primary, primaryHits := countingEstimateGasServer(t, "0x5208")
	// a provider that returns a non-hex string result for eth_estimateGas
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"not-a-hex-quantity"}`))
	}))
	t.Cleanup(provider.Close)
	b := newEstimateGasTestRPC(t, primary.URL, provider.URL)

	sender := ethcommon.HexToAddress("0x2222222222222222222222222222222222222222")
	b.alternativeSendTxProvider.recentSenders[sender] = recentSender{time: time.Now(), url: provider.URL, gen: 1}

	gas, err := b.EthereumTypeEstimateGas(map[string]interface{}{"from": sender.Hex()})
	if err != nil {
		t.Fatalf("EthereumTypeEstimateGas() error = %v, want fallback to primary", err)
	}
	if gas != 0x5208 {
		t.Fatalf("gas = %#x, want 0x5208 (primary fallback value)", gas)
	}
	if got := atomic.LoadInt32(primaryHits); got != 1 {
		t.Fatalf("primary hits = %d, want 1 (malformed provider result must fall back)", got)
	}
}

// TestEthereumTypeEstimateGasNoFromUsesPrimary: without a from address the gate cannot apply, so the
// estimate takes the primary path.
func TestEthereumTypeEstimateGasNoFromUsesPrimary(t *testing.T) {
	primary, primaryHits := countingEstimateGasServer(t, "0x5208")
	provider, providerHits := countingEstimateGasServer(t, "0x9999")
	b := newEstimateGasTestRPC(t, primary.URL, provider.URL)

	if _, err := b.EthereumTypeEstimateGas(map[string]interface{}{
		"to": "0x3333333333333333333333333333333333333333",
	}); err != nil {
		t.Fatalf("EthereumTypeEstimateGas() error = %v", err)
	}
	if got := atomic.LoadInt32(providerHits); got != 0 {
		t.Fatalf("provider hits = %d, want 0", got)
	}
	if got := atomic.LoadInt32(primaryHits); got != 1 {
		t.Fatalf("primary hits = %d, want 1", got)
	}
}

// TestEthereumTypeEstimateGasBoundsFallbackToRequestBudget pins that a slow relay cannot double the
// request's wall time. Both legs are configured from rpc_timeout, so a fallback given a fresh full
// budget makes the worst case 2x - and on this endpoint that holds a websocket pending-request slot
// for twice as long, per send-form keystroke, exactly while the relay is already degraded.
//
// The two existing fallback tests use instant-error providers, so they pass whether the deadline is
// built at function entry or after the provider leg. This one needs a provider that is slow rather
// than broken.
func TestEthereumTypeEstimateGasBoundsFallbackToRequestBudget(t *testing.T) {
	primary, primaryHits := countingEstimateGasServer(t, "0x5208")
	slowProvider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond) // outlives the provider leg's own timeout below
	}))
	t.Cleanup(slowProvider.Close)

	b := newEstimateGasTestRPC(t, primary.URL, slowProvider.URL)
	b.Timeout = 200 * time.Millisecond
	b.alternativeSendTxProvider.rpcTimeout = 200 * time.Millisecond
	sender := ethcommon.HexToAddress("0x1111111111111111111111111111111111111111")
	b.alternativeSendTxProvider.recentSenders[sender] = recentSender{time: time.Now(), url: slowProvider.URL}

	started := time.Now()
	gas, err := b.EthereumTypeEstimateGas(map[string]interface{}{"from": sender.Hex(), "to": sender.Hex()})
	elapsed := time.Since(started)
	if err != nil {
		t.Fatalf("EthereumTypeEstimateGas() error = %v", err)
	}
	if gas != 0x5208 {
		t.Errorf("gas = %d, want the primary's answer after the slow provider", gas)
	}
	if atomic.LoadInt32(primaryHits) != 1 {
		t.Errorf("primary hits = %d, want 1", atomic.LoadInt32(primaryHits))
	}
	// the provider leg burned the whole budget, so the fallback runs on the floor, not a second budget
	if elapsed > b.Timeout+minEstimateFallbackTimeout {
		t.Errorf("elapsed %s exceeds the request budget %s plus the fallback floor %s", elapsed, b.Timeout, minEstimateFallbackTimeout)
	}
}

// TestRemainingEstimateTimeout covers the budget arithmetic directly: what is left of the request, but
// never so little that the fallback is pre-expired.
func TestRemainingEstimateTimeout(t *testing.T) {
	b := &EthereumRPC{Timeout: 25 * time.Second}
	for _, tt := range []struct {
		name    string
		elapsed time.Duration
		wantMin time.Duration
		wantMax time.Duration
	}{
		{name: "nothing spent yet", elapsed: 0, wantMin: 24 * time.Second, wantMax: 25 * time.Second},
		{name: "half spent", elapsed: 12 * time.Second, wantMin: 12*time.Second - time.Second, wantMax: 13 * time.Second},
		{name: "budget exhausted falls to the floor", elapsed: 25 * time.Second, wantMin: minEstimateFallbackTimeout, wantMax: minEstimateFallbackTimeout},
		{name: "budget overrun still gets the floor", elapsed: time.Minute, wantMin: minEstimateFallbackTimeout, wantMax: minEstimateFallbackTimeout},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := b.remainingEstimateTimeout(time.Now().Add(-tt.elapsed))
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("remainingEstimateTimeout() = %s, want within [%s, %s]", got, tt.wantMin, tt.wantMax)
			}
		})
	}
}
