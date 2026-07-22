package server

import "time"

// HTTP server timeouts for the REST/UI (public) and admin/metrics (internal) interfaces;
// Go's http.Server disables them all by default, so without these a connection that stalls before reaching
// a handler holds a goroutine and file descriptor indefinitely.
const (
	// httpReadHeaderTimeout bounds the time a client may take to send the request line and headers
	httpReadHeaderTimeout = 10 * time.Second
	// httpReadTimeout bounds the whole request read
	httpReadTimeout = 30 * time.Second
	// httpWriteTimeout bounds response writes
	httpWriteTimeout = 120 * time.Second
	// httpIdleTimeout closes idle keep-alive connections
	httpIdleTimeout = 120 * time.Second
	// httpMaxHeaderBytes caps the request-header size at Go's default
	httpMaxHeaderBytes = 1 << 20
)
