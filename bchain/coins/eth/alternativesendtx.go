package eth

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
)

// storedTx is one entry of the alternative-provider pending cache. Its tx body is IMMUTABLE once
// published into mempoolTxs: nothing writes through the pointer and GetTransaction hands out a copy,
// which is what makes every read of it race-free. Mutating a body in place reopens all of them at once.
type storedTx struct {
	tx        *bchain.RpcTransaction
	time      uint32
	gen       uint64 // send generation of the submission that created this entry, orders it against later sends
	lastProbe uint32 // unix seconds of the last reconcile probe, 0 until the first one; see probeInterval
	// missingSince is the unix time the current run of consecutive null eth_getTransactionByHash
	// answers started, 0 while the relay surfaces the tx. The relay answers from one consistent store
	// over its whole pending window, so a run outlasting missingTimeout() means dropped or cancelled
	// and reconcile evicts; requiring a run rather than one null absorbs a transient relay fluke. See
	// defaultAlternativeMissingTxTimeout.
	missingSince uint32
	// The nonce slot this entry fills, decoded from the body once at insert rather than on every scan.
	// The body is immutable, so these cannot drift from it. Four scans walk the whole cache - the
	// pending-nonce floor on every address request, and three per accepted send - and re-parsing an
	// address and a quantity per entry inside mempoolTxsMux is what made cache depth cost.
	from    ethcommon.Address
	nonce   uint64
	decoded bool
}

// recentSender records when an address last successfully sent a transaction through an
// alternative provider and which provider URL accepted it.
type recentSender struct {
	time time.Time
	url  string
	gen  uint64 // monotonic send generation, orders the send against cached-tx evictions
}

// nonceSlot identifies the account nonce slot a transaction fills - only one transaction per
// (sender, nonce) can ever mine.
type nonceSlot struct {
	addr  ethcommon.Address
	nonce uint64
}

// acceptedSlot records the newest send generation a relay accepted for a nonce slot. It outlives the
// cache entry that send produces (or fails to produce) - see slotSupersededBy.
type acceptedSlot struct {
	gen  uint64
	time time.Time
}

const alternativeMempoolTxCheckPeriod = time.Minute

// alternativeNonceRoutingTimeout caps how long after an accepted send the sender's eth_getTransactionCount
// and eth_estimateGas lookups keep being routed to the relay. It is deliberately shorter than the cache
// retention: a released sender still gets the same nonce, because the cached transaction carries it and
// the floor is applied to the primary RPC's answer too (see EthereumTypeGetNonces), while eth_estimateGas
// is called on every send-form keystroke, which is how the relay's rate quota was exhausted in #1629. The
// window still covers the minutes right after a send, when a cache entry can be missing and only the
// relay has the transaction.
const alternativeNonceRoutingTimeout = 15 * time.Minute

// probeInterval returns how often reconcile re-asks the relay about a cached entry of the given age; the
// tick itself stays at alternativeMempoolTxCheckPeriod. A young entry is where the answer still changes
// (about to mine, or its nonce about to be consumed), so it is probed every cycle. An entry that has
// waited an hour is waiting on a builder, where probing every cycle buys a quarter-hour of eviction
// latency at sixty times the relay quota: a fresh dial per probe plus, through
// transactionSupersededByNonce, an eth_getTransactionCount per URL.
func probeInterval(age time.Duration) time.Duration {
	switch {
	case age < 10*time.Minute:
		return alternativeMempoolTxCheckPeriod
	case age < time.Hour:
		return 5 * time.Minute
	default:
		return 15 * time.Minute
	}
}

// maxBackgroundFetchBacks and maxExposeFetchBacks bound the post-send fetch-backs in flight. The send
// path no longer waits for its own, so in-flight work is no longer bounded by in-flight requests. The
// allowances are separate because a refused refresh loses nothing while a refused expose loses the send
// (see the two call sites), and one shared allowance let refresh traffic starve the exposing one.
const (
	maxBackgroundFetchBacks = 16
	maxExposeFetchBacks     = 16
)

// backgroundDrainMargin and backgroundDrainPoll bound and pace the shutdown drain (see drainBackground).
const (
	backgroundDrainMargin = 2 * time.Second
	backgroundDrainPoll   = 10 * time.Millisecond
)

// backgroundKind selects which allowance a fetch-back draws on: backgroundRefresh for the ones that only
// replace a cached body, backgroundExpose for the one that is the sole thing able to expose a send.
type backgroundKind int

const (
	backgroundRefresh backgroundKind = iota
	backgroundExpose
)

// AlternativeSendTxProvider handles sending transactions to alternative providers
type AlternativeSendTxProvider struct {
	urls                         []string
	onlyAlternative              bool
	fetchMempoolTx               bool
	mempoolTxs                   map[string]storedTx
	mempoolTxsMux                sync.Mutex
	mempoolTxsTimeout            time.Duration
	missingTxTimeout             time.Duration
	rpcTimeout                   time.Duration
	mempool                      *bchain.MempoolEthereumType
	metrics                      *common.Metrics
	removeTransactionFromMempool func(string)
	watchMempoolTxsOnce          sync.Once
	stop                         chan struct{}
	stopOnce                     sync.Once
	recentSenders                map[ethcommon.Address]recentSender
	acceptedSlots                map[nonceSlot]acceptedSlot // guarded by recentSendersMux
	sendGeneration               uint64                     // counts successful sends; guarded by recentSendersMux
	recentSendersMux             sync.Mutex
	backgroundCount              int        // refresh fetch-backs in flight; guarded by backgroundMux
	exposeCount                  int        // decode-failure fetch-backs in flight; guarded by backgroundMux
	backgroundIdle               *sync.Cond // signalled when backgroundCount reaches 0; created lazily
	backgroundMux                sync.Mutex
	exposeFetchBackDelay         time.Duration // test seam; 0 means exposeFetchBackRetryDelay
}

// NewAlternativeSendTxProvider creates a new alternative send tx provider if enabled
func NewAlternativeSendTxProvider(network string, rpcTimeout int, mempoolTxsTimeout, missingTxTimeout time.Duration, metrics *common.Metrics) *AlternativeSendTxProvider {
	urls := strings.Split(os.Getenv(strings.ToUpper(network)+"_ALTERNATIVE_SENDTX_URLS"), ",")
	onlyAlternative := strings.ToUpper(os.Getenv(strings.ToUpper(network)+"_ALTERNATIVE_SENDTX_ONLY")) == "TRUE"
	fetchMempoolTx := strings.ToUpper(os.Getenv(strings.ToUpper(network)+"_ALTERNATIVE_FETCH_MEMPOOL_TX")) == "TRUE"
	// Empty URL keeps the normal public RPC send path.
	if len(urls) == 0 || urls[0] == "" {
		return nil
	}

	provider := &AlternativeSendTxProvider{
		urls:              urls,
		onlyAlternative:   onlyAlternative,
		fetchMempoolTx:    fetchMempoolTx,
		rpcTimeout:        time.Duration(rpcTimeout) * time.Second,
		mempoolTxsTimeout: mempoolTxsTimeout,
		missingTxTimeout:  missingTxTimeout,
		mempoolTxs:        make(map[string]storedTx),
		recentSenders:     make(map[ethcommon.Address]recentSender),
		acceptedSlots:     make(map[nonceSlot]acceptedSlot),
		metrics:           metrics,
		stop:              make(chan struct{}),
	}

	// hosts only - the configured URLs commonly carry an API key
	glog.Infof("Using %d alternative send transaction providers %v. Only alternative providers %v", len(urls), providerLabels(urls), onlyAlternative)
	if fetchMempoolTx {
		glog.Infof("Alternative fetch mempool tx %v", fetchMempoolTx)
	}

	return provider
}

// SetupMempool sets up connection to the mempool
func (p *AlternativeSendTxProvider) SetupMempool(mempool *bchain.MempoolEthereumType, removeTransactionFromMempool func(string)) {
	p.mempool = mempool
	p.removeTransactionFromMempool = removeTransactionFromMempool
	if p.fetchMempoolTx {
		p.watchMempoolTxsOnce.Do(func() {
			go p.watchMempoolTxs()
		})
	}
}

