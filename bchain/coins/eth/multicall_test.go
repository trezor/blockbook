package eth

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/rpc"
	"github.com/trezor/blockbook/bchain"
)

// padHex32 left-pads `s` (a hex string without 0x) with zeros to 64 chars (32 bytes).
func padHex32(s string) string {
	if len(s) >= 64 {
		return s
	}
	return strings.Repeat("0", 64-len(s)) + s
}

// rightPadHex pads `s` (hex without 0x) with zeros on the right to a multiple of 64 hex chars.
func rightPadHex(s string) string {
	rem := len(s) % 64
	if rem == 0 {
		return s
	}
	return s + strings.Repeat("0", 64-rem)
}

func TestEncodeAggregate3KnownLayout(t *testing.T) {
	calls := []bchain.EthereumMulticallCall{
		{Target: "0x00000000000000000000000000000000000000aa", CallData: "0x06fdde03", AllowFailure: false},
		{Target: "0x00000000000000000000000000000000000000bb", CallData: "0x95d89b41", AllowFailure: true},
	}

	encoded, err := encodeAggregate3(calls)
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}
	raw, err := hex.DecodeString(strings.TrimPrefix(encoded, "0x"))
	if err != nil {
		t.Fatalf("decode hex: %v", err)
	}

	// Selector: 0x82ad56cb.
	if got := hex.EncodeToString(raw[:4]); got != "82ad56cb" {
		t.Fatalf("selector mismatch: got %s want 82ad56cb", got)
	}
	// Outer offset to array = 0x20.
	if v := bigUintAt(raw, 4); v.Cmp(big.NewInt(0x20)) != 0 {
		t.Fatalf("outer offset wrong: %s", v)
	}
	// Array length = 2 at byte 4+32 = 36.
	if v := bigUintAt(raw, 4+32); v.Cmp(big.NewInt(2)) != 0 {
		t.Fatalf("array length wrong: %s", v)
	}
	// Heads start at byte 4+64 = 68. Two offsets follow.
	if v := bigUintAt(raw, 4+64); v.Cmp(big.NewInt(64)) != 0 {
		t.Fatalf("first head offset wrong: %s, want 64", v)
	}
	// Each tuple: 32(address)+32(bool)+32(0x60)+32(len)+32(data padded) = 160 bytes.
	if v := bigUintAt(raw, 4+96); v.Cmp(big.NewInt(64+160)) != 0 {
		t.Fatalf("second head offset wrong: %s, want %d", v, 64+160)
	}
	// Total encoded size = selector(4) + outer(32) + len(32) + heads(64) + tuples(2*160) = 452.
	if got, want := len(raw), 4+32+32+64+2*160; got != want {
		t.Fatalf("total size: got %d want %d", got, want)
	}

	// Spot-check tuple 0's bool byte (false → 0) and tuple 1's bool byte (true → 1).
	tuple0Start := 4 + 32 + 32 + 64 // start of tuple 0
	if raw[tuple0Start+32+31] != 0 {
		t.Fatalf("tuple 0 bool byte should be 0")
	}
	tuple1Start := tuple0Start + 160
	if raw[tuple1Start+32+31] != 1 {
		t.Fatalf("tuple 1 bool byte should be 1")
	}
}

