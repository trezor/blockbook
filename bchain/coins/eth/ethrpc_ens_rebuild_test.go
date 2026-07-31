//go:build unittest

package eth

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/trezor/blockbook/bchain"
)

// ensLogEntry pairs a NameRegistered log with the block it was emitted in so the
// mock can honor the eth_getLogs block range the way a node would.
type ensLogEntry struct {
	block uint64
	log   rpcLogWithTxHash
}

// mockEnsLogsRPC serves eth_getLogs from a fixed set of logs, filtering only by
// block range (emitter/topic filtering is left to getEnsRecord, so the test
// exercises Blockbook's own emitter check rather than trusting the node's).
type mockEnsLogsRPC struct {
	entries     []ensLogEntry
	calls       int
	lastFilters []map[string]interface{}
}

func (m *mockEnsLogsRPC) EthSubscribe(ctx context.Context, channel interface{}, args ...interface{}) (bchain.EVMClientSubscription, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockEnsLogsRPC) Close() {}

func (m *mockEnsLogsRPC) CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error {
	if method != "eth_getLogs" {
		return fmt.Errorf("unexpected method %q", method)
	}
	m.calls++
	out, ok := result.(*[]rpcLogWithTxHash)
	if !ok {
		return fmt.Errorf("unexpected result type %T", result)
	}
	if len(args) != 1 {
		return fmt.Errorf("expected 1 filter arg, got %d", len(args))
	}
	filter, ok := args[0].(map[string]interface{})
	if !ok {
		return fmt.Errorf("unexpected filter type %T", args[0])
	}
	// Snapshot the filter: RebuildEnsAliases reuses one map and mutates its block
	// bounds in place, so we must copy to assert per-call.
	snap := make(map[string]interface{}, len(filter))
	for k, v := range filter {
		snap[k] = v
	}
	m.lastFilters = append(m.lastFilters, snap)
	from, err := blockBoundToUint(filter["fromBlock"])
	if err != nil {
		return err
	}
	to, err := blockBoundToUint(filter["toBlock"])
	if err != nil {
		return err
	}
	var logs []rpcLogWithTxHash
	for _, e := range m.entries {
		if e.block >= from && e.block <= to {
			logs = append(logs, e.log)
		}
	}
	*out = logs
	return nil
}

func blockBoundToUint(v interface{}) (uint64, error) {
	s, ok := v.(string)
	if !ok {
		return 0, fmt.Errorf("block bound is not a string: %T", v)
	}
	return strconv.ParseUint(strings.TrimPrefix(s, "0x"), 16, 64)
}

// unraveledLog builds a valid 5-arg NameRegistered log for name "unraveled"
// (owner 0x2C630b16…) emitted by the given contract. Payload mirrors the
// getEnsRecord unit-test vectors.
func unraveledLog(emitter string) rpcLogWithTxHash {
	return rpcLogWithTxHash{
		RpcLog: bchain.RpcLog{
			Address: emitter,
			Topics: []string{
				nameRegisteredEventSignature,
				"0x40ce2aa8cd9ee9fef4bf3a68abab7fbcceb6bac89370518caf6a602cefe836bd",
				"0x0000000000000000000000002c630b16aa53ae0189880e15c23323688acb607c",
			},
			Data: "0x00000000000000000000000000000000000000000000000000000000000000600000000000000000000000000000000000000000000000000017629245f5a86f0000000000000000000000000000000000000000000000000000000069dbb21d0000000000000000000000000000000000000000000000000000000000000009756e726176656c65640000000000000000000000000000000000000000000000",
		},
	}
}

var unraveledRecord = bchain.AddressAliasRecord{Address: "0x2C630b16Aa53ae0189880e15C23323688acb607c", Name: "unraveled"}

func Test_RebuildEnsAliases(t *testing.T) {
	const (
		trusted1  = "0x283Af0B28c62C092C9727F1Ee09c02CA627EB7F5"
		trusted2  = "0x253553366Da8546fC250F225fe3d25d0C782303b"
		untrusted = "0xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	)
	mock := &mockEnsLogsRPC{
		entries: []ensLogEntry{
			{block: 100, log: unraveledLog(trusted1)},
			{block: 200, log: unraveledLog(untrusted)},  // must be dropped by getEnsRecord
			{block: 15000, log: unraveledLog(trusted2)}, // lands in a later chunk
		},
	}
	b := &EthereumRPC{
		RPC:     mock,
		Timeout: time.Second,
		ensRegistrars: ensRegistrarSet([]string{
			"0x283af0b28c62c092c9727f1ee09c02ca627eb7f5",
			"0x253553366da8546fc250f225fe3d25d0c782303b",
		}),
	}
	var got []bchain.AddressAliasRecord
	store := func(r []bchain.AddressAliasRecord) error {
		got = append(got, r...)
		return nil
	}
	if err := b.RebuildEnsAliases(0, 20000, 1000, nil, store); err != nil {
		t.Fatal(err)
	}
	want := []bchain.AddressAliasRecord{unraveledRecord, unraveledRecord}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("records = %+v, want %+v (untrusted emitter must be dropped)", got, want)
	}
	// [0,20000] stepped by 1000 = 21 chunks.
	if mock.calls != 21 {
		t.Errorf("eth_getLogs calls = %d, want 21", mock.calls)
	}
	// Filter must restrict emitters to the trusted set and OR the 3 topic0 sigs.
	f := mock.lastFilters[0]
	gotAddrs := append([]string(nil), f["address"].([]string)...)
	sort.Strings(gotAddrs)
	wantAddrs := []string{
		"0x253553366da8546fc250f225fe3d25d0c782303b",
		"0x283af0b28c62c092c9727f1ee09c02ca627eb7f5",
	}
	if !reflect.DeepEqual(gotAddrs, wantAddrs) {
		t.Errorf("filter address = %v, want %v", gotAddrs, wantAddrs)
	}
	topics, ok := f["topics"].([]interface{})
	if !ok || len(topics) != 1 {
		t.Fatalf("filter topics = %v, want a single-element slice", f["topics"])
	}
	if topic0, ok := topics[0].([]string); !ok || len(topic0) != 3 {
		t.Errorf("filter topic0 = %v, want the 3 NameRegistered signatures", topics[0])
	}
}

