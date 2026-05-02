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

// TestIsTransientSyncErrorClassifiesAvaxAnnotatedNotFound covers the public
// classifier used by syncIndexLoop to demote retryable errors to warnings.
// The annotated ErrBlockNotFound shape comes from the Avalanche unfinalized
// data path: AvalancheRPCClient.CallContext returns ErrBlockNotFound, which
// getBlockRaw then wraps with `errors.Annotatef(err, "hash %v, height %v", ...)`.
func TestIsTransientSyncErrorClassifiesAvaxAnnotatedNotFound(t *testing.T) {
	annotated := jujuErrors.Annotatef(bchain.ErrBlockNotFound, "hash %v, height %v", "", uint32(84420029))
	if !IsTransientSyncError(annotated) {
		t.Fatalf("IsTransientSyncError(%v) = false, want true", annotated)
	}

	if !IsTransientSyncError(stdErrors.New("resync: remote best height error")) {
		t.Fatal("synthetic 'remote best height error' must be classified as transient")
	}

	if IsTransientSyncError(nil) {
		t.Fatal("nil must not be classified as transient")
	}
}

type missingHashChain struct {
	bchain.BlockChain
	bestHeight uint32
}

func (c *missingHashChain) GetBlockHash(height uint32) (string, error) {
	return "", bchain.ErrBlockNotFound
}

func (c *missingHashChain) GetBestBlockHeight() (uint32, error) {
	return c.bestHeight, nil
}

func TestGetBlockHashForSyncReturnsResyncWhenRequestedHeightPastTip(t *testing.T) {
	w := &SyncWorker{
		chain:             &missingHashChain{bestHeight: 9},
		missingBlockRetry: MissingBlockRetryConfig{RecheckThreshold: 2},
	}
	retries := 0

	hash, ok, err := w.getBlockHashForSync(10, &retries)
	if hash != "" || ok || err != nil {
		t.Fatalf("first getBlockHashForSync() = %q, %v, %v; want retry", hash, ok, err)
	}
	if retries != 1 {
		t.Fatalf("retries = %d, want 1", retries)
	}

	hash, ok, err = w.getBlockHashForSync(10, &retries)
	if hash != "" || ok || !stdErrors.Is(err, errResync) {
		t.Fatalf("second getBlockHashForSync() = %q, %v, %v; want errResync", hash, ok, err)
	}
}

type nonRetryableGetBlockChain struct {
	bchain.BlockChain
}

func (c *nonRetryableGetBlockChain) GetBlock(hash string, height uint32) (*bchain.Block, error) {
	return nil, stdErrors.New("bad block")
}

func TestGetBlockWorkerAbortsOnNonRetryableError(t *testing.T) {
	hch := make(chan hashHeight, 1)
	hch <- hashHeight{hash: "hash", height: 1}
	close(hch)

	bch := []chan *bchain.Block{make(chan *bchain.Block)}
	hchClosed := atomic.Value{}
	hchClosed.Store(false)
	terminating := make(chan struct{})
	abortCh := make(chan error, 1)
	w := &SyncWorker{chain: &nonRetryableGetBlockChain{}}

	var wg sync.WaitGroup
	wg.Add(1)
	go w.getBlockWorker(0, 1, &wg, hch, bch, &hchClosed, terminating, abortCh)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("getBlockWorker did not exit after non-retryable error")
	}

	select {
	case err := <-abortCh:
		if err == nil || err.Error() != "bad block" {
			t.Fatalf("abort error = %v, want bad block", err)
		}
	default:
		t.Fatal("expected abort error")
	}
}

func TestWaitForBlockWorkersTerminatesOnAbort(t *testing.T) {
	var wg sync.WaitGroup
	terminating := make(chan struct{})
	abortCh := make(chan error, 1)
	workerReleased := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		abortCh <- stdErrors.New("tail block failed")
		<-terminating
		close(workerReleased)
	}()

	var closeOnce sync.Once
	var err error
	w := &SyncWorker{}
	w.waitForBlockWorkers(&wg, abortCh, func() {
		closeOnce.Do(func() {
			close(terminating)
		})
	}, &err, "test connect")

	if err == nil || err.Error() != "tail block failed" {
		t.Fatalf("waitForBlockWorkers() error = %v, want tail block failed", err)
	}
	select {
	case <-workerReleased:
	case <-time.After(time.Second):
		t.Fatal("worker was not released after abort")
	}
}

