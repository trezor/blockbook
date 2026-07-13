package server

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/glog"
	"github.com/gorilla/websocket"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/api"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/db"
	"github.com/trezor/blockbook/fiat"
)

const upgradeFailed = "Upgrade failed: "
const outChannelSize = 500
const defaultTimeout = 60 * time.Second
const unknownMethodLabel = "unknown"
const maxWebsocketMessageBytes int64 = 4 * 1024 * 1024
// defaultWsPendingRequestsLimit is the default per-connection cap on
// concurrently executing requests; override with
// <network>_WS_PENDING_REQUESTS_LIMIT (0 disables), see docs/env.md.
const defaultWsPendingRequestsLimit = 48

// maxWebsocketMempoolFiltersResponses caps per-connection getMempoolFilters
// requests over their whole lifecycle: the slot is acquired before the handler
// computes the (potentially large) response and released only after the
// response is written to the websocket (or drained on close). The point is to
// bound the peak memory held in computed-but-unwritten filter responses, so the
// cap deliberately covers compute, queueing, and write together; a client that
// pipelines more than this many requests, or reads slower than it requests,
// gets a mempool_filters_limit error instead of queueing further responses.
const maxWebsocketMempoolFiltersResponses = 4
const maxWebsocketActiveRequests = 2048
const maxWebsocketEstimateFeeBlocks = 32
const maxWebsocketSubscribeAddresses = 1000
const maxWebsocketSubscribeAddressesWithNewBlockTxs = 100
const maxWebsocketSubscribeFiatRatesTokens = 1000
const websocketLogPreviewBytes = 256

// allRates is a special "currency" parameter that means all available currencies
const allFiatRates = "!ALL!"

var (
	// ErrorMethodNotAllowed is returned when client tries to upgrade method other than GET
	ErrorMethodNotAllowed = errors.New("Method not allowed")

	connectionCounter uint64
)

// websocketChannel is a single client connection. ipKey is the per-IP rate-limit
// key (IPv6 aggregated to /64); blockKey is the IP-blocklist key (full /128) kept
// narrower so a hard block does not take out a whole shared /64; blockable records
// whether blockKey is safe to add to the blocklist; messageRate is the per-connection
// message counter (nil when disabled). All are touched only by ServeHTTP/inputLoop.
type websocketChannel struct {
	id                           uint64
	requests                     uint64 // total requests received on this connection, accessed atomically
	conn                         *websocket.Conn
	out                          chan *WsRes
	pendingRequests              chan struct{}
	mempoolFiltersSlots          chan struct{} // semaphore capping in-flight getMempoolFilters responses, see maxWebsocketMempoolFiltersResponses
	ip                           string
	ipKey                        string
	blockKey                     string
	blockable                    bool
	messageRate                  *connMessageRate
	requestHeader                http.Header
	alive                        bool
	aliveLock                    sync.Mutex
	closeReason                  string
	addrDescs                    []string // subscribed address descriptors as strings
	getAddressInfoDescriptorsMux sync.Mutex
	getAddressInfoDescriptors    map[string]struct{}
}

type addressDetails struct {
	requestID string
	// publishNewBlockTxs enables notifications for confirmed transactions
	// detected while processing newly connected blocks.
	publishNewBlockTxs bool
}

// WebsocketServer is a handle to websocket server
type WebsocketServer struct {
	upgrader                        *websocket.Upgrader
	db                              *db.RocksDB
	txCache                         *db.TxCache
	chain                           bchain.BlockChain
	chainParser                     bchain.BlockChainParser
	mempool                         bchain.Mempool
	metrics                         *common.Metrics
	is                              *common.InternalState
	api                             *api.Worker
	block0hash                      string
	newBlockSubscriptions           map[*websocketChannel]string
	newBlockSubscriptionsLock       sync.Mutex
	newTransactionEnabled           bool
	newTransactionSubscriptions     map[*websocketChannel]string
	newTransactionSubscriptionsLock sync.Mutex
	addressSubscriptions            map[string]map[*websocketChannel]*addressDetails
	addressSubscriptionsLock        sync.Mutex
	// newBlockTxsSubscriptionCount is a fast-path guard for OnNewBlock.
	// It tracks how many address subscriptions requested newBlockTxs=true.
	newBlockTxsSubscriptionCount int
	fiatRatesSubscriptions       map[string]map[*websocketChannel]string
	fiatRatesTokenSubscriptions  map[*websocketChannel][]string
	fiatRatesSubscriptionsLock   sync.Mutex
	allowedOrigins               map[string]struct{}
	trustedProxyPrefixes         []netip.Prefix
	// cloudflarePrefixes gates trust of the CF-Connecting-* headers: when
	// non-empty, those headers are honored only when the TCP peer is inside one
	// of these ranges (or a loopback/private proxy). Empty disables verification
	// and falls back to the legacy "trust CF headers from any peer" behavior.
	cloudflarePrefixes []netip.Prefix
	// trustPseudoIPv6 honors the (otherwise client-spoofable) CF-Connecting-IPv6
	// header; only safe with Cloudflare "Pseudo IPv4: Overwrite Headers" on.
	trustPseudoIPv6 bool
	// messageRateLimit / messageRateWindow bound how many messages a single
	// connection may send in a trailing window before it is closed; 0 disables.
	// ipBlockDuration is how long an offending client key is blocked from
	// opening new connections; 0 disables blocking (the connection is still
	// closed on breach).
	messageRateLimit  int
	messageRateWindow time.Duration
	ipBlockDuration   time.Duration
	// pendingRequestsLimit caps how many requests a single connection may have
	// executing concurrently before it is closed; 0 disables the cap.
	pendingRequestsLimit int
	// ipBlockEnabled gates the whole IP blocklist: true only when the rate limit can
	// produce breaches (messageRateLimit > 0) and blocking is configured
	// (ipBlockDuration > 0). When false, all block checks/sweeps are skipped.
	ipBlockEnabled   bool
	websocketLimiter *websocketConnectionLimiter
	// Shutdown coordination: protects shuttingDown + activeChannels and gates
	// trackWork so RocksDB cannot be closed while a WS goroutine is mid-read.
	shutdownMu     sync.Mutex
	shuttingDown   bool
	activeChannels map[*websocketChannel]struct{}
	activeRequests int
	requestWg      sync.WaitGroup
}

// NewWebsocketServer creates new websocket interface to blockbook and returns its handle
func NewWebsocketServer(db *db.RocksDB, chain bchain.BlockChain, mempool bchain.Mempool, txCache *db.TxCache, metrics *common.Metrics, is *common.InternalState, fiatRates *fiat.FiatRates) (*WebsocketServer, error) {
	api, err := api.NewWorker(db, chain, mempool, txCache, metrics, is, fiatRates)
	if err != nil {
		return nil, err
	}
	b0, err := db.GetBlockHash(0)
	if err != nil {
		return nil, err
	}
	s := &WebsocketServer{
		db:                          db,
		txCache:                     txCache,
		chain:                       chain,
		chainParser:                 chain.GetChainParser(),
		mempool:                     mempool,
		metrics:                     metrics,
		is:                          is,
		api:                         api,
		block0hash:                  b0,
		newBlockSubscriptions:       make(map[*websocketChannel]string),
		newTransactionEnabled:       is.EnableSubNewTx,
		newTransactionSubscriptions: make(map[*websocketChannel]string),
		addressSubscriptions:        make(map[string]map[*websocketChannel]*addressDetails),
		fiatRatesSubscriptions:      make(map[string]map[*websocketChannel]string),
		fiatRatesTokenSubscriptions: make(map[*websocketChannel][]string),
		websocketLimiter:            newWebsocketConnectionLimiter(),
		activeChannels:              make(map[*websocketChannel]struct{}),
	}
	s.upgrader = &websocket.Upgrader{
		ReadBufferSize:    1024 * 32,
		WriteBufferSize:   1024 * 32,
		WriteBufferPool:   &sync.Pool{},
		CheckOrigin:       s.checkOrigin,
		EnableCompression: true,
	}
	originEnvName := strings.ToUpper(is.GetNetwork()) + "_WS_ALLOWED_ORIGINS"
	s.allowedOrigins = parseAllowedOrigins(originEnvName, os.Getenv(originEnvName))
	if err := initRpcCallAllowlists(db, is); err != nil {
		return nil, err
	}
	clientIPCfg, err := readClientIPConfig(is.GetNetwork())
	if err != nil {
		return nil, err
	}
	s.trustedProxyPrefixes = clientIPCfg.trustedProxies
	if len(clientIPCfg.trustedProxies) > 0 {
		glog.Info("Trusted proxy CIDRs (", clientIPCfg.trustedEnvName, "): ", clientIPCfg.trustedProxies)
	}
	s.cloudflarePrefixes = clientIPCfg.cloudflarePrefixes
	if len(clientIPCfg.cloudflarePrefixes) > 0 {
		glog.Info("Cloudflare peer verification enabled for CF-Connecting-* headers (", clientIPCfg.cloudflareEnvName, "; ", len(clientIPCfg.cloudflarePrefixes), " CIDRs)")
	} else {
		glog.Warning("Cloudflare peer verification disabled (", clientIPCfg.cloudflareEnvName, "=off); CF-Connecting-* headers are trusted from any peer")
	}
	s.trustPseudoIPv6 = clientIPCfg.trustPseudoIPv6
	if clientIPCfg.trustPseudoIPv6 {
		glog.Info("Cloudflare Pseudo-IPv4 mode enabled (", clientIPCfg.pseudoIPv6EnvName, "); CF-Connecting-IPv6 is honored as the client IP (requires Cloudflare \"Pseudo IPv4: Overwrite Headers\")")
	}
	if err := s.configureMessageRateLimit(is.GetNetwork()); err != nil {
		return nil, err
	}
	if err := s.configurePendingRequestsLimit(is.GetNetwork()); err != nil {
		return nil, err
	}
	if s.metrics != nil {
		s.metrics.WebsocketNewBlockTxsSubscriptions.Set(0)
		s.metrics.WebsocketUniqueIPs.Set(0)
		s.metrics.WebsocketMaxConnectionsPerIP.Set(0)
		s.metrics.WebsocketBlockedIPs.Set(0)
	}
	go s.runWebsocketLimiterMaintenance(websocketConnectionLimiterCleanupInterval)
	return s, nil
}