// SendRawTransaction sends raw transaction to alternative providers. A wallet waits for this call, and a
// relay slow past the wallet's deadline means a transaction the user is told failed while it is on its
// way to the chain - a re-send at the next nonce then pays the recipient twice. So the broadcast to every
// relay runs concurrently and the post-send fetch-back is not waited for at all.
func (p *AlternativeSendTxProvider) SendRawTransaction(hex string) (string, error) {
	var txid string
	var retErr error
	var acceptedURL string

	// decoded once for the log lines and the accepted-send bookkeeping below - a send is the only
	// moment Blockbook sees the sender and nonce of a private transaction, and both are what a
	// later "my tx vanished" or nonce-gap report has to be reconciled against
	sent, decErr := decodeAlternativeSendTx(hex)

	// Fanning out rather than looping keeps the wallet's answer off len(urls) * rpcTimeout. Results are
	// collected in URL order below, so the aggregation - and which URL counts as the accepting one -
	// stays as deterministic as the sequential loop it replaces.
	type sendResult struct {
		txid string
		err  error
	}
	results := make([]sendResult, len(p.urls))
	var wg sync.WaitGroup
	for i := range p.urls {
		// pre-set, so a panic recovered below cannot leave a zero value that reads as success
		results[i] = sendResult{err: errors.New(p.urls[i] + " eth_sendRawTransaction : no result")}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			host := providerLabel(p.urls[i])
			start := time.Now()
			// a panic here would take the process down: the calling request handler's own recover
			// cannot reach this goroutine
			defer func() {
				if r := recover(); r != nil {
					// count the attempt too: a panicking provider call would otherwise vanish from
					// eth_alternative_sendtx_total and the broadcast would look never-tried
					p.observeSendTx(host, time.Since(start), fmt.Errorf("panic in eth_sendRawTransaction: %v", r))
					glog.Errorf("eth_sendRawTransaction to %s panicked: %v", host, r)
				}
			}()
			r, err := p.callHttpStringResult(p.urls[i], "eth_sendRawTransaction", hex)
			duration := time.Since(start)
			p.observeSendTx(host, duration, err)
			if err != nil {
				// not terminal on its own - another provider may still accept the transaction, and the
				// all-rejected case is logged as an error below
				glog.Warningf("eth_sendRawTransaction to provider %d/%d %s rejected %s after %v: %v", i+1, len(p.urls), host, sentTxLogString(sent, decErr), duration.Round(time.Millisecond), err)
			} else {
				glog.Infof("eth_sendRawTransaction to provider %d/%d %s accepted %s as txid %s in %v", i+1, len(p.urls), host, sentTxLogString(sent, decErr), r, duration.Round(time.Millisecond))
			}
			results[i] = sendResult{txid: r, err: err}
		}(i)
	}
	wg.Wait()

	for i := range results {
		r, err := results[i].txid, results[i].err
		if err == nil && acceptedURL == "" {
			acceptedURL = p.urls[i]
		}
		// set success return value; or error only if there was no previous success
		if err == nil || len(txid) == 0 {
			txid = r
			retErr = err
		}
	}

	// keyed on acceptedURL rather than retErr, so the follow-up work does not silently depend on
	// callHttpStringResult never returning an empty result without an error. With nothing accepted
	// there is no send to register and no txid to fetch back - the lookup would query the zero
	// hash and log a confusing not-found error.
	if acceptedURL == "" {
		glog.Errorf("eth_sendRawTransaction rejected by all %d alternative providers, %s: %v", len(p.urls), sentTxLogString(sent, decErr), retErr)
		return txid, retErr
	}

	// Ordered before the decode-dependent registration, because a send that does not decode is
	// still a send: leaving it at the zero generation made it read as OLDER than every earlier
	// acceptance, so an undecodable replacement lost its own slot to the predecessor it replaces
	// and ended up exposed nowhere. Generations order acceptances, which is knowable here; what
	// the decode adds is which slot the acceptance fills.
	gen := p.nextSendGeneration()
	// the transaction was decoded once at the top of the send and the result is reused for
	// recent-sender registration, the RBF eviction and the cache entry below - a single ECDSA
	// sender recovery instead of one per consumer. On decode failure neither eviction nor local
	// caching runs, but the transaction is still handed to handleMempoolTransaction(txid, gen).
	if decErr != nil {
		glog.Warningf("cannot decode accepted transaction: %v", decErr)
	} else {
		p.registerSuccessfulSend(sent.from, sent.nonce, acceptedURL, gen)
		if !strings.EqualFold(sent.txid, txid) {
			// Report the locally derived hash, which is what the chain will show and what everything
			// below is keyed on; the echo would name a txid Blockbook has no entry for.
			glog.Errorf("eth_sendRawTransaction echoed txid %s, signed bytes hash to %s", txid, sent.txid)
		}
		txid = sent.txid
	}

	if p.onlyAlternative && p.fetchMempoolTx {
		// An accepted (from, nonce) means the relay now builds with THIS tx, so retire the slot's
		// predecessor now rather than waiting for the relay to surface the replacement - a drop-mode
		// cancel never is, which left the predecessor "Unconfirmed" until the timeout (#1573). Not
		// proof: a predecessor already forwarded to builders can still mine, and block sync
		// re-indexes it then. Still needed below the insert, which can bail out as superseded.
		if decErr == nil {
			p.evictReplacedByNonce(sent.from, sent.nonce, sent.txid, gen)
			// Cache and index from the signed bytes before asking the relay: an accepted send must
			// never end up exposed nowhere, and a relay is free to never surface what it accepted.
			p.cacheMempoolTransaction(sent.txid, sent.body, gen)
			// The fetch-back only probes (see probeSentTransaction), so nothing observable depends on
			// it and dropping it under load is safe - reconcile re-probes the entry within a cycle.
			p.inBackground(backgroundRefresh, func() { p.probeSentTransaction(sent.txid) })
		} else {
			// Nothing was cached (the raw hex did not decode), so here the fetch-back is the only
			// thing that can expose the transaction. Dropping it loses the accepted send: nothing
			// serves it, nothing indexes it and the pending-nonce floor does not rise, which is the
			// nonce-reuse precursor - so count it like a relay that does not surface what it accepted.
			if !p.inBackground(backgroundExpose, func() { p.exposeAcceptedSend(txid, gen) }) && !p.stopped() {
				p.observeSendNotSurfaced("dropped")
			}
		}
	}

	return txid, retErr
}

// providerLabel reduces a provider URL to its host for log lines and metric labels: configured
// provider URLs routinely carry an API key in the path or query, which must not end up in logs or
// on the metrics endpoint. Providers sharing a host therefore share a label; the log lines keep
// the URL's position in the configured list to tell them apart.
func providerLabel(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return "unknown"
	}
	return u.Host
}

// providerLabels maps providerLabel over a list of provider URLs.
func providerLabels(rawURLs []string) []string {
	labels := make([]string, len(rawURLs))
	for i, u := range rawURLs {
		labels[i] = providerLabel(u)
	}
	return labels
}

// decodedTx is a raw transaction decoded once per send. Recovering the sender costs an elliptic
// curve operation, by far the most expensive step of a broadcast outside the network call itself,
// so the value is decoded at the top of a send and reused by every consumer below it.
type decodedTx struct {
	tx     *types.Transaction
	sender ethcommon.Address
	err    error
}

// String renders the sender and nonce for a log line. Failures are rendered rather than returned -
// this only feeds logging and must never change a send's outcome.
func (d decodedTx) String() string {
	if d.tx == nil {
		return fmt.Sprintf("undecodable tx (%v)", d.err)
	}
	if d.err != nil {
		return fmt.Sprintf("tx nonce %d with unrecoverable sender (%v)", d.tx.Nonce(), d.err)
	}
	return fmt.Sprintf("tx from %s nonce %d", d.sender.Hex(), d.tx.Nonce())
}

// observeSendTx records the outcome and latency of one provider's eth_sendRawTransaction call.
// Per-provider is the granularity that matters: a broadcast succeeds if any provider accepts, so
// a single provider degrading (timeouts, rate limits) stays invisible in the overall send rate
// until it is the last one left.
func (p *AlternativeSendTxProvider) observeSendTx(host string, duration time.Duration, err error) {
	if p.metrics == nil {
		return
	}
	// provider_host, not provider: the registry's provider label carries a symbolic implementation
	// name (infura, 1inch), while these providers are configured as a list of URLs and a host is
	// the only stable identity they have - two value domains must not share a label name
	status := bchain.SendTxStatus(err)
	if p.metrics.EthAlternativeSendTx != nil {
		p.metrics.EthAlternativeSendTx.With(common.Labels{"provider_host": host, "status": status, "reason": bchain.ClassifySendTxError(err)}).Inc()
	}
	if p.metrics.EthAlternativeSendTxDuration != nil {
		p.metrics.EthAlternativeSendTxDuration.With(common.Labels{"provider_host": host, "status": status}).Observe(duration.Seconds())
	}
}

