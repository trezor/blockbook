package eth

import (
	"fmt"
	"testing"
	"time"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
)

// newFallbackTestMetrics builds a common.Metrics holding the collectors the erc20 balance path
// touches, left unregistered so each test owns fresh collectors and needs no global registry.
func newFallbackTestMetrics() *common.Metrics {
	return &common.Metrics{
		ChainDataFallbacks: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "test_chain_data_fallbacks"}, []string{"component", "reason"}),
		EthCallRequests: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "test_eth_call_requests"}, []string{"mode"}),
		EthCallErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "test_eth_call_errors"}, []string{"mode", "type"}),
		EthCallBatchSize: prometheus.NewHistogram(
			prometheus.HistogramOpts{Name: "test_eth_call_batch_size", Buckets: []float64{1, 10, 100}}),
		EthCallMulticallRequests: prometheus.NewCounter(
			prometheus.CounterOpts{Name: "test_eth_call_multicall_requests"}),
	}
}

// elem_fallback counts the elements aggregate3 could not settle, not the requests that hit
// one: two holes in a single request must move the counter by two.
func TestEthereumTypeGetErc20ContractBalancesFallbackMetricsCountElements(t *testing.T) {
	metrics := newFallbackTestMetrics()
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
	if got := counterVecValue(t, metrics.ChainDataFallbacks, "reason", "elem_fallback"); got != 2 {
		t.Fatalf("elem_fallback = %v, want 2 (one per unsettled element, not one per request)", got)
	}
	if got := counterVecValue(t, metrics.ChainDataFallbacks, "reason", "error"); got != 0 {
		t.Fatalf("error fallback = %v, want 0 (aggregate3 itself did not fail)", got)
	}
}

// A failing chunk is one request-level "error" event; only element-level holes reach
// elem_fallback, so the two reasons never count the same failure twice.
func TestEthereumTypeGetErc20ContractBalancesFallbackMetricsSeparateChunkErrorsFromHoles(t *testing.T) {
	metrics := newFallbackTestMetrics()
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
	if got := counterVecValue(t, metrics.ChainDataFallbacks, "reason", "elem_fallback"); got != 1 {
		t.Fatalf("elem_fallback = %v, want 1 (the element hole only; the failed chunk is one error event)", got)
	}
	if got := counterVecValue(t, metrics.ChainDataFallbacks, "reason", "error"); got != 1 {
		t.Fatalf("error fallback = %v, want 1 per request that abandoned multicall", got)
	}
}

// A systemic aggregate3 failure emits exactly one request-level fallback event.
func TestEthereumTypeGetErc20ContractBalancesSystemicFailureEmitsOneFallbackEvent(t *testing.T) {
	metrics := newFallbackTestMetrics()
	addr := ethcommon.HexToAddress("0x0000000000000000000000000000000000000011")
	n := 3 * multicall3MaxCallsPerAggregate // 3 chunks, every aggregate3 fails
	contracts := erc20TestContracts(n)
	inner := &mockBatchRPC{results: map[string]string{}}
	for i, c := range contracts {
		inner.results[hexutil.Encode(c)] = fmt.Sprintf("0x%064x", erc20BatchTestValue(i))
	}
	mock := &mockMulticallMixedRPC{mockBatchRPC: inner, holeAt: -1, failFrom: 0}
	rpcClient := &EthereumRPC{RPC: mock, Timeout: time.Second, metrics: metrics}
	if _, err := rpcClient.EthereumTypeGetErc20ContractBalances(bchain.AddressDescriptor(addr.Bytes()), contracts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := counterVecValue(t, metrics.ChainDataFallbacks, "reason", "error"); got != 1 {
		t.Fatalf("error fallback = %v, want 1 (one event per request, not one per chunk)", got)
	}
	// Nothing was resolved, so the whole list took the plain batch path, not the merge fallback.
	if got := counterVecValue(t, metrics.ChainDataFallbacks, "reason", "elem_fallback"); got != 0 {
		t.Fatalf("elem_fallback = %v, want 0 (no partial result to patch up)", got)
	}
}
