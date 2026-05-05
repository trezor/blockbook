package eth

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
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
	calls := []MulticallCall{
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
	calls := []MulticallCall{
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
	encoded, err := encodeAggregate3([]MulticallCall{
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
	encoded2, err := encodeAggregate3([]MulticallCall{
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
	if _, err := encodeAggregate3([]MulticallCall{{Target: "0xnothex"}}); err == nil {
		t.Fatal("expected error for invalid target")
	}
	if _, err := encodeAggregate3([]MulticallCall{{Target: "0x1234"}}); err == nil {
		t.Fatal("expected error for too-short address")
	}
	if _, err := encodeAggregate3([]MulticallCall{{Target: "0x00000000000000000000000000000000000000aa", CallData: "zz"}}); err == nil {
		t.Fatal("expected error for invalid calldata hex")
	}
}

// fixtureAggregate3Result builds a canonical aggregate3 return payload by hand for
// a small number of (success, data) tuples. Used to verify the decoder against bytes
// the test author can fully reason about.
func fixtureAggregate3Result(results []MulticallResult) string {
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
	expected := []MulticallResult{
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

// mockMulticallRPC routes eth_call to multicall3Address through a hand-written handler,
// so MulticallAggregate3 can be exercised end-to-end without a chain.
type mockMulticallRPC struct {
	handler func(callData string) (string, error)
}

func (m *mockMulticallRPC) EthSubscribe(ctx context.Context, channel interface{}, args ...interface{}) (bchain.EVMClientSubscription, error) {
	return nil, errors.New("not implemented")
}
func (m *mockMulticallRPC) Close() {}
func (m *mockMulticallRPC) BatchCallContext(ctx context.Context, batch []rpc.BatchElem) error {
	return errors.New("not implemented")
}
func (m *mockMulticallRPC) CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error {
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
	if !strings.EqualFold(to, multicall3Address) {
		return fmt.Errorf("unexpected target: %s", to)
	}
	data, _ := argMap["data"].(string)
	out, ok := result.(*string)
	if !ok {
		return errors.New("bad result type")
	}
	resp, err := m.handler(data)
	if err != nil {
		return err
	}
	*out = resp
	return nil
}

func TestMulticallAggregate3EndToEnd(t *testing.T) {
	expected := []MulticallResult{
		{Success: true, Data: "0xdeadbeef"},
		{Success: true, Data: "0xcafebabe"},
	}
	mock := &mockMulticallRPC{
		handler: func(_ string) (string, error) {
			return fixtureAggregate3Result(expected), nil
		},
	}
	rpcClient := &EthereumRPC{RPC: mock, Timeout: time.Second}

	got, err := rpcClient.MulticallAggregate3([]MulticallCall{
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
	rpcClient := &EthereumRPC{RPC: &mockMulticallRPC{handler: func(string) (string, error) {
		t.Fatal("RPC should not be called for empty input")
		return "", nil
	}}, Timeout: time.Second}
	got, err := rpcClient.MulticallAggregate3(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil result, got %v", got)
	}
}
