package server

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
)

const maxWebsocketConnectionAttemptsPerIP = 64
const maxWebsocketConnectionsPerIP = 128
const websocketConnectionAttemptWindow = time.Minute
const websocketConnectionLimiterTTL = 10 * time.Minute
const websocketConnectionLimiterCleanupInterval = time.Minute

// Per-connection message rate limit defaults. A single connection that sends
// more than defaultWsMessageRateLimit text messages within a trailing
// defaultWsMessageRateWindow is closed, and its client key is blocked from
// opening new connections for defaultWsIPBlockDuration. The default of 2500
// messages / 10 minutes is well above the maximum burst a Trezor Suite client
// produces, so it only trips clearly abusive (non-Suite) traffic. All three are
// overridable via env (see NewWebsocketServer).
const defaultWsMessageRateLimit = 2500
const defaultWsMessageRateWindow = 10 * time.Minute
const defaultWsIPBlockDuration = 12 * time.Hour

// messageRateWindowBuckets is the number of sub-buckets the message rate window
// is divided into. It bounds per-connection memory (a fixed array, independent
// of the limit) and sets the sliding-window resolution: with a 10-minute window
// this is one 10-second bucket, so the count can over-shoot the true trailing
// window by at most one bucket's worth of traffic.
const messageRateWindowBuckets = 60

// cloudflareEdgeCIDRs are Cloudflare's published edge ranges
// (https://www.cloudflare.com/ips/, fetched 2026-06). When Cloudflare peer
// verification is enabled (the default; see <NETWORK>_WS_CLOUDFLARE_IPS) the
// CF-Connecting-* headers are trusted only when the TCP peer falls inside one of
// these ranges (or is a loopback/private proxy fronting Cloudflare). Cloudflare
// changes these rarely; operators can override the list via env if it drifts.
var cloudflareEdgeCIDRs = []string{
	"173.245.48.0/20",
	"103.21.244.0/22",
	"103.22.200.0/22",
	"103.31.4.0/22",
	"141.101.64.0/18",
	"108.162.192.0/18",
	"190.93.240.0/20",
	"188.114.96.0/20",
	"197.234.240.0/22",
	"198.41.128.0/17",
	"162.158.0.0/15",
	"104.16.0.0/13",
	"104.24.0.0/14",
	"172.64.0.0/13",
	"131.0.72.0/22",
	"2400:cb00::/32",
	"2606:4700::/32",
	"2803:f800::/32",
	"2405:b500::/32",
	"2405:8100::/32",
	"2a06:98c0::/29",
	"2c0f:f248::/32",
}

type websocketClientLimit struct {
	active   int
	attempts []time.Time
	lastSeen time.Time
}

type websocketConnectionLimiter struct {
	mux         sync.Mutex
	clients     map[string]*websocketClientLimit
	lastCleanup time.Time
}

// configureMessageRateLimit reads the per-connection message rate limit and IP
// block configuration from the environment, applying defaults. Env vars (with
// <NET> = network or coin shortcut):
//
//	<NET>_WS_MESSAGE_RATE_LIMIT   max messages per window before a connection is
//	                              closed; default 2500, 0 disables the feature.
//	<NET>_WS_MESSAGE_RATE_WINDOW  trailing window as a Go duration (e.g. "10m");
//	                              default 10m.
//	<NET>_WS_IP_BLOCK_DURATION    how long an offending client key is blocked as a
//	                              Go duration (e.g. "12h"); default 12h, 0 closes
//	                              the connection without blocking the IP.
func (s *WebsocketServer) configureMessageRateLimit(network string) error {
	prefix := strings.ToUpper(network)
	s.messageRateLimit = defaultWsMessageRateLimit
	if v := os.Getenv(prefix + "_WS_MESSAGE_RATE_LIMIT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return fmt.Errorf("%s_WS_MESSAGE_RATE_LIMIT: invalid value %q (want a non-negative integer)", prefix, v)
		}
		s.messageRateLimit = n
	}
	s.messageRateWindow = defaultWsMessageRateWindow
	if v := os.Getenv(prefix + "_WS_MESSAGE_RATE_WINDOW"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return fmt.Errorf("%s_WS_MESSAGE_RATE_WINDOW: invalid duration %q (e.g. \"10m\")", prefix, v)
		}
		s.messageRateWindow = d
	}
	s.ipBlockDuration = defaultWsIPBlockDuration
	if v := os.Getenv(prefix + "_WS_IP_BLOCK_DURATION"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d < 0 {
			return fmt.Errorf("%s_WS_IP_BLOCK_DURATION: invalid duration %q (e.g. \"12h\", or \"0\" to disable blocking)", prefix, v)
		}
		s.ipBlockDuration = d
	}
	s.ipBlockEnabled = s.messageRateLimit > 0 && s.ipBlockDuration > 0
	if s.messageRateLimit > 0 {
		glog.Infof("Websocket per-connection message rate limit: %d messages / %s; offending IP block: %s",
			s.messageRateLimit, s.messageRateWindow, s.ipBlockDuration)
		if s.ipBlockDuration > 0 && len(s.cloudflarePrefixes) == 0 {
			glog.Warning("Websocket IP block is enabled without Cloudflare peer verification; behind Cloudflare set <NET>_WS_CLOUDFLARE_IPS=builtin so blocks key on the real client IP rather than being skipped for unverified forwarded headers")
		}
	} else {
		glog.Info("Websocket per-connection message rate limit disabled")
	}
	return nil
}

