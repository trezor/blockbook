package api

import (
	"hash/fnv"
	"sync"
)

// Serialize getXpubData per descriptor so concurrent requests for the same xpub
// never mutate one shared cache entry; sharding keeps distinct xpubs parallel.
const xpubUpdateShards = 256

var xpubUpdateLocks [xpubUpdateShards]sync.Mutex

func xpubUpdateLock(descriptor string) *sync.Mutex {
	h := fnv.New32a()
	h.Write([]byte(descriptor))
	return &xpubUpdateLocks[h.Sum32()%xpubUpdateShards]
}

// Private copy of the address slices so the update phase leaves any previously
// published snapshot immutable for concurrent readers. Shallow is enough: each
// entry's txids/balance are only ever reassigned, never mutated in place.
func cloneXpubAddresses(src [][]xpubAddress) [][]xpubAddress {
	dst := make([][]xpubAddress, len(src))
	for i, inner := range src {
		cp := make([]xpubAddress, len(inner))
		copy(cp, inner)
		dst[i] = cp
	}
	return dst
}
