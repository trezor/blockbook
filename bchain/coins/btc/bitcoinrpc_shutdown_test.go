package btc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TestShutdownAbortsInFlightCall verifies that Shutdown cancels an in-flight RPC
// HTTP request promptly via the per-client base context, rather than letting it
// run to the (much longer) client timeout — which would otherwise delay process
// shutdown by up to that timeout. This is the shared seam that aborts sync RPCs
// for every BitcoinRPC-embedding coin reaching the backend through Call, and the
// same mechanism Tron threads into its HTTP node clients.
func TestShutdownAbortsInFlightCall(t *testing.T) {
	var startedOnce sync.Once
	started := make(chan struct{})
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedOnce.Do(func() { close(started) })
		// Hold the request open until the client cancels it (Shutdown) or the test ends.
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer srv.Close()
	defer close(release)

	ctx, cancel := context.WithCancel(context.Background())
	b := &BitcoinRPC{
		// A long client timeout makes the test fail (block ~30s) if cancellation regresses.
		client:       http.Client{Timeout: 30 * time.Second},
		rpcURL:       srv.URL,
		RPCMarshaler: JSONMarshalerV2{},
		callCtx:      ctx,
		cancelCall:   cancel,
	}

	errCh := make(chan error, 1)
	go func() {
		_, err := b.GetBestBlockHash()
		errCh <- err
	}()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the RPC request to reach the server")
	}

	if err := b.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected GetBestBlockHash to fail after Shutdown, got nil")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("GetBestBlockHash did not return after Shutdown; in-flight call was not cancelled")
	}
}
