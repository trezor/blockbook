package eth

import (
	"context"
	"errors"
	"io"
	"math/big"
	"net"
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

type stubHeader struct {
	n int64
	h string // optional hash override; defaults to a value derived from n
}

func (h stubHeader) Hash() string {
	if h.h != "" {
		return h.h
	}
	return string(rune(h.n))
}
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

// On a sustained stall the watchdog must let a genuinely lower backend tip regress
// the cached tip. The hot-path monotonic guard rejects a lower height to ride out
// transient load-balancer lag, but that lag resolves well within TipStaleThreshold;
// a tip still below ours after the stall window is a real rollback. If the cached
// tip stayed frozen above the backend it would equal the local DB tip, so
// resyncIndex would keep early-exiting as "synced" (localBestHash == cached
// remoteBestHash) and never reach its GetBlockHash fork path.
func TestEthereumTipWatchdogRegressesTipOnRollback(t *testing.T) {
	pushes := make(chan bchain.NotificationType, 4)
	reconnectAttempted := false
	b := &EthereumRPC{
		ChainConfig: &Configuration{AverageBlockTimeMs: 2000},
		Timeout:     time.Second,
		PushHandler: func(nt bchain.NotificationType) { pushes <- nt },
	}
	// Cached tip is ahead at 200 (where the feed froze before the backend rolled back).
	if !b.setBestHeader(stubHeader{n: 200}, true) {
		t.Fatal("precondition: setting initial tip 200 failed")
	}
	// The backend now reports a lower tip (a real rollback to height 150).
	b.Client = &stubHeaderClient{height: 150}
	b.OpenRPC = func(string, string) (bchain.EVMRPCClient, bchain.EVMClient, error) {
		reconnectAttempted = true
		return nil, nil, errors.New("reconnect disabled in test")
	}
	b.lastSubNotifyNs.Store(time.Now().Add(-time.Hour).UnixNano())

	b.tipWatchdogTick(time.Millisecond)

	if h, err := b.getBestHeader(); err != nil {
		t.Fatal(err)
	} else if got := h.Number().Int64(); got != 150 {
		t.Fatalf("cached tip = %d, want 150 (regressed to the rolled-back backend tip so resyncIndex can detect the fork)", got)
	}
	select {
	case nt := <-pushes:
		if nt != bchain.NotificationNewBlock {
			t.Fatalf("pushed %v, want NotificationNewBlock", nt)
		}
	default:
		t.Fatal("watchdog did not push NotificationNewBlock after a rollback")
	}
	if !reconnectAttempted {
		t.Fatal("watchdog did not attempt reconnect after a rollback")
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

// The feed's own header must drive the cached tip even when HTTP (HeaderByNumber)
// is pinned to a lagging height, so a stale load-balanced HTTP view can no longer
// freeze sync into a false "synced". The advance must also stamp liveness and wake
// the sync loop.
func TestEthereumFeedHeaderAdvancesTipDespiteStaleHTTP(t *testing.T) {
	b := &EthereumRPC{
		ChainConfig: &Configuration{AverageBlockTimeMs: 2000},
		Timeout:     time.Second,
	}
	b.newBlockNotifyCh = make(chan struct{}, 1)
	// HTTP call path is pinned to a stale, lagging height; it must not be consulted.
	b.Client = &stubHeaderClient{height: 100}

	b.onFeedHeader(stubHeader{n: 200})

	h, err := b.getBestHeader()
	if err != nil {
		t.Fatal(err)
	}
	if got := h.Number().Int64(); got != 200 {
		t.Fatalf("tip = %d, want 200 (the feed header), not the stale HTTP height 100", got)
	}
	if b.lastSubNotifyNs.Load() == 0 {
		t.Fatal("feed advance did not stamp subscription liveness")
	}
	select {
	case <-b.newBlockNotifyCh:
	default:
		t.Fatal("feed advance did not wake the sync loop")
	}
}

// The cached tip must not regress to a lower height reported by a lagging
// load-balancer node (which would trip a spurious fork), but a same-height reorg
// must still be applied so resyncIndex can detect and handle it.
func TestEthereumSetBestHeaderMonotonic(t *testing.T) {
	b := &EthereumRPC{Timeout: time.Second}

	if !b.setBestHeader(stubHeader{n: 200}, true) {
		t.Fatal("first header should be accepted")
	}
	if b.setBestHeader(stubHeader{n: 150}, true) {
		t.Fatal("a lower height must be rejected under a monotonic update")
	}
	if h, _ := b.getBestHeader(); h.Number().Int64() != 200 {
		t.Fatalf("tip = %d, want 200 retained", h.Number().Int64())
	}
	if !b.setBestHeader(stubHeader{n: 200, h: "reorg"}, true) {
		t.Fatal("a same-height tip reorg must be applied")
	}
	// A non-monotonic update (the authoritative-feed/Tron path) may move down.
	if !b.setBestHeader(stubHeader{n: 150}, false) {
		t.Fatal("a non-monotonic update should accept a lower height")
	}
}

// A feed that re-delivers the same head (a stuck upstream) is not progress:
// liveness must not be refreshed and the sync loop must not be woken, so the
// watchdog can eventually treat the feed as stale.
func TestEthereumIdenticalFeedHeaderDoesNotRefreshLiveness(t *testing.T) {
	b := &EthereumRPC{}
	b.newBlockNotifyCh = make(chan struct{}, 1)

	b.onFeedHeader(stubHeader{n: 100})
	first := b.lastSubNotifyNs.Load()
	if first == 0 {
		t.Fatal("first feed header should stamp liveness")
	}
	select { // drain the wake-up from the first delivery
	case <-b.newBlockNotifyCh:
	default:
	}

	b.onFeedHeader(stubHeader{n: 100}) // identical head: no progress
	if b.lastSubNotifyNs.Load() != first {
		t.Fatal("an identical feed header must not refresh liveness")
	}
	select {
	case <-b.newBlockNotifyCh:
		t.Fatal("an identical feed header must not wake the sync loop")
	default:
	}
}

// A websocket backend behind a load balancer can accept the TCP socket but never
// complete the upgrade handshake — the silent stall tipWatchdog exists to heal.
// dialRPC must honor dialTimeout instead of blocking on context.Background()
// forever; otherwise reconnectRPC, and with it the lone tipWatchdog goroutine that
// is the sole feed-liveness healer, parks indefinitely, the cached tip stays frozen,
// and resyncIndex silently reports a false syncNotNeeded until a restart.
func TestDialRPCBoundsHungHandshake(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	// Accept the dial but swallow the upgrade request and never answer it, so the
	// client is left waiting on the handshake response — the load-balancer blackhole.
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		io.Copy(io.Discard, c)
	}()

	prev := dialTimeout
	dialTimeout = 250 * time.Millisecond
	defer func() { dialTimeout = prev }()

	done := make(chan error, 1)
	start := time.Now()
	go func() {
		_, err := dialRPC("ws://" + ln.Addr().String())
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("dialRPC returned a client though the backend never completed the WS handshake")
		}
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Fatalf("dialRPC took %s; expected it bounded near dialTimeout=%s, not effectively unbounded", elapsed, dialTimeout)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("dialRPC did not return: a hung handshake parks reconnectRPC and the lone tipWatchdog goroutine forever")
	}
}

// fakeSilentSub models a newHeads subscription that is established successfully but
// never delivers a header and never errors — the silent stall behind a load
// balancer that drops the upstream without closing the socket.
type fakeSilentSub struct{ errCh chan error }

func (s *fakeSilentSub) Err() <-chan error { return s.errCh }
func (s *fakeSilentSub) Unsubscribe()      {}

// fakeRPCClient hands out a fakeSilentSub for every EthSubscribe.
type fakeRPCClient struct{}

func (c *fakeRPCClient) EthSubscribe(context.Context, interface{}, ...interface{}) (bchain.EVMClientSubscription, error) {
	return &fakeSilentSub{errCh: make(chan error)}, nil
}
func (c *fakeRPCClient) CallContext(context.Context, interface{}, string, ...interface{}) error {
	return nil
}
func (c *fakeRPCClient) Close() {}

// A newHeads subscription that is established but never delivers a header (a feed
// born silent behind a load balancer) must still arm the watchdog's staleness
// clock. Liveness used to be stamped only on a tip advance, so such a feed left
// lastSubNotifyNs at 0 and the watchdog's `if lastNs == 0 { return }` gate disabled
// it forever: the cached tip froze and resyncIndex reported a silent false
// "synced" with no error or metric. subscribeEvents must seed liveness at
// subscribe time so the feed ages past the threshold and the watchdog can recover.
func TestSubscribeEventsArmsLivenessOnSilentFeed(t *testing.T) {
	b := &EthereumRPC{
		// 12s average -> 5min threshold / 60s sample, so the watchdog goroutine
		// started by subscribeEvents cannot fire (and reconnect) during the test.
		ChainConfig: &Configuration{AverageBlockTimeMs: 12000, DisableMempoolSync: true},
		Timeout:     time.Second,
	}
	b.NewBlock = NewEthereumNewBlock()
	b.newBlockNotifyCh = make(chan struct{}, 1)
	b.RPC = &fakeRPCClient{}

	if got := b.lastSubNotifyNs.Load(); got != 0 {
		t.Fatalf("precondition: lastSubNotifyNs = %d, want 0 before subscribe", got)
	}
	if err := b.subscribeEvents(); err != nil {
		t.Fatal(err)
	}
	if b.lastSubNotifyNs.Load() == 0 {
		t.Fatal("subscribeEvents left liveness at 0 for a silent feed; tipWatchdog would stay disabled forever")
	}
}
