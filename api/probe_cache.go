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

// defaultNegativeProbeTTL bounds how long a "the chain says no" verdict is trusted, so a later
// change (CREATE2 redeploy, proxy upgrade) is still picked up. Wall-clock so it is chain-agnostic.
const defaultNegativeProbeTTL = 15 * time.Minute

type negativeProbeCacheEntry struct {
	expireAt uint64
	reorgGen uint64
}

// negativeProbeCache is an in-memory LRU of "the chain says no" verdicts, keyed by whatever the
// caller uses to identify a probe; entries expire on ttlBlocks and on a reorgGen mismatch.
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

// blockTimeProvider exposes the chain's configured average block time, so wall-clock settings can
// be converted to a per-coin block count at request time. Implemented by EVM coins.
type blockTimeProvider interface {
	AverageBlockTimeDuration() (time.Duration, error)
}

// blocksForDuration converts a wall-clock duration to a block count, rounding up so "at least d" is
// honored. Returns 0 on non-positive input - callers read 0 as "unavailable, skip the behavior".
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

// negativeProbeTTLBlocks resolves a TTL to a per-coin block count. Returns 0 when the chain exposes
// no block time (e.g. non-EVM); the caller then just forfeits the negative cache for this request.
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
