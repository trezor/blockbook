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

// Wrong aggregate3 result count leaves the chunk's elements unresolved (caller falls back for
// them), never a silent misalign.
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
	_, _, unresolved := rpcClient.erc20BalancesMulticall3(callData, []bchain.AddressDescriptor{
		bchain.AddressDescriptor(contractA.Bytes()),
		bchain.AddressDescriptor(contractB.Bytes()),
	}, nil)
	if len(unresolved) != 2 || unresolved[0] != 0 || unresolved[1] != 1 {
		t.Fatalf("expected both elements unresolved on length mismatch, got %v", unresolved)
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

// mockMulticallNoBatchRPC answers the probe (deployed), one aggregate3 eth_call and plain single
// balanceOf eth_calls (from callResults/callErrors, keyed by target contract), but does NOT
// implement batchCaller, so re-resolve holes can only be settled with individual calls.
type mockMulticallNoBatchRPC struct {
	aggregate3Resp string
	callResults    map[string]string
	callErrors     map[string]error
	singleCalls    []rpcCall
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
		data, _ := argMap["data"].(string)
		m.singleCalls = append(m.singleCalls, rpcCall{to: to, data: data})
		if err, ok := m.callErrors[to]; ok {
			return err
		}
		res, ok := m.callResults[to]
		if !ok {
			return fmt.Errorf("unexpected single eth_call to %s (no batcher available)", to)
		}
		*out = res
		return nil
	default:
		return errors.New("unexpected method")
	}
}

// Without a batcher, a potentially gas-starved (empty-returndata) Success=false element is
// settled with an individual eth_call — single calls need no batcher — instead of being
// silently reported as an authoritative nil.
func TestEthereumTypeGetErc20ContractBalancesEmptyRevertWithoutBatcherRecoveredViaSingleCall(t *testing.T) {
	addr := common.HexToAddress("0x0000000000000000000000000000000000000011")
	contractA := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	contractB := common.HexToAddress("0x00000000000000000000000000000000000000bb")
	contractBKey := hexutil.Encode(contractB.Bytes())
	agg := fixtureAggregate3Result([]bchain.EthereumMulticallResult{
		{Success: true, Data: fmt.Sprintf("0x%064x", 50)},
		{Success: false, Data: "0x"},
	})
	mock := &mockMulticallNoBatchRPC{
		aggregate3Resp: agg,
		callResults:    map[string]string{contractBKey: fmt.Sprintf("0x%064x", 77)},
	}
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
	if balances[1] == nil || balances[1].Cmp(big.NewInt(77)) != 0 {
		t.Fatalf("balance[1]=%v, want 77 (recovered via single eth_call without a batcher)", balances[1])
	}
	// Only the empty-revert element is re-resolved, with the balanceOf calldata.
	if len(mock.singleCalls) != 1 || mock.singleCalls[0].to != contractBKey {
		t.Fatalf("expected 1 single re-resolve call to %s, got %+v", contractBKey, mock.singleCalls)
	}
	if callData := erc20BalanceOfCallData(bchain.AddressDescriptor(addr.Bytes())); mock.singleCalls[0].data != callData {
		t.Fatalf("expected single call data %q, got %q", callData, mock.singleCalls[0].data)
	}
}

// Companion: when the individual re-resolve call itself errors, the element stays nil — the same
// exhaustion behavior as the batch path's single-call fallback — and the rest is kept.
func TestEthereumTypeGetErc20ContractBalancesEmptyRevertWithoutBatcherSingleCallErrorStaysNil(t *testing.T) {
	addr := common.HexToAddress("0x0000000000000000000000000000000000000011")
	contractA := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	contractB := common.HexToAddress("0x00000000000000000000000000000000000000bb")
	agg := fixtureAggregate3Result([]bchain.EthereumMulticallResult{
		{Success: true, Data: fmt.Sprintf("0x%064x", 50)},
		{Success: false, Data: "0x"},
	})
	mock := &mockMulticallNoBatchRPC{
		aggregate3Resp: agg,
		callErrors:     map[string]error{hexutil.Encode(contractB.Bytes()): errors.New("still broken")},
	}
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
		t.Fatalf("balance[1]=%v, want nil (single re-resolve call failed)", balances[1])
	}
}

