//go:build unittest

package eth

import (
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
	// the relay no longer counts the declared tx at its pending tag, so it answers 42 for a tx that
	// occupies exactly that slot - the gap the hint exists to close
	server := newNonceRPCServer(t, map[string]string{"pending": "0x2a"}, nil)
	stub := &nonceBatchStub{results: map[string]string{"pending": "0x4"}}
	// no recent senders → without the hint this address would be served by the primary RPC
	b := &EthereumRPC{RPC: stub, Timeout: time.Second, alternativeSendTxProvider: newRecentSenderProvider(server)}

	pending, _, _, err := b.EthereumTypeGetNonces(nonceTestAddr, false, 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pending != 43 {
		t.Errorf("pending = %d, want 43 (the declared nonce 42 is in flight, so it is not handed out again)", pending)
	}
	if got := server.callCount("pending"); got != 1 {
		t.Errorf("alternative provider queried %d times, want 1 (hint must route despite no recent send)", got)
	}
	if len(stub.queried) != 0 {
		t.Errorf("primary RPC queried tags %v, want none once routed to the provider", stub.queried)
	}
}

// TestEthereumTypeGetNonces_PrivatePendingHint_ProviderAnswerWins confirms the floor only ever
// raises: a provider answer already past the declared nonce stands unchanged.
func TestEthereumTypeGetNonces_PrivatePendingHint_ProviderAnswerWins(t *testing.T) {
	server := newNonceRPCServer(t, map[string]string{"pending": "0x64"}, nil) // 100
	stub := &nonceBatchStub{results: map[string]string{"pending": "0x4"}}
	b := &EthereumRPC{RPC: stub, Timeout: time.Second, alternativeSendTxProvider: newRecentSenderProvider(server)}

	pending, _, _, err := b.EthereumTypeGetNonces(nonceTestAddr, false, 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pending != 100 {
		t.Errorf("pending = %d, want 100 (the declared nonce is long consumed, the provider answer stands)", pending)
	}
}

// TestEthereumTypeGetNonces_PrivatePendingHint_RaisesPrimaryFallback confirms the declared nonce is
// honored on the primary fallback path too: the hint routes to the provider, the provider fails, and
// the primary answer - which never knew the private tx - is raised past the declared slot so the
// wallet cannot reuse the nonce of the transaction it just declared.
func TestEthereumTypeGetNonces_PrivatePendingHint_RaisesPrimaryFallback(t *testing.T) {
	server := newNonceRPCServer(t, nil, map[string]bool{"pending": true}) // provider errors
	stub := &nonceBatchStub{results: map[string]string{"pending": "0x4"}}
	b := &EthereumRPC{RPC: stub, Timeout: time.Second, alternativeSendTxProvider: newRecentSenderProvider(server)}

	pending, _, _, err := b.EthereumTypeGetNonces(nonceTestAddr, false, 4)
	if err != nil {
		t.Fatalf("provider failure must fall back to the primary RPC, got error: %v", err)
	}
	if pending != 5 {
		t.Errorf("pending = %d, want 5 (declared nonce 4 held over the primary fallback answer)", pending)
	}
}

// TestEthereumTypeGetNonces_PrivatePendingHint_WithConfirmedNonce exercises the exact production
// combination (api/worker.go passes WithConfirmedNonce together with PrivatePendingNonces...): the
// declaration must raise only the PENDING nonce and leave the confirmed (latest) nonce untouched.
func TestEthereumTypeGetNonces_PrivatePendingHint_WithConfirmedNonce(t *testing.T) {
	server := newNonceRPCServer(t, map[string]string{"pending": "0x9", "latest": "0x5"}, nil)
	stub := &nonceBatchStub{results: map[string]string{"pending": "0x4", "latest": "0x2"}}
	// no recent senders → routed purely by the declaration
	b := &EthereumRPC{RPC: stub, Timeout: time.Second, alternativeSendTxProvider: newRecentSenderProvider(server)}

	// declared nonce 9 sits exactly at the provider's pending answer, so the walk advances to 10
	pending, confirmed, confirmedOK, err := b.EthereumTypeGetNonces(nonceTestAddr, true, 9)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pending != 10 {
		t.Errorf("pending = %d, want 10 (declared nonce 9 is in flight)", pending)
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
// relay-deployment feature: with no alternative provider configured there is no private mempool for
// it to describe, so it is ignored and the primary answer stands unchanged.
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

// TestEthereumTypeGetNonces_PrivatePendingHint_WalksCachedAndDeclaredTogether pins that a declared
// nonce and a cached one hold a slot identically: the answer walks across both in one run. Here the
// relay has forgotten the tx it cached at 7 and answers 7, the cache still holds it, and the wallet
// declares the 8 it sent afterwards through another replica - so the next free slot is 9.
func TestEthereumTypeGetNonces_PrivatePendingHint_WalksCachedAndDeclaredTogether(t *testing.T) {
	server := newNonceRPCServer(t, map[string]string{"pending": "0x7"}, nil)
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

	pending, _, _, err := b.EthereumTypeGetNonces(nonceTestAddr, false, 8)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pending != 9 {
		t.Errorf("pending = %d, want 9 (cached 7 then declared 8, walked as one run)", pending)
	}
}

// TestEthereumTypeGetNonces_PrivatePendingHint_DoesNotJumpAHole pins that a declared nonce is held
// to the same contiguity rule as a cached one (#1675). The wallet declares a transaction at 42 while
// the backend still reports 4, so nothing fills 4..41: answering 43 would queue every later send
// behind slots nothing is filling. The answer stays at the backend's, which is a nonce the wallet can
// actually use, and 42 is still never handed out.
func TestEthereumTypeGetNonces_PrivatePendingHint_DoesNotJumpAHole(t *testing.T) {
	server := newNonceRPCServer(t, map[string]string{"pending": "0x4"}, nil)
	stub := &nonceBatchStub{results: map[string]string{"pending": "0x1"}}
	b := &EthereumRPC{RPC: stub, Timeout: time.Second, alternativeSendTxProvider: newRecentSenderProvider(server)}

	pending, _, _, err := b.EthereumTypeGetNonces(nonceTestAddr, false, 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pending != 4 {
		t.Errorf("pending = %d, want 4 (a declared nonce above a hole strands, it does not lift the answer over it)", pending)
	}
}

// TestEthereumTypeGetNonces_PrivatePendingHint_DeclaredSetFillsItsOwnRun covers what Suite actually
// sends: its whole pending-nonce set, sorted. A contiguous declared run walks to just past its end in
// one request, even though this instance cached none of those transactions.
func TestEthereumTypeGetNonces_PrivatePendingHint_DeclaredSetFillsItsOwnRun(t *testing.T) {
	server := newNonceRPCServer(t, map[string]string{"pending": "0x4"}, nil)
	stub := &nonceBatchStub{results: map[string]string{"pending": "0x1"}}
	b := &EthereumRPC{RPC: stub, Timeout: time.Second, alternativeSendTxProvider: newRecentSenderProvider(server)}

	pending, _, _, err := b.EthereumTypeGetNonces(nonceTestAddr, false, 4, 5, 6)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pending != 7 {
		t.Errorf("pending = %d, want 7 (declared 4,5,6 are all in flight)", pending)
	}
}
