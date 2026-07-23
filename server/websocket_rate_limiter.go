package server

import (
	"errors"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/gorilla/websocket"
)

const maxWebsocketConnectionAttemptsPerIP = 64
const maxWebsocketConnectionsPerIP = 128
const websocketConnectionAttemptWindow = time.Minute
const websocketConnectionLimiterTTL = 10 * time.Minute
const websocketConnectionLimiterCleanupInterval = time.Minute

// Per-connection message rate limit defaults. A connection sending more than
// defaultWsMessageRateLimit messages within a trailing defaultWsMessageRateWindow is
// closed and its client key blocked for defaultWsIPBlockDuration. The 2500 / 10m
// default sits well above any Trezor Suite burst, so it only trips abusive traffic.
const defaultWsMessageRateLimit = 2500
const defaultWsMessageRateWindow = 10 * time.Minute
const defaultWsIPBlockDuration = 12 * time.Hour

// messageRateWindowBuckets is the number of sub-buckets the message rate window
// is divided into. It bounds per-connection memory (a fixed array, independent
// of the limit) and sets the sliding-window resolution: with a 10-minute window
// this is one 10-second bucket, so the count can over-shoot the true trailing
// window by at most one bucket's worth of traffic.
const messageRateWindowBuckets = 60

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

// configureMessageRateLimit reads the per-connection message-rate and IP-block
// config from the environment, applying defaults (see docs/env.md for the vars).
func (s *WebsocketServer) configureMessageRateLimit(network string) error {
	prefix := strings.ToUpper(network)
	var err error
	if s.messageRateLimit, err = parseNonNegativeIntEnv(prefix+"_WS_MESSAGE_RATE_LIMIT", defaultWsMessageRateLimit); err != nil {
		return err
	}
	if s.messageRateWindow, err = parsePositiveDurationEnv(prefix+"_WS_MESSAGE_RATE_WINDOW", defaultWsMessageRateWindow); err != nil {
		return err
	}
	if s.ipBlockDuration, err = parseNonNegativeDurationEnv(prefix+"_WS_IP_BLOCK_DURATION", defaultWsIPBlockDuration); err != nil {
		return err
	}
	s.ipBlockEnabled = s.messageRateLimit > 0 && s.ipBlockDuration > 0
	if s.messageRateLimit > 0 {
		glog.Infof("Websocket per-connection message rate limit: %d messages / %s; offending IP block: %s",
			s.messageRateLimit, s.messageRateWindow, s.ipBlockDuration)
		if s.ipBlockDuration > 0 && len(s.cloudflarePrefixes) == 0 {
			glog.Warning("Websocket IP block is enabled without Cloudflare peer verification; behind Cloudflare set <NET>_CLOUDFLARE_IPS=builtin so blocks key on the real client IP rather than being skipped for unverified forwarded headers")
		}
	} else {
		glog.Info("Websocket per-connection message rate limit disabled")
	}
	return nil
}

// configurePendingRequestsLimit reads the per-connection cap on concurrently
// executing ("pending") requests from the environment, applying the default
// (see docs/env.md); 0 disables the cap entirely.
func (s *WebsocketServer) configurePendingRequestsLimit(network string) error {
	prefix := strings.ToUpper(network)
	var err error
	if s.pendingRequestsLimit, err = parseNonNegativeIntEnv(prefix+"_WS_PENDING_REQUESTS_LIMIT", defaultWsPendingRequestsLimit); err != nil {
		return err
	}
	if s.pendingRequestsLimit > 0 {
		glog.Infof("Websocket per-connection pending request limit: %d", s.pendingRequestsLimit)
	} else {
		glog.Info("Websocket per-connection pending request limit disabled")
	}
	return nil
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
	client.attempts = trimTimes(client.attempts, now.Add(-websocketConnectionAttemptWindow))
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

// errWsMessageRateExceeded aborts the read loop from a control-frame handler;
// by the time it surfaces in inputLoop as a read error, onMessageRateBreach has
// already closed the channel (and blocked the key when blockable).
var errWsMessageRateExceeded = errors.New("websocket message rate limit exceeded")

// installControlFrameRateLimit counts client ping/pong control frames toward
// the per-connection message rate limit. gorilla consumes control frames inside
// ReadMessage and dispatches them to these handlers, so they never reach
// inputLoop's switch; without this a client could stream pings -- each costing
// the server a read plus a pong write -- without the limiter ever seeing them.
// The handlers run on the connection's read goroutine, the same one that
// observes text messages, so the counter stays single-goroutine. The ping
// handler otherwise mirrors gorilla's default: answer with a pong carrying the
// ping payload, swallowing closed-connection and write-timeout errors.
func (s *WebsocketServer) installControlFrameRateLimit(c *websocketChannel) {
	c.conn.SetPingHandler(func(message string) error {
		if count := c.messageRate.observe(time.Now()); count > s.messageRateLimit {
			s.onMessageRateBreach(c, count)
			return errWsMessageRateExceeded
		}
		err := c.conn.WriteControl(websocket.PongMessage, []byte(message), time.Now().Add(time.Second))
		if err == websocket.ErrCloseSent {
			return nil
		}
		if e, ok := err.(net.Error); ok && e.Timeout() {
			return nil
		}
		return err
	})
	c.conn.SetPongHandler(func(string) error {
		if count := c.messageRate.observe(time.Now()); count > s.messageRateLimit {
			s.onMessageRateBreach(c, count)
			return errWsMessageRateExceeded
		}
		return nil
	})
}

// onMessageRateBreach handles a connection that exceeded the per-connection
// message rate limit: it flags the client key for the configured block duration
// (when the attribution is trustworthy and blockable) and closes the connection.
// The connection is always closed; only the per-IP block is conditional.
func (s *WebsocketServer) onMessageRateBreach(c *websocketChannel, count int) {
	now := time.Now()
	blocked := false
	if s.ipBlockEnabled && c.blockable {
		s.is.BlockWsIP(c.blockKey, now.Add(s.ipBlockDuration), now)
		blocked = true
	}
	closed := s.closeChannel(c, "message_rate_limit")
	// The block takes effect regardless of which goroutine wins the close race,
	// so it is logged unconditionally; the close-only message is logged just by
	// the winner to avoid duplicates.
	if blocked {
		glog.Warning("Client ", c.id, " exceeded websocket message rate limit (", count, "/", s.messageRateLimit,
			"); blocking ", c.blockKey, " for ", s.ipBlockDuration)
	} else if closed {
		glog.Warning("Client ", c.id, " exceeded websocket message rate limit (", count, "/", s.messageRateLimit,
			"); closing connection (", c.ip, " not blockable)")
	}
}
