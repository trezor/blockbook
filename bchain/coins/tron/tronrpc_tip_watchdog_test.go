//go:build unittest

package tron

import (
	"testing"
	"time"

	"github.com/trezor/blockbook/bchain/coins/eth"
)

// Tron reuses EthereumRPC.TipStaleThreshold so its ZeroMQ-feed watchdog sizes its
// stall window from the same block-cadence policy as the EVM watchdog.
func TestTronTipStaleThreshold(t *testing.T) {
	tests := []struct {
		name             string
		averageBlockTime int
		want             time.Duration
	}{
		{name: "tron 3s -> 30 blocks", averageBlockTime: 3000, want: 90 * time.Second},
		{name: "unset falls back to max", averageBlockTime: 0, want: 5 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &TronRPC{EthereumRPC: &eth.EthereumRPC{ChainConfig: &eth.Configuration{AverageBlockTimeMs: tt.averageBlockTime}}}
			if got := b.TipStaleThreshold(); got != tt.want {
				t.Fatalf("TipStaleThreshold() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestTronMarkNotifyAlive(t *testing.T) {
	b := &TronRPC{}
	if got := b.lastNotifyNs.Load(); got != 0 {
		t.Fatalf("lastNotifyNs should start at 0, got %d", got)
	}
	before := time.Now().UnixNano()
	b.markNotifyAlive()
	if got := b.lastNotifyNs.Load(); got < before {
		t.Fatalf("markNotifyAlive() recorded %d, want >= %d", got, before)
	}
}