func Test_RebuildEnsAliases_EmptySetRecordsNothing(t *testing.T) {
	mock := &mockEnsLogsRPC{entries: []ensLogEntry{{block: 100, log: unraveledLog("0x283Af0B28c62C092C9727F1Ee09c02CA627EB7F5")}}}
	b := &EthereumRPC{RPC: mock, Timeout: time.Second, ensRegistrars: ensRegistrarSet(nil)}
	var got []bchain.AddressAliasRecord
	if err := b.RebuildEnsAliases(0, 1000, 1000, nil, func(r []bchain.AddressAliasRecord) error {
		got = append(got, r...)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("records = %v, want none for an empty trusted set", got)
	}
	if mock.calls != 0 {
		t.Errorf("eth_getLogs calls = %d, want 0 (no scan for trust-none)", mock.calls)
	}
}

func Test_RebuildEnsAliases_WildcardScansAllEmitters(t *testing.T) {
	untrusted := "0xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	mock := &mockEnsLogsRPC{entries: []ensLogEntry{{block: 5, log: unraveledLog(untrusted)}}}
	b := &EthereumRPC{RPC: mock, Timeout: time.Second, ensRegistrars: ensRegistrarSet([]string{"*"})}
	var got []bchain.AddressAliasRecord
	if err := b.RebuildEnsAliases(0, 10, 100, nil, func(r []bchain.AddressAliasRecord) error {
		got = append(got, r...)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []bchain.AddressAliasRecord{unraveledRecord}) {
		t.Fatalf("records = %+v, want the wildcard to accept the untrusted emitter", got)
	}
	if _, restricted := mock.lastFilters[0]["address"]; restricted {
		t.Error("wildcard filter must not restrict address")
	}
}

func Test_EnsRegistrarsFingerprint(t *testing.T) {
	fp := func(addrs []string) string {
		return (&EthereumRPC{ensRegistrars: ensRegistrarSet(addrs)}).EnsRegistrarsFingerprint()
	}
	if got := fp(nil); got != "" {
		t.Errorf("empty set fingerprint = %q, want empty (trust-none never auto-heals)", got)
	}
	// Order-independent: the same set in any order yields the same fingerprint.
	a := fp([]string{"0x283af0b28c62c092c9727f1ee09c02ca627eb7f5", "0x59e16fccd424cc24e280be16e11bcd56fb0ce547"})
	b := fp([]string{"0x59e16fccd424cc24e280be16e11bcd56fb0ce547", "0x283af0b28c62c092c9727f1ee09c02ca627eb7f5"})
	if a == "" {
		t.Fatal("non-empty set produced empty fingerprint")
	}
	if a != b {
		t.Errorf("fingerprint depends on order: %q vs %q", a, b)
	}
	// Changing a registrar address changes the fingerprint (drives the self-heal).
	c := fp([]string{"0x283af0b28c62c092c9727f1ee09c02ca627eb7f5", "0x0000000000000000000000000000000000000001"})
	if c == a {
		t.Errorf("fingerprint unchanged after a registrar address changed: %q", c)
	}
	if got := fp([]string{"*"}); got == "" {
		t.Error("wildcard set produced empty fingerprint")
	}
}

func Test_RebuildEnsAliases_Interrupted(t *testing.T) {
	mock := &mockEnsLogsRPC{entries: []ensLogEntry{{block: 5, log: unraveledLog("0x283Af0B28c62C092C9727F1Ee09c02CA627EB7F5")}}}
	b := &EthereumRPC{RPC: mock, Timeout: time.Second, ensRegistrars: ensRegistrarSet([]string{"0x283af0b28c62c092c9727f1ee09c02ca627eb7f5"})}
	err := b.RebuildEnsAliases(0, 1000000, 1000, func() bool { return true }, func(r []bchain.AddressAliasRecord) error { return nil })
	if err != ErrEnsRebuildInterrupted {
		t.Fatalf("err = %v, want ErrEnsRebuildInterrupted", err)
	}
	if mock.calls != 0 {
		t.Errorf("eth_getLogs calls = %d, want 0 (interrupted before first chunk)", mock.calls)
	}
}
