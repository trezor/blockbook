package db

import (
	"container/list"
	"sync"
)

// cachedAddressAliasRecordsLRUMaxSize bounds the package-level address alias cache.
// At ~140 B per entry, 100k caps the cache around ~14 MB.
const cachedAddressAliasRecordsLRUMaxSize = 100_000

type addressAliasLRUEntry struct {
	key   string
	value string
}

type addressAliasLRU struct {
	mu       sync.Mutex
	capacity int
	order    *list.List
	items    map[string]*list.Element
}

func newAddressAliasLRU(capacity int) *addressAliasLRU {
	if capacity <= 0 {
		return nil
	}
	return &addressAliasLRU{
		capacity: capacity,
		order:    list.New(),
		items:    make(map[string]*list.Element, capacity),
	}
}

func (c *addressAliasLRU) get(key string) (string, bool) {
	if c == nil {
		return "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return "", false
	}
	c.order.MoveToFront(el)
	return el.Value.(*addressAliasLRUEntry).value, true
}

func (c *addressAliasLRU) add(key, value string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		el.Value.(*addressAliasLRUEntry).value = value
		c.order.MoveToFront(el)
		return
	}
	el := c.order.PushFront(&addressAliasLRUEntry{key: key, value: value})
	c.items[key] = el
	if c.order.Len() <= c.capacity {
		return
	}
	oldest := c.order.Back()
	if oldest == nil {
		return
	}
	c.order.Remove(oldest)
	delete(c.items, oldest.Value.(*addressAliasLRUEntry).key)
}