// decodeRawTx unmarshals a raw transaction hex and recovers its sender. The chain id needed to
// derive the sender is taken from the transaction itself. A decoded transaction is kept even when
// the sender cannot be recovered, so a caller that only logs can still report its nonce.
func decodeRawTx(rawTxHex string) decodedTx {
	var tx types.Transaction
	if err := tx.UnmarshalBinary(ethcommon.FromHex(rawTxHex)); err != nil {
		return decodedTx{err: err}
	}
	sender, err := types.Sender(types.LatestSignerForChainID(tx.ChainId()), &tx)
	return decodedTx{tx: &tx, sender: sender, err: err}
}

// inBackground runs the post-send fetch-back off the send path, so the wallet's answer does not wait for
// a relay round-trip it cannot observe the outcome of. It reports whether it took the work: it declines
// at shutdown and drops beyond the per-kind allowance rather than queueing, because a queue either grows
// unbounded or pushes back into the wallet's deadline. What a drop costs differs per call site.
func (p *AlternativeSendTxProvider) inBackground(kind backgroundKind, fetchBack func()) bool {
	if !p.startBackground(kind) {
		return false
	}
	go func() {
		defer p.finishBackground(kind)
		// this goroutine outlives the request handler, whose own recover cannot protect it
		defer func() {
			if r := recover(); r != nil {
				glog.Errorf("background fetch-back panicked: %v", r)
			}
		}()
		fetchBack()
	}()

	return true
}

func (p *AlternativeSendTxProvider) inFlight(kind backgroundKind) int {
	if kind == backgroundExpose {
		return p.exposeCount
	}
	return p.backgroundCount
}

// startBackground reserves one of the per-kind fetch-back slots, reporting whether the caller got one. A
// plain sync.WaitGroup cannot serve here: Add would race the Wait in waitForRefreshes (the documented
// reuse misuse, which panics and leaves the counter stuck), and it could not cap anything.
func (p *AlternativeSendTxProvider) startBackground(kind backgroundKind) bool {
	p.backgroundMux.Lock()
	defer p.backgroundMux.Unlock()
	// under the mutex, so a send racing shutdown cannot pass this check and then spawn a goroutine
	// the drain in shutdown has already stopped waiting for
	if p.stopped() {
		return false
	}
	max := maxBackgroundFetchBacks
	if kind == backgroundExpose {
		max = maxExposeFetchBacks
	}
	if p.inFlight(kind) >= max {
		glog.Warningf("skipping fetch-back: %d of kind %d already in flight", p.inFlight(kind), kind)
		return false
	}
	if p.backgroundIdle == nil {
		p.backgroundIdle = sync.NewCond(&p.backgroundMux)
	}
	if kind == backgroundExpose {
		p.exposeCount++
	} else {
		p.backgroundCount++
	}
	return true
}

// stopped reports whether shutdown has been requested. A fetch-back declined at shutdown must not be
// reported as a lost send: an alert on the nonce-reuse precursor must not fire on every restart that
// catches a send in flight.
func (p *AlternativeSendTxProvider) stopped() bool {
	if p.stop == nil {
		return false
	}
	select {
	case <-p.stop:
		return true
	default:
		return false
	}
}

func (p *AlternativeSendTxProvider) finishBackground(kind backgroundKind) {
	p.backgroundMux.Lock()
	defer p.backgroundMux.Unlock()
	if kind == backgroundExpose {
		p.exposeCount--
	} else {
		p.backgroundCount--
	}
	if p.backgroundCount+p.exposeCount == 0 {
		p.backgroundIdle.Broadcast()
	}
}

// waitForRefreshes blocks until every background fetch-back has finished. For tests, which need a
// deterministic cache state after a send; production never waits.
func (p *AlternativeSendTxProvider) waitForRefreshes() {
	p.backgroundMux.Lock()
	defer p.backgroundMux.Unlock()
	if p.backgroundIdle == nil {
		p.backgroundIdle = sync.NewCond(&p.backgroundMux)
	}
	for p.backgroundCount+p.exposeCount > 0 {
		p.backgroundIdle.Wait()
	}
}

// probeSentTransaction asks the relay about a transaction the send path already cached from its signed
// bytes, and writes nothing. Adopting the relay's body would buy nothing (the signed bytes carry it all)
// and could serve its hash as the entry's identity, undoing the send path's own txid override. Evicting
// on a mined answer would be premature: the relay can see the block before Blockbook indexes it, and
// eviction clears the address index too, so the transaction would briefly be in neither store; block
// sync removes it as sync_removed, with reconcile as the backstop.
func (p *AlternativeSendTxProvider) probeSentTransaction(txid string) {
	tx, found, err := p.getTransactionFromProviders(txid)
	if err != nil {
		p.observeSendNotSurfaced("error")
		glog.Errorf("eth_getTransactionByHash from alternative providers returned error %v", err)
		return
	}
	if !found {
		p.observeSendNotSurfaced("not_found")
		glog.Errorf("eth_getTransactionByHash from alternative providers did not find txid %s", txid)
		return
	}
	if tx.BlockNumber != "" {
		glog.Infof("eth_getTransactionByHash from alternative providers already reports txid %s mined; leaving it to block sync", txid)
	}
}

// alternativeSendTx is everything the send path derives from an accepted raw transaction.
type alternativeSendTx struct {
	from  ethcommon.Address
	nonce uint64
	txid  string
	body  *bchain.RpcTransaction
}

// sentTxLogString renders the sender and nonce of a decoded send for a log line. Failures are
// rendered rather than returned - this only feeds logging and must never change a send's outcome.
func sentTxLogString(sent *alternativeSendTx, decErr error) string {
	if sent == nil {
		return fmt.Sprintf("undecodable tx (%v)", decErr)
	}
	return fmt.Sprintf("tx from %s nonce %d", sent.from.Hex(), sent.nonce)
}

// decodeAlternativeSendTx derives everything the send path needs from the signed bytes in one decode,
// taking the chain id from the transaction itself: the (from, nonce) slot the send fills, the txid the
// chain will show whatever the relay echoes, and a complete pending body, so an accepted send can be
// surfaced and indexed without the relay returning it. Encoding follows eth_getTransactionByHash:
// lower-case hex addresses, minimal quantities, gasPrice carrying the fee cap for typed fee-market txs.
func decodeAlternativeSendTx(rawTxHex string) (*alternativeSendTx, error) {
	var tx types.Transaction
	if err := tx.UnmarshalBinary(ethcommon.FromHex(rawTxHex)); err != nil {
		return nil, err
	}
	// LatestSignerForChainID PANICS for a chain id of zero or nil, which is what an unprotected
	// (pre-EIP-155) legacy transaction decodes to - and a panic here would abort the response to a
	// wallet whose transaction the relay has already accepted. Such a transaction is Homestead-signed.
	var signer types.Signer = types.HomesteadSigner{}
	if chainID := tx.ChainId(); chainID != nil && chainID.Sign() > 0 {
		signer = types.LatestSignerForChainID(chainID)
	}
	sender, err := types.Sender(signer, &tx)
	if err != nil {
		return nil, err
	}
	body := &bchain.RpcTransaction{
		AccountNonce: hexutil.EncodeUint64(tx.Nonce()),
		GasPrice:     hexutil.EncodeBig(tx.GasFeeCap()),
		GasLimit:     hexutil.EncodeUint64(tx.Gas()),
		Value:        hexutil.EncodeBig(tx.Value()),
		Payload:      hexutil.Encode(tx.Data()),
		Hash:         tx.Hash().Hex(),
		From:         strings.ToLower(sender.Hex()),
		// BlockNumber, BlockHash and TransactionIndex stay empty - this is a mempool transaction,
		// which is also what makes GetTransaction render it through the mempool branch.
	}
	if tx.Type() != types.LegacyTxType && tx.Type() != types.AccessListTxType {
		body.MaxFeePerGas = hexutil.EncodeBig(tx.GasFeeCap())
		body.MaxPriorityFeePerGas = hexutil.EncodeBig(tx.GasTipCap())
	}
	if to := tx.To(); to != nil {
		body.To = strings.ToLower(to.Hex())
	}
	return &alternativeSendTx{from: sender, nonce: tx.Nonce(), txid: body.Hash, body: body}, nil
}

// routingTimeout is how long after an accepted send the sender's nonce and gas-estimate lookups keep
// being routed to the relay: alternativeNonceRoutingTimeout, or the cache retention when that is
// configured shorter. Computed rather than stored so a provider built by a test from a struct literal
// cannot end up with a zero horizon that silently routes nothing.
func (p *AlternativeSendTxProvider) routingTimeout() time.Duration {
	if p.mempoolTxsTimeout < alternativeNonceRoutingTimeout {
		return p.mempoolTxsTimeout
	}
	return alternativeNonceRoutingTimeout
}

