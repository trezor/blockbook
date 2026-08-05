package eth

import (
	"context"
	"encoding/json"
	"fmt"
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

// storedTx is one entry of the alternative-provider pending cache.
//
// Its tx body is IMMUTABLE once published into mempoolTxs: nothing writes through the pointer, and
// GetTransaction hands out a copy rather than the body itself. That is what lets the reads under mempoolTxsMux
// (pendingNonceFloor, txSenderAndNonce via insertMempoolTx / evictReplacedByNonce, the senderSettled
// scan in removeTransaction) and the lockless ones outside it (the reconcileMempoolTxs snapshot) be
// race-free, since a body is fully built before it is published and the publication happens under the
// mutex. Anything that mutates a body in place reopens all of them at once.
type storedTx struct {
	tx   *bchain.RpcTransaction
	time uint32
	gen  uint64 // send generation of the submission that created this entry, orders it against later sends
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

// maxBackgroundFetchBacks and maxExposeFetchBacks cap the post-send fetch-backs running at once, per
// kind. The send path used to wait for its own fetch-back, so in-flight fetch-backs were bounded by
// in-flight send requests (and by the websocket server's per-connection pending-request limit); now
// that it does not, this is the backpressure. Private send volume is low, so a cap is only ever
// reached by a burst or by a relay that has stopped answering - and in that second case shedding is
// near-total for as long as it lasts, since each refused slot is held for len(urls) * rpcTimeout and
// only the reconcile loop keeps probing.
//
// The two allowances are independent because a refusal costs the two call sites completely different
// things: a refused refresh loses nothing (the transaction is already cached and indexed, and
// reconcile re-probes it within a cycle), while a refused fetch-back on the raw-hex-decode-failure
// path loses the send itself - nothing serves it, nothing indexes it and the pending-nonce floor does
// not rise. Sharing one allowance let ordinary refresh traffic, which is all of it in practice, starve
// the only fetch-back that carries data.
const (
	maxBackgroundFetchBacks = 16
	maxExposeFetchBacks     = 16
)

// backgroundKind selects which allowance a fetch-back draws on: backgroundRefresh for the ones that
// only replace a cached body, backgroundExpose for the one that is the sole thing able to expose an
// accepted send.
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
}

// NewAlternativeSendTxProvider creates a new alternative send tx provider if enabled
func NewAlternativeSendTxProvider(network string, rpcTimeout int, mempoolTxsTimeout time.Duration, metrics *common.Metrics) *AlternativeSendTxProvider {
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

// SendRawTransaction sends raw transaction to alternative providers.
//
// A wallet waits for this call, and its own deadline is what turns a slow relay into a transaction
// the user is told failed while it is on its way to the chain - after which a re-send at the next
// nonce pays the recipient twice. Two things therefore bound the time spent here: the broadcast to
// every configured relay runs concurrently (below), so the wall time is the slowest single URL
// rather than their sum, and the post-send fetch-back is not waited for at all (see the end of this
// function).
func (p *AlternativeSendTxProvider) SendRawTransaction(hex string) (string, error) {
	var txid string
	var retErr error
	var acceptedURL string

	// decoded once for the log lines and the accepted-send bookkeeping below - a send is the only
	// moment Blockbook sees the sender and nonce of a private transaction, and both are what a
	// later "my tx vanished" or nonce-gap report has to be reconciled against
	sent, decErr := decodeAlternativeSendTx(hex)

	// Broadcasting to every configured relay is deliberate redundancy, but it must not cost
	// len(urls) * rpcTimeout in the worst case: one unresponsive relay used to hold up the answer to
	// the wallet for its full timeout before the next URL was even tried. Results are collected in
	// URL order below, so the aggregation - and which URL counts as the accepting one - is exactly
	// as deterministic as the sequential loop it replaces.
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
			// A relay broadcast must not be able to take the process down; the websocket/HTTP request
			// handler that called us cannot recover a panic raised in this goroutine.
			defer func() {
				if r := recover(); r != nil {
					glog.Errorf("eth_sendRawTransaction to %s panicked: %v", providerLabel(p.urls[i]), r)
				}
			}()
			host := providerLabel(p.urls[i])
			start := time.Now()
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

	// the transaction was decoded once at the top of the send and the result is reused for
	// recent-sender registration, the RBF eviction and the cache entry below - a single ECDSA
	// sender recovery instead of one per consumer. On decode failure gen stays 0 and neither
	// eviction nor local caching runs, but the transaction is still handed to the fetch-back
	// with gen 0, exactly as before.
	var gen uint64
	if decErr != nil {
		glog.Warningf("cannot decode accepted transaction: %v", decErr)
	} else {
		gen = p.registerSuccessfulSend(sent.from, sent.nonce, acceptedURL)
		if !strings.EqualFold(sent.txid, txid) {
			// A relay echoed something other than the hash of the bytes it was given. Report the
			// locally derived hash: it is what the chain will show, and it is what the cache, the
			// address index and the wallet notification below are keyed on, so returning the echo
			// would hand the wallet a txid Blockbook has no entry for.
			glog.Errorf("eth_sendRawTransaction echoed txid %s, signed bytes hash to %s", txid, sent.txid)
		}
		txid = sent.txid
	}

	if p.onlyAlternative && p.fetchMempoolTx {
		// An accepted (from, nonce) means the relay now builds with THIS tx, so retire any cached
		// predecessor for the slot immediately, without waiting for the relay to surface this one:
		// drop-mode cancels are never surfaced and an unsurfaced RBF replacement never reached
		// handleMempoolTransaction's removal, so both left the predecessor "Unconfirmed" until the
		// cache timeout (#1573). Much stronger than an empty getTransactionByHash probe, which says
		// nothing about mineability (see reconcileMempoolTxs), but not proof - a predecessor already
		// forwarded to builders can still mine, and then this dropped the tx that wins (block sync
		// re-indexes it). Still needed even though the replacement is cached right below: that
		// insert can bail out as obsolete/superseded when a newer send already holds the slot, and
		// the predecessor must be retired regardless.
		if decErr == nil {
			p.evictReplacedByNonce(sent.from, sent.nonce, sent.txid, gen)
			// Cache and index the transaction from its own signed bytes, before asking the relay
			// for it: an accepted send must never end up exposed nowhere. The relay is free to
			// stop surfacing (or never surface) a transaction it has accepted, and every such
			// send used to be cached and indexed nowhere at all - not served as pending, not in
			// the address index, and not raising the pending-nonce floor, so the next send could
			// reuse its nonce (the send-not-surfaced precursor). The signed bytes carry
			// everything a pending transaction needs, so this needs no round-trip and cannot fail.
			p.cacheMempoolTransaction(sent.txid, sent.body, gen)
			// Only a probe: it reports whether the relay surfaces what it accepted, and never touches
			// the cache. Nothing the caller can observe depends on it, so it runs in the background
			// rather than making the wallet wait another rpcTimeout per URL. It must not write the
			// cache either: by the time it answers (up to rpcTimeout per URL later, which straddles a
			// block on every chain we index) the transaction may have mined and block sync may have
			// cleared it, and re-inserting the pending body would flip a confirmed transaction back to
			// Unconfirmed, re-index it as pending and count its cache exit twice. Dropping it under
			// load is safe - the entry is already cached and indexed, and reconcileMempoolTxs
			// re-probes it within a cycle anyway.
			p.inBackground(backgroundRefresh, func() { p.probeSentTransaction(sent.txid) })
		} else {
			// Nothing was cached (the raw hex did not decode), so here the fetch-back is the only
			// thing that can expose the transaction at all and keeps its create semantics. Dropping
			// it therefore loses the accepted send: nothing serves it, nothing indexes it and the
			// pending-nonce floor does not rise, which is the nonce-reuse precursor - so report it
			// under the same counter as a relay that does not surface what it accepted, rather than
			// leaving the one unobservable variant of that failure.
			if !p.inBackground(backgroundExpose, func() { p.handleMempoolTransaction(txid, gen) }) && !p.stopped() {
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

// inBackground runs the post-send fetch-back off the send path, so the wallet's answer does not wait
// for another relay round-trip (up to rpcTimeout per configured URL) it cannot observe the outcome of.
// It reports whether it took the work: it declines once the provider is shut down, and drops rather
// than queues beyond maxBackgroundFetchBacks, because the send path no longer applies backpressure and
// a burst of accepted sends would otherwise grow goroutines, sockets and relay-quota consumption
// without bound. Queueing is not an alternative - a queue either grows unbounded or pushes back into
// the wallet's own deadline, and a fetch-back that runs minutes late tells us nothing reconcile has
// not already decided. What a drop costs differs per call site, so the callers decide what to make of
// a false return.
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

// inFlight reports the fetch-backs currently running, per kind.
func (p *AlternativeSendTxProvider) inFlight(kind backgroundKind) int {
	if kind == backgroundExpose {
		return p.exposeCount
	}
	return p.backgroundCount
}

// startBackground reserves one of the maxBackgroundFetchBacks slots, reporting whether the caller got
// one. A plain sync.WaitGroup cannot serve here: Add would race the Wait in waitForRefreshes (the
// documented reuse misuse, which panics and leaves the counter stuck), and it could not cap anything.
func (p *AlternativeSendTxProvider) startBackground(kind backgroundKind) bool {
	if p.stopped() {
		return false
	}
	p.backgroundMux.Lock()
	defer p.backgroundMux.Unlock()
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

// stopped reports whether shutdown has been requested. It keeps a fetch-back declined at shutdown from
// being reported as a lost send: the process is going down, and an alert on the nonce-reuse precursor
// must not fire on every restart that catches a send in flight.
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
// deterministic cache state after a send; production code never has to wait for a refresh.
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

// probeSentTransaction asks the relay about a transaction the send path has already cached from its
// own signed bytes. It only observes: whether the relay reports back what it accepted (counted by
// fetchMempoolTransaction) and whether it already considers the transaction mined.
//
// It deliberately does not touch the cache. Two reasons, both learned the hard way:
//
//   - Adopting the relay's body buys nothing and costs trust. Everything a pending RpcTransaction
//     carries is in the signed bytes, so the relay's view is at best identical - and at worst it
//     carries a different hash, `to` or `value`, which EthTxToTx would then serve to the wallet as the
//     transaction's own identity (it takes txid from tx.Hash). The send path deliberately overrides a
//     relay that echoes the wrong txid; adopting that same echo one round-trip later would undo it.
//   - Evicting on a mined answer is premature. The relay can see the transaction in a block before
//     Blockbook's own block sync has indexed that block, and eviction clears the wrapped mempool's
//     address index too - so between the two the transaction would be in neither store: not pending,
//     not confirmed, absent from its addresses. On a 250 ms-block chain the fetch-back regularly wins
//     that race. Block sync removes it as sync_removed when it indexes the block, and reconcile's
//     mined branch is the backstop; neither can produce the gap, because reconcile only probes an
//     entry that is at least one check period old.
func (p *AlternativeSendTxProvider) probeSentTransaction(txid string) {
	tx, found := p.fetchMempoolTransaction(txid)
	if !found {
		return
	}
	if tx.BlockNumber != "" {
		glog.Infof("eth_getTransactionByHash from alternative providers already reports txid %s mined; leaving it to block sync", txid)
	}
}

// alternativeSendTx is everything the send path derives from an accepted raw transaction: the
// (from, nonce) slot it fills, its txid, and a pending-transaction body to cache for it.
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

// decodeAlternativeSendTx decodes a raw transaction hex once and derives everything the send path
// needs from the signed bytes. The chain id needed to recover the sender is taken from the
// transaction itself.
//
//   - the sender and nonce identify the (from, nonce) slot the send fills, so a cached predecessor
//     for that slot can be retired at acceptance time (see evictReplacedByNonce)
//   - the txid is the hash of the signed bytes, i.e. what the chain will show, independent of what
//     the relay echoes back
//   - the body is a complete pending-transaction body: everything bchain.RpcTransaction carries for
//     an unmined transaction is in the signed bytes, so Blockbook can surface and index an accepted
//     private transaction without the relay having to return it from eth_getTransactionByHash
//     (see cacheMempoolTransaction). Field encoding follows eth_getTransactionByHash: lower-case
//     hex addresses, minimal hex quantities, and gasPrice carrying the fee cap for typed
//     fee-market transactions (as geth reports for a pending one).
func decodeAlternativeSendTx(rawTxHex string) (*alternativeSendTx, error) {
	var tx types.Transaction
	if err := tx.UnmarshalBinary(ethcommon.FromHex(rawTxHex)); err != nil {
		return nil, err
	}
	// LatestSignerForChainID PANICS for a chain id of zero or nil, which is what an unprotected
	// (pre-EIP-155) legacy transaction decodes to - and a panic here would abort the response to a
	// wallet whose transaction the relay has already accepted, the exact failure this path exists to
	// prevent. Such a transaction is Homestead-signed, so derive its sender with that signer.
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

// registerSuccessfulSend records the sender of a transaction accepted by an alternative
// provider so that useForNonces routes the sender's nonce lookups to that provider while
// the transaction may still be pending there. A broadcast succeeds if ANY configured URL
// accepts it, so the accepting URL is recorded too - it is the one provider guaranteed to
// know the transaction (see nonceURL). Expired entries are swept on the way; the map only
// ever holds senders of the last mempoolTxsTimeout window, so the sweep is cheap.
// It returns the send generation assigned to this submission (0 when the sender cannot be
// decoded); the caller must carry that exact value to the cache entry it creates for the
// transaction, so that releaseRecentSender can order evictions against later sends.
// The (sender, nonce) slot this send fills is recorded alongside, so the acceptance survives a
// fetch-back that never produces a cache entry (see slotSupersededBy).
func (p *AlternativeSendTxProvider) registerSuccessfulSend(sender ethcommon.Address, nonce uint64, acceptedURL string) uint64 {
	now := time.Now()
	p.recentSendersMux.Lock()
	defer p.recentSendersMux.Unlock()
	if p.recentSenders == nil {
		p.recentSenders = make(map[ethcommon.Address]recentSender)
	}
	if p.acceptedSlots == nil {
		p.acceptedSlots = make(map[nonceSlot]acceptedSlot)
	}
	for addr, s := range p.recentSenders {
		if now.Sub(s.time) > p.mempoolTxsTimeout {
			delete(p.recentSenders, addr)
		}
	}
	// same horizon and cheap inline sweep as recentSenders above
	for slot, s := range p.acceptedSlots {
		if now.Sub(s.time) > p.mempoolTxsTimeout {
			delete(p.acceptedSlots, slot)
		}
	}
	p.sendGeneration++
	p.recentSenders[sender] = recentSender{time: now, url: acceptedURL, gen: p.sendGeneration}
	p.acceptedSlots[nonceSlot{addr: sender, nonce: nonce}] = acceptedSlot{gen: p.sendGeneration, time: now}
	return p.sendGeneration
}

// slotSupersededBy reports whether a send strictly newer than generation gen has already been
// accepted for the (from, nonce) slot. It makes the acceptance-time retirement in SendRawTransaction
// durable: that retirement scans only the cache, so it finds nothing while the predecessor's own
// fetch-back is still in flight, and if the replacement's fetch-back then fails (the Blink drop-mode
// case) no cache entry ever exists to order the predecessor against - which left it surfaced as
// pending until the cache timeout (#1573).
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

// useForNonces reports whether nonce lookups for addr should be routed to the alternative
// provider. Only addresses that recently (within mempoolTxsTimeout, the same horizon at
// which Blockbook stops surfacing the tx as pending) sent a transaction through it can have
// a pending transaction the primary RPC does not know about; for everybody else the primary
// is authoritative and the provider round-trip is pure waste of its rate-limit quota.
// Senders whose cached transactions have all settled are released before the timeout (see
// releaseRecentSender). Accepted limitations: a restart wipes the map (exposure bounded by
// mempoolTxsTimeout), a transaction pending longer than the timeout, and private
// transactions submitted outside this Blockbook instance - which includes sends accepted
// by another replica in a load-balanced deployment without request affinity (wallet
// websocket flows are naturally sticky to one instance; see docs/env.md).
func (p *AlternativeSendTxProvider) useForNonces(addr ethcommon.Address) bool {
	p.recentSendersMux.Lock()
	defer p.recentSendersMux.Unlock()
	s, found := p.recentSenders[addr]
	if !found {
		return false
	}
	if time.Since(s.time) > p.mempoolTxsTimeout {
		delete(p.recentSenders, addr)
		return false
	}
	return true
}

// releaseRecentSender drops the sender's nonce-routing entry once its last cached
// transaction left the alternative mempool cache (mined, superseded, replaced or timed
// out), so address polling stops consuming the alternative provider's quota as soon as no
// private transaction remains pending. The entry is kept when its send generation is newer
// than the evicted transaction's: the sender submitted again after that transaction was
// cached (even within the same wall-clock second) and the newer transaction may not have a
// cache entry of its own (e.g. when the post-send fetch-back failed).
// Residual risk, accepted: an UNCACHED send OLDER than the evicted transaction cannot be
// represented and loses its routing with the release. It needs a failed fetch-back, and
// mined evictions largely exclude it anyway - the sender's nonces are sequential, so an
// older transaction cannot still be pending once a newer one mined.
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

// pendingNonceFloor returns the lowest pending nonce consistent with the private
// transactions the alternative mempool cache holds for addr (highest cached account nonce
// + 1), and whether any such transaction exists. Blockbook exposes these cached txs as
// pending, so reporting a pending nonce below the floor would contradict its own view and
// lead a wallet to reuse the nonce of an in-flight private transaction.
func (p *AlternativeSendTxProvider) pendingNonceFloor(addr ethcommon.Address) (uint64, bool) {
	p.mempoolTxsMux.Lock()
	defer p.mempoolTxsMux.Unlock()
	var floor uint64
	var found bool
	for _, storedTx := range p.mempoolTxs {
		if storedTx.tx == nil || ethcommon.HexToAddress(storedTx.tx.From) != addr {
			continue
		}
		nonce, err := hexutil.DecodeUint64(storedTx.tx.AccountNonce)
		if err != nil {
			continue
		}
		if nonce+1 > floor {
			floor = nonce + 1
			found = true
		}
	}
	return floor, found
}

// raiseToPendingFloor returns pending, raised to pendingNonceFloor(addr) when the cache
// holds a higher-nonce private transaction for the address.
func (p *AlternativeSendTxProvider) raiseToPendingFloor(addr ethcommon.Address, pending uint64) uint64 {
	if floor, found := p.pendingNonceFloor(addr); found && floor > pending {
		return floor
	}
	return pending
}

// handleMempoolTransaction fetches the transaction back from the alternative providers and caches it.
// Reached only when the send path could not derive the transaction from its own signed bytes (the raw
// hex did not decode), which makes this fetch-back the only thing that can expose the transaction at
// all; everywhere else the entry already exists and the fetch-back only probes the relay.
// gen is the send generation registerSuccessfulSend assigned to THIS submission - it must be
// passed in rather than read from recentSenders here, because the fetch-back is a network
// round-trip during which a concurrent send from the same sender can bump the sender's current
// generation; stamping the cache entry with that newer generation would let its eviction release
// the sender's routing while the newer transaction is still pending.
func (p *AlternativeSendTxProvider) handleMempoolTransaction(txid string, gen uint64) (string, error) {
	tx, found := p.fetchMempoolTransaction(txid)
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

// fetchMempoolTransaction asks the alternative providers for the transaction and reports whether any
// of them returned it. A relay that accepted the send but does not return it is counted under
// observeSendNotSurfaced: the transaction is still cached and indexed from its own signed bytes
// (unless those did not decode - see handleMempoolTransaction), so this is not the nonce-reuse
// precursor it used to be, but it does mean the entry cannot be reconciled against the relay's own
// view of it and only the cache timeout can retire it (#1638 review).
func (p *AlternativeSendTxProvider) fetchMempoolTransaction(txid string) (*bchain.RpcTransaction, bool) {
	tx, found, err := p.getTransactionFromProviders(txid)
	if err != nil {
		p.observeSendNotSurfaced("error")
		glog.Errorf("eth_getTransactionByHash from alternative providers returned error %v", err)
		return nil, false
	}
	if !found {
		p.observeSendNotSurfaced("not_found")
		glog.Errorf("eth_getTransactionByHash from alternative providers did not find txid %s", txid)
		return nil, false
	}
	return tx, true
}

// cacheMempoolTransaction caches a pending transaction body under txid and indexes it in the wrapped
// Blockbook mempool, ordering itself against concurrent sends for the same (from, nonce) slot through
// the send generation gen. Reached from the send path with the body decoded from the signed bytes,
// and from handleMempoolTransaction when those bytes did not decode.
func (p *AlternativeSendTxProvider) cacheMempoolTransaction(txid string, tx *bchain.RpcTransaction, gen uint64) {
	from, nonce, decoded := txSenderAndNonce(tx)

	// checked before the cache scan below: the scan only sees slots that produced an entry
	if decoded && p.slotSupersededBy(from, nonce, gen) {
		return
	}

	if !p.insertMempoolTx(txid, tx, gen, from, nonce, decoded) {
		// a newer replacement already occupies this nonce slot; do not cache or surface this one
		return
	}

	// Retire any cached predecessor sharing this tx's sender and nonce. The send path already
	// does this from the raw hex the moment the relay accepts the replacement (see
	// SendRawTransaction, #1573); repeating it here is a harmless no-op in that flow and keeps the
	// removal correct when handleMempoolTransaction is reached directly.
	if decoded {
		p.evictReplacedByNonce(from, nonce, txid, gen)
	}

	if p.mempool != nil {
		p.mempool.AddTransactionToMempool(txid)
		// A concurrent higher-generation send for this same (from, nonce) can run its
		// evictReplacedByNonce during the AddTransactionToMempool round-trip above, deleting txid
		// from BOTH the provider cache and the wrapped mempool; the add then re-inserts it into the
		// wrapped mempool only. reconcileMempoolTxs walks only the provider cache (the source of
		// truth), so that orphan would linger as "Unconfirmed" until the 10-minute sweep (#1573). If
		// txid is no longer cached, undo the add. The lock covers only the map read, never a network
		// call, so it cannot serialize backend RPCs.
		p.mempoolTxsMux.Lock()
		_, stillCached := p.mempoolTxs[txid]
		p.mempoolTxsMux.Unlock()
		if !stillCached {
			p.mempool.RemoveTransactionFromMempool(txid)
		}
	}
}

// insertMempoolTx inserts the entry unless a strictly newer send for the same (from, nonce) slot has
// already cached its own, reporting whether it inserted. Deliberately a separate function: the unlock
// is deferred, so a panic inside cannot leave mempoolTxsMux held - which, since the fetch-back moved
// to a goroutine with a recover, would deadlock every send, read, reconcile and nonce-floor lookup
// instead of crashing the process. It also lazily creates the map, the one panic it could hit.
func (p *AlternativeSendTxProvider) insertMempoolTx(txid string, tx *bchain.RpcTransaction, gen uint64, from ethcommon.Address, nonce uint64, decoded bool) bool {
	p.mempoolTxsMux.Lock()
	defer p.mempoolTxsMux.Unlock()
	if p.mempoolTxs == nil {
		p.mempoolTxs = make(map[string]storedTx)
	}
	// Skip a stale insert: a concurrent, higher-generation send for the same (from, nonce) slot can
	// already have cached its replacement. Inserting this older submission would surface a second
	// pending tx for the same nonce and, worse, its eviction by the caller would drop the newer
	// replacement that will actually mine. The send generations exist to order exactly this
	// (#1573 follow-up). The comparison must stay the plain `>` evictReplacedByNonce applies to the
	// same pair, or a gen-0 submission neither skips itself here nor evicts the other entry there,
	// leaving both cached for one nonce slot.
	if decoded {
		for otherTxid, st := range p.mempoolTxs {
			if otherTxid == txid {
				continue
			}
			if f, n, ok := txSenderAndNonce(st.tx); ok && f == from && n == nonce && st.gen > gen {
				return false
			}
		}
	}
	p.mempoolTxs[txid] = storedTx{tx: tx, time: uint32(time.Now().Unix()), gen: gen}
	return true
}

// txSenderAndNonce decodes the sender address and account nonce of a cached RPC transaction,
// reporting ok=false when either field is missing or unparsable so callers skip the entry rather
// than act on a zero value.
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

// evictReplacedByNonce removes any cached transaction that shares sender `from` and account
// `nonce` with a newly accepted replacement identified by (keepTxid, keepGen), except that
// replacement itself. Once a replacement for a nonce slot is accepted, the previously cached
// transaction for that slot can never mine, so it leaves the cache by fee-replacement: the exit is
// counted as rbf_replaced and its residence observed, consistent with every other way an entry
// leaves (so the lifecycle metrics stay balanced). Matching is by decoded address and numeric
// nonce rather than raw strings, so provider differences in hex casing or zero-padding cannot hide
// a predecessor. A cached entry from a strictly higher send generation is left intact: when an older
// submission's slow fetch-back races a newer replacement, the older one must not evict the newer. A
// keepGen of 0 means this replacement's own send order is unknown (raw-hex sender recovery failed at
// send time), so it evicts only other unordered (generation-0) entries and never a
// generation-carrying replacement that may still mine.
//
// EVERY match is retired, not just the first. Two entries can share a slot transiently: insertMempoolTx
// only refuses an insert when a STRICTLY newer entry is already cached, so a send whose scan runs before
// a newer send inserts can still land beside it, and stopping after one victim then left the slot with
// two cached, address-indexed transactions - the state insertMempoolTx's own comment says must never
// exist. It needs three concurrent same-slot sends and both stragglers inserting inside a
// sub-microsecond window (0 occurrences in 27k forced races, ~0.01% of exhaustive interleavings), and
// the next same-nonce send or a reconcile cycle heals it, so this is invariant hardening rather than an
// incident fix: every entry the scan skipped satisfies exactly the predicate above.
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
		cachedFrom, cachedNonce, ok := txSenderAndNonce(storedTx.tx)
		if !ok || cachedFrom != from || cachedNonce != nonce {
			continue
		}
		// Keep any cached entry from a higher send generation. For keepGen==0 (this replacement's
		// order is unknown) that still protects every generation-carrying replacement while evicting
		// only other unordered (gen==0) predecessors. handleMempoolTransaction's staleness check uses
		// the same comparison, so the two rules cannot disagree about a slot.
		if storedTx.gen > keepGen {
			continue
		}
		victims = append(victims, victim{txid: txid, time: storedTx.time})
	}
	p.mempoolTxsMux.Unlock()

	for _, v := range victims {
		glog.Infof("eth_sendRawTransaction replacing txid %s by %s", v.txid, keepTxid)
		// Meter the fee-replacement exit only if this call is the one that removed the predecessor;
		// the acceptance-time and handleMempoolTransaction passes can both target it, as can a
		// concurrent reconcile eviction, so gating on the removal avoids double-counting rbf_replaced.
		if p.removeMempoolTx(v.txid) {
			p.observeMempoolReconciliation("rbf_replaced")
			p.observeMempoolTxResidence("rbf_replaced", v.time)
		}
	}
}

// GetTransaction gets a transaction from alternative mempool cache
func (p *AlternativeSendTxProvider) GetTransaction(txid string) (*bchain.RpcTransaction, bool) {
	if !p.fetchMempoolTx {
		return nil, false
	}

	var storedTx storedTx
	var found bool

	p.mempoolTxsMux.Lock()
	storedTx, found = p.mempoolTxs[txid]
	p.mempoolTxsMux.Unlock()

	if found {
		if time.Unix(int64(storedTx.time), 0).Before(time.Now().Add(-p.mempoolTxsTimeout)) {
			// the same staleness timeout the reconcile loop applies, just reached on the read path
			// first; route it through removeMempoolTx (not RemoveTransaction) so the wrapped
			// Blockbook mempool's per-address index is cleared too - otherwise, when the caller's
			// own primary-RPC lookup errors instead of returning null, the expired private tx keeps
			// being listed as pending for the address until the 10-minute mempool sweep. Record the
			// exit only if this read is the one that removed the entry, so a concurrent reconcile
			// eviction of the same expired tx does not also count it.
			if p.removeMempoolTx(txid) {
				p.observeMempoolReconciliation("timeout")
				p.observeMempoolTxResidence("timeout", storedTx.time)
			}
			return nil, false
		}
		if storedTx.tx == nil {
			return nil, false
		}
		// Hand out a copy, never the cached body itself. The caller (EthereumRPC.GetTransaction ->
		// EthTxToTx with fixEIP55=true) rewrites From and To in place, and it holds no lock - which is
		// what makes it a data race against every reader here, all of which hold mempoolTxsMux. The
		// send path triggers that writer on its own freshly cached entry, through
		// cacheMempoolTransaction -> AddTransactionToMempool -> GetTransactionForMempool. It is also
		// what silently rewrote the body decodeAlternativeSendTx documents as lower-case hex.
		// RpcTransaction is all strings, so this shallow copy is a complete one: keep it that way, or
		// deep-copy any reference-typed field a future version adds.
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

// shutdown stops the background mempool reconciliation goroutine. Safe to call on a
// nil receiver and more than once.
func (p *AlternativeSendTxProvider) shutdown() {
	if p == nil || p.stop == nil {
		return
	}
	p.stopOnce.Do(func() { close(p.stop) })
}

func (p *AlternativeSendTxProvider) reconcileMempoolTxs() {
	type cachedTx struct {
		txid string
		tx   storedTx
	}

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
		// a freshly submitted tx may transiently be unknown to a load-balanced provider node,
		// give it one check period before reconciling
		if time.Since(time.Unix(int64(tx.tx.time), 0)) < alternativeMempoolTxCheckPeriod {
			p.observeMempoolReconciliation("skipped_fresh")
			continue
		}
		timedOut := time.Unix(int64(tx.tx.time), 0).Before(time.Now().Add(-p.mempoolTxsTimeout))
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

		// The provider answered without error and the tx is not mined: it is either still reported as
		// pending (known) or no longer surfaced by eth_getTransactionByHash (!known). If a different
		// transaction has already consumed its nonce (e.g. a replacement submitted outside Blockbook),
		// it can never be mined, so evict it deterministically instead of waiting for the timeout -
		// regardless of whether the provider still surfaces it, because a spent nonce is a positive,
		// irreversible on-chain fact. Only nonces strictly below the confirmed account nonce are
		// treated as superseded; equal or higher nonces are still mineable (the next tx, or a gap
		// waiting to be filled) and are left intact.
		if p.transactionSupersededByNonce(tx.tx.tx, confirmedNonces, confirmedNonceFailed) {
			p.evictMempoolTx("nonce_superseded", tx.txid, tx.tx.time)
			continue
		}

		if !known {
			// A null/empty eth_getTransactionByHash is NOT authoritative proof the tx is gone:
			// Blink-style private/MEV relays stop surfacing a still-pending, still-mineable tx via
			// eth_getTransactionByHash while it stays broadcast. Evicting on a single empty probe
			// deleted the tx from both sender and recipient ~1-2 minutes after send, even though it
			// could still be mined. Defer eviction to the absolute cache timeout instead; mined and
			// nonce_superseded above remain the only deterministic early evictions.
			if timedOut {
				p.evictMempoolTx("provider_missing", tx.txid, tx.tx.time)
				continue
			}
			p.observeMempoolReconciliation("provider_missing_pending")
			continue
		}

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

func (p *AlternativeSendTxProvider) observeMempoolReconciliation(action string) {
	if p.metrics == nil || p.metrics.EthAlternativeMempoolEvents == nil {
		return
	}
	p.metrics.EthAlternativeMempoolEvents.With(common.Labels{"action": action}).Inc()
}

// evictMempoolTx removes the cache entry and, only when this call actually removed it, records the
// terminal reconcile decision: the event counter plus the entry's residence (how long it lived
// before this eviction reason fired), so the eviction rate and the per-reason lifetime distribution
// stay consistent. Gating on the removal is what keeps the count honest - reconcile works off a
// snapshot taken cycle-start, so the read-path or a concurrent RBF eviction may already have removed
// (and metered) the same entry; without the gate this call would double-count it under a second
// action. Decisions that keep an entry for a later cycle use observeMempoolReconciliation directly.
func (p *AlternativeSendTxProvider) evictMempoolTx(action, txid string, addedUnix uint32) {
	if !p.removeMempoolTx(txid) {
		return
	}
	p.observeMempoolReconciliation(action)
	p.observeMempoolTxResidence(action, addedUnix)
}

// observeMempoolTxResidence records the age of a cache entry (seconds since it was broadcast) at the
// moment it is evicted, labeled by the deciding action. This makes the non-deterministic lifetime of
// an unconfirmed tx visible per eviction reason - e.g. provider_missing clustering near the timeout
// rather than at ~1-2 min would show a premature-eviction regression like the one #1573 describes.
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

// setMempoolOldestAge records how long the oldest cached entry has lived (seconds since broadcast),
// or 0 when the cache is empty. Sampled once per reconcile cycle alongside the cache size. A value
// climbing toward the cache timeout at non-zero depth is the live stuck-tx signal the exit-only
// residence histogram cannot show until an entry finally times out.
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

// observeSendNotSurfaced counts a relay-accepted private send whose fetch-back failed to surface
// it, so it was cached and indexed nowhere - the precursor to a nonce-reuse / hanging-tx incident.
func (p *AlternativeSendTxProvider) observeSendNotSurfaced(reason string) {
	if p.metrics == nil || p.metrics.EthAlternativeSendNotSurfaced == nil {
		return
	}
	p.metrics.EthAlternativeSendNotSurfaced.With(common.Labels{"reason": reason}).Inc()
}

// transactionSupersededByNonce reports whether a different transaction has already consumed the
// cached transaction's nonce, making it permanently unmineable. Confirmed-nonce lookups are memoized
// per sender via resolved/failed so each sender is queried at most once per reconcile cycle.
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

// getConfirmedNonce returns the number of transactions mined from the address at the latest block,
// i.e. the lowest nonce not yet consumed on-chain. It queries every configured provider and returns
// the most conservative (lowest) value so a lagging or misbehaving provider cannot cause a still
// mineable transaction to be evicted.
//
// The "latest" tag carries the usual chain-tip caveat: if the nonce was consumed only in the tip
// block and that block is later reorged out, an eviction here may turn out premature. This is the
// same exposure as the mined-tx removal above and is bounded - eviction only drops Blockbook's cache
// entry, it cancels nothing on-chain, and a still-valid tx is re-indexed when it is actually mined.
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

// removeMempoolTx evicts txid from the alternative-provider cache and, when a delegate is wired,
// from the wrapped Blockbook mempool too. It returns whether the entry was actually present in the
// provider cache. The provider-cache delete (RemoveTransaction) is the single point of truth for
// that, so it runs first: concurrent reconcile / read-path / RBF evictions of the same txid then
// see removed=false for all but the one that actually deleted it, letting callers meter the exit
// exactly once. The delegate (which also clears the wrapped mempool's address index) is invoked
// only on a real removal; it re-enters RemoveTransaction as a harmless no-op.
func (p *AlternativeSendTxProvider) removeMempoolTx(txid string) bool {
	// action "" - the caller meters this exit under its own reconcile decision
	removed := p.removeTransaction(txid, "")
	if removed && p.removeTransactionFromMempool != nil {
		p.removeTransactionFromMempool(txid)
	}
	return removed
}

// RemoveTransaction removes a transaction from alternative mempool cache. It is the entry point for
// removals carrying no reconcile decision of their own - block sync indexing a mined transaction and
// the read path finding one mined or unknown, both via EthereumRPC.removeTransactionFromMempool - so
// it meters them as sync_removed. Without that they were counted nowhere: evictMempoolTx meters only
// the goroutine whose own delete removed the entry, and block sync (seconds after the block) almost
// always beats the next reconcile probe (up to a minute later). Reached again as the delegate of
// removeMempoolTx, where the entry is already gone, so nothing is metered twice.
func (p *AlternativeSendTxProvider) RemoveTransaction(txid string) bool {
	return p.removeTransaction(txid, "sync_removed")
}

// removeTransaction removes a transaction from the alternative mempool cache. When the removed
// transaction was the sender's last cached one, the sender's nonce-routing entry is released
// as well (see releaseRecentSender) so address polling stops hitting the alternative provider
// once nothing private remains pending. It reports whether the entry was actually present, so
// callers can attribute a lifecycle metric to the single goroutine that truly evicted it (see
// evictMempoolTx / evictReplacedByNonce). A non-empty action meters the exit here instead.
func (p *AlternativeSendTxProvider) removeTransaction(txid string, action string) bool {
	if !p.fetchMempoolTx {
		return false
	}

	p.mempoolTxsMux.Lock()
	removedTx, found := p.mempoolTxs[txid]
	delete(p.mempoolTxs, txid)
	senderSettled := false
	var sender ethcommon.Address
	if found && removedTx.tx != nil && removedTx.tx.From != "" {
		sender = ethcommon.HexToAddress(removedTx.tx.From)
		senderSettled = true
		for _, storedTx := range p.mempoolTxs {
			if storedTx.tx != nil && ethcommon.HexToAddress(storedTx.tx.From) == sender {
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

// nonceURL returns the provider URL to use for addr's nonce lookup: the URL that accepted
// the sender's most recent transaction when known (a broadcast succeeds if ANY configured
// provider accepts it, so the first URL may never have seen the transaction), falling back
// to the first configured URL.
func (p *AlternativeSendTxProvider) nonceURL(addr ethcommon.Address) string {
	p.recentSendersMux.Lock()
	defer p.recentSendersMux.Unlock()
	if s, found := p.recentSenders[addr]; found && s.url != "" {
		return s.url
	}
	return p.urls[0]
}

// getNonces returns the pending account nonce from the alternative provider that accepted
// the sender's most recent transaction (see nonceURL), plus the confirmed (latest) nonce
// when withConfirmed is set. When both are requested they are fetched in a single JSON-RPC
// batch round-trip; otherwise only the pending nonce is requested. The confirmed nonce is
// best-effort: a failed latest lookup yields confirmedOK=false (not an error) so the caller
// can omit it. An error is returned only when the required pending nonce cannot be obtained.
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
