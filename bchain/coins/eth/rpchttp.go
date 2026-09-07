package eth

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/ethereum/go-ethereum/rpc"
)

const (
	// go-ethereum's default of 2 idle conns per host burns a socket per extra in-flight call
	// and exhausts the ephemeral range; sized above -workers (16) as bitcoinrpc.go already is.
	rpcMaxIdleConnsPerHost = 100
	// Bounds the pool across hosts, leaving room for a second endpoint.
	rpcMaxIdleConns = 200
)

// One client per process: net/http pools per client, so a client per dial would pool nothing.
var rpcHTTPClient = newRPCHTTPClient()

func newRPCHTTPClient() *http.Client {
	// Clone keeps DefaultTransport's proxy, dial/TLS timeouts and HTTP/2; only the pool grows.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = rpcMaxIdleConns
	transport.MaxIdleConnsPerHost = rpcMaxIdleConnsPerHost
	// No client timeout: per-call rpc_timeout contexts govern, and a blanket one would cut
	// long debug_traceBlockByHash reads.
	return &http.Client{Transport: transport}
}

// RPCDialOptions returns the dial options for rawURL, shared by every EVM coin: the pooled
// client for http(s), and an unlimited message size for ws(s), which needs no pooling.
func RPCDialOptions(rawURL string) []rpc.ClientOption {
	if isWebsocketRPCURL(rawURL) {
		return []rpc.ClientOption{rpc.WithWebsocketMessageSizeLimit(0)}
	}
	return []rpc.ClientOption{rpc.WithHTTPClient(rpcHTTPClient)}
}

// isWebsocketRPCURL reports whether rawURL is dialed as a websocket; an unparsable or
// scheme-less URL falls through to HTTP, matching go-ethereum's own dispatch.
func isWebsocketRPCURL(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "ws", "wss":
		return true
	}
	return false
}