// mockMulticallChunkFailRPC: probe deployed; the first failAfter aggregate3 eth_calls succeed with
// one Success=true result per sub-call, and every later one fails. The fallback JSON-RPC batch is
// served by the embedded mockBatchRPC.
type mockMulticallChunkFailRPC struct {
	*mockBatchRPC
	failAfter int
	value     int64
	ethCalls  int
}

func (m *mockMulticallChunkFailRPC) CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error {
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
		if !strings.EqualFold(to, multicall3Address) {
			return fmt.Errorf("unexpected single eth_call to %s", to)
		}
		m.ethCalls++
		if m.ethCalls > m.failAfter {
			return errors.New("aggregate3 chunk out of gas")
		}
		data, _ := argMap["data"].(string)
		raw, err := hexutil.Decode(data)
		if err != nil {
			return err
		}
		// Array length sits at selector(4) + outer-offset(1 word).
		cnt := int(bigUintAt(raw, 4+evmWordBytes).Uint64())
		res := make([]bchain.EthereumMulticallResult, cnt)
		for i := range res {
			res[i] = bchain.EthereumMulticallResult{Success: true, Data: fmt.Sprintf("0x%064x", m.value)}
		}
		*out = fixtureAggregate3Result(res)
		return nil
	default:
		return errors.New("unexpected method")
	}
}

