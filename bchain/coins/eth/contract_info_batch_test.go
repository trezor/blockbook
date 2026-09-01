//go:build unittest

package eth

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/trezor/blockbook/bchain"
)

// abiString encodes s the way a solidity `returns (string)` getter would.
func abiString(s string) string {
	data := hexutil.Encode([]byte(s))[2:]
	for len(data)%64 != 0 {
		data += "00"
	}
	return "0x" + padHex32("20") + padHex32(fmt.Sprintf("%x", len(s))) + data
}

func selectorOf(callData string) string {
	if len(callData) < 10 {
		return callData
	}
	return callData[:10]
}

// mockContractInfoRPC answers the deployment probe, aggregate3 calls to the Multicall3
// address and single eth_calls to individual contracts, recording each of them.
type mockContractInfoRPC struct {
	mu sync.Mutex
	// code is the eth_getCode answer for the probe; "0x" reports Multicall3 as not deployed.
	code            string
	aggregate3      func(callData string) (string, error)
	single          func(to, selector string) (string, error)
	aggregate3Calls int
	// singles are the per-contract eth_calls as "to|selector", in call order.
	singles []string
}

func (m *mockContractInfoRPC) singleCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.singles...)
}

func (m *mockContractInfoRPC) EthSubscribe(ctx context.Context, channel interface{}, args ...interface{}) (bchain.EVMClientSubscription, error) {
	return nil, errors.New("not implemented")
}
func (m *mockContractInfoRPC) Close() {}
func (m *mockContractInfoRPC) BatchCallContext(ctx context.Context, batch []rpc.BatchElem) error {
	return errors.New("not implemented")
}

func (m *mockContractInfoRPC) CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error {
	out, ok := result.(*string)
	if !ok {
		return errors.New("bad result type")
	}
	switch method {
	case "eth_getCode":
		code := m.code
		if code == "" {
			code = "0x6080604052"
		}
		*out = code
		return nil
	case "eth_call":
		argMap, _ := args[0].(map[string]interface{})
		to, _ := argMap["to"].(string)
		data, _ := argMap["data"].(string)
		if strings.EqualFold(to, multicall3Address) {
			m.mu.Lock()
			m.aggregate3Calls++
			m.mu.Unlock()
			if m.aggregate3 == nil {
				return errors.New("no aggregate3 handler installed")
			}
			resp, err := m.aggregate3(data)
			if err != nil {
				return err
			}
			*out = resp
			return nil
		}
		m.mu.Lock()
		m.singles = append(m.singles, to+"|"+selectorOf(data))
		m.mu.Unlock()
		if m.single == nil {
			return errors.New("no single eth_call handler installed")
		}
		resp, err := m.single(to, selectorOf(data))
		if err != nil {
			return err
		}
		*out = resp
		return nil
	default:
		return fmt.Errorf("unexpected method: %s", method)
	}
}

// tokenMetadataSingle answers the sequential probe; a contract not in tokens reverts on name(), the
// way a non-token contract does.
func tokenMetadataSingle(tokens map[string]string) func(to, selector string) (string, error) {
	return func(to, selector string) (string, error) {
		name, ok := tokens[strings.ToLower(to)]
		if !ok {
			return "", errors.New("execution reverted")
		}
		switch selector {
		case contractNameSignature:
			return abiString(name), nil
		case contractSymbolSignature:
			return abiString(strings.ToUpper(name[:1])), nil
		case contractDecimalsSignature:
			return abiWord(18), nil
		}
		return "", fmt.Errorf("unexpected selector %s", selector)
	}
}

var (
	contractInfoA = common.HexToAddress("0x00000000000000000000000000000000000000aa")
	contractInfoB = common.HexToAddress("0x00000000000000000000000000000000000000bb")
	contractInfoC = common.HexToAddress("0x00000000000000000000000000000000000000cc")
)

func descs(addrs ...common.Address) []bchain.AddressDescriptor {
	out := make([]bchain.AddressDescriptor, len(addrs))
	for i, a := range addrs {
		out[i] = bchain.AddressDescriptor(a.Bytes())
	}
	return out
}