// missingTimeout is how long a cached transaction may stay missing from the relay - consecutive null
// eth_getTransactionByHash answers - before reconcile evicts it; the cache timeout stays the backstop
// regardless. Defaulted here as well so a provider built by a test from a struct literal cannot end up
// with a zero horizon that evicts on the first null answer.
func (p *AlternativeSendTxProvider) missingTimeout() time.Duration {
	if p.missingTxTimeout > 0 {
		return p.missingTxTimeout
	}
	return defaultAlternativeMissingTxTimeout
}

// nextSendGeneration assigns the send generation of an accepted submission, sweeping the expired
// entries of both send-tracking maps on the way. Every acceptance takes one, whether or not its raw hex
// decodes, so the generation always orders it against the other acceptances; the caller carries it to
// the cache entry it creates, so releaseRecentSender and slotSupersededBy can order evictions against
// later sends.
func (p *AlternativeSendTxProvider) nextSendGeneration() uint64 {
	p.recentSendersMux.Lock()
	defer p.recentSendersMux.Unlock()
	p.sweepRecentSendsLocked()
	p.sendGeneration++

	return p.sendGeneration
}

// registerSuccessfulSend records the sender of an accepted transaction so useForNonces routes its nonce
// lookups to the accepting URL - the one provider guaranteed to know the tx - for the routing window,
// and records the (sender, nonce) slot so the acceptance survives a fetch-back that produces no cache
// entry (see slotSupersededBy). gen comes from nextSendGeneration; only a strictly newer acceptance may
// overwrite either entry, since separating the two calls left nothing else ordering them.
func (p *AlternativeSendTxProvider) registerSuccessfulSend(sender ethcommon.Address, nonce uint64, acceptedURL string, gen uint64) {
	now := time.Now()
	p.recentSendersMux.Lock()
	defer p.recentSendersMux.Unlock()
	if p.recentSenders == nil {
		p.recentSenders = make(map[ethcommon.Address]recentSender)
	}
	if p.acceptedSlots == nil {
		p.acceptedSlots = make(map[nonceSlot]acceptedSlot)
	}
	if s, found := p.recentSenders[sender]; !found || gen > s.gen {
		p.recentSenders[sender] = recentSender{time: now, url: acceptedURL, gen: gen}
	}
	slot := nonceSlot{addr: sender, nonce: nonce}
	if s, found := p.acceptedSlots[slot]; !found || gen > s.gen {
		p.acceptedSlots[slot] = acceptedSlot{gen: gen, time: now}
	}
}

// sweepRecentSendsLocked drops the expired entries of both send-tracking maps and returns the instant
// it swept at. The two horizons differ on purpose: routing is a quota decision that stops paying off
// within minutes, while an accepted slot must outlive every predecessor it could still retire, i.e. as
// long as one can land (see slotSupersededBy). Callers must hold recentSendersMux.
func (p *AlternativeSendTxProvider) sweepRecentSendsLocked() time.Time {
	now := time.Now()
	routing := p.routingTimeout()
	for addr, s := range p.recentSenders {
		if now.Sub(s.time) > routing {
			delete(p.recentSenders, addr)
		}
	}
	for slot, s := range p.acceptedSlots {
		if now.Sub(s.time) > p.mempoolTxsTimeout {
			delete(p.acceptedSlots, slot)
		}
	}
	return now
}

// sweepRecentSends is the reconcile loop's call into the sweep above. Without it the maps shrink only
// when a new send arrives, so an instance that goes quiet after a burst holds every entry until the next
// one.
func (p *AlternativeSendTxProvider) sweepRecentSends() {
	p.recentSendersMux.Lock()
	defer p.recentSendersMux.Unlock()
	p.sweepRecentSendsLocked()
}

// slotSupersededBy reports whether a send strictly newer than gen has already been accepted for the
// (from, nonce) slot. It makes SendRawTransaction's acceptance-time retirement durable: that retirement
// scans only the cache, so it finds nothing while the predecessor's own fetch-back is still in flight,
// and if the replacement's fetch-back then fails (the Blink drop-mode case) no cache entry ever exists to
// order the predecessor against - which left it surfaced as pending until the cache timeout (#1573).
func (p *AlternativeSendTxProvider) slotSupersededBy(from ethcommon.Address, nonce uint64, gen uint64) bool {
	slot := nonceSlot{addr: from, nonce: nonce}
	p.recentSendersMux.Lock()
	defer p.recentSendersMux.Unlock()
	s, found := p.acceptedSlots[slot]
	if !found {
		return false
	}
	if time.Since(s.time) > p.mempoolTxsTimeout {
		delete(p.acceptedSlots, slot)
		return false
	}
	return s.gen > gen
}

// useForNonces reports whether nonce and gas-estimate lookups for addr should be routed to the
// alternative provider. Only an address that sent through it within the routing window (see
// routingTimeout) can have a pending transaction the primary RPC does not know about AND no cache entry
// carrying it; for everybody else the primary is authoritative and the provider round-trip is pure waste
// of its rate-limit quota. Accepted limitations: a restart wipes the map, a transaction still pending
// past the window - after which the cache and the pending-nonce floor, not the relay, keep its nonce
// reserved - and sends accepted outside this instance, including by another replica in a load-balanced
// deployment without request affinity (see docs/env.md).
func (p *AlternativeSendTxProvider) useForNonces(addr ethcommon.Address) bool {
	p.recentSendersMux.Lock()
	defer p.recentSendersMux.Unlock()
	s, found := p.recentSenders[addr]
	if !found {
		return false
	}
	if time.Since(s.time) > p.routingTimeout() {
		delete(p.recentSenders, addr)
		return false
	}
	return true
}

// releaseRecentSender drops the sender's nonce-routing entry once its last cached transaction left the
// cache (mined, superseded, replaced or timed out), so address polling stops consuming the provider's
// quota before the routing window ends. The entry is kept when its send generation is newer than the
// evicted transaction's: the sender submitted again after that transaction was cached (even within the
// same wall-clock second) and the newer transaction may have no cache entry of its own. Residual risk,
// accepted: an UNCACHED send OLDER than the evicted transaction cannot be represented and loses its
// routing here - it needs a failed fetch-back, and sequential nonces mean an older transaction cannot
// still be pending once a newer one mined.
func (p *AlternativeSendTxProvider) releaseRecentSender(sender ethcommon.Address, evictedTxGen uint64) {
	p.recentSendersMux.Lock()
	defer p.recentSendersMux.Unlock()
	s, found := p.recentSenders[sender]
	if !found {
		return
	}
	if s.gen > evictedTxGen {
		return
	}
	delete(p.recentSenders, sender)
}

// cachedNoncesFor returns the account nonces of the private transactions the cache holds for addr.
// Decoding through txSenderAndNonce keeps a body with no sender out of the answer instead of folding it
// into the zero address's floor.
func (p *AlternativeSendTxProvider) cachedNoncesFor(addr ethcommon.Address) map[uint64]struct{} {
	nonces := make(map[uint64]struct{})
	p.mempoolTxsMux.Lock()
	defer p.mempoolTxsMux.Unlock()
	for _, storedTx := range p.mempoolTxs {
		from, nonce, ok := storedTx.slot()
		if !ok || from != addr {
			continue
		}
		nonces[nonce] = struct{}{}
	}
	return nonces
}

// raiseToPendingFloor advances the backend's pending nonce across the CONTIGUOUS run of cached private
// nonces that starts at it, and reports whether the cache also holds a nonce above that run. Blockbook
// exposes the cached txs as pending, so answering below the run would hand a wallet the nonce of one
// that is in flight. Answering above a hole is the opposite failure and the one #1675 describes: the
// cache can hold N+1 while nothing fills N - an accepted send that never reached the cache, or a reorg
// re-exposing the slot - and a wallet given N+2 there queues every later send behind a nonce that may
// never be consumed, which is all a blind max(cached)+1 floor could answer. The walk yields
// pending <= floor <= pending+len(cached), always consistent with what Blockbook itself displays.
//
// The trade this accepts: an under-reporting backend now lowers the answer where max(cached)+1 rode
// over it. That input is indistinguishable from a real hole, and costs one collision that resolves in
// a block against a wallet stranded for the whole cache retention.
func (p *AlternativeSendTxProvider) raiseToPendingFloor(addr ethcommon.Address, pending uint64) (uint64, bool) {
	nonces := p.cachedNoncesFor(addr)
	floor := pending
	for {
		if _, cached := nonces[floor]; !cached {
			break
		}
		if floor == math.MaxUint64 {
			break
		}
		floor++
	}
	for nonce := range nonces {
		if nonce > floor {
			return floor, true
		}
	}
	return floor, false
}

