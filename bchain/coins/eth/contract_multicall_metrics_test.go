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

// elem_fallback counts the elements aggregate3 could not settle, not the requests that hit
// one: two holes in a single request must move the counter by two.
func TestEthereumTypeGetErc20ContractBalancesFallbackMetricsCountElements(t *testing.T) {
	useTestPrometheusRegistry(t)
	metrics, err := common.GetMetrics("Erc20MulticallFallbackTest")
	if err != nil {
		t.Fatalf("metrics: %v", err)
	}
	addr := ethcommon.HexToAddress("0x0000000000000000000000000000000000000011")
	contracts := erc20TestContracts(3)
	// A resolves; B and C come back Success=false with empty returndata (possibly gas-starved).
	agg := fixtureAggregate3Result([]bchain.EthereumMulticallResult{
		{Success: true, Data: fmt.Sprintf("0x%064x", erc20MulticallTestValue(0))},
		{Success: false, Data: "0x"},
		{Success: false, Data: "0x"},
	})
	inner := &mockBatchRPC{results: map[string]string{
		hexutil.Encode(contracts[1]): fmt.Sprintf("0x%064x", erc20BatchTestValue(1)),
		hexutil.Encode(contracts[2]): fmt.Sprintf("0x%064x", erc20BatchTestValue(2)),
	}}
	mock := &mockMulticallThenBatchRPC{mockBatchRPC: inner, aggregate3Resp: agg}
	rpcClient := &EthereumRPC{RPC: mock, Timeout: time.Second, metrics: metrics}
	if _, err := rpcClient.EthereumTypeGetErc20ContractBalances(bchain.AddressDescriptor(addr.Bytes()), contracts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := erc20MulticallFallbackCount(t, metrics, "elem_fallback"); got != 2 {
		t.Fatalf("elem_fallback = %v, want 2 (one per unsettled element, not one per request)", got)
	}
	if got := erc20MulticallFallbackCount(t, metrics, "error"); got != 0 {
		t.Fatalf("error fallback = %v, want 0 (aggregate3 itself did not fail)", got)
	}
}

// A failing chunk is one request-level "error" event; only element-level holes reach
// elem_fallback, so the two reasons never count the same failure twice.
func TestEthereumTypeGetErc20ContractBalancesFallbackMetricsSeparateChunkErrorsFromHoles(t *testing.T) {
	useTestPrometheusRegistry(t)
	metrics, err := common.GetMetrics("Erc20MulticallMixedFallbackTest")
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
	if got := erc20MulticallFallbackCount(t, metrics, "elem_fallback"); got != 1 {
		t.Fatalf("elem_fallback = %v, want 1 (the element hole only; the failed chunk is one error event)", got)
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
