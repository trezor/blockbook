package server

import (
	"fmt"
	"math"
	"net/http"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/trezor/blockbook/common"
)

const (
	// Public HTTP (explorer UI + REST API) is for individual human use; Suite uses
	// the WebSocket interface. Keep per-client rate/burst tight so a distributed
	// crawler needs many more source IPs to sustain a given aggregate rate.
	defaultRestUIRateLimit     = 20
	defaultRestUIRateWindow    = time.Minute
	defaultRestUIBurst         = 20
	defaultRestUIMaxConcurrent = 12
	defaultRestUIStateTTL      = 10 * time.Minute
	defaultRestUIBlockDuration = 0

	restUILimiterCleanupInterval = time.Minute
	restUIBreachWindow           = 10 * time.Minute
	restUIBreachBlockThreshold   = 3
	// restUIBreachMinSpacing is the minimum quiet gap between counted breaches:
	// one burst produces many rejections in the same instant (e.g. a page firing
	// dozens of parallel fetches), and the block threshold should mean separate
	// abuse episodes, not one spike.
	restUIBreachMinSpacing = 10 * time.Second
	// restUIMaxTrackedClients bounds the per-key state map so a flood rotating
	// client keys cannot grow it (and the sweeps over it) without bound; past the
	// cap, new keys are temporarily admitted untracked (fail open) while
	// already-tracked keys stay limited.
	restUIMaxTrackedClients = 100_000
)

const (
	restUIRejectRequestRate        = "request_rate"
	restUIRejectConcurrentRequests = "concurrent_requests"
	restUIRejectIPBlocked          = "ip_blocked"
)

type restUILimiterConfig struct {
	rateLimit       int
	rateWindow      time.Duration
	burst           int
	maxConcurrent   int
	stateTTL        time.Duration
	blockDuration   time.Duration
	trustedProxies  []netip.Prefix
	cloudflareCIDRs []netip.Prefix
	trustPseudoIPv6 bool
}

type restUIRateLimiter struct {
	mux                sync.Mutex
	clients            map[string]*restUIClientLimit
	lastCleanup        time.Time
	capWarned          bool
	metrics            *common.Metrics
	rateLimit          int
	rateWindow         time.Duration
	burst              int
	maxConcurrent      int
	stateTTL           time.Duration
	blockDuration      time.Duration
	trustedProxies     []netip.Prefix
	cloudflarePrefixes []netip.Prefix
	trustPseudoIPv6    bool
	localBypassWarn    sync.Once
}

type restUIClientLimit struct {
	active         int
	bucket         restUITokenBucket
	breaches       []time.Time
	blockedUntil   time.Time
	blockRejected  int
	lastSeen       time.Time
	lastRejectLog  time.Time
	lastRejectKind string
}

type restUITokenBucket struct {
	tokens     float64
	lastRefill time.Time
}

type restUILimitDecision struct {
	accepted bool
	// untracked marks an accepted request for which no per-key state was
	// created (tracking-cap fail-open); the caller must not release it.
	untracked  bool
	reason     string
	retryAfter time.Duration
	// shouldLog is the per-client log-throttling decision for a rejection,
	// made inside accept while the client state is at hand so the caller can
	// log outside the limiter lock.
	shouldLog bool
}