// handleMempoolTransaction fetches the transaction back from the alternative providers and caches it -
// one ask; exposeAcceptedSend is the retrying send-path wrapper. A failed lookup (the provider error)
// is returned distinct from the relay answering null (bchain.ErrTxNotFound), because per the relay's
// contract an error means retry-the-poll while only a null is a statement about the transaction; the
// callers meter the two differently. gen must be THIS submission's generation rather than the sender's
// current one, which a concurrent send can have bumped during the round-trip: stamping the newer one
// would let this entry's eviction release the sender's routing while that newer transaction is still
// pending.
func (p *AlternativeSendTxProvider) handleMempoolTransaction(txid string, gen uint64) (string, error) {
	tx, found, err := p.getTransactionFromProviders(txid)
	if err != nil {
		return txid, err
	}
	if !found {
		return txid, bchain.ErrTxNotFound
	}
	if tx.BlockNumber != "" {
		// Already mined: block sync owns it from here. Caching it would surface it as pending.
		glog.Infof("eth_getTransactionByHash from alternative providers returned mined txid %s", txid)
		return txid, nil
	}

	p.cacheMempoolTransaction(txid, tx, gen)

	return txid, nil
}

// exposeFetchBackAttempts and exposeFetchBackRetryDelay bound how long exposeAcceptedSend keeps
// re-asking the relay. No cache entry exists on that path, so nothing ever re-asks after it gives up -
// reconcile walks only cached entries. The relay answers from one consistent store, so the first ask
// should already surface an accepted send; the retries absorb a transient lookup error, which the
// relay's contract says means retry, never gone. Four asks over ~30s also stay inside the 96s
// revert-protection retention, the shortest window an accepted send can have.
const (
	exposeFetchBackAttempts   = 4
	exposeFetchBackRetryDelay = 10 * time.Second
)

// exposeAcceptedSend fetches an accepted send back from the relay and caches it, re-asking up to
// exposeFetchBackAttempts times. Reached only when the raw hex did not decode, which makes this
// fetch-back the only thing that can expose the transaction - so unlike the probe path, a final
// failure means the send is exposed NOWHERE (not served, not indexed, floor not raised), for
// not_found and error exactly as for dropped. The failure is observed once, at the final attempt,
// not per ask.
func (p *AlternativeSendTxProvider) exposeAcceptedSend(txid string, gen uint64) {
	for attempt := 1; ; attempt++ {
		_, err := p.handleMempoolTransaction(txid, gen)
		if err == nil {
			return
		}
		if attempt < exposeFetchBackAttempts && !p.stopped() {
			select {
			case <-p.stop:
				return
			case <-time.After(p.exposeRetryDelay()):
			}
			continue
		}
		if err == bchain.ErrTxNotFound {
			p.observeSendNotSurfaced("not_found")
			glog.Errorf("eth_getTransactionByHash from alternative providers did not find accepted send %s; it is exposed nowhere", txid)
		} else {
			p.observeSendNotSurfaced("error")
			glog.Errorf("eth_getTransactionByHash from alternative providers returned error %v for accepted send %s; it is exposed nowhere", err, txid)
		}
		return
	}
}

// exposeRetryDelay is exposeFetchBackRetryDelay unless a test shortens it through the struct field.
func (p *AlternativeSendTxProvider) exposeRetryDelay() time.Duration {
	if p.exposeFetchBackDelay > 0 {
		return p.exposeFetchBackDelay
	}
	return exposeFetchBackRetryDelay
}

// normalizeTxid canonicalizes a transaction hash to the form the cache is keyed on - 0x-prefixed
// lower-case, the encoding of tx.Hash().Hex() the send path inserts under. The cache is authoritative
// for a relay-accepted send (the primary RPC does not know it), so a lookup or removal arriving in any
// other spelling - upper case, missing prefix - would read as the transaction not existing anywhere,
// which is the exposed-nowhere outcome this subsystem exists to prevent. Applied at every boundary
// that takes a caller-supplied txid, so no single call site has to remember it.
func normalizeTxid(txid string) string {
	return ethcommon.HexToHash(txid).Hex()
}

// cacheMempoolTransaction caches a pending transaction body under txid and indexes it in the wrapped
// Blockbook mempool, ordering itself against concurrent sends for the same (from, nonce) slot through
// the send generation gen. Reached from the send path, and from handleMempoolTransaction when the
// signed bytes did not decode - there txid is the relay's echo, which normalizeTxid folds onto the
// cache's canonical key form.
func (p *AlternativeSendTxProvider) cacheMempoolTransaction(txid string, tx *bchain.RpcTransaction, gen uint64) {
	txid = normalizeTxid(txid)
	from, nonce, decoded := txSenderAndNonce(tx)

	// checked before the cache scan below: the scan only sees slots that produced an entry
	if decoded && p.slotSupersededBy(from, nonce, gen) {
		return
	}

	if !p.insertMempoolTx(txid, tx, gen, from, nonce, decoded) {
		// a newer replacement already occupies this nonce slot
		return
	}

	// Retire any cached predecessor sharing this tx's sender and nonce. The send path already does
	// this at acceptance time (see SendRawTransaction, #1573); repeating it here is a no-op in that
	// flow and covers the decode-failure path.
	if decoded {
		p.evictReplacedByNonce(from, nonce, txid, gen)
	}

	if p.mempool != nil {
		p.mempool.AddTransactionToMempool(txid)
		// A concurrent higher-generation send for this slot can evict txid from both stores during the
		// add above, which then re-inserts it into the wrapped mempool only - and reconcile walks just
		// the provider cache, so that orphan would linger as "Unconfirmed" until the wrapped mempool
		// sweep (#1573). The lock covers only a map read, never a call.
		p.mempoolTxsMux.Lock()
		_, stillCached := p.mempoolTxs[txid]
		p.mempoolTxsMux.Unlock()
		if !stillCached {
			p.mempool.RemoveTransactionFromMempool(txid)
		}
	}
}

// insertMempoolTx inserts the entry unless a strictly newer send for the same (from, nonce) slot has
// already cached its own, reporting whether it inserted. Deliberately a separate function so the unlock
// is deferred: a panic leaving mempoolTxsMux held would deadlock every send, read, reconcile and
// nonce-floor lookup instead of crashing the process, since the fetch-back goroutine recovers panics.
func (p *AlternativeSendTxProvider) insertMempoolTx(txid string, tx *bchain.RpcTransaction, gen uint64, from ethcommon.Address, nonce uint64, decoded bool) bool {
	p.mempoolTxsMux.Lock()
	defer p.mempoolTxsMux.Unlock()
	if p.mempoolTxs == nil {
		p.mempoolTxs = make(map[string]storedTx)
	}
	// Skip a stale insert: a concurrent higher-generation send for this slot may already have cached its
	// replacement, and inserting this older submission would surface a second pending tx for the nonce
	// and let the caller's eviction drop the newer one. The comparison must stay the plain `>` that
	// evictReplacedByNonce applies, or a gen-0 submission neither skips here nor evicts there.
	if decoded {
		for otherTxid, st := range p.mempoolTxs {
			if otherTxid == txid {
				continue
			}
			if f, n, ok := st.slot(); ok && f == from && n == nonce && st.gen > gen {
				return false
			}
		}
	}
	p.mempoolTxs[txid] = storedTx{tx: tx, time: uint32(time.Now().Unix()), gen: gen, from: from, nonce: nonce, decoded: decoded}
	return true
}

// slot returns the nonce slot the entry fills, from the copy decoded at insert. It falls back to
// decoding the body for an entry that did not come through insertMempoolTx - only tests build those, and
// the fallback is what keeps them equivalent to a real one rather than silently invisible to every scan.
// A body carrying no sender decodes to ok=false either way, so the two paths never disagree.
func (st storedTx) slot() (ethcommon.Address, uint64, bool) {
	if st.decoded {
		return st.from, st.nonce, true
	}

	return txSenderAndNonce(st.tx)
}

// txSenderAndNonce decodes the sender address and account nonce of a cached RPC transaction, reporting
// ok=false when either is missing or unparsable so callers skip the entry rather than act on a zero value.
func txSenderAndNonce(tx *bchain.RpcTransaction) (ethcommon.Address, uint64, bool) {
	if tx == nil || tx.From == "" || tx.AccountNonce == "" {
		return ethcommon.Address{}, 0, false
	}
	nonce, err := hexutil.DecodeUint64(tx.AccountNonce)
	if err != nil {
		return ethcommon.Address{}, 0, false
	}
	return ethcommon.HexToAddress(tx.From), nonce, true
}

