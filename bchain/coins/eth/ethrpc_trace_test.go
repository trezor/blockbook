package eth

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/trezor/blockbook/bchain"
)

type mockTraceRPC struct {
	method string
	args   []interface{}
	trace  []rpcTraceResult // returned from debug_traceBlockByHash
}

func (m *mockTraceRPC) EthSubscribe(ctx context.Context, channel interface{}, args ...interface{}) (bchain.EVMClientSubscription, error) {
	return nil, errors.New("not implemented")
}

func (m *mockTraceRPC) CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error {
	m.method = method
	m.args = append([]interface{}{}, args...)
	if out, ok := result.(*[]rpcTraceResult); ok {
		*out = append([]rpcTraceResult{}, m.trace...)
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

// Trace subtree of mainnet tx 0xb247a9589bc1638dbd157937b121b20105d5d613d802ec0dab98017c861bfe39,
// where a contract fakes 3 ETH transfers to arbitrary addresses via CALLCODE (issue #1225).
func TestProcessCallTraceIgnoresFakeCallcodeTransfers(t *testing.T) {
	const (
		attacker  = "0x66a0e978c0b91034a27d0da7207d5f80f11e86dc"
		sender    = "0xa9d1e08c7793af67e9d92fe308d5697fb81d3e43"
		library   = "0x60f760bb7068e5ae61af835b10076d40d0a3d958"
		victim1   = "0xa3b36b1ee03f71926957194abad51a7c652e77d6"
		victim2   = "0x03533db5ac95abe2164ffd9199e96524a2207a1a"
		threeEth  = "0x29a2241af62c0000"
		halfEther = "0x6f05b59d3b20000"
	)
	trace := &rpcCallTrace{
		Type: "CALL", From: sender, To: attacker, Value: threeEth,
		Calls: []rpcCallTrace{
			{
				Type: "DELEGATECALL", From: attacker, To: library, Value: threeEth,
				Calls: []rpcCallTrace{
					{Type: "CALLCODE", From: attacker, To: victim1, Value: threeEth},
				},
			},
			{
				Type: "DELEGATECALL", From: attacker, To: library, Value: threeEth,
				Calls: []rpcCallTrace{
					{Type: "CALLCODE", From: attacker, To: victim2, Value: threeEth},
				},
			},
			{Type: "CALL", From: attacker, To: victim1, Value: halfEther},
		},
	}

	b := &EthereumRPC{ChainConfig: &Configuration{}}
	d := &bchain.EthereumInternalData{}
	b.processCallTrace(trace, d, nil, 1)

	want := []bchain.EthereumInternalTransfer{
		{Value: *hexutil.MustDecodeBig(threeEth), From: sender, To: attacker},
		{Value: *hexutil.MustDecodeBig(halfEther), From: attacker, To: victim1},
	}
	if len(d.Transfers) != len(want) {
		t.Fatalf("transfers = %+v, want only the real CALL transfers %+v", d.Transfers, want)
	}
	for i := range want {
		if d.Transfers[i].From != want[i].From || d.Transfers[i].To != want[i].To ||
			d.Transfers[i].Value.Cmp(&want[i].Value) != 0 || d.Transfers[i].Type != want[i].Type {
			t.Errorf("transfer[%d] = %+v, want %+v", i, d.Transfers[i], want[i])
		}
	}
}

// Failed frames are reverted in their entirety, so neither the frame itself nor anything
// beneath it may be indexed as a transfer or contract lifecycle event (issue #1621).
func TestProcessCallTraceSkipsFailedFramesAndTheirSubtrees(t *testing.T) {
	const (
		sender   = "0xa9d1e08c7793af67e9d92fe308d5697fb81d3e43"
		router   = "0x66a0e978c0b91034a27d0da7207d5f80f11e86dc"
		target   = "0xa3b36b1ee03f71926957194abad51a7c652e77d6"
		nested   = "0x03533db5ac95abe2164ffd9199e96524a2207a1a"
		created  = "0x60f760bb7068e5ae61af835b10076d40d0a3d958"
		oneEth   = "0xde0b6b3a7640000"
		twoEth   = "0x1bc16d674ec80000"
		threeEth = "0x29a2241af62c0000"
	)
	tests := []struct {
		name          string
		trace         *rpcCallTrace
		wantTransfers []bchain.EthereumInternalTransfer
		wantContracts int
		wantError     string
	}{
		{
			name: "successful root with insufficient-balance child",
			trace: &rpcCallTrace{
				Type: "CALL", From: sender, To: router, Value: oneEth,
				Calls: []rpcCallTrace{
					{Type: "CALL", From: router, To: target, Value: threeEth, Error: "insufficient balance for transfer"},
				},
			},
			wantTransfers: []bchain.EthereumInternalTransfer{
				{Value: *hexutil.MustDecodeBig(oneEth), From: sender, To: router},
			},
			wantError: "insufficient balance for transfer",
		},
		{
			name: "reverted parent hides successful descendants, sibling survives",
			trace: &rpcCallTrace{
				Type: "CALL", From: sender, To: router, Value: oneEth,
				Calls: []rpcCallTrace{
					{
						Type: "CALL", From: router, To: target, Value: twoEth, Error: "execution reverted",
						Calls: []rpcCallTrace{
							{Type: "CALL", From: target, To: nested, Value: oneEth},
						},
					},
					{Type: "CALL", From: router, To: nested, Value: oneEth},
				},
			},
			wantTransfers: []bchain.EthereumInternalTransfer{
				{Value: *hexutil.MustDecodeBig(oneEth), From: sender, To: router},
				{Value: *hexutil.MustDecodeBig(oneEth), From: router, To: nested},
			},
			wantError: "execution reverted",
		},
		{
			name: "failed CREATE and SELFDESTRUCT leave no lifecycle records",
			trace: &rpcCallTrace{
				Type: "CALL", From: sender, To: router,
				Calls: []rpcCallTrace{
					{Type: "CREATE", From: router, To: created, Value: oneEth, Error: "out of gas"},
					{Type: "SELFDESTRUCT", From: router, To: target, Value: twoEth, Error: "execution reverted"},
					{Type: "CREATE2", From: router, To: nested},
				},
			},
			wantTransfers: []bchain.EthereumInternalTransfer{
				{Type: bchain.CREATE, From: router, To: nested},
			},
			wantContracts: 1,
			wantError:     "execution reverted",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &EthereumRPC{ChainConfig: &Configuration{}}
			d := &bchain.EthereumInternalData{}
			contracts := b.processCallTrace(tt.trace, d, nil, 1)
			assertInternalTransfers(t, d.Transfers, tt.wantTransfers)
			if len(contracts) != tt.wantContracts {
				t.Errorf("contracts = %+v, want %d records", contracts, tt.wantContracts)
			}
			if d.Error != tt.wantError {
				t.Errorf("error = %q, want %q", d.Error, tt.wantError)
			}
		})
	}
}

// A reverted root transaction undoes every child frame, so a failed tx must not carry
// internal transfers into the index even when its children succeeded locally (issue #1621).
func TestGetInternalDataForBlockDropsChildrenOfRevertedRoot(t *testing.T) {
	const (
		sender = "0xa9d1e08c7793af67e9d92fe308d5697fb81d3e43"
		router = "0x66a0e978c0b91034a27d0da7207d5f80f11e86dc"
		target = "0xa3b36b1ee03f71926957194abad51a7c652e77d6"
		oneEth = "0xde0b6b3a7640000"
	)
	rpcClient := &mockTraceRPC{trace: []rpcTraceResult{
		{Result: rpcCallTrace{
			Type: "CALL", From: sender, To: router, Error: "execution reverted",
			Calls: []rpcCallTrace{{Type: "CALL", From: router, To: target, Value: oneEth}},
		}},
		{Result: rpcCallTrace{
			Type: "CALL", From: sender, To: router,
			Calls: []rpcCallTrace{{Type: "CALL", From: router, To: target, Value: oneEth}},
		}},
	}}
	b := &EthereumRPC{RPC: rpcClient, ChainConfig: &Configuration{ProcessInternalTransactions: true}}
	bchain.ProcessInternalTransactions = true
	t.Cleanup(func() {
		bchain.ProcessInternalTransactions = false
	})

	data, _, err := b.getInternalDataForBlock(context.Background(), "0xabc", 1, make([]bchain.RpcTransaction, 2))
	if err != nil {
		t.Fatalf("getInternalDataForBlock() error = %v", err)
	}
	assertInternalTransfers(t, data[0].Transfers, nil)
	if data[0].Error == "" {
		t.Errorf("reverted root should keep its error, got empty")
	}
	assertInternalTransfers(t, data[1].Transfers, []bchain.EthereumInternalTransfer{
		{Value: *hexutil.MustDecodeBig(oneEth), From: router, To: target},
	})
}

func assertInternalTransfers(t *testing.T, got, want []bchain.EthereumInternalTransfer) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("transfers = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i].From != want[i].From || got[i].To != want[i].To ||
			got[i].Value.Cmp(&want[i].Value) != 0 || got[i].Type != want[i].Type {
			t.Errorf("transfer[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}