// One aggregate3 call resolves every contract; a reverting name() comes back as a conclusive
// "not a token" (nil info, nil error) rather than an error.
func TestEthereumTypeGetContractInfos_OneMulticall(t *testing.T) {
	fixture := fixtureAggregate3Result([]bchain.EthereumMulticallResult{
		{Success: true, Data: abiString("Token A")},
		{Success: true, Data: abiString("TKA")},
		{Success: true, Data: abiWord(6)},
		// B implements none of the three - the ERC1155 / dead contract shape
		{Success: false, Data: "0x"},
		{Success: false, Data: "0x"},
		{Success: false, Data: "0x"},
		// canary
		{Success: true, Data: "0x"},
	})
	mock := &mockContractInfoRPC{aggregate3: func(string) (string, error) { return fixture, nil }}
	rpcClient := &EthereumRPC{RPC: mock, Timeout: time.Second}

	got := rpcClient.EthereumTypeGetContractInfos(descs(contractInfoA, contractInfoB))

	if len(got) != 2 {
		t.Fatalf("got %d results, want 2", len(got))
	}
	if got[0].Err != nil || got[0].Info == nil {
		t.Fatalf("result[0] = %+v, want a resolved token", got[0])
	}
	want := bchain.ContractInfo{
		Contract: EIP55Address(contractInfoA.Bytes()),
		Name:     "Token A",
		Symbol:   "TKA",
		Decimals: 6,
	}
	if *got[0].Info != want {
		t.Fatalf("result[0].Info = %+v, want %+v", *got[0].Info, want)
	}
	if got[1].Info != nil || got[1].Err != nil {
		t.Fatalf("result[1] = %+v, want a conclusive not-a-token", got[1])
	}
	if mock.aggregate3Calls != 1 {
		t.Fatalf("aggregate3 calls = %d, want 1", mock.aggregate3Calls)
	}
	if calls := mock.singleCalls(); len(calls) != 0 {
		t.Fatalf("single eth_calls = %v, want none", calls)
	}
}

// Without Multicall3 the sequential probe still short-circuits: a non-token costs one eth_call.
func TestEthereumTypeGetContractInfos_SequentialWithoutMulticall3(t *testing.T) {
	mock := &mockContractInfoRPC{
		code:   "0x",
		single: tokenMetadataSingle(map[string]string{strings.ToLower(contractInfoA.Hex()): "alpha"}),
	}
	rpcClient := &EthereumRPC{RPC: mock, Timeout: time.Second}

	got := rpcClient.EthereumTypeGetContractInfos(descs(contractInfoA, contractInfoB))

	if got[0].Info == nil || got[0].Info.Name != "alpha" {
		t.Fatalf("result[0] = %+v, want the resolved token", got[0])
	}
	if got[1].Info != nil || got[1].Err != nil {
		t.Fatalf("result[1] = %+v, want a conclusive not-a-token", got[1])
	}
	if mock.aggregate3Calls != 0 {
		t.Fatalf("aggregate3 calls = %d, want 0", mock.aggregate3Calls)
	}
	wantCalls := []string{
		contractInfoA.Hex() + "|" + contractNameSignature,
		contractInfoA.Hex() + "|" + contractSymbolSignature,
		contractInfoA.Hex() + "|" + contractDecimalsSignature,
		contractInfoB.Hex() + "|" + contractNameSignature,
	}
	if got := mock.singleCalls(); !slices.Equal(got, wantCalls) {
		t.Fatalf("single eth_calls = %v, want %v", got, wantCalls)
	}
}

// A failing canary means the chunk ran out of gas, so an empty failure may be starvation rather than
// a revert; those contracts are re-read instead of being called not-a-token.
func TestEthereumTypeGetContractInfos_StarvedChunkReReads(t *testing.T) {
	fixture := fixtureAggregate3Result([]bchain.EthereumMulticallResult{
		{Success: false, Data: "0x"},
		{Success: false, Data: "0x"},
		{Success: false, Data: "0x"},
		{Success: true, Data: abiString("Token B")},
		{Success: true, Data: abiString("TKB")},
		{Success: true, Data: abiWord(8)},
		// canary out of gas
		{Success: false, Data: "0x"},
	})
	mock := &mockContractInfoRPC{
		aggregate3: func(string) (string, error) { return fixture, nil },
		single:     tokenMetadataSingle(map[string]string{strings.ToLower(contractInfoA.Hex()): "alpha"}),
	}
	rpcClient := &EthereumRPC{RPC: mock, Timeout: time.Second}

	got := rpcClient.EthereumTypeGetContractInfos(descs(contractInfoA, contractInfoB))

	if got[0].Info == nil || got[0].Info.Name != "alpha" {
		t.Fatalf("result[0] = %+v, want the starved element re-read as a token", got[0])
	}
	// the settled element keeps the batched answer, no second read
	if got[1].Info == nil || got[1].Info.Name != "Token B" || got[1].Info.Decimals != 8 {
		t.Fatalf("result[1] = %+v, want the batched token", got[1])
	}
	if calls := mock.singleCalls(); len(calls) != 3 {
		t.Fatalf("single eth_calls = %v, want the three reads of contract A only", calls)
	}
}

