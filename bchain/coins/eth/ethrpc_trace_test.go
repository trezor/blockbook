package eth

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/trezor/blockbook/bchain"
)

type mockTraceRPC struct {
	method string
	args   []interface{}
}

func (m *mockTraceRPC) EthSubscribe(ctx context.Context, channel interface{}, args ...interface{}) (bchain.EVMClientSubscription, error) {
	return nil, errors.New("not implemented")
}

func (m *mockTraceRPC) CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error {
	m.method = method
	m.args = append([]interface{}{}, args...)
	if out, ok := result.(*[]rpcTraceResult); ok {
		*out = []rpcTraceResult{}
	}
	return nil
}

func (m *mockTraceRPC) Close() {}

func TestNewEthereumRPCRejectsInvalidTraceTimeout(t *testing.T) {
	_, err := NewEthereumRPC(json.RawMessage(`{
		"coin_name":"Ethereum",
		"coin_shortcut":"ETH",
		"rpc_timeout":25,
		"trace_timeout":"not-a-duration",
		"block_addresses_to_keep":600
	}`), nil)
	if err == nil {
		t.Fatal("expected invalid trace_timeout error")
	}
}

func TestGetInternalDataForBlockIncludesTraceTimeout(t *testing.T) {
	rpcClient := &mockTraceRPC{}
	b := &EthereumRPC{
		RPC: rpcClient,
		ChainConfig: &Configuration{
			ProcessInternalTransactions: true,
			TraceTimeout:                "20s",
		},
	}
	bchain.ProcessInternalTransactions = true
	t.Cleanup(func() {
		bchain.ProcessInternalTransactions = false
	})

	_, _, err := b.getInternalDataForBlock(context.Background(), "0xabc", 1, nil)
	if err != nil {
		t.Fatalf("getInternalDataForBlock() error = %v", err)
	}
	if rpcClient.method != "debug_traceBlockByHash" {
		t.Fatalf("method = %q, want %q", rpcClient.method, "debug_traceBlockByHash")
	}
	if len(rpcClient.args) != 2 {
		t.Fatalf("args len = %d, want 2", len(rpcClient.args))
	}
	traceConfig, ok := rpcClient.args[1].(map[string]interface{})
	if !ok {
		t.Fatalf("trace config type = %T, want map[string]interface{}", rpcClient.args[1])
	}
	if got := traceConfig["tracer"]; got != "callTracer" {
		t.Fatalf("tracer = %#v, want %q", got, "callTracer")
	}
	if got := traceConfig["timeout"]; got != "20s" {
		t.Fatalf("timeout = %#v, want %q", got, "20s")
	}
}

func TestGetInternalDataForBlockOmitsTraceTimeoutWhenUnset(t *testing.T) {
	rpcClient := &mockTraceRPC{}
	b := &EthereumRPC{
		RPC: rpcClient,
		ChainConfig: &Configuration{
			ProcessInternalTransactions: true,
		},
	}
	bchain.ProcessInternalTransactions = true
	t.Cleanup(func() {
		bchain.ProcessInternalTransactions = false
	})

	_, _, err := b.getInternalDataForBlock(context.Background(), "0xabc", 1, nil)
	if err != nil {
		t.Fatalf("getInternalDataForBlock() error = %v", err)
	}
	traceConfig, ok := rpcClient.args[1].(map[string]interface{})
	if !ok {
		t.Fatalf("trace config type = %T, want map[string]interface{}", rpcClient.args[1])
	}
	if _, ok := traceConfig["timeout"]; ok {
		t.Fatalf("timeout should be omitted when unset, config = %#v", traceConfig)
	}
}
