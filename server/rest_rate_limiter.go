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
	defaultRestAPIRateLimit     = 600
	defaultRestAPIRateWindow    = time.Minute
	defaultRestAPIBurst         = 120
	defaultRestAPIMaxConcurrent = 24
	defaultRestAPIStateTTL      = 10 * time.Minute
	defaultRestAPIBlockDuration = 0

	restAPILimiterCleanupInterval = time.Minute
	restAPIBreachWindow           = 10 * time.Minute
	restAPIBreachBlockThreshold   = 3
	// restAPIBreachMinSpacing is the minimum quiet gap between counted breaches:
	// one burst produces many rejections in the same instant (e.g. a page firing
	// dozens of parallel fetches), and the block threshold should mean separate
	// abuse episodes, not one spike.
	restAPIBreachMinSpacing = 10 * time.Second
	// restAPIMaxTrackedClients bounds the per-key state map so a flood rotating
	// client keys cannot grow it (and the sweeps over it) without bound; past the
	// cap, new keys are temporarily admitted untracked (fail open) while
	// already-tracked keys stay limited.
	restAPIMaxTrackedClients = 100_000
)

const (
	restAPIRejectRequestRate        = "request_rate"
	restAPIRejectConcurrentRequests = "concurrent_requests"
	restAPIRejectIPBlocked          = "ip_blocked"
)