// TestEncodeAggregate3MatchesCanonicalABI locks the hand-rolled encoder against
// the byte-for-byte output of go-ethereum's accounts/abi package for a small
// fixture. If this drifts, the encoder has gone non-canonical.
func TestEncodeAggregate3MatchesCanonicalABI(t *testing.T) {
	calls := []bchain.EthereumMulticallCall{
		{Target: "0x00000000000000000000000000000000000000aa", CallData: "0x06fdde03", AllowFailure: false},
		{Target: "0x00000000000000000000000000000000000000bb", CallData: "0x95d89b41", AllowFailure: true},
	}
	const expected = "0x82ad56cb" +
		"0000000000000000000000000000000000000000000000000000000000000020" +
		"0000000000000000000000000000000000000000000000000000000000000002" +
		"0000000000000000000000000000000000000000000000000000000000000040" +
		"00000000000000000000000000000000000000000000000000000000000000e0" +
		"00000000000000000000000000000000000000000000000000000000000000aa" +
		"0000000000000000000000000000000000000000000000000000000000000000" +
		"0000000000000000000000000000000000000000000000000000000000000060" +
		"0000000000000000000000000000000000000000000000000000000000000004" +
		"06fdde0300000000000000000000000000000000000000000000000000000000" +
		"00000000000000000000000000000000000000000000000000000000000000bb" +
		"0000000000000000000000000000000000000000000000000000000000000001" +
		"0000000000000000000000000000000000000000000000000000000000000060" +
		"0000000000000000000000000000000000000000000000000000000000000004" +
		"95d89b4100000000000000000000000000000000000000000000000000000000"
	got, err := encodeAggregate3(calls)
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}
	if !strings.EqualFold(got, expected) {
		t.Fatalf("encoder drift:\n got: %s\nwant: %s", got, expected)
	}
}

func TestEncodeAggregate3EmptyAndPadding(t *testing.T) {
	// Empty CallData should produce a tuple with 0 length bytes and no data words.
	encoded, err := encodeAggregate3([]bchain.EthereumMulticallCall{
		{Target: "0x00000000000000000000000000000000000000ee", CallData: "0x", AllowFailure: false},
	})
	if err != nil {
		t.Fatalf("encode empty calldata: %v", err)
	}
	raw, _ := hex.DecodeString(strings.TrimPrefix(encoded, "0x"))
	// selector(4) + outer(32) + len(32) + heads(32) + tuple_size(128) = 228.
	// tuple_size when payload is empty: 32(addr)+32(bool)+32(0x60)+32(len=0)+0 = 128.
	if got, want := len(raw), 4+32+32+32+128; got != want {
		t.Fatalf("empty-payload size: got %d want %d", got, want)
	}
	// 5-byte payload should pad up to 32 bytes.
	encoded2, err := encodeAggregate3([]bchain.EthereumMulticallCall{
		{Target: "0x00000000000000000000000000000000000000ee", CallData: "0x1234567890"},
	})
	if err != nil {
		t.Fatalf("encode 5-byte calldata: %v", err)
	}
	raw2, _ := hex.DecodeString(strings.TrimPrefix(encoded2, "0x"))
	if got, want := len(raw2), 4+32+32+32+160; got != want {
		t.Fatalf("5-byte-padded size: got %d want %d", got, want)
	}
}

func TestEncodeAggregate3RejectsBadInput(t *testing.T) {
	if _, err := encodeAggregate3([]bchain.EthereumMulticallCall{{Target: "0xnothex"}}); err == nil {
		t.Fatal("expected error for invalid target")
	}
	if _, err := encodeAggregate3([]bchain.EthereumMulticallCall{{Target: "0x1234"}}); err == nil {
		t.Fatal("expected error for too-short address")
	}
	if _, err := encodeAggregate3([]bchain.EthereumMulticallCall{{Target: "0x00000000000000000000000000000000000000aa", CallData: "zz"}}); err == nil {
		t.Fatal("expected error for invalid calldata hex")
	}
}

// fixtureAggregate3Result builds a canonical aggregate3 return payload by hand for
// a small number of (success, data) tuples. Used to verify the decoder against bytes
// the test author can fully reason about.
func fixtureAggregate3Result(results []bchain.EthereumMulticallResult) string {
	type encoded struct {
		successByte byte
		data        []byte
	}
	enc := make([]encoded, len(results))
	for i, r := range results {
		var b byte
		if r.Success {
			b = 1
		}
		raw, _ := hex.DecodeString(strings.TrimPrefix(r.Data, "0x"))
		enc[i] = encoded{successByte: b, data: raw}
	}

	headBytes := len(enc) * 32
	cursor := headBytes
	offsets := make([]int, len(enc))
	for i, e := range enc {
		offsets[i] = cursor
		// (bool, offset, len, data padded)
		cursor += 32*3 + paddedLen(len(e.data))
	}

	var out strings.Builder
	// outer offset 0x20
	out.WriteString(padHex32("20"))
	// length
	out.WriteString(padHex32(fmt.Sprintf("%x", len(enc))))
	for _, off := range offsets {
		out.WriteString(padHex32(fmt.Sprintf("%x", off)))
	}
	for _, e := range enc {
		// success
		bword := "00"
		if e.successByte == 1 {
			bword = "01"
		}
		out.WriteString(padHex32(bword))
		// bytes offset within tuple = 0x40 (2 head words)
		out.WriteString(padHex32("40"))
		// bytes length
		out.WriteString(padHex32(fmt.Sprintf("%x", len(e.data))))
		// padded data
		dataHex := hex.EncodeToString(e.data)
		out.WriteString(rightPadHex(dataHex))
	}
	return "0x" + out.String()
}

