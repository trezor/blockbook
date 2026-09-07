package api

import (
	"strconv"
	"strings"

	"golang.org/x/sync/singleflight"
)

// erc4626CacheCapacity bounds the live-values cache, keyed by
// (contract, height, reorgGen). Old entries age out as best-block advances.
const erc4626CacheCapacity = 1024
const erc4626NegativeProbeCacheCapacity = 4096

var erc4626LiveCache = newErc4626Cache(erc4626CacheCapacity)
var erc4626NegativeProbeCache = newNegativeProbeCache(erc4626NegativeProbeCacheCapacity)

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
