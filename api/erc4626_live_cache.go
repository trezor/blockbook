package api

import (
	"container/list"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/sync/singleflight"
)

// erc4626CacheCapacity bounds the package-level live-values cache. The cache
// is keyed by (contract, blockHeight); old block entries age out of the LRU as
// best-block advances and new requests displace them. Sizing is a function of
// active vault count per block, not request volume - 1024 covers far more
// distinct vaults than any wallet portfolio actually holds.
const erc4626CacheCapacity = 1024
const erc4626NegativeProbeCacheCapacity = 4096
const erc4626NegativeProbeTTLBlocks = 256

var erc4626LiveCache = newErc4626Cache(erc4626CacheCapacity)
var erc4626NegativeProbeCache = newErc4626NegativeCache(erc4626NegativeProbeCacheCapacity, erc4626NegativeProbeTTLBlocks)

// lruCache is a small string-keyed LRU shared by the live-values and
// negative-probe caches in this file. Methods are nil-safe so a disabled cache
// (newX(0)) silently no-ops.
type lruCache[V any] struct {
	mu       sync.Mutex
	capacity int
	order    *list.List
	items    map[string]*list.Element
}

type lruEntry[V any] struct {
	key   string
	value V
}

func newLRUCache[V any](capacity int) *lruCache[V] {
	if capacity <= 0 {
		return nil
	}
	return &lruCache[V]{
		capacity: capacity,
		order:    list.New(),
		items:    make(map[string]*list.Element, capacity),
	}
}

func (c *lruCache[V]) get(key string) (V, bool) {
	var zero V
	if c == nil {
		return zero, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return zero, false
	}
	c.order.MoveToFront(el)
	return el.Value.(*lruEntry[V]).value, true
}

func (c *lruCache[V]) add(key string, value V) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		el.Value.(*lruEntry[V]).value = value
		c.order.MoveToFront(el)
		return
	}
	el := c.order.PushFront(&lruEntry[V]{key: key, value: value})
	c.items[key] = el
	if c.order.Len() <= c.capacity {
		return
	}
	oldest := c.order.Back()
	if oldest == nil {
		return
	}
	c.order.Remove(oldest)
	delete(c.items, oldest.Value.(*lruEntry[V]).key)
}

func (c *lruCache[V]) remove(key string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return
	}
	c.order.Remove(el)
	delete(c.items, key)
}

// erc4626Cache memoises Erc4626Token results within a block, with singleflight
// collapsing concurrent requests for the same (contract, height) into one
// upstream multicall. Nil tokens (non-vaults) are cached too, to suppress
// re-detection of plain fungible tokens within the block.
type erc4626Cache struct {
	lru *lruCache[*Erc4626Token]
	sf  singleflight.Group
}

func newErc4626Cache(capacity int) *erc4626Cache {
	lru := newLRUCache[*Erc4626Token](capacity)
	if lru == nil {
		return nil
	}
	return &erc4626Cache{lru: lru}
}

func erc4626CacheKey(contract string, blockHeight uint32) string {
	return erc4626ContractKey(contract) + ":" + strconv.FormatUint(uint64(blockHeight), 10)
}

// erc4626CacheLookupOrBuild returns the cached Erc4626Token for the given key
// or invokes build() exactly once across concurrent callers, caching its
// result. Nil results (non-vaults) are cached for the lifetime of the block to
// suppress repeated detection on plain fungible tokens.
func erc4626CacheLookupOrBuild(cache *erc4626Cache, key string, build func() *Erc4626Token) *Erc4626Token {
	if cache == nil {
		return build()
	}
	if cached, ok := cache.lru.get(key); ok {
		return cached
	}
	v, _, _ := cache.sf.Do(key, func() (interface{}, error) {
		// Re-check inside singleflight: a peer goroutine may have populated.
		if cached, ok := cache.lru.get(key); ok {
			return cached, nil
		}
		result := build()
		cache.lru.add(key, result)
		return result, nil
	})
	if v == nil {
		return nil
	}
	return v.(*Erc4626Token)
}

func erc4626ContractKey(contract string) string {
	return strings.ToLower(contract)
}

// erc4626NegativeCache is a tiny in-memory LRU of recent "not a vault"
// probe results for the accountInfo path. Unlike positive detections, negatives
// are not persisted to DB; they expire after a bounded number of indexed blocks
// so upgradeable or newly-activated contracts will eventually be re-probed.
type erc4626NegativeCache struct {
	lru       *lruCache[uint64]
	ttlBlocks uint32
}

func newErc4626NegativeCache(capacity int, ttlBlocks uint32) *erc4626NegativeCache {
	lru := newLRUCache[uint64](capacity)
	if lru == nil || ttlBlocks == 0 {
		return nil
	}
	return &erc4626NegativeCache{lru: lru, ttlBlocks: ttlBlocks}
}

func (c *erc4626NegativeCache) contains(contract string, currentHeight uint32) bool {
	if c == nil || currentHeight == 0 {
		return false
	}
	key := erc4626ContractKey(contract)
	expireAt, ok := c.lru.get(key)
	if !ok {
		return false
	}
	if uint64(currentHeight) > expireAt {
		c.lru.remove(key)
		return false
	}
	return true
}

func (c *erc4626NegativeCache) add(contract string, currentHeight uint32) {
	if c == nil || currentHeight == 0 {
		return
	}
	c.lru.add(erc4626ContractKey(contract), uint64(currentHeight)+uint64(c.ttlBlocks))
}

func (c *erc4626NegativeCache) remove(contract string) {
	if c == nil {
		return
	}
	c.lru.remove(erc4626ContractKey(contract))
}
