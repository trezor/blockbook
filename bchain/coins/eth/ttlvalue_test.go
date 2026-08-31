//go:build unittest

package eth

import (
	"strings"
	"testing"
	"time"

	"github.com/juju/errors"
)

const testTTL = time.Minute

// waitUntil polls cond for a short real-time window - used only to observe background
// refreshes landing; the TTL rules themselves always run on an injected clock.
func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestTTLValue(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("cold fetch is synchronous and cached for a TTL", func(t *testing.T) {
		c := &TTLValue[string]{}
		calls := 0
		fetch := func() (string, error) { calls++; return "v1", nil }
		v, failingFor, err := c.Get(base, testTTL, fetch)
		if err != nil || v == nil || *v != "v1" || failingFor != 0 {
			t.Fatalf("cold get = %v, %v, %v", v, failingFor, err)
		}
		// every read inside the TTL must be free - these are the /api/ request paths
		for _, at := range []time.Time{base, base.Add(time.Second), base.Add(testTTL - time.Nanosecond)} {
			if v, _, err := c.Get(at, testTTL, fetch); err != nil || *v != "v1" {
				t.Fatalf("get at %v = %v, %v", at, v, err)
			}
		}
		if calls != 1 {
			t.Fatalf("fetch calls = %d, want 1", calls)
		}
	})

	t.Run("a stale value is refreshed in the background, readers never block", func(t *testing.T) {
		c := &TTLValue[string]{}
		c.Get(base, testTTL, func() (string, error) { return "v1", nil })
		at := base.Add(testTTL)
		started := make(chan struct{})
		release := make(chan struct{})
		v, _, err := c.Get(at, testTTL, func() (string, error) {
			close(started)
			<-release
			return "v2", nil
		})
		if err != nil || *v != "v1" {
			t.Fatalf("stale get = %v, %v, want the previous value immediately", v, err)
		}
		<-started
		// a reader far past the TTL must not start a second fetch while one is in flight
		if v, _, _ := c.Get(base.Add(10*testTTL), testTTL, func() (string, error) {
			t.Error("second fetch while one is in flight")
			return "", nil
		}); *v != "v1" {
			t.Fatalf("in-flight read = %v", v)
		}
		close(release)
		waitUntil(t, "the background refresh to land", func() bool {
			v, _, _ := c.Get(at, testTTL, nil)
			return *v == "v2"
		})
	})

	t.Run("failures keep the last value and are retried sooner than the TTL", func(t *testing.T) {
		c := &TTLValue[string]{}
		c.Get(base, testTTL, func() (string, error) { return "v1", nil })
		at := base.Add(testTTL)
		calls := 0
		fail := func() (string, error) { calls++; return "", errors.New("dial tcp: refused") }
		if v, _, err := c.Get(at, testTTL, fail); err != nil || *v != "v1" {
			t.Fatalf("get during outage = %v, %v, want the cached value", v, err)
		}
		waitUntil(t, "the failed refresh to be recorded", func() bool {
			_, _, err := c.Get(at, testTTL, fail)
			return err != nil
		})
		v, failingFor, err := c.Get(at.Add(time.Second), testTTL, fail)
		if *v != "v1" || failingFor != time.Second || err == nil {
			t.Fatalf("get after failure = %v, %v, %v", v, failingFor, err)
		}
		if calls != 1 {
			t.Fatalf("fetch calls inside the retry window = %d, want 1", calls)
		}
		// after TTLFailureRetry the fetch runs again and a success resets the failure state
		recoveredAt := at.Add(TTLFailureRetry)
		c.Get(recoveredAt, testTTL, func() (string, error) { return "v2", nil })
		waitUntil(t, "the recovery to land", func() bool {
			v, failingFor, err := c.Get(recoveredAt, testTTL, nil)
			return err == nil && failingFor == 0 && *v == "v2"
		})
	})

	t.Run("cold failure propagates and is not retried inside the retry window", func(t *testing.T) {
		c := &TTLValue[string]{}
		calls := 0
		fail := func() (string, error) { calls++; return "", errors.New("dial tcp: refused") }
		v, _, err := c.Get(base, testTTL, fail)
		if v != nil || err == nil || !strings.Contains(err.Error(), "refused") {
			t.Fatalf("cold failure = %v, %v", v, err)
		}
		if v, _, err := c.Get(base.Add(TTLFailureRetry-time.Nanosecond), testTTL, fail); v != nil || err == nil {
			t.Fatalf("suppressed cold get = %v, %v, want the recorded error", v, err)
		}
		if calls != 1 {
			t.Fatalf("fetch calls inside the retry window = %d, want 1", calls)
		}
		if v, _, err := c.Get(base.Add(TTLFailureRetry), testTTL, func() (string, error) { return "v1", nil }); err != nil || *v != "v1" {
			t.Fatalf("cold retry = %v, %v", v, err)
		}
	})

	t.Run("cold fetch is single-flight", func(t *testing.T) {
		c := &TTLValue[string]{}
		started := make(chan struct{})
		release := make(chan struct{})
		done := make(chan struct{})
		go func() {
			defer close(done)
			c.Get(base, testTTL, func() (string, error) { close(started); <-release; return "v1", nil })
		}()
		<-started
		// nothing fetched yet and no failure recorded - the concurrent caller just has no data
		if v, _, err := c.Get(base, testTTL, func() (string, error) {
			t.Error("second cold fetch while one is in flight")
			return "", nil
		}); v != nil || err != nil {
			t.Fatalf("cold in-flight get = %v, %v", v, err)
		}
		close(release)
		<-done
		if v, _, err := c.Get(base, testTTL, nil); err != nil || *v != "v1" {
			t.Fatalf("get after the cold fetch = %v, %v", v, err)
		}
	})

	t.Run("a panicking cold fetch propagates but does not wedge the cache", func(t *testing.T) {
		c := &TTLValue[string]{}
		func() {
			defer func() {
				if recover() == nil {
					t.Error("cold fetch panic must propagate to the caller")
				}
			}()
			c.Get(base, testTTL, func() (string, error) { panic("boom") })
		}()
		if _, _, err := c.Get(base.Add(time.Second), testTTL, func() (string, error) {
			t.Error("retry inside the retry window")
			return "", nil
		}); err == nil || !strings.Contains(err.Error(), "panicked") {
			t.Fatalf("state after panic = %v, want a recorded failure", err)
		}
		if v, _, err := c.Get(base.Add(TTLFailureRetry), testTTL, func() (string, error) { return "v1", nil }); err != nil || *v != "v1" {
			t.Fatalf("recovery after panic = %v, %v", v, err)
		}
	})

	t.Run("a panic in a background refresh is contained", func(t *testing.T) {
		c := &TTLValue[string]{}
		c.Get(base, testTTL, func() (string, error) { return "v1", nil })
		panicked := make(chan struct{})
		c.Get(base.Add(testTTL), testTTL, func() (string, error) { close(panicked); panic("boom") })
		<-panicked
		waitUntil(t, "the panic to be recorded as a failure", func() bool {
			v, _, err := c.Get(base.Add(testTTL), testTTL, nil)
			return err != nil && v != nil && *v == "v1"
		})
	})
}
