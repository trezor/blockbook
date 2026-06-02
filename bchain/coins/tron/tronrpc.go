package tron

import (
	"context"
	"encoding/json"
	"math/big"
	"net"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
	"github.com/trezor/blockbook/common"
)

const (
	// MainNet is production network
	MainNet     eth.Network = 11111
	TestNetNile eth.Network = 201910292

	tronDefaultFullNodeHTTPPort = "8090"
	tronDefaultSolidityHTTPPort = "8091"

	TRC10TokenType   bchain.TokenStandardName = "TRC10"
	TRC20TokenType   bchain.TokenStandardName = "TRC20"
	TRC721TokenType  bchain.TokenStandardName = "TRC721"
	TRC1155TokenType bchain.TokenStandardName = "TRC1155"

	tronBestHeaderMaxAge = 30 * time.Second
)

type TronConfiguration struct {
	eth.Configuration
	MessageQueueBinding     string `json:"message_queue_binding"`
	FullNodeHTTPURLTemplate string `json:"tron_fullnode_http_url_template"`
	SolidityHTTPURLTemplate string `json:"tron_solidity_http_url_template"`
}

type tronResourceCode int64

type tronTxContractValue struct {
	OwnerAddress    string            `json:"owner_address,omitempty"`
	ToAddress       string            `json:"to_address,omitempty"`
	AccountAddress  string            `json:"account_address,omitempty"`
	ContractAddress string            `json:"contract_address,omitempty"`
	ReceiverAddress string            `json:"receiver_address,omitempty"`
	Resource        *tronResourceCode `json:"resource,omitempty"`
	Amount          *int64            `json:"amount,omitempty"`
	CallValue       *int64            `json:"call_value,omitempty"`
	FrozenBalance   *int64            `json:"frozen_balance,omitempty"`
	UnfreezeBalance *int64            `json:"unfreeze_balance,omitempty"`
	Balance         *int64            `json:"balance,omitempty"`
	Votes           []tronTxVote      `json:"votes,omitempty"`
	Data            string            `json:"data,omitempty"`
}

type tronTxVote struct {
	VoteAddress string `json:"vote_address,omitempty"`
	VoteCount   *int64 `json:"vote_count,omitempty"`
}

type tronTxContract struct {
	Type      string `json:"type"`
	Parameter struct {
		Value tronTxContractValue `json:"value"`
	} `json:"parameter"`
}

type tronGetTransactionByIDResponse struct {
	TxID       string `json:"txID,omitempty"`
	RawDataHex string `json:"raw_data_hex"`
	RawData    struct {
		Timestamp *int64           `json:"timestamp,omitempty"`
		FeeLimit  *int64           `json:"fee_limit,omitempty"`
		Data      string           `json:"data,omitempty"`
		Contract  []tronTxContract `json:"contract"`
	} `json:"raw_data"`
}

type TronRPC struct {
	*eth.EthereumRPC
	Parser      *TronParser
	ChainConfig *TronConfiguration
	mq          *bchain.MQ
	// callCtx is the base context for RPC calls (the embedded RPC client and the
	// HTTP node clients); Shutdown cancels it so an in-flight sync call aborts
	// promptly. The rpc-client side is also covered by CloseRPC; this additionally
	// reaches the HTTP node fetches, which CloseRPC cannot.
	callCtx              context.Context
	cancelCall           context.CancelFunc
	fullNodeHTTP         TronHTTP
	solidityNodeHTTP     TronHTTP
	internalDataProvider *TronInternalDataProvider
	bestHeaderLock       sync.Mutex
	bestHeader           bchain.EVMHeader
	bestHeaderTime       time.Time
	bestSolidifiedHeight uint64
	hasSolidifiedHeight  bool
	newBlockNotifyCh     chan struct{}
	newBlockNotifyOnce   sync.Once
	// lastNotifyNs is the UnixNano of the last ZeroMQ block notification that drove
	// a tip refresh. tipWatchdog uses it to detect a silently stalled ZeroMQ feed
	// (Tron has no newHeads WS subscription; if the publisher stops, nothing errors).
	lastNotifyNs atomic.Int64
}

func NewTronRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	ethereumRPC, err := eth.NewEthereumRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}

	var cfg TronConfiguration
	err = json.Unmarshal(config, &cfg)
	if err != nil {
		return nil, errors.Annotatef(err, "Invalid Tron configuration file")
	}

	cfg.Eip1559Fees = false

	bchain.EthereumTokenStandardMap = []bchain.TokenStandardName{TRC20TokenType, TRC721TokenType, TRC1155TokenType}

	tronRpc := &TronRPC{
		EthereumRPC:      ethereumRPC.(*eth.EthereumRPC),
		Parser:           NewTronParser(cfg.BlockAddressesToKeep, cfg.AddressAliases),
		newBlockNotifyCh: make(chan struct{}, 1),
	}
	ethChainConfig := tronRpc.EthereumRPC.ChainConfig

	tronRpc.Parser.HotAddressMinContracts = ethChainConfig.HotAddressMinContracts
	tronRpc.Parser.HotAddressLRUCacheSize = ethChainConfig.HotAddressLRUCacheSize
	tronRpc.Parser.HotAddressMinHits = ethChainConfig.HotAddressMinHits
	tronRpc.Parser.AddrContractsCacheMinSize = ethChainConfig.AddressContractsCacheMinSize
	tronRpc.Parser.AddrContractsCacheMaxBytes = ethChainConfig.AddressContractsCacheMaxBytes
	tronRpc.Parser.AddrContractsCacheBulkMaxBytes = ethChainConfig.AddressContractsCacheBulkMaxBytes

	tronRpc.EthereumRPC.Parser = tronRpc.Parser
	tronRpc.ChainConfig = &cfg
	tronRpc.PushHandler = pushHandler

	fullNodeURL, err := resolveTronHTTPURL(cfg.FullNodeHTTPURLTemplate, cfg.RPCURL, tronDefaultFullNodeHTTPPort)
	if err != nil {
		return nil, errors.Annotate(err, "resolve Tron full node HTTP URL")
	}
	solidityURL, err := resolveTronHTTPURL(cfg.SolidityHTTPURLTemplate, cfg.RPCURL, tronDefaultSolidityHTTPPort)
	if err != nil {
		return nil, errors.Annotate(err, "resolve Tron solidity node HTTP URL")
	}

	// ethChainConfig.RPCTimeout has already been clamped to a positive value by
	// NewEthereumRPC, so the HTTP node clients inherit the same finite timeout.
	timeout := time.Duration(ethChainConfig.RPCTimeout) * time.Second
	tronRpc.fullNodeHTTP = NewTronHTTPClient(fullNodeURL, timeout)
	tronRpc.solidityNodeHTTP = NewTronHTTPClient(solidityURL, timeout)

	internalProvider := NewTronInternalDataProvider(
		tronRpc.solidityNodeHTTP,
		timeout,
	)

	tronRpc.internalDataProvider = internalProvider
	tronRpc.EthereumRPC.InternalDataProvider = internalProvider

	tronRpc.callCtx, tronRpc.cancelCall = context.WithCancel(context.Background())

	return tronRpc, nil
}

// requestContext returns the base context for RPC calls. Shutdown cancels it so
// in-flight calls abort promptly. Falls back to context.Background() when unset
// (e.g. a directly-constructed test instance).
func (b *TronRPC) requestContext() context.Context {
	if b.callCtx != nil {
		return b.callCtx
	}
	return context.Background()
}