// parseTrustedProxies parses a comma-separated list of CIDRs that augment the
// loopback/RFC1918/link-local defaults for trusting X-Real-Ip. Any prefix
// broad enough to cover meaningful chunks of the public internet is rejected
// with an error so misconfiguration fails fast at startup rather than
// silently turning X-Real-Ip into an IP-spoofing primitive.
func parseTrustedProxies(envName, value string) ([]netip.Prefix, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	const minIPv4Bits = 8
	const minIPv6Bits = 16
	var prefixes []netip.Prefix
	for _, raw := range strings.Split(value, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		p, err := netip.ParsePrefix(raw)
		if err != nil {
			return nil, fmt.Errorf("%s: invalid CIDR %q: %w", envName, raw, err)
		}
		if p.Addr().Is4In6() {
			return nil, fmt.Errorf("%s: refusing IPv4-mapped CIDR %q; use IPv4 CIDR notation", envName, raw)
		}
		bits := p.Bits()
		if p.Addr().Is4() && bits < minIPv4Bits {
			return nil, fmt.Errorf("%s: refusing CIDR %q: prefix /%d is too broad (minimum /%d for IPv4)", envName, raw, bits, minIPv4Bits)
		}
		if p.Addr().Is6() && !p.Addr().Is4In6() && bits < minIPv6Bits {
			return nil, fmt.Errorf("%s: refusing CIDR %q: prefix /%d is too broad (minimum /%d for IPv6)", envName, raw, bits, minIPv6Bits)
		}
		prefixes = append(prefixes, p.Masked())
	}
	return prefixes, nil
}

// parseCloudflareProxies parses the <NET>_WS_CLOUDFLARE_IPS env value used to
// gate trust of the CF-Connecting-* headers. Recognized values:
//
//	""            (unset)  -> built-in Cloudflare edge ranges (verification on)
//	"builtin"              -> built-in Cloudflare edge ranges (verification on)
//	"off" / "none" / "0"   -> disabled; CF headers are trusted from any peer
//	                          (legacy behavior, intended for an origin firewalled
//	                          to Cloudflare ranges out of band)
//	"<cidr>,<cidr>,..."    -> use these CIDRs instead of the built-in list
//
// A non-empty result means verification is enabled and getIP trusts the CF
// headers only when the TCP peer is inside one of the prefixes (or a
// loopback/private proxy fronting Cloudflare). Returning nil disables it; only
// the explicit "off" spellings do that -- a custom value that parses to no CIDRs
// is rejected so a typo cannot silently disable verification.
func parseCloudflareProxies(envName, value string) ([]netip.Prefix, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "builtin", "default":
		return parseCIDRList(envName, cloudflareEdgeCIDRs)
	case "off", "none", "false", "0", "disabled":
		return nil, nil
	default:
		prefixes, err := parseCIDRList(envName, strings.Split(value, ","))
		if err != nil {
			return nil, err
		}
		if len(prefixes) == 0 {
			return nil, fmt.Errorf("%s: no CIDRs in %q; use \"builtin\", \"off\", or a comma-separated CIDR list", envName, value)
		}
		return prefixes, nil
	}
}