// A whole-chunk failure is systemic: the contracts it covered are re-read sequentially.
func TestEthereumTypeGetContractInfos_ChunkErrorFallsBack(t *testing.T) {
	mock := &mockContractInfoRPC{
		aggregate3: func(string) (string, error) { return "", errors.New("aggregate3 transport boom") },
		single:     tokenMetadataSingle(map[string]string{strings.ToLower(contractInfoB.Hex()): "beta"}),
	}
	rpcClient := &EthereumRPC{RPC: mock, Timeout: time.Second}

	got := rpcClient.EthereumTypeGetContractInfos(descs(contractInfoA, contractInfoB))

	if got[0].Info != nil || got[0].Err != nil {
		t.Fatalf("result[0] = %+v, want a conclusive not-a-token", got[0])
	}
	if got[1].Info == nil || got[1].Info.Name != "beta" {
		t.Fatalf("result[1] = %+v, want the resolved token", got[1])
	}
}

// Multicall3MaxCalls bounds sub-calls, so a chunk holds (max-1)/3 contracts: three metadata reads
// each, plus the canary.
func TestEthereumTypeGetContractInfos_ChunksByConfiguredMaxCalls(t *testing.T) {
	full := func(name string) []bchain.EthereumMulticallResult {
		return []bchain.EthereumMulticallResult{
			{Success: true, Data: abiString(name)},
			{Success: true, Data: abiString("SYM")},
			{Success: true, Data: abiWord(18)},
		}
	}
	canary := bchain.EthereumMulticallResult{Success: true, Data: "0x"}
	responses := []string{
		fixtureAggregate3Result(append(append(full("one"), full("two")...), canary)),
		fixtureAggregate3Result(append(full("three"), canary)),
	}
	mock := &mockContractInfoRPC{}
	mock.aggregate3 = func(string) (string, error) {
		resp := responses[mock.aggregate3Calls-1]
		return resp, nil
	}
	rpcClient := &EthereumRPC{
		RPC:     mock,
		Timeout: time.Second,
		// two contracts (three reads each) plus the canary
		ChainConfig: &Configuration{Multicall3MaxCalls: 7},
	}

	got := rpcClient.EthereumTypeGetContractInfos(descs(contractInfoA, contractInfoB, contractInfoC))

	if mock.aggregate3Calls != 2 {
		t.Fatalf("aggregate3 calls = %d, want 2 (two contracts per chunk)", mock.aggregate3Calls)
	}
	for i, wantName := range []string{"one", "two", "three"} {
		if got[i].Info == nil || got[i].Info.Name != wantName {
			t.Fatalf("result[%d] = %+v, want name %q", i, got[i], wantName)
		}
	}
}

