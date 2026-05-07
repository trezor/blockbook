package api

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestErc4626Cache_HitAndMiss(t *testing.T) {
	cache := newErc4626Cache(4)
	build := func() (*Erc4626Token, error) { return &Erc4626Token{Error: "first"}, nil }

	got := erc4626CacheLookupOrBuild(cache, "k1", build)
	if got == nil || got.Error != "first" {
		t.Fatalf("first call: got %+v", got)
	}

	// Same key returns cached entry without invoking build.
	called := 0
	again := erc4626CacheLookupOrBuild(cache, "k1", func() (*Erc4626Token, error) {
		called++
		return &Erc4626Token{Error: "second"}, nil
	})
	if called != 0 {
		t.Fatalf("build invoked on cache hit (called=%d)", called)
	}
	if again == nil || again.Error != "first" {
		t.Fatalf("expected cached value, got %+v", again)
	}

	// Different key triggers build.
	other := erc4626CacheLookupOrBuild(cache, "k2", func() (*Erc4626Token, error) {
		return &Erc4626Token{Error: "other"}, nil
	})
	if other == nil || other.Error != "other" {
		t.Fatalf("k2 wrong: %+v", other)
	}
}

func TestErc4626Cache_StoresNil(t *testing.T) {
	cache := newErc4626Cache(4)
	got := erc4626CacheLookupOrBuild(cache, "non-vault", func() (*Erc4626Token, error) { return nil, nil })
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
	// Subsequent call must not re-invoke build for the same key.
	called := 0
	got = erc4626CacheLookupOrBuild(cache, "non-vault", func() (*Erc4626Token, error) {
		called++
		return nil, nil
	})
	if called != 0 {
		t.Fatalf("build invoked on cached nil (called=%d)", called)
	}
	if got != nil {
		t.Fatalf("expected cached nil, got %+v", got)
	}
}

// A transient transport error must surface the value to the caller (so the
// current request still gets a sensible response) without polluting the LRU.
// Two consecutive calls must both invoke build, and the LRU must contain no
// entry for the key after either call.
func TestErc4626Cache_TransportErrorNotCached(t *testing.T) {
	cache := newErc4626Cache(4)
	var calls atomic.Int32
	build := func() (*Erc4626Token, error) {
		calls.Add(1)
		return nil, errors.New("rpc down")
	}

	if got := erc4626CacheLookupOrBuild(cache, "k1", build); got != nil {
		t.Fatalf("expected nil on transport error, got %+v", got)
	}
	if got := erc4626CacheLookupOrBuild(cache, "k1", build); got != nil {
		t.Fatalf("expected nil on transport error, got %+v", got)
	}
	if n := calls.Load(); n != 2 {
		t.Fatalf("transport-errored build must not be cached: expected 2 invocations, got %d", n)
	}
	if _, ok := cache.lru.get("k1"); ok {
		t.Fatal("LRU must not contain an entry for a transport-errored build")
	}

	// A successful follow-up must still land in the cache.
	follow := erc4626CacheLookupOrBuild(cache, "k1", func() (*Erc4626Token, error) {
		return &Erc4626Token{Error: "recovered"}, nil
	})
	if follow == nil || follow.Error != "recovered" {
		t.Fatalf("post-error retry must rebuild and cache, got %+v", follow)
	}
	if _, ok := cache.lru.get("k1"); !ok {
		t.Fatal("LRU must contain an entry after a successful build")
	}
}

// A partial result paired with a transient error (e.g. warm-path multicall RPC
// failed but metadata is populated) must reach the caller without being cached.
func TestErc4626Cache_PartialResultWithErrorNotCached(t *testing.T) {
	cache := newErc4626Cache(4)
	var calls atomic.Int32
	build := func() (*Erc4626Token, error) {
		calls.Add(1)
		return &Erc4626Token{Error: "multicall: rpc down"}, errors.New("multicall: rpc down")
	}

	first := erc4626CacheLookupOrBuild(cache, "k1", build)
	if first == nil || first.Error == "" {
		t.Fatalf("expected partial result returned to caller, got %+v", first)
	}
	second := erc4626CacheLookupOrBuild(cache, "k1", build)
	if second == nil {
		t.Fatal("expected partial result on second call")
	}
	if n := calls.Load(); n != 2 {
		t.Fatalf("partial-with-error must not be cached: expected 2 invocations, got %d", n)
	}
	if _, ok := cache.lru.get("k1"); ok {
		t.Fatal("LRU must not contain an entry for a partial result paired with an error")
	}
}

