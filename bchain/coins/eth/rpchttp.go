package eth

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/ethereum/go-ethereum/rpc"
)

const (
	// rpcMaxIdleConnsPerHost sizes the keep-alive pool above the sync worker count
	// (-workers, 16 on the fastest chains) plus mempool and API callers, so concurrent
	// calls reuse sockets instead of dialing new ones. go-ethereum otherwise dials with
	// http.DefaultTransport, whose http.DefaultMaxIdleConnsPerHost of 2 made every extra
	// in-flight call close its socket: on a sub-second chain that burned ~340 ports/s into
	// TIME_WAIT, exhausted the host's ephemeral range and failed dials with EADDRNOTAVAIL,
	// which silently stalled sync (bitcoinrpc.go and tronhttp.go tune this for the same reason).
	rpcMaxIdleConnsPerHost = 100
	// rpcMaxIdleConns bounds the pool across hosts; a Blockbook instance talks to one
	// backend, so this only has to leave room for a redirected or fallback endpoint.
	rpcMaxIdleConns = 200
)

// rpcHTTPClient is shared by all JSON-RPC dials of this process: net/http pools per
// client, so a client per dial would defeat the reuse this exists for.
var rpcHTTPClient = newRPCHTTPClient()

func newRPCHTTPClient() *http.Client {
	// Clone keeps DefaultTransport's proxy, dial/TLS timeouts and HTTP/2 settings, which
	// the CI runners need to reach backends through an egress proxy; only the pool grows.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = rpcMaxIdleConns
	transport.MaxIdleConnsPerHost = rpcMaxIdleConnsPerHost
	// No client-level timeout on purpose: every call carries its own context deadline
	// (rpc_timeout), and a blanket timeout would also cut long debug_traceBlockByHash reads.
	return &http.Client{Transport: transport}
}

// RPCDialOptions returns the go-ethereum dial options for rawURL: the pooled HTTP client
// for http(s) endpoints, and an unlimited message size for ws(s) ones, which hold a single
// long-lived socket and so need no pooling. Shared by every EVM coin that dials a backend.
func RPCDialOptions(rawURL string) []rpc.ClientOption {
	if isWebsocketRPCURL(rawURL) {
		return []rpc.ClientOption{rpc.WithWebsocketMessageSizeLimit(0)}
	}
	return []rpc.ClientOption{rpc.WithHTTPClient(rpcHTTPClient)}
}

// isWebsocketRPCURL reports whether rawURL is dialed as a websocket. An unparsable or
// scheme-less URL falls through to the HTTP client, matching go-ethereum's own dispatch.
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
