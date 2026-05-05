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

type erc4626CacheEntry struct {
	key   string
	value *Erc4626Token
}

// erc4626Cache is a tiny LRU plus a singleflight group, used to dedupe and
// memoise Erc4626Token results within a block. The two pieces are colocated
// because their lifecycles match: the singleflight key and the cache key are
// the same string, and the cache write happens inside the singleflight
// callback so concurrent requests for one (contract, height) collapse to one
// upstream multicall. Returning a nil token (i.e., the contract turned out
// not to be a vault) is also cached for the duration of the block to avoid
// re-detecting every request for non-vault fungible tokens.
type erc4626Cache struct {
	mu       sync.Mutex
	capacity int
	order    *list.List
	items    map[string]*list.Element
	sf       singleflight.Group
}

var erc4626LiveCache = newErc4626Cache(erc4626CacheCapacity)

func newErc4626Cache(capacity int) *erc4626Cache {
	if capacity <= 0 {
		return nil
	}
	return &erc4626Cache{
		capacity: capacity,
		order:    list.New(),
		items:    make(map[string]*list.Element, capacity),
	}
}

func (c *erc4626Cache) get(key string) (*Erc4626Token, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return nil, false
	}
	c.order.MoveToFront(el)
	return el.Value.(*erc4626CacheEntry).value, true
}

func (c *erc4626Cache) add(key string, value *Erc4626Token) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		el.Value.(*erc4626CacheEntry).value = value
		c.order.MoveToFront(el)
		return
	}
	el := c.order.PushFront(&erc4626CacheEntry{key: key, value: value})
	c.items[key] = el
	if c.order.Len() <= c.capacity {
		return
	}
	oldest := c.order.Back()
	if oldest == nil {
		return
	}
	c.order.Remove(oldest)
	delete(c.items, oldest.Value.(*erc4626CacheEntry).key)
}

func erc4626CacheKey(contract string, blockHeight uint32) string {
	return strings.ToLower(contract) + ":" + strconv.FormatUint(uint64(blockHeight), 10)
}

// erc4626CacheLookupOrBuild returns the cached Erc4626Token for the given key
// or invokes build() exactly once across concurrent callers, caching its
// result. Nil results (non-vaults) are cached for the lifetime of the block to
// suppress repeated detection on plain fungible tokens.
func erc4626CacheLookupOrBuild(cache *erc4626Cache, key string, build func() *Erc4626Token) *Erc4626Token {
	if cache == nil {
		return build()
	}
	if cached, ok := cache.get(key); ok {
		return cached
	}
	v, _, _ := cache.sf.Do(key, func() (interface{}, error) {
		// Re-check inside singleflight: a peer goroutine may have populated.
		if cached, ok := cache.get(key); ok {
			return cached, nil
		}
		result := build()
		cache.add(key, result)
		return result, nil
	})
	if v == nil {
		return nil
	}
	return v.(*Erc4626Token)
}
