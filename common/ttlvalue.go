package common

import (
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/juju/errors"
)

// ttlFailureRetry is how soon a failed fetch may be retried - much shorter than typical value
// TTLs so recovery from an outage is noticed within seconds, not a whole TTL later.
const ttlFailureRetry = 5 * time.Second

// errFetchPanicked is preset before each fetch so a panic is recorded as a failed attempt.
var errFetchPanicked = errors.New("fetch panicked")

// TTLValue caches one slowly-changing fact, refreshed through fetch at most once per ttl.
// Refreshes of an already cached value run in the background so no caller ever waits on the
// fetch; only the very first one is synchronous. Fetches are single-flight (hand-rolled:
// singleflight would block concurrent callers) and a failure keeps the last known value.
// It refreshes only when read - a fact that must stay fresh with no readers needs a poller
// goroutine instead.
type TTLValue[T any] struct {
	ttl   time.Duration
	fetch func() (T, error)

	mu        sync.Mutex
	value     *T
	fetchedAt time.Time
	fetching  bool
	// lastAttempt paces retries: failures wait only ttlFailureRetry instead of a whole ttl.
	lastAttempt time.Time
	failure     ttlFailure
}

// ttlFailure is the current run of consecutive fetch failures; the zero value means healthy.
type ttlFailure struct {
	since time.Time
	err   error
}

// NewTTLValue returns a cache serving a snapshot at most ttl old, refilled by fetch.
func NewTTLValue[T any](ttl time.Duration, fetch func() (T, error)) *TTLValue[T] {
	return &TTLValue[T]{ttl: ttl, fetch: fetch}
}

// Get returns the last fetched value (nil if no fetch ever succeeded), how long refresh
// attempts have been failing (0 while healthy) and the last fetch error. now is injected so
// the TTL rules can be tested without sleeping.
func (c *TTLValue[T]) Get(now time.Time) (*T, time.Duration, error) {
	c.mu.Lock()
	if c.value != nil && now.Sub(c.fetchedAt) < c.ttl {
		defer c.mu.Unlock()
		return c.value, 0, nil
	}
	retryAfter := c.ttl
	if c.failure.err != nil {
		retryAfter = ttlFailureRetry
	}
	// single-flight with retry pacing: everyone but one fetcher gets the current state at once
	if c.fetching || (!c.lastAttempt.IsZero() && now.Sub(c.lastAttempt) < retryAfter) {
		defer c.mu.Unlock()
		return c.stateLocked(now)
	}
	c.fetching = true
	c.lastAttempt = now
	warm := c.value != nil
	c.mu.Unlock()

	if warm {
		// refresh in the background - a caller holding a stale value must never block on a
		// fetch that can sit until an RPC timeout
		go c.refresh(now)
	} else {
		// cold: the first caller fetches synchronously so its error (or panic) propagates
		c.fetchAndStore(now)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stateLocked(now)
}

// stateLocked returns the cached state; the caller holds mu.
func (c *TTLValue[T]) stateLocked(now time.Time) (*T, time.Duration, error) {
	var failingFor time.Duration
	if c.failure.err != nil {
		failingFor = now.Sub(c.failure.since)
	}
	return c.value, failingFor, c.failure.err
}

// refresh recovers a fetch panic so it cannot crash the process from a bare goroutine -
// fetchAndStore has recorded it as a failed attempt by the time it propagates here.
func (c *TTLValue[T]) refresh(now time.Time) {
	defer func() {
		if r := recover(); r != nil {
			glog.Errorf("TTLValue refresh panicked: %v", r)
		}
	}()
	c.fetchAndStore(now)
}

// fetchAndStore runs one fetch and records its outcome in a defer, so even a panicking fetch
// resets the in-flight flag (as a failed attempt) instead of wedging the cache forever.
func (c *TTLValue[T]) fetchAndStore(now time.Time) {
	var value T
	err := errFetchPanicked
	defer func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.fetching = false
		if err != nil {
			if c.failure.err == nil {
				c.failure.since = now
			}
			c.failure.err = err
			return
		}
		c.failure = ttlFailure{}
		c.value = &value
		c.fetchedAt = now
	}()
	value, err = c.fetch()
}