func newRestUIRateLimiter(network string, metrics *common.Metrics) (*restUIRateLimiter, error) {
	cfg, err := readRestUILimiterConfig(network)
	if err != nil {
		return nil, err
	}
	if cfg.rateLimit == 0 && cfg.maxConcurrent == 0 {
		glog.Info("REST/UI rate limiter disabled")
		return nil, nil
	}
	l := &restUIRateLimiter{
		clients:            make(map[string]*restUIClientLimit),
		metrics:            metrics,
		rateLimit:          cfg.rateLimit,
		rateWindow:         cfg.rateWindow,
		burst:              cfg.burst,
		maxConcurrent:      cfg.maxConcurrent,
		stateTTL:           cfg.stateTTL,
		blockDuration:      cfg.blockDuration,
		trustedProxies:     cfg.trustedProxies,
		cloudflarePrefixes: cfg.cloudflareCIDRs,
		trustPseudoIPv6:    cfg.trustPseudoIPv6,
	}
	if metrics != nil {
		metrics.RestUIActiveIPs.Set(0)
		metrics.RestUIMaxActiveRequestsPerIP.Set(0)
		metrics.RestUIBlockedIPs.Set(0)
	}
	if cfg.rateLimit > 0 {
		glog.Infof("REST/UI rate limit: %d requests / %s; burst: %d", cfg.rateLimit, cfg.rateWindow, cfg.burst)
	} else {
		glog.Info("REST/UI request-rate limit disabled")
	}
	if cfg.maxConcurrent > 0 {
		glog.Infof("REST/UI per-client concurrency limit: %d active requests", cfg.maxConcurrent)
	} else {
		glog.Info("REST/UI per-client concurrency limit disabled")
	}
	if cfg.blockDuration > 0 {
		glog.Infof("REST/UI temporary IP block enabled after repeated breaches: %s", cfg.blockDuration)
		if len(cfg.cloudflareCIDRs) == 0 {
			glog.Warning("REST/UI temporary IP block is enabled without Cloudflare peer verification; CF-Connecting-* derived addresses are not blockable in this mode")
		}
	}
	go l.runMaintenance(restUILimiterCleanupInterval)
	return l, nil
}

func readRestUILimiterConfig(network string) (restUILimiterConfig, error) {
	prefix := strings.ToUpper(network)
	cfg := restUILimiterConfig{
		rateLimit:     defaultRestUIRateLimit,
		rateWindow:    defaultRestUIRateWindow,
		burst:         defaultRestUIBurst,
		maxConcurrent: defaultRestUIMaxConcurrent,
		stateTTL:      defaultRestUIStateTTL,
		blockDuration: defaultRestUIBlockDuration,
	}

	var err error
	if cfg.rateLimit, err = parseNonNegativeIntEnv(prefix+"_REST_UI_RATE_LIMIT", cfg.rateLimit); err != nil {
		return cfg, err
	}
	if cfg.rateWindow, err = parsePositiveDurationEnv(prefix+"_REST_UI_RATE_WINDOW", cfg.rateWindow); err != nil {
		return cfg, err
	}
	if cfg.burst, err = parseNonNegativeIntEnv(prefix+"_REST_UI_BURST", cfg.burst); err != nil {
		return cfg, err
	}
	if cfg.rateLimit > 0 && cfg.burst <= 0 {
		return cfg, fmt.Errorf("%s_REST_UI_BURST: invalid value %d (want a positive integer when REST request-rate limiting is enabled)", prefix, cfg.burst)
	}
	if cfg.maxConcurrent, err = parseNonNegativeIntEnv(prefix+"_REST_UI_MAX_CONCURRENT", cfg.maxConcurrent); err != nil {
		return cfg, err
	}
	if cfg.stateTTL, err = parsePositiveDurationEnv(prefix+"_REST_UI_STATE_TTL", cfg.stateTTL); err != nil {
		return cfg, err
	}
	if cfg.blockDuration, err = parseNonNegativeDurationEnv(prefix+"_REST_UI_BLOCK_DURATION", cfg.blockDuration); err != nil {
		return cfg, err
	}

	clientIPCfg, err := readClientIPConfig(network)
	if err != nil {
		return cfg, err
	}
	cfg.trustedProxies = clientIPCfg.trustedProxies
	cfg.cloudflareCIDRs = clientIPCfg.cloudflarePrefixes
	cfg.trustPseudoIPv6 = clientIPCfg.trustPseudoIPv6
	return cfg, nil
}

func parseNonNegativeIntEnv(envName string, defaultValue int) (int, error) {
	v := os.Getenv(envName)
	if v == "" {
		return defaultValue, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%s: invalid value %q (want a non-negative integer)", envName, v)
	}
	return n, nil
}

