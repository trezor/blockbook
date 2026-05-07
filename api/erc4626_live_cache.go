package api

import (
	"container/list"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/sync/singleflight"
)

// erc4626CacheCapacity bounds the live-values cache, keyed by
// (contract, height, reorgGen). Old entries age out as best-block advances.
const erc4626CacheCapacity = 1024
const erc4626NegativeProbeCacheCapacity = 4096

var erc4626LiveCache = newErc4626Cache(erc4626CacheCapacity)
var erc4626NegativeProbeCache = newErc4626NegativeCache(erc4626NegativeProbeCacheCapacity)

// lruCache is a string-keyed LRU shared by the live-values and negative
// caches. Methods are nil-safe so a disabled (capacity<=0) cache no-ops.
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

// erc4626Cache memoises Erc4626Token (including nil for non-vaults) per
// (contract, height, gen); singleflight dedupes concurrent builds.
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

// erc4626CacheKey scopes entries by (contract, height, reorgGen) so a
// same-height reorg invalidates pre-reorg entries via key mismatch.
func erc4626CacheKey(contract string, blockHeight uint32, reorgGen uint64) string {
	return erc4626ContractKey(contract) + ":" + strconv.FormatUint(uint64(blockHeight), 10) + ":" + strconv.FormatUint(reorgGen, 10)
}

// erc4626CacheLookupOrBuild returns the cached token, or runs build() once
// across concurrent callers via singleflight. build's error is a cache-policy
// signal: nil ⇒ memoise; non-nil ⇒ skip cache (so a transient failure doesn't
// poison detection for the rest of the block). Callers see only the token.
func erc4626CacheLookupOrBuild(cache *erc4626Cache, key string, build func() (*Erc4626Token, error)) *Erc4626Token {
	if cache == nil {
		token, _ := build()
		return token
	}
	if cached, ok := cache.lru.get(key); ok {
		return cached
	}
	v, _, _ := cache.sf.Do(key, func() (interface{}, error) {
		// Re-check: a peer may have populated while we waited to enter Do.
		if cached, ok := cache.lru.get(key); ok {
			return cached, nil
		}
		token, err := build()
		if err == nil {
			cache.lru.add(key, token)
		}
		// Never echo build's error to waiters; they want the token.
		return token, nil
	})
	if v == nil {
		return nil
	}
	return v.(*Erc4626Token)
}

func erc4626ContractKey(contract string) string {
	return strings.ToLower(contract)
}

// erc4626NegativeCache is an in-memory LRU of recent "not a vault" results
// for accountInfo. Not persisted; entries expire after the per-add ttlBlocks
// and on reorgGen mismatch (so a pre-reorg negative misses after disconnect).
//
// ttlBlocks is supplied per add() rather than fixed at construction so the
// caller can derive it from the chain's averageBlockTimeMs at request time.
// That keeps the user-visible TTL roughly the same wall-clock duration
// across chains regardless of block cadence.
type erc4626NegativeCacheEntry struct {
	expireAt uint64
	reorgGen uint64
}

type erc4626NegativeCache struct {
	lru *lruCache[erc4626NegativeCacheEntry]
}

func newErc4626NegativeCache(capacity int) *erc4626NegativeCache {
	lru := newLRUCache[erc4626NegativeCacheEntry](capacity)
	if lru == nil {
		return nil
	}
	return &erc4626NegativeCache{lru: lru}
}

func (c *erc4626NegativeCache) contains(contract string, currentHeight uint32, reorgGen uint64) bool {
	if c == nil || currentHeight == 0 {
		return false
	}
	key := erc4626ContractKey(contract)
	entry, ok := c.lru.get(key)
	if !ok {
		return false
	}
	if entry.reorgGen != reorgGen || uint64(currentHeight) > entry.expireAt {
		c.lru.remove(key)
		return false
	}
	return true
}

func (c *erc4626NegativeCache) add(contract string, currentHeight, ttlBlocks uint32, reorgGen uint64) {
	if c == nil || currentHeight == 0 || ttlBlocks == 0 {
		return
	}
	c.lru.add(erc4626ContractKey(contract), erc4626NegativeCacheEntry{
		expireAt: uint64(currentHeight) + uint64(ttlBlocks),
		reorgGen: reorgGen,
	})
}

func (c *erc4626NegativeCache) remove(contract string) {
	if c == nil {
		return
	}
	c.lru.remove(erc4626ContractKey(contract))
}
