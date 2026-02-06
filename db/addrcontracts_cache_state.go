package db

import (
	"sync"
	"sync/atomic"
	"time"
)

type addrContractsCacheState struct {
	cacheMux  sync.Mutex
	cache     map[string]*unpackedAddrContracts
	cacheMeta map[string]*addrContractsCacheMeta

	hotMux      sync.Mutex
	hot         map[string]*addrContractsHotEntry
	hotSeen     map[string]struct{}
	hotBlock    uint32
	hotLastTime int64

	minSizeBytes    int
	alwaysSizeBytes int
	hotMinScore     float64
	hotHalfLife     time.Duration
	hotEvictAfter   time.Duration
	flushIdle       time.Duration
	flushMaxAge     time.Duration
	enabled         atomic.Bool

	hit               uint64
	miss              uint64
	skipped           uint64
	writeEntries      uint64
	writeBytes        uint64
	cacheWriteEntries uint64
	cacheWriteBytes   uint64
	cacheFlushes      uint64
}
