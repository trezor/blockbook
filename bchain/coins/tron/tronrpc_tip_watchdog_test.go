//go:build unittest

package tron

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/trezor/blockbook/bchain"
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

// stubTronHTTP makes the solidified-head lookup fail; refreshBestHeaderFromChain
// logs and ignores it, so the tip refresh still succeeds.
type stubTronHTTP struct{}

func (stubTronHTTP) Request(context.Context, string, interface{}, interface{}) error {
	return errors.New("no solidified head in test")
}

// Tron has no WS to reconnect; on a stalled ZeroMQ feed the watchdog must poll the
// tip and re-trigger sync (new block + mempool refresh) without waiting on the ticker.
func TestTronTipWatchdogTickOnStaleFeed(t *testing.T) {
	pushes := make(chan bchain.NotificationType, 4)
	ethRPC := &eth.EthereumRPC{ChainConfig: &eth.Configuration{AverageBlockTimeMs: 3000}, Timeout: time.Second}
	ethRPC.Client = &stubHeaderClient{height: 200}
	ethRPC.PushHandler = func(nt bchain.NotificationType) { pushes <- nt }
	b := &TronRPC{EthereumRPC: ethRPC, solidityNodeHTTP: stubTronHTTP{}}
	b.lastNotifyNs.Store(time.Now().Add(-time.Hour).UnixNano())

	b.tipWatchdogTick(time.Millisecond)

	for _, want := range []bchain.NotificationType{bchain.NotificationNewBlock, bchain.NotificationNewTx} {
		select {
		case nt := <-pushes:
			if nt != want {
				t.Fatalf("pushed %v, want %v", nt, want)
			}
		default:
			t.Fatalf("watchdog did not push %v on a stale feed", want)
		}
	}
}

// The watchdog's own tip poll must not refresh feed liveness: lastNotifyNs is
// stamped only by a real ZeroMQ delivery (newBlockNotifier). If the poll re-armed
// it, a feed that has gone permanently silent while the poll keeps advancing the
// tip would keep looking alive and subscription_age_seconds would sawtooth below
// the threshold instead of climbing past it, hiding the dead feed from alerts.
func TestTronTipWatchdogPollDoesNotRefreshLiveness(t *testing.T) {
	ethRPC := &eth.EthereumRPC{ChainConfig: &eth.Configuration{AverageBlockTimeMs: 3000}, Timeout: time.Second}
	ethRPC.Client = &stubHeaderClient{height: 200}
	ethRPC.PushHandler = func(bchain.NotificationType) {}
	b := &TronRPC{EthereumRPC: ethRPC, solidityNodeHTTP: stubTronHTTP{}}
	stale := time.Now().Add(-time.Hour).UnixNano()
	b.lastNotifyNs.Store(stale)

	b.tipWatchdogTick(time.Millisecond)

	if got := b.lastNotifyNs.Load(); got != stale {
		t.Fatalf("watchdog poll refreshed liveness (lastNotifyNs %d -> %d); a permanently dead feed would be masked", stale, got)
	}
}
