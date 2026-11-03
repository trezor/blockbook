package eth

import (
	"bytes"
	"encoding/binary"
	"sync/atomic"

	"github.com/trezor/blockbook/bchain"
)

// addressFormatKeyMax fits an EVM address and a 0x41-prefixed Tron address.
const addressFormatKeyMax = 21

// AddressFormatCache remembers the display form of address descriptors. Formatting an
// address costs a hash (Keccak for EIP-55, double SHA-256 for Tron) and the API formats
// the same few thousand addresses over and over: the address whose page is served, the
// popular token contracts.
//
// It is a direct-mapped table of immutable entries behind atomic pointers: lookups take
// no lock and allocate nothing, a miss overwrites the slot the address maps to. Slots are
// allocated on first use, so a Blockbook of another coin family pays nothing. Because the
// key is the descriptor alone, one cache must serve exactly one format function.
type AddressFormatCache struct {
	mask  uint64
	slots atomic.Pointer[[]atomic.Pointer[addressFormatEntry]]
}

type addressFormatEntry struct {
	key    [addressFormatKeyMax]byte
	keyLen uint8
	value  string
}

// NewAddressFormatCache returns a cache with at least slots entries, rounded up to a power of two.
func NewAddressFormatCache(slots int) *AddressFormatCache {
	n := 1
	for n < slots {
		n <<= 1
	}
	return &AddressFormatCache{mask: uint64(n - 1)}
}

func (c *AddressFormatCache) reset() {
	c.slots.Store(nil)
}

func (c *AddressFormatCache) table() *[]atomic.Pointer[addressFormatEntry] {
	if t := c.slots.Load(); t != nil {
		return t
	}
	t := make([]atomic.Pointer[addressFormatEntry], c.mask+1)
	if c.slots.CompareAndSwap(nil, &t) {
		return &t
	}
	return c.slots.Load()
}

// Format returns the display form of addrDesc, computing it with format on a miss.
func (c *AddressFormatCache) Format(addrDesc bchain.AddressDescriptor, format func(bchain.AddressDescriptor) string) string {
	n := len(addrDesc)
	if n < 8 || n > addressFormatKeyMax {
		return format(addrDesc)
	}
	// addresses are hash-derived, their low bytes are as good a slot index as any hash
	slot := &(*c.table())[binary.LittleEndian.Uint64(addrDesc[n-8:])&c.mask]
	if e := slot.Load(); e != nil && bytes.Equal(e.key[:e.keyLen], addrDesc) {
		return e.value
	}
	e := &addressFormatEntry{keyLen: uint8(n), value: format(addrDesc)}
	copy(e.key[:], addrDesc)
	slot.Store(e)
	return e.value
}