func parseAllowedOrigins(originEnvName, envAllowedOrigins string) map[string]struct{} {
	if envAllowedOrigins == "" {
		glog.Warning("Websocket origin allowlist not configured (", originEnvName, "); all origins allowed")
		return nil
	}
	allowedOrigins := make(map[string]struct{})
	for _, origin := range strings.Split(envAllowedOrigins, ",") {
		origin = strings.TrimSpace(origin)
		if origin == "" {
			continue
		}
		normalizedOrigin, ok := normalizeOrigin(origin)
		if !ok {
			glog.Warning("Ignoring invalid websocket origin in ", originEnvName, ": ", origin)
			continue
		}
		allowedOrigins[normalizedOrigin] = struct{}{}
	}
	if len(allowedOrigins) == 0 {
		glog.Warning("Websocket origin allowlist is empty after parsing ", originEnvName, "; all origins allowed")
		return nil
	}
	glog.Info("Websocket origin allowlist enabled: ", envAllowedOrigins)
	return allowedOrigins
}

func (s *WebsocketServer) checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	if len(s.allowedOrigins) == 0 {
		return true
	}
	normalizedOrigin, ok := normalizeOrigin(origin)
	if !ok {
		return false
	}
	_, ok = s.allowedOrigins[normalizedOrigin]
	return ok
}

func normalizeOrigin(origin string) (string, bool) {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", false
	}
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host), true
}

func getWebsocketPayloadPreview(d []byte) string {
	if len(d) <= websocketLogPreviewBytes {
		return string(d)
	}
	return string(d[:websocketLogPreviewBytes]) + "...(truncated)"
}

// ServeHTTP sets up handler of websocket channel
func (s *WebsocketServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, upgradeFailed+ErrorMethodNotAllowed.Error(), http.StatusServiceUnavailable)
		return
	}
	s.shutdownMu.Lock()
	shuttingDown := s.shuttingDown
	s.shutdownMu.Unlock()
	if shuttingDown {
		http.Error(w, "Server shutting down", http.StatusServiceUnavailable)
		return
	}
	ip, blockSafe, _ := resolveClientIP(r, s.trustedProxyPrefixes, s.cloudflarePrefixes, s.trustPseudoIPv6)
	ipKey := rateLimitKey(ip)
	// blockKey/blockable are computed only when the IP blocklist is enabled (skips the
	// O(prefixes) isBlockableKey scan otherwise). blockKey keeps IPv6 at the full /128
	// so a block never takes out a shared /64 (ipKey still aggregates to /64).
	bKey := ""
	blockable := false
	if s.ipBlockEnabled {
		bKey = blockKey(ip)
		blockable = blockSafe && isBlockableKey(ip, s.trustedProxyPrefixes, s.cloudflarePrefixes)
	}

	// Reject keys that are on the temporary IP blocklist before doing any
	// upgrade work. Checked ahead of the connection limiter so a blocked client
	// cannot keep consuming attempt slots.
	if s.ipBlockEnabled {
		if blocked, rejected := s.is.IsWsIPBlocked(bKey, time.Now()); blocked {
			if s.metrics != nil {
				s.metrics.WebsocketBlockedConnections.Inc()
			}
			// A blocked client may hammer reconnects for the whole block
			// duration; log the first rejection and then every 1000th instead of
			// one line per attempt.
			if rejected == 1 || rejected%1000 == 0 {
				glog.Warning("Websocket connection rejected, ", ip, ", ip_blocked (attempt ", rejected, ")")
			}
			http.Error(w, "Too many websocket connections", http.StatusTooManyRequests)
			return
		}
	}

	limited := false
	if s.websocketLimiter != nil {
		ok, reason := s.websocketLimiter.accept(ipKey, time.Now())
		if !ok {
			if s.metrics != nil {
				s.metrics.WebsocketConnectionRejections.With(common.Labels{"reason": reason}).Inc()
			}
			glog.Warning("Websocket connection rejected, ", ip, ", ", reason)
			http.Error(w, "Too many websocket connections", http.StatusTooManyRequests)
			return
		}
		limited = true
	}
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		if limited {
			s.websocketLimiter.release(ipKey, time.Now())
		}
		http.Error(w, upgradeFailed+err.Error(), http.StatusServiceUnavailable)
		return
	}
	conn.SetReadLimit(maxWebsocketMessageBytes)
	// a nil channel disables the per-connection pending request cap
	var pendingRequests chan struct{}
	if s.pendingRequestsLimit > 0 {
		pendingRequests = make(chan struct{}, s.pendingRequestsLimit)
	}
	c := &websocketChannel{
		id:                  atomic.AddUint64(&connectionCounter, 1),
		conn:                conn,
		out:                 make(chan *WsRes, outChannelSize),
		pendingRequests:     pendingRequests,
		mempoolFiltersSlots: make(chan struct{}, maxWebsocketMempoolFiltersResponses),
		ip:                  ip,
		ipKey:               ipKey,
		blockKey:            bKey,
		blockable:           blockable,
		requestHeader:       r.Header,
		alive:               true,
	}
	if s.messageRateLimit > 0 {
		c.messageRate = newConnMessageRate(s.messageRateWindow)
		// count ping/pong control frames too; gorilla handles them inside
		// ReadMessage so they never reach inputLoop and would otherwise be a
		// free flood channel
		s.installControlFrameRateLimit(c)
	}
	if s.is.WsGetAccountInfoLimit > 0 {
		c.getAddressInfoDescriptors = make(map[string]struct{})
	}
	if !s.registerChannel(c) {
		conn.Close()
		if limited {
			s.websocketLimiter.release(ipKey, time.Now())
		}
		return
	}
	go s.inputLoop(c)
	go s.outputLoop(c)
	s.onConnect(c)
}

// GetHandler returns http handler
func (s *WebsocketServer) GetHandler() http.Handler {
	return s
}

// registerChannel adds channel to activeChannels unless the server is shutting
// down. Returns false on shutdown so the caller can close the connection.
func (s *WebsocketServer) registerChannel(c *websocketChannel) bool {
	s.shutdownMu.Lock()
	defer s.shutdownMu.Unlock()
	if s.shuttingDown {
		return false
	}
	s.activeChannels[c] = struct{}{}
	return true
}

func (s *WebsocketServer) unregisterChannel(c *websocketChannel) {
	s.shutdownMu.Lock()
	defer s.shutdownMu.Unlock()
	delete(s.activeChannels, c)
}

// trackWork increments requestWg unless the server is shutting down. Callers
// that get true must invoke workDone exactly once when the goroutine they
// spawn returns. Used to gate goroutines that touch the DB/chain/api so that
// Shutdown can wait for them to drain before RocksDB is closed.
func (s *WebsocketServer) trackWork() (bool, string) {
	s.shutdownMu.Lock()
	defer s.shutdownMu.Unlock()
	if s.shuttingDown {
		return false, "server_shutdown"
	}
	if s.activeRequests >= maxWebsocketActiveRequests {
		return false, "work_limit"
	}
	s.activeRequests++
	s.requestWg.Add(1)
	return true, ""
}

func (s *WebsocketServer) workDone() {
	s.shutdownMu.Lock()
	if s.activeRequests > 0 {
		s.activeRequests--
	}
	s.shutdownMu.Unlock()
	s.requestWg.Done()
}

// Shutdown initiates graceful WebSocket server shutdown: it refuses new
// connections, closes existing ones, and blocks until in-flight DB-touching
// goroutines finish or ctx is canceled. This must run before RocksDB is
// closed; otherwise a long-running getAccountInfo can race rocksdb_close in
// cgo and SIGSEGV the process.
func (s *WebsocketServer) Shutdown(ctx context.Context) error {
	s.shutdownMu.Lock()
	if s.shuttingDown {
		s.shutdownMu.Unlock()
		return nil
	}
	s.shuttingDown = true
	chans := make([]*websocketChannel, 0, len(s.activeChannels))
	for c := range s.activeChannels {
		chans = append(chans, c)
	}
	s.shutdownMu.Unlock()

	for _, c := range chans {
		s.closeChannel(c, "server_shutdown")
	}

	done := make(chan struct{})
	go func() {
		s.requestWg.Wait()
		close(done)
	}()
	select {
	case <-done:
		glog.Info("websocket: shutdown complete, all in-flight requests drained")
		return nil
	case <-ctx.Done():
		glog.Warning("websocket: shutdown timed out waiting for in-flight requests; waiting to avoid RocksDB close race")
		<-done
		glog.Info("websocket: shutdown complete after timeout")
		return ctx.Err()
	}
}

func (s *WebsocketServer) closeChannel(c *websocketChannel, reason string) bool {
	if closed, closeReason := c.CloseOut(reason); closed {
		if s.metrics != nil {
			s.metrics.WebsocketChannelCloses.With(common.Labels{"reason": closeReason}).Inc()
		}
		c.conn.Close()
		s.onDisconnect(c)
		return true
	}
	return false
}