// TestWaitForBlockWorkersUnsticksBlockedWorker is the regression test for the
// coordinator-deadlock fix: one worker reports a fatal error via abortCh while
// another worker is parked on a bch send with no consumer. Without the
// abort-aware wait, wg.Wait() would block forever because the second worker
// can only escape by selecting on terminating.
func TestWaitForBlockWorkersUnsticksBlockedWorker(t *testing.T) {
	var wg sync.WaitGroup
	terminating := make(chan struct{})
	abortCh := make(chan error, 1)
	bch := make(chan *bchain.Block) // unbuffered, no consumer

	wg.Add(1)
	go func() {
		defer wg.Done()
		abortCh <- stdErrors.New("worker fatal")
	}()

	wg.Add(1)
	workerReleased := make(chan struct{})
	go func() {
		defer wg.Done()
		select {
		case bch <- &bchain.Block{}:
		case <-terminating:
			close(workerReleased)
		}
	}()

	var closeOnce sync.Once
	var err error
	w := &SyncWorker{}

	done := make(chan struct{})
	go func() {
		w.waitForBlockWorkers(&wg, abortCh, func() {
			closeOnce.Do(func() {
				close(terminating)
			})
		}, &err, "test")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("waitForBlockWorkers did not return; coordinator deadlocked")
	}

	select {
	case <-workerReleased:
	default:
		t.Fatal("blocked worker was not released by closeTerminating")
	}

	if err == nil || err.Error() != "worker fatal" {
		t.Fatalf("err = %v, want worker fatal", err)
	}
}

func TestWaitForBlockWorkersHandlesOsSignal(t *testing.T) {
	var wg sync.WaitGroup
	terminating := make(chan struct{})
	abortCh := make(chan error, 1)
	osSignal := make(chan os.Signal, 1)

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-terminating
	}()

	var closeOnce sync.Once
	var err error
	w := &SyncWorker{chanOsSignal: osSignal}

	done := make(chan struct{})
	go func() {
		w.waitForBlockWorkers(&wg, abortCh, func() {
			closeOnce.Do(func() {
				close(terminating)
			})
		}, &err, "test")
		close(done)
	}()

	osSignal <- syscall.SIGTERM

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("waitForBlockWorkers did not return on OS signal")
	}

	if !stdErrors.Is(err, ErrOperationInterrupted) {
		t.Fatalf("err = %v, want ErrOperationInterrupted", err)
	}
}

func TestRecordBlockWorkerAbortKeepsFirstError(t *testing.T) {
	var err error
	first := stdErrors.New("first")
	recordBlockWorkerAbort(&err, first, "test")
	if err == nil || err.Error() != "first" {
		t.Fatalf("err = %v, want first", err)
	}
	recordBlockWorkerAbort(&err, stdErrors.New("second"), "test")
	if err.Error() != "first" {
		t.Fatalf("err = %v, want first preserved", err)
	}

	// errResync routes through the warning branch but should still be captured
	// when no prior error exists.
	var resyncErr error
	recordBlockWorkerAbort(&resyncErr, errResync, "test")
	if !stdErrors.Is(resyncErr, errResync) {
		t.Fatalf("resyncErr = %v, want errResync", resyncErr)
	}
}

type reorgChain struct {
	bchain.BlockChain
	bestHeight  uint32
	currentHash string
	hashErr     error
}

func (c *reorgChain) GetBlock(hash string, height uint32) (*bchain.Block, error) {
	return nil, bchain.ErrBlockNotFound
}

func (c *reorgChain) GetBestBlockHeight() (uint32, error) {
	return c.bestHeight, nil
}

func (c *reorgChain) GetBlockHash(height uint32) (string, error) {
	return c.currentHash, c.hashErr
}

