package eth

import (
	"testing"
	"time"
)

// tipStaleThreshold scales the silent-subscription window to the chain's block
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
				t.Fatalf("tipStaleThreshold() = %s, want %s", got, tt.want)
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
	got := b.lastSubNotifyNs.Load()
	if got < before {
		t.Fatalf("markSubscriptionAlive() recorded %d, want >= %d", got, before)
	}
}