// evictReplacedByNonce retires EVERY cached transaction sharing (from, nonce) with the newly accepted
// (keepTxid, keepGen) - a replacement for a slot means the others can never mine - and counts each exit
// as rbf_replaced. Matching is by decoded address and numeric nonce, so relay differences in hex casing
// cannot hide a predecessor. A strictly higher generation is left intact, so an older submission's slow
// fetch-back cannot evict the newer replacement; keepGen 0 (unknown send order) therefore evicts only
// other generation-0 entries. See docs/evm-send.md for why all matches, not just the first.
func (p *AlternativeSendTxProvider) evictReplacedByNonce(from ethcommon.Address, nonce uint64, keepTxid string, keepGen uint64) {
	// Collect every victim, then remove them after unlocking: removeMempoolTx re-acquires this same
	// non-reentrant mutex, so removing inside the scan would deadlock the provider.
	type victim struct {
		txid string
		time uint32
	}
	var victims []victim
	p.mempoolTxsMux.Lock()
	for txid, storedTx := range p.mempoolTxs {
		if txid == keepTxid {
			continue
		}
		cachedFrom, cachedNonce, ok := storedTx.slot()
		if !ok || cachedFrom != from || cachedNonce != nonce {
			continue
		}
		// see the doc comment; insertMempoolTx's staleness check uses the same comparison, so the two
		// rules cannot disagree about a slot
		if storedTx.gen > keepGen {
			continue
		}
		victims = append(victims, victim{txid: txid, time: storedTx.time})
	}
	p.mempoolTxsMux.Unlock()

	for _, v := range victims {
		glog.Infof("eth_sendRawTransaction replacing txid %s by %s", v.txid, keepTxid)
		// Meter the exit only if this call is the one that removed the predecessor: the acceptance-time
		// pass, the cacheMempoolTransaction pass and a concurrent reconcile eviction can all target it.
		if p.removeMempoolTx(v.txid) {
			p.observeMempoolReconciliation("rbf_replaced")
			p.observeMempoolTxResidence("rbf_replaced", v.time)
		}
	}
}

// GetTransaction gets a transaction from alternative mempool cache. The txid is normalized to the
// cache's key form first: the caller's spelling comes off the wire, and a case-mismatch miss here
// falls through to the primary RPC, which by design does not have the private transaction.
func (p *AlternativeSendTxProvider) GetTransaction(txid string) (*bchain.RpcTransaction, bool) {
	if !p.fetchMempoolTx {
		return nil, false
	}
	txid = normalizeTxid(txid)

	var storedTx storedTx
	var found bool

	p.mempoolTxsMux.Lock()
	storedTx, found = p.mempoolTxs[txid]
	p.mempoolTxsMux.Unlock()

	if found {
		if time.Unix(int64(storedTx.time), 0).Before(time.Now().Add(-p.mempoolTxsTimeout)) {
			// The reconcile loop's staleness timeout, reached on the read path first. It goes through
			// removeMempoolTx so the wrapped mempool's address index is cleared too, or the expired tx
			// stays listed as pending whenever the caller's own primary lookup errors instead of
			// returning null.
			if p.removeMempoolTx(txid) {
				p.observeMempoolReconciliation("timeout")
				p.observeMempoolTxResidence("timeout", storedTx.time)
			}
			return nil, false
		}
		if storedTx.tx == nil {
			return nil, false
		}
		// A copy, never the cached body: the caller passes it to EthTxToTx with fixEIP55=true, which
		// rewrites From and To in place holding no lock - a data race against every reader here.
		// RpcTransaction is all strings, so this shallow copy is complete; deep-copy any
		// reference-typed field added later.
		body := *storedTx.tx
		return &body, true
	}

	return nil, false
}

func (p *AlternativeSendTxProvider) watchMempoolTxs() {
	ticker := time.NewTicker(alternativeMempoolTxCheckPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-p.stop:
			return
		case <-ticker.C:
			p.reconcileMempoolTxs()
		}
	}
}

// shutdown stops the background mempool reconciliation goroutine. Safe on a nil receiver and repeated.
func (p *AlternativeSendTxProvider) shutdown() {
	if p == nil || p.stop == nil {
		return
	}
	p.stopOnce.Do(func() { close(p.stop) })
	p.drainBackground()
}

// drainBackground waits out the fetch-backs already in flight when shutdown was requested, so none of
// them is still running when the caller tears down what they write to: a fetch-back reaching
// cacheMempoolTransaction pushes a NewTx through the wrapped mempool, and returning before that lands
// puts the push after the public server has closed and on into the deferred database close.
//
// Bounded rather than unconditional, because closing the shutdown channel does not stop a probe: an
// HTTP rpc.Client's Close is a no-op, so a request already issued runs to its own rpcTimeout whatever
// happens here. The bound is that timeout plus a margin, which is how long the last probe can take;
// past it the work is abandoned, since a shutdown that does not finish is worse than a lost probe.
func (p *AlternativeSendTxProvider) drainBackground() {
	deadline := time.Now().Add(p.rpcTimeout + backgroundDrainMargin)
	for {
		p.backgroundMux.Lock()
		inFlight := p.backgroundCount + p.exposeCount
		p.backgroundMux.Unlock()
		if inFlight == 0 {
			return
		}
		if time.Now().After(deadline) {
			glog.Warningf("shutdown: abandoning %d alternative-provider fetch-backs still in flight", inFlight)
			return
		}
		time.Sleep(backgroundDrainPoll)
	}
}

func (p *AlternativeSendTxProvider) reconcileMempoolTxs() {
	type cachedTx struct {
		txid string
		tx   storedTx
	}

	p.sweepRecentSends()

	p.mempoolTxsMux.Lock()
	txs := make([]cachedTx, 0, len(p.mempoolTxs))
	for txid, tx := range p.mempoolTxs {
		txs = append(txs, cachedTx{txid: txid, tx: tx})
	}
	p.mempoolTxsMux.Unlock()

	// memoize confirmed-nonce lookups per sender so each sender is queried at most once per cycle
	confirmedNonces := make(map[string]uint64)
	confirmedNonceFailed := make(map[string]bool)

	for _, tx := range txs {
		age := time.Since(time.Unix(int64(tx.tx.time), 0))
		// a freshly submitted tx was cached by the send path moments ago and its own fetch-back is
		// probing the relay; re-asking within the first check period only spends relay quota
		if age < alternativeMempoolTxCheckPeriod {
			p.observeMempoolReconciliation("skipped_fresh")
			continue
		}
		// an entry that has already waited is re-asked less often (see probeInterval); the timeout
		// eviction below is not gated on it, so a backed-off entry still leaves on schedule
		timedOut := time.Unix(int64(tx.tx.time), 0).Before(time.Now().Add(-p.mempoolTxsTimeout))
		if !timedOut && tx.tx.lastProbe != 0 && time.Since(time.Unix(int64(tx.tx.lastProbe), 0)) < probeInterval(age) {
			p.observeMempoolReconciliation("skipped_backoff")
			continue
		}
		p.markProbed(tx.txid, tx.tx)
		known, mined, err := p.providerKnowsTransaction(tx.txid)
		if err != nil {
			glog.Warningf("eth_getTransactionByHash from alternative provider failed for %s: %v", tx.txid, err)
			if timedOut {
				p.evictMempoolTx("timeout", tx.txid, tx.tx.time)
				continue
			}
			p.observeMempoolReconciliation("provider_error")
			continue
		}
		if mined {
			p.evictMempoolTx("mined", tx.txid, tx.tx.time)
			continue
		}

		// The provider answered without error and the tx is not mined. A nonce already consumed by a
		// different transaction (e.g. a replacement submitted outside Blockbook) means this one can never
		// mine, so evict it deterministically rather than wait for the timeout - whether or not the
		// provider still surfaces it, since a spent nonce is a positive, irreversible on-chain fact. Only
		// nonces strictly below the confirmed account nonce count; equal or higher are still mineable.
		if p.transactionSupersededByNonce(tx.tx.tx, confirmedNonces, confirmedNonceFailed) {
			p.evictMempoolTx("nonce_superseded", tx.txid, tx.tx.time)
			continue
		}

		if !known {
			// The relay answers eth_getTransactionByHash from one consistent store over the whole
			// pending window (the Blinklabs alignment that closed #1573's root cause), so a null is no
			// longer the non-authoritative "stopped surfacing" it used to be: a run of nulls outlasting
			// missingTimeout means the tx was dropped or cancelled - a drop-mode cancel leaves no
			// replacement behind to retire it - and the entry is evicted, releasing its nonce slot.
			// Requiring a short run rather than a single null absorbs a transient relay fluke; a relay
			// without that consistency is accommodated by configuring alternativeMissingTxTimeout at
			// the pending window, which restores timeout-only eviction.
			// An entry at the cache timeout leaves as timeout whatever the final probe answered:
			// provider_missing is reserved for the missing-run rule, so its residence reads as "how
			// long after the drop", and the retention boundary - where Blink's 3h window makes a
			// final null the EXPECTED answer for a tx stuck the whole window - does not pollute it.
			if timedOut {
				p.evictMempoolTx("timeout", tx.txid, tx.tx.time)
				continue
			}
			missingSince := p.markMissing(tx.txid, tx.tx)
			if missingSince != 0 && time.Since(time.Unix(int64(missingSince), 0)) >= p.missingTimeout() {
				p.evictMempoolTx("provider_missing", tx.txid, tx.tx.time)
				continue
			}
			p.observeMempoolReconciliation("provider_missing_pending")
			continue
		}
		// surfaced (again): a transient gap must not accumulate toward the missing eviction
		p.clearMissing(tx.txid, tx.tx)

		if timedOut {
			p.evictMempoolTx("timeout", tx.txid, tx.tx.time)
			continue
		}
		p.observeMempoolReconciliation("kept")
	}

	p.mempoolTxsMux.Lock()
	size := len(p.mempoolTxs)
	var oldest uint32
	for _, st := range p.mempoolTxs {
		if oldest == 0 || st.time < oldest {
			oldest = st.time
		}
	}
	p.mempoolTxsMux.Unlock()
	p.setMempoolCacheSize(size)
	p.setMempoolOldestAge(oldest)
}