func TestShouldRestartSyncOnMissingBlock(t *testing.T) {
	tests := []struct {
		name        string
		chain       *reorgChain
		height      uint32
		expected    string
		wantRestart bool
		wantErr     bool
	}{
		{
			name:        "tip below requested height",
			chain:       &reorgChain{bestHeight: 5},
			height:      10,
			expected:    "expected",
			wantRestart: true,
		},
		{
			name:        "block hash gone",
			chain:       &reorgChain{bestHeight: 20, hashErr: bchain.ErrBlockNotFound},
			height:      10,
			expected:    "expected",
			wantRestart: true,
		},
		{
			name:        "different hash at height",
			chain:       &reorgChain{bestHeight: 20, currentHash: "different"},
			height:      10,
			expected:    "expected",
			wantRestart: true,
		},
		{
			name:        "matching hash",
			chain:       &reorgChain{bestHeight: 20, currentHash: "expected"},
			height:      10,
			expected:    "expected",
			wantRestart: false,
		},
		{
			name:     "GetBlockHash error propagates",
			chain:    &reorgChain{bestHeight: 20, hashErr: stdErrors.New("rpc fail")},
			height:   10,
			expected: "expected",
			wantErr:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := &SyncWorker{chain: tc.chain}
			restart, err := w.shouldRestartSyncOnMissingBlock(tc.height, tc.expected)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if restart != tc.wantRestart {
				t.Errorf("restart = %v, want %v", restart, tc.wantRestart)
			}
		})
	}
}

// TestGetBlockWorkerSignalsResyncOnReorg verifies that once the per-block
// retry threshold is exceeded and the chain confirms a reorg, the worker
// aborts the parallel sync via errResync rather than spinning forever.
func TestGetBlockWorkerSignalsResyncOnReorg(t *testing.T) {
	hch := make(chan hashHeight, 1)
	hch <- hashHeight{hash: "expected", height: 10}
	close(hch)

	bch := []chan *bchain.Block{make(chan *bchain.Block)}
	hchClosed := atomic.Value{}
	hchClosed.Store(true) // tail-of-range path uses TipRecheckThreshold
	terminating := make(chan struct{})
	abortCh := make(chan error, 1)

	w := &SyncWorker{
		chain: &reorgChain{bestHeight: 5},
		missingBlockRetry: MissingBlockRetryConfig{
			RecheckThreshold:    5,
			TipRecheckThreshold: 1,
			RetryDelay:          time.Millisecond,
		},
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go w.getBlockWorker(0, 1, &wg, hch, bch, &hchClosed, terminating, abortCh)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("getBlockWorker did not exit after reorg detection")
	}

	select {
	case err := <-abortCh:
		if !stdErrors.Is(err, errResync) {
			t.Fatalf("abort err = %v, want errResync", err)
		}
	default:
		t.Fatal("expected abortCh to receive errResync")
	}
}

// TestGetBlockWorkerExitsOnTerminating verifies that a worker stuck in its
// retry-delay sleep wakes up and exits promptly when the coordinator closes
// the terminating channel.
func TestGetBlockWorkerExitsOnTerminating(t *testing.T) {
	hch := make(chan hashHeight, 1)
	hch <- hashHeight{hash: "h", height: 1}
	// keep hch open so the worker stays in the retry loop

	bch := []chan *bchain.Block{make(chan *bchain.Block)}
	hchClosed := atomic.Value{}
	hchClosed.Store(false)
	terminating := make(chan struct{})
	abortCh := make(chan error, 1)

	w := &SyncWorker{
		chain: &blockingSendChain{},
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go w.getBlockWorker(0, 1, &wg, hch, bch, &hchClosed, terminating, abortCh)

	// Give the worker a moment to enter the bch send.
	time.Sleep(20 * time.Millisecond)
	close(terminating)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("getBlockWorker did not exit on terminating close")
	}
}

type blockingSendChain struct {
	bchain.BlockChain
}

func (c *blockingSendChain) GetBlock(hash string, height uint32) (*bchain.Block, error) {
	return &bchain.Block{BlockHeader: bchain.BlockHeader{Hash: hash, Height: height}}, nil
}