func TestErc4626Cache_LRUEvictsOldest(t *testing.T) {
	cache := newErc4626Cache(2)
	a := erc4626CacheLookupOrBuild(cache, "a", func() (*Erc4626Token, error) { return &Erc4626Token{Error: "a"}, nil })
	_ = erc4626CacheLookupOrBuild(cache, "b", func() (*Erc4626Token, error) { return &Erc4626Token{Error: "b"}, nil })
	// Touch a to keep it hot.
	_ = erc4626CacheLookupOrBuild(cache, "a", func() (*Erc4626Token, error) { t.Fatal("a should be cached"); return nil, nil })
	// Add c -> b should be evicted, a should remain.
	_ = erc4626CacheLookupOrBuild(cache, "c", func() (*Erc4626Token, error) { return &Erc4626Token{Error: "c"}, nil })
	if v, ok := cache.lru.get("a"); !ok || v != a {
		t.Fatalf("a evicted unexpectedly")
	}
	if _, ok := cache.lru.get("b"); ok {
		t.Fatal("b should have been evicted")
	}
	if _, ok := cache.lru.get("c"); !ok {
		t.Fatal("c not cached")
	}
}

func TestErc4626Cache_SingleflightCollapsesConcurrentCalls(t *testing.T) {
	cache := newErc4626Cache(4)
	const concurrency = 32

	var calls atomic.Int32
	gate := make(chan struct{})
	build := func() (*Erc4626Token, error) {
		calls.Add(1)
		<-gate // hold first caller until peers have all entered Do
		return &Erc4626Token{Error: "shared"}, nil
	}

	var wg sync.WaitGroup
	results := make([]*Erc4626Token, concurrency)
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		i := i
		go func() {
			defer wg.Done()
			results[i] = erc4626CacheLookupOrBuild(cache, "shared-key", build)
		}()
	}
	// Wait for the first builder to enter Do; the singleflight group only
	// dedupes calls that arrive while the first is still in flight. Bounded
	// by a deadline so a regression that prevents calls from ever reaching 1
	// fails the test instead of hanging CI.
	deadline := time.Now().Add(2 * time.Second)
	for calls.Load() < 1 {
		if time.Now().After(deadline) {
			close(gate)
			wg.Wait()
			t.Fatalf("timed out waiting for first builder; calls=%d", calls.Load())
		}
		time.Sleep(time.Millisecond)
	}
	close(gate)
	wg.Wait()

	if got := calls.Load(); got != 1 {
		t.Fatalf("singleflight should have collapsed to 1 build call, got %d", got)
	}
	for i, r := range results {
		if r == nil || r.Error != "shared" {
			t.Fatalf("result[%d] mismatch: %+v", i, r)
		}
	}
}

// Under concurrent first-time access, an errored build must not end up in the
// LRU regardless of how many peers raced into the singleflight group, and a
// follow-up call must rebuild fresh rather than seeing a stale negative.
//
// We deliberately do NOT assert a specific singleflight collapse count here.
// Errored builds are not cached, so any goroutine that reaches Do after the
// in-flight call has returned legitimately starts its own build — the exact
// number of build invocations is scheduler-dependent (especially under -race).
// The cacheable success path is exercised by
// TestErc4626Cache_SingleflightCollapsesConcurrentCalls; this test focuses on
// the policy that distinguishes it: errors must not poison the cache.
func TestErc4626Cache_ConcurrentErrorsDoNotPoisonCache(t *testing.T) {
	cache := newErc4626Cache(4)
	const concurrency = 16

	var calls atomic.Int32
	gate := make(chan struct{})
	build := func() (*Erc4626Token, error) {
		calls.Add(1)
		<-gate
		return nil, errors.New("rpc down")
	}

	var wg sync.WaitGroup
	results := make([]*Erc4626Token, concurrency)
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		i := i
		go func() {
			defer wg.Done()
			results[i] = erc4626CacheLookupOrBuild(cache, "errored-key", build)
		}()
	}
	deadline := time.Now().Add(2 * time.Second)
	for calls.Load() < 1 {
		if time.Now().After(deadline) {
			close(gate)
			wg.Wait()
			t.Fatalf("timed out waiting for first builder; calls=%d", calls.Load())
		}
		time.Sleep(time.Millisecond)
	}
	close(gate)
	wg.Wait()

	if calls.Load() < 1 {
		t.Fatal("expected at least one build invocation")
	}
	for i, r := range results {
		if r != nil {
			t.Fatalf("result[%d] expected nil on errored build, got %+v", i, r)
		}
	}
	if _, ok := cache.lru.get("errored-key"); ok {
		t.Fatal("LRU must not contain an entry for an errored build, even under concurrent load")
	}

	// Post-error: the next caller must rebuild fresh (no stale negative).
	follow := erc4626CacheLookupOrBuild(cache, "errored-key", func() (*Erc4626Token, error) {
		return &Erc4626Token{Error: "recovered"}, nil
	})
	if follow == nil || follow.Error != "recovered" {
		t.Fatalf("post-error retry must rebuild, got %+v", follow)
	}
}