func TestDecodeAggregate3RoundTripFixture(t *testing.T) {
	expected := []bchain.EthereumMulticallResult{
		{Success: true, Data: "0x1234567890"},
		{Success: false, Data: "0x"},
		{Success: true, Data: "0x" + strings.Repeat("ab", 64)}, // 64 bytes, exactly two padded words
	}
	got, err := decodeAggregate3Result(fixtureAggregate3Result(expected))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != len(expected) {
		t.Fatalf("length: got %d want %d", len(got), len(expected))
	}
	for i := range expected {
		if got[i].Success != expected[i].Success {
			t.Fatalf("[%d] success: got %v want %v", i, got[i].Success, expected[i].Success)
		}
		if !strings.EqualFold(got[i].Data, expected[i].Data) {
			t.Fatalf("[%d] data: got %s want %s", i, got[i].Data, expected[i].Data)
		}
	}
}

func TestDecodeAggregate3Rejects(t *testing.T) {
	cases := []struct {
		name string
		hex  string
	}{
		{"empty", "0x"},
		{"too short for header", "0x" + padHex32("20")},
		{"bad outer offset", "0x" + padHex32("21") + padHex32("0")},
		{"truncated heads", "0x" + padHex32("20") + padHex32("2") + padHex32("40")}, // declares 2 elements but only 1 head word
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := decodeAggregate3Result(tc.hex); err == nil {
				t.Fatalf("expected error for %q", tc.name)
			}
		})
	}
}

// mockMulticallRPC routes eth_call and eth_getCode through hand-written
// handlers so MulticallAggregate3 (and the deployment probe in front of it)
// can be exercised end-to-end without a chain.
type mockMulticallRPC struct {
	mu sync.Mutex
	// eth_call handler. Required for tests that exercise the multicall path.
	handler func(callData string) (string, error)
	// eth_getCode handler for the deployment probe. When nil, the probe is
	// answered with a stub "deployed" bytecode so existing multicall tests
	// don't need to care about the probe.
	getCodeHandler func(address string) (string, error)

	ethCallCalls int
	getCodeCalls int
}

func (m *mockMulticallRPC) callCounts() (ethCall, getCode int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ethCallCalls, m.getCodeCalls
}

func (m *mockMulticallRPC) EthSubscribe(ctx context.Context, channel interface{}, args ...interface{}) (bchain.EVMClientSubscription, error) {
	return nil, errors.New("not implemented")
}
func (m *mockMulticallRPC) Close() {}
func (m *mockMulticallRPC) BatchCallContext(ctx context.Context, batch []rpc.BatchElem) error {
	return errors.New("not implemented")
}
func (m *mockMulticallRPC) CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error {
	out, ok := result.(*string)
	if !ok {
		return errors.New("bad result type")
	}
	switch method {
	case "eth_getCode":
		m.mu.Lock()
		m.getCodeCalls++
		m.mu.Unlock()
		if len(args) < 2 {
			return errors.New("eth_getCode: missing args")
		}
		addr, _ := args[0].(string)
		if !strings.EqualFold(addr, multicall3Address) {
			return fmt.Errorf("unexpected eth_getCode target: %s", addr)
		}
		if m.getCodeHandler == nil {
			// Default: report deployed with stub bytecode. Lets unrelated
			// tests proceed straight to the eth_call handler.
			*out = "0x6080604052"
			return nil
		}
		s, err := m.getCodeHandler(addr)
		if err != nil {
			return err
		}
		*out = s
		return nil
	case "eth_call":
		m.mu.Lock()
		m.ethCallCalls++
		m.mu.Unlock()
		if len(args) < 2 {
			return errors.New("eth_call: missing args")
		}
		argMap, ok := args[0].(map[string]interface{})
		if !ok {
			return errors.New("eth_call: bad args")
		}
		to, _ := argMap["to"].(string)
		if !strings.EqualFold(to, multicall3Address) {
			return fmt.Errorf("unexpected eth_call target: %s", to)
		}
		data, _ := argMap["data"].(string)
		if m.handler == nil {
			return errors.New("no eth_call handler installed")
		}
		resp, err := m.handler(data)
		if err != nil {
			return err
		}
		*out = resp
		return nil
	default:
		return fmt.Errorf("unexpected method: %s", method)
	}
}