// markProbed stamps this cycle onto the cache entry so probeInterval can pace the next probe. It updates
// only an entry still identical to the snapshot the loop is working from, so a concurrent eviction is
// never undone and a replacement cached in the meantime never inherits its predecessor's probe schedule.
// The tx body is carried over untouched (see storedTx).
func (p *AlternativeSendTxProvider) markProbed(txid string, snapshot storedTx) {
	p.mempoolTxsMux.Lock()
	defer p.mempoolTxsMux.Unlock()
	current, found := p.mempoolTxs[txid]
	if !found || current.gen != snapshot.gen || current.time != snapshot.time {
		return
	}
	current.lastProbe = uint32(time.Now().Unix())
	p.mempoolTxs[txid] = current
}

// markMissing stamps the start of the entry's current run of null relay answers - keeping an already
// running one - and returns the run's start, 0 when the entry is gone or replaced. Guarded like
// markProbed, so a replacement cached in the meantime never inherits its predecessor's run.
func (p *AlternativeSendTxProvider) markMissing(txid string, snapshot storedTx) uint32 {
	p.mempoolTxsMux.Lock()
	defer p.mempoolTxsMux.Unlock()
	current, found := p.mempoolTxs[txid]
	if !found || current.gen != snapshot.gen || current.time != snapshot.time {
		return 0
	}
	if current.missingSince == 0 {
		current.missingSince = uint32(time.Now().Unix())
		p.mempoolTxs[txid] = current
	}
	return current.missingSince
}

// clearMissing resets the entry's missing run once the relay surfaces the tx again, so transient gaps
// do not accumulate toward the missing eviction. Guarded like markProbed.
func (p *AlternativeSendTxProvider) clearMissing(txid string, snapshot storedTx) {
	p.mempoolTxsMux.Lock()
	defer p.mempoolTxsMux.Unlock()
	current, found := p.mempoolTxs[txid]
	if !found || current.gen != snapshot.gen || current.time != snapshot.time || current.missingSince == 0 {
		return
	}
	current.missingSince = 0
	p.mempoolTxs[txid] = current
}

func (p *AlternativeSendTxProvider) observeMempoolReconciliation(action string) {
	if p.metrics == nil || p.metrics.EthAlternativeMempoolEvents == nil {
		return
	}
	p.metrics.EthAlternativeMempoolEvents.With(common.Labels{"action": action}).Inc()
}

// evictMempoolTx removes the cache entry and, only when this call actually removed it, records the
// terminal reconcile decision plus the entry's residence: reconcile works off a cycle-start snapshot, so
// the read path or a concurrent RBF eviction may already have removed and metered the same entry.
func (p *AlternativeSendTxProvider) evictMempoolTx(action, txid string, addedUnix uint32) {
	if !p.removeMempoolTx(txid) {
		return
	}
	p.observeMempoolReconciliation(action)
	p.observeMempoolTxResidence(action, addedUnix)
}

// observeMempoolTxResidence records the age of a cache entry (seconds since it was broadcast) at the
// moment it is evicted, labeled by the deciding action, making the non-deterministic lifetime of an
// unconfirmed tx visible per reason - provider_missing clustering within minutes rather than near the
// timeout is the premature-eviction regression #1573 describes.
func (p *AlternativeSendTxProvider) observeMempoolTxResidence(action string, addedUnix uint32) {
	if p.metrics == nil || p.metrics.EthAlternativeMempoolTxResidence == nil {
		return
	}
	residence := time.Since(time.Unix(int64(addedUnix), 0)).Seconds()
	if residence < 0 {
		residence = 0
	}
	p.metrics.EthAlternativeMempoolTxResidence.With(common.Labels{"action": action}).Observe(residence)
}

// setMempoolCacheSize records the current depth of the alternative send-tx mempool cache.
func (p *AlternativeSendTxProvider) setMempoolCacheSize(size int) {
	if p.metrics == nil || p.metrics.EthAlternativeMempoolCacheSize == nil {
		return
	}
	p.metrics.EthAlternativeMempoolCacheSize.Set(float64(size))
}

// setMempoolOldestAge records how long the oldest cached entry has lived (seconds since broadcast), or 0
// when the cache is empty. A value climbing toward the cache timeout at non-zero depth is the live
// stuck-tx signal the exit-only residence histogram cannot show until an entry finally times out.
func (p *AlternativeSendTxProvider) setMempoolOldestAge(oldestUnix uint32) {
	if p.metrics == nil || p.metrics.EthAlternativeMempoolOldestAge == nil {
		return
	}
	var age float64
	if oldestUnix != 0 {
		if a := time.Since(time.Unix(int64(oldestUnix), 0)).Seconds(); a > 0 {
			age = a
		}
	}
	p.metrics.EthAlternativeMempoolOldestAge.Set(age)
}

// observeSendNotSurfaced counts a relay-accepted private send whose post-send fetch-back did not surface
// it. What a reason means depends on which fetch-back observed it. On the probe path (the raw hex
// decoded and the send is already cached from its signed bytes) not_found and error only mean the relay
// does not report back what it accepted; a relay that keeps not surfacing it retires the entry through
// the missing eviction (see reconcileMempoolTxs), with the cache timeout as the backstop. On the expose
// path (the raw hex did not decode, see exposeAcceptedSend) every reason means the send is exposed
// nowhere: not_found and error after the retries run out, dropped when the fetch-back was refused under
// load and never ran at all.
func (p *AlternativeSendTxProvider) observeSendNotSurfaced(reason string) {
	if p.metrics == nil || p.metrics.EthAlternativeSendNotSurfaced == nil {
		return
	}
	p.metrics.EthAlternativeSendNotSurfaced.With(common.Labels{"reason": reason}).Inc()
}

// transactionSupersededByNonce reports whether a different transaction has already consumed the cached
// transaction's nonce, making it permanently unmineable. resolved/failed memoize the lookups per sender.
func (p *AlternativeSendTxProvider) transactionSupersededByNonce(tx *bchain.RpcTransaction, resolved map[string]uint64, failed map[string]bool) bool {
	if tx == nil || tx.From == "" || tx.AccountNonce == "" {
		return false
	}
	txNonce, err := hexutil.DecodeUint64(tx.AccountNonce)
	if err != nil {
		glog.Warningf("alternative mempool: cannot parse nonce %q for tx %s: %v", tx.AccountNonce, tx.Hash, err)
		return false
	}
	from := strings.ToLower(tx.From)
	confirmed, ok := resolved[from]
	if !ok {
		if failed[from] {
			return false
		}
		confirmed, err = p.getConfirmedNonce(tx.From)
		if err != nil {
			// keep the transaction on lookup failure; the timeout path remains the safety net
			failed[from] = true
			return false
		}
		resolved[from] = confirmed
	}
	return txNonce < confirmed
}