func (c *websocketChannel) CloseOut(reason string) (bool, string) {
	c.aliveLock.Lock()
	defer c.aliveLock.Unlock()
	if c.alive {
		c.alive = false
		if c.closeReason == "" {
			c.closeReason = reason
		}
		closeReason := c.closeReason
		//clean out
		close(c.out)
		for len(c.out) > 0 {
			c.finalize(<-c.out)
		}
		return true, closeReason
	}
	return false, ""
}

func (c *websocketChannel) DataOut(data *WsRes) {
	c.aliveLock.Lock()
	defer c.aliveLock.Unlock()
	if c.alive {
		if len(c.out) < outChannelSize-1 {
			// Enqueued: ownership passes to the out pipeline, which finalizes it
			// once written (outputLoop) or drained (CloseOut).
			c.out <- data
			return
		}
		glog.Warning("Channel ", c.id, " overflow, closing")
		if c.closeReason == "" {
			c.closeReason = "overflow"
		}
		// close the connection but do not call CloseOut - would call duplicate c.aliveLock.Lock
		// CloseOut will be called because the closed connection will cause break in the inputLoop
		c.conn.Close()
	}
	// Not enqueued (overflow or dead connection): the response never reaches the
	// out pipeline, so release any slot it held here.
	c.finalize(data)
}

func (c *websocketChannel) acquireRequestSlot() bool {
	if c.pendingRequests == nil {
		// pending request limit disabled
		return true
	}
	select {
	case c.pendingRequests <- struct{}{}:
		return true
	default:
		return false
	}
}

func (c *websocketChannel) releaseRequestSlot() {
	if c.pendingRequests == nil {
		return
	}
	<-c.pendingRequests
}

func (c *websocketChannel) acquireMempoolFiltersSlot() bool {
	select {
	case c.mempoolFiltersSlots <- struct{}{}:
		return true
	default:
		return false
	}
}

func (c *websocketChannel) releaseMempoolFiltersSlot() {
	<-c.mempoolFiltersSlots
}

// finalize releases any resources a response held, exactly once
func (c *websocketChannel) finalize(res *WsRes) {
	if res != nil && res.release != nil {
		res.release()
		res.release = nil
	}
}

func (s *WebsocketServer) inputLoop(c *websocketChannel) {
	defer func() {
		if r := recover(); r != nil {
			glog.Error("recovered from panic: ", r, ", ", c.id)
			debug.PrintStack()
			s.closeChannel(c, "panic")
		}
	}()
	for {
		t, d, err := c.conn.ReadMessage()
		if err != nil {
			s.closeChannel(c, "read_error")
			return
		}
		switch t {
		case websocket.TextMessage:
			var req WsReq
			err := json.Unmarshal(d, &req)
			if err != nil {
				glog.Error("Error parsing message from ", c.id, ", len ", len(d), ", preview ", getWebsocketPayloadPreview(d), ", ", err)
				s.closeChannel(c, "protocol_error")
				return
			}
			atomic.AddUint64(&c.requests, 1)
			if c.messageRate != nil {
				// Breach on the message that pushes the trailing-window count past
				// the limit, so exactly messageRateLimit messages are allowed per
				// window (matches the "sends more than" contract).
				if count := c.messageRate.observe(time.Now()); count > s.messageRateLimit {
					s.onMessageRateBreach(c, count)
					return
				}
			}
			if !c.acquireRequestSlot() {
				glog.Warning("Client ", c.id, " exceeded pending websocket request limit, ", c.ip)
				s.closeChannel(c, "pending_requests_limit")
				return
			}
			if ok, reason := s.trackWork(); !ok {
				c.releaseRequestSlot()
				if reason == "work_limit" {
					e := resultError{}
					e.Error.Message = reason
					c.DataOut(&WsRes{
						ID:   req.ID,
						Data: e,
					})
					continue
				}
				s.closeChannel(c, reason)
				return
			}
			go func(req WsReq) {
				defer s.workDone()
				defer c.releaseRequestSlot()
				s.onRequest(c, &req)
			}(req)
		case websocket.BinaryMessage:
			glog.Error("Binary message received from ", c.id, ", ", c.ip)
			s.closeChannel(c, "protocol_error")
			return
		}
		// ReadMessage returns only data frames; ping/pong/close control frames
		// are consumed inside gorilla and dispatched to the connection's
		// handlers (see installControlFrameRateLimit), surfacing here only as a
		// read error.
	}
}

func (s *WebsocketServer) outputLoop(c *websocketChannel) {
	defer func() {
		if r := recover(); r != nil {
			glog.Error("recovered from panic: ", r, ", ", c.id)
			s.closeChannel(c, "panic")
		}
	}()
	for m := range c.out {
		c.conn.SetWriteDeadline(time.Now().Add(defaultTimeout))
		err := c.conn.WriteJSON(m)
		c.finalize(m)
		if err != nil {
			glog.Error("Error sending message to ", c.id, ", ", err)
			s.closeChannel(c, "write_error")
			return
		}
	}
}

func (s *WebsocketServer) onConnect(c *websocketChannel) {
	glog.Info("Client connected ", c.id, ", ", c.ip)
	s.metrics.WebsocketClients.Inc()
}

func (s *WebsocketServer) onDisconnect(c *websocketChannel) {
	s.unsubscribeNewBlock(c)
	s.unsubscribeNewTransaction(c)
	s.unsubscribeAddresses(c)
	s.unsubscribeFiatRates(c)
	if s.websocketLimiter != nil {
		s.websocketLimiter.release(c.ipKey, time.Now())
	}
	s.unregisterChannel(c)
	glog.Info("Client disconnected ", c.id, ", ", c.ip)
	s.metrics.WebsocketConnectionRequests.Observe(float64(atomic.LoadUint64(&c.requests)))
	s.metrics.WebsocketClients.Dec()
}

var requestHandlers = map[string]func(*WebsocketServer, *websocketChannel, *WsReq) (interface{}, error){
	"getAccountInfo": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		r, err := unmarshalGetAccountInfoRequest(req.Params)
		if err == nil {
			if s.is.WsGetAccountInfoLimit > 0 {
				c.getAddressInfoDescriptorsMux.Lock()
				c.getAddressInfoDescriptors[r.Descriptor] = struct{}{}
				l := len(c.getAddressInfoDescriptors)
				c.getAddressInfoDescriptorsMux.Unlock()
				if l > s.is.WsGetAccountInfoLimit {
					if s.closeChannel(c, "limit_exceeded") {
						glog.Info("Client ", c.id, " exceeded getAddressInfo limit, ", c.ip)
						s.is.AddWsLimitExceedingIP(c.ip)
					}
					return
				}
			}
			rv, err = s.getAccountInfo(r)
		}
		return
	},
	"getContractInfo": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		r := WsContractInfoReq{}
		err = json.Unmarshal(req.Params, &r)
		if err == nil {
			rv, err = s.getContractInfo(r.Contract, strings.ToLower(r.Currency), r.Protocols)
		}
		return
	},
	"getInfo": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		return s.getInfo()
	},
	"getBlockHash": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		r := WsBlockHashReq{}
		err = json.Unmarshal(req.Params, &r)
		if err == nil {
			rv, err = s.getBlockHash(r.Height)
		}
		return
	},
	"getBlock": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		if !s.is.ExtendedIndex {
			return nil, errors.New("Not supported")
		}
		r := WsBlockReq{}
		err = json.Unmarshal(req.Params, &r)
		if err == nil {
			r.Page, r.PageSize = sanitizePagingParams(r.Page, r.PageSize, txsInAPI, maxWebsocketBlockPageSize)
			rv, err = s.getBlock(r.Id, r.Page, r.PageSize)
		}
		return
	},
	"getAccountUtxo": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		r := WsAccountUtxoReq{}
		err = json.Unmarshal(req.Params, &r)
		if err == nil {
			rv, err = s.getAccountUtxo(r.Descriptor)
		}
		return
	},
	"getBalanceHistory": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		r := WsBalanceHistoryReq{}
		err = json.Unmarshal(req.Params, &r)
		if err == nil {
			if r.From <= 0 {
				r.From = 0
			}
			if r.To <= 0 {
				r.To = 0
			}
			if r.GroupBy <= 0 {
				r.GroupBy = 3600
			}
			rv, err = s.api.GetXpubBalanceHistory(r.Descriptor, r.From, r.To, r.Currencies, r.Gap, r.GroupBy, s.is.BalanceHistoryMaxTxsWS, api.BalanceHistoryTransportWS)
			if apiErr, ok := err.(*api.APIError); ok && apiErr.Public {
				// A public error from the xpub path (e.g. the range spans too many
				// transactions) is definitive for a valid xpub; do not retry as an
				// address, which would mask it with an address-parse error.
			} else if err != nil {
				rv, err = s.api.GetBalanceHistory(r.Descriptor, r.From, r.To, r.Currencies, r.GroupBy, s.is.BalanceHistoryMaxTxsWS, api.BalanceHistoryTransportWS)
			}
		}
		return
	},
	"getTransaction": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		r := WsTransactionReq{}
		err = json.Unmarshal(req.Params, &r)
		if err == nil {
			rv, err = s.getTransaction(r.Txid)
		}
		return
	},
	"getTransactionSpecific": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		r := WsTransactionSpecificReq{}
		err = json.Unmarshal(req.Params, &r)
		if err == nil {
			rv, err = s.getTransactionSpecific(r.Txid)
		}
		return
	},
	"estimateFee": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		return s.estimateFee(req.Params)
	},
	"longTermFeeRate": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		return s.longTermFeeRate()
	},
	"sendTransaction": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		r := WsSendTransactionReq{}
		err = json.Unmarshal(req.Params, &r)
		if err == nil {
			rv, err = s.sendTransaction(r.Hex, r.DisableAlternativeRPC)
		}
		return
	},

	"getMempoolFilters": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		r := WsMempoolFiltersReq{}
		err = json.Unmarshal(req.Params, &r)
		if err == nil {
			rv, err = s.getMempoolFilters(&r)
		}
		return
	},
	"getBlockFilter": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		r := WsBlockFilterReq{}
		err = json.Unmarshal(req.Params, &r)
		if err == nil {
			rv, err = s.getBlockFilter(&r)
		}
		return
	},
	"getBlockFiltersBatch": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		r := WsBlockFiltersBatchReq{}
		err = json.Unmarshal(req.Params, &r)
		if err == nil {
			rv, err = s.getBlockFiltersBatch(&r)
		}
		return
	},
	"rpcCall": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		r := WsRpcCallReq{}
		err = json.Unmarshal(req.Params, &r)
		if err == nil {
			rv, err = s.rpcCall(&r)
		}
		return
	},
	"subscribeNewBlock": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		return s.subscribeNewBlock(c, req)
	},
	"unsubscribeNewBlock": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		return s.unsubscribeNewBlock(c)
	},
	"subscribeNewTransaction": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		return s.subscribeNewTransaction(c, req)
	},
	"unsubscribeNewTransaction": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		return s.unsubscribeNewTransaction(c)
	},
	"subscribeAddresses": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		ad, nbtxs, err := s.unmarshalAddresses(req.Params)
		if err == nil {
			rv, err = s.subscribeAddresses(c, ad, nbtxs, req)
		}
		return
	},
	"unsubscribeAddresses": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		return s.unsubscribeAddresses(c)
	},
	"subscribeFiatRates": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		var r WsSubscribeFiatRatesReq
		err = json.Unmarshal(req.Params, &r)
		if err != nil {
			return nil, err
		}
		if len(r.Tokens) > maxWebsocketSubscribeFiatRatesTokens {
			return nil, api.NewAPIError("tokens max "+strconv.Itoa(maxWebsocketSubscribeFiatRatesTokens), true)
		}
		r.Currency = strings.ToLower(r.Currency)

		return s.subscribeFiatRates(c, &r, req)
	},
	"unsubscribeFiatRates": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		return s.unsubscribeFiatRates(c)
	},
	"ping": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		r := struct{}{}
		return r, nil
	},
	"getCurrentFiatRates": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		r := WsCurrentFiatRatesReq{}
		err = json.Unmarshal(req.Params, &r)
		if err == nil {
			rv, err = s.getCurrentFiatRates(r.Currencies, r.Token)
		}
		return
	},
	"getFiatRatesForTimestamps": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		r := WsFiatRatesForTimestampsReq{}
		err = json.Unmarshal(req.Params, &r)
		if err == nil {
			rv, err = s.getFiatRatesForTimestamps(r.Timestamps, r.Currencies, r.Token)
		}
		return
	},
	"getFiatRatesTickersList": func(s *WebsocketServer, c *websocketChannel, req *WsReq) (rv interface{}, err error) {
		r := WsFiatRatesTickersListReq{}
		err = json.Unmarshal(req.Params, &r)
		if err == nil {
			rv, err = s.getAvailableVsCurrencies(r.Timestamp, r.Token)
		}
		return
	},
}

