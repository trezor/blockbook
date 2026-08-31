package avalanche

import (
	"testing"
	"time"

	"github.com/trezor/blockbook/bchain/coins/eth"
)

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

// The TTL/single-flight mechanics are covered by eth's TestTTLValue; these tests cover the
// avalanche policy layered on top of it.
func TestNodeVersionCached(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("value is cached for a TTL", func(t *testing.T) {
		b := &AvalancheRPC{}
		probes := 0
		probe := func() string { probes++; return "avm/1.11.0" }
		if v := b.nodeVersionCached(base, probe); v != "avm/1.11.0" {
			t.Fatalf("first probe = %q", v)
		}
		if v := b.nodeVersionCached(base.Add(nodeVersionTTL-time.Nanosecond), probe); v != "avm/1.11.0" {
			t.Fatalf("cached read = %q", v)
		}
		if probes != 1 {
			t.Fatalf("probes = %d, want 1", probes)
		}
	})

	t.Run("a probe that yields no version stays empty and is not retried immediately", func(t *testing.T) {
		// the info endpoint being down, or answering without an avm entry, must not cost an
		// RPC per API request
		b := &AvalancheRPC{}
		probes := 0
		empty := func() string { probes++; return "" }
		for _, at := range []time.Time{base, base.Add(time.Second), base.Add(eth.TTLFailureRetry - time.Nanosecond)} {
			if v := b.nodeVersionCached(at, empty); v != "" {
				t.Fatalf("probe at %v = %q, want empty", at, v)
			}
		}
		if probes != 1 {
			t.Fatalf("probes = %d, want 1", probes)
		}
		b.nodeVersionCached(base.Add(eth.TTLFailureRetry), empty)
		if probes != 2 {
			t.Fatalf("probes after the retry interval = %d, want 2", probes)
		}
	})

	t.Run("a later failure keeps the previous value", func(t *testing.T) {
		b := &AvalancheRPC{}
		b.nodeVersionCached(base, func() string { return "avm/1.11.0" })
		at := base.Add(nodeVersionTTL)
		probed := make(chan struct{})
		// the refresh runs in the background, the reader gets the old value immediately
		if v := b.nodeVersionCached(at, func() string { close(probed); return "" }); v != "avm/1.11.0" {
			t.Fatalf("read during refresh = %q, want the previous value", v)
		}
		<-probed
		waitUntil(t, "the failed refresh to be recorded", func() bool {
			_, _, err := b.nodeVersion.Get(at, nodeVersionTTL, nil)
			return err != nil
		})
		if v := b.nodeVersionCached(at.Add(time.Second), func() string {
			t.Error("probe inside the retry window")
			return ""
		}); v != "avm/1.11.0" {
			t.Fatalf("value after failed refresh = %q, want the previous value", v)
		}
	})
}
