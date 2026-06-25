//go:build unittest

package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newTestRestUIRateLimiter() *restUIRateLimiter {
	return &restUIRateLimiter{
		clients:       make(map[string]*restUIClientLimit),
		rateLimit:     0,
		rateWindow:    time.Minute,
		burst:         1,
		maxConcurrent: 0,
		stateTTL:      defaultRestUIStateTTL,
	}
}

type trackingBody struct {
	read bool
}

func (b *trackingBody) Read(_ []byte) (int, error) {
	b.read = true
	return 0, io.EOF
}

func (b *trackingBody) Close() error {
	return nil
}

func TestRestUIRateLimiterRejectsWith429BeforeHandlerWork(t *testing.T) {
	limiter := newTestRestUIRateLimiter()
	limiter.rateLimit = 1
	limiter.burst = 1
	var handlerCalls int
	handler := limiter.wrapPublic(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalls++
		w.WriteHeader(http.StatusNoContent)
	}), "/")

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/v2/status", nil)
	req.RemoteAddr = "192.0.2.1:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("first request status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	body := &trackingBody{}
	req = httptest.NewRequest(http.MethodPost, "http://example.com/api/v2/sendtx/", body)
	req.RemoteAddr = "192.0.2.1:12345"
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second request status = %d, want %d", rec.Code, http.StatusTooManyRequests)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := rec.Header().Get("Retry-After"); got == "" {
		t.Fatal("Retry-After header missing")
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"error":"rate limit exceeded"}` {
		t.Fatalf("body = %q", got)
	}
	if handlerCalls != 1 {
		t.Fatalf("handler calls = %d, want 1", handlerCalls)
	}
	if body.read {
		t.Fatal("request body was read before rate-limit rejection")
	}
}

func TestRestUIRateLimiterConcurrentRequests(t *testing.T) {
	limiter := newTestRestUIRateLimiter()
	limiter.maxConcurrent = 1
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	var startedOnce sync.Once

	handler := limiter.wrapPublic(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedOnce.Do(func() { close(started) })
		<-release
		w.WriteHeader(http.StatusNoContent)
	}), "/")

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/v2/status", nil)
	req.RemoteAddr = "192.0.2.2:12345"
	go func(req *http.Request) {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Errorf("first request status = %d, want %d", rec.Code, http.StatusNoContent)
		}
		close(done)
	}(req)
	<-started

	req = httptest.NewRequest(http.MethodGet, "http://example.com/api/v2/status", nil)
	req.RemoteAddr = "192.0.2.2:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("concurrent request status = %d, want %d", rec.Code, http.StatusTooManyRequests)
	}

	close(release)
	<-done

	req = httptest.NewRequest(http.MethodGet, "http://example.com/api/v2/status", nil)
	req.RemoteAddr = "192.0.2.2:12345"
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("request after release status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestRestUIRateLimiterRouteScope(t *testing.T) {
	limiter := newTestRestUIRateLimiter()
	limiter.rateLimit = 1
	limiter.burst = 1
	var handlerCalls int
	handler := limiter.wrapPublic(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalls++
		w.WriteHeader(http.StatusNoContent)
	}), "/bb/")

	// Static assets, api-docs, the OpenAPI spec, and the WebSocket endpoint bypass
	// the limiter and are served unconditionally.
	for _, path := range []string{
		"/bb/static/app.js",
		"/bb/websocket",
		"/bb/favicon.ico",
		"/bb/api-docs",
		"/bb/openapi.yaml",
		"/bb/test-websocket.html",
	} {
		req := httptest.NewRequest(http.MethodGet, "http://example.com"+path, nil)
		req.RemoteAddr = "192.0.2.3:12345"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("%s status = %d, want %d", path, rec.Code, http.StatusNoContent)
		}
	}

	// One dynamic route consumes the single burst token; the next dynamic route is
	// rejected, regardless of whether it is the API or an explorer UI page.
	// deny-all-except-static limits every non-excluded route.
	for i, path := range []string{"/bb/address/abc", "/bb/api/v2/status"} {
		req := httptest.NewRequest(http.MethodGet, "http://example.com"+path, nil)
		req.RemoteAddr = "192.0.2.3:12345"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		wantCode := http.StatusNoContent
		if i > 0 {
			wantCode = http.StatusTooManyRequests
		}
		if rec.Code != wantCode {
			t.Fatalf("%s status = %d, want %d", path, rec.Code, wantCode)
		}
	}
	// 6 bypass requests + 1 accepted dynamic request reached the handler.
	if handlerCalls != 7 {
		t.Fatalf("handler calls = %d, want 7", handlerCalls)
	}
}

func TestClientIPConfigEnvFallback(t *testing.T) {
	t.Run("shared trusted proxies override legacy websocket fallback", func(t *testing.T) {
		prefix := "CLIENTIPTESTA"
		t.Setenv(prefix+"_TRUSTED_PROXIES", "203.0.113.0/24")
		t.Setenv(prefix+"_WS_TRUSTED_PROXIES", "198.51.100.0/24")
		cfg, err := readClientIPConfig(prefix)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.trustedEnvName != prefix+"_TRUSTED_PROXIES" {
			t.Fatalf("trusted env = %q", cfg.trustedEnvName)
		}
		if !prefixesContain(cfg.trustedProxies, "203.0.113.0/24") || prefixesContain(cfg.trustedProxies, "198.51.100.0/24") {
			t.Fatalf("trusted proxies = %v", cfg.trustedProxies)
		}
	})

	t.Run("unset shared trusted proxies fall back to legacy websocket", func(t *testing.T) {
		prefix := "CLIENTIPTESTB"
		t.Setenv(prefix+"_WS_TRUSTED_PROXIES", "198.51.100.0/24")
		cfg, err := readClientIPConfig(prefix)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.trustedEnvName != prefix+"_WS_TRUSTED_PROXIES" {
			t.Fatalf("trusted env = %q", cfg.trustedEnvName)
		}
		if !prefixesContain(cfg.trustedProxies, "198.51.100.0/24") {
			t.Fatalf("trusted proxies = %v", cfg.trustedProxies)
		}
	})

	t.Run("explicit shared trusted proxies do not fall back", func(t *testing.T) {
		prefix := "CLIENTIPTESTC"
		t.Setenv(prefix+"_TRUSTED_PROXIES", "")
		t.Setenv(prefix+"_WS_TRUSTED_PROXIES", "198.51.100.0/24")
		cfg, err := readClientIPConfig(prefix)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.trustedEnvName != prefix+"_TRUSTED_PROXIES" {
			t.Fatalf("trusted env = %q", cfg.trustedEnvName)
		}
		if len(cfg.trustedProxies) != 0 {
			t.Fatalf("trusted proxies = %v, want none", cfg.trustedProxies)
		}
	})

	t.Run("shared Cloudflare CIDRs override legacy websocket fallback", func(t *testing.T) {
		prefix := "CLIENTIPTESTD"
		t.Setenv(prefix+"_CLOUDFLARE_IPS", "203.0.113.0/24")
		t.Setenv(prefix+"_WS_CLOUDFLARE_IPS", "198.51.100.0/24")
		cfg, err := readClientIPConfig(prefix)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.cloudflareEnvName != prefix+"_CLOUDFLARE_IPS" {
			t.Fatalf("Cloudflare env = %q", cfg.cloudflareEnvName)
		}
		if !prefixesContain(cfg.cloudflarePrefixes, "203.0.113.0/24") || prefixesContain(cfg.cloudflarePrefixes, "198.51.100.0/24") {
			t.Fatalf("Cloudflare CIDRs = %v", cfg.cloudflarePrefixes)
		}
	})

	t.Run("unset shared Cloudflare CIDRs fall back to legacy websocket", func(t *testing.T) {
		prefix := "CLIENTIPTESTE"
		t.Setenv(prefix+"_WS_CLOUDFLARE_IPS", "203.0.113.0/24")
		cfg, err := readClientIPConfig(prefix)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.cloudflareEnvName != prefix+"_WS_CLOUDFLARE_IPS" {
			t.Fatalf("Cloudflare env = %q", cfg.cloudflareEnvName)
		}
		if !prefixesContain(cfg.cloudflarePrefixes, "203.0.113.0/24") {
			t.Fatalf("Cloudflare CIDRs = %v", cfg.cloudflarePrefixes)
		}
	})

	t.Run("explicit shared Cloudflare off does not fall back", func(t *testing.T) {
		prefix := "CLIENTIPTESTF"
		t.Setenv(prefix+"_CLOUDFLARE_IPS", "off")
		t.Setenv(prefix+"_WS_CLOUDFLARE_IPS", "203.0.113.0/24")
		cfg, err := readClientIPConfig(prefix)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.cloudflareEnvName != prefix+"_CLOUDFLARE_IPS" {
			t.Fatalf("Cloudflare env = %q", cfg.cloudflareEnvName)
		}
		if len(cfg.cloudflarePrefixes) != 0 {
			t.Fatalf("Cloudflare CIDRs = %v, want none", cfg.cloudflarePrefixes)
		}
	})

	t.Run("Pseudo-IPv4 defaults to off and parses true", func(t *testing.T) {
		prefix := "CLIENTIPTESTG"
		cfg, err := readClientIPConfig(prefix)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.trustPseudoIPv6 {
			t.Fatalf("trustPseudoIPv6 = true by default, want false")
		}
		t.Setenv(prefix+"_CLOUDFLARE_PSEUDO_IPV4", "true")
		cfg, err = readClientIPConfig(prefix)
		if err != nil {
			t.Fatal(err)
		}
		if !cfg.trustPseudoIPv6 {
			t.Fatalf("trustPseudoIPv6 = false, want true")
		}
	})

	t.Run("Pseudo-IPv4 rejects an unparseable value", func(t *testing.T) {
		prefix := "CLIENTIPTESTH"
		t.Setenv(prefix+"_CLOUDFLARE_PSEUDO_IPV4", "maybe")
		if _, err := readClientIPConfig(prefix); err == nil {
			t.Fatal("expected error for invalid CLOUDFLARE_PSEUDO_IPV4, got nil")
		}
	})
}

func TestRestUIRateLimiterTemporaryBlock(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	limiter := newTestRestUIRateLimiter()
	limiter.rateLimit = 1
	limiter.burst = 1
	limiter.blockDuration = time.Minute

	if decision := limiter.accept("192.0.2.10", "192.0.2.10", true, now); !decision.accepted {
		t.Fatalf("first decision = %+v, want accepted", decision)
	}
	limiter.release("192.0.2.10", now)
	// breaches must be separate episodes (>= restUIBreachMinSpacing apart) to
	// count toward the block threshold
	var last time.Time
	for i := 0; i < restUIBreachBlockThreshold; i++ {
		last = now.Add(time.Duration(i) * restUIBreachMinSpacing)
		decision := limiter.accept("192.0.2.10", "192.0.2.10", true, last)
		if decision.accepted || decision.reason != restUIRejectRequestRate {
			t.Fatalf("breach %d decision = %+v, want request-rate rejection", i, decision)
		}
	}
	decision := limiter.accept("192.0.2.10", "192.0.2.10", true, last.Add(time.Second))
	if decision.accepted || decision.reason != restUIRejectIPBlocked {
		t.Fatalf("blocked decision = %+v, want ip_blocked", decision)
	}

	// a burst of same-instant rejections is one breach episode and must not
	// trip the temporary block
	limiter = newTestRestUIRateLimiter()
	limiter.rateLimit = 1
	limiter.burst = 1
	limiter.blockDuration = time.Minute
	if decision := limiter.accept("192.0.2.11", "192.0.2.11", true, now); !decision.accepted {
		t.Fatalf("first burst decision = %+v, want accepted", decision)
	}
	limiter.release("192.0.2.11", now)
	for i := 0; i < 3*restUIBreachBlockThreshold; i++ {
		decision := limiter.accept("192.0.2.11", "192.0.2.11", true, now)
		if decision.accepted || decision.reason != restUIRejectRequestRate {
			t.Fatalf("burst rejection %d decision = %+v, want request-rate rejection", i, decision)
		}
	}
	if limiter.clients["192.0.2.11"].blockedUntil.After(now) {
		t.Fatal("same-instant rejection burst tripped the temporary block")
	}

	limiter = newTestRestUIRateLimiter()
	limiter.rateLimit = 1
	limiter.burst = 1
	limiter.blockDuration = time.Minute
	if decision := limiter.accept("127.0.0.1", "127.0.0.1", false, now); !decision.accepted {
		t.Fatalf("first unblockable decision = %+v, want accepted", decision)
	}
	limiter.release("127.0.0.1", now)
	for i := 0; i < restUIBreachBlockThreshold+1; i++ {
		decision = limiter.accept("127.0.0.1", "127.0.0.1", false, now.Add(time.Duration(i)*restUIBreachMinSpacing))
		if decision.accepted || decision.reason != restUIRejectRequestRate {
			t.Fatalf("unblockable breach %d decision = %+v, want request-rate rejection", i, decision)
		}
	}
	if limiter.clients["127.0.0.1"].blockedUntil.After(now) {
		t.Fatal("unblockable key was temporarily blocked")
	}
}

func TestRestUIRateLimiterIPv6BlockIsPer128(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	limiter := newTestRestUIRateLimiter()
	limiter.rateLimit = 1
	limiter.burst = 1
	limiter.blockDuration = time.Minute

	// Two distinct /128s sharing one /64. The rate limiter aggregates to /64
	// (key64); the block must accrue per /128 (b1, b2).
	key64 := rateLimitKey("2001:db8:1:2::1")
	b1 := blockKey("2001:db8:1:2::1")
	b2 := blockKey("2001:db8:1:2::2")
	if key64 != rateLimitKey("2001:db8:1:2::2") {
		t.Fatalf("test setup: addresses are not in the same /64 (%q vs %q)", key64, rateLimitKey("2001:db8:1:2::2"))
	}

	// Consume the single shared token, then drive separate breach episodes that
	// are all attributed to b1.
	if d := limiter.accept(key64, b1, true, now); !d.accepted {
		t.Fatalf("first decision = %+v, want accepted", d)
	}
	limiter.release(key64, now)
	var last time.Time
	for i := 0; i < restUIBreachBlockThreshold; i++ {
		last = now.Add(time.Duration(i) * restUIBreachMinSpacing)
		if d := limiter.accept(key64, b1, true, last); d.accepted || d.reason != restUIRejectRequestRate {
			t.Fatalf("breach %d decision = %+v, want request-rate rejection", i, d)
		}
	}
	after := last.Add(time.Second)
	if d := limiter.accept(key64, b1, true, after); d.accepted || d.reason != restUIRejectIPBlocked {
		t.Fatalf("b1 decision = %+v, want ip_blocked", d)
	}

	// A different /128 in the same /64 must NOT inherit the block: it is still
	// subject to the shared /64 rate limit (request_rate) but never ip_blocked.
	if d := limiter.accept(key64, b2, true, after); d.reason == restUIRejectIPBlocked {
		t.Fatalf("b2 (same /64) wrongly ip_blocked: %+v", d)
	}
	if c := limiter.clients[b2]; c != nil && c.blockedUntil.After(after) {
		t.Fatal("b2 must not be blocked when only b1 breached")
	}
}

func TestRestUIRateLimiterCleanup(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	limiter := newTestRestUIRateLimiter()
	limiter.clients["idle"] = &restUIClientLimit{lastSeen: now.Add(-limiter.stateTTL - time.Second)}
	limiter.clients["active"] = &restUIClientLimit{active: 1, lastSeen: now.Add(-limiter.stateTTL - time.Second)}
	limiter.clients["blocked"] = &restUIClientLimit{blockedUntil: now.Add(time.Minute), lastSeen: now.Add(-limiter.stateTTL - time.Second)}

	limiter.sweep(now)
	if _, ok := limiter.clients["idle"]; ok {
		t.Fatal("idle client was not removed")
	}
	if _, ok := limiter.clients["active"]; !ok {
		t.Fatal("active client was removed")
	}
	if _, ok := limiter.clients["blocked"]; !ok {
		t.Fatal("blocked client was removed")
	}
}

func TestRestUIRouteMatching(t *testing.T) {
	tests := []struct {
		path     string
		basePath string
		want     bool
	}{
		// Dynamic routes (API + explorer UI) are limited under the base path.
		{"/", "/", true},
		{"/api", "/", true},
		{"/api/v2/status", "/", true},
		{"/address/abc", "/", true},
		{"/xpub/x", "/", true},
		{"/apix", "/", true},
		{"/bb/", "/bb/", true},
		{"/bb/api/v2/status", "/bb/", true},
		{"/bb/address/abc", "/bb/", true},
		// Static assets, docs, the OpenAPI spec, and WebSocket are exempt.
		{"/static/app.js", "/", false},
		{"/favicon.ico", "/", false},
		{"/api-docs", "/", false},
		{"/api-docs/", "/", false},
		// A path that only shares the "api-docs" prefix is not the docs route and
		// must stay limited (it falls through to the explorer index handler).
		{"/api-docsx", "/", true},
		{"/openapi.yaml", "/", false},
		{"/websocket", "/", false},
		{"/test-websocket.html", "/", false},
		{"/bb/static/app.js", "/bb/", false},
		{"/bb/websocket", "/bb/", false},
		// Requests outside the configured base path are not limited here.
		{"/other/path", "/bb/", false},
	}
	for _, tt := range tests {
		if got := isRateLimitedRoute(tt.path, tt.basePath); got != tt.want {
			t.Fatalf("isRateLimitedRoute(%q, %q) = %v, want %v", tt.path, tt.basePath, got, tt.want)
		}
	}
}

func TestRestUIRateLimiterStats(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	limiter := newTestRestUIRateLimiter()
	limiter.clients["a"] = &restUIClientLimit{active: 2}
	limiter.clients["b"] = &restUIClientLimit{active: 1, blockedUntil: now.Add(time.Minute)}
	limiter.clients["c"] = &restUIClientLimit{}

	activeIPs, maxActive, blockedIPs := limiter.stats(now)
	if activeIPs != 2 || maxActive != 2 || blockedIPs != 1 {
		t.Fatalf("stats = %d, %d, %d; want 2, 2, 1", activeIPs, maxActive, blockedIPs)
	}
}

func prefixesContain(prefixes []netip.Prefix, cidr string) bool {
	want := netip.MustParsePrefix(cidr)
	for _, p := range prefixes {
		if p == want {
			return true
		}
	}
	return false
}

func TestRestUIRateLimiterReleaseIsPerKey(t *testing.T) {
	limiter := newTestRestUIRateLimiter()
	limiter.maxConcurrent = 2
	now := time.Unix(1_700_000_000, 0)
	if decision := limiter.accept("192.0.2.20", "192.0.2.20", true, now); !decision.accepted {
		t.Fatalf("accept = %+v", decision)
	}
	if decision := limiter.accept("192.0.2.20", "192.0.2.20", true, now); !decision.accepted {
		t.Fatalf("second accept = %+v", decision)
	}
	limiter.release("192.0.2.20", now)
	if got := limiter.clients["192.0.2.20"].active; got != 1 {
		t.Fatalf("active = %d, want 1", got)
	}
	limiter.release("192.0.2.20", now)
	if got := limiter.clients["192.0.2.20"].active; got != 0 {
		t.Fatalf("active after second release = %d, want 0", got)
	}
}

func TestRestUIRateLimiterBypassDoesNotConsumeToken(t *testing.T) {
	limiter := newTestRestUIRateLimiter()
	limiter.rateLimit = 1
	limiter.burst = 1
	var calls atomic.Int32
	handler := limiter.wrapPublic(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}), "/")

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/static/app.js", nil)
		req.RemoteAddr = "192.0.2.30:12345"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("bypass request %d status = %d, want %d", i, rec.Code, http.StatusNoContent)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/v2/status", nil)
	req.RemoteAddr = "192.0.2.30:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("API request after bypass status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if calls.Load() != 6 {
		t.Fatalf("handler calls = %d, want 6", calls.Load())
	}
}

func TestRestUIRateLimiterLocalPeerBypass(t *testing.T) {
	limiter := newTestRestUIRateLimiter()
	limiter.rateLimit = 1
	limiter.burst = 1
	handler := limiter.wrapPublic(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), "/")

	// A loopback/private peer without any attribution header is the operator's
	// own tooling or a proxy that forwards no client IP; limiting that key would
	// throttle a whole deployment as one client, so it is exempt.
	for _, remote := range []string{"127.0.0.1:40000", "[::1]:40000", "10.1.2.3:40000"} {
		for i := 0; i < 5; i++ {
			req := httptest.NewRequest(http.MethodGet, "http://example.com/api/v2/status", nil)
			req.RemoteAddr = remote
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusNoContent {
				t.Fatalf("local request %d from %s status = %d, want %d", i, remote, rec.Code, http.StatusNoContent)
			}
		}
	}
	if len(limiter.clients) != 0 {
		t.Fatalf("local bypass created limiter state for %d keys", len(limiter.clients))
	}

	// The same loopback peer forwarding a client IP via X-Real-Ip is limited on
	// that client key.
	for i, want := range []int{http.StatusNoContent, http.StatusTooManyRequests} {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/api/v2/status", nil)
		req.RemoteAddr = "127.0.0.1:40000"
		req.Header.Set("X-Real-Ip", "192.0.2.77")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("attributed request %d status = %d, want %d", i, rec.Code, want)
		}
	}
}

func TestRestUIRateLimiterTrackingCap(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	limiter := newTestRestUIRateLimiter()
	limiter.rateLimit = 1
	limiter.burst = 1
	for i := 0; i < restUIMaxTrackedClients; i++ {
		limiter.clients[strconv.Itoa(i)] = &restUIClientLimit{lastSeen: now}
	}

	decision := limiter.accept("192.0.2.99", "192.0.2.99", true, now)
	if !decision.accepted || !decision.untracked {
		t.Fatalf("decision at cap = %+v, want accepted and untracked", decision)
	}
	if _, ok := limiter.clients["192.0.2.99"]; ok {
		t.Fatal("untracked accept created limiter state")
	}
	// release of an untracked key is a no-op
	limiter.release("192.0.2.99", now)

	// already-tracked keys stay limited at the cap
	if decision := limiter.accept("0", "0", true, now); !decision.accepted {
		t.Fatalf("tracked accept = %+v, want accepted", decision)
	}
	if decision := limiter.accept("0", "0", true, now); decision.accepted {
		t.Fatalf("tracked second accept = %+v, want rejected", decision)
	}
}
