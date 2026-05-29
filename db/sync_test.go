//go:build unittest

package db

import (
	"context"
	stdErrors "errors"
	"io"
	"net"
	"net/url"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	jujuErrors "github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
)

var (
	testMetricsOnce sync.Once
	testMetrics     *common.Metrics
	testMetricsErr  error
)

func getTestMetrics(t *testing.T) *common.Metrics {
	testMetricsOnce.Do(func() {
		testMetrics, testMetricsErr = common.GetMetrics("test")
	})
	if testMetricsErr != nil {
		t.Fatalf("GetMetrics: %v", testMetricsErr)
	}
	return testMetrics
}

func TestIsRetryableGetBlockError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil",
			err:  nil,
			want: false,
		},
		{
			name: "block not found",
			err:  bchain.ErrBlockNotFound,
			want: true,
		},
		{
			name: "deadline exceeded",
			err:  context.DeadlineExceeded,
			want: true,
		},
		{
			name: "unexpected EOF",
			err:  io.ErrUnexpectedEOF,
			want: true,
		},
		{
			name: "EOF",
			err:  io.EOF,
			want: true,
		},
		{
			name: "annotated deadline exceeded",
			err:  jujuErrors.Annotatef(context.DeadlineExceeded, "eth_getLogs blockNumber %v", "0x1"),
			want: true,
		},
		{
			name: "annotated unexpected EOF",
			err:  jujuErrors.Annotatef(io.ErrUnexpectedEOF, "eth_getLogs blockNumber %v", "0x1"),
			want: true,
		},
		{
			name: "network timeout",
			err: &net.DNSError{
				Err:       "i/o timeout",
				Name:      "example.org",
				IsTimeout: true,
			},
			want: true,
		},
		{
			name: "connection reset by peer",
			err: &url.Error{
				Op:  "Post",
				URL: "http://127.0.0.1:8545",
				Err: syscall.ECONNRESET,
			},
			want: true,
		},
		{
			name: "connection refused",
			err: &url.Error{
				Op:  "Post",
				URL: "http://127.0.0.1:8545",
				Err: syscall.ECONNREFUSED,
			},
			want: true,
		},
		{
			name: "rpc 503",
			err:  stdErrors.New("503 Service Unavailable: backend overloaded"),
			want: true,
		},
		{
			name: "rpc 429",
			err:  stdErrors.New("429 Too Many Requests"),
			want: true,
		},
		{
			name: "header not found",
			err:  stdErrors.New("header not found"),
			want: true,
		},
		{
			name: "other error",
			err:  stdErrors.New("boom"),
			want: false,
		},
		{
			name: "annotated other error",
			err:  jujuErrors.Annotatef(stdErrors.New("boom"), "eth_getLogs blockNumber %v", "0x1"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRetryableGetBlockError(tt.err)
			if got != tt.want {
				t.Fatalf("isRetryableGetBlockError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestConnectBlocksHonorsClosedShutdownBeforeStart(t *testing.T) {
	for i := 0; i < 100; i++ {
		ch := make(chan os.Signal)
		close(ch)

		w := &SyncWorker{
			chanOsSignal: ch,
		}

		if err := w.connectBlocks(nil, false); !stdErrors.Is(err, ErrOperationInterrupted) {
			t.Fatalf("connectBlocks error = %v, want %v", err, ErrOperationInterrupted)
		}
	}
}

type getBlockChainTestChain struct {
	bchain.BlockChain
	bestHeight      uint32
	bestHeightErr   error
	bestHeightCalls int
	hashes          map[uint32]string
	blocks          map[uint32]*bchain.Block
	blockErrors     map[uint32][]error
	getBlockCalls   map[uint32]int
	getBlockHashErr error
}

func (c *getBlockChainTestChain) GetBestBlockHeight() (uint32, error) {
	c.bestHeightCalls++
	if c.bestHeightErr != nil {
		return 0, c.bestHeightErr
	}
	return c.bestHeight, nil
}

func (c *getBlockChainTestChain) GetBlockHash(height uint32) (string, error) {
	if c.getBlockHashErr != nil {
		return "", c.getBlockHashErr
	}
	if hash, ok := c.hashes[height]; ok {
		return hash, nil
	}
	return "", bchain.ErrBlockNotFound
}

func (c *getBlockChainTestChain) GetBlock(hash string, height uint32) (*bchain.Block, error) {
	c.getBlockCalls[height]++
	if errs := c.blockErrors[height]; len(errs) > 0 {
		err := errs[0]
		c.blockErrors[height] = errs[1:]
		return nil, err
	}
	if block := c.blocks[height]; block != nil {
		copy := *block
		return &copy, nil
	}
	return nil, bchain.ErrBlockNotFound
}

func newGetBlockChainTestWorker(t *testing.T, chain *getBlockChainTestChain, startHash string, startHeight uint32) *SyncWorker {
	return &SyncWorker{
		chain:       chain,
		startHash:   startHash,
		startHeight: startHeight,
		missingBlockRetry: MissingBlockRetryConfig{
			TipRecheckThreshold: 2,
			RetryDelay:          time.Millisecond,
		},
		metrics: getTestMetrics(t),
	}
}

func runGetBlockChain(w *SyncWorker) []blockResult {
	out := make(chan blockResult)
	done := make(chan struct{})
	go w.getBlockChain(out, done)
	var results []blockResult
	for res := range out {
		results = append(results, res)
	}
	return results
}

func TestGetBlockChainRetriesSequentialTipBlock(t *testing.T) {
	chain := &getBlockChainTestChain{
		bestHeight: 1,
		hashes:     map[uint32]string{1: "h1"},
		blocks: map[uint32]*bchain.Block{
			1: {BlockHeader: bchain.BlockHeader{Hash: "h1", Height: 1}},
		},
		blockErrors: map[uint32][]error{
			1: {bchain.ErrBlockNotFound, bchain.ErrBlockNotFound},
		},
		getBlockCalls: map[uint32]int{},
	}
	w := newGetBlockChainTestWorker(t, chain, "h1", 1)

	results := runGetBlockChain(w)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].err != nil {
		t.Fatalf("unexpected error: %v", results[0].err)
	}
	if results[0].block == nil || results[0].block.Hash != "h1" {
		t.Fatalf("unexpected block: %+v", results[0].block)
	}
	if calls := chain.getBlockCalls[1]; calls != 3 {
		t.Fatalf("GetBlock height 1 calls = %d, want 3", calls)
	}
}

func TestGetBlockChainStopsAboveBestHeight(t *testing.T) {
	chain := &getBlockChainTestChain{
		bestHeight:    0,
		hashes:        map[uint32]string{},
		blocks:        map[uint32]*bchain.Block{},
		blockErrors:   map[uint32][]error{},
		getBlockCalls: map[uint32]int{},
	}
	w := newGetBlockChainTestWorker(t, chain, "", 1)

	results := runGetBlockChain(w)
	if len(results) != 0 {
		t.Fatalf("got %d results, want 0: %+v", len(results), results)
	}
	if calls := chain.getBlockCalls[1]; calls != 1 {
		t.Fatalf("GetBlock height 1 calls = %d, want 1", calls)
	}
}

func TestGetBlockChainRetriesKnownHashAboveObservedBestHeight(t *testing.T) {
	chain := &getBlockChainTestChain{
		bestHeight: 0,
		hashes:     map[uint32]string{1: "h1"},
		blocks: map[uint32]*bchain.Block{
			1: {BlockHeader: bchain.BlockHeader{Hash: "h1", Height: 1}},
		},
		blockErrors: map[uint32][]error{
			1: {bchain.ErrBlockNotFound},
		},
		getBlockCalls: map[uint32]int{},
	}
	w := newGetBlockChainTestWorker(t, chain, "h1", 1)

	results := runGetBlockChain(w)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].err != nil {
		t.Fatalf("unexpected error: %v", results[0].err)
	}
	if results[0].block == nil || results[0].block.Hash != "h1" {
		t.Fatalf("unexpected block: %+v", results[0].block)
	}
	if calls := chain.getBlockCalls[1]; calls != 2 {
		t.Fatalf("GetBlock height 1 calls = %d, want 2", calls)
	}
}

func TestGetBlockChainMissingBlockChangedHashResyncs(t *testing.T) {
	chain := &getBlockChainTestChain{
		bestHeight:    1,
		hashes:        map[uint32]string{1: "real-hash"},
		blocks:        map[uint32]*bchain.Block{},
		blockErrors:   map[uint32][]error{},
		getBlockCalls: map[uint32]int{},
	}
	w := newGetBlockChainTestWorker(t, chain, "fake-hash", 1)

	results := runGetBlockChain(w)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if !stdErrors.Is(results[0].err, errResync) {
		t.Fatalf("error = %v, want errResync", results[0].err)
	}
	if calls := chain.getBlockCalls[1]; calls != 2 {
		t.Fatalf("GetBlock height 1 calls = %d, want 2", calls)
	}
}

func TestShouldRestartSyncOnMissingBlockIgnoresLaggingBestHeight(t *testing.T) {
	chain := &getBlockChainTestChain{
		bestHeight: 9,
		hashes:     map[uint32]string{},
	}
	w := newGetBlockChainTestWorker(t, chain, "h10", 10)

	restart, err := w.shouldRestartSyncOnMissingBlock(10, "h10")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if restart {
		t.Fatal("restart = true, want false for a single lagging best-height probe")
	}
}

func TestShouldRestartSyncOnMissingBlockIgnoresMissingHashProbe(t *testing.T) {
	chain := &getBlockChainTestChain{
		bestHeight: 10,
		hashes:     map[uint32]string{},
	}
	w := newGetBlockChainTestWorker(t, chain, "h10", 10)

	restart, err := w.shouldRestartSyncOnMissingBlock(10, "h10")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if restart {
		t.Fatal("restart = true, want false for a single missing hash probe")
	}
}

func TestGetBlockChainNonRetryableErrorReturns(t *testing.T) {
	boom := stdErrors.New("boom")
	chain := &getBlockChainTestChain{
		bestHeight: 1,
		hashes:     map[uint32]string{1: "h1"},
		blocks:     map[uint32]*bchain.Block{},
		blockErrors: map[uint32][]error{
			1: {boom},
		},
		getBlockCalls: map[uint32]int{},
	}
	w := newGetBlockChainTestWorker(t, chain, "h1", 1)

	results := runGetBlockChain(w)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if !stdErrors.Is(results[0].err, boom) {
		t.Fatalf("error = %v, want %v", results[0].err, boom)
	}
	if calls := chain.getBlockCalls[1]; calls != 1 {
		t.Fatalf("GetBlock height 1 calls = %d, want 1", calls)
	}
}

func TestGetBlockChainWallClockCap(t *testing.T) {
	// Block 1 exists on chain (so first ErrBlockNotFound does not short-circuit
	// to "above best height") but GetBlock never produces it. TipRecheckThreshold
	// is set high enough that the recheck path cannot fire before the cap.
	chain := &getBlockChainTestChain{
		bestHeight:    1,
		hashes:        map[uint32]string{1: "h1"},
		blocks:        map[uint32]*bchain.Block{},
		blockErrors:   map[uint32][]error{},
		getBlockCalls: map[uint32]int{},
	}
	w := &SyncWorker{
		chain:       chain,
		startHash:   "h1",
		startHeight: 1,
		missingBlockRetry: MissingBlockRetryConfig{
			TipRecheckThreshold: 1_000_000,
			RetryDelay:          time.Millisecond,
			MaxStallDuration:    50 * time.Millisecond,
		},
		metrics: getTestMetrics(t),
	}

	start := time.Now()
	results := runGetBlockChain(w)
	elapsed := time.Since(start)

	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if !stdErrors.Is(results[0].err, errResync) {
		t.Fatalf("error = %v, want errResync", results[0].err)
	}
	if elapsed < 50*time.Millisecond {
		t.Fatalf("wall-clock cap returned in %v, expected at least 50ms", elapsed)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("wall-clock cap took %v, expected to return shortly after 50ms", elapsed)
	}
	if calls := chain.getBlockCalls[1]; calls < 2 {
		t.Fatalf("GetBlock height 1 calls = %d, want at least 2", calls)
	}
}

func TestGetBlockWorkerCheckErrAbortsAfterStreak(t *testing.T) {
	// GetBlock keeps returning ErrBlockNotFound (retryable). GetBestBlockHeight
	// fails too, so onRetryableMiss returns (false, checkErr) on every call past
	// the threshold. After three consecutive checkErrs the worker must surface
	// the error via abortCh instead of spinning silently.
	probeErr := stdErrors.New("backend unreachable")
	chain := &getBlockChainTestChain{
		bestHeight:    1,
		bestHeightErr: probeErr,
		hashes:        map[uint32]string{1: "h1"},
		blocks:        map[uint32]*bchain.Block{},
		blockErrors:   map[uint32][]error{},
		getBlockCalls: map[uint32]int{},
	}
	w := &SyncWorker{
		chain: chain,
		missingBlockRetry: MissingBlockRetryConfig{
			RecheckThreshold:    1,
			TipRecheckThreshold: 1,
			RetryDelay:          time.Millisecond,
			MaxStallDuration:    10 * time.Second, // do not let the wall-clock cap fire first
		},
		metrics: getTestMetrics(t),
	}

	const workers = 1
	hch := make(chan hashHeight, workers)
	bch := make([]chan *bchain.Block, workers)
	for i := range bch {
		bch[i] = make(chan *bchain.Block, 1)
	}
	var hchClosed atomic.Value
	hchClosed.Store(true)
	terminating := make(chan struct{})
	abortCh := make(chan error, 1)
	hch <- hashHeight{hash: "h1", height: 1}
	close(hch)

	var wg sync.WaitGroup
	wg.Add(1)
	go w.getBlockWorker(0, workers, &wg, hch, bch, &hchClosed, terminating, abortCh)

	select {
	case err := <-abortCh:
		if !stdErrors.Is(err, probeErr) {
			t.Fatalf("abortCh got %v, want %v", err, probeErr)
		}
	case <-time.After(2 * time.Second):
		close(terminating)
		t.Fatalf("worker did not abort after consecutive checkErrs")
	}

	wg.Wait()
	if chain.bestHeightCalls < 3 {
		t.Fatalf("GetBestBlockHeight calls = %d, want at least 3", chain.bestHeightCalls)
	}
}

func TestParallelConnectBlocksReturnsWorkerAbortWhenHashQueueFull(t *testing.T) {
	hashes := make(map[uint32]string)
	for h := uint32(1); h <= 10; h++ {
		hashes[h] = "h" + strconv.Itoa(int(h))
	}
	chain := &getBlockChainTestChain{
		bestHeight:    10,
		hashes:        hashes,
		blocks:        map[uint32]*bchain.Block{},
		blockErrors:   map[uint32][]error{},
		getBlockCalls: map[uint32]int{},
	}
	w := &SyncWorker{
		chain: chain,
		missingBlockRetry: MissingBlockRetryConfig{
			RecheckThreshold:    1,
			TipRecheckThreshold: 1,
			RetryDelay:          time.Millisecond,
			MaxStallDuration:    30 * time.Millisecond,
		},
		metrics: getTestMetrics(t),
	}

	done := make(chan error, 1)
	go func() {
		done <- w.ParallelConnectBlocks(nil, 1, 10, 1)
	}()

	select {
	case err := <-done:
		if !stdErrors.Is(err, errResync) {
			t.Fatalf("ParallelConnectBlocks error = %v, want errResync", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ParallelConnectBlocks did not return after worker abort")
	}
}

// MaxStallDuration is the load-bearing liveness cap: the retry loops disable the
// cap when it is <= 0, so construction must clamp it to a safe default regardless
// of which caller (or partial test cfg) supplied the config.
func TestNewSyncWorkerClampsMaxStallDuration(t *testing.T) {
	def := DefaultMissingBlockRetryConfig().MaxStallDuration
	cases := []struct {
		name string
		cfg  *SyncWorkerConfig
		want time.Duration
	}{
		{name: "nil cfg keeps default", cfg: nil, want: def},
		{
			name: "zero stall clamped to default",
			cfg:  &SyncWorkerConfig{MissingBlockRetry: MissingBlockRetryConfig{MaxStallDuration: 0}},
			want: def,
		},
		{
			name: "negative stall clamped to default",
			cfg:  &SyncWorkerConfig{MissingBlockRetry: MissingBlockRetryConfig{MaxStallDuration: -time.Second}},
			want: def,
		},
		{
			name: "explicit positive stall preserved",
			cfg:  &SyncWorkerConfig{MissingBlockRetry: MissingBlockRetryConfig{MaxStallDuration: 5 * time.Second}},
			want: 5 * time.Second,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w, err := NewSyncWorkerWithConfig(nil, nil, 1, 0, 0, false, nil, getTestMetrics(t), nil, tc.cfg)
			if err != nil {
				t.Fatalf("NewSyncWorkerWithConfig: %v", err)
			}
			if got := w.missingBlockRetry.MaxStallDuration; got != tc.want {
				t.Fatalf("MaxStallDuration = %s, want %s", got, tc.want)
			}
		})
	}
}
