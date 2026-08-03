package eth

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/trezor/blockbook/bchain"
)

type mockBatchRPC struct {
	results    map[string]string
	perErr     map[string]error
	lastBatch  []rpc.BatchElem
	batchSizes []int
}

func (m *mockBatchRPC) EthSubscribe(ctx context.Context, channel interface{}, args ...interface{}) (bchain.EVMClientSubscription, error) {
	return nil, errors.New("not implemented")
}

func (m *mockBatchRPC) CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error {
	// Probe answers "not deployed" so these tests deterministically exercise the batch fallback.
	if method == "eth_getCode" {
		if out, ok := result.(*string); ok {
			*out = "0x"
		}
		return nil
	}
	return errors.New("not implemented")
}

func (m *mockBatchRPC) Close() {}

func (m *mockBatchRPC) BatchCallContext(ctx context.Context, batch []rpc.BatchElem) error {
	m.lastBatch = batch
	m.batchSizes = append(m.batchSizes, len(batch))
	for i := range batch {
		elem := &batch[i]
		if elem.Method != "eth_call" {
			elem.Error = errors.New("unexpected method")
			continue
		}
		if len(elem.Args) < 2 {
			elem.Error = errors.New("missing args")
			continue
		}
		args, ok := elem.Args[0].(map[string]interface{})
		if !ok {
			elem.Error = errors.New("bad args")
			continue
		}
		to, _ := args["to"].(string)
		if err, ok := m.perErr[to]; ok {
			elem.Error = err
			continue
		}
		res, ok := m.results[to]
		if !ok {
			elem.Error = errors.New("missing result")
			continue
		}
		out, ok := elem.Result.(*string)
		if !ok {
			elem.Error = errors.New("bad result type")
			continue
		}
		*out = res
	}
	return nil
}

type rpcCall struct {
	to   string
	data string
}

type mockBatchCallRPC struct {
	batchResults map[string]string
	batchErrors  map[string]error
	batchRPCErr  error
	callResults  map[string]string
	callErrors   map[string]error
	batchCalls   []rpcCall
	calls        []rpcCall
}

func (m *mockBatchCallRPC) EthSubscribe(ctx context.Context, channel interface{}, args ...interface{}) (bchain.EVMClientSubscription, error) {
	return nil, errors.New("not implemented")
}

func (m *mockBatchCallRPC) Close() {}

func (m *mockBatchCallRPC) CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error {
	if method != "eth_call" {
		return errors.New("unexpected method")
	}
	if len(args) < 2 {
		return errors.New("missing args")
	}
	argMap, ok := args[0].(map[string]interface{})
	if !ok {
		return errors.New("bad args")
	}
	to, _ := argMap["to"].(string)
	data, _ := argMap["data"].(string)
	m.calls = append(m.calls, rpcCall{to: to, data: data})
	if err, ok := m.callErrors[to]; ok {
		return err
	}
	res, ok := m.callResults[to]
	if !ok {
		return errors.New("missing result")
	}
	out, ok := result.(*string)
	if !ok {
		return errors.New("bad result type")
	}
	*out = res
	return nil
}

func (m *mockBatchCallRPC) BatchCallContext(ctx context.Context, batch []rpc.BatchElem) error {
	if m.batchRPCErr != nil {
		return m.batchRPCErr
	}
	for i := range batch {
		elem := &batch[i]
		if elem.Method != "eth_call" {
			elem.Error = errors.New("unexpected method")
			continue
		}
		if len(elem.Args) < 2 {
			elem.Error = errors.New("missing args")
			continue
		}
		argMap, ok := elem.Args[0].(map[string]interface{})
		if !ok {
			elem.Error = errors.New("bad args")
			continue
		}
		to, _ := argMap["to"].(string)
		data, _ := argMap["data"].(string)
		m.batchCalls = append(m.batchCalls, rpcCall{to: to, data: data})
		if err, ok := m.batchErrors[to]; ok {
			elem.Error = err
			continue
		}
		res, ok := m.batchResults[to]
		if !ok {
			elem.Error = errors.New("missing result")
			continue
		}
		out, ok := elem.Result.(*string)
		if !ok {
			elem.Error = errors.New("bad result type")
			continue
		}
		*out = res
	}
	return nil
}

