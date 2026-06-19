package eth

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
)

type storedTx struct {
	tx   *bchain.RpcTransaction
	time uint32
}

const alternativeMempoolTxCheckPeriod = time.Minute

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
		metrics:           metrics,
		stop:              make(chan struct{}),
	}

	glog.Infof("Using alternative send transaction providers %v. Only alternative providers %v", urls, onlyAlternative)
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

// SendRawTransaction sends raw transaction to alternative providers
func (p *AlternativeSendTxProvider) SendRawTransaction(hex string) (string, error) {
	var txid string
	var retErr error

	for i := range p.urls {
		r, err := p.callHttpStringResult(p.urls[i], "eth_sendRawTransaction", hex)
		glog.Infof("eth_sendRawTransaction to %s, txid %s", p.urls[i], r)
		// set success return value; or error only if there was no previous success
		if err == nil || len(txid) == 0 {
			txid = r
			retErr = err
		}
	}

	if p.onlyAlternative && p.fetchMempoolTx {
		p.handleMempoolTransaction(txid)
	}

	return txid, retErr
}

// handleMempoolTransaction handles the transaction when using only alternative providers
func (p *AlternativeSendTxProvider) handleMempoolTransaction(txid string) (string, error) {
	tx, found, err := p.getTransactionFromProviders(txid)
	if err != nil {
		glog.Errorf("eth_getTransactionByHash from alternative providers returned error %v", err)
		return txid, err
	} else if !found {
		glog.Errorf("eth_getTransactionByHash from alternative providers did not find txid %s", txid)
		return txid, bchain.ErrTxNotFound
	}

	p.mempoolTxsMux.Lock()
	// remove potential RBF transactions - with equal from and nonce
	var rbfTxid string
	var rbfTime uint32
	for rbf, storedTx := range p.mempoolTxs {
		if storedTx.tx.From == tx.From && storedTx.tx.AccountNonce == tx.AccountNonce {
			rbfTxid = rbf
			rbfTime = storedTx.time
			break
		}
	}
	p.mempoolTxs[txid] = storedTx{tx: tx, time: uint32(time.Now().Unix())}
	p.mempoolTxsMux.Unlock()

	if rbfTxid != "" {
		glog.Infof("eth_sendRawTransaction replacing txid %s by %s", rbfTxid, txid)
		// the replaced entry leaves the cache by fee-replacement rather than reconciliation; record the
		// exit reason and its residence so the lifecycle metrics account for every way an entry leaves.
		p.observeMempoolReconciliation("rbf_replaced")
		p.observeMempoolTxResidence("rbf_replaced", rbfTime)
		if p.removeTransactionFromMempool != nil {
			p.removeTransactionFromMempool(rbfTxid)
		}
	}

	if p.mempool != nil {
		p.mempool.AddTransactionToMempool(txid)
	}

	return txid, nil
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
			p.mempoolTxsMux.Lock()
			delete(p.mempoolTxs, txid)
			p.mempoolTxsMux.Unlock()
			// the same staleness timeout the reconcile loop applies, just reached on the read path
			// first; record it so the timeout counter and residence histogram do not undercount
			// entries read after expiry but before the next reconcile cycle evicts them.
			p.observeMempoolReconciliation("timeout")
			p.observeMempoolTxResidence("timeout", storedTx.time)
			return nil, false
		}
		return storedTx.tx, true
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
	p.mempoolTxsMux.Unlock()
	p.setMempoolCacheSize(size)
}

func (p *AlternativeSendTxProvider) observeMempoolReconciliation(action string) {
	if p.metrics == nil || p.metrics.EthAlternativeMempoolEvents == nil {
		return
	}
	p.metrics.EthAlternativeMempoolEvents.With(common.Labels{"action": action}).Inc()
}

// evictMempoolTx records a terminal reconcile decision and removes the cache entry. It counts the
// decision and observes the entry's residence (how long it lived before this eviction reason fired),
// so the eviction rate and the per-reason lifetime distribution stay consistent. Decisions that keep
// an entry for a later cycle use observeMempoolReconciliation directly instead.
func (p *AlternativeSendTxProvider) evictMempoolTx(action, txid string, addedUnix uint32) {
	p.observeMempoolReconciliation(action)
	p.observeMempoolTxResidence(action, addedUnix)
	p.removeMempoolTx(txid)
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

func (p *AlternativeSendTxProvider) removeMempoolTx(txid string) {
	if p.removeTransactionFromMempool != nil {
		p.removeTransactionFromMempool(txid)
		return
	}
	p.RemoveTransaction(txid)
}

// RemoveTransaction removes a transaction from alternative mempool cache
func (p *AlternativeSendTxProvider) RemoveTransaction(txid string) {
	if !p.fetchMempoolTx {
		return
	}

	p.mempoolTxsMux.Lock()
	delete(p.mempoolTxs, txid)
	p.mempoolTxsMux.Unlock()
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

// getNonces returns the pending account nonce from the first configured alternative
// provider, plus the confirmed (latest) nonce when withConfirmed is set. When both are
// requested they are fetched in a single JSON-RPC batch round-trip; otherwise only the
// pending nonce is requested. The confirmed nonce is best-effort: a failed latest lookup
// yields confirmedOK=false (not an error) so the caller can omit it. An error is returned
// only when the required pending nonce cannot be obtained.
func (p *AlternativeSendTxProvider) getNonces(addr ethcommon.Address, withConfirmed bool) (uint64, uint64, bool, error) {
	if len(p.urls) == 0 {
		return 0, 0, false, errors.New("no alternative provider url configured")
	}
	if !withConfirmed {
		pendingHex, err := p.callHttpStringResult(p.urls[0], "eth_getTransactionCount", addr, "pending")
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
	client, err := rpc.DialContext(ctx, p.urls[0])
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
