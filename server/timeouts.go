package server

import "time"

// HTTP server timeouts that harden the REST/UI (public) and admin/metrics
// (internal) interfaces against slow-request (Slowloris) resource-exhaustion
// attacks. Go's http.Server ships with every timeout disabled, so without these
// a single host can open thousands of connections and dribble the request line
// and headers one byte at a time (or complete the headers and then stall on the
// body). Such connections never reach a handler and therefore never reach the
// REST/UI rate limiters or the WebSocket connection accounting (all of which
// run inside handlers), yet each pins a goroutine and a file descriptor
// indefinitely, eventually exhausting FDs and taking down both the public API
// and the WebSocket capacity.
const (
	// httpReadHeaderTimeout bounds the time a client may take to send the
	// request line and all headers. This is the primary Slowloris defense.
	// net/http applies it only while reading headers and resets the underlying
	// read deadline before the handler runs (ReadTimeout governs the body), so
	// it never interferes with the long-lived WebSocket connections that upgrade
	// inside a handler.
	httpReadHeaderTimeout = 10 * time.Second
	// httpReadTimeout bounds the whole request read, including a body that is
	// sent slowly or never completed. The public server clears this deadline on
	// the connections it hijacks for WebSocket (see WebsocketServer.ServeHTTP),
	// so idle subscription streams are unaffected.
	httpReadTimeout = 30 * time.Second
	// httpWriteTimeout bounds response writes, defending against slow-read
	// ("RUDY") clients that read the response one byte at a time to hold the
	// connection open. It is deliberately generous so that paginated API and
	// explorer responses over slow links are never truncated, and it is cleared
	// for hijacked WebSocket connections (outputLoop sets its own per-message
	// write deadline).
	httpWriteTimeout = 120 * time.Second
	// httpIdleTimeout closes keep-alive connections left idle between requests.
	// It does not apply to hijacked WebSocket connections.
	httpIdleTimeout = 120 * time.Second
	// httpMaxHeaderBytes caps the accepted request-header size. This is Go's
	// default value, pinned explicitly so header-memory growth is bounded by
	// intent rather than by a library default that could change.
	httpMaxHeaderBytes = 1 << 20
)
