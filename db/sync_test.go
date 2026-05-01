//go:build unittest

package db

import (
	"context"
	stdErrors "errors"
	"io"
	"net"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	jujuErrors "github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
)

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

// stubChain satisfies bchain.BlockChain by embedding the interface (nil) and
// overriding only the methods we need. Calling any non-overridden method
// panics — tests must exercise only the configured methods.
type stubChain struct {
	bchain.BlockChain
	bestHeightFn func() (uint32, error)
	blockHashFn  func(uint32) (string, error)
	getBlockFn   func(string, uint32) (*bchain.Block, error)
}

func (s *stubChain) GetBestBlockHeight() (uint32, error) { return s.bestHeightFn() }
func (s *stubChain) GetBlockHash(h uint32) (string, error) {
	return s.blockHashFn(h)
}
func (s *stubChain) GetBlock(hash string, height uint32) (*bchain.Block, error) {
	return s.getBlockFn(hash, height)
}

func newTestSyncWorker(t *testing.T, chain bchain.BlockChain) *SyncWorker {
	t.Helper()
	metrics, err := common.GetMetrics("test_" + t.Name())
	if err != nil {
		t.Fatalf("GetMetrics() error = %v", err)
	}
	return &SyncWorker{
		chain:        chain,
		chanOsSignal: make(chan os.Signal, 1),
		metrics:      metrics,
		missingBlockRetry: MissingBlockRetryConfig{
			RecheckThreshold:    3,
			TipRecheckThreshold: 1,
			RetryDelay:          time.Millisecond,
		},
	}
}

func TestCoordinatorPastTip(t *testing.T) {
	tests := []struct {
		name       string
		bestHeight uint32
		bestErr    error
		h          uint32
		want       bool
		wantErr    bool
	}{
		{name: "tip above h", bestHeight: 100, h: 50, want: false},
		{name: "tip equals h", bestHeight: 100, h: 100, want: false},
		{name: "tip below h", bestHeight: 100, h: 200, want: true},
		{name: "best height error", bestErr: stdErrors.New("rpc gone"), h: 50, want: false, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain := &stubChain{
				bestHeightFn: func() (uint32, error) { return tt.bestHeight, tt.bestErr },
			}
			w := newTestSyncWorker(t, chain)
			got, err := w.coordinatorPastTip(tt.h)
			if tt.wantErr && err == nil {
				t.Fatalf("coordinatorPastTip() error = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("coordinatorPastTip() error = %v, want nil", err)
			}
			if got != tt.want {
				t.Fatalf("coordinatorPastTip() = %v, want %v", got, tt.want)
			}
		})
	}
}

// runWorkerForOneBlock spins up a single getBlockWorker on a one-element hch
// (closed immediately so the worker iterates exactly once over the supplied
// hashHeight) and waits until it terminates or the test deadline expires.
// Returns whatever appeared on abortCh, or nil if the worker exited cleanly.
func runWorkerForOneBlock(t *testing.T, w *SyncWorker, hh hashHeight) error {
	t.Helper()
	hch := make(chan hashHeight, 1)
	hch <- hh
	close(hch)
	bch := []chan *bchain.Block{make(chan *bchain.Block, 1)}
	hchClosed := atomic.Value{}
	// Force the worker into the mid-range branch — the new bounded path —
	// even though hch itself is closed. Without this, a non-retryable error
	// would take the existing tail-of-range escape and not exercise the fix.
	hchClosed.Store(false)
	terminating := make(chan struct{})
	abortCh := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go w.getBlockWorker(0, 1, &wg, hch, bch, &hchClosed, terminating, abortCh)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case err := <-abortCh:
		<-done
		return err
	case <-done:
		return nil
	case <-time.After(2 * time.Second):
		close(terminating)
		<-done
		t.Fatal("getBlockWorker did not terminate within deadline")
		return nil
	}
}