func TestMulticallAggregate3EndToEnd(t *testing.T) {
	expected := []bchain.EthereumMulticallResult{
		{Success: true, Data: "0xdeadbeef"},
		{Success: true, Data: "0xcafebabe"},
	}
	mock := &mockMulticallRPC{
		handler: func(_ string) (string, error) {
			return fixtureAggregate3Result(expected), nil
		},
	}
	rpcClient := &EthereumRPC{RPC: mock, Timeout: time.Second}

	got, err := rpcClient.EthereumTypeMulticallAggregate3([]bchain.EthereumMulticallCall{
		{Target: "0x00000000000000000000000000000000000000aa", CallData: "0x06fdde03"},
		{Target: "0x00000000000000000000000000000000000000bb", CallData: "0x95d89b41"},
	}, nil)
	if err != nil {
		t.Fatalf("MulticallAggregate3 error: %v", err)
	}
	if len(got) != len(expected) {
		t.Fatalf("len mismatch: got %d want %d", len(got), len(expected))
	}
	for i := range expected {
		if got[i].Success != expected[i].Success || !strings.EqualFold(got[i].Data, expected[i].Data) {
			t.Fatalf("[%d] mismatch: got %+v want %+v", i, got[i], expected[i])
		}
	}
}

func TestMulticallAggregate3EmptyCalls(t *testing.T) {
	mock := &mockMulticallRPC{
		handler: func(string) (string, error) {
			t.Fatal("eth_call should not be issued for empty input")
			return "", nil
		},
		getCodeHandler: func(string) (string, error) {
			t.Fatal("eth_getCode probe should not fire for empty input")
			return "", nil
		},
	}
	rpcClient := &EthereumRPC{RPC: mock, Timeout: time.Second}
	got, err := rpcClient.EthereumTypeMulticallAggregate3(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil result, got %v", got)
	}
}

// --- Multicall3 deployment probe ---

func TestProbeMulticall3_DetectsDeployedAndCachesResult(t *testing.T) {
	mock := &mockMulticallRPC{
		// Any non-empty bytecode counts as deployed.
		getCodeHandler: func(string) (string, error) { return "0x6080604052348015", nil },
	}
	rpc := &EthereumRPC{RPC: mock, Timeout: time.Second}

	if got, err := rpc.probeMulticall3(); err != nil || !got {
		t.Fatalf("probe should report deployed for non-empty bytecode, got=%v err=%v", got, err)
	}
	if got, err := rpc.probeMulticall3(); err != nil || !got {
		t.Fatalf("probe should still report deployed on second call, got=%v err=%v", got, err)
	}
	if _, getCode := mock.callCounts(); getCode != 1 {
		t.Fatalf("expected 1 eth_getCode call (cached on 2nd), got %d", getCode)
	}
	if state := rpc.multicall3Probe.Load(); state != multicall3Deployed {
		t.Fatalf("expected state=Deployed, got %d", state)
	}
}