// The batched decode has to reach the same verdict the three sequential eth_calls would.
func Test_contractInfoFromAggregate3(t *testing.T) {
	ok := func(data string) bchain.EthereumMulticallResult {
		return bchain.EthereumMulticallResult{Success: true, Data: data}
	}
	failed := bchain.EthereumMulticallResult{Success: false, Data: "0x"}
	bytes32Name := "0x" + padHex32(hexutil.Encode([]byte("MKR"))[2:]+strings.Repeat("00", 29))

	tests := []struct {
		name  string
		slots []bchain.EthereumMulticallResult
		want  *bchain.ContractInfo
	}{
		{
			name:  "resolved token",
			slots: []bchain.EthereumMulticallResult{ok(abiString("Token")), ok(abiString("TKN")), ok(abiWord(6))},
			want:  &bchain.ContractInfo{Contract: "0xC", Name: "Token", Symbol: "TKN", Decimals: 6},
		},
		{
			name:  "bytes32 name and symbol",
			slots: []bchain.EthereumMulticallResult{ok(bytes32Name), ok(bytes32Name), ok(abiWord(18))},
			want:  &bchain.ContractInfo{Contract: "0xC", Name: "MKR", Symbol: "MKR", Decimals: 18},
		},
		{
			name:  "empty name is not a token",
			slots: []bchain.EthereumMulticallResult{ok("0x"), ok(abiString("TKN")), ok(abiWord(6))},
			want:  nil,
		},
		{
			name:  "reverting name is not a token",
			slots: []bchain.EthereumMulticallResult{failed, ok(abiString("TKN")), ok(abiWord(6))},
			want:  nil,
		},
		{
			name:  "reverting symbol is not a token",
			slots: []bchain.EthereumMulticallResult{ok(abiString("Token")), failed, ok(abiWord(6))},
			want:  nil,
		},
		{
			name:  "reverting decimals falls back to the coin default",
			slots: []bchain.EthereumMulticallResult{ok(abiString("Token")), ok(abiString("TKN")), failed},
			want:  &bchain.ContractInfo{Contract: "0xC", Name: "Token", Symbol: "TKN", Decimals: EtherAmountDecimalPoint},
		},
		{
			name:  "unparseable decimals falls back to the coin default",
			slots: []bchain.EthereumMulticallResult{ok(abiString("Token")), ok(abiString("TKN")), ok("0x")},
			want:  &bchain.ContractInfo{Contract: "0xC", Name: "Token", Symbol: "TKN", Decimals: EtherAmountDecimalPoint},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := contractInfoFromAggregate3("0xC", tt.slots)
			if got.Err != nil {
				t.Fatalf("unexpected error %v", got.Err)
			}
			if tt.want == nil {
				if got.Info != nil {
					t.Fatalf("got %+v, want a conclusive not-a-token", *got.Info)
				}
				return
			}
			if got.Info == nil || *got.Info != *tt.want {
				t.Fatalf("got %+v, want %+v", got.Info, *tt.want)
			}
		})
	}
}

// The (info, err) pair is what tells a cacheable "not a token" from a read that simply failed, so
// every eth_call outcome has to land on the right side.
func Test_fetchContractInfo_Verdicts(t *testing.T) {
	tests := []struct {
		name      string
		errors    map[string]error
		wantInfo  *bchain.ContractInfo
		wantErr   bool
		wantCalls int
	}{
		{
			name:      "reverting name is a conclusive not-a-token",
			errors:    map[string]error{contractNameSignature: errors.New("execution reverted")},
			wantCalls: 1,
		},
		{
			name:      "unreachable node says nothing about the contract",
			errors:    map[string]error{contractNameSignature: errors.New("dial tcp: connection refused")},
			wantErr:   true,
			wantCalls: 1,
		},
		{
			name:      "reverting symbol keeps the pre-batch not-a-token verdict",
			errors:    map[string]error{contractSymbolSignature: errors.New("execution reverted")},
			wantCalls: 2,
		},
		{
			name:      "failed symbol read is not a verdict",
			errors:    map[string]error{contractSymbolSignature: errors.New("context deadline exceeded")},
			wantErr:   true,
			wantCalls: 2,
		},
		{
			name:   "unreadable decimals falls back to the coin default",
			errors: map[string]error{contractDecimalsSignature: errors.New("execution reverted")},
			wantInfo: &bchain.ContractInfo{
				Contract: contractInfoA.Hex(),
				Name:     "alpha",
				Symbol:   "A",
				Decimals: EtherAmountDecimalPoint,
			},
			wantCalls: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			answer := tokenMetadataSingle(map[string]string{strings.ToLower(contractInfoA.Hex()): "alpha"})
			mock := &mockContractInfoRPC{
				single: func(to, selector string) (string, error) {
					if err, ok := tt.errors[selector]; ok {
						return "", err
					}
					return answer(to, selector)
				},
			}
			rpcClient := &EthereumRPC{RPC: mock, Timeout: time.Second}

			info, err := rpcClient.fetchContractInfo(contractInfoA.Hex())

			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantInfo == nil {
				if info != nil {
					t.Fatalf("info = %+v, want nil", *info)
				}
			} else if info == nil || *info != *tt.wantInfo {
				t.Fatalf("info = %+v, want %+v", info, *tt.wantInfo)
			}
			if calls := mock.singleCalls(); len(calls) != tt.wantCalls {
				t.Fatalf("eth_calls = %v, want %d", calls, tt.wantCalls)
			}
		})
	}
}
