//go:build unittest

package db

import (
	"context"
	stdErrors "errors"
	"io"
	"net"
	"net/url"
	"sync/atomic"
	"syscall"
	"testing"

	jujuErrors "github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
)

// recorderChain is a minimal bchain.BlockChain stand-in that records which
// methods were invoked on it. It panics on any unmocked method so tests catch
// unintended fan-out across the chain/tipChain boundary.
type recorderChain struct {
	bchain.BlockChain // nil — calls to unmocked methods panic
	getBlockCalls     atomic.Int32
}

func (r *recorderChain) GetBlock(hash string, height uint32) (*bchain.Block, error) {
	n := r.getBlockCalls.Add(1)
	// One block, then signal end-of-chain so getBlockChain's loop exits.
	if n > 1 {
		return nil, bchain.ErrBlockNotFound
	}
	return &bchain.Block{
		BlockHeader: bchain.BlockHeader{Hash: hash, Height: height},
	}, nil
}

// TestSyncWorkerGetBlockChainUsesTipChain locks in the routing contract added
// by the traffic-routing change: the sequential connectBlocks/getBlockChain
// path must fetch blocks via tipChain (which on EVM coins applies
// WithSyncRoute → WS), and must not fan out to chain (which carries bulk and
// public-API HTTP traffic).
func TestSyncWorkerGetBlockChainUsesTipChain(t *testing.T) {
	main := &recorderChain{}
	tip := &recorderChain{}
	w := &SyncWorker{
		chain:       main,
		tipChain:    tip,
		startHash:   "0xtip",
		startHeight: 100,
	}

	out := make(chan blockResult, 4)
	done := make(chan struct{})
	w.getBlockChain(out, done)

	if got := tip.getBlockCalls.Load(); got == 0 {
		t.Fatalf("expected tipChain.GetBlock to be called, got 0")
	}
	if got := main.getBlockCalls.Load(); got != 0 {
		t.Fatalf("chain.GetBlock must not be called from getBlockChain, got %d", got)
	}
}

// TestSyncWorkerNilTipChainFallsBackToChain documents the safety net in
// NewSyncWorkerWithConfig: when no tipChain is supplied (non-EVM coins, older
// tests, etc.) the sequential path stays on the regular chain.
func TestSyncWorkerNilTipChainFallsBackToChain(t *testing.T) {
	main := &recorderChain{}
	w, err := NewSyncWorkerWithConfig(nil, main, nil, 1, 1, 0, true, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewSyncWorkerWithConfig: %v", err)
	}
	if w.tipChain != bchain.BlockChain(main) {
		t.Fatalf("tipChain must default to chain when nil; got %p, want %p", w.tipChain, main)
	}
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