// One failed aggregate3 chunk must not discard the chunks that succeeded: only the failed chunk's
// contracts go to the JSON-RPC batch fallback, never the whole list (which would cost more metered
// calls than never using multicall at all).
func TestEthereumTypeGetErc20ContractBalancesPartialChunkFailureFallsBackForThatChunkOnly(t *testing.T) {
	addr := common.HexToAddress("0x0000000000000000000000000000000000000011")
	n := multicall3MaxCallsPerAggregate + 20 // chunk 1 = 100 contracts, chunk 2 = 20
	contracts := make([]bchain.AddressDescriptor, n)
	for i := range contracts {
		var a common.Address
		a[19] = byte((i + 1) & 0xff)
		a[18] = byte(((i + 1) >> 8) & 0xff)
		contracts[i] = bchain.AddressDescriptor(a.Bytes())
	}
	// Only the second chunk's contracts are resolvable via the batch, so a whole-list fallback
	// would leave the first chunk nil instead of keeping its aggregate3 balances.
	inner := &mockBatchRPC{results: map[string]string{}}
	for _, c := range contracts[multicall3MaxCallsPerAggregate:] {
		inner.results[hexutil.Encode(c)] = fmt.Sprintf("0x%064x", 5)
	}
	mock := &mockMulticallChunkFailRPC{mockBatchRPC: inner, failAfter: 1, value: 1}
	rpcClient := &EthereumRPC{RPC: mock, Timeout: time.Second}
	balances, err := rpcClient.EthereumTypeGetErc20ContractBalances(bchain.AddressDescriptor(addr.Bytes()), contracts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(balances) != n {
		t.Fatalf("expected %d balances, got %d", n, len(balances))
	}
	// The successful chunk keeps its aggregate3 results.
	for i := 0; i < multicall3MaxCallsPerAggregate; i++ {
		if balances[i] == nil || balances[i].Cmp(big.NewInt(1)) != 0 {
			t.Fatalf("balance[%d]=%v, want 1 from the successful aggregate3 chunk", i, balances[i])
		}
	}
	// The failed chunk is recovered through the batch.
	for i := multicall3MaxCallsPerAggregate; i < n; i++ {
		if balances[i] == nil || balances[i].Cmp(big.NewInt(5)) != 0 {
			t.Fatalf("balance[%d]=%v, want 5 from the batch fallback", i, balances[i])
		}
	}
	if want := n - multicall3MaxCallsPerAggregate; len(inner.batchSizes) != 1 || inner.batchSizes[0] != want {
		t.Fatalf("expected one fallback batch of %d (the failed chunk only), got %v", want, inner.batchSizes)
	}
	if mock.ethCalls != 2 {
		t.Fatalf("expected 2 aggregate3 calls, got %d", mock.ethCalls)
	}
}

// Distinct per-index balances, so a fallback result written back to the wrong contract is visible.
func erc20MulticallTestValue(i int) int64 { return int64(1000 + i) }
func erc20BatchTestValue(i int) int64     { return int64(500000 + i) }

func erc20TestContracts(n int) []bchain.AddressDescriptor {
	contracts := make([]bchain.AddressDescriptor, n)
	for i := range contracts {
		var a common.Address
		a[19] = byte((i + 1) & 0xff)
		a[18] = byte(((i + 1) >> 8) & 0xff)
		contracts[i] = bchain.AddressDescriptor(a.Bytes())
	}
	return contracts
}

// A systemic aggregate3 failure must cost one doomed eth_call, not one per remaining chunk.
func TestEthereumTypeGetErc20ContractBalancesSystemicChunkFailureStopsAtFirstChunk(t *testing.T) {
	addr := common.HexToAddress("0x0000000000000000000000000000000000000011")
	n := 3 * multicall3MaxCallsPerAggregate // 3 aggregate3 chunks
	contracts := erc20TestContracts(n)
	inner := &mockBatchRPC{results: map[string]string{}}
	for i, c := range contracts {
		inner.results[hexutil.Encode(c)] = fmt.Sprintf("0x%064x", erc20BatchTestValue(i))
	}
	// failAfter 0: every aggregate3 call fails, as on a chain whose eth_call gas cap is too low.
	mock := &mockMulticallChunkFailRPC{mockBatchRPC: inner, failAfter: 0}
	rpcClient := &EthereumRPC{RPC: mock, Timeout: time.Second}
	balances, err := rpcClient.EthereumTypeGetErc20ContractBalances(bchain.AddressDescriptor(addr.Bytes()), contracts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i := range contracts {
		if balances[i] == nil || balances[i].Cmp(big.NewInt(erc20BatchTestValue(i))) != 0 {
			t.Fatalf("balance[%d]=%v, want %d from the batch fallback", i, balances[i], erc20BatchTestValue(i))
		}
	}
	// The regression: 1 failing aggregate3, not one per chunk.
	if mock.ethCalls != 1 {
		t.Fatalf("expected 1 aggregate3 call before giving up, got %d", mock.ethCalls)
	}
	// Nothing resolved, so the whole list goes through the normal chunked batch path.
	if len(inner.batchSizes) != 3 {
		t.Fatalf("expected 3 batch chunks covering the whole list, got %v", inner.batchSizes)
	}
}

// mockMulticallMixedRPC: probe deployed. aggregate3 calls are served in input order from a running
// offset, each element carrying a value derived from its global index so a mis-mapped write-back
// shows up as a wrong balance. The element at holeAt comes back Success=false with empty returndata
// (a reresolve candidate) and the chunk starting at failFrom fails outright, so one request yields
// both a reresolve and an unresolved index set. The fallback batch is served by mockBatchRPC.
type mockMulticallMixedRPC struct {
	*mockBatchRPC
	holeAt   int
	failFrom int
	offset   int
	ethCalls int
}

func (m *mockMulticallMixedRPC) CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error {
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
		if !strings.EqualFold(to, multicall3Address) {
			return fmt.Errorf("unexpected single eth_call to %s", to)
		}
		m.ethCalls++
		if m.offset >= m.failFrom {
			return errors.New("aggregate3 chunk out of gas")
		}
		data, _ := argMap["data"].(string)
		raw, err := hexutil.Decode(data)
		if err != nil {
			return err
		}
		cnt := int(bigUintAt(raw, 4+evmWordBytes).Uint64())
		res := make([]bchain.EthereumMulticallResult, cnt)
		for i := range res {
			if g := m.offset + i; g == m.holeAt {
				res[i] = bchain.EthereumMulticallResult{Success: false, Data: "0x"}
			} else {
				res[i] = bchain.EthereumMulticallResult{Success: true, Data: fmt.Sprintf("0x%064x", erc20MulticallTestValue(g))}
			}
		}
		m.offset += cnt
		*out = fixtureAggregate3Result(res)
		return nil
	default:
		return errors.New("unexpected method")
	}
}

// A request carrying both a reresolve hole and a failed chunk must merge the two index sets into
// one fallback batch and write every result back to its own contract.
func TestEthereumTypeGetErc20ContractBalancesMixedReresolveAndUnresolved(t *testing.T) {
	addr := common.HexToAddress("0x0000000000000000000000000000000000000011")
	n := multicall3MaxCallsPerAggregate + 20 // chunk 1 succeeds with a hole, chunk 2 fails
	holeAt := 3
	contracts := erc20TestContracts(n)
	// The batch may only answer for the hole and the failed chunk; anything else means the
	// fallback subset was built wrong.
	inner := &mockBatchRPC{results: map[string]string{
		hexutil.Encode(contracts[holeAt]): fmt.Sprintf("0x%064x", erc20BatchTestValue(holeAt)),
	}}
	for i := multicall3MaxCallsPerAggregate; i < n; i++ {
		inner.results[hexutil.Encode(contracts[i])] = fmt.Sprintf("0x%064x", erc20BatchTestValue(i))
	}
	mock := &mockMulticallMixedRPC{mockBatchRPC: inner, holeAt: holeAt, failFrom: multicall3MaxCallsPerAggregate}
	rpcClient := &EthereumRPC{RPC: mock, Timeout: time.Second}
	balances, err := rpcClient.EthereumTypeGetErc20ContractBalances(bchain.AddressDescriptor(addr.Bytes()), contracts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(balances) != n {
		t.Fatalf("expected %d balances, got %d", n, len(balances))
	}
	for i := range contracts {
		want := erc20MulticallTestValue(i)
		source := "aggregate3"
		if i == holeAt || i >= multicall3MaxCallsPerAggregate {
			want, source = erc20BatchTestValue(i), "fallback batch"
		}
		if balances[i] == nil || balances[i].Cmp(big.NewInt(want)) != 0 {
			t.Fatalf("balance[%d]=%v, want %d from the %s", i, balances[i], want, source)
		}
	}
	// One batch covering the reresolve hole plus the failed chunk, nothing else.
	if want := 1 + n - multicall3MaxCallsPerAggregate; len(inner.batchSizes) != 1 || inner.batchSizes[0] != want {
		t.Fatalf("expected one fallback batch of %d (hole + failed chunk), got %v", want, inner.batchSizes)
	}
	if mock.ethCalls != 2 {
		t.Fatalf("expected 2 aggregate3 calls, got %d", mock.ethCalls)
	}
}

// mockMulticallNoBatchChunkFailRPC: probe deployed, the first aggregate3 chunk succeeds and the
// next fails, and the client does NOT implement batchCaller, so unresolved holes cannot be filled.
type mockMulticallNoBatchChunkFailRPC struct {
	offset   int
	failFrom int
	ethCalls int
}

func (m *mockMulticallNoBatchChunkFailRPC) EthSubscribe(ctx context.Context, channel interface{}, args ...interface{}) (bchain.EVMClientSubscription, error) {
	return nil, errors.New("not implemented")
}
func (m *mockMulticallNoBatchChunkFailRPC) Close() {}
func (m *mockMulticallNoBatchChunkFailRPC) CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error {
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
		if !strings.EqualFold(to, multicall3Address) {
			return fmt.Errorf("unexpected single eth_call to %s (no batcher available)", to)
		}
		m.ethCalls++
		if m.offset >= m.failFrom {
			return errors.New("aggregate3 chunk out of gas")
		}
		data, _ := argMap["data"].(string)
		raw, err := hexutil.Decode(data)
		if err != nil {
			return err
		}
		cnt := int(bigUintAt(raw, 4+evmWordBytes).Uint64())
		res := make([]bchain.EthereumMulticallResult, cnt)
		for i := range res {
			res[i] = bchain.EthereumMulticallResult{Success: true, Data: fmt.Sprintf("0x%064x", erc20MulticallTestValue(m.offset+i))}
		}
		m.offset += cnt
		*out = fixtureAggregate3Result(res)
		return nil
	default:
		return errors.New("unexpected method")
	}
}

// A failed chunk with no batcher to fill the holes must surface an error so the caller falls back
// to single calls. Returning the partial result would report unknown balances as nil, and callers
// treat a present entry as authoritative.
func TestEthereumTypeGetErc20ContractBalancesChunkFailureWithoutBatcherErrors(t *testing.T) {
	addr := common.HexToAddress("0x0000000000000000000000000000000000000011")
	n := multicall3MaxCallsPerAggregate + 20 // chunk 1 succeeds, chunk 2 fails
	contracts := erc20TestContracts(n)
	mock := &mockMulticallNoBatchChunkFailRPC{failFrom: multicall3MaxCallsPerAggregate}
	rpcClient := &EthereumRPC{RPC: mock, Timeout: time.Second}
	balances, err := rpcClient.EthereumTypeGetErc20ContractBalances(bchain.AddressDescriptor(addr.Bytes()), contracts)
	if err == nil {
		t.Fatalf("expected an error so the caller uses single calls, got %d balances", len(balances))
	}
	if balances != nil {
		t.Fatalf("expected nil balances: unknown holes must not be returned as nil entries, got %d", len(balances))
	}
	if mock.ethCalls != 2 {
		t.Fatalf("expected 2 aggregate3 calls (second fails), got %d", mock.ethCalls)
	}
}

// A malformed (non-20-byte) contract descriptor must not poison aggregate3 for the whole request:
// it is excluded up front and stays nil (as in the batch path, where a bogus `to` yields an
// element error and thus nil), while the valid contracts resolve in one aggregate3 call with no
// batch fallback.
func TestEthereumTypeGetErc20ContractBalancesMalformedDescriptorExcluded(t *testing.T) {
	addr := common.HexToAddress("0x0000000000000000000000000000000000000011")
	contractA := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	contractC := common.HexToAddress("0x00000000000000000000000000000000000000cc")
	badDesc := make(bchain.AddressDescriptor, 21) // 21 bytes: not an address
	var subCallCounts []int
	mock := &mockMulticallRPC{
		handler: func(callData string) (string, error) {
			raw, err := hexutil.Decode(callData)
			if err != nil {
				return "", err
			}
			// Array length sits at selector(4) + outer-offset(1 word).
			cnt := int(bigUintAt(raw, 4+evmWordBytes).Uint64())
			subCallCounts = append(subCallCounts, cnt)
			res := make([]bchain.EthereumMulticallResult, cnt)
			for i := range res {
				res[i] = bchain.EthereumMulticallResult{Success: true, Data: fmt.Sprintf("0x%064x", 100*(i+1))}
			}
			return fixtureAggregate3Result(res), nil
		},
	}
	rpcClient := &EthereumRPC{RPC: mock, Timeout: time.Second}
	balances, err := rpcClient.EthereumTypeGetErc20ContractBalances(
		bchain.AddressDescriptor(addr.Bytes()),
		[]bchain.AddressDescriptor{
			bchain.AddressDescriptor(contractA.Bytes()),
			badDesc,
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
		t.Fatalf("balance[0]=%v, want 100 (first valid contract)", balances[0])
	}
	if balances[1] != nil {
		t.Fatalf("balance[1]=%v, want nil (malformed descriptor)", balances[1])
	}
	if balances[2] == nil || balances[2].Cmp(big.NewInt(200)) != 0 {
		t.Fatalf("balance[2]=%v, want 200 (second valid contract)", balances[2])
	}
	// Exactly one aggregate3 covering only the two valid contracts; no batch fallback (which
	// would show up as extra eth_calls against this mock).
	if len(subCallCounts) != 1 || subCallCounts[0] != 2 {
		t.Fatalf("expected one aggregate3 with 2 sub-calls (the valid contracts), got %v", subCallCounts)
	}
	ethCall, _ := mock.callCounts()
	if ethCall != 1 {
		t.Fatalf("expected 1 aggregate3 eth_call and no fallback, got %d", ethCall)
	}
}

// --- erc20 multicall circuit breaker ---

// While the breaker is open, no aggregate3 eth_call is issued: the request goes straight to the
// JSON-RPC batch path.
func TestEthereumTypeGetErc20ContractBalancesSuspendedGoesStraightToBatch(t *testing.T) {
	addr := common.HexToAddress("0x0000000000000000000000000000000000000011")
	contracts := erc20TestContracts(3)
	inner := &mockBatchRPC{results: map[string]string{}}
	for i, c := range contracts {
		inner.results[hexutil.Encode(c)] = fmt.Sprintf("0x%064x", erc20BatchTestValue(i))
	}
	mock := &mockMulticallChunkFailRPC{mockBatchRPC: inner, failAfter: 0}
	rpcClient := &EthereumRPC{RPC: mock, Timeout: time.Second}
	rpcClient.erc20MulticallSuspendedUntil.Store(time.Now().Add(time.Hour).UnixNano())
	balances, err := rpcClient.EthereumTypeGetErc20ContractBalances(bchain.AddressDescriptor(addr.Bytes()), contracts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i := range contracts {
		if balances[i] == nil || balances[i].Cmp(big.NewInt(erc20BatchTestValue(i))) != 0 {
			t.Fatalf("balance[%d]=%v, want %d from the batch path", i, balances[i], erc20BatchTestValue(i))
		}
	}
	if mock.ethCalls != 0 {
		t.Fatalf("expected 0 aggregate3 calls while suspended, got %d", mock.ethCalls)
	}
	if len(inner.batchSizes) != 1 {
		t.Fatalf("expected one JSON-RPC batch, got %v", inner.batchSizes)
	}
}

// After erc20MulticallMaxConsecutiveFailures requests in which aggregate3 resolved nothing, the
// suspension deadline is set (counter reset) and the next request issues no aggregate3 call.
func TestEthereumTypeGetErc20ContractBalancesBreakerSuspendsAfterConsecutiveFailures(t *testing.T) {
	addr := common.HexToAddress("0x0000000000000000000000000000000000000011")
	contracts := erc20TestContracts(3)
	inner := &mockBatchRPC{results: map[string]string{}}
	for i, c := range contracts {
		inner.results[hexutil.Encode(c)] = fmt.Sprintf("0x%064x", erc20BatchTestValue(i))
	}
	// failAfter 0: every aggregate3 call fails, as on a chain whose eth_call gas cap is too low.
	mock := &mockMulticallChunkFailRPC{mockBatchRPC: inner, failAfter: 0}
	rpcClient := &EthereumRPC{RPC: mock, Timeout: time.Second}
	for r := 0; r < erc20MulticallMaxConsecutiveFailures; r++ {
		if _, err := rpcClient.EthereumTypeGetErc20ContractBalances(bchain.AddressDescriptor(addr.Bytes()), contracts); err != nil {
			t.Fatalf("request %d: unexpected error: %v", r, err)
		}
	}
	if mock.ethCalls != erc20MulticallMaxConsecutiveFailures {
		t.Fatalf("expected %d doomed aggregate3 calls (one per request), got %d", erc20MulticallMaxConsecutiveFailures, mock.ethCalls)
	}
	if until := rpcClient.erc20MulticallSuspendedUntil.Load(); until == 0 || until <= time.Now().UnixNano() {
		t.Fatalf("expected a suspension deadline in the future, got %d", until)
	}
	if got := rpcClient.erc20MulticallFailures.Load(); got != 0 {
		t.Fatalf("expected failure counter reset to 0 on suspension, got %d", got)
	}
	// The next request must not pay another doomed aggregate3.
	if _, err := rpcClient.EthereumTypeGetErc20ContractBalances(bchain.AddressDescriptor(addr.Bytes()), contracts); err != nil {
		t.Fatalf("suspended request: unexpected error: %v", err)
	}
	if mock.ethCalls != erc20MulticallMaxConsecutiveFailures {
		t.Fatalf("expected no aggregate3 call while suspended, got %d total", mock.ethCalls)
	}
}

// A request in which aggregate3 resolves anything closes the breaker again: the consecutive
// failure counter goes back to 0.
func TestEthereumTypeGetErc20ContractBalancesBreakerResetsOnSuccess(t *testing.T) {
	addr := common.HexToAddress("0x0000000000000000000000000000000000000011")
	contracts := erc20TestContracts(2)
	mock := &mockMulticallChunkFailRPC{mockBatchRPC: &mockBatchRPC{}, failAfter: 1000, value: 9}
	rpcClient := &EthereumRPC{RPC: mock, Timeout: time.Second}
	rpcClient.erc20MulticallFailures.Store(erc20MulticallMaxConsecutiveFailures - 1)
	balances, err := rpcClient.EthereumTypeGetErc20ContractBalances(bchain.AddressDescriptor(addr.Bytes()), contracts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i := range contracts {
		if balances[i] == nil || balances[i].Cmp(big.NewInt(9)) != 0 {
			t.Fatalf("balance[%d]=%v, want 9 from aggregate3", i, balances[i])
		}
	}
	if got := rpcClient.erc20MulticallFailures.Load(); got != 0 {
		t.Fatalf("expected failure counter reset to 0 after a resolving request, got %d", got)
	}
	if until := rpcClient.erc20MulticallSuspendedUntil.Load(); until != 0 {
		t.Fatalf("expected no suspension deadline, got %d", until)
	}
}
