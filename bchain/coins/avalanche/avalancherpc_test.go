package avalanche

import (
	"testing"
	"time"
)

func TestNodeVersionCached(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("value is cached for a TTL and refreshed after it", func(t *testing.T) {
		b := &AvalancheRPC{}
		probes := 0
		probe := func() string {
			probes++
			return "avm/1.11.0"
		}
		if v := b.nodeVersionCached(base, probe); v != "avm/1.11.0" {
			t.Fatalf("first probe = %q", v)
		}
		if v := b.nodeVersionCached(base.Add(nodeVersionTTL-time.Nanosecond), probe); v != "avm/1.11.0" {
			t.Fatalf("cached read = %q", v)
		}
		if probes != 1 {
			t.Fatalf("probes = %d, want 1", probes)
		}
		b.nodeVersionCached(base.Add(nodeVersionTTL), probe)
		if probes != 2 {
			t.Fatalf("probes after TTL = %d, want 2", probes)
		}
	})

	t.Run("a probe that never yields a version is still suppressed", func(t *testing.T) {
		// The info endpoint being down, or answering without an avm entry, used to leave the
		// TTL unreachable and cost one RPC per API request.
		b := &AvalancheRPC{}
		probes := 0
		empty := func() string {
			probes++
			return ""
		}
		for _, at := range []time.Time{base, base.Add(time.Second), base.Add(nodeVersionTTL - time.Nanosecond)} {
			if v := b.nodeVersionCached(at, empty); v != "" {
				t.Fatalf("probe at %v = %q, want empty", at, v)
			}
		}
		if probes != 1 {
			t.Fatalf("probes = %d, want 1", probes)
		}
		b.nodeVersionCached(base.Add(nodeVersionTTL), empty)
		if probes != 2 {
			t.Fatalf("probes after TTL = %d, want 2", probes)
		}
	})

	t.Run("a later failure keeps the previous value", func(t *testing.T) {
		b := &AvalancheRPC{}
		b.nodeVersionCached(base, func() string { return "avm/1.11.0" })
		if v := b.nodeVersionCached(base.Add(nodeVersionTTL), func() string { return "" }); v != "avm/1.11.0" {
			t.Fatalf("after failed refresh = %q, want the previous value", v)
		}
	})

	t.Run("a reader during an in-flight probe is not blocked", func(t *testing.T) {
		b := &AvalancheRPC{}
		b.nodeVersionCached(base, func() string { return "avm/1.11.0" })
		at := base.Add(nodeVersionTTL)
		var concurrent string
		// Re-entering from inside the probe is what a second request sees while the first is
		// on the wire: holding nodeVersionMu across the RPC would deadlock here instead.
		b.nodeVersionCached(at, func() string {
			concurrent = b.nodeVersionCached(at, func() string {
				t.Error("second caller must not probe while a probe is in flight")
				return "avm/9.9.9"
			})
			return "avm/1.12.0"
		})
		if concurrent != "avm/1.11.0" {
			t.Fatalf("in-flight reader got %q, want the previous value", concurrent)
		}
		if v := b.nodeVersionCached(at, func() string { t.Error("unexpected probe"); return "" }); v != "avm/1.12.0" {
			t.Fatalf("after the probe completed = %q", v)
		}
	})
}
