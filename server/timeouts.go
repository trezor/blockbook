package server

import "time"

// HTTP server timeouts for the REST/UI (public) and admin/metrics (internal)
// interfaces. The public write timeout remains disabled because net/http
// applies it to the full handler and response lifetime, which can truncate
// valid expensive requests such as uncached xpub lookups.
const (
	// httpReadHeaderTimeout bounds the time a client may take to send the request line and headers
	httpReadHeaderTimeout = 10 * time.Second
	// httpReadTimeout bounds the whole request read
	httpReadTimeout = 30 * time.Second
	// httpPublicWriteTimeout avoids a hard cap on valid public handlers
	httpPublicWriteTimeout time.Duration = 0
	// httpInternalWriteTimeout bounds the short admin and metrics handlers
	httpInternalWriteTimeout = 120 * time.Second
	// httpIdleTimeout closes idle keep-alive connections
	httpIdleTimeout = 120 * time.Second
	// httpMaxHeaderBytes caps the request-header size at Go's default
	httpMaxHeaderBytes = 1 << 20
)