func TestProbeMulticall3_DetectsNotDeployedAndCachesResult(t *testing.T) {
	mock := &mockMulticallRPC{
		getCodeHandler: func(string) (string, error) { return "0x", nil },
	}
	rpc := &EthereumRPC{RPC: mock, Timeout: time.Second}

	if got, err := rpc.probeMulticall3(); err != nil || got {
		t.Fatalf("probe should report not-deployed for '0x', got=%v err=%v", got, err)
	}
	if got, err := rpc.probeMulticall3(); err != nil || got {
		t.Fatalf("probe should still report not-deployed on second call, got=%v err=%v", got, err)
	}
	if _, getCode := mock.callCounts(); getCode != 1 {
		t.Fatalf("expected 1 eth_getCode call (cached on 2nd), got %d", getCode)
	}
	if state := rpc.multicall3Probe.Load(); state != multicall3NotDeployed {
		t.Fatalf("expected state=NotDeployed, got %d", state)
	}
}

func TestProbeMulticall3_TransientErrorRetriesNextCall(t *testing.T) {
	// First eth_getCode errors (RPC blip); second succeeds. The probe must
	// retry rather than caching the transient failure.
	var attempt atomic.Int32
	mock := &mockMulticallRPC{
		getCodeHandler: func(string) (string, error) {
			n := attempt.Add(1)
			if n == 1 {
				return "", errors.New("rpc down")
			}
			return "0x6080604052", nil
		},
	}
	rpc := &EthereumRPC{RPC: mock, Timeout: time.Second}

	got, err := rpc.probeMulticall3()
	if err == nil {
		t.Fatal("first probe should propagate the transient RPC error")
	}
	if got {
		t.Fatalf("first probe should report deployed=false on transient error, got=%v", got)
	}
	if state := rpc.multicall3Probe.Load(); state != multicall3Unprobed {
		t.Fatalf("transient error must NOT cache state, got %d", state)
	}
	if got, err := rpc.probeMulticall3(); err != nil || !got {
		t.Fatalf("second probe should detect deployed (transient error not cached), got=%v err=%v", got, err)
	}
	if _, getCode := mock.callCounts(); getCode != 2 {
		t.Fatalf("expected 2 eth_getCode calls (no caching after transient), got %d", getCode)
	}
	if state := rpc.multicall3Probe.Load(); state != multicall3Deployed {
		t.Fatalf("expected state=Deployed after recovery, got %d", state)
	}
}

func TestProbeMulticall3_ConcurrentFirstCallsCollapseToOneRPC(t *testing.T) {
	// 32 concurrent first-time probes against a slow eth_getCode must result
	// in exactly one upstream RPC and a deployed verdict for every caller.
	const concurrency = 32
	gate := make(chan struct{})
	mock := &mockMulticallRPC{
		getCodeHandler: func(string) (string, error) {
			<-gate
			return "0x6080", nil
		},
	}
	rpc := &EthereumRPC{RPC: mock, Timeout: time.Second}

	type probeOutcome struct {
		deployed bool
		err      error
	}
	results := make([]probeOutcome, concurrency)
	var wg sync.WaitGroup
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		i := i
		go func() {
			defer wg.Done()
			deployed, err := rpc.probeMulticall3()
			results[i] = probeOutcome{deployed: deployed, err: err}
		}()
	}
	// Wait for the in-flight probe to register one eth_getCode call before
	// releasing it; concurrent peers will join singleflight in the meantime.
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, gc := mock.callCounts(); gc >= 1 {
			break
		}
		if time.Now().After(deadline) {
			close(gate)
			wg.Wait()
			t.Fatal("timed out waiting for first eth_getCode")
		}
		time.Sleep(time.Millisecond)
	}
	close(gate)
	wg.Wait()

	if _, gc := mock.callCounts(); gc != 1 {
		t.Fatalf("singleflight must collapse concurrent probes to 1 RPC, got %d", gc)
	}
	for i, r := range results {
		if r.err != nil || !r.deployed {
			t.Fatalf("result[%d]: expected deployed=true, got=%+v", i, r)
		}
	}
}

