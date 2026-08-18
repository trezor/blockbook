//go:build unittest

package eth

import (
	"net/http"
	"testing"
)

// The pool sizing is the whole point of the client, and losing DefaultTransport's proxy
// would break the CI runners that reach backends through an egress proxy.
func TestNewRPCHTTPClientPoolsConnections(t *testing.T) {
	client := newRPCHTTPClient()

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", client.Transport)
	}
	if transport.MaxIdleConnsPerHost != rpcMaxIdleConnsPerHost {
		t.Errorf("MaxIdleConnsPerHost = %d, want %d", transport.MaxIdleConnsPerHost, rpcMaxIdleConnsPerHost)
	}
	if transport.MaxIdleConnsPerHost <= http.DefaultMaxIdleConnsPerHost {
		t.Errorf("MaxIdleConnsPerHost = %d does not improve on the default %d",
			transport.MaxIdleConnsPerHost, http.DefaultMaxIdleConnsPerHost)
	}
	if transport.MaxIdleConns != rpcMaxIdleConns {
		t.Errorf("MaxIdleConns = %d, want %d", transport.MaxIdleConns, rpcMaxIdleConns)
	}

	defaultTransport := http.DefaultTransport.(*http.Transport)
	if transport.Proxy == nil {
		t.Error("Proxy must be inherited from DefaultTransport")
	}
	if transport.IdleConnTimeout != defaultTransport.IdleConnTimeout {
		t.Errorf("IdleConnTimeout = %s, want inherited %s", transport.IdleConnTimeout, defaultTransport.IdleConnTimeout)
	}
	if transport.TLSHandshakeTimeout != defaultTransport.TLSHandshakeTimeout {
		t.Errorf("TLSHandshakeTimeout = %s, want inherited %s", transport.TLSHandshakeTimeout, defaultTransport.TLSHandshakeTimeout)
	}
	// A blanket client timeout would cut long debug_traceBlockByHash reads that the
	// per-call rpc_timeout context is meant to govern.
	if client.Timeout != 0 {
		t.Errorf("Timeout = %s, want 0 (per-call contexts govern)", client.Timeout)
	}
}

// One client for the whole process: net/http pools per client, so a fresh client per
// dial would reuse nothing.
func TestRPCHTTPClientIsShared(t *testing.T) {
	if rpcHTTPClient == nil {
		t.Fatal("rpcHTTPClient must be initialized")
	}
	if rpcHTTPClient.Transport == http.DefaultTransport {
		t.Error("rpcHTTPClient must not fall back to http.DefaultTransport")
	}
}

func TestIsWebsocketRPCURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"ws://backend:8213", true},
		{"wss://backend:8213", true},
		{"WSS://backend:8213", true},
		{"  ws://backend:8213  ", true},
		{"http://backend:8313", false},
		{"https://backend:8313", false},
		{"HTTP://backend:8313", false},
		{"backend:8313", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isWebsocketRPCURL(tt.url); got != tt.want {
			t.Errorf("isWebsocketRPCURL(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

// Every dial must carry an option: an empty slice for the HTTP branch would silently
// restore go-ethereum's unpooled default client.
func TestRPCDialOptionsAlwaysConfigureTheDial(t *testing.T) {
	for _, rawURL := range []string{"http://backend:8313", "https://backend:8313", "ws://backend:8213", "wss://backend:8213"} {
		if got := len(RPCDialOptions(rawURL)); got != 1 {
			t.Errorf("RPCDialOptions(%q) returned %d options, want 1", rawURL, got)
		}
	}
}
