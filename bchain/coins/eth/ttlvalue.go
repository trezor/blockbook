package eth

import (
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/juju/errors"
)

// TTLFailureRetry is how soon a failed fetch may be retried - much shorter than the value
// TTLs so recovery from an outage is noticed within seconds, not a whole TTL later.
const TTLFailureRetry = 5 * time.Second

// TTLValue caches one slowly-changing backend fact, refreshed through a caller-supplied fetch
// at most once per TTL. Refreshes of an already cached value run in the background so no
// caller ever waits on the backend; only the very first fetch is synchronous. Fetches are
// single-flight and a failure keeps the last known value. The zero value is ready to use.
type TTLValue[T any] struct {
	mu        sync.Mutex
	value     *T
	fetchedAt time.Time
	fetching  bool
	// lastAttempt paces retries: failures wait only TTLFailureRetry instead of a whole TTL.
	lastAttempt time.Time
	lastErr     error
	// failingSince is when the current run of consecutive failures started, zero when healthy.
	failingSince time.Time
}

// Get returns the last fetched value (nil if no fetch ever succeeded), how long refresh
// attempts have been failing (0 while healthy) and the last fetch error. now is injected so
// the TTL rules can be tested without sleeping.
func (c *TTLValue[T]) Get(now time.Time, ttl time.Duration, fetch func() (T, error)) (*T, time.Duration, error) {
	c.mu.Lock()
	if c.value != nil && now.Sub(c.fetchedAt) < ttl {
		v := c.value
		c.mu.Unlock()
		return v, 0, nil
	}
	retryAfter := ttl
	if c.lastErr != nil {
		retryAfter = TTLFailureRetry
	}
	// single-flight with retry pacing: everyone but one fetcher gets the current state at once
	if c.fetching || (!c.lastAttempt.IsZero() && now.Sub(c.lastAttempt) < retryAfter) {
		return c.stateLocked(now)
	}
	c.fetching = true
	c.lastAttempt = now
	warm := c.value != nil
	c.mu.Unlock()

	if warm {
		// refresh in the background - a caller holding a stale value must never block on an
		// RPC that can sit until the timeout
		go func() {
			// fetchAndStore has already recorded the failure; without this recover a fetch
			// panic would crash the process instead of reaching an http handler's recovery
			defer func() {
				if r := recover(); r != nil {
					glog.Errorf("TTLValue refresh panicked: %v", r)
				}
			}()
			c.fetchAndStore(now, fetch)
		}()
	} else {
		c.fetchAndStore(now, fetch)
	}
	c.mu.Lock()
	return c.stateLocked(now)
}

// stateLocked returns the cached state and releases the lock.
func (c *TTLValue[T]) stateLocked(now time.Time) (*T, time.Duration, error) {
	defer c.mu.Unlock()
	var failingFor time.Duration
	if !c.failingSince.IsZero() {
		failingFor = now.Sub(c.failingSince)
	}
	return c.value, failingFor, c.lastErr
}

// fetchAndStore runs one fetch and records its outcome in a defer, so even a panicking fetch
// resets the in-flight flag (as a failed attempt) instead of wedging the cache forever.
func (c *TTLValue[T]) fetchAndStore(now time.Time, fetch func() (T, error)) {
	var value T
	// preset so a panic inside fetch is recorded as a failure by the defer
	err := errors.New("fetch panicked")
	defer func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.fetching = false
		c.lastErr = err
		if err != nil {
			if c.failingSince.IsZero() {
				c.failingSince = now
			}
			return
		}
		c.failingSince = time.Time{}
		c.value = &value
		c.fetchedAt = now
	}()
	value, err = fetch()
}
