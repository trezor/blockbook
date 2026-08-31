package api

import (
	"container/list"
	"sync"
	"time"

	"github.com/golang/glog"
)

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

// negativeProbeCache is an in-memory LRU of recent "the chain says no" probe results
// (not a vault, no token at this address). Not persisted; entries expire after the
// per-add ttlBlocks and on reorgGen mismatch (so a pre-reorg negative misses after
// disconnect). Keys are normalized by the caller, which knows what identifies its probe.
//
// ttlBlocks is supplied per add() rather than fixed at construction so the
// caller can derive it from the chain's averageBlockTimeMs at request time.
// That keeps the user-visible TTL roughly the same wall-clock duration
// across chains regardless of block cadence.
type negativeProbeCacheEntry struct {
	expireAt uint64
	reorgGen uint64
}

type negativeProbeCache struct {
	lru *lruCache[negativeProbeCacheEntry]
}

func newNegativeProbeCache(capacity int) *negativeProbeCache {
	lru := newLRUCache[negativeProbeCacheEntry](capacity)
	if lru == nil {
		return nil
	}
	return &negativeProbeCache{lru: lru}
}

func (c *negativeProbeCache) contains(key string, currentHeight uint32, reorgGen uint64) bool {
	if c == nil || currentHeight == 0 {
		return false
	}
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

func (c *negativeProbeCache) add(key string, currentHeight, ttlBlocks uint32, reorgGen uint64) {
	if c == nil || currentHeight == 0 || ttlBlocks == 0 {
		return
	}
	c.lru.add(key, negativeProbeCacheEntry{
		expireAt: uint64(currentHeight) + uint64(ttlBlocks),
		reorgGen: reorgGen,
	})
}

func (c *negativeProbeCache) remove(key string) {
	if c == nil {
		return
	}
	c.lru.remove(key)
}

// blockTimeProvider exposes the chain's configured average block time so the API can
// convert chain-time settings (negative-cache TTLs) into a per-coin block count at
// request time. Implemented by EVM coins via EthereumRPC.AverageBlockTimeDuration.
type blockTimeProvider interface {
	AverageBlockTimeDuration() (time.Duration, error)
}

// blocksForDuration converts a wall-clock duration to the equivalent
// per-chain block count, rounding up so a duration of "at least N" is honored.
// Returns 0 when either input is non-positive — callers treat 0 as
// "configuration unavailable, skip the time-derived behavior."
func blocksForDuration(d, blockTime time.Duration) uint32 {
	if d <= 0 || blockTime <= 0 {
		return 0
	}
	n := (d + blockTime - 1) / blockTime
	if n < 1 {
		return 1
	}
	return uint32(n)
}

// negativeProbeTTLBlocks resolves a negative-cache TTL to a per-coin block count using
// the chain's configured averageBlockTimeMs. Returns 0 if the chain doesn't expose a
// block time (e.g. non-EVM); the caller treats 0 as "do not negative-cache for this
// request" - a safe fallback that just forfeits the optimization.
func (w *Worker) negativeProbeTTLBlocks(ttl time.Duration) uint32 {
	provider, ok := w.chain.(blockTimeProvider)
	if !ok {
		return 0
	}
	bt, err := provider.AverageBlockTimeDuration()
	if err != nil {
		glog.Warningf("averageBlockTime unavailable, negative probe cache disabled: %v", err)
		return 0
	}
	return blocksForDuration(ttl, bt)
}
