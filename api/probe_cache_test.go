package api

import (
	"testing"
	"time"
)

func TestNegativeProbeCache_HitExpireAndRemove(t *testing.T) {
	cache := newNegativeProbeCache(2)
	const ttl = uint32(2)
	if cache.contains("0xabc", 10, 0) {
		t.Fatal("empty cache should miss")
	}

	cache.add("0xabc", 10, ttl, 0)
	if !cache.contains("0xabc", 10, 0) {
		t.Fatal("expected hit at insertion height")
	}
	if !cache.contains("0xabc", 12, 0) {
		t.Fatal("expected hit before expiry")
	}
	if cache.contains("0xabc", 13, 0) {
		t.Fatal("expected miss after expiry")
	}

	cache.add("0xabc", 20, ttl, 0)
	cache.remove("0xabc")
	if cache.contains("0xabc", 20, 0) {
		t.Fatal("expected miss after explicit remove")
	}
}

func TestNegativeProbeCache_ZeroTTLBlocksIsNoOp(t *testing.T) {
	// ttlBlocks == 0 represents "chain block time unavailable" — the cache
	// must drop the add silently and treat it as a miss on lookup.
	cache := newNegativeProbeCache(2)
	cache.add("0xabc", 10, 0, 0)
	if cache.contains("0xabc", 10, 0) {
		t.Fatal("entry inserted with ttlBlocks==0 should be absent")
	}
}

func TestNegativeProbeCache_ReorgGenInvalidates(t *testing.T) {
	cache := newNegativeProbeCache(2)
	const ttl = uint32(100)
	cache.add("0xabc", 10, ttl, 7)
	if !cache.contains("0xabc", 10, 7) {
		t.Fatal("hit on matching reorg generation expected")
	}
	if cache.contains("0xabc", 10, 8) {
		t.Fatal("entry from older reorg generation must miss")
	}
	// the mismatched-gen lookup also evicts the entry, so a same-gen reprobe sees a fresh miss
	if cache.contains("0xabc", 10, 7) {
		t.Fatal("entry should have been evicted on reorg-gen mismatch")
	}
}

func TestBlocksForDuration(t *testing.T) {
	// 15 minutes / 12s blocks → 75 blocks (Ethereum).
	if got := blocksForDuration(15*time.Minute, 12*time.Second); got != 75 {
		t.Fatalf("Ethereum: got %d, want 75", got)
	}
	// 15 minutes / 250ms blocks → 3600 blocks (Arbitrum).
	if got := blocksForDuration(15*time.Minute, 250*time.Millisecond); got != 3600 {
		t.Fatalf("Arbitrum: got %d, want 3600", got)
	}
	// Rounding up: 1ns under a clean block boundary still uses one full block.
	if got := blocksForDuration(13*time.Second, 12*time.Second); got != 2 {
		t.Fatalf("ceil division: got %d, want 2", got)
	}
	// Zero / negative inputs disable the optimization.
	if got := blocksForDuration(0, time.Second); got != 0 {
		t.Fatalf("zero duration must yield 0, got %d", got)
	}
	if got := blocksForDuration(time.Minute, 0); got != 0 {
		t.Fatalf("zero blockTime must yield 0, got %d", got)
	}
}
