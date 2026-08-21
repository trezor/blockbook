//go:build unittest

package eth

import (
	"context"
	stdErrors "errors"
	"strings"
	"testing"
	"time"

	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
)

// mockBlockRawRPC returns a fixed error from CallContext, standing in for a backend
// that rejects a tip read (Avalanche's "cannot query unfinalized data" is mapped to
// bchain.ErrBlockNotFound by the avalanche client before it reaches getBlockRaw).
type mockBlockRawRPC struct {
	err error
}

func (m *mockBlockRawRPC) EthSubscribe(ctx context.Context, channel interface{}, args ...interface{}) (bchain.EVMClientSubscription, error) {
	return nil, stdErrors.New("not implemented")
}

func (m *mockBlockRawRPC) CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error {
	return m.err
}

func (m *mockBlockRawRPC) Close() {}

// Documents why getBlockRaw must not annotate a sentinel: the juju/errors pin has no
// Unwrap, so Annotatef is an errors.Is barrier. If this ever starts passing, juju was
// upgraded and the special case in getBlockRaw can be dropped.
func TestJujuAnnotatefHidesSentinelFromErrorsIs(t *testing.T) {
	annotated := errors.Annotatef(bchain.ErrBlockNotFound, "hash %v, height %v", "", 1)
	if stdErrors.Is(annotated, bchain.ErrBlockNotFound) {
		t.Fatal("Annotatef now preserves errors.Is; drop the sentinel special case in getBlockRaw")
	}
}

func TestGetBlockRawKeepsBlockNotFoundUnwrappable(t *testing.T) {
	b := &EthereumRPC{
		RPC:     &mockBlockRawRPC{err: bchain.ErrBlockNotFound},
		Timeout: time.Second,
	}

	_, err := b.getBlockRaw("", 93244979, true)
	if !stdErrors.Is(err, bchain.ErrBlockNotFound) {
		t.Fatalf("getBlockRaw error = %v, want it to satisfy errors.Is(ErrBlockNotFound)", err)
	}
}

func TestGetBlockRawAnnotatesOtherErrors(t *testing.T) {
	b := &EthereumRPC{
		RPC:     &mockBlockRawRPC{err: stdErrors.New("boom")},
		Timeout: time.Second,
	}

	_, err := b.getBlockRaw("", 93244979, true)
	if err == nil {
		t.Fatal("getBlockRaw error = nil, want an annotated error")
	}
	if !strings.Contains(err.Error(), "height 93244979") {
		t.Fatalf("getBlockRaw error = %q, want it annotated with the height", err.Error())
	}
}