func (s *WebsocketServer) onRequest(c *websocketChannel, req *WsReq) {
	var err error
	var data interface{}
	// release is non-nil while this request holds a rate-capped endpoint slot.
	var release func()
	f, ok := requestHandlers[req.Method]
	methodLabel := req.Method
	if !ok {
		methodLabel = unknownMethodLabel
	}
	defer func() {
		if r := recover(); r != nil {
			glog.Error("Client ", c.id, ", onRequest ", req.Method, " recovered from panic: ", r)
			debug.PrintStack()
			e := resultError{}
			e.Error.Message = "Internal error"
			data = e
		}
		// nil data means no response
		if data != nil {
			c.DataOut(&WsRes{
				ID:      req.ID,
				Data:    data,
				release: release,
			})
			release = nil // ownership handed to the response
		}
		if release != nil {
			release() // no response was produced — free the slot now
		}
		s.metrics.WebsocketPendingRequests.With(common.Labels{"method": methodLabel}).Dec()
	}()
	t := time.Now()
	s.metrics.WebsocketPendingRequests.With(common.Labels{"method": methodLabel}).Inc()
	defer func() {
		s.metrics.WebsocketReqDuration.With(common.Labels{"method": methodLabel}).Observe(float64(time.Since(t)) / 1e3) // in microseconds
	}()
	if ok {
		if req.Method == "getMempoolFilters" {
			if !c.acquireMempoolFiltersSlot() {
				e := resultError{}
				e.Error.Message = "mempool_filters_limit"
				data = e
				s.metrics.WebsocketRequests.With(common.Labels{"method": methodLabel, "status": "failure"}).Inc()
				glog.Warning("Client ", c.id, " exceeded getMempoolFilters response limit, ", c.ip)
				return
			}
			release = c.releaseMempoolFiltersSlot
		}
		data, err = f(s, c, req)
		if err == nil {
			glog.V(1).Info("Client ", c.id, " onRequest ", req.Method, " success")
			s.metrics.WebsocketRequests.With(common.Labels{"method": methodLabel, "status": "success"}).Inc()
		} else {
			if apiErr, ok := err.(*api.APIError); !ok || !apiErr.Public {
				glog.Error("Client ", c.id, " onMessage ", req.Method, ": ", errors.ErrorStack(err), ", data ", string(req.Params))
			}
			s.metrics.WebsocketRequests.With(common.Labels{"method": methodLabel, "status": "failure"}).Inc()
			e := resultError{}
			e.Error.Message = err.Error()
			data = e
		}
	} else {
		s.metrics.WebsocketUnknownMethods.With(common.Labels{"method": methodLabel}).Inc()
		s.metrics.WebsocketRequests.With(common.Labels{"method": methodLabel, "status": "failure"}).Inc()
		glog.V(1).Info("Client ", c.id, " onMessage ", req.Method, ": unknown method, data ", string(req.Params))
	}
}

func unmarshalGetAccountInfoRequest(params []byte) (*WsAccountInfoReq, error) {
	var r WsAccountInfoReq
	err := json.Unmarshal(params, &r)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *WebsocketServer) getAccountInfo(req *WsAccountInfoReq) (res *api.Address, err error) {
	if err := s.api.ValidateProtocolsForChain(req.Protocols); err != nil {
		return nil, err
	}
	var opt api.AccountDetails
	switch req.Details {
	case "tokens":
		opt = api.AccountDetailsTokens
	case "tokenBalances":
		opt = api.AccountDetailsTokenBalances
	case "txids":
		opt = api.AccountDetailsTxidHistory
	case "txslight":
		opt = api.AccountDetailsTxHistoryLight
	case "txs":
		opt = api.AccountDetailsTxHistory
	default:
		opt = api.AccountDetailsBasic
	}
	var tokensToReturn api.TokensToReturn
	switch req.Tokens {
	case "used":
		tokensToReturn = api.TokensToReturnUsed
	case "nonzero":
		tokensToReturn = api.TokensToReturnNonzeroBalance
	default:
		tokensToReturn = api.TokensToReturnDerived
	}
	filter := api.AddressFilter{
		FromHeight:         uint32(req.FromHeight),
		ToHeight:           uint32(req.ToHeight),
		Contract:           req.ContractFilter,
		Vout:               api.AddressFilterVoutOff,
		TokensToReturn:     tokensToReturn,
		Protocols:          req.Protocols,
		WithConfirmedNonce: req.ConfirmedNonce,
	}
	req.Page, req.PageSize = sanitizeAccountPagingParams(req.Page, req.PageSize, txsOnPage, txsInAPI)
	req.Gap = validateIntValue(req.Gap, 0, 0, maxGapValue)
	a, err := s.api.GetXpubAddress(req.Descriptor, req.Page, req.PageSize, opt, &filter, req.Gap, strings.ToLower(req.SecondaryCurrency))
	if err != nil {
		return s.api.GetAddress(req.Descriptor, req.Page, req.PageSize, opt, &filter, strings.ToLower(req.SecondaryCurrency))
	}
	return a, nil
}

func (s *WebsocketServer) getContractInfo(contract string, currency string, protocols []string) (*api.ContractInfoResult, error) {
	return s.api.GetContractInfoData(contract, currency, protocols)
}

func (s *WebsocketServer) getAccountUtxo(descriptor string) (api.Utxos, error) {
	utxo, err := s.api.GetXpubUtxo(descriptor, false, 0)
	if err != nil {
		return s.api.GetAddressUtxo(descriptor, false)
	}
	return utxo, nil
}

func (s *WebsocketServer) getTransaction(txid string) (*api.Tx, error) {
	return s.api.GetTransaction(txid, false, false)
}

func (s *WebsocketServer) getTransactionSpecific(txid string) (interface{}, error) {
	return s.chain.GetTransactionSpecific(&bchain.Tx{Txid: txid})
}

