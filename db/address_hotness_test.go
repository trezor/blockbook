//go:build unittest

package db

import "testing"

func makeHotKey(seed byte) addressHotnessKey {
	var key addressHotnessKey
	for i := range key {
		key[i] = seed
	}
	return key
}

func Test_newAddressHotness_Disabled(t *testing.T) {
	if got := newAddressHotness(0, 1, 1); got != nil {
		t.Fatal("expected nil when minContracts is disabled")
	}
	if got := newAddressHotness(1, 0, 1); got != nil {
		t.Fatal("expected nil when lruSize is disabled")
	}
	if got := newAddressHotness(1, 1, 0); got != nil {
		t.Fatal("expected nil when minHits is disabled")
	}
}

func Test_addressHotness_MinContractsGate(t *testing.T) {
	hot := newAddressHotness(5, 4, 1)
	if hot == nil {
		t.Fatal("expected hotness tracker to be initialized")
	}
	key := makeHotKey(1)

	if hot.ShouldUseIndex(key, 4) {
		t.Fatal("expected contractCount below minContracts to skip index")
	}
	if !hot.ShouldUseIndex(key, 5) {
		t.Fatal("expected hot address to use index once minContracts is met")
	}
}

func Test_addressHotness_HitsPromotionAndBeginBlock(t *testing.T) {
	hot := newAddressHotness(2, 4, 3)
	if hot == nil {
		t.Fatal("expected hotness tracker to be initialized")
	}
	key := makeHotKey(2)
	hot.BeginBlock()

	if hot.ShouldUseIndex(key, 2) {
		t.Fatal("expected first hit to stay cold")
	}
	if hot.ShouldUseIndex(key, 2) {
		t.Fatal("expected second hit to stay cold")
	}
	if !hot.ShouldUseIndex(key, 2) {
		t.Fatal("expected third hit to promote to hot")
	}

	hot.BeginBlock()
	if !hot.ShouldUseIndex(key, 2) {
		t.Fatal("expected hot address to stay hot across blocks")
	}
}

func Test_addressHotness_LRUEviction(t *testing.T) {
	hot := newAddressHotness(1, 2, 1)
	if hot == nil {
		t.Fatal("expected hotness tracker to be initialized")
	}
	a := makeHotKey(10)
	b := makeHotKey(11)
	c := makeHotKey(12)
	hot.BeginBlock()

	if !hot.ShouldUseIndex(a, 1) || !hot.ShouldUseIndex(b, 1) {
		t.Fatal("expected A and B to be promoted to hot")
	}
	// Touch A so B becomes the least-recently used.
	if !hot.ShouldUseIndex(a, 1) {
		t.Fatal("expected A to remain hot after touch")
	}
	// Promote C; should evict B.
	if !hot.ShouldUseIndex(c, 1) {
		t.Fatal("expected C to be promoted to hot")
	}
	if _, ok := hot.lru.items[b]; ok {
		t.Fatal("expected LRU eviction of B after promoting C")
	}
	if _, ok := hot.lru.items[a]; !ok {
		t.Fatal("expected A to remain hot after eviction")
	}
	if _, ok := hot.lru.items[c]; !ok {
		t.Fatal("expected C to be hot after promotion")
	}
}

func Test_addressHotness_Specs(t *testing.T) {
	t.Run("it should reset per-block hits", func(t *testing.T) {
		hot := newAddressHotness(1, 2, 2)
		if hot == nil {
			t.Fatal("expected hotness tracker to be initialized")
		}
		key := makeHotKey(20)
		hot.BeginBlock()
		if hot.ShouldUseIndex(key, 1) {
			t.Fatal("expected first hit to stay cold")
		}
		hot.BeginBlock()
		if hot.ShouldUseIndex(key, 1) {
			t.Fatal("expected hit count to reset between blocks")
		}
	})

	t.Run("it should report a non-empty log suffix after activity", func(t *testing.T) {
		hot := newAddressHotness(1, 2, 1)
		if hot == nil {
			t.Fatal("expected hotness tracker to be initialized")
		}
		key := makeHotKey(24)
		hot.BeginBlock()
		if !hot.ShouldUseIndex(key, 1) {
			t.Fatal("expected promotion to happen")
		}
		if got := hot.LogSuffix(); got == "" {
			t.Fatal("expected log suffix to be non-empty after activity")
		}
	})

	t.Run("it should not use index below minContracts even if hot", func(t *testing.T) {
		hot := newAddressHotness(3, 2, 1)
		if hot == nil {
			t.Fatal("expected hotness tracker to be initialized")
		}
		key := makeHotKey(21)
		hot.BeginBlock()
		if !hot.ShouldUseIndex(key, 3) {
			t.Fatal("expected address to become hot at minContracts")
		}
		if hot.ShouldUseIndex(key, 2) {
			t.Fatal("expected address below minContracts to skip index")
		}
	})

	t.Run("it should promote immediately when minHits is one", func(t *testing.T) {
		hot := newAddressHotness(1, 2, 1)
		if hot == nil {
			t.Fatal("expected hotness tracker to be initialized")
		}
		key := makeHotKey(22)
		hot.BeginBlock()
		if !hot.ShouldUseIndex(key, 1) {
			t.Fatal("expected immediate promotion when minHits is one")
		}
		if _, ok := hot.lru.items[key]; !ok {
			t.Fatal("expected key to be present in LRU after promotion")
		}
	})

	t.Run("it should not add to LRU before minHits", func(t *testing.T) {
		hot := newAddressHotness(1, 2, 3)
		if hot == nil {
			t.Fatal("expected hotness tracker to be initialized")
		}
		key := makeHotKey(23)
		hot.BeginBlock()
		if hot.ShouldUseIndex(key, 1) {
			t.Fatal("expected first hit to stay cold")
		}
		if len(hot.lru.items) != 0 {
			t.Fatal("expected LRU to remain empty before promotion")
		}
		if hot.hits[key] != 1 {
			t.Fatal("expected hit counter to increment before promotion")
		}
	})

	t.Run("it should reject short address descriptors", func(t *testing.T) {
		if _, ok := addressHotnessKeyFromDesc([]byte{1, 2}); ok {
			t.Fatal("expected short address descriptor to be rejected")
		}
	})
}