func TestErc4626CacheKey_NormalizesContract(t *testing.T) {
	if a, b := erc4626CacheKey("0xAbCd", 7, 0), erc4626CacheKey("0xabcd", 7, 0); a != b {
		t.Fatalf("expected case-insensitive key, got %q vs %q", a, b)
	}
	if a, b := erc4626CacheKey("0xabcd", 7, 0), erc4626CacheKey("0xabcd", 8, 0); a == b {
		t.Fatal("different heights must yield different keys")
	}
	if a, b := erc4626CacheKey("0xabcd", 7, 0), erc4626CacheKey("0xabcd", 7, 1); a == b {
		t.Fatal("different reorg generations must yield different keys")
	}
}

func TestErc4626CacheLookupOrBuild_NilCacheFallsThrough(t *testing.T) {
	called := 0
	got := erc4626CacheLookupOrBuild(nil, "k", func() (*Erc4626Token, error) {
		called++
		return &Erc4626Token{Error: "bypass"}, nil
	})
	if called != 1 || got == nil || got.Error != "bypass" {
		t.Fatalf("nil cache should bypass: called=%d got=%+v", called, got)
	}

	// Nil cache also drops the build error and surfaces the value (matches the
	// no-bestHeight path in buildErc4626Token, which has no cache to skip).
	called = 0
	got = erc4626CacheLookupOrBuild(nil, "k2", func() (*Erc4626Token, error) {
		called++
		return &Erc4626Token{Error: "partial"}, errors.New("transient")
	})
	if called != 1 || got == nil || got.Error != "partial" {
		t.Fatalf("nil cache should still pass through partial result on error: called=%d got=%+v", called, got)
	}
}

func TestErc4626NegativeProbeCache_HitExpireAndRemove(t *testing.T) {
	cache := newErc4626NegativeCache(2)
	const ttl = uint32(2)
	if cache.contains("0xabc", 10, 0) {
		t.Fatal("empty cache should miss")
	}

	cache.add("0xAbC", 10, ttl, 0)
	if !cache.contains("0xabc", 10, 0) {
		t.Fatal("expected hit at insertion height")
	}
	if !cache.contains("0xABC", 12, 0) {
		t.Fatal("expected hit before expiry")
	}
	if cache.contains("0xabc", 13, 0) {
		t.Fatal("expected miss after expiry")
	}

	cache.add("0xabc", 20, ttl, 0)
	cache.remove("0xABC")
	if cache.contains("0xabc", 20, 0) {
		t.Fatal("expected miss after explicit remove")
	}
}

func TestErc4626NegativeProbeCache_ZeroTTLBlocksIsNoOp(t *testing.T) {
	// ttlBlocks == 0 represents "chain block time unavailable" — the cache
	// must drop the add silently and treat it as a miss on lookup.
	cache := newErc4626NegativeCache(2)
	cache.add("0xabc", 10, 0, 0)
	if cache.contains("0xabc", 10, 0) {
		t.Fatal("entry inserted with ttlBlocks==0 should be absent")
	}
}

func TestErc4626NegativeProbeCache_ReorgGenInvalidates(t *testing.T) {
	cache := newErc4626NegativeCache(2)
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

func TestErc4626BlocksForDuration(t *testing.T) {
	// 15 minutes / 12s blocks → 75 blocks (Ethereum).
	if got := erc4626BlocksForDuration(15*time.Minute, 12*time.Second); got != 75 {
		t.Fatalf("Ethereum: got %d, want 75", got)
	}
	// 15 minutes / 250ms blocks → 3600 blocks (Arbitrum).
	if got := erc4626BlocksForDuration(15*time.Minute, 250*time.Millisecond); got != 3600 {
		t.Fatalf("Arbitrum: got %d, want 3600", got)
	}
	// Rounding up: 1ns under a clean block boundary still uses one full block.
	if got := erc4626BlocksForDuration(13*time.Second, 12*time.Second); got != 2 {
		t.Fatalf("ceil division: got %d, want 2", got)
	}
	// Zero / negative inputs disable the optimization.
	if got := erc4626BlocksForDuration(0, time.Second); got != 0 {
		t.Fatalf("zero duration must yield 0, got %d", got)
	}
	if got := erc4626BlocksForDuration(time.Minute, 0); got != 0 {
		t.Fatalf("zero blockTime must yield 0, got %d", got)
	}
}