type restAPILimiterConfig struct {
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

type restAPIRateLimiter struct {
	mux                sync.Mutex
	clients            map[string]*restAPIClientLimit
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

type restAPIClientLimit struct {
	active         int
	bucket         restAPITokenBucket
	breaches       []time.Time
	blockedUntil   time.Time
	blockRejected  int
	lastSeen       time.Time
	lastRejectLog  time.Time
	lastRejectKind string
}

type restAPITokenBucket struct {
	tokens     float64
	lastRefill time.Time
}

type restAPILimitDecision struct {
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

func newRestAPIRateLimiter(network string, metrics *common.Metrics) (*restAPIRateLimiter, error) {
	cfg, err := readRestAPILimiterConfig(network)
	if err != nil {
		return nil, err
	}
	if cfg.rateLimit == 0 && cfg.maxConcurrent == 0 {
		glog.Info("REST API rate limiter disabled")
		return nil, nil
	}
	l := &restAPIRateLimiter{
		clients:            make(map[string]*restAPIClientLimit),
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
		metrics.RestAPIActiveIPs.Set(0)
		metrics.RestAPIMaxActiveRequestsPerIP.Set(0)
		metrics.RestAPIBlockedIPs.Set(0)
	}
	if cfg.rateLimit > 0 {
		glog.Infof("REST API rate limit: %d requests / %s; burst: %d", cfg.rateLimit, cfg.rateWindow, cfg.burst)
	} else {
		glog.Info("REST API request-rate limit disabled")
	}
	if cfg.maxConcurrent > 0 {
		glog.Infof("REST API per-client concurrency limit: %d active requests", cfg.maxConcurrent)
	} else {
		glog.Info("REST API per-client concurrency limit disabled")
	}
	if cfg.blockDuration > 0 {
		glog.Infof("REST API temporary IP block enabled after repeated breaches: %s", cfg.blockDuration)
		if len(cfg.cloudflareCIDRs) == 0 {
			glog.Warning("REST API temporary IP block is enabled without Cloudflare peer verification; CF-Connecting-* derived addresses are not blockable in this mode")
		}
	}
	go l.runMaintenance(restAPILimiterCleanupInterval)
	return l, nil
}

func readRestAPILimiterConfig(network string) (restAPILimiterConfig, error) {
	prefix := strings.ToUpper(network)
	cfg := restAPILimiterConfig{
		rateLimit:     defaultRestAPIRateLimit,
		rateWindow:    defaultRestAPIRateWindow,
		burst:         defaultRestAPIBurst,
		maxConcurrent: defaultRestAPIMaxConcurrent,
		stateTTL:      defaultRestAPIStateTTL,
		blockDuration: defaultRestAPIBlockDuration,
	}

	var err error
	if cfg.rateLimit, err = parseNonNegativeIntEnv(prefix+"_REST_RATE_LIMIT", cfg.rateLimit); err != nil {
		return cfg, err
	}
	if cfg.rateWindow, err = parsePositiveDurationEnv(prefix+"_REST_RATE_WINDOW", cfg.rateWindow); err != nil {
		return cfg, err
	}
	if cfg.burst, err = parseNonNegativeIntEnv(prefix+"_REST_BURST", cfg.burst); err != nil {
		return cfg, err
	}
	if cfg.rateLimit > 0 && cfg.burst <= 0 {
		return cfg, fmt.Errorf("%s_REST_BURST: invalid value %d (want a positive integer when REST request-rate limiting is enabled)", prefix, cfg.burst)
	}
	if cfg.maxConcurrent, err = parseNonNegativeIntEnv(prefix+"_REST_MAX_CONCURRENT", cfg.maxConcurrent); err != nil {
		return cfg, err
	}
	if cfg.stateTTL, err = parsePositiveDurationEnv(prefix+"_REST_STATE_TTL", cfg.stateTTL); err != nil {
		return cfg, err
	}
	if cfg.blockDuration, err = parseNonNegativeDurationEnv(prefix+"_REST_BLOCK_DURATION", cfg.blockDuration); err != nil {
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

func (l *restAPIRateLimiter) wrapAPI(next http.Handler, apiRoot string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isRestAPIRoute(r.URL.Path, apiRoot) {
			next.ServeHTTP(w, r)
			return
		}
		ip, blockSafe, fromHeader := resolveClientIP(r, l.trustedProxies, l.cloudflarePrefixes, l.trustPseudoIPv6)
		if !fromHeader && isLocalOrTrustedProxyIP(ip, l.trustedProxies) {
			// The request came straight from the operator's own
			// loopback/LAN/trusted proxy with no client-attribution header: the
			// key would not identify a client but the shared infrastructure
			// address (local tooling, or a proxy that does not set X-Real-Ip),
			// so limiting it would throttle a whole deployment as one client.
			// These were never limited before, keep them exempt.
			l.localBypassWarn.Do(func() {
				glog.Info("REST API request from local/trusted peer ", ip,
					" without a client attribution header; such requests are not rate limited")
			})
			next.ServeHTTP(w, r)
			return
		}
		ipKey := rateLimitKey(ip)
		blockable := false
		if l.blockDuration > 0 {
			blockable = blockSafe && isBlockableKey(ip, l.trustedProxies, l.cloudflarePrefixes)
		}
		decision := l.accept(ipKey, blockable, time.Now())
		if !decision.accepted {
			l.observeRejection(decision.reason)
			if decision.shouldLog {
				glog.Warning("REST API request rejected, ", ipKey, ", ", decision.reason)
			}
			writeRestAPIRateLimitResponse(w, decision.retryAfter)
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

func isRestAPIRoute(path, apiRoot string) bool {
	return path == apiRoot || strings.HasPrefix(path, apiRoot+"/")
}

func (l *restAPIRateLimiter) accept(ipKey string, blockable bool, now time.Time) restAPILimitDecision {
	l.mux.Lock()
	defer l.mux.Unlock()

	l.cleanupLocked(now)
	client := l.clients[ipKey]
	if client == nil {
		if len(l.clients) >= restAPIMaxTrackedClients {
			l.sweepLocked(now)
		}
		if len(l.clients) >= restAPIMaxTrackedClients {
			if !l.capWarned {
				l.capWarned = true
				glog.Warning("REST API rate limiter is tracking ", restAPIMaxTrackedClients,
					" client keys; admitting new keys unlimited until the map shrinks")
			}
			return restAPILimitDecision{accepted: true, untracked: true}
		}
		client = &restAPIClientLimit{}
		l.clients[ipKey] = client
	}
	client.lastSeen = now

	if client.blockedUntil.After(now) {
		client.blockRejected++
		return restAPILimitDecision{
			reason:     restAPIRejectIPBlocked,
			retryAfter: client.blockedUntil.Sub(now),
			shouldLog:  client.shouldLogRejection(restAPIRejectIPBlocked, now),
		}
	}

	if l.maxConcurrent > 0 && client.active >= l.maxConcurrent {
		l.recordBreachLocked(client, blockable, now)
		return restAPILimitDecision{
			reason:    restAPIRejectConcurrentRequests,
			shouldLog: client.shouldLogRejection(restAPIRejectConcurrentRequests, now),
		}
	}

	if l.rateLimit > 0 {
		ok, retryAfter := client.bucket.allow(now, l.rateLimit, l.rateWindow, l.burst)
		if !ok {
			l.recordBreachLocked(client, blockable, now)
			return restAPILimitDecision{
				reason:     restAPIRejectRequestRate,
				retryAfter: retryAfter,
				shouldLog:  client.shouldLogRejection(restAPIRejectRequestRate, now),
			}
		}
	}

	client.active++
	return restAPILimitDecision{accepted: true}
}

func (l *restAPIRateLimiter) release(ipKey string, now time.Time) {
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

func (l *restAPIRateLimiter) recordBreachLocked(client *restAPIClientLimit, blockable bool, now time.Time) {
	if l.blockDuration <= 0 || !blockable {
		return
	}
	cutoff := now.Add(-restAPIBreachWindow)
	client.breaches = trimTimes(client.breaches, cutoff)
	// every rejection of an over-limit client lands here; only count one breach
	// per quiet gap so the block threshold means separate abuse episodes
	if n := len(client.breaches); n > 0 && now.Sub(client.breaches[n-1]) < restAPIBreachMinSpacing {
		return
	}
	client.breaches = append(client.breaches, now)
	if len(client.breaches) >= restAPIBreachBlockThreshold {
		client.blockedUntil = now.Add(l.blockDuration)
		client.blockRejected = 0
		client.breaches = client.breaches[:0]
	}
}

func (b *restAPITokenBucket) allow(now time.Time, rateLimit int, rateWindow time.Duration, burst int) (bool, time.Duration) {
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

func (l *restAPIRateLimiter) cleanupLocked(now time.Time) {
	if !l.lastCleanup.IsZero() && now.Sub(l.lastCleanup) < restAPILimiterCleanupInterval {
		return
	}
	l.sweepLocked(now)
}

func (l *restAPIRateLimiter) sweepLocked(now time.Time) {
	l.lastCleanup = now
	for ipKey, client := range l.clients {
		client.breaches = trimTimes(client.breaches, now.Add(-restAPIBreachWindow))
		if client.active == 0 && !client.blockedUntil.After(now) && now.Sub(client.lastSeen) > l.stateTTL {
			delete(l.clients, ipKey)
		}
	}
	if l.capWarned && len(l.clients) < restAPIMaxTrackedClients/2 {
		l.capWarned = false
	}
}

func (l *restAPIRateLimiter) sweep(now time.Time) {
	l.mux.Lock()
	defer l.mux.Unlock()
	l.sweepLocked(now)
}

func (l *restAPIRateLimiter) stats(now time.Time) (activeIPs int, maxActiveRequestsPerIP int, blockedIPs int) {
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

func (l *restAPIRateLimiter) runMaintenance(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for now := range ticker.C {
		l.sweep(now)
		if l.metrics != nil {
			activeIPs, maxActive, blockedIPs := l.stats(now)
			l.metrics.RestAPIActiveIPs.Set(float64(activeIPs))
			l.metrics.RestAPIMaxActiveRequestsPerIP.Set(float64(maxActive))
			l.metrics.RestAPIBlockedIPs.Set(float64(blockedIPs))
		}
	}
}

func (l *restAPIRateLimiter) observeRejection(reason string) {
	if l.metrics != nil {
		l.metrics.RestAPIRateLimitRejections.With(common.Labels{"reason": reason}).Inc()
	}
}

// shouldLogRejection throttles per-client rejection logging: log when the
// rejection kind changes or at most once a minute, plus the first and every
// 1000th rejection of a blocked client. Called with the limiter mutex held (it
// mutates client state); the caller emits the log line after unlocking.
func (client *restAPIClientLimit) shouldLogRejection(reason string, now time.Time) bool {
	shouldLog := reason != client.lastRejectKind || now.Sub(client.lastRejectLog) >= time.Minute
	if reason == restAPIRejectIPBlocked && (client.blockRejected == 1 || client.blockRejected%1000 == 0) {
		shouldLog = true
	}
	if shouldLog {
		client.lastRejectKind = reason
		client.lastRejectLog = now
	}
	return shouldLog
}

func writeRestAPIRateLimitResponse(w http.ResponseWriter, retryAfter time.Duration) {
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
		glog.Warning("write REST API rate-limit response: ", err)
	}
}