func (s *WebsocketServer) getInfo() (*WsInfoRes, error) {
	vi := common.GetVersionInfo()
	bi := s.is.GetBackendInfo()
	height, hash, err := s.db.GetBestBlock()
	if err != nil {
		return nil, err
	}
	return &WsInfoRes{
		Name:       s.is.Coin,
		Shortcut:   s.is.CoinShortcut,
		Network:    s.is.GetNetwork(),
		Decimals:   s.chainParser.AmountDecimals(),
		BestHeight: int(height),
		BestHash:   hash,
		Version:    vi.Version,
		Block0Hash: s.block0hash,
		Testnet:    s.chain.IsTestnet(),
		Backend: WsBackendInfo{
			Version:          bi.Version,
			Subversion:       bi.Subversion,
			ConsensusVersion: bi.ConsensusVersion,
			Consensus:        bi.Consensus,
		},
	}, nil
}

func (s *WebsocketServer) getBlockHash(height int) (*WsBlockHashRes, error) {
	h, err := s.db.GetBlockHash(uint32(height))
	if err != nil {
		return nil, err
	}
	return &WsBlockHashRes{
		Hash: h,
	}, nil
}

func (s *WebsocketServer) getBlock(id string, page, pageSize int) (interface{}, error) {
	block, err := s.api.GetBlock(id, page, pageSize)
	if err != nil {
		return nil, err
	}
	return block, nil
}

func eip1559FeesToApi(fee *bchain.Eip1559Fee) *api.Eip1559Fee {
	if fee == nil {
		return nil
	}
	apiFee := api.Eip1559Fee{}
	apiFee.MaxFeePerGas = (*api.Amount)(fee.MaxFeePerGas)
	apiFee.MaxPriorityFeePerGas = (*api.Amount)(fee.MaxPriorityFeePerGas)
	apiFee.MaxWaitTimeEstimate = fee.MaxWaitTimeEstimate
	apiFee.MinWaitTimeEstimate = fee.MinWaitTimeEstimate
	return &apiFee
}

func eip1559FeeRangeToApi(feeRange []*big.Int) []*api.Amount {
	if feeRange == nil {
		return nil
	}
	apiFeeRange := make([]*api.Amount, len(feeRange))
	for i := range feeRange {
		apiFeeRange[i] = (*api.Amount)(feeRange[i])
	}
	return apiFeeRange
}

func (s *WebsocketServer) estimateFee(params []byte) (interface{}, error) {
	var r WsEstimateFeeReq
	err := json.Unmarshal(params, &r)
	if err != nil {
		return nil, err
	}
	if len(r.Blocks) > maxWebsocketEstimateFeeBlocks {
		return nil, api.NewAPIError("blocks max "+strconv.Itoa(maxWebsocketEstimateFeeBlocks), true)
	}
	res := make([]WsEstimateFeeRes, len(r.Blocks))
	if s.chainParser.GetChainType() == bchain.ChainEthereumType {
		gas, err := s.chain.EthereumTypeEstimateGas(r.Specific)
		if err != nil {
			return nil, err
		}
		sg := strconv.FormatUint(gas, 10)
		b := 1
		if len(r.Blocks) > 0 {
			b = r.Blocks[0]
		}
		fee, err := s.api.EstimateFee(b, true)
		if err != nil {
			return nil, err
		}
		feePerTx := new(big.Int)
		feePerTx.Mul(&fee, new(big.Int).SetUint64(gas))
		eip1559, err := s.chain.EthereumTypeGetEip1559Fees()
		if err != nil {
			return nil, err
		}
		var eip1559Api *api.Eip1559Fees
		if eip1559 != nil {
			eip1559Api = &api.Eip1559Fees{}
			eip1559Api.BaseFeePerGas = (*api.Amount)(eip1559.BaseFeePerGas)
			eip1559Api.Instant = eip1559FeesToApi(eip1559.Instant)
			eip1559Api.High = eip1559FeesToApi(eip1559.High)
			eip1559Api.Medium = eip1559FeesToApi(eip1559.Medium)
			eip1559Api.Low = eip1559FeesToApi(eip1559.Low)
			eip1559Api.NetworkCongestion = eip1559.NetworkCongestion
			eip1559Api.BaseFeeTrend = eip1559.BaseFeeTrend
			eip1559Api.PriorityFeeTrend = eip1559.PriorityFeeTrend
			eip1559Api.LatestPriorityFeeRange = eip1559FeeRangeToApi(eip1559.LatestPriorityFeeRange)
			eip1559Api.HistoricalBaseFeeRange = eip1559FeeRangeToApi(eip1559.HistoricalBaseFeeRange)
			eip1559Api.HistoricalPriorityFeeRange = eip1559FeeRangeToApi(eip1559.HistoricalPriorityFeeRange)
		}
		for i := range r.Blocks {
			res[i].FeePerUnit = fee.String()
			res[i].FeeLimit = sg
			res[i].FeePerTx = feePerTx.String()
			res[i].Eip1559 = eip1559Api
		}
	} else {
		conservative := true
		v, ok := r.Specific["conservative"]
		if ok {
			vc, ok := v.(bool)
			if ok {
				conservative = vc
			}
		}
		txSize := 0
		v, ok = r.Specific["txsize"]
		if ok {
			f, ok := v.(float64)
			if ok {
				txSize = int(f)
			}
		}
		for i, b := range r.Blocks {
			fee, err := s.api.EstimateFee(b, conservative)
			if err != nil {
				return nil, err
			}
			res[i].FeePerUnit = fee.String()
			if txSize > 0 {
				fee.Mul(&fee, big.NewInt(int64(txSize)))
				fee.Add(&fee, big.NewInt(500))
				fee.Div(&fee, big.NewInt(1000))
				res[i].FeePerTx = fee.String()
			}
		}
	}
	return res, nil
}

func (s *WebsocketServer) longTermFeeRate() (res interface{}, err error) {
	feeRate, err := s.chain.LongTermFeeRate()
	if err != nil {
		return nil, err
	}
	return WsLongTermFeeRateRes{
		FeePerUnit: feeRate.FeePerUnit.String(),
		Blocks:     feeRate.Blocks,
	}, nil
}

func (s *WebsocketServer) sendTransaction(tx string, disableAlternativeRPC bool) (res resultSendTransaction, err error) {
	txid, err := s.chain.SendRawTransaction(tx, disableAlternativeRPC)
	if err != nil {
		return res, err
	}
	res.Result = txid
	return
}

func (s *WebsocketServer) getMempoolFilters(r *WsMempoolFiltersReq) (res interface{}, err error) {
	type resMempoolFilters struct {
		ParamP    uint8             `json:"P"`
		ParamM    uint64            `json:"M"`
		ZeroedKey bool              `json:"zeroedKey"`
		Entries   map[string]string `json:"entries"`
	}
	filterEntries, err := s.mempool.GetTxidFilterEntries(r.ScriptType, r.FromTimestamp)
	if err != nil {
		return nil, err
	}
	return resMempoolFilters{
		ParamP:    s.is.BlockGolombFilterP,
		ParamM:    bchain.GetGolombParamM(s.is.BlockGolombFilterP),
		ZeroedKey: filterEntries.UsedZeroedKey,
		Entries:   filterEntries.Entries,
	}, nil
}

func (s *WebsocketServer) getBlockFilter(r *WsBlockFilterReq) (res interface{}, err error) {
	type resBlockFilter struct {
		ParamP      uint8  `json:"P"`
		ParamM      uint64 `json:"M"`
		ZeroedKey   bool   `json:"zeroedKey"`
		BlockFilter string `json:"blockFilter"`
	}
	if s.is.BlockFilterScripts != r.ScriptType {
		return nil, errors.Errorf("Unsupported script type %s", r.ScriptType)
	}
	blockFilter, err := s.db.GetBlockFilter(r.BlockHash)
	if err != nil {
		return nil, err
	}
	return resBlockFilter{
		ParamP:      s.is.BlockGolombFilterP,
		ParamM:      bchain.GetGolombParamM(s.is.BlockGolombFilterP),
		ZeroedKey:   s.is.BlockFilterUseZeroedKey,
		BlockFilter: blockFilter,
	}, nil
}

func (s *WebsocketServer) getBlockFiltersBatch(r *WsBlockFiltersBatchReq) (res interface{}, err error) {
	type resBlockFiltersBatch struct {
		ParamP            uint8    `json:"P"`
		ParamM            uint64   `json:"M"`
		ZeroedKey         bool     `json:"zeroedKey"`
		BlockFiltersBatch []string `json:"blockFiltersBatch"`
	}
	if s.is.BlockFilterScripts != r.ScriptType {
		return nil, errors.Errorf("Unsupported script type %s", r.ScriptType)
	}
	blockFiltersBatch, err := s.api.GetBlockFiltersBatch(r.BlockHash, r.PageSize)
	if err != nil {
		return nil, err
	}
	return resBlockFiltersBatch{
		ParamP:            s.is.BlockGolombFilterP,
		ParamM:            bchain.GetGolombParamM(s.is.BlockGolombFilterP),
		ZeroedKey:         s.is.BlockFilterUseZeroedKey,
		BlockFiltersBatch: blockFiltersBatch,
	}, nil
}

// evmCallSelector extracts the 4-byte function selector from hex-encoded EVM
// calldata as lowercase hex without the 0x prefix. It validates the full
// calldata hex (odd length or non-hex characters fail closed) but decodes
// only the first 4 bytes (8 hex chars) so that arbitrarily long calldata
// does not cause a large allocation that is then discarded.
func evmCallSelector(data string) (string, bool) {
	if len(data) < 10 || data[0] != '0' || (data[1] != 'x' && data[1] != 'X') {
		return "", false
	}
	s := data[2:]
	if len(s)&1 == 1 {
		return "", false
	}
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] >= '0' && s[i] <= '9':
		case s[i] >= 'a' && s[i] <= 'f':
		case s[i] >= 'A' && s[i] <= 'F':
		default:
			return "", false
		}
	}
	b, err := hex.DecodeString(s[:8])
	if err != nil {
		return "", false
	}
	return hex.EncodeToString(b), true
}

