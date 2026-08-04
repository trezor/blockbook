package eth

import (
	"fmt"
	"sync"
	"testing"
	"time"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
)

var prometheusRegistryMu sync.Mutex

// useTestPrometheusRegistry swaps the global prometheus registry for a fresh one for the
// duration of the test, so repeated in-binary runs (go test -count=2) do not hit
// duplicate-registration errors from common.GetMetrics.
func useTestPrometheusRegistry(t *testing.T) {
	t.Helper()

	prometheusRegistryMu.Lock()
	oldRegisterer := prometheus.DefaultRegisterer
	oldGatherer := prometheus.DefaultGatherer
	registry := prometheus.NewRegistry()
	prometheus.DefaultRegisterer = registry
	prometheus.DefaultGatherer = registry

	t.Cleanup(func() {
		prometheus.DefaultRegisterer = oldRegisterer
		prometheus.DefaultGatherer = oldGatherer
		prometheusRegistryMu.Unlock()
	})
}

// erc20MulticallFallbackCount reads one reason of the erc20_multicall fallback counter.
func erc20MulticallFallbackCount(t *testing.T, metrics *common.Metrics, reason string) float64 {
	t.Helper()
	var m dto.Metric
	c := metrics.ChainDataFallbacks.With(common.Labels{"component": "erc20_multicall", "reason": reason})
	if err := c.Write(&m); err != nil {
		t.Fatalf("reading %q counter: %v", reason, err)
	}
	return m.GetCounter().GetValue()
}

// elem_fallback must count every contract the fallback re-fetched, whichever hole put it
// there; the error counter stays one event per request, not one per failing chunk.
func TestEthereumTypeGetErc20ContractBalancesFallbackMetricsCountEveryContract(t *testing.T) {
	useTestPrometheusRegistry(t)
	metrics, err := common.GetMetrics("Erc20MulticallFallbackTest")
	if err != nil {
		t.Fatalf("metrics: %v", err)
	}
	addr := ethcommon.HexToAddress("0x0000000000000000000000000000000000000011")
	n := multicall3MaxCallsPerAggregate + 20 // chunk 1 succeeds with a hole, chunk 2 fails
	holeAt := 3
	contracts := erc20TestContracts(n)
	inner := &mockBatchRPC{results: map[string]string{
		hexutil.Encode(contracts[holeAt]): fmt.Sprintf("0x%064x", erc20BatchTestValue(holeAt)),
	}}
	for i := multicall3MaxCallsPerAggregate; i < n; i++ {
		inner.results[hexutil.Encode(contracts[i])] = fmt.Sprintf("0x%064x", erc20BatchTestValue(i))
	}
	mock := &mockMulticallMixedRPC{mockBatchRPC: inner, holeAt: holeAt, failFrom: multicall3MaxCallsPerAggregate}
	rpcClient := &EthereumRPC{RPC: mock, Timeout: time.Second, metrics: metrics}
	if _, err := rpcClient.EthereumTypeGetErc20ContractBalances(bchain.AddressDescriptor(addr.Bytes()), contracts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want, got := float64(1+n-multicall3MaxCallsPerAggregate), erc20MulticallFallbackCount(t, metrics, "elem_fallback"); got != want {
		t.Fatalf("elem_fallback = %v, want %v (the reresolve hole plus every contract of the failed chunk)", got, want)
	}
	if got := erc20MulticallFallbackCount(t, metrics, "error"); got != 1 {
		t.Fatalf("error fallback = %v, want 1 per request that abandoned multicall", got)
	}
}

// A systemic aggregate3 failure emits exactly one request-level fallback event.
func TestEthereumTypeGetErc20ContractBalancesSystemicFailureEmitsOneFallbackEvent(t *testing.T) {
	useTestPrometheusRegistry(t)
	metrics, err := common.GetMetrics("Erc20MulticallSystemicFallbackTest")
	if err != nil {
		t.Fatalf("metrics: %v", err)
	}
	addr := ethcommon.HexToAddress("0x0000000000000000000000000000000000000011")
	n := 3 * multicall3MaxCallsPerAggregate // 3 chunks, every aggregate3 fails
	contracts := erc20TestContracts(n)
	inner := &mockBatchRPC{results: map[string]string{}}
	for i, c := range contracts {
		inner.results[hexutil.Encode(c)] = fmt.Sprintf("0x%064x", erc20BatchTestValue(i))
	}
	mock := &mockMulticallChunkFailRPC{mockBatchRPC: inner, failAfter: 0}
	rpcClient := &EthereumRPC{RPC: mock, Timeout: time.Second, metrics: metrics}
	if _, err := rpcClient.EthereumTypeGetErc20ContractBalances(bchain.AddressDescriptor(addr.Bytes()), contracts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := erc20MulticallFallbackCount(t, metrics, "error"); got != 1 {
		t.Fatalf("error fallback = %v, want 1 (one event per request, not one per chunk)", got)
	}
	// Nothing was resolved, so the whole list took the plain batch path, not the merge fallback.
	if got := erc20MulticallFallbackCount(t, metrics, "elem_fallback"); got != 0 {
		t.Fatalf("elem_fallback = %v, want 0 (no partial result to patch up)", got)
	}
}
