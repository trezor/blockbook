//go:build unittest

package common

import (
	"testing"
	"time"
)

func TestWsIPBlocklistBlockAndExpire(t *testing.T) {
	is := &InternalState{}
	now := time.Unix(1_700_000_000, 0)
	key := "192.0.2.10"

	if blocked, _ := is.IsWsIPBlocked(key, now); blocked {
		t.Fatal("key should not be blocked before any Block call")
	}

	is.BlockWsIP(key, now.Add(time.Hour), now)
	if blocked, _ := is.IsWsIPBlocked(key, now); !blocked {
		t.Fatal("key should be blocked immediately after Block")
	}
	if blocked, _ := is.IsWsIPBlocked(key, now.Add(59*time.Minute)); !blocked {
		t.Fatal("key should still be blocked before expiry")
	}
	if blocked, _ := is.IsWsIPBlocked(key, now.Add(time.Hour+time.Second)); blocked {
		t.Fatal("key should not be blocked after expiry")
	}
}

func TestWsIPBlocklistExtendAndBreaches(t *testing.T) {
	is := &InternalState{}
	now := time.Unix(1_700_000_000, 0)
	key := "192.0.2.10"

	is.BlockWsIP(key, now.Add(time.Hour), now)
	// Re-block before expiry with a later deadline: extends Until, bumps breaches,
	// keeps the original BlockedAt.
	is.BlockWsIP(key, now.Add(2*time.Hour), now.Add(time.Minute))

	snap := is.WsBlockedIPsSnapshot(now.Add(time.Minute))
	if len(snap) != 1 {
		t.Fatalf("snapshot len = %d, want 1", len(snap))
	}
	e := snap[0]
	if e.Breaches != 2 {
		t.Fatalf("breaches = %d, want 2", e.Breaches)
	}
	if !e.Until.Equal(now.Add(2 * time.Hour)) {
		t.Fatalf("until = %v, want %v", e.Until, now.Add(2*time.Hour))
	}
	if !e.BlockedAt.Equal(now) {
		t.Fatalf("blockedAt = %v, want %v (original)", e.BlockedAt, now)
	}
}

func TestWsIPBlocklistReblockAfterExpiryResetsWindow(t *testing.T) {
	is := &InternalState{}
	now := time.Unix(1_700_000_000, 0)
	key := "192.0.2.10"

	is.BlockWsIP(key, now.Add(time.Hour), now)
	later := now.Add(2 * time.Hour) // after the first block expired
	is.BlockWsIP(key, later.Add(time.Hour), later)

	snap := is.WsBlockedIPsSnapshot(later)
	if len(snap) != 1 {
		t.Fatalf("snapshot len = %d, want 1", len(snap))
	}
	if !snap[0].BlockedAt.Equal(later) {
		t.Fatalf("blockedAt = %v, want %v (window reset on re-block after expiry)", snap[0].BlockedAt, later)
	}
	if snap[0].Breaches != 1 {
		t.Fatalf("breaches = %d, want 1 after window reset", snap[0].Breaches)
	}
}

func TestWsIPBlocklistRejectedCount(t *testing.T) {
	is := &InternalState{}
	now := time.Unix(1_700_000_000, 0)
	key := "192.0.2.10"

	is.BlockWsIP(key, now.Add(time.Hour), now)
	for i := 0; i < 3; i++ {
		blocked, rejected := is.IsWsIPBlocked(key, now)
		if !blocked || rejected != i+1 {
			t.Fatalf("attempt %d: blocked=%v rejected=%d, want true, %d", i, blocked, rejected, i+1)
		}
	}
	// A non-blocking probe of an unblocked key must not record a rejection.
	is.IsWsIPBlocked("198.51.100.1", now)

	snap := is.WsBlockedIPsSnapshot(now)
	if len(snap) != 1 || snap[0].Rejected != 3 {
		t.Fatalf("rejected = %v, want 3", snap)
	}
}

func TestWsIPBlocklistSweep(t *testing.T) {
	is := &InternalState{}
	now := time.Unix(1_700_000_000, 0)

	is.BlockWsIP("192.0.2.1", now.Add(time.Hour), now)
	is.BlockWsIP("192.0.2.2", now.Add(2*time.Hour), now)

	if got := is.SweepWsBlockedIPs(now); got != 2 {
		t.Fatalf("sweep before expiry returned %d, want 2", got)
	}
	// After the first entry expires, sweep drops it and returns the live count.
	if got := is.SweepWsBlockedIPs(now.Add(time.Hour + time.Minute)); got != 1 {
		t.Fatalf("sweep after one expiry returned %d, want 1", got)
	}
	snap := is.WsBlockedIPsSnapshot(now.Add(time.Hour + time.Minute))
	if len(snap) != 1 || snap[0].Key != "192.0.2.2" {
		t.Fatalf("snapshot after sweep = %v, want only 192.0.2.2", snap)
	}
}

func TestWsIPBlocklistSnapshotOrderAndReset(t *testing.T) {
	is := &InternalState{}
	now := time.Unix(1_700_000_000, 0)

	is.BlockWsIP("192.0.2.1", now.Add(time.Hour), now)
	is.BlockWsIP("192.0.2.2", now.Add(3*time.Hour), now)
	is.BlockWsIP("192.0.2.3", now.Add(2*time.Hour), now)

	snap := is.WsBlockedIPsSnapshot(now)
	if len(snap) != 3 {
		t.Fatalf("snapshot len = %d, want 3", len(snap))
	}
	// Sorted by Until descending.
	if snap[0].Key != "192.0.2.2" || snap[1].Key != "192.0.2.3" || snap[2].Key != "192.0.2.1" {
		t.Fatalf("snapshot order = %v, want [.2 .3 .1] by expiry desc", []string{snap[0].Key, snap[1].Key, snap[2].Key})
	}

	is.ResetWsBlockedIPs()
	if got := len(is.WsBlockedIPsSnapshot(now)); got != 0 {
		t.Fatalf("after reset snapshot len = %d, want 0", got)
	}
}