// rpcCallAllowed reports whether a rpcCall request passes the allowlists. With
// no allowlist configured rpcCall is unrestricted; otherwise the call must
// target an allowed address or invoke an allowed method selector. The snapshot
// is nil only when initRpcCallAllowlists has not run (bare test servers);
// NewWebsocketServer and NewInternalServer fail construction when they cannot
// publish one.
func (s *WebsocketServer) rpcCallAllowed(r *WsRpcCallReq) bool {
	a := s.is.GetRpcCallAllowlists()
	if a == nil || (a.To == nil && a.Methods == nil) {
		return true
	}
	if a.To != nil {
		if _, ok := a.To[strings.ToLower(r.To)]; ok {
			return true
		}
	}
	if a.Methods != nil {
		if selector, ok := evmCallSelector(r.Data); ok {
			if _, ok := a.Methods[selector]; ok {
				return true
			}
		}
	}
	return false
}

func (s *WebsocketServer) rpcCall(r *WsRpcCallReq) (*WsRpcCallRes, error) {
	if !s.rpcCallAllowed(r) {
		return nil, errors.New("Not supported")
	}
	data, err := s.chain.EthereumTypeRpcCall(r.Data, r.To, r.From)
	if err != nil {
		return nil, err
	}
	return &WsRpcCallRes{Data: data}, nil
}

type subscriptionResponse struct {
	Subscribed bool `json:"subscribed"`
}
type subscriptionResponseMessage struct {
	Subscribed bool   `json:"subscribed"`
	Message    string `json:"message"`
}

func (s *WebsocketServer) subscribeNewBlock(c *websocketChannel, req *WsReq) (res interface{}, err error) {
	s.newBlockSubscriptionsLock.Lock()
	defer s.newBlockSubscriptionsLock.Unlock()
	s.newBlockSubscriptions[c] = req.ID
	s.metrics.WebsocketSubscribes.With(common.Labels{"method": "subscribeNewBlock"}).Set(float64(len(s.newBlockSubscriptions)))
	return &subscriptionResponse{true}, nil
}

func (s *WebsocketServer) unsubscribeNewBlock(c *websocketChannel) (res interface{}, err error) {
	s.newBlockSubscriptionsLock.Lock()
	defer s.newBlockSubscriptionsLock.Unlock()
	delete(s.newBlockSubscriptions, c)
	s.metrics.WebsocketSubscribes.With(common.Labels{"method": "subscribeNewBlock"}).Set(float64(len(s.newBlockSubscriptions)))
	return &subscriptionResponse{false}, nil
}

func (s *WebsocketServer) subscribeNewTransaction(c *websocketChannel, req *WsReq) (res interface{}, err error) {
	s.newTransactionSubscriptionsLock.Lock()
	defer s.newTransactionSubscriptionsLock.Unlock()
	if !s.newTransactionEnabled {
		return &subscriptionResponseMessage{false, "subscribeNewTransaction not enabled, use -enablesubnewtx flag to enable."}, nil
	}
	s.newTransactionSubscriptions[c] = req.ID
	s.metrics.WebsocketSubscribes.With(common.Labels{"method": "subscribeNewTransaction"}).Set(float64(len(s.newTransactionSubscriptions)))
	return &subscriptionResponse{true}, nil
}

func (s *WebsocketServer) unsubscribeNewTransaction(c *websocketChannel) (res interface{}, err error) {
	s.newTransactionSubscriptionsLock.Lock()
	defer s.newTransactionSubscriptionsLock.Unlock()
	if !s.newTransactionEnabled {
		return &subscriptionResponseMessage{false, "unsubscribeNewTransaction not enabled, use -enablesubnewtx flag to enable."}, nil
	}
	delete(s.newTransactionSubscriptions, c)
	s.metrics.WebsocketSubscribes.With(common.Labels{"method": "subscribeNewTransaction"}).Set(float64(len(s.newTransactionSubscriptions)))
	return &subscriptionResponse{false}, nil
}

func (s *WebsocketServer) unmarshalAddresses(params []byte) ([]string, bool, error) {
	r := WsSubscribeAddressesReq{}
	err := json.Unmarshal(params, &r)
	if err != nil {
		return nil, false, api.NewAPIError("Invalid subscribeAddresses params", true)
	}
	limit := maxWebsocketSubscribeAddresses
	if r.NewBlockTxs {
		limit = maxWebsocketSubscribeAddressesWithNewBlockTxs
	}
	if len(r.Addresses) > limit {
		return nil, false, api.NewAPIError("addresses max "+strconv.Itoa(limit), true)
	}
	rv := make([]string, 0, len(r.Addresses))
	for _, a := range r.Addresses {
		ad, err := s.chainParser.GetAddrDescFromAddress(a)
		if err != nil {
			return nil, false, api.NewAPIError("Invalid address "+strconv.Quote(a)+", "+err.Error(), true)
		}
		rv = append(rv, string(ad))
	}
	return deduplicateAddressDescriptors(rv), r.NewBlockTxs, nil
}

func deduplicateAddressDescriptors(addrDesc []string) []string {
	if len(addrDesc) < 2 {
		return addrDesc
	}
	seen := make(map[string]struct{}, len(addrDesc))
	rv := addrDesc[:0]
	for _, ads := range addrDesc {
		if _, exists := seen[ads]; exists {
			continue
		}
		seen[ads] = struct{}{}
		rv = append(rv, ads)
	}
	return rv
}

// doUnsubscribeAddresses removes all address subscriptions for a channel.
// addressSubscriptionsLock must be held by the caller.
func (s *WebsocketServer) doUnsubscribeAddresses(c *websocketChannel) {
	for _, ads := range c.addrDescs {
		sa, e := s.addressSubscriptions[ads]
		if e {
			for sc, details := range sa {
				if sc == c {
					if details.publishNewBlockTxs {
						s.newBlockTxsSubscriptionCount--
					}
					delete(sa, c)
				}
			}
			if len(sa) == 0 {
				delete(s.addressSubscriptions, ads)
			}
		}
	}
	c.addrDescs = nil
}

// subscribeAddresses replaces previous address subscriptions for the channel.
// If newBlockTxs is enabled, the channel receives both mempool notifications and
// confirmed notifications detected from newly connected blocks.
func (s *WebsocketServer) subscribeAddresses(c *websocketChannel, addrDesc []string, newBlockTxs bool, req *WsReq) (res interface{}, err error) {
	addrDesc = deduplicateAddressDescriptors(addrDesc)
	s.addressSubscriptionsLock.Lock()
	defer s.addressSubscriptionsLock.Unlock()
	// unsubscribe all previous subscriptions
	s.doUnsubscribeAddresses(c)
	for _, ads := range addrDesc {
		as, ok := s.addressSubscriptions[ads]
		if !ok {
			as = make(map[*websocketChannel]*addressDetails)
			s.addressSubscriptions[ads] = as
		}
		as[c] = &addressDetails{
			requestID:          req.ID,
			publishNewBlockTxs: newBlockTxs,
		}
		if newBlockTxs {
			s.newBlockTxsSubscriptionCount++
		}
	}
	c.addrDescs = addrDesc
	s.metrics.WebsocketSubscribes.With(common.Labels{"method": "subscribeAddresses"}).Set(float64(len(s.addressSubscriptions)))
	s.metrics.WebsocketNewBlockTxsSubscriptions.Set(float64(s.newBlockTxsSubscriptionCount))
	return &subscriptionResponse{true}, nil
}

// unsubscribeAddresses unsubscribes all address subscriptions by this channel
func (s *WebsocketServer) unsubscribeAddresses(c *websocketChannel) (res interface{}, err error) {
	s.addressSubscriptionsLock.Lock()
	defer s.addressSubscriptionsLock.Unlock()
	s.doUnsubscribeAddresses(c)
	s.metrics.WebsocketSubscribes.With(common.Labels{"method": "subscribeAddresses"}).Set(float64(len(s.addressSubscriptions)))
	s.metrics.WebsocketNewBlockTxsSubscriptions.Set(float64(s.newBlockTxsSubscriptionCount))
	return &subscriptionResponse{false}, nil
}

// doUnsubscribeFiatRates fiat rates without fiatRatesSubscriptionsLock - can be called only from subscribeFiatRates and unsubscribeFiatRates
func (s *WebsocketServer) doUnsubscribeFiatRates(c *websocketChannel) {
	for fr, sa := range s.fiatRatesSubscriptions {
		for sc := range sa {
			if sc == c {
				delete(sa, c)
			}
		}
		if len(sa) == 0 {
			delete(s.fiatRatesSubscriptions, fr)
		}
	}
	delete(s.fiatRatesTokenSubscriptions, c)
}

// subscribeFiatRates subscribes all FiatRates subscriptions by this channel
func (s *WebsocketServer) subscribeFiatRates(c *websocketChannel, d *WsSubscribeFiatRatesReq, req *WsReq) (res interface{}, err error) {
	s.fiatRatesSubscriptionsLock.Lock()
	defer s.fiatRatesSubscriptionsLock.Unlock()
	// unsubscribe all previous subscriptions
	s.doUnsubscribeFiatRates(c)
	currency := d.Currency
	if currency == "" {
		currency = allFiatRates
	} else {
		currency = strings.ToLower(currency)
	}
	as, ok := s.fiatRatesSubscriptions[currency]
	if !ok {
		as = make(map[*websocketChannel]string)
		s.fiatRatesSubscriptions[currency] = as
	}
	as[c] = req.ID
	if len(d.Tokens) != 0 {
		s.fiatRatesTokenSubscriptions[c] = d.Tokens
	}
	s.metrics.WebsocketSubscribes.With(common.Labels{"method": "subscribeFiatRates"}).Set(float64(len(s.fiatRatesSubscriptions)))
	return &subscriptionResponse{true}, nil
}