func TestGetBlockWorkerEscalatesNonRetryableErrorsToAbort(t *testing.T) {
	// Non-retryable error mid-range used to loop forever. After the fix,
	// the worker should give up after RecheckThreshold attempts and surface
	// the original error to abortCh, since the chain confirms the hash is
	// still expected at the same height.
	boom := stdErrors.New("malformed block payload")
	chain := &stubChain{
		bestHeightFn: func() (uint32, error) { return 100, nil },
		blockHashFn:  func(uint32) (string, error) { return "expected-hash", nil },
		getBlockFn:   func(string, uint32) (*bchain.Block, error) { return nil, boom },
	}
	w := newTestSyncWorker(t, chain)

	err := runWorkerForOneBlock(t, w, hashHeight{hash: "expected-hash", height: 50})
	if err == nil {
		t.Fatal("expected abortCh error, got nil")
	}
	if stdErrors.Is(err, errResync) {
		t.Fatalf("got errResync, want original error %v", boom)
	}
	if err.Error() != boom.Error() {
		t.Fatalf("abortCh error = %v, want %v", err, boom)
	}
}

func TestGetBlockWorkerRestartsOnReorgForNonRetryable(t *testing.T) {
	// Non-retryable error mid-range, but the chain reports a different hash
	// at the same height — that's a reorg. The worker should bail out with
	// errResync rather than the original error.
	boom := stdErrors.New("malformed block payload")
	chain := &stubChain{
		bestHeightFn: func() (uint32, error) { return 100, nil },
		blockHashFn:  func(uint32) (string, error) { return "different-hash", nil },
		getBlockFn:   func(string, uint32) (*bchain.Block, error) { return nil, boom },
	}
	w := newTestSyncWorker(t, chain)

	err := runWorkerForOneBlock(t, w, hashHeight{hash: "expected-hash", height: 50})
	if !stdErrors.Is(err, errResync) {
		t.Fatalf("abortCh error = %v, want errResync", err)
	}
}

func TestWaitForWorkersReturnsCleanly(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	terminating := make(chan struct{})
	abortCh := make(chan error, 1)

	go func() {
		time.Sleep(5 * time.Millisecond)
		wg.Done()
	}()

	done := make(chan struct{})
	var got error
	go func() {
		got = waitForWorkers(&wg, terminating, abortCh)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("waitForWorkers did not return")
	}
	if got != nil {
		t.Fatalf("waitForWorkers() = %v, want nil", got)
	}
	select {
	case <-terminating:
		t.Fatal("terminating should not be closed when no abort occurred")
	default:
	}
}

func TestWaitForWorkersUnsticksBlockedWorkers(t *testing.T) {
	// Reproduces the deadlock the reviewer flagged: one worker sends an abort
	// and exits, another worker is stuck on a bch-send-equivalent (here,
	// blocked on terminating). Without abort-aware wait, wg.Wait() never
	// returns. waitForWorkers must close terminating to release the stuck
	// worker so the wait can complete.
	var wg sync.WaitGroup
	wg.Add(2)
	terminating := make(chan struct{})
	abortCh := make(chan error, 1)
	boom := stdErrors.New("worker fatal")

	// Worker A: aborts and exits.
	go func() {
		abortCh <- boom
		wg.Done()
	}()
	// Worker B: blocked until terminating is closed (simulating bch-send wedge).
	go func() {
		<-terminating
		wg.Done()
	}()

	done := make(chan struct{})
	var got error
	go func() {
		got = waitForWorkers(&wg, terminating, abortCh)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("waitForWorkers did not return — deadlock not resolved")
	}
	if !stdErrors.Is(got, boom) {
		t.Fatalf("waitForWorkers() = %v, want %v", got, boom)
	}
	select {
	case <-terminating:
	default:
		t.Fatal("terminating should be closed after abort")
	}
}

func TestWaitForWorkersCapturesLateAbort(t *testing.T) {
	// Workers may push to abortCh and then call wg.Done() in quick succession.
	// If wg completes before the wait loop reads abortCh, the abort must still
	// be drained — otherwise the caller treats a failed sync as successful.
	var wg sync.WaitGroup
	wg.Add(1)
	terminating := make(chan struct{})
	abortCh := make(chan error, 1)
	boom := stdErrors.New("late abort")

	abortCh <- boom // already buffered before the wait starts
	wg.Done()

	got := waitForWorkers(&wg, terminating, abortCh)
	if !stdErrors.Is(got, boom) {
		t.Fatalf("waitForWorkers() = %v, want %v", got, boom)
	}
}
