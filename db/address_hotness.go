package db

import (
	"container/list"
	"fmt"

	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
)

type hotAddressConfigProvider interface {
	HotAddressConfig() (minContracts, lruSize, minHits int)
}

type addressContractsCacheConfigProvider interface {
	AddressContractsCacheConfig() eth.AddressContractsCacheConfig
}

type addressHotnessKey [eth.EthereumTypeAddressDescriptorLen]byte

func addressHotnessKeyFromDesc(addr bchain.AddressDescriptor) (addressHotnessKey, bool) {
	var key addressHotnessKey
	if len(addr) != len(key) {
		return key, false
	}
	copy(key[:], addr)
	return key, true
}

type addressHotness struct {
	minContracts int
	minHits      int
	lru          *hotAddressLRU
	onEvict      func(addressHotnessKey)
	// hits tracks lookup counts so we can decide when an address is hot. Counts
	// accumulate across blocks so an address that recurs over several blocks (not
	// only within one busy block) can become hot; the map is reset in BeginBlock
	// once it grows past maxPendingHits to keep memory bounded.
	hits map[addressHotnessKey]uint16
	// maxPendingHits bounds the hits map (set to the LRU size, the natural ceiling
	// for promotion candidates).
	maxPendingHits int
	// block stats (reset after reporting) to keep logging cheap.
	// blockEligibleLookups counts lookups with contractCount >= minContracts (i.e., eligible for hotness).
	blockEligibleLookups uint64
	// blockLRUHits counts eligible lookups that hit an already-hot address in the LRU.
	blockLRUHits uint64
	// blockPromotions counts addresses promoted to hot (minHits reached) in the current block.
	blockPromotions uint64
	// blockEvictions counts LRU evictions triggered by promotions in the current block.
	blockEvictions uint64
}

func newAddressHotness(minContracts, lruSize, minHits int) *addressHotness {
	if minContracts <= 0 || lruSize <= 0 || minHits <= 0 {
		return nil
	}
	return &addressHotness{
		minContracts:   minContracts,
		minHits:        minHits,
		lru:            newHotAddressLRU(lruSize),
		maxPendingHits: lruSize,
		hits:           make(map[addressHotnessKey]uint16),
	}
}

func newAddressHotnessFromParser(parser bchain.BlockChainParser) *addressHotness {
	cfg, ok := parser.(hotAddressConfigProvider)
	if !ok {
		return nil
	}
	minContracts, lruSize, minHits := cfg.HotAddressConfig()
	return newAddressHotness(minContracts, lruSize, minHits)
}

func (h *addressHotness) BeginBlock() {
	if h == nil {
		return
	}
	// Hit counts accumulate across blocks so addresses looked up repeatedly over
	// several blocks (not only within one busy block) can become hot — this lets
	// the index help lower-throughput chains, not just very busy ones. Reset only
	// when the candidate map grows past its bound; dropping pending counts merely
	// delays a promotion and never affects correctness (lookups fall back to a
	// linear scan when the index is not used). The LRU survives across blocks.
	if len(h.hits) > h.maxPendingHits {
		// Reinitialize rather than clear(): Go's clear() does not shrink a map's
		// underlying bucket allocation, so a single oversized block would leave the
		// allocation at its high-water mark. Pre-size to maxPendingHits, the
		// steady-state ceiling, so the oversized buckets can be released.
		h.hits = make(map[addressHotnessKey]uint16, h.maxPendingHits)
	}
	// Reset per-block stats counters (metrics report per-block deltas).
	h.blockEligibleLookups = 0
	h.blockLRUHits = 0
	h.blockPromotions = 0
	h.blockEvictions = 0
}

func (h *addressHotness) ShouldUseIndex(addrKey addressHotnessKey, contractCount int) bool {
	if h == nil || contractCount < h.minContracts {
		return false
	}
	h.blockEligibleLookups++
	// Rule B: once an address is hot, reuse the index immediately.
	if h.lru != nil && h.lru.touch(addrKey) {
		h.blockLRUHits++
		return true
	}
	// Count hits within the current block; once minHits is reached, promote to LRU.
	hits := h.hits[addrKey] + 1
	if hits < uint16(h.minHits) {
		h.hits[addrKey] = hits
		return false
	}
	delete(h.hits, addrKey)
	if h.lru != nil {
		// Promotion: once hot, an address stays hot until evicted by LRU capacity.
		if evictedKey, evicted := h.lru.add(addrKey); evicted {
			h.blockEvictions++
			if h.onEvict != nil {
				h.onEvict(evictedKey)
			}
		}
		h.blockPromotions++
	}
	return true
}

func (h *addressHotness) LogSuffix() string {
	if h == nil {
		return ""
	}
	if h.blockEligibleLookups == 0 && h.blockLRUHits == 0 && h.blockPromotions == 0 && h.blockEvictions == 0 {
		return ""
	}
	hitRate := 0.0
	if h.blockEligibleLookups > 0 {
		hitRate = float64(h.blockLRUHits) / float64(h.blockEligibleLookups)
	}
	return fmt.Sprintf(", hotness[eligible_lookups=%d, lru_hits=%d, promotions=%d, evictions=%d, hit_rate=%.3f]",
		h.blockEligibleLookups, h.blockLRUHits, h.blockPromotions, h.blockEvictions, hitRate)
}

func (h *addressHotness) Stats() (eligible, hits, promotions, evictions uint64) {
	if h == nil {
		return 0, 0, 0, 0
	}
	return h.blockEligibleLookups, h.blockLRUHits, h.blockPromotions, h.blockEvictions
}

type hotAddressLRU struct {
	capacity int
	order    *list.List
	items    map[addressHotnessKey]*list.Element
}

func newHotAddressLRU(capacity int) *hotAddressLRU {
	if capacity <= 0 {
		return nil
	}
	return &hotAddressLRU{
		capacity: capacity,
		order:    list.New(),
		// items maps address -> list element; the list order is MRU->LRU.
		items: make(map[addressHotnessKey]*list.Element, capacity),
	}
}

func (l *hotAddressLRU) touch(key addressHotnessKey) bool {
	if l == nil {
		return false
	}
	if el, ok := l.items[key]; ok {
		// Hot: move to front so it won't be evicted soon.
		l.order.MoveToFront(el)
		return true
	}
	return false
}

func (l *hotAddressLRU) add(key addressHotnessKey) (addressHotnessKey, bool) {
	var zero addressHotnessKey
	if l == nil {
		return zero, false
	}
	if el, ok := l.items[key]; ok {
		// Already hot; refresh recency.
		l.order.MoveToFront(el)
		return zero, false
	}
	el := l.order.PushFront(key)
	l.items[key] = el
	if l.order.Len() <= l.capacity {
		return zero, false
	}
	// Evict the least-recently used hot address.
	oldest := l.order.Back()
	if oldest == nil {
		return zero, false
	}
	evictedKey := oldest.Value.(addressHotnessKey)
	l.order.Remove(oldest)
	delete(l.items, evictedKey)
	return evictedKey, true
}