func TestEthereumTypeMulticallAggregate3_NotDeployed_ShortCircuits(t *testing.T) {
	// With probe state pre-set to NotDeployed, MulticallAggregate3 must return
	// errMulticall3NotDeployed without issuing any eth_call.
	mock := &mockMulticallRPC{
		handler: func(string) (string, error) {
			t.Fatal("eth_call must not be issued when multicall3 is known absent")
			return "", nil
		},
		getCodeHandler: func(string) (string, error) {
			t.Fatal("eth_getCode must not be issued when probe state is already known")
			return "", nil
		},
	}
	rpc := &EthereumRPC{RPC: mock, Timeout: time.Second}
	rpc.multicall3Probe.Store(multicall3NotDeployed)

	got, err := rpc.EthereumTypeMulticallAggregate3([]bchain.EthereumMulticallCall{
		{Target: "0x00000000000000000000000000000000000000aa", CallData: "0x06fdde03"},
	}, nil)
	if got != nil {
		t.Fatalf("expected nil result, got %+v", got)
	}
	if !errors.Is(err, errMulticall3NotDeployed) {
		t.Fatalf("expected errMulticall3NotDeployed, got %v", err)
	}
}

// A transient probe failure must surface as a real error to callers rather
// than being collapsed to errMulticall3NotDeployed — otherwise an RPC blip
// during the first request would look indistinguishable from "this chain
// has no Multicall3" in caller telemetry and short-circuit logic.
func TestEthereumTypeMulticallAggregate3_TransientProbeError_PropagatesAndIsDistinct(t *testing.T) {
	probeErr := errors.New("rpc down")
	mock := &mockMulticallRPC{
		handler: func(string) (string, error) {
			t.Fatal("eth_call must not be issued when probe failed transiently")
			return "", nil
		},
		getCodeHandler: func(string) (string, error) { return "", probeErr },
	}
	rpc := &EthereumRPC{RPC: mock, Timeout: time.Second}

	got, err := rpc.EthereumTypeMulticallAggregate3([]bchain.EthereumMulticallCall{
		{Target: "0x00000000000000000000000000000000000000aa", CallData: "0x06fdde03"},
	}, nil)
	if got != nil {
		t.Fatalf("expected nil result, got %+v", got)
	}
	if err == nil {
		t.Fatal("expected non-nil error from transient probe failure")
	}
	if errors.Is(err, errMulticall3NotDeployed) {
		t.Fatalf("transient error must be distinguishable from errMulticall3NotDeployed, got %v", err)
	}
	if !errors.Is(err, probeErr) {
		t.Fatalf("expected wrapped probe error, got %v", err)
	}
	// Probe state must remain unprobed so the next call retries.
	if state := rpc.multicall3Probe.Load(); state != multicall3Unprobed {
		t.Fatalf("transient probe failure must not cache state, got %d", state)
	}
}

func TestEthereumTypeMulticallAggregate3_ProbesOnFirstCall(t *testing.T) {
	// First call on a fresh EthereumRPC must probe via eth_getCode then
	// proceed to eth_call. Subsequent calls must skip the probe.
	expected := []bchain.EthereumMulticallResult{{Success: true, Data: "0xdead"}}
	mock := &mockMulticallRPC{
		handler:        func(string) (string, error) { return fixtureAggregate3Result(expected), nil },
		getCodeHandler: func(string) (string, error) { return "0x6080", nil },
	}
	rpc := &EthereumRPC{RPC: mock, Timeout: time.Second}

	for i := 0; i < 3; i++ {
		got, err := rpc.EthereumTypeMulticallAggregate3([]bchain.EthereumMulticallCall{
			{Target: "0x00000000000000000000000000000000000000aa", CallData: "0x06fdde03"},
		}, nil)
		if err != nil {
			t.Fatalf("call %d: unexpected error %v", i, err)
		}
		if len(got) != 1 || !strings.EqualFold(got[0].Data, expected[0].Data) {
			t.Fatalf("call %d: unexpected result %+v", i, got)
		}
	}
	ethCall, getCode := mock.callCounts()
	if getCode != 1 {
		t.Fatalf("expected exactly 1 eth_getCode (probe runs once), got %d", getCode)
	}
	if ethCall != 3 {
		t.Fatalf("expected 3 eth_call (one per request), got %d", ethCall)
	}
}
