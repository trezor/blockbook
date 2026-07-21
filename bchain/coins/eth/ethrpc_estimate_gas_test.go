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

// countingEstimateGasServer answers eth_estimateGas with a fixed hex gas value and counts how many
// times it was hit, so a test can assert whether a given path (provider vs. primary) was consulted.
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

// newEstimateGasTestRPC wires an EthereumRPC whose primary Client points at primaryURL and whose
// alternative send-tx provider points at providerURL, so a routing decision is observable by which
// server receives the eth_estimateGas hit.
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

// TestEthereumTypeEstimateGasSkipsProviderForNonRecentSender is the core of #1629: a sender that has
// not recently sent a private transaction through the alternative provider must not have its gas
// estimate routed there - it goes straight to the primary backend, so the hot estimateFee endpoint
// does not burn the provider's rate-limit quota.
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

// TestEthereumTypeEstimateGasRoutesRecentSenderToProvider confirms the provider path is preserved
// for the case it exists for: a sender with a recent private transaction is routed to the provider
// URL that accepted its send (see nonceURL), which may know a pending tx the primary does not.
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

// TestEthereumTypeEstimateGasFallsBackWhenProviderFails checks that a provider error is not fatal:
// a recent sender whose provider call fails still gets an estimate from the primary backend.
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

// TestEthereumTypeEstimateGasNoFromUsesPrimary confirms an estimate without a sender takes the
// primary path - the gate cannot apply without a from address.
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