func TestErc20BalanceOfCallData(t *testing.T) {
	addr := common.HexToAddress("0x0000000000000000000000000000000000000011")
	data := erc20BalanceOfCallData(bchain.AddressDescriptor(addr.Bytes()))
	if !strings.HasPrefix(data, contractBalanceOfSignature) {
		t.Fatalf("expected prefix %q, got %q", contractBalanceOfSignature, data)
	}
	payload := data[len(contractBalanceOfSignature):]
	if len(payload) != 64 {
		t.Fatalf("expected 64 hex chars payload, got %d", len(payload))
	}
	addrHex := strings.TrimPrefix(hexutil.Encode(addr.Bytes()), "0x")
	if !strings.HasSuffix(payload, addrHex) {
		t.Fatalf("expected payload suffix %q, got %q", addrHex, payload)
	}
	padding := payload[:len(payload)-len(addrHex)]
	if padding != strings.Repeat("0", len(padding)) {
		t.Fatalf("expected zero padding, got %q", padding)
	}
}

func TestErc20BalancesBatchSuccess(t *testing.T) {
	addr := common.HexToAddress("0x0000000000000000000000000000000000000011")
	contractA := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	contractB := common.HexToAddress("0x00000000000000000000000000000000000000bb")
	contractAKey := hexutil.Encode(contractA.Bytes())
	contractBKey := hexutil.Encode(contractB.Bytes())
	callData := erc20BalanceOfCallData(bchain.AddressDescriptor(addr.Bytes()))
	mock := &mockBatchCallRPC{
		batchResults: map[string]string{
			contractAKey: fmt.Sprintf("0x%064x", 7),
			contractBKey: fmt.Sprintf("0x%064x", 9),
		},
	}
	rpcClient := &EthereumRPC{
		RPC:     mock,
		Timeout: time.Second,
	}
	balances, err := rpcClient.erc20BalancesBatch(mock, callData, []bchain.AddressDescriptor{
		bchain.AddressDescriptor(contractA.Bytes()),
		bchain.AddressDescriptor(contractB.Bytes()),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if balances[0] == nil || balances[0].Cmp(big.NewInt(7)) != 0 {
		t.Fatalf("unexpected balance[0]: %v", balances[0])
	}
	if balances[1] == nil || balances[1].Cmp(big.NewInt(9)) != 0 {
		t.Fatalf("unexpected balance[1]: %v", balances[1])
	}
	if len(mock.calls) != 0 {
		t.Fatalf("expected no fallback calls, got %d", len(mock.calls))
	}
	if len(mock.batchCalls) != 2 {
		t.Fatalf("expected 2 batch calls, got %d", len(mock.batchCalls))
	}
	for _, call := range mock.batchCalls {
		if call.data != callData {
			t.Fatalf("unexpected batch call data: %q", call.data)
		}
	}
}

func TestErc20BalancesBatchFallback(t *testing.T) {
	addr := common.HexToAddress("0x0000000000000000000000000000000000000011")
	contractA := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	contractB := common.HexToAddress("0x00000000000000000000000000000000000000bb")
	contractAKey := hexutil.Encode(contractA.Bytes())
	contractBKey := hexutil.Encode(contractB.Bytes())
	callData := erc20BalanceOfCallData(bchain.AddressDescriptor(addr.Bytes()))
	mock := &mockBatchCallRPC{
		batchResults: map[string]string{
			contractAKey: fmt.Sprintf("0x%064x", 1),
		},
		batchErrors: map[string]error{
			contractBKey: errors.New("boom"),
		},
		callResults: map[string]string{
			contractBKey: fmt.Sprintf("0x%064x", 5),
		},
	}
	rpcClient := &EthereumRPC{
		RPC:     mock,
		Timeout: time.Second,
	}
	balances, err := rpcClient.erc20BalancesBatch(mock, callData, []bchain.AddressDescriptor{
		bchain.AddressDescriptor(contractA.Bytes()),
		bchain.AddressDescriptor(contractB.Bytes()),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if balances[0] == nil || balances[0].Cmp(big.NewInt(1)) != 0 {
		t.Fatalf("unexpected balance[0]: %v", balances[0])
	}
	if balances[1] == nil || balances[1].Cmp(big.NewInt(5)) != 0 {
		t.Fatalf("unexpected balance[1]: %v", balances[1])
	}
	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 fallback call, got %d", len(mock.calls))
	}
	if mock.calls[0].to != contractBKey {
		t.Fatalf("expected fallback call to %q, got %q", contractBKey, mock.calls[0].to)
	}
	if mock.calls[0].data != callData {
		t.Fatalf("expected fallback call data %q, got %q", callData, mock.calls[0].data)
	}
}

func TestErc20BalancesBatchWholeBatchRPCError(t *testing.T) {
	addr := common.HexToAddress("0x0000000000000000000000000000000000000011")
	contractA := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	contractB := common.HexToAddress("0x00000000000000000000000000000000000000bb")
	contractAKey := hexutil.Encode(contractA.Bytes())
	contractBKey := hexutil.Encode(contractB.Bytes())
	callData := erc20BalanceOfCallData(bchain.AddressDescriptor(addr.Bytes()))
	mock := &mockBatchCallRPC{
		batchRPCErr: errors.New("connection reset"),
		callResults: map[string]string{
			contractAKey: fmt.Sprintf("0x%064x", 11),
			contractBKey: fmt.Sprintf("0x%064x", 22),
		},
	}
	rpcClient := &EthereumRPC{
		RPC:     mock,
		Timeout: time.Second,
	}
	balances, err := rpcClient.erc20BalancesBatch(mock, callData, []bchain.AddressDescriptor{
		bchain.AddressDescriptor(contractA.Bytes()),
		bchain.AddressDescriptor(contractB.Bytes()),
	})
	if err != nil {
		t.Fatalf("expected nil error after fallback, got %v", err)
	}
	if len(balances) != 2 {
		t.Fatalf("expected 2 balances, got %d", len(balances))
	}
	if balances[0] == nil || balances[0].Cmp(big.NewInt(11)) != 0 {
		t.Fatalf("unexpected balance[0]: %v", balances[0])
	}
	if balances[1] == nil || balances[1].Cmp(big.NewInt(22)) != 0 {
		t.Fatalf("unexpected balance[1]: %v", balances[1])
	}
	if len(mock.calls) != 2 {
		t.Fatalf("expected 2 single-call fallbacks, got %d", len(mock.calls))
	}
	gotTos := map[string]bool{mock.calls[0].to: true, mock.calls[1].to: true}
	if !gotTos[contractAKey] || !gotTos[contractBKey] {
		t.Fatalf("expected fallbacks for both contracts, got %+v", mock.calls)
	}
	for _, call := range mock.calls {
		if call.data != callData {
			t.Fatalf("unexpected fallback call data: %q", call.data)
		}
	}
}

func TestErc20BalancesBatchWholeBatchRPCErrorPartialSingleFailure(t *testing.T) {
	addr := common.HexToAddress("0x0000000000000000000000000000000000000011")
	contractA := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	contractB := common.HexToAddress("0x00000000000000000000000000000000000000bb")
	contractAKey := hexutil.Encode(contractA.Bytes())
	contractBKey := hexutil.Encode(contractB.Bytes())
	callData := erc20BalanceOfCallData(bchain.AddressDescriptor(addr.Bytes()))
	mock := &mockBatchCallRPC{
		batchRPCErr: errors.New("connection reset"),
		callResults: map[string]string{
			contractAKey: fmt.Sprintf("0x%064x", 11),
		},
		callErrors: map[string]error{
			contractBKey: errors.New("still broken"),
		},
	}
	rpcClient := &EthereumRPC{
		RPC:     mock,
		Timeout: time.Second,
	}
	balances, err := rpcClient.erc20BalancesBatch(mock, callData, []bchain.AddressDescriptor{
		bchain.AddressDescriptor(contractA.Bytes()),
		bchain.AddressDescriptor(contractB.Bytes()),
	})
	if err != nil {
		t.Fatalf("expected nil error after fallback, got %v", err)
	}
	if len(balances) != 2 {
		t.Fatalf("expected 2 balances, got %d", len(balances))
	}
	if balances[0] == nil || balances[0].Cmp(big.NewInt(11)) != 0 {
		t.Fatalf("unexpected balance[0]: %v", balances[0])
	}
	if balances[1] != nil {
		t.Fatalf("expected balance[1] to be nil after single-call failure, got %v", balances[1])
	}
}

func TestErc20BalancesBatchInvalidResult(t *testing.T) {
	addr := common.HexToAddress("0x0000000000000000000000000000000000000011")
	contractA := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	contractB := common.HexToAddress("0x00000000000000000000000000000000000000bb")
	contractAKey := hexutil.Encode(contractA.Bytes())
	contractBKey := hexutil.Encode(contractB.Bytes())
	callData := erc20BalanceOfCallData(bchain.AddressDescriptor(addr.Bytes()))
	mock := &mockBatchCallRPC{
		batchResults: map[string]string{
			contractAKey: "0x01",
			contractBKey: fmt.Sprintf("0x%064x", 2),
		},
	}
	rpcClient := &EthereumRPC{
		RPC:     mock,
		Timeout: time.Second,
	}
	balances, err := rpcClient.erc20BalancesBatch(mock, callData, []bchain.AddressDescriptor{
		bchain.AddressDescriptor(contractA.Bytes()),
		bchain.AddressDescriptor(contractB.Bytes()),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if balances[0] != nil {
		t.Fatalf("expected balance[0] to be nil, got %v", balances[0])
	}
	if balances[1] == nil || balances[1].Cmp(big.NewInt(2)) != 0 {
		t.Fatalf("unexpected balance[1]: %v", balances[1])
	}
	if len(mock.calls) != 0 {
		t.Fatalf("expected no fallback calls, got %d", len(mock.calls))
	}
}

func TestEthereumTypeGetErc20ContractBalances(t *testing.T) {
	addr := common.HexToAddress("0x0000000000000000000000000000000000000011")
	contractA := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	contractB := common.HexToAddress("0x00000000000000000000000000000000000000bb")
	contractAKey := hexutil.Encode(contractA.Bytes())
	contractBKey := hexutil.Encode(contractB.Bytes())
	mock := &mockBatchRPC{
		results: map[string]string{
			contractAKey: fmt.Sprintf("0x%064x", 123),
			contractBKey: fmt.Sprintf("0x%064x", 0),
		},
	}
	rpcClient := &EthereumRPC{
		RPC:     mock,
		Timeout: time.Second,
	}
	balances, err := rpcClient.EthereumTypeGetErc20ContractBalances(
		bchain.AddressDescriptor(addr.Bytes()),
		[]bchain.AddressDescriptor{
			bchain.AddressDescriptor(contractA.Bytes()),
			bchain.AddressDescriptor(contractB.Bytes()),
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(balances) != 2 {
		t.Fatalf("expected 2 balances, got %d", len(balances))
	}
	if balances[0] == nil || balances[0].Cmp(big.NewInt(123)) != 0 {
		t.Fatalf("unexpected balance[0]: %v", balances[0])
	}
	if balances[1] == nil || balances[1].Sign() != 0 {
		t.Fatalf("unexpected balance[1]: %v", balances[1])
	}
}

func TestEthereumTypeGetErc20ContractBalanceReverted(t *testing.T) {
	// A deterministic balanceOf revert must map to the benign ErrInvalidErc20Balance,
	// matching the batch path, so callers log it at V(2) instead of warning.
	addr := common.HexToAddress("0x0000000000000000000000000000000000000011")
	for _, revertMsg := range []string{
		"execution reverted: division or modulo by zero",
		"execution reverted",
		"invalid opcode: INVALID",
		"out of gas",
	} {
		contract := common.HexToAddress("0x00000000000000000000000000000000000000aa")
		contractKey := hexutil.Encode(contract.Bytes())
		mock := &mockBatchCallRPC{
			callErrors: map[string]error{
				contractKey: errors.New(revertMsg),
			},
		}
		rpcClient := &EthereumRPC{
			RPC:     mock,
			Timeout: time.Second,
		}
		balance, err := rpcClient.EthereumTypeGetErc20ContractBalance(
			bchain.AddressDescriptor(addr.Bytes()),
			bchain.AddressDescriptor(contract.Bytes()),
		)
		if balance != nil {
			t.Fatalf("%q: expected nil balance, got %v", revertMsg, balance)
		}
		if err != ErrInvalidErc20Balance {
			t.Fatalf("%q: expected ErrInvalidErc20Balance, got %v", revertMsg, err)
		}
	}
}

func TestEthereumTypeGetErc20ContractBalanceRPCError(t *testing.T) {
	// A genuine (non-revert) RPC error must surface unchanged so it stays visible at warning level.
	addr := common.HexToAddress("0x0000000000000000000000000000000000000011")
	contract := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	contractKey := hexutil.Encode(contract.Bytes())
	rpcErr := errors.New("connection refused")
	mock := &mockBatchCallRPC{
		callErrors: map[string]error{
			contractKey: rpcErr,
		},
	}
	rpcClient := &EthereumRPC{
		RPC:     mock,
		Timeout: time.Second,
	}
	balance, err := rpcClient.EthereumTypeGetErc20ContractBalance(
		bchain.AddressDescriptor(addr.Bytes()),
		bchain.AddressDescriptor(contract.Bytes()),
	)
	if balance != nil {
		t.Fatalf("expected nil balance, got %v", balance)
	}
	if err != rpcErr {
		t.Fatalf("expected raw RPC error to surface unchanged, got %v", err)
	}
	if err == ErrInvalidErc20Balance {
		t.Fatalf("genuine RPC error must not be classified as ErrInvalidErc20Balance")
	}
}

func TestEthereumTypeGetErc20ContractBalancesBatchSize(t *testing.T) {
	addr := common.HexToAddress("0x0000000000000000000000000000000000000011")
	contractA := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	contractB := common.HexToAddress("0x00000000000000000000000000000000000000bb")
	contractC := common.HexToAddress("0x00000000000000000000000000000000000000cc")
	mock := &mockBatchRPC{
		results: map[string]string{
			hexutil.Encode(contractA.Bytes()): fmt.Sprintf("0x%064x", 1),
			hexutil.Encode(contractB.Bytes()): fmt.Sprintf("0x%064x", 2),
			hexutil.Encode(contractC.Bytes()): fmt.Sprintf("0x%064x", 3),
		},
	}
	rpcClient := &EthereumRPC{
		RPC:         mock,
		Timeout:     time.Second,
		ChainConfig: &Configuration{Erc20BatchSize: 2},
	}
	balances, err := rpcClient.EthereumTypeGetErc20ContractBalances(
		bchain.AddressDescriptor(addr.Bytes()),
		[]bchain.AddressDescriptor{
			bchain.AddressDescriptor(contractA.Bytes()),
			bchain.AddressDescriptor(contractB.Bytes()),
			bchain.AddressDescriptor(contractC.Bytes()),
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(balances) != 3 {
		t.Fatalf("expected 3 balances, got %d", len(balances))
	}
	if len(mock.batchSizes) != 2 || mock.batchSizes[0] != 2 || mock.batchSizes[1] != 1 {
		t.Fatalf("unexpected batch sizes: %v", mock.batchSizes)
	}
}

func TestEthereumTypeGetErc20ContractBalancesPartialError(t *testing.T) {
	addr := common.HexToAddress("0x0000000000000000000000000000000000000011")
	contractA := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	contractB := common.HexToAddress("0x00000000000000000000000000000000000000bb")
	contractAKey := hexutil.Encode(contractA.Bytes())
	contractBKey := hexutil.Encode(contractB.Bytes())
	mock := &mockBatchRPC{
		results: map[string]string{
			contractAKey: fmt.Sprintf("0x%064x", 42),
		},
		perErr: map[string]error{
			contractBKey: errors.New("boom"),
		},
	}
	rpcClient := &EthereumRPC{
		RPC:     mock,
		Timeout: time.Second,
	}
	balances, err := rpcClient.EthereumTypeGetErc20ContractBalances(
		bchain.AddressDescriptor(addr.Bytes()),
		[]bchain.AddressDescriptor{
			bchain.AddressDescriptor(contractA.Bytes()),
			bchain.AddressDescriptor(contractB.Bytes()),
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if balances[0] == nil || balances[0].Cmp(big.NewInt(42)) != 0 {
		t.Fatalf("unexpected balance[0]: %v", balances[0])
	}
	if balances[1] != nil {
		t.Fatalf("expected balance[1] to be nil, got %v", balances[1])
	}
}

// --- Multicall3 balance path ---

// Multicall3 deployed -> balances served by one aggregate3 call, order + nil-on-failure preserved.
func TestEthereumTypeGetErc20ContractBalancesViaMulticall3(t *testing.T) {
	addr := common.HexToAddress("0x0000000000000000000000000000000000000011")
	contractA := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	contractB := common.HexToAddress("0x00000000000000000000000000000000000000bb")
	contractC := common.HexToAddress("0x00000000000000000000000000000000000000cc")
	// A=100; B and C return empty data -> nil. All Success=true, so no re-resolve.
	fixture := fixtureAggregate3Result([]bchain.EthereumMulticallResult{
		{Success: true, Data: fmt.Sprintf("0x%064x", 100)},
		{Success: true, Data: "0x"},
		{Success: true, Data: "0x"},
	})
	mock := &mockMulticallRPC{
		handler: func(string) (string, error) { return fixture, nil },
		// nil getCodeHandler -> default "deployed" stub bytecode.
	}
	rpcClient := &EthereumRPC{RPC: mock, Timeout: time.Second}
	balances, err := rpcClient.EthereumTypeGetErc20ContractBalances(
		bchain.AddressDescriptor(addr.Bytes()),
		[]bchain.AddressDescriptor{
			bchain.AddressDescriptor(contractA.Bytes()),
			bchain.AddressDescriptor(contractB.Bytes()),
			bchain.AddressDescriptor(contractC.Bytes()),
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(balances) != 3 {
		t.Fatalf("expected 3 balances, got %d", len(balances))
	}
	if balances[0] == nil || balances[0].Cmp(big.NewInt(100)) != 0 {
		t.Fatalf("balance[0]=%v, want 100", balances[0])
	}
	if balances[1] != nil {
		t.Fatalf("balance[1]=%v, want nil (empty data)", balances[1])
	}
	if balances[2] != nil {
		t.Fatalf("balance[2]=%v, want nil (empty data)", balances[2])
	}
	// Exactly one probe + one aggregate3 eth_call; no per-token calls.
	ethCall, getCode := mock.callCounts()
	if getCode != 1 {
		t.Fatalf("expected 1 eth_getCode probe, got %d", getCode)
	}
	if ethCall != 1 {
		t.Fatalf("expected 1 aggregate3 eth_call (all balances in one call), got %d", ethCall)
	}
}

// mockMulticallThenBatchRPC: probe deployed; the aggregate3 eth_call returns
// aggregate3Resp when set (a successful batch, possibly with Success=false
// elements) else aggregate3Err (forcing a whole-chunk fallback). The re-resolve
// / fallback JSON-RPC batch is served by the embedded mockBatchRPC.
type mockMulticallThenBatchRPC struct {
	*mockBatchRPC
	aggregate3Err  error
	aggregate3Resp string
}

func (m *mockMulticallThenBatchRPC) CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error {
	switch method {
	case "eth_getCode":
		if out, ok := result.(*string); ok {
			*out = "0x6080" // deployed
		}
		return nil
	case "eth_call":
		argMap, _ := args[0].(map[string]interface{})
		to, _ := argMap["to"].(string)
		if strings.EqualFold(to, multicall3Address) {
			if m.aggregate3Resp != "" {
				*result.(*string) = m.aggregate3Resp
				return nil
			}
			return m.aggregate3Err
		}
		return fmt.Errorf("unexpected single eth_call to %s", to)
	default:
		return errors.New("unexpected method")
	}
}

// Transient aggregate3 failure -> JSON-RPC batch fallback, balances still correct + ordered.
func TestEthereumTypeGetErc20ContractBalancesFallsBackToBatchOnMulticallError(t *testing.T) {
	addr := common.HexToAddress("0x0000000000000000000000000000000000000011")
	contractA := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	contractB := common.HexToAddress("0x00000000000000000000000000000000000000bb")
	inner := &mockBatchRPC{
		results: map[string]string{
			hexutil.Encode(contractA.Bytes()): fmt.Sprintf("0x%064x", 7),
			hexutil.Encode(contractB.Bytes()): fmt.Sprintf("0x%064x", 9),
		},
	}
	mock := &mockMulticallThenBatchRPC{mockBatchRPC: inner, aggregate3Err: errors.New("aggregate3 transport boom")}
	rpcClient := &EthereumRPC{RPC: mock, Timeout: time.Second}
	balances, err := rpcClient.EthereumTypeGetErc20ContractBalances(
		bchain.AddressDescriptor(addr.Bytes()),
		[]bchain.AddressDescriptor{
			bchain.AddressDescriptor(contractA.Bytes()),
			bchain.AddressDescriptor(contractB.Bytes()),
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if balances[0] == nil || balances[0].Cmp(big.NewInt(7)) != 0 {
		t.Fatalf("balance[0]=%v, want 7", balances[0])
	}
	if balances[1] == nil || balances[1].Cmp(big.NewInt(9)) != 0 {
		t.Fatalf("balance[1]=%v, want 9", balances[1])
	}
	if len(inner.batchSizes) != 1 || inner.batchSizes[0] != 2 {
		t.Fatalf("expected one JSON-RPC batch of size 2, got %v", inner.batchSizes)
	}
}

// Wrong aggregate3 result count is a hard error (caller falls back), not a silent misalign.
func TestErc20BalancesMulticall3LengthMismatch(t *testing.T) {
	addr := common.HexToAddress("0x0000000000000000000000000000000000000011")
	contractA := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	contractB := common.HexToAddress("0x00000000000000000000000000000000000000bb")
	// One result returned for two calls.
	fixture := fixtureAggregate3Result([]bchain.EthereumMulticallResult{{Success: true, Data: "0x01"}})
	mock := &mockMulticallRPC{
		handler: func(string) (string, error) { return fixture, nil },
	}
	rpcClient := &EthereumRPC{RPC: mock, Timeout: time.Second}
	callData := erc20BalanceOfCallData(bchain.AddressDescriptor(addr.Bytes()))
	if _, _, err := rpcClient.erc20BalancesMulticall3(callData, []bchain.AddressDescriptor{
		bchain.AddressDescriptor(contractA.Bytes()),
		bchain.AddressDescriptor(contractB.Bytes()),
	}, nil); err == nil {
		t.Fatal("expected a length-mismatch error")
	}
}

// aggregate3 is chunked at multicall3MaxCallsPerAggregate (gas bound), independent of a larger
// erc20_batch_size (which sizes the JSON-RPC batch by request count).
func TestEthereumTypeGetErc20ContractBalancesMulticallChunkBounded(t *testing.T) {
	addr := common.HexToAddress("0x0000000000000000000000000000000000000011")
	n := multicall3MaxCallsPerAggregate + 20 // spills into a second aggregate3 chunk
	contracts := make([]bchain.AddressDescriptor, n)
	for i := range contracts {
		var a common.Address
		a[19] = byte((i + 1) & 0xff)
		a[18] = byte(((i + 1) >> 8) & 0xff)
		contracts[i] = bchain.AddressDescriptor(a.Bytes())
	}
	// Return one Success=true result per call in the received aggregate3 (array length at
	// byte selector(4)+outer-offset(32)), so each sub-chunk decodes cleanly.
	mock := &mockMulticallRPC{
		handler: func(callData string) (string, error) {
			raw, err := hexutil.Decode(callData)
			if err != nil {
				return "", err
			}
			cnt := int(bigUintAt(raw, 4+evmWordBytes).Uint64())
			res := make([]bchain.EthereumMulticallResult, cnt)
			for i := range res {
				res[i] = bchain.EthereumMulticallResult{Success: true, Data: fmt.Sprintf("0x%064x", 1)}
			}
			return fixtureAggregate3Result(res), nil
		},
	}
	rpcClient := &EthereumRPC{RPC: mock, Timeout: time.Second, ChainConfig: &Configuration{Erc20BatchSize: n}}
	balances, err := rpcClient.EthereumTypeGetErc20ContractBalances(bchain.AddressDescriptor(addr.Bytes()), contracts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(balances) != n {
		t.Fatalf("expected %d balances, got %d", n, len(balances))
	}
	for i, bal := range balances {
		if bal == nil || bal.Sign() != 1 {
			t.Fatalf("balance[%d]=%v, want 1", i, bal)
		}
	}
	ethCall, getCode := mock.callCounts()
	if getCode != 1 {
		t.Fatalf("expected 1 probe, got %d", getCode)
	}
	if ethCall != 2 {
		t.Fatalf("expected 2 aggregate3 calls (chunked at %d despite erc20_batch_size=%d), got %d", multicall3MaxCallsPerAggregate, n, ethCall)
	}
}

// A Success=false aggregate3 element (e.g. gas-starved under the shared budget) must be
// re-resolved via an independent eth_call, not silently reported as nil.
func TestEthereumTypeGetErc20ContractBalancesReresolvesFailedElement(t *testing.T) {
	addr := common.HexToAddress("0x0000000000000000000000000000000000000011")
	contractA := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	contractB := common.HexToAddress("0x00000000000000000000000000000000000000bb")
	contractC := common.HexToAddress("0x00000000000000000000000000000000000000cc")
	// aggregate3: A ok (=50), B Success=false (re-resolved -> 77), C Success=false + genuine revert (-> nil).
	agg := fixtureAggregate3Result([]bchain.EthereumMulticallResult{
		{Success: true, Data: fmt.Sprintf("0x%064x", 50)},
		{Success: false, Data: "0x"},
		{Success: false, Data: "0x"},
	})
	inner := &mockBatchRPC{
		results: map[string]string{hexutil.Encode(contractB.Bytes()): fmt.Sprintf("0x%064x", 77)},
		perErr:  map[string]error{hexutil.Encode(contractC.Bytes()): errors.New("execution reverted")},
	}
	mock := &mockMulticallThenBatchRPC{mockBatchRPC: inner, aggregate3Resp: agg}
	rpcClient := &EthereumRPC{RPC: mock, Timeout: time.Second}
	balances, err := rpcClient.EthereumTypeGetErc20ContractBalances(
		bchain.AddressDescriptor(addr.Bytes()),
		[]bchain.AddressDescriptor{
			bchain.AddressDescriptor(contractA.Bytes()),
			bchain.AddressDescriptor(contractB.Bytes()),
			bchain.AddressDescriptor(contractC.Bytes()),
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if balances[0] == nil || balances[0].Cmp(big.NewInt(50)) != 0 {
		t.Fatalf("balance[0]=%v, want 50 (from multicall)", balances[0])
	}
	if balances[1] == nil || balances[1].Cmp(big.NewInt(77)) != 0 {
		t.Fatalf("balance[1]=%v, want 77 (recovered via re-resolve, not nil)", balances[1])
	}
	if balances[2] != nil {
		t.Fatalf("balance[2]=%v, want nil (genuine revert stays nil)", balances[2])
	}
	// Re-resolve batch must cover only the two Success=false elements.
	if len(inner.batchSizes) != 1 || inner.batchSizes[0] != 2 {
		t.Fatalf("expected one re-resolve batch of size 2 (the failed subset), got %v", inner.batchSizes)
	}
}

// A Success=false element carrying revert returndata (e.g. Solidity Panic) is a genuine revert,
// never gas starvation, so it stays nil and is NOT re-resolved; only the empty-returndata
// failure is re-resolved.
func TestEthereumTypeGetErc20ContractBalancesGenuineRevertNotReresolved(t *testing.T) {
	addr := common.HexToAddress("0x0000000000000000000000000000000000000011")
	contractA := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	contractB := common.HexToAddress("0x00000000000000000000000000000000000000bb")
	contractC := common.HexToAddress("0x00000000000000000000000000000000000000cc")
	// A ok (=50), B empty revert (re-resolved -> 88), C Panic(0x11) with returndata (stays nil).
	panicData := "0x4e487b71" + fmt.Sprintf("%064x", 0x11)
	agg := fixtureAggregate3Result([]bchain.EthereumMulticallResult{
		{Success: true, Data: fmt.Sprintf("0x%064x", 50)},
		{Success: false, Data: "0x"},
		{Success: false, Data: panicData},
	})
	inner := &mockBatchRPC{
		results: map[string]string{hexutil.Encode(contractB.Bytes()): fmt.Sprintf("0x%064x", 88)},
	}
	mock := &mockMulticallThenBatchRPC{mockBatchRPC: inner, aggregate3Resp: agg}
	rpcClient := &EthereumRPC{RPC: mock, Timeout: time.Second}
	balances, err := rpcClient.EthereumTypeGetErc20ContractBalances(
		bchain.AddressDescriptor(addr.Bytes()),
		[]bchain.AddressDescriptor{
			bchain.AddressDescriptor(contractA.Bytes()),
			bchain.AddressDescriptor(contractB.Bytes()),
			bchain.AddressDescriptor(contractC.Bytes()),
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if balances[0] == nil || balances[0].Cmp(big.NewInt(50)) != 0 {
		t.Fatalf("balance[0]=%v, want 50", balances[0])
	}
	if balances[1] == nil || balances[1].Cmp(big.NewInt(88)) != 0 {
		t.Fatalf("balance[1]=%v, want 88 (empty-revert re-resolved)", balances[1])
	}
	if balances[2] != nil {
		t.Fatalf("balance[2]=%v, want nil (genuine revert not re-resolved)", balances[2])
	}
	// Only B is re-resolved: one batch of size 1, not 2.
	if len(inner.batchSizes) != 1 || inner.batchSizes[0] != 1 {
		t.Fatalf("expected one re-resolve batch of size 1 (only the empty-revert element), got %v", inner.batchSizes)
	}
}

// mockMulticallNoBatchRPC answers the probe (deployed) and one aggregate3 eth_call, but does
// NOT implement batchCaller, so the re-resolve path is unavailable.
type mockMulticallNoBatchRPC struct {
	aggregate3Resp string
}

func (m *mockMulticallNoBatchRPC) EthSubscribe(ctx context.Context, channel interface{}, args ...interface{}) (bchain.EVMClientSubscription, error) {
	return nil, errors.New("not implemented")
}
func (m *mockMulticallNoBatchRPC) Close() {}
func (m *mockMulticallNoBatchRPC) CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error {
	out, ok := result.(*string)
	if !ok {
		return errors.New("bad result type")
	}
	switch method {
	case "eth_getCode":
		*out = "0x6080" // deployed
		return nil
	case "eth_call":
		argMap, _ := args[0].(map[string]interface{})
		to, _ := argMap["to"].(string)
		if strings.EqualFold(to, multicall3Address) {
			*out = m.aggregate3Resp
			return nil
		}
		return fmt.Errorf("unexpected single eth_call to %s (no batcher available)", to)
	default:
		return errors.New("unexpected method")
	}
}

// Without a batcher, a potentially gas-starved (empty-returndata) Success=false element cannot be
// re-resolved and stays nil (best effort) rather than surfacing a wrong balance.
func TestEthereumTypeGetErc20ContractBalancesEmptyRevertWithoutBatcherStaysNil(t *testing.T) {
	addr := common.HexToAddress("0x0000000000000000000000000000000000000011")
	contractA := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	contractB := common.HexToAddress("0x00000000000000000000000000000000000000bb")
	agg := fixtureAggregate3Result([]bchain.EthereumMulticallResult{
		{Success: true, Data: fmt.Sprintf("0x%064x", 50)},
		{Success: false, Data: "0x"},
	})
	mock := &mockMulticallNoBatchRPC{aggregate3Resp: agg}
	rpcClient := &EthereumRPC{RPC: mock, Timeout: time.Second}
	balances, err := rpcClient.EthereumTypeGetErc20ContractBalances(
		bchain.AddressDescriptor(addr.Bytes()),
		[]bchain.AddressDescriptor{
			bchain.AddressDescriptor(contractA.Bytes()),
			bchain.AddressDescriptor(contractB.Bytes()),
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if balances[0] == nil || balances[0].Cmp(big.NewInt(50)) != 0 {
		t.Fatalf("balance[0]=%v, want 50", balances[0])
	}
	if balances[1] != nil {
		t.Fatalf("balance[1]=%v, want nil (no batcher to re-resolve)", balances[1])
	}
}
