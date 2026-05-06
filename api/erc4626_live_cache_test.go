package api

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestErc4626Cache_HitAndMiss(t *testing.T) {
	cache := newErc4626Cache(4)
	build := func() *Erc4626Token { return &Erc4626Token{Error: "first"} }

	got := erc4626CacheLookupOrBuild(cache, "k1", build)
	if got == nil || got.Error != "first" {
		t.Fatalf("first call: got %+v", got)
	}

	// Same key returns cached entry without invoking build.
	called := 0
	again := erc4626CacheLookupOrBuild(cache, "k1", func() *Erc4626Token {
		called++
		return &Erc4626Token{Error: "second"}
	})
	if called != 0 {
		t.Fatalf("build invoked on cache hit (called=%d)", called)
	}
	if again == nil || again.Error != "first" {
		t.Fatalf("expected cached value, got %+v", again)
	}

	// Different key triggers build.
	other := erc4626CacheLookupOrBuild(cache, "k2", func() *Erc4626Token {
		return &Erc4626Token{Error: "other"}
	})
	if other == nil || other.Error != "other" {
		t.Fatalf("k2 wrong: %+v", other)
	}
}

func TestErc4626Cache_StoresNil(t *testing.T) {
	cache := newErc4626Cache(4)
	got := erc4626CacheLookupOrBuild(cache, "non-vault", func() *Erc4626Token { return nil })
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
	// Subsequent call must not re-invoke build for the same key.
	called := 0
	got = erc4626CacheLookupOrBuild(cache, "non-vault", func() *Erc4626Token {
		called++
		return nil
	})
	if called != 0 {
		t.Fatalf("build invoked on cached nil (called=%d)", called)
	}
	if got != nil {
		t.Fatalf("expected cached nil, got %+v", got)
	}
}

func TestErc4626Cache_LRUEvictsOldest(t *testing.T) {
	cache := newErc4626Cache(2)
	a := erc4626CacheLookupOrBuild(cache, "a", func() *Erc4626Token { return &Erc4626Token{Error: "a"} })
	_ = erc4626CacheLookupOrBuild(cache, "b", func() *Erc4626Token { return &Erc4626Token{Error: "b"} })
	// Touch a to keep it hot.
	_ = erc4626CacheLookupOrBuild(cache, "a", func() *Erc4626Token { t.Fatal("a should be cached"); return nil })
	// Add c -> b should be evicted, a should remain.
	_ = erc4626CacheLookupOrBuild(cache, "c", func() *Erc4626Token { return &Erc4626Token{Error: "c"} })
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
	build := func() *Erc4626Token {
		calls.Add(1)
		<-gate // hold first caller until peers have all entered Do
		return &Erc4626Token{Error: "shared"}
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
	// Give the goroutines time to enter Do; the singleflight group only
	// dedupes calls that arrive while the first is still in flight.
	for {
		if calls.Load() == 1 {
			break
		}
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

func TestErc4626CacheKey_NormalizesContract(t *testing.T) {
	if a, b := erc4626CacheKey("0xAbCd", 7), erc4626CacheKey("0xabcd", 7); a != b {
		t.Fatalf("expected case-insensitive key, got %q vs %q", a, b)
	}
	if a, b := erc4626CacheKey("0xabcd", 7), erc4626CacheKey("0xabcd", 8); a == b {
		t.Fatal("different heights must yield different keys")
	}
}

func TestErc4626CacheLookupOrBuild_NilCacheFallsThrough(t *testing.T) {
	called := 0
	got := erc4626CacheLookupOrBuild(nil, "k", func() *Erc4626Token {
		called++
		return &Erc4626Token{Error: "bypass"}
	})
	if called != 1 || got == nil || got.Error != "bypass" {
		t.Fatalf("nil cache should bypass: called=%d got=%+v", called, got)
	}
}

func TestErc4626NegativeProbeCache_HitExpireAndRemove(t *testing.T) {
	cache := newErc4626NegativeCache(2, 2)
	if cache.contains("0xabc", 10) {
		t.Fatal("empty cache should miss")
	}

	cache.add("0xAbC", 10)
	if !cache.contains("0xabc", 10) {
		t.Fatal("expected hit at insertion height")
	}
	if !cache.contains("0xABC", 12) {
		t.Fatal("expected hit before expiry")
	}
	if cache.contains("0xabc", 13) {
		t.Fatal("expected miss after expiry")
	}

	cache.add("0xabc", 20)
	cache.remove("0xABC")
	if cache.contains("0xabc", 20) {
		t.Fatal("expected miss after explicit remove")
	}
}