// unsubscribeFiatRates unsubscribes all FiatRates subscriptions by this channel
func (s *WebsocketServer) unsubscribeFiatRates(c *websocketChannel) (res interface{}, err error) {
	s.fiatRatesSubscriptionsLock.Lock()
	defer s.fiatRatesSubscriptionsLock.Unlock()
	s.doUnsubscribeFiatRates(c)
	s.metrics.WebsocketSubscribes.With(common.Labels{"method": "subscribeFiatRates"}).Set(float64(len(s.fiatRatesSubscriptions)))
	return &subscriptionResponse{false}, nil
}

// newBlockNotification builds the subscribeNewBlock payload for a connected block. For EVM chains it
// attaches block-level gas data so subscribers can project the next EIP-1559 base fee; EVMData stays
// nil (evmData: null) for non-EVM chains and pre-London blocks (BaseFeePerGas absent).
func newBlockNotification(block *bchain.Block) *WsNewBlock {
	data := &WsNewBlock{
		Height: block.Height,
		Hash:   block.Hash,
	}
	if bsd, ok := block.CoinSpecificData.(*bchain.EthereumBlockSpecificData); ok && bsd != nil && bsd.BaseFeePerGas != nil {
		data.EVMData = &EthereumGasData{
			BaseFeePerGas: (*api.Amount)(bsd.BaseFeePerGas),
			BlockGasUsed:  (*api.Amount)(bsd.GasUsed),
			BlockGasLimit: (*api.Amount)(bsd.GasLimit),
		}
	}
	return data
}

// observeNewBlockGas records push-path block-gas metrics from the most recently connected block.
// It is called synchronously from OnNewBlock (which the single writeBlockWorker invokes in height
// order) before the async broadcast, so the gauges advance monotonically without a mutex; the
// per-block broadcast goroutines could otherwise reorder and let an older block clobber a newer
// value. Last-value semantics: it sweeps catch-up blocks and settles on the tip. Non-EVM and
// pre-London blocks (no EthereumBlockSpecificData with gas set) are skipped.
func (s *WebsocketServer) observeNewBlockGas(block *bchain.Block) {
	if s.metrics == nil {
		return
	}
	bsd, ok := block.CoinSpecificData.(*bchain.EthereumBlockSpecificData)
	if !ok || bsd == nil {
		return
	}
	if s.metrics.EthBlockGasUsedRatio != nil && bsd.GasUsed != nil && bsd.GasLimit != nil && bsd.GasLimit.Sign() > 0 {
		ratio, _ := new(big.Float).Quo(new(big.Float).SetInt(bsd.GasUsed), new(big.Float).SetInt(bsd.GasLimit)).Float64()
		s.metrics.EthBlockGasUsedRatio.Set(ratio)
	}
	if s.metrics.EthBlockBaseFee != nil && bsd.BaseFeePerGas != nil {
		baseFee, _ := new(big.Float).SetInt(bsd.BaseFeePerGas).Float64()
		s.metrics.EthBlockBaseFee.Set(baseFee)
	}
}

func (s *WebsocketServer) onNewBlockAsync(block *bchain.Block) {
	s.newBlockSubscriptionsLock.Lock()
	defer s.newBlockSubscriptionsLock.Unlock()
	data := newBlockNotification(block)
	for c, id := range s.newBlockSubscriptions {
		c.DataOut(&WsRes{
			ID:   id,
			Data: data,
		})
	}
	s.metrics.WebsocketNewBlockNotifications.Add(float64(len(s.newBlockSubscriptions)))
	glog.V(2).Info("broadcasting new block ", block.Height, " ", block.Hash, " to ", len(s.newBlockSubscriptions), " channels")
}

// setConfirmedBlockTxMetadata normalizes parsed block transactions.
// ParseBlock can return txs with zero confirmations; we force first-confirmed
// metadata so conversion does not take mempool-only branches.
func setConfirmedBlockTxMetadata(tx *bchain.Tx, blockTime int64) {
	if tx.Confirmations == 0 {
		tx.Confirmations = 1
		tx.Blocktime = blockTime
		tx.Time = blockTime
	}
}

// getEthereumInternalTransfers safely extracts internal transfers from
// CoinSpecificData when present.
func getEthereumInternalTransfers(tx *bchain.Tx) []bchain.EthereumInternalTransfer {
	esd, ok := tx.CoinSpecificData.(bchain.EthereumSpecificData)
	if !ok || esd.InternalData == nil {
		return nil
	}
	return esd.InternalData.Transfers
}

// setEthereumReceiptIfAvailable adds receipt data to Ethereum txs on a
// best-effort basis; failures are logged and notifications continue.
func setEthereumReceiptIfAvailable(tx *bchain.Tx, getReceipt func(string) (*bchain.RpcReceipt, error)) string {
	csd, ok := tx.CoinSpecificData.(bchain.EthereumSpecificData)
	if !ok {
		return "skipped_non_eth"
	}
	receipt, err := getReceipt(tx.Txid)
	if err != nil {
		glog.Error("EthereumTypeGetTransactionReceipt error ", err, " for ", tx.Txid)
		return "error"
	}
	csd.Receipt = receipt
	tx.CoinSpecificData = csd
	return "success"
}

func observeNewBlockTxDuration(metrics *common.Metrics, stage string, started time.Time) {
	if metrics == nil {
		return
	}
	metrics.WebsocketNewBlockTxsDuration.With(common.Labels{"stage": stage}).Observe(time.Since(started).Seconds())
}

func incNewBlockTxMetric(metrics *common.Metrics, stage, status string, value float64) {
	if metrics == nil {
		return
	}
	counter := metrics.WebsocketNewBlockTxs.With(common.Labels{"stage": stage, "status": status})
	if value == 1 {
		counter.Inc()
	} else {
		counter.Add(value)
	}
}

// populateBitcoinVinAddrDescs fills missing vin address descriptors by loading
// previous outputs. This enables sender-side address subscription matching for
// Bitcoin transactions parsed from connected blocks.
func populateBitcoinVinAddrDescs(vins []bchain.MempoolVin, getAddrDesc func(string, uint32) (bchain.AddressDescriptor, error)) {
	if getAddrDesc == nil {
		return
	}
	for i := range vins {
		if len(vins[i].AddrDesc) > 0 || vins[i].Txid == "" {
			continue
		}
		addrDesc, err := getAddrDesc(vins[i].Txid, vins[i].Vout)
		if err == nil && len(addrDesc) > 0 {
			vins[i].AddrDesc = addrDesc
		}
	}
}

// getBitcoinVinAddrDesc resolves an input outpoint to an address descriptor
// using txCache. It is best-effort and can return chain-level not-found errors.
func (s *WebsocketServer) getBitcoinVinAddrDesc(txid string, vout uint32) (bchain.AddressDescriptor, error) {
	if s.txCache == nil {
		return nil, bchain.ErrTxNotFound
	}
	prevTx, _, err := s.txCache.GetTransaction(txid)
	if err != nil {
		return nil, err
	}
	if int(vout) >= len(prevTx.Vout) {
		return nil, bchain.ErrAddressMissing
	}
	return s.chainParser.GetAddrDescFromVout(&prevTx.Vout[vout])
}

// publishNewBlockTxsByAddr emits confirmed transaction notifications only for
// subscribed addresses touched by transactions in the connected block.
func (s *WebsocketServer) publishNewBlockTxsByAddr(block *bchain.Block) {
	blockStart := time.Now()
	defer observeNewBlockTxDuration(s.metrics, "per_block", blockStart)
	chainType := s.chainParser.GetChainType()
	for _, tx := range block.Txs {
		incNewBlockTxMetric(s.metrics, "scanned", "success", 1)
		setConfirmedBlockTxMetadata(&tx, block.Time)
		var tokenTransfers bchain.TokenTransfers
		var internalTransfers []bchain.EthereumInternalTransfer
		if chainType == bchain.ChainEthereumType {
			tokenTransfers, _ = s.chainParser.EthereumTypeGetTokenTransfersFromTx(&tx)
			internalTransfers = getEthereumInternalTransfers(&tx)
		}
		vins := make([]bchain.MempoolVin, len(tx.Vin))
		for i, vin := range tx.Vin {
			vins[i] = bchain.MempoolVin{Vin: vin}
		}
		if chainType == bchain.ChainBitcoinType {
			populateBitcoinVinAddrDescs(vins, s.getBitcoinVinAddrDesc)
		}
		matchStart := time.Now()
		subscribed := s.getNewTxSubscriptions(vins, tx.Vout, tokenTransfers, internalTransfers, true)
		observeNewBlockTxDuration(s.metrics, "match", matchStart)
		if len(subscribed) > 0 {
			incNewBlockTxMetric(s.metrics, "matched", "success", 1)
			if ok, _ := s.trackWork(); !ok {
				return
			}
			// Convert and publish asynchronously so heavy tx conversion does not
			// block processing of other transactions in the same block.
			go func(tx bchain.Tx, subscribed map[string]struct{}) {
				defer s.workDone()
				if chainType == bchain.ChainEthereumType {
					receiptStatus := setEthereumReceiptIfAvailable(&tx, s.chain.EthereumTypeGetTransactionReceipt)
					if s.metrics != nil {
						s.metrics.WebsocketEthReceipt.With(common.Labels{"status": receiptStatus}).Inc()
					}
				}
				convertStart := time.Now()
				atx, err := s.api.GetTransactionFromBchainTx(&tx, int(block.Height), false, false, nil)
				observeNewBlockTxDuration(s.metrics, "convert", convertStart)
				if err != nil {
					incNewBlockTxMetric(s.metrics, "converted", "failure", 1)
					glog.Error("GetTransactionFromBchainTx error ", err, " for ", tx.Txid)
					return
				}
				incNewBlockTxMetric(s.metrics, "converted", "success", 1)
				for stringAddressDescriptor := range subscribed {
					s.sendOnNewTxAddr(stringAddressDescriptor, atx, true)
				}
				incNewBlockTxMetric(s.metrics, "published", "success", float64(len(subscribed)))
			}(tx, subscribed)
		}
	}
}

