package coins

import (
	"testing"

	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
)

// stubChain is a minimal bchain.BlockChain that records whether the wrapper
// asked the underlying chain for its sync view. It panics on any method
// that the test does not exercise so accidental fan-out is caught.
type stubChain struct {
	bchain.BlockChain // nil — unmocked methods panic
	syncView          bchain.BlockChain
	syncCalled        bool
}

func (c *stubChain) SyncBlockChain() bchain.BlockChain {
	c.syncCalled = true
	return c.syncView
}

// stubSyncView is a distinct BlockChain instance returned by
// stubChain.SyncBlockChain so the test can verify routing by identity.
type stubSyncView struct {
	bchain.BlockChain
}

// TestBlockChainWithMetricsForwardsSyncBlockChain pins the production-routing
// fix: blockChainWithMetrics must forward SyncBlockChain() so blockbook.go's
// `chain.(bchain.SyncableBlockChain)` assertion succeeds and the SyncWorker
// receives the WS-routed view instead of silently collapsing to the
// HTTP-routed wrapper.
func TestBlockChainWithMetricsForwardsSyncBlockChain(t *testing.T) {
	m, err := common.GetMetrics("blockchain_with_metrics_sync_forward_test")
	if err != nil {
		t.Fatalf("GetMetrics: %v", err)
	}
	view := &stubSyncView{}
	inner := &stubChain{syncView: view}

	wrapped := &blockChainWithMetrics{b: inner, m: m}

	syncable, ok := bchain.BlockChain(wrapped).(bchain.SyncableBlockChain)
	if !ok {
		t.Fatalf("wrapper does not satisfy bchain.SyncableBlockChain — production tipChain wiring would fall back to HTTP")
	}
	got := syncable.SyncBlockChain()
	if !inner.syncCalled {
		t.Fatalf("wrapper did not delegate SyncBlockChain to underlying chain")
	}
	gotWrapped, ok := got.(*blockChainWithMetrics)
	if !ok {
		t.Fatalf("wrapper.SyncBlockChain returned %T, want *blockChainWithMetrics so metrics still observe tip-sync RPC latency", got)
	}
	if gotWrapped.b != bchain.BlockChain(view) {
		t.Fatalf("wrapper.SyncBlockChain wrapped %T, want the inner sync view", gotWrapped.b)
	}
	if gotWrapped.m != m {
		t.Fatalf("wrapper.SyncBlockChain dropped the metrics reference")
	}
}

// chainWithoutSyncView is a bchain.BlockChain that does NOT implement
// SyncableBlockChain (the non-EVM coin case).
type chainWithoutSyncView struct {
	bchain.BlockChain
}

// TestBlockChainWithMetricsSyncBlockChainPassthrough verifies the wrapper
// returns itself when the underlying chain is not Syncable, so callers using
// the optional interface get a stable BlockChain instead of nil.
func TestBlockChainWithMetricsSyncBlockChainPassthrough(t *testing.T) {
	m, err := common.GetMetrics("blockchain_with_metrics_sync_passthrough_test")
	if err != nil {
		t.Fatalf("GetMetrics: %v", err)
	}
	wrapped := &blockChainWithMetrics{b: &chainWithoutSyncView{}, m: m}

	syncable, ok := bchain.BlockChain(wrapped).(bchain.SyncableBlockChain)
	if !ok {
		t.Fatalf("wrapper must always satisfy SyncableBlockChain (returns self when underlying does not)")
	}
	if got := syncable.SyncBlockChain(); got != bchain.BlockChain(wrapped) {
		t.Fatalf("wrapper.SyncBlockChain = %p, want self %p when underlying chain is not Syncable", got, wrapped)
	}
}
