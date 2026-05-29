package eth

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/trezor/blockbook/bchain"
)

// TipStaleThreshold scales the silent-subscription window to the chain's block
// cadence (replacing the old fixed 15m), clamped so fast chains don't react to
// jitter and slow chains still recover in bounded time.
func TestTipStaleThreshold(t *testing.T) {
	tests := []struct {
		name             string
		averageBlockTime int
		want             time.Duration
	}{
		{name: "polygon 2s -> 30 blocks", averageBlockTime: 2000, want: 60 * time.Second},
		{name: "bsc 3s -> 30 blocks", averageBlockTime: 3000, want: 90 * time.Second},
		{name: "ethereum 12s clamped to max", averageBlockTime: 12000, want: tipWatchdogMaxStale},
		{name: "10s lands exactly on max", averageBlockTime: 10000, want: tipWatchdogMaxStale},
		{name: "arbitrum 250ms clamped to min", averageBlockTime: 250, want: tipWatchdogMinStale},
		{name: "unset falls back to max", averageBlockTime: 0, want: tipWatchdogMaxStale},
		{name: "negative falls back to max", averageBlockTime: -1, want: tipWatchdogMaxStale},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &EthereumRPC{ChainConfig: &Configuration{AverageBlockTimeMs: tt.averageBlockTime}}
			if got := b.TipStaleThreshold(); got != tt.want {
				t.Fatalf("TipStaleThreshold() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestMarkSubscriptionAlive(t *testing.T) {
	b := &EthereumRPC{}
	if got := b.lastSubNotifyNs.Load(); got != 0 {
		t.Fatalf("lastSubNotifyNs should start at 0, got %d", got)
	}
	before := time.Now().UnixNano()
	b.markSubscriptionAlive()
	if got := b.lastSubNotifyNs.Load(); got < before {
		t.Fatalf("markSubscriptionAlive() recorded %d, want >= %d", got, before)
	}
}

// --- minimal fakes implementing only what the watchdog touches ---

type stubHeader struct{ n int64 }

func (h stubHeader) Hash() string         { return string(rune(h.n)) }
func (h stubHeader) Number() *big.Int     { return big.NewInt(h.n) }
func (h stubHeader) Difficulty() *big.Int { return big.NewInt(0) }

type stubHeaderClient struct {
	bchain.EVMClient // embed for the methods the watchdog never calls
	height           int64
}

func (c *stubHeaderClient) HeaderByNumber(context.Context, *big.Int) (bchain.EVMHeader, error) {
	return stubHeader{n: c.height}, nil
}

// On a stale feed the watchdog must poll the tip, push a new-block notification,
// and attempt a reconnect — exercised here without waiting on the real ticker.
// Reconnect runs after the poll/push, so we let OpenRPC fail (closeRPC is nil-safe)
// to assert it was attempted without standing up subscription plumbing whose only
// job would be to echo success back.
func TestEthereumTipWatchdogTickOnStaleFeed(t *testing.T) {
	pushes := make(chan bchain.NotificationType, 4)
	reconnectAttempted := false
	b := &EthereumRPC{
		ChainConfig: &Configuration{AverageBlockTimeMs: 2000},
		Timeout:     time.Second,
		PushHandler: func(nt bchain.NotificationType) { pushes <- nt },
	}
	b.Client = &stubHeaderClient{height: 100}
	b.OpenRPC = func(string, string) (bchain.EVMRPCClient, bchain.EVMClient, error) {
		reconnectAttempted = true
		return nil, nil, errors.New("reconnect disabled in test")
	}
	// Simulate a silently stalled subscription: last notification long ago.
	b.lastSubNotifyNs.Store(time.Now().Add(-time.Hour).UnixNano())

	b.tipWatchdogTick(time.Millisecond)

	select {
	case nt := <-pushes:
		if nt != bchain.NotificationNewBlock {
			t.Fatalf("pushed %v, want NotificationNewBlock", nt)
		}
	default:
		t.Fatal("watchdog did not push NotificationNewBlock on a stale feed")
	}
	if !reconnectAttempted {
		t.Fatal("watchdog did not attempt reconnect on a stale feed")
	}
}

// A fresh feed (recent notification) must not poll or reconnect.
func TestEthereumTipWatchdogTickFreshFeedNoop(t *testing.T) {
	pushes := make(chan bchain.NotificationType, 1)
	b := &EthereumRPC{
		ChainConfig: &Configuration{AverageBlockTimeMs: 2000},
		PushHandler: func(nt bchain.NotificationType) { pushes <- nt },
	}
	b.OpenRPC = func(string, string) (bchain.EVMRPCClient, bchain.EVMClient, error) {
		t.Fatal("watchdog reconnected on a fresh feed")
		return nil, nil, nil
	}
	b.lastSubNotifyNs.Store(time.Now().UnixNano())

	b.tipWatchdogTick(time.Minute)

	if len(pushes) != 0 {
		t.Fatal("watchdog pushed on a fresh feed")
	}
}
