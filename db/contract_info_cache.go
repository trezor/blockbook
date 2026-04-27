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
	key   string
	value *bchain.ContractInfo
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

func (c *contractInfoLRU) get(key string) (*bchain.ContractInfo, bool) {
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
	return el.Value.(*contractInfoLRUEntry).value, true
}

func (c *contractInfoLRU) add(key string, value *bchain.ContractInfo) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		el.Value.(*contractInfoLRUEntry).value = value
		c.order.MoveToFront(el)
		return
	}
	el := c.order.PushFront(&contractInfoLRUEntry{key: key, value: value})
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