// parseCIDRList parses CIDRs into masked prefixes, skipping blanks and rejecting
// IPv4-mapped notation, mirroring parseTrustedProxies' validation (minus the
// minimum-width check, since Cloudflare's published ranges are intentionally
// wide and the resulting set is only ever matched against the TCP peer).
func parseCIDRList(envName string, raws []string) ([]netip.Prefix, error) {
	var prefixes []netip.Prefix
	for _, raw := range raws {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		p, err := netip.ParsePrefix(raw)
		if err != nil {
			return nil, fmt.Errorf("%s: invalid CIDR %q: %w", envName, raw, err)
		}
		if p.Addr().Is4In6() {
			return nil, fmt.Errorf("%s: refusing IPv4-mapped CIDR %q; use IPv4 CIDR notation", envName, raw)
		}
		prefixes = append(prefixes, p.Masked())
	}
	return prefixes, nil
}

func newWebsocketConnectionLimiter() *websocketConnectionLimiter {
	return &websocketConnectionLimiter{
		clients: make(map[string]*websocketClientLimit),
	}
}

func (l *websocketConnectionLimiter) accept(ip string, now time.Time) (bool, string) {
	l.mux.Lock()
	defer l.mux.Unlock()

	l.cleanupLocked(now)
	client := l.clients[ip]
	if client == nil {
		client = &websocketClientLimit{}
		l.clients[ip] = client
	}
	client.lastSeen = now
	client.trimAttempts(now)

	if client.active >= maxWebsocketConnectionsPerIP {
		return false, "connection_limit"
	}
	if len(client.attempts) >= maxWebsocketConnectionAttemptsPerIP {
		return false, "connection_attempt_limit"
	}

	client.attempts = append(client.attempts, now)
	client.active++
	return true, ""
}

func (l *websocketConnectionLimiter) release(ip string, now time.Time) {
	l.mux.Lock()
	defer l.mux.Unlock()

	client := l.clients[ip]
	if client == nil {
		return
	}
	if client.active > 0 {
		client.active--
	}
	client.lastSeen = now
	l.cleanupLocked(now)
}

func (l *websocketConnectionLimiter) cleanupLocked(now time.Time) {
	if !l.lastCleanup.IsZero() && now.Sub(l.lastCleanup) < websocketConnectionLimiterCleanupInterval {
		return
	}
	l.sweepLocked(now)
}

func (l *websocketConnectionLimiter) sweepLocked(now time.Time) {
	l.lastCleanup = now
	for ip, client := range l.clients {
		client.trimAttempts(now)
		if client.active == 0 && now.Sub(client.lastSeen) > websocketConnectionLimiterTTL {
			delete(l.clients, ip)
		}
	}
}

// sweep evicts TTL-expired idle entries unconditionally. Used by the
// background ticker so that idle servers don't retain stale entries.
func (l *websocketConnectionLimiter) sweep(now time.Time) {
	l.mux.Lock()
	defer l.mux.Unlock()
	l.sweepLocked(now)
}

// stats returns the number of distinct client IPs that currently hold at least
// one active websocket connection and the largest per-IP connection count. The
// snapshot is taken under the limiter lock; idle entries retained for the TTL
// window are skipped so the numbers track live connections, not recent history.
func (l *websocketConnectionLimiter) stats() (uniqueActiveIPs int, maxConnectionsPerIP int) {
	l.mux.Lock()
	defer l.mux.Unlock()
	for _, client := range l.clients {
		if client.active <= 0 {
			continue
		}
		uniqueActiveIPs++
		if client.active > maxConnectionsPerIP {
			maxConnectionsPerIP = client.active
		}
	}
	return uniqueActiveIPs, maxConnectionsPerIP
}

// runWebsocketLimiterMaintenance ticks every interval to sweep TTL-expired
// entries from the connection limiter and to publish the per-IP clustering
// gauges. It does not terminate; it is started once per WebsocketServer at
// construction time and runs for the lifetime of the process.
func (s *WebsocketServer) runWebsocketLimiterMaintenance(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for now := range ticker.C {
		s.websocketLimiter.sweep(now)
		blockedIPs := 0
		if s.ipBlockEnabled {
			blockedIPs = s.is.SweepWsBlockedIPs(now)
		}
		if s.metrics != nil {
			uniqueIPs, maxConnectionsPerIP := s.websocketLimiter.stats()
			s.metrics.WebsocketUniqueIPs.Set(float64(uniqueIPs))
			s.metrics.WebsocketMaxConnectionsPerIP.Set(float64(maxConnectionsPerIP))
			s.metrics.WebsocketBlockedIPs.Set(float64(blockedIPs))
		}
	}
}