func resolveTronHTTPURL(explicitURL, rpcURL, defaultPort string) (string, error) {
	explicitURL = strings.TrimSpace(explicitURL)
	if explicitURL != "" {
		return explicitURL, nil
	}

	parsed, err := url.Parse(strings.TrimSpace(rpcURL))
	if err != nil {
		return "", errors.Annotate(err, "invalid rpc_url")
	}
	if parsed.Scheme == "" {
		return "", errors.New("missing scheme in rpc_url")
	}

	host := parsed.Hostname()
	if host == "" {
		return "", errors.New("missing host in rpc_url")
	}

	parsed.Host = net.JoinHostPort(host, defaultPort)
	parsed.Path = ""
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

// OpenRPC opens an RPC connection to the Tron backend (wsURL is unused – Tron has no WS subscriptions)
var OpenRPC = func(url, _ string) (bchain.EVMRPCClient, bchain.EVMClient, error) {
	opts := []rpc.ClientOption{}
	opts = append(opts, rpc.WithWebsocketMessageSizeLimit(0))

	r, err := rpc.DialOptions(context.Background(), url, opts...)
	if err != nil {
		return nil, nil, err
	}

	rpcClient := &TronRPCClient{Client: r}
	ethClient := ethclient.NewClient(r) // Ethereum client for compatibility
	tc := &TronClient{
		Client:    ethClient,
		rpcClient: rpcClient,
	}

	return rpcClient, tc, nil
}

// Initialize Tron RPC
func (b *TronRPC) Initialize() error {
	b.OpenRPC = OpenRPC

	rc, ec, err := b.OpenRPC(b.ChainConfig.RPCURL, "")
	if err != nil {
		return err
	}

	b.Client = ec
	b.RPC = rc
	b.MainNetChainID = MainNet

	ctx, cancel := context.WithTimeout(b.requestContext(), b.Timeout)
	defer cancel()

	id, err := b.Client.NetworkID(ctx)
	if err != nil {
		return err
	}

	// parameters for getInfo request
	switch eth.Network(id.Uint64()) {
	case MainNet:
		b.Testnet = false
		b.Network = "mainnet"
	case TestNetNile:
		b.Testnet = true
		b.Network = "nile"
	default:
		return errors.Errorf("Unknown network id %v", id)
	}

	log.Info("TronRPC: initialized Tron blockchain: ", b.Network)
	return nil
}

// GetBestBlockHash returns hash of the tip of the best-block-chain
// need to overwrite this because the getBestHeader method in EthRpc is
// relying on the subscription
func (b *TronRPC) GetBestBlockHash() (string, error) {
	var err error
	var header bchain.EVMHeader

	header, err = b.getBestHeader()
	if err != nil {
		return "", err
	}

	return strip0xPrefix(header.Hash()), nil
}

// GetBlockHash returns block hash in Tron API format (without 0x prefix).
func (b *TronRPC) GetBlockHash(height uint32) (string, error) {
	hash, err := b.EthereumRPC.GetBlockHash(height)
	if err != nil {
		return "", err
	}
	return strip0xPrefix(hash), nil
}

// GetChainInfo returns information about connected backend with Tron-formatted IDs (without 0x).
func (b *TronRPC) GetChainInfo() (*bchain.ChainInfo, error) {
	ci, err := b.EthereumRPC.GetChainInfo()
	if err != nil {
		return nil, err
	}
	ci.Bestblockhash = strip0xPrefix(ci.Bestblockhash)
	return ci, nil
}

// GetBestBlockHeight returns height of the tip of the best-block-chain
func (b *TronRPC) GetBestBlockHeight() (uint32, error) {
	var err error
	var header bchain.EVMHeader

	header, err = b.getBestHeader()
	if err != nil {
		return 0, err
	}

	return uint32(header.Number().Uint64()), nil
}

// GetBlockHeader returns block header with Tron-formatted hashes (without 0x).
func (b *TronRPC) GetBlockHeader(hash string) (*bchain.BlockHeader, error) {
	ethHash := normalizeHexString(hash)
	bh, err := b.EthereumRPC.GetBlockHeader(ethHash)
	if err != nil {
		return nil, err
	}
	bh.Hash = strip0xPrefix(bh.Hash)
	bh.Prev = strip0xPrefix(bh.Prev)
	bh.Next = strip0xPrefix(bh.Next)
	return bh, nil
}

// GetBlockInfo returns block info with Tron-formatted hashes and txids (without 0x).
func (b *TronRPC) GetBlockInfo(hash string) (*bchain.BlockInfo, error) {
	ethHash := normalizeHexString(hash)
	bi, err := b.EthereumRPC.GetBlockInfo(ethHash)
	if err != nil {
		return nil, err
	}
	bi.Hash = strip0xPrefix(bi.Hash)
	bi.Prev = strip0xPrefix(bi.Prev)
	bi.Next = strip0xPrefix(bi.Next)
	for i := range bi.Txids {
		bi.Txids[i] = strip0xPrefix(bi.Txids[i])
	}
	return bi, nil
}

func (b *TronRPC) getBestHeader() (bchain.EVMHeader, error) {
	// During initial sync (before ZeroMQ is initialized) there is no push-based
	// tip refresh, so always read the latest header from the backend.
	if b.mq == nil {
		_, err := b.refreshBestHeaderFromChain()
		if err != nil {
			return nil, err
		}
		b.bestHeaderLock.Lock()
		defer b.bestHeaderLock.Unlock()
		if b.bestHeader == nil || b.bestHeader.Number() == nil {
			return nil, errors.New("best header is nil")
		}
		return b.bestHeader, nil
	}

	b.bestHeaderLock.Lock()
	cachedHeader := b.bestHeader
	cachedAt := b.bestHeaderTime
	b.bestHeaderLock.Unlock()

	if cachedHeader != nil && cachedAt.Add(tronBestHeaderMaxAge).After(time.Now()) {
		return cachedHeader, nil
	}

	_, err := b.refreshBestHeaderFromChain()
	if err != nil {
		return nil, err
	}

	b.bestHeaderLock.Lock()
	defer b.bestHeaderLock.Unlock()
	if b.bestHeader == nil || b.bestHeader.Number() == nil {
		return nil, errors.New("best header is nil")
	}
	return b.bestHeader, nil
}

// setBestHeader stores h as the cached tip and reports whether it changed.
//
// Unlike EthereumRPC.setBestHeader this is intentionally NON-monotonic: a lower
// height is accepted. Tron's tip is never taken from the feed header (the ZeroMQ
// notification carries none) — it is always an HTTP re-query (refreshBestHeaderFromChain),
// refreshed on every notification and on a tronBestHeaderMaxAge timer, so the cache
// is meant to track whatever the backend currently reports. Accepting a lower height
// is what lets a genuine rollback surface immediately to resyncIndex, so Tron is not
// subject to the frozen-tip masking that the EVM monotonic guard introduces (and which
// EVM has to undo with a watchdog regress).
//
// Tradeoff: with a load-balanced Tron RPC, a single lagging node answering a re-query
// could regress the tip and trip a spurious fork in resyncIndex (the case the EVM
// monotonic guard exists to prevent). That is acceptable for the common single-node
// java-tron backend; if Tron is ever fronted by a load balancer, port the EVM pattern
// here (monotonic hot path + on-advance liveness + allowRegress watchdog poll).
func (b *TronRPC) setBestHeader(h bchain.EVMHeader) bool {
	if h == nil || h.Number() == nil {
		return false
	}
	b.bestHeaderLock.Lock()
	defer b.bestHeaderLock.Unlock()
	changed := false
	if b.bestHeader == nil || b.bestHeader.Number() == nil {
		changed = true
	} else {
		prevNum := b.bestHeader.Number().Uint64()
		newNum := h.Number().Uint64()
		if prevNum != newNum || b.bestHeader.Hash() != h.Hash() {
			changed = true
		}
	}
	b.bestHeader = h
	b.bestHeaderTime = time.Now()
	b.UpdateBestHeader(h)
	return changed
}

func (b *TronRPC) setBestSolidifiedHeight(height uint64) {
	b.bestHeaderLock.Lock()
	defer b.bestHeaderLock.Unlock()
	b.bestSolidifiedHeight = height
	b.hasSolidifiedHeight = true
}

func (b *TronRPC) getBestSolidifiedHeight() (uint64, bool) {
	b.bestHeaderLock.Lock()
	defer b.bestHeaderLock.Unlock()
	return b.bestSolidifiedHeight, b.hasSolidifiedHeight
}

func (b *TronRPC) isBlockSolidified(blockNumber uint64) bool {
	bestSolidifiedHeight, ok := b.getBestSolidifiedHeight()
	if !ok {
		return false
	}
	return blockNumber <= bestSolidifiedHeight
}

func (b *TronRPC) refreshBestHeaderFromChain() (bool, error) {
	if b.Client == nil {
		return false, errors.New("rpc client not initialized")
	}
	ctx, cancel := context.WithTimeout(b.requestContext(), b.Timeout)
	defer cancel()
	h, err := b.Client.HeaderByNumber(ctx, nil)
	if err != nil {
		return false, err
	}
	if h == nil || h.Number() == nil {
		return false, errors.New("best header is nil")
	}
	updated := b.setBestHeader(h)

	solidifiedHeight, err := b.requestLatestSolidifiedBlockHeight(ctx)
	if err != nil {
		glog.V(1).Infof("TronRPC: failed to refresh solidified head: %v", err)
	} else {
		b.setBestSolidifiedHeight(solidifiedHeight)
	}

	return updated, nil
}

func (b *TronRPC) signalNewBlock() {
	select {
	case b.newBlockNotifyCh <- struct{}{}:
	default:
	}
}

func (b *TronRPC) markNotifyAlive() {
	b.lastNotifyNs.Store(time.Now().UnixNano())
}

func (b *TronRPC) newBlockNotifier() {
	for range b.newBlockNotifyCh {
		// Record that the ZeroMQ feed is delivering (the signal tipWatchdog watches);
		// watchdog fallback polls deliberately do not touch this, so they cannot mask
		// a dead feed.
		b.markNotifyAlive()
		updated, err := b.refreshBestHeaderFromChain()
		if err != nil {
			glog.Error("refreshBestHeaderFromChain ", err)
			continue
		}
		if updated && b.PushHandler != nil {
			b.PushHandler(bchain.NotificationNewBlock)
			// Tron mempool is refreshed via periodic/backend resync rather than per-tx
			// subscriptions, so a new block should also trigger a mempool refresh.
			b.PushHandler(bchain.NotificationNewTx)
		}
	}
}

// tipWatchdog detects a silently stalled ZeroMQ block feed. Unlike the EVM
// watchdog there is no WS subscription to reconnect (Tron's ZeroMQ SUB
// auto-reconnects at the transport level), so on a stall it polls the tip
// directly and, if it advanced, re-triggers sync. This keeps the index moving
// when the publisher goes quiet instead of waiting for the ~15-minute periodic
// resync tick. Started exactly once via newBlockNotifyOnce.
func (b *TronRPC) tipWatchdog() {
	threshold := b.TipStaleThreshold()
	interval := threshold / 3
	if interval < 5*time.Second {
		interval = 5 * time.Second
	}
	if interval > 60*time.Second {
		interval = 60 * time.Second
	}
	glog.Infof("TronRPC: tip watchdog started, stall threshold %s, sampling every %s", threshold, interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		if common.IsInShutdown() {
			return
		}
		b.tipWatchdogTick(threshold)
	}
}

// tipWatchdogTick is one watchdog evaluation, split out from the ticker loop so
// it is unit-testable with an injected threshold and a fake client (no wait).
func (b *TronRPC) tipWatchdogTick(threshold time.Duration) {
	// Heartbeat: prove the watchdog goroutine is still ticking (see eth tipWatchdogTick).
	// rate(blockbook_backend_subscription_events{event="watchdog_tick"})==0 means it died.
	b.ObserveSubscriptionEvent("zeromq", "watchdog_tick")
	lastNs := b.lastNotifyNs.Load()
	if lastNs == 0 {
		return
	}
	age := time.Since(time.Unix(0, lastNs))
	b.SetSubscriptionAgeSeconds(age.Seconds())
	if age < threshold {
		return
	}
	glog.Warningf("TronRPC: ZeroMQ block feed silent for %s (threshold %s); polling tip", age.Truncate(time.Second), threshold)
	b.ObserveSubscriptionEvent("zeromq", "watchdog_stall")
	updated, err := b.refreshBestHeaderFromChain()
	if err != nil {
		glog.Error("TronRPC: tip watchdog tip poll error ", err)
		return
	}
	if updated && b.PushHandler != nil {
		b.ObserveSubscriptionEvent("zeromq", "watchdog_tip_advanced")
		b.PushHandler(bchain.NotificationNewBlock)
		b.PushHandler(bchain.NotificationNewTx)
	}
	// Deliberately do NOT re-arm liveness here: lastNotifyNs is refreshed only by a
	// real ZeroMQ delivery (newBlockNotifier), never by the watchdog's own poll —
	// the same invariant the EVM watchdog keeps. If the poll re-armed it, a feed that
	// has gone permanently silent while the poll keeps advancing the tip would look
	// alive and subscription_age_seconds would sawtooth below the threshold instead
	// of climbing past it, hiding the dead feed from any age-based alert. Polling
	// every sample interval while the feed stays silent is the intended recovery, not
	// a problem: Tron blocks are seconds apart, so reaching the threshold already
	// means an abnormal gap, and the poll keeps sync moving until delivery resumes.
}

func (b *TronRPC) handleMQNotification(nt bchain.NotificationType) {
	if nt == bchain.NotificationNewBlock {
		b.signalNewBlock()
		return
	}
	if b.PushHandler != nil {
		b.PushHandler(nt)
	}
}

// GetChainParser returns Tron-specific BlockChainParser
func (b *TronRPC) GetChainParser() bchain.BlockChainParser {
	return b.Parser
}

func (b *TronRPC) CreateMempool(chain bchain.BlockChain) (bchain.Mempool, error) {
	if b.Mempool == nil {
		mempoolTxTimeout, err := b.ChainConfig.MempoolTxTimeoutDuration(false)
		if err != nil {
			return nil, err
		}
		b.Mempool = bchain.NewMempoolEthereumType(chain, mempoolTxTimeout, b.ChainConfig.QueryBackendOnMempoolResync)
	}
	return b.Mempool, nil
}

func (b *TronRPC) InitializeMempool(addrDescForOutpoint bchain.AddrDescForOutpointFunc, onNewTx bchain.OnNewTxFunc) error {
	if b.Mempool == nil {
		return errors.New("Tron Mempool not created")
	}
	b.Mempool.OnNewTx = onNewTx
	b.newBlockNotifyOnce.Do(func() {
		go b.newBlockNotifier()
		go b.tipWatchdog()
	})

	if b.mq == nil {
		tronTopics := bchain.SubscriptionTopics{
			BlockSubscribe: "block",
			BlockReceive:   "blockTrigger",
			TxSubscribe:    "",
			TxReceive:      "",
		}

		mq, err := bchain.NewMQ(b.ChainConfig.MessageQueueBinding, b.handleMQNotification, tronTopics)
		if err != nil {
			return err
		}
		b.mq = mq
	}

	// Arm the watchdog's staleness clock once the ZeroMQ feed is established, not on
	// the first notification that advances the tip. Otherwise a feed that never
	// advances would leave lastNotifyNs at 0 and the watchdog's (lastNs == 0) gate
	// disabled (see EthereumRPC.subscribeEvents for the same fix on the EVM path).
	b.markNotifyAlive()

	return nil
}

func (b *TronRPC) Shutdown(ctx context.Context) error {
	// Abort in-flight RPC-client calls (GetBlockHash, raw block fetch, tip re-query)
	// so a sync call cannot block shutdown up to the RPC timeout. CloseRPC mirrors
	// EthereumRPC.Shutdown; cancelCall additionally aborts the HTTP node fetches
	// (tx details) that CloseRPC cannot reach.
	b.EthereumRPC.CloseRPC()
	if b.cancelCall != nil {
		b.cancelCall()
	}
	if b.mq != nil {
		if err := b.mq.Shutdown(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (b *TronRPC) computeConfirmationsFromBlockNumber(txid string, blockNumber uint64, hasBlockNumber bool) uint32 {
	if !hasBlockNumber {
		return 0
	}
	confirmations, err := b.computeBlockConfirmations(blockNumber)
	if err != nil {
		glog.V(1).Infof("Tron eth_blockNumber tx %v: %v", txid, err)
		return 0
	}
	return confirmations
}

func (b *TronRPC) computeBlockConfirmations(blockNumber uint64) (uint32, error) {
	bh, err := b.getBestHeader()
	if err != nil {
		return 0, err
	}
	bestHeight := bh.Number().Uint64()
	if bestHeight < blockNumber {
		return 0, nil
	}
	return uint32(bestHeight - blockNumber + 1), nil
}

func (b *TronRPC) buildTxFromHTTPData(txByID *tronGetTransactionByIDResponse, txInfo *tronGetTransactionInfoByIDResponse, blockTime int64, confirmations uint32, internalData *bchain.EthereumInternalData, isSolidified bool) (*bchain.Tx, error) {
	csd := tronBuildEthereumSpecificData(txByID, txInfo)
	csd.InternalData = internalData

	if !isSolidified {
		csd.Receipt = nil // set to nil so it can be considered as pending
	}

	tx, err := b.Parser.EthTxToTx(csd.Tx, csd.Receipt, csd.InternalData, blockTime, confirmations, true)
	if err != nil {
		return nil, errors.Annotatef(err, "txid %v", txByID.TxID)
	}

	if len(tx.Vout) > 0 &&
		tx.Vout[0].ScriptPubKey.Addresses == nil &&
		csd.Receipt != nil &&
		csd.Receipt.ContractAddress != "" {
		tx.Vout = []bchain.Vout{{
			ValueSat: tx.Vout[0].ValueSat,
			N:        0,
			ScriptPubKey: bchain.ScriptPubKey{
				Addresses: []string{ToTronAddressFromAddress(csd.Receipt.ContractAddress)},
			},
		}}

		contractAddress := ToTronAddressFromAddress(csd.Receipt.ContractAddress)
		if csd.InternalData == nil {
			csd.InternalData = &bchain.EthereumInternalData{
				Type:     bchain.CREATE,
				Contract: contractAddress,
			}
		} else if csd.InternalData.Contract == "" {
			csd.InternalData.Type = bchain.CREATE
			csd.InternalData.Contract = contractAddress
		}
	}
	tx.Txid = strip0xPrefix(tx.Txid)
	tx.CoinSpecificData = csd
	return tx, nil
}

func synthesizeGenesisTxByID(tx *bchain.RpcTransaction, blockHeight uint32) *tronGetTransactionByIDResponse {
	if blockHeight != 0 || tx == nil {
		return nil
	}

	contract := tronTxContract{}
	contract.Parameter.Value.OwnerAddress = strip0xPrefix(tx.From)

	if strings.TrimSpace(tx.Payload) != "" && tx.Payload != "0x" {
		contract.Type = "TriggerSmartContract"
		contract.Parameter.Value.ContractAddress = strip0xPrefix(tx.To)
		contract.Parameter.Value.CallValue = tronHexQuantityToInt64Ptr(tx.Value)
		contract.Parameter.Value.Data = strip0xPrefix(tx.Payload)
	} else {
		contract.Type = "TransferContract"
		contract.Parameter.Value.ToAddress = strip0xPrefix(tx.To)
		contract.Parameter.Value.Amount = tronHexQuantityToInt64Ptr(tx.Value)
	}

	txByID := &tronGetTransactionByIDResponse{
		TxID: strip0xPrefix(tx.Hash),
	}
	txByID.RawData.FeeLimit = tronHexQuantityToInt64Ptr(tx.GasLimit)
	txByID.RawData.Contract = []tronTxContract{contract}
	return txByID
}

func synthesizeGenesisTxInfo(txHash string, blockHeight uint32, blockTime int64) *tronGetTransactionInfoByIDResponse {
	if blockHeight != 0 {
		return nil
	}

	blockNumber := int64(0)
	txInfo := &tronGetTransactionInfoByIDResponse{
		ID:          strip0xPrefix(txHash),
		BlockNumber: &blockNumber,
	}
	if blockTime >= 0 {
		blockTimestamp := blockTime * 1000
		txInfo.BlockTimeStamp = &blockTimestamp
	}
	return txInfo
}

func (b *TronRPC) getTransactionByIDMapForBlockWithContext(ctx context.Context, hash string, blockHeight uint32, isSolidified bool) (map[string]*tronGetTransactionByIDResponse, error) {
	var (
		blockResp *tronGetBlockResponse
		err       error
	)
	if hash != "" && hash != "pending" {
		blockResp, err = b.requestBlockByID(ctx, hash, isSolidified)
	} else {
		blockResp, err = b.requestBlockByNum(ctx, blockHeight, isSolidified)
	}
	if err != nil {
		return nil, err
	}
	if blockResp == nil {
		return nil, nil
	}
	return mapTransactionByID(blockResp.Transactions), nil
}

type tronRPCBlockHeader struct {
	Hash       string `json:"hash"`
	ParentHash string `json:"parentHash"`
	Number     string `json:"number"`
	Time       string `json:"timestamp"`
	Size       string `json:"size"`
}

type tronRPCBlockWithTransactions struct {
	tronRPCBlockHeader
	Transactions []bchain.RpcTransaction `json:"transactions"`
}

// GetBlock returns block with given hash or height, hash has precedence if both passed.
// Tron implementation enriches each tx with data from Tron HTTP endpoints and does not call EthereumRPC.GetBlock.
func (b *TronRPC) GetBlock(hash string, height uint32) (*bchain.Block, error) {
	raw, err := b.EthereumRPC.GetBlockRawByHashOrHeight(hash, height, true)
	if err != nil {
		return nil, err
	}
	var block tronRPCBlockWithTransactions
	if err := json.Unmarshal(raw, &block); err != nil {
		return nil, errors.Annotatef(err, "hash %v, height %v", hash, height)
	}

	blockNumber, ok := tronUint64(block.Number)
	if !ok {
		return nil, errors.Errorf("invalid block number %q", block.Number)
	}
	blockTime, ok := tronUint64(block.Time)
	if !ok {
		return nil, errors.Errorf("invalid block timestamp %q", block.Time)
	}
	blockSize, ok := tronUint64(block.Size)
	if !ok {
		return nil, errors.Errorf("invalid block size %q", block.Size)
	}

	confirmations, err := b.computeBlockConfirmations(blockNumber)
	if err != nil {
		return nil, err
	}
	isSolidified := b.isBlockSolidified(blockNumber)

	bbh := bchain.BlockHeader{
		Hash:          strip0xPrefix(block.Hash),
		Prev:          strip0xPrefix(block.ParentHash),
		Height:        uint32(blockNumber),
		Confirmations: int(confirmations),
		Time:          int64(blockTime),
		Size:          int(blockSize),
	}

	txInfosByID := map[string]*tronGetTransactionInfoByIDResponse{}
	txByIDByID := map[string]*tronGetTransactionByIDResponse{}
	internalData := make([]bchain.EthereumInternalData, len(block.Transactions))
	contracts := make([]bchain.ContractInfo, 0)
	var internalErr error

	if len(block.Transactions) > 0 {
		ctx, cancel := context.WithTimeout(b.requestContext(), b.Timeout)
		defer cancel()

		type txInfosResult struct {
			infos []tronGetTransactionInfoByIDResponse
			err   error
		}
		type txByIDResult struct {
			txByID map[string]*tronGetTransactionByIDResponse
			err    error
		}

		infosCh := make(chan txInfosResult, 1)
		txByIDCh := make(chan txByIDResult, 1)

		go func() {
			infos, err := b.requestTransactionInfoByBlockNum(ctx, bbh.Height, isSolidified)
			infosCh <- txInfosResult{infos: infos, err: err}
		}()
		go func() {
			txByID, err := b.getTransactionByIDMapForBlockWithContext(ctx, hash, bbh.Height, isSolidified)
			txByIDCh <- txByIDResult{txByID: txByID, err: err}
		}()

		infosRes := <-infosCh
		if infosRes.err != nil {
			return nil, errors.Annotatef(infosRes.err, "height %v", bbh.Height)
		}
		if m := mapTransactionInfoByID(infosRes.infos); m != nil {
			txInfosByID = m
		}

		txByIDRes := <-txByIDCh
		if txByIDRes.err != nil {
			return nil, errors.Annotatef(txByIDRes.err, "height %v", bbh.Height)
		}
		if txByIDRes.txByID != nil {
			txByIDByID = txByIDRes.txByID
		}

		if bchain.ProcessInternalTransactions {
			internalData, contracts, internalErr = buildInternalDataFromTronInfos(
				tronTxInfosFromResponses(infosRes.infos),
				block.Transactions,
				bbh.Height,
			)
		}
	}

	txs := make([]bchain.Tx, len(block.Transactions))
	for i := range block.Transactions {
		tx := &block.Transactions[i]
		txByID := txByIDByID[strip0xPrefix(tx.Hash)]
		if txByID == nil {
			txByID = synthesizeGenesisTxByID(tx, bbh.Height)
		}

		if txByID == nil { // todo possibly can be deleted
			b.ObserveChainDataFallback("tron_getblock", "missing_tx_by_id_map")
			glog.V(1).Infof("Tron GetBlock fallback to gettransactionbyid for tx %s in block %d", tx.Hash, bbh.Height)
			txByID, err = b.getTransactionByID(tx.Hash, isSolidified)
			if err != nil {
				return nil, err
			}
		}

		txInfo := txInfosByID[strip0xPrefix(tx.Hash)]
		if txInfo == nil {
			txInfo = synthesizeGenesisTxInfo(tx.Hash, bbh.Height, bbh.Time)
		}
		if txInfo == nil {
			b.ObserveChainDataFallback("tron_getblock", "missing_tx_info_by_block")
			glog.V(1).Infof("Tron GetBlock fallback to gettransactioninfobyid for tx %s in block %d", tx.Hash, bbh.Height)
			txInfo, err = b.getTransactionInfoByID(tx.Hash, isSolidified)
			if err != nil {
				return nil, err
			}
		}
		if txInfo == nil {
			return nil, errors.Errorf("missing txInfo for tx %s in block %d", tx.Hash, bbh.Height)
		}

		var txInternalData *bchain.EthereumInternalData
		if i < len(internalData) {
			txInternalData = &internalData[i]
		}

		rebuiltTx, err := b.buildTxFromHTTPData(txByID, txInfo, bbh.Time, confirmations, txInternalData, isSolidified)
		if err != nil {
			return nil, err
		}
		txs[i] = *rebuiltTx

		if isSolidified && b.Mempool != nil {
			b.Mempool.RemoveTransactionFromMempool(strip0xPrefix(tx.Hash))
		}
	}

	var blockSpecificData *bchain.EthereumBlockSpecificData
	if internalErr != nil || len(contracts) > 0 {
		blockSpecificData = &bchain.EthereumBlockSpecificData{}
		if internalErr != nil {
			blockSpecificData.InternalDataError = internalErr.Error()
		}
		if len(contracts) > 0 {
			blockSpecificData.Contracts = contracts
		}
	}

	return &bchain.Block{
		BlockHeader:      bbh,
		Txs:              txs,
		CoinSpecificData: blockSpecificData,
	}, nil
}

func isTronTxNotFound(err error) bool {
	return errors.Cause(err) == bchain.ErrTxNotFound
}

func reconcileTronMempoolWithPendingList(m bchain.Mempool, pendingTxids []string, removeTx func(string)) int {
	if m == nil || removeTx == nil {
		return 0
	}

	pendingSet := make(map[string]struct{}, len(pendingTxids))
	for _, txid := range pendingTxids {
		pendingSet[strip0xPrefix(txid)] = struct{}{}
	}

	removed := 0
	for _, entry := range m.GetAllEntries() {
		txid := strip0xPrefix(entry.Txid)
		if _, ok := pendingSet[txid]; ok {
			continue
		}
		removeTx(txid)
		removed++
	}

	return removed
}

func (b *TronRPC) reconcileMempoolWithPendingList(pendingTxids []string) {
	if b.Mempool == nil {
		return
	}

	removed := reconcileTronMempoolWithPendingList(b.Mempool, pendingTxids, b.Mempool.RemoveTransactionFromMempool)
	if removed > 0 {
		glog.V(1).Infof("Tron mempool reconcile removed %d stale tx(s)", removed)
	}
}

func (b *TronRPC) getTransactionByIDWithFallback(txid string) (*tronGetTransactionByIDResponse, bool, error) {
	resp, err := b.getTransactionByID(txid, true)
	if err == nil {
		return resp, true, nil
	}
	if !isTronTxNotFound(err) {
		return nil, false, err
	}
	resp, err = b.getTransactionByID(txid, false)
	if err != nil {
		return nil, false, err
	}
	return resp, false, nil
}

func (b *TronRPC) getTransactionInfoByIDWithFallback(txid string) (*tronGetTransactionInfoByIDResponse, bool, error) {
	resp, err := b.getTransactionInfoByID(txid, true)
	if err == nil {
		return resp, true, nil
	}
	if !isTronTxNotFound(err) {
		return nil, false, err
	}
	resp, err = b.getTransactionInfoByID(txid, false)
	if err != nil {
		return nil, false, err
	}
	return resp, false, nil
}

func (b *TronRPC) GetTransaction(txid string) (*bchain.Tx, error) {
	txInfo, isSolidified, err := b.getTransactionInfoByIDWithFallback(txid)
	if err != nil {
		if isTronTxNotFound(err) {
			return b.GetTransactionForMempool(txid)
		}
		return nil, err
	}
	txByID, err := b.getTransactionByID(txid, isSolidified)
	if err != nil {
		return nil, err
	}

	blockTime, blockNumber, hasBlockNumber := tronTxMeta(txInfo)
	confirmations := b.computeConfirmationsFromBlockNumber(txid, blockNumber, hasBlockNumber)
	tx, err := b.buildTxFromHTTPData(txByID, txInfo, blockTime, confirmations, nil, isSolidified)
	if err != nil {
		return nil, err
	}
	if isSolidified && b.Mempool != nil {
		b.Mempool.RemoveTransactionFromMempool(strip0xPrefix(txid))
	}
	return tx, nil
}

// GetTransactionForMempool returns a transaction by the transaction ID using
// the full node HTTP API
func (b *TronRPC) GetTransactionForMempool(txid string) (*bchain.Tx, error) {
	ctx, cancel := context.WithTimeout(b.requestContext(), b.Timeout)
	defer cancel()

	txByID, err := b.requestTransactionFromPending(ctx, txid)
	if err != nil {
		if isTronTxNotFound(err) {
			if b.Mempool != nil {
				b.Mempool.RemoveTransactionFromMempool(strip0xPrefix(txid))
			}
			return nil, bchain.ErrTxNotFound
		}
		return nil, err
	}

	txInfo := &tronGetTransactionInfoByIDResponse{ID: strip0xPrefix(txid)}
	blockTime, blockNumber, hasBlockNumber := tronTxMeta(txInfo)
	confirmations := b.computeConfirmationsFromBlockNumber(txid, blockNumber, hasBlockNumber)
	return b.buildTxFromHTTPData(txByID, txInfo, blockTime, confirmations, nil, false)
}

// GetTransactionSpecific returns tx-specific JSON in Tron API format (without 0x in tx hash fields).
func (b *TronRPC) GetTransactionSpecific(tx *bchain.Tx) (json.RawMessage, error) {
	csd, ok := tx.CoinSpecificData.(bchain.EthereumSpecificData)
	if !ok {
		ntx, err := b.GetTransaction(tx.Txid)
		if err != nil {
			return nil, err
		}
		csd, ok = ntx.CoinSpecificData.(bchain.EthereumSpecificData)
		if !ok {
			return nil, errors.New("Cannot get CoinSpecificData")
		}
	}
	csdCopy := csd
	if csd.Tx != nil {
		txCopy := *csd.Tx
		txCopy.Hash = strip0xPrefix(txCopy.Hash)
		txCopy.BlockHash = strip0xPrefix(txCopy.BlockHash)
		csdCopy.Tx = &txCopy
	}
	m, err := json.Marshal(&csdCopy)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (b *TronRPC) EthereumTypeGetBalance(addrDesc bchain.AddressDescriptor) (*big.Int, error) {
	ctx, cancel := context.WithTimeout(b.requestContext(), b.Timeout)
	defer cancel()

	return b.Client.BalanceAt(ctx, addrDesc, nil)
}

// EthereumTypeEstimateGas supports both EVM hex and Tron Base58 in `from`/`to`
// and calls eth_estimateGas using Tron-compatible params: from, to, value, data.
func (b *TronRPC) EthereumTypeEstimateGas(params map[string]interface{}) (uint64, error) {
	req := make(map[string]interface{}, 4)
	for _, field := range []string{"from", "to"} {
		address, ok := eth.GetStringFromMap(field, params)
		if !ok || address == "" {
			continue
		}
		hexAddress, err := b.Parser.FromTronAddressToHex(address)
		if err != nil {
			return 0, err
		}
		req[field] = hexAddress
	}
	if value, ok := eth.GetStringFromMap("value", params); ok && value != "" {
		req["value"] = value
	}
	if data, ok := eth.GetStringFromMap("data", params); ok && data != "" {
		req["data"] = data
	}

	ctx, cancel := context.WithTimeout(b.requestContext(), b.Timeout)
	defer cancel()

	var result string
	if err := b.RPC.CallContext(ctx, &result, "eth_estimateGas", req); err != nil {
		return 0, err
	}
	return hexutil.DecodeUint64(result)
}

// EthereumTypeRpcCall supports both EVM hex and Tron Base58 in `to`/`from`.
func (b *TronRPC) EthereumTypeRpcCall(data, to, from string) (string, error) {
	normalizedTo := to
	if to != "" {
		hexAddress, err := b.Parser.FromTronAddressToHex(to)
		if err != nil {
			return "", err
		}
		normalizedTo = hexAddress
	}
	normalizedFrom := from
	if from != "" {
		hexAddress, err := b.Parser.FromTronAddressToHex(from)
		if err != nil {
			return "", err
		}
		normalizedFrom = hexAddress
	}
	return b.EthereumRPC.EthereumTypeRpcCall(data, normalizedTo, normalizedFrom)
}

// EthereumTypeGetNonce returns current balance of an address
func (b *TronRPC) EthereumTypeGetNonce(addrDesc bchain.AddressDescriptor) (uint64, error) {
	ctx, cancel := context.WithTimeout(b.requestContext(), b.Timeout)
	defer cancel()
	return b.Client.NonceAt(ctx, addrDesc, nil)
}

// GetContractInfo returns information about a contract
func (b *TronRPC) GetContractInfo(contractDesc bchain.AddressDescriptor) (*bchain.ContractInfo, error) {
	contract, err := b.EthereumRPC.GetContractInfo(contractDesc)
	if err != nil {
		return nil, err
	}
	if contract == nil {
		return nil, nil
	}
	contract.Contract = ToTronAddressFromAddress(contract.Contract)
	glog.Infof("Getting contract info for: %s", contract.Contract)
	return contract, nil
}

func (b *TronRPC) EthereumTypeGetRawTransaction(txid string) (string, error) {
	resp, _, err := b.getTransactionByIDWithFallback(txid)
	if err != nil {
		return "", err
	}
	if resp.RawDataHex == "" {
		return "", errors.Errorf("Tron gettransactionbyid returned empty raw_data_hex for %s", txid)
	}
	return normalizeHexString(resp.RawDataHex), nil
}