func parsePositiveDurationEnv(envName string, defaultValue time.Duration) (time.Duration, error) {
	v := os.Getenv(envName)
	if v == "" {
		return defaultValue, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s: invalid duration %q (e.g. \"1m\")", envName, v)
	}
	return d, nil
}

func parseNonNegativeDurationEnv(envName string, defaultValue time.Duration) (time.Duration, error) {
	v := os.Getenv(envName)
	if v == "" {
		return defaultValue, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("%s: invalid duration %q (e.g. \"10m\", or \"0\" to disable)", envName, v)
	}
	return d, nil
}

func (l *restUIRateLimiter) wrapPublic(next http.Handler, basePath string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isRateLimitedRoute(r.URL.Path, basePath) {
			next.ServeHTTP(w, r)
			return
		}
		ip, blockSafe, fromHeader := resolveClientIP(r, l.trustedProxies, l.cloudflarePrefixes, l.trustPseudoIPv6)
		if !fromHeader && isLocalOrTrustedProxyIP(ip, l.trustedProxies) {
			// Request came straight from the operator's own loopback/LAN/trusted proxy
			// with no client-attribution header: the key would be a shared infrastructure
			// address, so limiting it would throttle the whole deployment as one client.
			l.localBypassWarn.Do(func() {
				glog.Info("REST/UI request from local/trusted peer ", ip,
					" without a client attribution header; such requests are not rate limited")
			})
			next.ServeHTTP(w, r)
			return
		}
		ipKey := rateLimitKey(ip)
		// blockKey keeps IPv6 at the full /128 so a temporary block never takes
		// out a whole shared /64 (rate limiting still aggregates to /64 via
		// ipKey). For IPv4 the two keys are identical.
		bKey := ""
		blockable := false
		if l.blockDuration > 0 {
			bKey = blockKey(ip)
			blockable = blockSafe && isBlockableKey(ip, l.trustedProxies, l.cloudflarePrefixes)
		}
		decision := l.accept(ipKey, bKey, blockable, time.Now())
		if !decision.accepted {
			l.observeRejection(decision.reason)
			if decision.shouldLog {
				glog.Warning("REST/UI request rejected, ", ipKey, ", ", decision.reason)
			}
			writeRestUIRateLimitResponse(w, decision.retryAfter)
			return
		}
		if !decision.untracked {
			// Wrap the release so time.Now() is evaluated when the handler
			// finishes, not when the defer is registered.
			defer func() { l.release(ipKey, time.Now()) }()
		}
		next.ServeHTTP(w, r)
	})
}

func isRateLimitedRoute(reqPath, basePath string) bool {
	if !strings.HasPrefix(reqPath, basePath) {
		return false
	}
	// Routes are registered by raw concatenation (basePath+"address/" etc.), so rel
	// is the registered suffix. splitBinding does not guarantee basePath ends in "/"
	// (a "-public=:port/path" binding without a trailing slash yields basePath
	// "/path"), yet static assets are still registered via publicPath at
	// "/path/favicon.ico"; trim any leading slash so the deny-list matches the
	// registered suffix regardless of the binding's trailing-slash shape.
	rel := strings.TrimPrefix(strings.TrimPrefix(reqPath, basePath), "/")
	switch {
	case rel == "favicon.ico",
		rel == "openapi.yaml",
		rel == "test-websocket.html",
		rel == "websocket",
		rel == "api-docs",
		strings.HasPrefix(rel, "static/"),
		strings.HasPrefix(rel, "api-docs/"):
		return false
	}
	return true
}

