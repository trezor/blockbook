package eth

import (
	"sync"

	"github.com/trezor/blockbook/bchain"
)

const addressFormatShards = 64

// AddressFormatCache remembers the display form of address descriptors. Formatting an
// address costs a hash (Keccak for EIP-55, double SHA-256 for Tron) and the API formats
// the same few thousand addresses over and over: the address whose page is served, the
// popular token contracts. Each shard keeps a current and a previous generation and drops
// the previous one when the current fills up, which bounds the size without bookkeeping.
type AddressFormatCache struct {
	shards       [addressFormatShards]addressFormatShard
	shardEntries int
}

type addressFormatShard struct {
	mu       sync.RWMutex
	cur, old map[string]string
}

// NewAddressFormatCache returns a cache holding at least entries formatted addresses.
func NewAddressFormatCache(entries int) *AddressFormatCache {
	c := &AddressFormatCache{shardEntries: max(entries/addressFormatShards, 1)}
	c.reset()
	return c
}

func (c *AddressFormatCache) reset() {
	for i := range c.shards {
		s := &c.shards[i]
		s.mu.Lock()
		s.cur = make(map[string]string, c.shardEntries)
		s.old = nil
		s.mu.Unlock()
	}
}

// Format returns the display form of addrDesc, computing it with format on a miss.
func (c *AddressFormatCache) Format(addrDesc bchain.AddressDescriptor, format func(bchain.AddressDescriptor) string) string {
	if len(addrDesc) == 0 {
		return format(addrDesc)
	}
	s := &c.shards[addrDesc[len(addrDesc)-1]%addressFormatShards]
	s.mu.RLock()
	v, ok := s.cur[string(addrDesc)]
	if !ok {
		v, ok = s.old[string(addrDesc)]
	}
	s.mu.RUnlock()
	if ok {
		return v
	}
	v = format(addrDesc)
	s.mu.Lock()
	if len(s.cur) >= c.shardEntries {
		s.old, s.cur = s.cur, make(map[string]string, c.shardEntries)
	}
	s.cur[string(addrDesc)] = v
	s.mu.Unlock()
	return v
}