// getConfirmedNonce returns the number of transactions mined from the address at the latest block, i.e.
// the lowest nonce not yet consumed on-chain. It queries every configured provider and returns the most
// conservative (lowest) value so a lagging or misbehaving provider cannot get a still mineable
// transaction evicted. The "latest" tag carries the usual chain-tip caveat: a nonce consumed only in a
// tip block later reorged out makes the eviction premature, bounded as with the mined-tx removal above -
// eviction drops only the cache entry, and a still-valid tx is re-indexed when it is actually mined.
func (p *AlternativeSendTxProvider) getConfirmedNonce(from string) (uint64, error) {
	address := ethcommon.HexToAddress(from)
	var lowest uint64
	var found bool
	var firstErr error
	for _, url := range p.urls {
		result, err := p.callHttpStringResult(url, "eth_getTransactionCount", address, "latest")
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		nonce, err := hexutil.DecodeUint64(result)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if !found || nonce < lowest {
			lowest = nonce
			found = true
		}
	}
	if !found {
		if firstErr == nil {
			firstErr = errors.New("no alternative provider returned a confirmed nonce")
		}
		return 0, firstErr
	}
	return lowest, nil
}

func (p *AlternativeSendTxProvider) providerKnowsTransaction(txid string) (bool, bool, error) {
	tx, found, err := p.getTransactionFromProviders(txid)
	if err != nil || !found {
		return found, false, err
	}
	return true, tx.BlockNumber != "", nil
}

func (p *AlternativeSendTxProvider) getTransactionFromProviders(txid string) (*bchain.RpcTransaction, bool, error) {
	hash := ethcommon.HexToHash(txid)
	var firstErr error
	for _, url := range p.urls {
		raw, err := p.callHttpRawResult(url, "eth_getTransactionByHash", hash)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		var tx bchain.RpcTransaction
		if err := json.Unmarshal(raw, &tx); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if tx.Hash == "" {
			continue
		}
		return &tx, true, nil
	}
	if firstErr != nil {
		return nil, false, firstErr
	}
	return nil, false, nil
}

// removeMempoolTx evicts txid from the provider cache and, when a delegate is wired, from the wrapped
// Blockbook mempool too, reporting whether the entry was present. The cache delete runs first and is the
// single point of truth: concurrent reconcile / read-path / RBF evictions of the same txid see false for
// all but the one that deleted it.
func (p *AlternativeSendTxProvider) removeMempoolTx(txid string) bool {
	// action "" - the caller meters this exit under its own reconcile decision
	removed := p.removeTransaction(txid, "")
	if removed && p.removeTransactionFromMempool != nil {
		p.removeTransactionFromMempool(txid)
	}
	return removed
}

// RemoveTransaction removes a transaction from the alternative mempool cache. It is the entry point for
// removals carrying no reconcile decision of their own - block sync indexing a mined transaction, the
// read path finding one mined or unknown - and meters them as sync_removed, which nothing else would.
// In practice block sync is the sole source: the read path serves cached entries without asking the
// node, so its mined/unknown branches run only on a cache miss, where there is nothing to remove. Reached again as removeMempoolTx's
// delegate, where the entry is already gone, so nothing is metered twice.
func (p *AlternativeSendTxProvider) RemoveTransaction(txid string) bool {
	return p.removeTransaction(txid, "sync_removed")
}

// removeTransaction removes a transaction from the alternative mempool cache. When the removed
// transaction was the sender's last cached one, the sender's nonce-routing entry is released as well
// (see releaseRecentSender). It reports whether the entry was actually present, so callers can meter the
// exit exactly once (see evictMempoolTx); a non-empty action meters it here instead.
func (p *AlternativeSendTxProvider) removeTransaction(txid string, action string) bool {
	if !p.fetchMempoolTx {
		return false
	}
	// normalized like GetTransaction: block sync and the read path hand over the caller's spelling
	txid = normalizeTxid(txid)

	p.mempoolTxsMux.Lock()
	removedTx, found := p.mempoolTxs[txid]
	delete(p.mempoolTxs, txid)
	senderSettled := false
	var sender ethcommon.Address
	if from, _, ok := removedTx.slot(); found && ok {
		sender = from
		senderSettled = true
		for _, storedTx := range p.mempoolTxs {
			if f, _, ok := storedTx.slot(); ok && f == sender {
				senderSettled = false
				break
			}
		}
	}
	p.mempoolTxsMux.Unlock()

	if senderSettled {
		p.releaseRecentSender(sender, removedTx.gen)
	}
	if found && action != "" {
		p.observeMempoolReconciliation(action)
		p.observeMempoolTxResidence(action, removedTx.time)
	}
	return found
}

// UseOnlyAlternativeProvider returns true if only alternative providers should be used
func (p *AlternativeSendTxProvider) UseOnlyAlternativeProvider() bool {
	return p.onlyAlternative
}

// Helper function for calling ETH RPC over http with parameters. Creates and closes a new client for every call.
func (p *AlternativeSendTxProvider) callHttpRawResult(url string, rpcMethod string, args ...interface{}) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), p.rpcTimeout)
	defer cancel()
	client, err := rpc.DialContext(ctx, url)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	var raw json.RawMessage
	err = client.CallContext(ctx, &raw, rpcMethod, args...)
	if err != nil {
		return nil, err
	} else if len(raw) == 0 {
		return nil, errors.New(url + " " + rpcMethod + " : failed")
	}
	return raw, nil
}

// Helper function for calling ETH RPC over http with parameters and getting string result. Creates and closes a new client for every call.
func (p *AlternativeSendTxProvider) callHttpStringResult(url string, rpcMethod string, args ...interface{}) (string, error) {
	raw, err := p.callHttpRawResult(url, rpcMethod, args...)
	if err != nil {
		return "", err
	}
	var result string
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", errors.Annotatef(err, "%s %s raw result %v", url, rpcMethod, raw)
	}
	if result == "" {
		return "", errors.New(url + " " + rpcMethod + " : failed, empty result")
	}
	return result, nil
}

// nonceURL returns the provider URL to use for addr's nonce lookup: the URL that accepted the sender's
// most recent transaction when known (a broadcast succeeds if ANY configured provider accepts it, so the
// first URL may never have seen the transaction), falling back to the first configured URL.
func (p *AlternativeSendTxProvider) nonceURL(addr ethcommon.Address) string {
	p.recentSendersMux.Lock()
	defer p.recentSendersMux.Unlock()
	if s, found := p.recentSenders[addr]; found && s.url != "" {
		return s.url
	}
	return p.urls[0]
}

// getNonces returns the pending account nonce from the alternative provider that accepted the sender's
// most recent transaction (see nonceURL), plus the confirmed (latest) nonce when withConfirmed is set,
// both in a single JSON-RPC batch round-trip. The confirmed nonce is best-effort: a failed latest lookup
// yields confirmedOK=false (not an error) so the caller can omit it. An error is returned only when the
// required pending nonce cannot be obtained.
func (p *AlternativeSendTxProvider) getNonces(addr ethcommon.Address, withConfirmed bool) (uint64, uint64, bool, error) {
	if len(p.urls) == 0 {
		return 0, 0, false, errors.New("no alternative provider url configured")
	}
	url := p.nonceURL(addr)
	if !withConfirmed {
		pendingHex, err := p.callHttpStringResult(url, "eth_getTransactionCount", addr, "pending")
		if err != nil {
			return 0, 0, false, err
		}
		pending, err := hexutil.DecodeUint64(pendingHex)
		if err != nil {
			return 0, 0, false, errors.Annotatef(err, "pending nonce %q", pendingHex)
		}
		return pending, 0, false, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), p.rpcTimeout)
	defer cancel()
	client, err := rpc.DialContext(ctx, url)
	if err != nil {
		return 0, 0, false, err
	}
	defer client.Close()
	var pendingHex, confirmedHex string
	batch := []rpc.BatchElem{
		{Method: "eth_getTransactionCount", Args: []interface{}{addr, "pending"}, Result: &pendingHex},
		{Method: "eth_getTransactionCount", Args: []interface{}{addr, "latest"}, Result: &confirmedHex},
	}
	if err := client.BatchCallContext(ctx, batch); err != nil {
		return 0, 0, false, err
	}
	if batch[0].Error != nil {
		return 0, 0, false, batch[0].Error
	}
	pending, err := hexutil.DecodeUint64(pendingHex)
	if err != nil {
		return 0, 0, false, errors.Annotatef(err, "pending nonce %q", pendingHex)
	}
	confirmed, confirmedOK := decodeConfirmedNonce(addr, confirmedHex, batch[1].Error)
	return pending, confirmed, confirmedOK, nil
}
