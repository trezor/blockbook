package db

import (
	"container/list"
	"sync"

	"github.com/trezor/blockbook/bchain"
)

// cachedContractsLRUMaxSize bounds the package-level ContractInfo cache.
// At ~250 B per entry, 50k caps the cache around ~12 MB.
const cachedContractsLRUMaxSize = 50_000

type contractInfoLRUEntry struct {
	key         string
	value       *bchain.ContractInfo
	reorgGen    uint64
	protocolGen uint64
}

type contractInfoLRU struct {
	mu       sync.Mutex
	capacity int
	order    *list.List
	items    map[string]*list.Element
}

func newContractInfoLRU(capacity int) *contractInfoLRU {
	if capacity <= 0 {
		return nil
	}
	return &contractInfoLRU{
		capacity: capacity,
		order:    list.New(),
		items:    make(map[string]*list.Element, capacity),
	}
}

// get returns the cached entry only if it was populated under the same
// (reorgGen, protocolGen) the caller now observes. A mismatch on either
// counter misses lazily, so:
//   - a populate-after-delete race during a disconnect (reorgGen bumped) and
//   - a populate-after-write race during a protocol mutation (protocolGen bumped)
//
// both cause the stale entry to be evicted on the next read.
func (c *contractInfoLRU) get(key string, reorgGen, protocolGen uint64) (*bchain.ContractInfo, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return nil, false
	}
	entry := el.Value.(*contractInfoLRUEntry)
	if entry.reorgGen != reorgGen || entry.protocolGen != protocolGen {
		c.order.Remove(el)
		delete(c.items, key)
		return nil, false
	}
	c.order.MoveToFront(el)
	return entry.value, true
}

// add stamps the entry with both counters sampled before the underlying CF
// reads; a subsequent disconnect (reorgGen bump) or protocol write
// (protocolGen bump) forces a miss on the next read.
func (c *contractInfoLRU) add(key string, value *bchain.ContractInfo, reorgGen, protocolGen uint64) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		entry := el.Value.(*contractInfoLRUEntry)
		entry.value = value
		entry.reorgGen = reorgGen
		entry.protocolGen = protocolGen
		c.order.MoveToFront(el)
		return
	}
	el := c.order.PushFront(&contractInfoLRUEntry{key: key, value: value, reorgGen: reorgGen, protocolGen: protocolGen})
	c.items[key] = el
	if c.order.Len() <= c.capacity {
		return
	}
	oldest := c.order.Back()
	if oldest == nil {
		return
	}
	c.order.Remove(oldest)
	delete(c.items, oldest.Value.(*contractInfoLRUEntry).key)
}

func (c *contractInfoLRU) delete(key string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.order.Remove(el)
		delete(c.items, key)
	}
}