func (l *restUIRateLimiter) accept(ipKey, blockKey string, blockable bool, now time.Time) restUILimitDecision {
	l.mux.Lock()
	defer l.mux.Unlock()

	l.cleanupLocked(now)

	// Block check first, keyed on the (narrower) block key so a per-/128 block is
	// enforced without consulting the /64 rate-limit entry. The block entry is
	// created only when a breach trips the block (recordBreachLocked), so for a
	// client that is not blocked this is a plain lookup that never grows the map.
	if bc := l.clients[blockKey]; bc != nil && bc.blockedUntil.After(now) {
		bc.lastSeen = now
		bc.blockRejected++
		return restUILimitDecision{
			reason:     restUIRejectIPBlocked,
			retryAfter: bc.blockedUntil.Sub(now),
			shouldLog:  bc.shouldLogRejection(restUIRejectIPBlocked, now),
		}
	}

	client := l.clients[ipKey]
	if client == nil {
		if len(l.clients) >= restUIMaxTrackedClients {
			l.sweepLocked(now)
		}
		if len(l.clients) >= restUIMaxTrackedClients {
			if !l.capWarned {
				l.capWarned = true
				glog.Warning("REST/UI rate limiter is tracking ", restUIMaxTrackedClients,
					" client keys; admitting new keys unlimited until the map shrinks")
			}
			return restUILimitDecision{accepted: true, untracked: true}
		}
		client = &restUIClientLimit{}
		l.clients[ipKey] = client
	}
	client.lastSeen = now

	if l.maxConcurrent > 0 && client.active >= l.maxConcurrent {
		l.recordBreachLocked(blockKey, blockable, now)
		return restUILimitDecision{
			reason:    restUIRejectConcurrentRequests,
			shouldLog: client.shouldLogRejection(restUIRejectConcurrentRequests, now),
		}
	}

	if l.rateLimit > 0 {
		ok, retryAfter := client.bucket.allow(now, l.rateLimit, l.rateWindow, l.burst)
		if !ok {
			l.recordBreachLocked(blockKey, blockable, now)
			return restUILimitDecision{
				reason:     restUIRejectRequestRate,
				retryAfter: retryAfter,
				shouldLog:  client.shouldLogRejection(restUIRejectRequestRate, now),
			}
		}
	}

	client.active++
	return restUILimitDecision{accepted: true}
}

func (l *restUIRateLimiter) release(ipKey string, now time.Time) {
	l.mux.Lock()
	defer l.mux.Unlock()

	client := l.clients[ipKey]
	if client == nil {
		return
	}
	if client.active > 0 {
		client.active--
	}
	client.lastSeen = now
	l.cleanupLocked(now)
}

func (l *restUIRateLimiter) recordBreachLocked(blockKey string, blockable bool, now time.Time) {
	if l.blockDuration <= 0 || !blockable {
		return
	}
	// Breaches and the block accrue on the block key (full /128 for IPv6, == the
	// rate-limit key for IPv4) so one address cannot block a shared /64. Respect the
	// tracking cap so a flood rotating block keys cannot grow the map (fail open --
	// the /64 rate limiter still throttles them).
	client := l.clients[blockKey]
	if client == nil {
		if len(l.clients) >= restUIMaxTrackedClients {
			return
		}
		client = &restUIClientLimit{}
		l.clients[blockKey] = client
	}
	client.lastSeen = now
	cutoff := now.Add(-restUIBreachWindow)
	client.breaches = trimTimes(client.breaches, cutoff)
	// every rejection of an over-limit client lands here; only count one breach
	// per quiet gap so the block threshold means separate abuse episodes
	if n := len(client.breaches); n > 0 && now.Sub(client.breaches[n-1]) < restUIBreachMinSpacing {
		return
	}
	client.breaches = append(client.breaches, now)
	if len(client.breaches) >= restUIBreachBlockThreshold {
		client.blockedUntil = now.Add(l.blockDuration)
		client.blockRejected = 0
		client.breaches = client.breaches[:0]
	}
}

func (b *restUITokenBucket) allow(now time.Time, rateLimit int, rateWindow time.Duration, burst int) (bool, time.Duration) {
	if rateLimit <= 0 {
		return true, 0
	}
	if b.lastRefill.IsZero() {
		b.tokens = float64(burst)
		b.lastRefill = now
	}
	if now.Before(b.lastRefill) {
		now = b.lastRefill
	}
	ratePerSecond := float64(rateLimit) / rateWindow.Seconds()
	b.tokens = math.Min(float64(burst), b.tokens+now.Sub(b.lastRefill).Seconds()*ratePerSecond)
	b.lastRefill = now
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	if ratePerSecond <= 0 {
		return false, rateWindow
	}
	return false, time.Duration(math.Ceil((1 - b.tokens) / ratePerSecond * float64(time.Second)))
}