// OnNewBlock is a callback that broadcasts info about new block to subscribed clients
func (s *WebsocketServer) OnNewBlock(block *bchain.Block) {
	// Synchronous and before the async dispatch: OnNewBlock is called in monotonic height order, so
	// the push-path gas gauges never get an older block's value written after a newer one's.
	s.observeNewBlockGas(block)
	s.addressSubscriptionsLock.Lock()
	defer s.addressSubscriptionsLock.Unlock()
	go s.onNewBlockAsync(block)
	if s.newBlockTxsSubscriptionCount > 0 {
		// Skip per-tx address matching when nobody opted into newBlockTxs.
		if ok, _ := s.trackWork(); ok {
			go func() {
				defer s.workDone()
				s.publishNewBlockTxsByAddr(block)
			}()
		}
	}
}

func (s *WebsocketServer) sendOnNewTx(tx *api.Tx) {
	s.newTransactionSubscriptionsLock.Lock()
	defer s.newTransactionSubscriptionsLock.Unlock()
	for c, id := range s.newTransactionSubscriptions {
		c.DataOut(&WsRes{
			ID:   id,
			Data: &tx,
		})
	}
	glog.Info("broadcasting new tx ", tx.Txid, " to ", len(s.newTransactionSubscriptions), " channels")
}

func (s *WebsocketServer) sendOnNewTxAddr(stringAddressDescriptor string, tx *api.Tx, newBlockTx bool) {
	addrDesc := bchain.AddressDescriptor(stringAddressDescriptor)
	addr, _, err := s.chainParser.GetAddressesFromAddrDesc(addrDesc)
	if err != nil {
		glog.Error("GetAddressesFromAddrDesc error ", err, " for ", addrDesc)
		return
	}
	if len(addr) == 1 {
		data := struct {
			Address string  `json:"address"`
			Tx      *api.Tx `json:"tx"`
		}{
			Address: addr[0],
			Tx:      tx,
		}
		s.addressSubscriptionsLock.Lock()
		defer s.addressSubscriptionsLock.Unlock()
		as, ok := s.addressSubscriptions[stringAddressDescriptor]
		if ok {
			source := "mempool"
			if newBlockTx {
				source = "new_block"
			}
			for c, details := range as {
				// Mempool notifications go to all address subscribers; confirmed
				// block notifications only go to subscribers that requested them.
				if newBlockTx && !details.publishNewBlockTxs {
					continue
				}
				if s.metrics != nil {
					s.metrics.WebsocketAddrNotifications.With(common.Labels{"source": source}).Inc()
				}
				c.DataOut(&WsRes{
					ID:   details.requestID,
					Data: &data,
				})
			}
			glog.Info("broadcasting new tx ", tx.Txid, ", addr ", addr[0], " to ", len(as), " channels")
		}
	}
}

func (s *WebsocketServer) getNewTxSubscriptions(vins []bchain.MempoolVin, vouts []bchain.Vout, tokenTransfers bchain.TokenTransfers, internalTransfers []bchain.EthereumInternalTransfer, newBlockTxsOnly bool) map[string]struct{} {
	// check if there is any subscription in inputs, outputs and transfers
	candidates := make(map[string]struct{})
	addAddrDesc := func(addrDesc bchain.AddressDescriptor) {
		if len(addrDesc) > 0 {
			candidates[string(addrDesc)] = struct{}{}
		}
	}
	processAddress := func(address string) {
		if addrDesc, err := s.chainParser.GetAddrDescFromAddress(address); err == nil && len(addrDesc) > 0 {
			addAddrDesc(addrDesc)
		}
	}
	processVout := func(vout bchain.Vout) {
		if addrDesc, err := s.chainParser.GetAddrDescFromVout(&vout); err == nil && len(addrDesc) > 0 {
			addAddrDesc(addrDesc)
		}
	}
	for i := range vins {
		if sad := string(vins[i].AddrDesc); len(sad) > 0 {
			candidates[sad] = struct{}{}
		} else {
			switch s.chainParser.GetChainType() {
			case bchain.ChainBitcoinType:
				vout := int(vins[i].Vout)
				if vout >= 0 && vout < len(vouts) {
					processVout(vouts[vout])
				}
			case bchain.ChainEthereumType:
				if len(vins[i].Addresses) > 0 {
					processAddress(vins[i].Addresses[0])
				}
			}
		}
	}
	for i := range vouts {
		processVout(vouts[i])
	}
	for i := range tokenTransfers {
		processAddress(tokenTransfers[i].From)
		processAddress(tokenTransfers[i].To)
	}
	for i := range internalTransfers {
		processAddress(internalTransfers[i].From)
		processAddress(internalTransfers[i].To)
	}

	subscribed := make(map[string]struct{})
	s.addressSubscriptionsLock.Lock()
	defer s.addressSubscriptionsLock.Unlock()
	for sad := range candidates {
		as, ok := s.addressSubscriptions[sad]
		if !ok || len(as) == 0 {
			continue
		}
		if !newBlockTxsOnly {
			subscribed[sad] = struct{}{}
			continue
		}
		for _, details := range as {
			if details.publishNewBlockTxs {
				subscribed[sad] = struct{}{}
				break
			}
		}
	}
	return subscribed
}

func (s *WebsocketServer) onNewTxAsync(tx *bchain.MempoolTx, subscribed map[string]struct{}) {
	atx, err := s.api.GetTransactionFromMempoolTx(tx)
	if err != nil {
		glog.Error("GetTransactionFromMempoolTx error ", err, " for ", tx.Txid)
		return
	}
	s.sendOnNewTx(atx)
	for stringAddressDescriptor := range subscribed {
		s.sendOnNewTxAddr(stringAddressDescriptor, atx, false)
	}
}

// OnNewTx is a callback that broadcasts info about a tx affecting subscribed address
func (s *WebsocketServer) OnNewTx(tx *bchain.MempoolTx) {
	subscribed := s.getNewTxSubscriptions(tx.Vin, tx.Vout, tx.TokenTransfers, nil, false)
	if len(s.newTransactionSubscriptions) > 0 || len(subscribed) > 0 {
		if ok, _ := s.trackWork(); ok {
			go func() {
				defer s.workDone()
				s.onNewTxAsync(tx, subscribed)
			}()
		}
	}
}

func (s *WebsocketServer) broadcastTicker(currency string, rates map[string]float32, ticker *common.CurrencyRatesTicker) {
	as, ok := s.fiatRatesSubscriptions[currency]
	if ok && len(as) > 0 {
		data := struct {
			Rates interface{} `json:"rates"`
		}{
			Rates: rates,
		}
		for c, id := range as {
			var tokens []string
			if ticker != nil {
				tokens = s.fiatRatesTokenSubscriptions[c]
			}
			if len(tokens) > 0 {
				dataWithTokens := struct {
					Rates      interface{}        `json:"rates"`
					TokenRates map[string]float32 `json:"tokenRates,omitempty"`
				}{
					Rates:      rates,
					TokenRates: map[string]float32{},
				}
				for _, token := range tokens {
					rate := ticker.TokenRateInCurrency(token, currency)
					if rate > 0 {
						dataWithTokens.TokenRates[token] = rate
					}
				}
				c.DataOut(&WsRes{
					ID:   id,
					Data: &dataWithTokens,
				})
			} else {
				c.DataOut(&WsRes{
					ID:   id,
					Data: &data,
				})
			}
		}
		glog.Info("broadcasting new rates for currency ", currency, " to ", len(as), " channels")
	}
}

// OnNewFiatRatesTicker is a callback that broadcasts info about fiat rates affecting subscribed currency
func (s *WebsocketServer) OnNewFiatRatesTicker(ticker *common.CurrencyRatesTicker) {
	s.fiatRatesSubscriptionsLock.Lock()
	defer s.fiatRatesSubscriptionsLock.Unlock()
	for currency, rate := range ticker.Rates {
		s.broadcastTicker(currency, map[string]float32{currency: rate}, ticker)
	}
	s.broadcastTicker(allFiatRates, ticker.Rates, nil)
}

func (s *WebsocketServer) getCurrentFiatRates(currencies []string, token string) (*api.FiatTicker, error) {
	ret, err := s.api.GetCurrentFiatRates(currencies, token)
	return ret, err
}

func (s *WebsocketServer) getFiatRatesForTimestamps(timestamps []int64, currencies []string, token string) (*api.FiatTickers, error) {
	ret, err := s.api.GetFiatRatesForTimestamps(timestamps, currencies, token)
	return ret, err
}

func (s *WebsocketServer) getAvailableVsCurrencies(timestamp int64, token string) (*api.AvailableVsCurrencies, error) {
	ret, err := s.api.GetAvailableVsCurrencies(timestamp, token)
	return ret, err
}
