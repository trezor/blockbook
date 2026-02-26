//go:build unittest

package db

import (
	"context"
	stdErrors "errors"
	"io"
	"net"
	"net/url"
	"syscall"
	"testing"

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