func trimTimes(times []time.Time, cutoff time.Time) []time.Time {
	i := 0
	for i < len(times) && times[i].Before(cutoff) {
		i++
	}
	if i > 0 {
		copy(times, times[i:])
		times = times[:len(times)-i]
	}
	return times
}

func (l *restUIRateLimiter) cleanupLocked(now time.Time) {
	if !l.lastCleanup.IsZero() && now.Sub(l.lastCleanup) < restUILimiterCleanupInterval {
		return
	}
	l.sweepLocked(now)
}

func (l *restUIRateLimiter) sweepLocked(now time.Time) {
	l.lastCleanup = now
	for ipKey, client := range l.clients {
		client.breaches = trimTimes(client.breaches, now.Add(-restUIBreachWindow))
		if client.active == 0 && !client.blockedUntil.After(now) && now.Sub(client.lastSeen) > l.stateTTL {
			delete(l.clients, ipKey)
		}
	}
	if l.capWarned && len(l.clients) < restUIMaxTrackedClients/2 {
		l.capWarned = false
	}
}

func (l *restUIRateLimiter) sweep(now time.Time) {
	l.mux.Lock()
	defer l.mux.Unlock()
	l.sweepLocked(now)
}

func (l *restUIRateLimiter) stats(now time.Time) (activeIPs int, maxActiveRequestsPerIP int, blockedIPs int) {
	l.mux.Lock()
	defer l.mux.Unlock()
	for _, client := range l.clients {
		if client.active > 0 {
			activeIPs++
			if client.active > maxActiveRequestsPerIP {
				maxActiveRequestsPerIP = client.active
			}
		}
		if client.blockedUntil.After(now) {
			blockedIPs++
		}
	}
	return activeIPs, maxActiveRequestsPerIP, blockedIPs
}

func (l *restUIRateLimiter) runMaintenance(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for now := range ticker.C {
		l.sweep(now)
		if l.metrics != nil {
			activeIPs, maxActive, blockedIPs := l.stats(now)
			l.metrics.RestUIActiveIPs.Set(float64(activeIPs))
			l.metrics.RestUIMaxActiveRequestsPerIP.Set(float64(maxActive))
			l.metrics.RestUIBlockedIPs.Set(float64(blockedIPs))
		}
	}
}

func (l *restUIRateLimiter) observeRejection(reason string) {
	if l.metrics != nil {
		l.metrics.RestUIRateLimitRejections.With(common.Labels{"reason": reason}).Inc()
	}
}

// shouldLogRejection throttles per-client rejection logging: log when the
// rejection kind changes or at most once a minute, plus the first and every
// 1000th rejection of a blocked client. Called with the limiter mutex held (it
// mutates client state); the caller emits the log line after unlocking.
func (client *restUIClientLimit) shouldLogRejection(reason string, now time.Time) bool {
	shouldLog := reason != client.lastRejectKind || now.Sub(client.lastRejectLog) >= time.Minute
	if reason == restUIRejectIPBlocked && (client.blockRejected == 1 || client.blockRejected%1000 == 0) {
		shouldLog = true
	}
	if shouldLog {
		client.lastRejectKind = reason
		client.lastRejectLog = now
	}
	return shouldLog
}

func writeRestUIRateLimitResponse(w http.ResponseWriter, retryAfter time.Duration) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Security-Policy", getContentSecurityPolicy())
	if retryAfter > 0 {
		seconds := int64(math.Ceil(retryAfter.Seconds()))
		if seconds < 1 {
			seconds = 1
		}
		w.Header().Set("Retry-After", strconv.FormatInt(seconds, 10))
	}
	w.WriteHeader(http.StatusTooManyRequests)
	if _, err := w.Write([]byte("{\"error\":\"rate limit exceeded\"}\n")); err != nil {
		glog.Warning("write REST/UI rate-limit response: ", err)
	}
}