func (client *websocketClientLimit) trimAttempts(now time.Time) {
	cutoff := now.Add(-websocketConnectionAttemptWindow)
	i := 0
	for i < len(client.attempts) && client.attempts[i].Before(cutoff) {
		i++
	}
	if i > 0 {
		copy(client.attempts, client.attempts[i:])
		client.attempts = client.attempts[:len(client.attempts)-i]
	}
}

// resolveClientIP returns the per-IP rate-limit address for the request and
// whether that attribution is trustworthy enough to add to the IP blocklist
// (blockSafe). trustedProxies governs X-Real-Ip; cloudflareProxies governs
// CF-Connecting-* (empty disables verification and trusts those headers from any
// peer, the legacy behavior). When neither header is trusted for this peer it
// falls back to the bare TCP peer address.
//
// blockSafe centralizes the spoof-protection decision so callers never have to
// re-inspect headers: a CF-Connecting-* value is block-safe only when peer
// verification is enabled (otherwise it is forgeable); X-Real-Ip is block-safe
// because it is only honored from a verified trusted proxy; the bare TCP peer is
// block-safe unless the request also carried a CF-Connecting-* header we did not
// trust (a spoof attempt, or a real but unrecognized Cloudflare edge -- blocking
// the peer would be wrong in both cases).
func resolveClientIP(r *http.Request, trustedProxies, cloudflareProxies []netip.Prefix) (string, bool) {
	host := r.RemoteAddr
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		host = h
	}
	remote, remoteOK := parseAddr(host)

	// Default Cloudflare mode (no configured trusted proxies). Trust the
	// CF-Connecting-* headers either from any peer (verification disabled) or
	// only when the TCP peer is a published Cloudflare edge range or a
	// loopback/private proxy fronting Cloudflare (verification enabled). For a
	// direct public non-Cloudflare peer the headers are attacker-controlled and
	// are ignored so they cannot spoof a client IP past the limiter or blocklist.
	if len(trustedProxies) == 0 {
		cfTrusted := len(cloudflareProxies) == 0 || (remoteOK && isTrustedProxy(remote, cloudflareProxies))
		if cfTrusted {
			cfBlockSafe := len(cloudflareProxies) > 0
			if ip, ok := parseIP(r.Header.Get("CF-Connecting-IPv6")); ok {
				return ip, cfBlockSafe
			}
			if ip, ok := parseIP(r.Header.Get("CF-Connecting-IP")); ok {
				return ip, cfBlockSafe
			}
		}
	}

	// Trust X-Real-Ip only when the TCP peer is on a private/loopback network
	// (an upstream proxy on the same host or LAN) or in a configured trusted
	// CIDR. For direct internet peers the header is attacker-controlled and
	// would let any client spoof their IP past the per-IP rate limiter.
	if remoteOK && isTrustedProxy(remote, trustedProxies) {
		if ip, ok := parseIP(r.Header.Get("X-Real-Ip")); ok {
			return ip, true
		}
	}

	hadCFHeader := r.Header.Get("CF-Connecting-IP") != "" || r.Header.Get("CF-Connecting-IPv6") != ""
	if remoteOK {
		return remote.String(), !hadCFHeader
	}
	return strings.TrimSpace(r.RemoteAddr), !hadCFHeader
}

// rateLimitKey returns the key used for per-IP connection limiting and for the
// IP blocklist. IPv6 is aggregated to its /64 because a single client is
// routinely delegated a whole /64, so keying on the full /128 would let it
// evade limits (and outlive a block) by rotating the low 64 bits across genuine
// addresses. IPv4 is keyed verbatim (IPv4-mapped IPv6 is unmapped to its IPv4
// form first, so both notations share a key); anything unparseable is keyed
// verbatim.
func rateLimitKey(ip string) string {
	addr, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil {
		return ip
	}
	addr = addr.Unmap().WithZone("")
	if addr.Is6() {
		if p, err := addr.Prefix(64); err == nil {
			return p.String()
		}
	}
	return addr.String()
}

// isBlockableKey reports whether ip is safe to add to the IP blocklist. It
// refuses loopback/private/link-local addresses and any configured trusted-proxy
// or Cloudflare edge range, so a misconfiguration that collapses many clients
// onto a shared proxy/edge address (or the proxy itself) can never get that
// shared address -- and therefore every client behind it -- blocked.
func isBlockableKey(ip string, trustedProxies, cloudflareProxies []netip.Prefix) bool {
	addr, ok := parseAddr(ip)
	if !ok {
		return false
	}
	if addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() || addr.IsUnspecified() || addr.IsMulticast() {
		return false
	}
	for _, p := range trustedProxies {
		if p.Contains(addr) {
			return false
		}
	}
	for _, p := range cloudflareProxies {
		if p.Contains(addr) {
			return false
		}
	}
	return true
}

