//go:build unittest

package eth

import (
	"math"
	"testing"
	"time"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/trezor/blockbook/bchain"
)

// TestEthereumTypeGetNonces_PrivatePendingHint_RoutesUnknownAddress verifies the declared
// private-pending hint short-circuits routing: an address that is NOT a recent private sender (so
// useForNonces is false and it would otherwise go to the primary RPC) is routed to the alternative
// provider purely because the request declared an in-flight private nonce.
func TestEthereumTypeGetNonces_PrivatePendingHint_RoutesUnknownAddress(t *testing.T) {
	server := newNonceRPCServer(t, map[string]string{"pending": "0x9"}, nil)
	stub := &nonceBatchStub{results: map[string]string{"pending": "0x4"}}
	// no recent senders → without the hint this address would be served by the primary RPC
	b := &EthereumRPC{RPC: stub, Timeout: time.Second, alternativeSendTxProvider: newRecentSenderProvider(server)}

	// declared nonce 42 → floor 43, which exceeds the provider's answer of 9
	pending, _, _, err := b.EthereumTypeGetNonces(nonceTestAddr, false, 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pending != 43 {
		t.Errorf("pending = %d, want 43 (declared floor over the provider answer)", pending)
	}
	if got := server.callCount("pending"); got != 1 {
		t.Errorf("alternative provider queried %d times, want 1 (hint must route despite no recent send)", got)
	}
	if len(stub.queried) != 0 {
		t.Errorf("primary RPC queried tags %v, want none once routed to the provider", stub.queried)
	}
}

// TestEthereumTypeGetNonces_PrivatePendingHint_ProviderAnswerWins confirms the provider's own
// answer is used when it already exceeds the declared floor (the floor only raises, never lowers).
func TestEthereumTypeGetNonces_PrivatePendingHint_ProviderAnswerWins(t *testing.T) {
	server := newNonceRPCServer(t, map[string]string{"pending": "0x64"}, nil) // 100
	stub := &nonceBatchStub{results: map[string]string{"pending": "0x4"}}
	b := &EthereumRPC{RPC: stub, Timeout: time.Second, alternativeSendTxProvider: newRecentSenderProvider(server)}

	pending, _, _, err := b.EthereumTypeGetNonces(nonceTestAddr, false, 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pending != 100 {
		t.Errorf("pending = %d, want 100 (provider answer exceeds the declared floor)", pending)
	}
}

// TestEthereumTypeGetNonces_PrivatePendingHint_RaisesPrimaryFallback confirms the declared floor is
// applied on the primary fallback path too: the hint routes to the provider, the provider fails, and
// the primary answer (4) is raised to the declared floor (43) so the wallet cannot reuse the nonce
// of the private tx it just declared.
func TestEthereumTypeGetNonces_PrivatePendingHint_RaisesPrimaryFallback(t *testing.T) {
	server := newNonceRPCServer(t, nil, map[string]bool{"pending": true}) // provider errors
	stub := &nonceBatchStub{results: map[string]string{"pending": "0x4"}}
	b := &EthereumRPC{RPC: stub, Timeout: time.Second, alternativeSendTxProvider: newRecentSenderProvider(server)}

	pending, _, _, err := b.EthereumTypeGetNonces(nonceTestAddr, false, 42)
	if err != nil {
		t.Fatalf("provider failure must fall back to the primary RPC, got error: %v", err)
	}
	if pending != 43 {
		t.Errorf("pending = %d, want 43 (declared floor over the primary fallback answer)", pending)
	}
}

// TestEthereumTypeGetNonces_PrivatePendingHint_WithConfirmedNonce exercises the exact production
// combination (api/worker.go passes WithConfirmedNonce together with PrivatePendingNonces...): the
// declared floor must raise only the PENDING nonce and leave the confirmed (latest) nonce untouched.
func TestEthereumTypeGetNonces_PrivatePendingHint_WithConfirmedNonce(t *testing.T) {
	server := newNonceRPCServer(t, map[string]string{"pending": "0x9", "latest": "0x5"}, nil)
	stub := &nonceBatchStub{results: map[string]string{"pending": "0x4", "latest": "0x2"}}
	// no recent senders → routed purely by the declared floor
	b := &EthereumRPC{RPC: stub, Timeout: time.Second, alternativeSendTxProvider: newRecentSenderProvider(server)}

	// declared nonce 42 → pending floor 43, above the provider's pending answer of 9
	pending, confirmed, confirmedOK, err := b.EthereumTypeGetNonces(nonceTestAddr, true, 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pending != 43 {
		t.Errorf("pending = %d, want 43 (declared floor over the provider answer)", pending)
	}
	if confirmed != 5 || !confirmedOK {
		t.Errorf("confirmed = (%d, ok=%v), want (5, true) — the floor must not touch the confirmed nonce", confirmed, confirmedOK)
	}
	if len(stub.queried) != 0 {
		t.Errorf("primary RPC queried tags %v, want none once routed to the provider", stub.queried)
	}
}

// TestEthereumTypeGetNonces_PrivatePendingHint_RoutesOnDeclaredZero confirms a declared nonce of 0
// (a wallet's very first tx) still trips the routing guard (declaredFloor 1 > 0) and raises the
// pending nonce to 1 — the boundary the routing tests above (nonce 42) do not exercise.
func TestEthereumTypeGetNonces_PrivatePendingHint_RoutesOnDeclaredZero(t *testing.T) {
	server := newNonceRPCServer(t, map[string]string{"pending": "0x0"}, nil)
	stub := &nonceBatchStub{results: map[string]string{"pending": "0x0"}}
	b := &EthereumRPC{RPC: stub, Timeout: time.Second, alternativeSendTxProvider: newRecentSenderProvider(server)}

	pending, _, _, err := b.EthereumTypeGetNonces(nonceTestAddr, false, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pending != 1 {
		t.Errorf("pending = %d, want 1 (declared nonce 0 → floor 1)", pending)
	}
	if got := server.callCount("pending"); got != 1 {
		t.Errorf("alternative provider queried %d times, want 1 (declared 0 must still route)", got)
	}
	if len(stub.queried) != 0 {
		t.Errorf("primary RPC queried tags %v, want none once routed to the provider", stub.queried)
	}
}

// TestEthereumTypeGetNonces_PrivatePendingHint_IgnoredWithoutProvider confirms the hint is a
// relay-deployment feature: with no alternative provider configured it is ignored and the primary
// answer stands unchanged.
func TestEthereumTypeGetNonces_PrivatePendingHint_IgnoredWithoutProvider(t *testing.T) {
	stub := &nonceBatchStub{results: map[string]string{"pending": "0x4"}}
	b := &EthereumRPC{RPC: stub, Timeout: time.Second} // no alternativeSendTxProvider

	pending, _, _, err := b.EthereumTypeGetNonces(nonceTestAddr, false, 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pending != 4 {
		t.Errorf("pending = %d, want 4 (hint ignored without a provider)", pending)
	}
}

// TestEthereumTypeGetNonces_PrivatePendingHint_FoldsCacheAndDeclaredFloor confirms the reported
// nonce is the maximum of the provider answer, the cache floor, and the declared floor. Here:
// provider answer 2, cache floor 8 (cached nonce 0x7 + 1), declared floor 10 (declared nonce 9 + 1)
// → the declared floor wins at 10.
func TestEthereumTypeGetNonces_PrivatePendingHint_FoldsCacheAndDeclaredFloor(t *testing.T) {
	server := newNonceRPCServer(t, map[string]string{"pending": "0x2"}, nil)
	stub := &nonceBatchStub{results: map[string]string{"pending": "0x1"}}
	sender := ethcommon.BytesToAddress(nonceTestAddr)
	provider := newRecentSenderProvider(server, sender)
	provider.fetchMempoolTx = true
	provider.mempoolTxs = map[string]storedTx{
		testAlternativeTxID: {
			tx:   &bchain.RpcTransaction{Hash: testAlternativeTxID, From: sender.Hex(), AccountNonce: "0x7"},
			time: uint32(time.Now().Unix()),
		},
	}
	b := &EthereumRPC{RPC: stub, Timeout: time.Second, alternativeSendTxProvider: provider}

	pending, _, _, err := b.EthereumTypeGetNonces(nonceTestAddr, false, 9)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pending != 10 {
		t.Errorf("pending = %d, want 10 (declared floor 10 over cache floor 8 and provider answer 2)", pending)
	}
}

// TestDeclaredPendingFloor covers the floor helper directly.
func TestDeclaredPendingFloor(t *testing.T) {
	cases := []struct {
		in   []uint64
		want uint64
	}{
		{nil, 0},
		{[]uint64{}, 0},
		{[]uint64{0}, 1},
		{[]uint64{5}, 6},
		{[]uint64{5, 42, 7}, 43},
		// n+1 wraps to 0 at MaxUint64; the entry is silently ignored (benign: the floor is only
		// ever a max() operand, so a spurious 0 can never lower the reported nonce).
		{[]uint64{math.MaxUint64}, 0},
		// a physically-unreachable max value co-declared with a real nonce must not corrupt it.
		{[]uint64{math.MaxUint64, 43}, 44},
	}
	for _, c := range cases {
		if got := declaredPendingFloor(c.in); got != c.want {
			t.Errorf("declaredPendingFloor(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}
