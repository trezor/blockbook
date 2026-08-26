//go:build unittest

package eth

import (
	"strings"
	"testing"
	"time"

	"github.com/juju/errors"
)

func TestBackendIdentityCache(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("cold fetch is cached for a TTL", func(t *testing.T) {
		c := &backendIdentityCache{}
		calls := 0
		fetch := func() (*backendIdentity, error) {
			calls++
			return &backendIdentity{chainID: 1, clientVersion: "Geth/v1.16"}, nil
		}
		got, err := c.get(base, fetch)
		if err != nil || got.chainID != 1 || got.clientVersion != "Geth/v1.16" {
			t.Fatalf("cold get = %+v, %v", got, err)
		}
		// Every read inside the TTL must be free - these are the /api/ request paths.
		for _, at := range []time.Time{base, base.Add(time.Second), base.Add(backendIdentityTTL - time.Nanosecond)} {
			if _, err := c.get(at, fetch); err != nil {
				t.Fatalf("get at %v: %v", at, err)
			}
		}
		if calls != 1 {
			t.Fatalf("fetch calls = %d, want 1", calls)
		}
		if _, err := c.get(base.Add(backendIdentityTTL), fetch); err != nil {
			t.Fatal(err)
		}
		if calls != 2 {
			t.Fatalf("fetch calls after TTL = %d, want 2", calls)
		}
	})

	t.Run("cold fetch failure propagates", func(t *testing.T) {
		c := &backendIdentityCache{}
		if _, err := c.get(base, func() (*backendIdentity, error) { return nil, errors.New("dial tcp: refused") }); err == nil {
			t.Fatal("want error on cold failure")
		}
	})

	t.Run("stale value survives a short outage", func(t *testing.T) {
		c := &backendIdentityCache{}
		if _, err := c.get(base, func() (*backendIdentity, error) {
			return &backendIdentity{chainID: 137, clientVersion: "bor/v2.0"}, nil
		}); err != nil {
			t.Fatal(err)
		}
		calls := 0
		failing := func() (*backendIdentity, error) {
			calls++
			return nil, errors.New("timeout")
		}
		got, err := c.get(base.Add(backendIdentityTTL), failing)
		if err != nil || got.clientVersion != "bor/v2.0" {
			t.Fatalf("stale get = %+v, %v, want the cached value", got, err)
		}
		// A failed attempt is not retried for a TTL, so an outage costs one call a minute.
		if _, err := c.get(base.Add(backendIdentityTTL+time.Second), failing); err != nil {
			t.Fatal(err)
		}
		if calls != 1 {
			t.Fatalf("fetch calls during outage = %d, want 1", calls)
		}
	})

	t.Run("past max age the outage is reported", func(t *testing.T) {
		c := &backendIdentityCache{}
		if _, err := c.get(base, func() (*backendIdentity, error) {
			return &backendIdentity{chainID: 56, clientVersion: "bsc/v1"}, nil
		}); err != nil {
			t.Fatal(err)
		}
		failing := func() (*backendIdentity, error) { return nil, errors.New("dial tcp: refused") }
		if _, err := c.get(base.Add(backendIdentityMaxAge), failing); err != nil {
			t.Fatalf("at max age the cached value should still serve: %v", err)
		}
		_, err := c.get(base.Add(backendIdentityMaxAge+time.Second), failing)
		if err == nil {
			t.Fatal("want error once the snapshot is older than backendIdentityMaxAge")
		}
		if got := err.Error(); !strings.Contains(got, "not refreshed") || !strings.Contains(got, "dial tcp: refused") {
			t.Fatalf("error %q should carry the age and the backend error", got)
		}
	})

	t.Run("concurrent readers do not queue behind the fetch", func(t *testing.T) {
		c := &backendIdentityCache{}
		if _, err := c.get(base, func() (*backendIdentity, error) {
			return &backendIdentity{chainID: 1, clientVersion: "v1"}, nil
		}); err != nil {
			t.Fatal(err)
		}
		at := base.Add(backendIdentityTTL)
		calls := 0
		// Re-entering get from inside fetch is what a second concurrent request sees while
		// the first one is still on the wire: it must be served the old snapshot at once.
		var reentrant *backendIdentity
		_, err := c.get(at, func() (*backendIdentity, error) {
			calls++
			reentrant, _ = c.get(at, func() (*backendIdentity, error) {
				t.Error("second caller must not fetch while a fetch is in flight")
				return nil, nil
			})
			return &backendIdentity{chainID: 1, clientVersion: "v2"}, nil
		})
		if err != nil || calls != 1 {
			t.Fatalf("get = %v, calls = %d", err, calls)
		}
		if reentrant == nil || reentrant.clientVersion != "v1" {
			t.Fatalf("in-flight reader got %+v, want the previous snapshot", reentrant)
		}
	})
}