// connMessageRate is a fixed-memory, bucketed sliding-window message counter
// owned by a single websocket connection. It approximates the number of
// messages seen in the trailing window by summing messageRateWindowBuckets
// sub-buckets; the running count can over-shoot the true window by at most one
// bucket's worth of traffic. It is NOT safe for concurrent use: only the
// connection's inputLoop goroutine calls observe().
type connMessageRate struct {
	bucketDur  time.Duration
	counts     [messageRateWindowBuckets]int32
	total      int32
	lastBucket int64 // absolute index of the most recently touched bucket
	inited     bool
}

func newConnMessageRate(window time.Duration) *connMessageRate {
	bucketDur := window / messageRateWindowBuckets
	if bucketDur <= 0 {
		bucketDur = 1
	}
	return &connMessageRate{bucketDur: bucketDur}
}

// observe records one message at time now and returns the approximate number of
// messages seen in the trailing window, including this one.
func (m *connMessageRate) observe(now time.Time) int {
	idx := now.UnixNano() / int64(m.bucketDur)
	if !m.inited {
		m.inited = true
		m.lastBucket = idx
	}
	if idx < m.lastBucket {
		// Clock moved backwards; fold into the current bucket rather than
		// rewriting history.
		idx = m.lastBucket
	}
	if advance := idx - m.lastBucket; advance > 0 {
		// Zero the buckets we are advancing into; they hold stale counts from a
		// previous window. Capping at the ring size clears the whole window.
		if advance > messageRateWindowBuckets {
			advance = messageRateWindowBuckets
		}
		for i := int64(1); i <= advance; i++ {
			slot := (m.lastBucket + i) % messageRateWindowBuckets
			m.total -= m.counts[slot]
			m.counts[slot] = 0
		}
		m.lastBucket = idx
	}
	slot := idx % messageRateWindowBuckets
	m.counts[slot]++
	m.total++
	return int(m.total)
}

func parseIP(value string) (string, bool) {
	addr, ok := parseAddr(value)
	if !ok {
		return "", false
	}
	return addr.String(), true
}

func parseAddr(value string) (netip.Addr, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return netip.Addr{}, false
	}
	addr, err := netip.ParseAddr(value)
	if err != nil {
		return netip.Addr{}, false
	}
	// Unmap IPv4-mapped IPv6 (::ffff:a.b.c.d -> a.b.c.d) so both notations
	// share one rate-limit key and IPv4 prefixes match in isTrustedProxy and
	// isBlockableKey, and strip the IPv6 zone identifier so that rate-limit keys
	// are zone-free and netip.Prefix.Contains matches unzoned prefixes against
	// link-local peers.
	return addr.Unmap().WithZone(""), true
}

func isTrustedProxy(addr netip.Addr, extras []netip.Prefix) bool {
	if addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() {
		return true
	}
	for _, p := range extras {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// onMessageRateBreach handles a connection that exceeded the per-connection
// message rate limit: it flags the client key for the configured block duration
// (when the attribution is trustworthy and blockable) and closes the connection.
// The connection is always closed; only the per-IP block is conditional.
func (s *WebsocketServer) onMessageRateBreach(c *websocketChannel, count int) {
	now := time.Now()
	blocked := false
	if s.ipBlockEnabled && c.blockable {
		s.is.BlockWsIP(c.ipKey, now.Add(s.ipBlockDuration), now)
		blocked = true
	}
	closed := s.closeChannel(c, "message_rate_limit")
	// The block takes effect regardless of which goroutine wins the close race,
	// so it is logged unconditionally; the close-only message is logged just by
	// the winner to avoid duplicates.
	if blocked {
		glog.Warning("Client ", c.id, " exceeded websocket message rate limit (", count, "/", s.messageRateLimit,
			"); blocking ", c.ipKey, " for ", s.ipBlockDuration)
	} else if closed {
		glog.Warning("Client ", c.id, " exceeded websocket message rate limit (", count, "/", s.messageRateLimit,
			"); closing connection (", c.ip, " not blockable)")
	}
}
