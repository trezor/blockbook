package eth

import (
	"context"
	"encoding/hex"
	"encoding/json"
	stdErrors "errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
	"golang.org/x/crypto/sha3"
	"golang.org/x/sync/singleflight"
)

// Network type specifies the type of ethereum network
type Network uint32

const (
	// MainNet is production network
	MainNet Network = 1
	// TestNetSepolia is Sepolia test network
	TestNetSepolia Network = 11155111
	// TestNetHolesky is Holesky test network
	TestNetHolesky Network = 17000
	// TestNetHoodi is Hoodi test network
	TestNetHoodi Network = 560048
)

const (
	defaultErc20BatchSize = 100

	// defaultRPCTimeoutSeconds is used when rpc_timeout is unset or non-positive.
	// A zero b.Timeout makes context.WithTimeout expire immediately (breaking every
	// call), so a finite floor is enforced rather than trusting the config. Kept
	// above the 10s trace_timeout default so the fallback still lets a block's
	// internal-data trace finish.
	defaultRPCTimeoutSeconds = 15

	// Alternative/private relays expire pending txs quickly, so local pending state
	// must not inherit the legacy hour-scale public mempool timeout.
	defaultMempoolTxTimeoutWithAlternativeProvider = 10 * time.Minute
	defaultAlternativeMempoolTxTimeout             = 5 * time.Minute
)

// Ethereum address constants
const (
	// EthereumZeroAddress is the zero address (0x0000...0000) used to check for unset addresses
	EthereumZeroAddress = "0x0000000000000000000000000000000000000000"
	// EthereumAddressHexLength represents the length of an Ethereum address in hex characters (20 bytes * 2)
	EthereumAddressHexLength = 40
	// ENSResolverFunctionSelector is the function selector for ENS registry's resolver(bytes32) method
	ENSResolverFunctionSelector = "0x0178b8bf"
	// ENSAddrFunctionSelector is the function selector for the resolver's addr(bytes32) method
	ENSAddrFunctionSelector = "0x3b3b57de"
	// ENSExpirationFunctionSelector is the function selector for ENS registry's nameExpires(bytes32) method
	ENSExpirationFunctionSelector = "0x1aa2e643"
	// ENSBaseRegistrarAddress is needed for checking .eth domain expiration
	ENSBaseRegistrarAddress = "0x57f1887a8BF19b14fC0dF6Fd9B2acc9Af147eA85"
)

// Configuration represents json config file
type Configuration struct {
	CoinName                          string `json:"coin_name"`
	CoinShortcut                      string `json:"coin_shortcut"`
	Network                           string `json:"network"`
	RPCURL                            string `json:"rpc_url"`
	RPCURLWS                          string `json:"rpc_url_ws"`
	RPCTimeout                        int    `json:"rpc_timeout"`
	TraceTimeout                      string `json:"trace_timeout,omitempty"`
	Erc20BatchSize                    int    `json:"erc20_batch_size,omitempty"`
	BlockAddressesToKeep              int    `json:"block_addresses_to_keep"`
	HotAddressMinContracts            int    `json:"hot_address_min_contracts,omitempty"`
	HotAddressLRUCacheSize            int    `json:"hot_address_lru_cache_size,omitempty"`
	HotAddressMinHits                 int    `json:"hot_address_min_hits,omitempty"`
	AddressContractsCacheMinSize      int    `json:"address_contracts_cache_min_size,omitempty"`
	AddressContractsCacheMaxBytes     int64  `json:"address_contracts_cache_max_bytes,omitempty"`
	AddressContractsCacheBulkMaxBytes int64  `json:"address_contracts_cache_bulk_max_bytes,omitempty"`
	AddressAliases                    bool   `json:"address_aliases,omitempty"`
	MempoolTxTimeoutHours             int    `json:"mempoolTxTimeoutHours"`
	MempoolTxTimeout                  string `json:"mempoolTxTimeout,omitempty"`
	AlternativeMempoolTxTimeout       string `json:"alternativeMempoolTxTimeout,omitempty"`
	QueryBackendOnMempoolResync       bool   `json:"queryBackendOnMempoolResync"`
	ProcessInternalTransactions       bool   `json:"processInternalTransactions"`
	ProcessZeroInternalTransactions   bool   `json:"processZeroInternalTransactions"`
	ConsensusNodeVersionURL           string `json:"consensusNodeVersion"`
	DisableMempoolSync                bool   `json:"disableMempoolSync,omitempty"`
	Eip1559Fees                       bool   `json:"eip1559Fees,omitempty"`
	AlternativeEstimateFee            string `json:"alternative_estimate_fee,omitempty"`
	AlternativeEstimateFeeParams      string `json:"alternative_estimate_fee_params,omitempty"`
	// AverageBlockTimeMs is the chain's nominal block cadence in ms;
	// required for EVM coins (translates duration settings to block counts).
	AverageBlockTimeMs int `json:"averageBlockTimeMs,omitempty"`
	// MissingBlockRetry overrides the sync-worker missing-block retry policy
	// per chain. All fields are optional; missing fields use built-in defaults.
	MissingBlockRetry *bchain.MissingBlockRetry `json:"missingBlockRetry,omitempty"`
}

func parseNonNegativeDuration(name string, value string) (time.Duration, error) {
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, errors.Annotatef(err, "invalid %s", name)
	}
	if d < 0 {
		return 0, errors.Errorf("%s must not be negative", name)
	}
	return d, nil
}

func parsePositiveDuration(name string, value string) (time.Duration, error) {
	d, err := parseNonNegativeDuration(name, value)
	if err != nil {
		return 0, err
	}
	if d == 0 {
		return 0, errors.Errorf("%s must be positive", name)
	}
	return d, nil
}

// MempoolTxTimeoutDuration returns the Blockbook-side EVM mempool retention.
func (c *Configuration) MempoolTxTimeoutDuration(alternativeSendTxProviderEnabled bool) (time.Duration, error) {
	if c.MempoolTxTimeout != "" {
		return parseNonNegativeDuration("mempoolTxTimeout", c.MempoolTxTimeout)
	}
	// Keep the shorter timeout scoped to alternative/private submission only.
	if alternativeSendTxProviderEnabled {
		return defaultMempoolTxTimeoutWithAlternativeProvider, nil
	}
	return time.Duration(c.MempoolTxTimeoutHours) * time.Hour, nil
}

// AlternativeMempoolTxTimeoutDuration returns the alternative-provider cache retention.
func (c *Configuration) AlternativeMempoolTxTimeoutDuration() (time.Duration, error) {
	if c.AlternativeMempoolTxTimeout != "" {
		return parsePositiveDuration("alternativeMempoolTxTimeout", c.AlternativeMempoolTxTimeout)
	}
	return defaultAlternativeMempoolTxTimeout, nil
}

// AverageBlockTimeDuration returns AverageBlockTimeMs as a time.Duration.
func (c *Configuration) AverageBlockTimeDuration() (time.Duration, error) {
	if c.AverageBlockTimeMs <= 0 {
		return 0, errors.Errorf("averageBlockTimeMs must be a positive integer")
	}
	return time.Duration(c.AverageBlockTimeMs) * time.Millisecond, nil
}

// EthereumRPC is an interface to JSON-RPC eth service.
type EthereumRPC struct {
	*bchain.BaseChain
	Client             bchain.EVMClient
	RPC                bchain.EVMRPCClient
	MainNetChainID     Network
	Timeout            time.Duration
	Parser             EthereumLikeParser
	PushHandler        func(bchain.NotificationType)
	OpenRPC            func(string, string) (bchain.EVMRPCClient, bchain.EVMClient, error)
	Mempool            *bchain.MempoolEthereumType
	mempoolInitialized bool
	bestHeaderLock     sync.Mutex
	bestHeader         bchain.EVMHeader
	// newBlockNotifyCh coalesces bursts of newHeads events into a single wake-up.
	// This keeps the subscription reader unblocked while we refresh the canonical tip.
	newBlockNotifyCh chan struct{}
	// subscribeReadersOnce guards the long-lived consumer goroutines (tip notifier,
	// tip watchdog and the NewBlock/NewTx channel readers) so reconnectRPC ->
	// subscribeEvents only re-creates the connection-bound subscriptions and never
	// leaks a fresh set of readers on every reconnect.
	subscribeReadersOnce sync.Once
	// lastSubNotifyNs is the UnixNano of the last newHeads notification that
	// advanced the cached tip (subscription path only, never watchdog polls).
	// Keying liveness on tip advance, not mere arrival, lets the watchdog also
	// catch a feed that keeps delivering but is stuck on one height.
	lastSubNotifyNs           atomic.Int64
	NewBlock                  bchain.EVMNewBlockSubscriber
	newBlockSubscription      bchain.EVMClientSubscription
	NewTx                     bchain.EVMNewTxSubscriber
	newTxSubscription         bchain.EVMClientSubscription
	ChainConfig               *Configuration
	metrics                   *common.Metrics
	supportedStakingPools     []string
	stakingPoolNames          []string
	stakingPoolContracts      []string
	alternativeFeeProvider    alternativeFeeProviderInterface
	alternativeSendTxProvider *AlternativeSendTxProvider
	InternalDataProvider      bchain.EthereumInternalDataProvider
	consensusMonitor          *consensusVersionMonitor
	// Multicall3 deployment state; lazily probed on first call. See multicall.go.
	multicall3Probe   atomic.Int32
	multicall3ProbeSF singleflight.Group
}

// NewEthereumRPC returns new EthRPC instance.
func NewEthereumRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	var err error
	var c Configuration
	err = json.Unmarshal(config, &c)
	if err != nil {
		return nil, errors.Annotatef(err, "Invalid configuration file")
	}
	// keep at least 100 mappings block->addresses to allow rollback
	if c.BlockAddressesToKeep < 100 {
		c.BlockAddressesToKeep = 100
	}
	if c.Erc20BatchSize <= 0 {
		c.Erc20BatchSize = defaultErc20BatchSize
	}
	if c.HotAddressMinContracts <= 0 {
		c.HotAddressMinContracts = defaultHotAddressMinContracts
	}
	if c.HotAddressLRUCacheSize <= 0 {
		c.HotAddressLRUCacheSize = defaultHotAddressLRUCacheSize
	} else if c.HotAddressLRUCacheSize > maxHotAddressLRUCacheSize {
		glog.Warningf("hot_address_lru_cache_size=%d is too large, clamping to %d", c.HotAddressLRUCacheSize, maxHotAddressLRUCacheSize)
		c.HotAddressLRUCacheSize = maxHotAddressLRUCacheSize
	}
	if c.HotAddressMinHits <= 0 {
		c.HotAddressMinHits = defaultHotAddressMinHits
	} else if c.HotAddressMinHits > maxHotAddressMinHits {
		glog.Warningf("hot_address_min_hits=%d is too large, clamping to %d", c.HotAddressMinHits, maxHotAddressMinHits)
		c.HotAddressMinHits = maxHotAddressMinHits
	}
	if c.AddressContractsCacheMinSize <= 0 {
		c.AddressContractsCacheMinSize = defaultAddressContractsCacheMinSize
	}
	if c.AddressContractsCacheMaxBytes <= 0 {
		c.AddressContractsCacheMaxBytes = defaultAddressContractsCacheMaxBytes
	}
	if c.AddressContractsCacheBulkMaxBytes <= 0 {
		c.AddressContractsCacheBulkMaxBytes = defaultAddressContractsCacheBulkMaxBytes
	}
	if c.AddressContractsCacheBulkMaxBytes < c.AddressContractsCacheMaxBytes {
		glog.Warningf("address_contracts_cache_bulk_max_bytes=%d is less than address_contracts_cache_max_bytes=%d", c.AddressContractsCacheBulkMaxBytes, c.AddressContractsCacheMaxBytes)
	}
	if c.TraceTimeout != "" {
		if _, err := time.ParseDuration(c.TraceTimeout); err != nil {
			return nil, errors.Annotatef(err, "invalid trace_timeout")
		}
	}
	if _, err := c.MempoolTxTimeoutDuration(false); err != nil {
		return nil, err
	}
	if _, err := c.AlternativeMempoolTxTimeoutDuration(); err != nil {
		return nil, err
	}
	if _, err := c.AverageBlockTimeDuration(); err != nil {
		return nil, err
	}

	s := &EthereumRPC{
		BaseChain:   &bchain.BaseChain{},
		ChainConfig: &c,
	}
	// 1-slot buffer ensures we only queue one "refresh tip" signal at a time.
	s.newBlockNotifyCh = make(chan struct{}, 1)

	bchain.ProcessInternalTransactions = c.ProcessInternalTransactions

	// always create parser
	parser := NewEthereumParser(c.BlockAddressesToKeep, c.AddressAliases)
	parser.HotAddressMinContracts = c.HotAddressMinContracts
	parser.HotAddressLRUCacheSize = c.HotAddressLRUCacheSize
	parser.HotAddressMinHits = c.HotAddressMinHits
	parser.AddrContractsCacheMinSize = c.AddressContractsCacheMinSize
	parser.AddrContractsCacheMaxBytes = c.AddressContractsCacheMaxBytes
	parser.AddrContractsCacheBulkMaxBytes = c.AddressContractsCacheBulkMaxBytes
	s.Parser = parser
	if c.RPCTimeout <= 0 {
		glog.Warningf("rpc_timeout=%d is invalid, using default %d seconds", c.RPCTimeout, defaultRPCTimeoutSeconds)
		c.RPCTimeout = defaultRPCTimeoutSeconds
	}
	s.Timeout = time.Duration(c.RPCTimeout) * time.Second
	s.PushHandler = pushHandler

	return s, nil
}

func (b *EthereumRPC) SetMetrics(metrics *common.Metrics) {
	b.metrics = metrics
}

// AverageBlockTimeDuration exposes the chain's nominal block cadence.
func (b *EthereumRPC) AverageBlockTimeDuration() (time.Duration, error) {
	return b.ChainConfig.AverageBlockTimeDuration()
}

// MissingBlockRetryOverride exposes the per-chain sync-worker retry override
// (or nil to use built-in defaults). Consumed by blockbook.go at SyncWorker
// construction via a duck-typed interface assertion.
func (b *EthereumRPC) MissingBlockRetryOverride() *bchain.MissingBlockRetry {
	if b.ChainConfig == nil {
		return nil
	}
	return b.ChainConfig.MissingBlockRetry
}

func (b *EthereumRPC) observeEthCall(mode string, count int) {
	if b.metrics == nil || count <= 0 {
		return
	}
	b.metrics.EthCallRequests.With(common.Labels{"mode": mode}).Add(float64(count))
}

// ObserveChainDataFallback increments a metric for chain-data fallback paths.
func (b *EthereumRPC) ObserveChainDataFallback(component, reason string) {
	if b.metrics == nil || component == "" || reason == "" {
		return
	}
	b.metrics.ChainDataFallbacks.With(common.Labels{"component": component, "reason": reason}).Inc()
}

func (b *EthereumRPC) observeEthCallError(mode, errType string) {
	if b.metrics == nil {
		return
	}
	b.metrics.EthCallErrors.With(common.Labels{"mode": mode, "type": errType}).Inc()
}

func (b *EthereumRPC) observeEthCallBatch(size int) {
	if b.metrics == nil || size <= 0 {
		return
	}
	b.metrics.EthCallBatchSize.Observe(float64(size))
}

func (b *EthereumRPC) observeEthCallContractInfo(field string) {
	if b.metrics == nil {
		return
	}
	b.metrics.EthCallContractInfo.With(common.Labels{"field": field}).Inc()
}

func (b *EthereumRPC) observeEthCallTokenURI(method string) {
	if b.metrics == nil {
		return
	}
	b.metrics.EthCallTokenURI.With(common.Labels{"method": method}).Inc()
}

func (b *EthereumRPC) observeEthCallStakingPool(field string) {
	if b.metrics == nil {
		return
	}
	b.metrics.EthCallStakingPool.With(common.Labels{"field": field}).Inc()
}

func ethSyncRpcErrStatus(err error) string {
	if stdErrors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var httpErr rpc.HTTPError
	if stdErrors.As(err, &httpErr) {
		switch {
		case httpErr.StatusCode >= 500:
			return "http_5xx"
		case httpErr.StatusCode >= 400:
			return "http_4xx"
		default:
			return "http_other"
		}
	}
	var rpcErr rpc.Error
	if stdErrors.As(err, &rpcErr) {
		return "rpc_" + strconv.Itoa(rpcErr.ErrorCode())
	}
	return "error"
}

func (b *EthereumRPC) observeEthSyncRpcError(method string, err error) {
	if b.metrics == nil || err == nil {
		return
	}
	b.metrics.EthSyncRpcErrors.With(common.Labels{"method": method, "status": ethSyncRpcErrStatus(err)}).Inc()
}

// EnsureSameRPCHost validates both RPC URLs and logs a warning if hosts differ.
func EnsureSameRPCHost(httpURL, wsURL string) error {
	if httpURL == "" || wsURL == "" {
		return nil
	}
	httpHost, err := rpcURLHost(httpURL)
	if err != nil {
		return errors.Annotatef(err, "rpc_url")
	}
	wsHost, err := rpcURLHost(wsURL)
	if err != nil {
		return errors.Annotatef(err, "rpc_url_ws")
	}
	if !strings.EqualFold(httpHost, wsHost) {
		glog.Warningf("rpc_url host %q and rpc_url_ws host %q differ", httpHost, wsHost)
	}
	return nil
}

// NormalizeRPCURLs validates HTTP and WS RPC endpoints and enforces same-host rules.
func NormalizeRPCURLs(httpURL, wsURL string) (string, string, error) {
	callURL := strings.TrimSpace(httpURL)
	subURL := strings.TrimSpace(wsURL)
	if callURL == "" {
		return "", "", errors.New("rpc_url is empty")
	}
	if subURL == "" {
		return "", "", errors.New("rpc_url_ws is empty")
	}
	if err := validateRPCURLScheme(callURL, "rpc_url", []string{"http", "https"}); err != nil {
		return "", "", err
	}
	if err := validateRPCURLScheme(subURL, "rpc_url_ws", []string{"ws", "wss"}); err != nil {
		return "", "", err
	}
	if err := EnsureSameRPCHost(callURL, subURL); err != nil {
		return "", "", err
	}
	return callURL, subURL, nil
}

func validateRPCURLScheme(rawURL, field string, allowedSchemes []string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return errors.Annotatef(err, "%s", field)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme == "" {
		return errors.Errorf("%s missing scheme in %q", field, rawURL)
	}
	for _, allowed := range allowedSchemes {
		if scheme == allowed {
			return nil
		}
	}
	return errors.Errorf("%s must use %s scheme: %q", field, strings.Join(allowedSchemes, " or "), rawURL)
}

func rpcURLHost(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	host := parsed.Hostname()
	if host == "" {
		return "", errors.Errorf("missing host in %q", rawURL)
	}
	return host, nil
}

// dialTimeout bounds the initial RPC/WS handshake. A websocket backend behind a
// load balancer can accept the TCP socket but never complete the upgrade — the
// exact silent stall tipWatchdog exists to heal. Dialing with context.Background()
// then blocks forever, and because reconnectRPC runs on the lone tipWatchdog
// goroutine that single healer parks indefinitely: the cached tip stays frozen,
// resyncIndex keeps reporting a false syncNotNeeded, and sync silently stalls until
// a restart. go-ethereum uses this context only for the first handshake, so the
// established connection's lifetime is unaffected. A var so tests can shorten it.
var dialTimeout = 30 * time.Second

func dialRPC(rawURL string) (*rpc.Client, error) {
	if rawURL == "" {
		return nil, errors.New("empty rpc url")
	}
	opts := []rpc.ClientOption{}
	if strings.HasPrefix(rawURL, "ws://") || strings.HasPrefix(rawURL, "wss://") {
		opts = append(opts, rpc.WithWebsocketMessageSizeLimit(0))
	}
	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()
	return rpc.DialOptions(ctx, rawURL, opts...)
}

// OpenRPC opens RPC connection to ETH backend.
var OpenRPC = func(httpURL, wsURL string) (bchain.EVMRPCClient, bchain.EVMClient, error) {
	callURL, subURL, err := NormalizeRPCURLs(httpURL, wsURL)
	if err != nil {
		return nil, nil, err
	}
	callClient, err := dialRPC(callURL)
	if err != nil {
		return nil, nil, err
	}
	subClient := callClient
	if subURL != callURL {
		subClient, err = dialRPC(subURL)
		if err != nil {
			callClient.Close()
			return nil, nil, err
		}
	}
	rc := &DualRPCClient{CallClient: callClient, SubClient: subClient}
	ec := &EthereumClient{Client: ethclient.NewClient(callClient)}
	return rc, ec, nil
}

// Initialize initializes ethereum rpc interface
func (b *EthereumRPC) Initialize() error {
	b.OpenRPC = OpenRPC

	rc, ec, err := b.OpenRPC(b.ChainConfig.RPCURL, b.ChainConfig.RPCURLWS)
	if err != nil {
		return err
	}

	// set chain specific
	b.Client = ec
	b.RPC = rc
	b.MainNetChainID = MainNet
	b.NewBlock = &EthereumNewBlock{channel: make(chan *types.Header)}
	b.NewTx = &EthereumNewTx{channel: make(chan ethcommon.Hash)}

	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()

	id, err := b.Client.NetworkID(ctx)
	if err != nil {
		return err
	}

	// parameters for getInfo request
	switch Network(id.Uint64()) {
	case MainNet:
		b.Testnet = false
		b.Network = "livenet"
	case TestNetSepolia:
		b.Testnet = true
		b.Network = "sepolia"
	case TestNetHolesky:
		b.Testnet = true
		b.Network = "holesky"
	case TestNetHoodi:
		b.Testnet = true
		b.Network = "hoodi"
	default:
		return errors.Errorf("Unknown network id %v", id)
	}

	err = b.initStakingPools()
	if err != nil {
		return err
	}

	if err = b.InitAlternativeProviders(); err != nil {
		return err
	}

	b.consensusMonitor = newConsensusVersionMonitor(b.ChainConfig.ConsensusNodeVersionURL)
	b.consensusMonitor.start()

	glog.Info("rpc: block chain ", b.Network)

	return nil
}

const (
	consensusVersionUnreachable = "unreachable-locally"
	consensusVersionPollPeriod  = 60 * time.Second
)

// consensusVersionMonitor probes the configured consensus node /eth/v1/node/version
// endpoint and caches the latest result. The cached value (real version or
// "unreachable-locally") is the signal exposed via getInfo and the Prometheus
// backend_subversion label; periodic re-probes are silent so a node being
// down does not spam the log.
type consensusVersionMonitor struct {
	url      string
	mu       sync.RWMutex
	version  string
	stop     chan struct{}
	stopOnce sync.Once
}

func newConsensusVersionMonitor(url string) *consensusVersionMonitor {
	if url == "" {
		return nil
	}
	return &consensusVersionMonitor{url: url, stop: make(chan struct{})}
}

// start performs an initial synchronous probe (logging one WARN if it fails)
// and then launches a background goroutine that re-probes every
// consensusVersionPollPeriod. Safe to call on a nil receiver.
func (m *consensusVersionMonitor) start() {
	if m == nil {
		return
	}
	v, err := m.fetch()
	if err != nil {
		glog.Warningf("consensus node version probe failed for %s: %v", m.url, err)
		v = consensusVersionUnreachable
	}
	m.set(v)
	go m.run()
}

func (m *consensusVersionMonitor) run() {
	ticker := time.NewTicker(consensusVersionPollPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-m.stop:
			return
		case <-ticker.C:
			v, err := m.fetch()
			if err != nil {
				v = consensusVersionUnreachable
			}
			m.set(v)
		}
	}
}

func (m *consensusVersionMonitor) fetch() (string, error) {
	httpClient := &http.Client{Timeout: 2 * time.Second}
	resp, err := httpClient.Get(m.url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var v struct {
		Data struct {
			Version string `json:"version"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return "", err
	}
	return v.Data.Version, nil
}

func (m *consensusVersionMonitor) set(v string) {
	m.mu.Lock()
	m.version = v
	m.mu.Unlock()
}

func (m *consensusVersionMonitor) get() string {
	if m == nil {
		return ""
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.version
}

func (m *consensusVersionMonitor) shutdown() {
	if m == nil {
		return
	}
	m.stopOnce.Do(func() { close(m.stop) })
}

// InitAlternativeProviders initializes alternative providers
func (b *EthereumRPC) InitAlternativeProviders() error {
	b.initAlternativeFeeProvider()

	// Env prefix follows explicit network aliases such as OP/BASE, otherwise ETH.
	network := b.ChainConfig.Network
	if network == "" {
		network = b.ChainConfig.CoinShortcut
	}
	alternativeMempoolTxTimeout, err := b.ChainConfig.AlternativeMempoolTxTimeoutDuration()
	if err != nil {
		return err
	}
	b.alternativeSendTxProvider = NewAlternativeSendTxProvider(network, b.ChainConfig.RPCTimeout, alternativeMempoolTxTimeout)
	return nil
}

// CreateMempool creates mempool if not already created, however does not initialize it
func (b *EthereumRPC) CreateMempool(chain bchain.BlockChain) (bchain.Mempool, error) {
	if b.Mempool == nil {
		mempoolTxTimeout, err := b.ChainConfig.MempoolTxTimeoutDuration(b.alternativeSendTxProvider != nil)
		if err != nil {
			return nil, err
		}
		b.Mempool = bchain.NewMempoolEthereumType(chain, mempoolTxTimeout, b.ChainConfig.QueryBackendOnMempoolResync)
		glog.Info("mempool created, MempoolTxTimeout=", mempoolTxTimeout, ", QueryBackendOnMempoolResync=", b.ChainConfig.QueryBackendOnMempoolResync, ", DisableMempoolSync=", b.ChainConfig.DisableMempoolSync)
		if b.alternativeSendTxProvider != nil {
			b.alternativeSendTxProvider.SetupMempool(b.Mempool, b.removeTransactionFromMempool)
		}

	}
	return b.Mempool, nil
}

// InitializeMempool creates subscriptions to newHeads and newPendingTransactions
func (b *EthereumRPC) InitializeMempool(addrDescForOutpoint bchain.AddrDescForOutpointFunc, onNewTx bchain.OnNewTxFunc) error {
	if b.Mempool == nil {
		return errors.New("Mempool not created")
	}

	var err error
	var txs []string
	// get initial mempool transactions
	// workaround for an occasional `decoding block` error from getBlockRaw - try 3 times with a delay and then proceed
	for i := 0; i < 3; i++ {
		txs, err = b.GetMempoolTransactions()
		if err == nil {
			break
		}
		glog.Error("GetMempoolTransaction ", err)
		time.Sleep(time.Second * 5)
	}

	for _, txid := range txs {
		b.Mempool.AddTransactionToMempool(txid)
	}

	b.Mempool.OnNewTx = onNewTx

	if err = b.subscribeEvents(); err != nil {
		return err
	}

	b.mempoolInitialized = true

	return nil
}

func (b *EthereumRPC) subscribeEvents() error {
	// The tip notifier, tip watchdog and the NewBlock/NewTx channel readers bind to
	// the persistent channels, not to a specific connection, so start them exactly
	// once. reconnectRPC -> subscribeEvents then only re-creates the EthSubscribe
	// bound subscriptions below, instead of leaking a fresh reader set per reconnect.
	b.subscribeReadersOnce.Do(func() {
		go b.newBlockNotifier()
		go b.tipWatchdog()
		// new block notifications handling
		go func() {
			for {
				h, ok := b.NewBlock.Read()
				if !ok {
					break
				}
				// Advance the tip from the delivered header, not a re-query over
				// the load-balanced HTTP path (see onFeedHeader).
				b.onFeedHeader(h)
			}
		}()
		// new mempool transaction notifications handling
		if !b.ChainConfig.DisableMempoolSync {
			go func() {
				for {
					t, ok := b.NewTx.Read()
					if !ok {
						break
					}
					hex := t.Hex()
					if glog.V(2) {
						glog.Info("rpc: new tx ", hex)
					}
					added := b.Mempool.AddTransactionToMempool(hex)
					if added {
						b.PushHandler(bchain.NotificationNewTx)
					}
				}
			}()
		}
	})

	// new block subscription - re-created on every (re)connect
	if err := b.subscribe("newHeads", func() (bchain.EVMClientSubscription, error) {
		// invalidate the previous subscription - it is either the first one or there was an error
		b.newBlockSubscription = nil
		ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
		defer cancel()
		sub, err := b.RPC.EthSubscribe(ctx, b.NewBlock.Channel(), "newHeads")
		if err != nil {
			return nil, errors.Annotatef(err, "EthSubscribe newHeads")
		}
		b.newBlockSubscription = sub
		glog.Info("Subscribed to newHeads")
		return sub, nil
	}); err != nil {
		return err
	}

	// Arm lastSubNotifyNs at subscribe time, not only on the first tip advance.
	// Liveness is otherwise stamped only when a header advances the tip, so a
	// subscription that never delivers a usable header leaves it at 0 and keeps
	// tipWatchdog's lastNs == 0 gate closed forever: the cached tip never refreshes
	// and resyncIndex reports a silent syncNotNeeded. Seeding here lets a stalled
	// feed age past the threshold so the watchdog polls and reconnects.
	b.markSubscriptionAlive()

	if !b.ChainConfig.DisableMempoolSync {
		// new mempool transaction subscription - re-created on every (re)connect
		if err := b.subscribe("newPendingTransactions", func() (bchain.EVMClientSubscription, error) {
			// invalidate the previous subscription - it is either the first one or there was an error
			b.newTxSubscription = nil
			ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
			defer cancel()
			sub, err := b.RPC.EthSubscribe(ctx, b.NewTx.Channel(), "newPendingTransactions")
			if err != nil {
				return nil, errors.Annotatef(err, "EthSubscribe newPendingTransactions")
			}
			b.newTxSubscription = sub
			glog.Info("Subscribed to newPendingTransactions")
			return sub, nil
		}); err != nil {
			return err
		}
	}

	return nil
}

// subscribe subscribes notification and tries to resubscribe in case of error
func (b *EthereumRPC) subscribe(name string, f func() (bchain.EVMClientSubscription, error)) error {
	s, err := f()
	if err != nil {
		return err
	}
	go func() {
	Loop:
		for {
			// wait for error in subscription
			e := <-s.Err()
			// nil error means sub.Unsubscribe called, exit goroutine
			if e == nil {
				return
			}
			glog.Error("Subscription error ", name, ": ", e)
			b.ObserveSubscriptionEvent(name, "error")
			timer := time.NewTimer(time.Second * 2)
			// try in 2 second interval to resubscribe
			for {
				select {
				case e = <-s.Err():
					if e == nil {
						return
					}
				case <-timer.C:
					ns, err := f()
					if err == nil {
						// subscription successful, restart wait for next error
						b.ObserveSubscriptionEvent(name, "resubscribed")
						s = ns
						continue Loop
					}
					glog.Error("Resubscribe error ", name, ": ", err)
					b.ObserveSubscriptionEvent(name, "resubscribe_failed")
					timer.Reset(time.Second * 2)
				}
			}
		}
	}()
	return nil
}

func (b *EthereumRPC) initAlternativeFeeProvider() {
	var err error
	if b.ChainConfig.AlternativeEstimateFee == "1inch" {
		if b.alternativeFeeProvider, err = NewOneInchFeesProvider(b, b.ChainConfig.AlternativeEstimateFeeParams, b.metrics); err != nil {
			glog.Error("New1InchFeesProvider error ", err, " Reverting to default estimateFee functionality")
			// disable AlternativeEstimateFee logic
			b.alternativeFeeProvider = nil
		}
	} else if b.ChainConfig.AlternativeEstimateFee == "infura" {
		if b.alternativeFeeProvider, err = NewInfuraFeesProvider(b, b.ChainConfig.AlternativeEstimateFeeParams, b.metrics); err != nil {
			glog.Error("NewInfuraFeesProvider error ", err, " Reverting to default estimateFee functionality")
			// disable AlternativeEstimateFee logic
			b.alternativeFeeProvider = nil
		}
	}
	if b.alternativeFeeProvider != nil {
		glog.Info("Using alternative fee provider ", b.ChainConfig.AlternativeEstimateFee)
	}

}

func (b *EthereumRPC) closeRPC() {
	if b.newBlockSubscription != nil {
		b.newBlockSubscription.Unsubscribe()
	}
	if b.newTxSubscription != nil {
		b.newTxSubscription.Unsubscribe()
	}
	if b.RPC != nil {
		b.RPC.Close()
	}
}

// CloseRPC closes the underlying RPC client, aborting any in-flight calls.
// Exported so embedders (e.g. Tron) can abort sync RPCs on shutdown without
// running the EVM-specific subscription/monitor teardown done by Shutdown.
func (b *EthereumRPC) CloseRPC() {
	b.closeRPC()
}

func (b *EthereumRPC) reconnectRPC() error {
	glog.Info("Reconnecting RPC")
	b.closeRPC()
	rc, ec, err := b.OpenRPC(b.ChainConfig.RPCURL, b.ChainConfig.RPCURLWS)
	if err != nil {
		return err
	}
	b.RPC = rc
	b.Client = ec
	return b.subscribeEvents()
}

// Shutdown cleans up rpc interface to ethereum
func (b *EthereumRPC) Shutdown(ctx context.Context) error {
	b.closeRPC()
	b.NewBlock.Close()
	b.NewTx.Close()
	b.consensusMonitor.shutdown()
	glog.Info("rpc: shutdown")
	return nil
}

// GetCoinName returns coin name
func (b *EthereumRPC) GetCoinName() string {
	return b.ChainConfig.CoinName
}

// GetSubversion returns empty string, ethereum does not have subversion
func (b *EthereumRPC) GetSubversion() string {
	return ""
}

// GetChainInfo returns information about the connected backend
func (b *EthereumRPC) GetChainInfo() (*bchain.ChainInfo, error) {
	h, err := b.getBestHeader()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	id, err := b.Client.NetworkID(ctx)
	if err != nil {
		return nil, err
	}
	var ver string
	if err := b.RPC.CallContext(ctx, &ver, "web3_clientVersion"); err != nil {
		return nil, err
	}
	rv := &bchain.ChainInfo{
		Blocks:           int(h.Number().Int64()),
		Bestblockhash:    h.Hash(),
		Difficulty:       h.Difficulty().String(),
		Version:          ver,
		ConsensusVersion: b.consensusMonitor.get(),
	}
	idi := int(id.Uint64())
	if idi == int(b.MainNetChainID) {
		rv.Chain = "mainnet"
	} else {
		rv.Chain = "testnet " + strconv.Itoa(idi)
	}
	return rv, nil
}

func (b *EthereumRPC) getBestHeader() (bchain.EVMHeader, error) {
	b.bestHeaderLock.Lock()
	defer b.bestHeaderLock.Unlock()
	// Subscription liveness (detecting a silently stalled newHeads feed and
	// reconnecting) is owned by tipWatchdog, which runs off the bestHeaderLock so a
	// reconnect can no longer block every concurrent tip reader. Here we only lazily
	// fetch the very first header; afterwards the cache is advanced by the
	// subscription-driven newBlockNotifier and by the watchdog's fallback poll.
	if b.bestHeader == nil {
		var err error
		ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
		defer cancel()
		b.bestHeader, err = b.Client.HeaderByNumber(ctx, nil)
		if err != nil {
			b.bestHeader = nil
			return nil, err
		}
	}
	return b.bestHeader, nil
}

// UpdateBestHeader keeps track of the latest block header confirmed on chain.
// Non-monotonic: callers (Tron's ZeroMQ feed) own their ordering/reorg handling.
func (b *EthereumRPC) UpdateBestHeader(h bchain.EVMHeader) {
	if h == nil || h.Number() == nil {
		return
	}
	glog.V(2).Info("rpc: new block header ", h.Number().Uint64())
	b.setBestHeader(h, false)
}

func (b *EthereumRPC) signalNewBlock() {
	// Non-blocking send: one pending signal is enough to wake the sync loop.
	select {
	case b.newBlockNotifyCh <- struct{}{}:
	default:
	}
}

// onFeedHeader advances the cached tip from the header the newHeads feed just
// delivered (not a re-query over HTTP) and, only on a real advance, refreshes
// liveness and wakes the sync loop. Behind a load balancer an HTTP re-query can
// hit a lagging node and report a stale tip, freezing sync into a false "synced"
// while newHeads still flows; the feed's header is authoritative. The update is
// monotonic so a resubscribe onto a behind node cannot regress the tip.
func (b *EthereumRPC) onFeedHeader(h bchain.EVMHeader) {
	if b.setBestHeader(h, true) {
		b.markSubscriptionAlive()
		b.signalNewBlock()
	}
}

// newBlockNotifier wakes the sync loop after onFeedHeader advanced the tip. It is
// decoupled from the reader via newBlockNotifyCh so a slow PushHandler cannot
// stall the reader and back the newHeads channel up.
func (b *EthereumRPC) newBlockNotifier() {
	for range b.newBlockNotifyCh {
		b.PushHandler(bchain.NotificationNewBlock)
	}
}

const (
	// tipWatchdogStaleBlocks scales the silent-stall window to the chain's cadence:
	// if no newHeads notification arrives for this many nominal block intervals, the
	// subscription is presumed dead and the watchdog heals it. Behind a load
	// balancer a newHeads feed can stop delivering with no error on sub.Err(), so a
	// purely error-driven resubscribe never fires.
	tipWatchdogStaleBlocks = 30
	// tipWatchdogMinStale / tipWatchdogMaxStale clamp the derived window so fast
	// chains do not react to routine jitter and slow/misconfigured chains still
	// recover in bounded time (the previous behaviour was a fixed 15 minutes, which
	// on Polygon's 2s blocks meant ~450 missed blocks before any reaction).
	tipWatchdogMinStale = 30 * time.Second
	tipWatchdogMaxStale = 5 * time.Minute
	// tipWatchdogMinInterval / tipWatchdogMaxInterval bound the sampling cadence.
	tipWatchdogMinInterval = 5 * time.Second
	tipWatchdogMaxInterval = 60 * time.Second
)

// ObserveSubscriptionEvent records a push-subscription lifecycle event. Exported
// so embedders with their own notification feed (e.g. Tron's ZeroMQ) emit the
// same metric.
func (b *EthereumRPC) ObserveSubscriptionEvent(subscription, event string) {
	if b.metrics == nil {
		return
	}
	b.metrics.BackendSubscriptionEvents.With(common.Labels{"subscription": subscription, "event": event}).Inc()
}

// SetSubscriptionAgeSeconds records the age of the newest notification from the
// tip feed. Exported for embedders that run their own watchdog.
func (b *EthereumRPC) SetSubscriptionAgeSeconds(seconds float64) {
	if b.metrics == nil {
		return
	}
	b.metrics.BackendSubscriptionAgeSeconds.Set(seconds)
}

// markSubscriptionAlive records that the feed just advanced the cached tip — the
// signal tipWatchdog uses to tell a live, progressing feed from one that went
// silent or got stuck on a single height.
func (b *EthereumRPC) markSubscriptionAlive() {
	b.lastSubNotifyNs.Store(time.Now().UnixNano())
}

// TipStaleThreshold derives the silent-feed window from the chain's average block
// time, clamped to a sane range. Exported so embedders (Tron, Avalanche) size
// their watchdog window with the same policy.
func (b *EthereumRPC) TipStaleThreshold() time.Duration {
	avg := time.Duration(b.ChainConfig.AverageBlockTimeMs) * time.Millisecond
	if avg <= 0 {
		return tipWatchdogMaxStale
	}
	d := tipWatchdogStaleBlocks * avg
	if d < tipWatchdogMinStale {
		return tipWatchdogMinStale
	}
	if d > tipWatchdogMaxStale {
		return tipWatchdogMaxStale
	}
	return d
}

// tipWatchdog detects a newHeads subscription that has silently stopped
// delivering (common behind load balancers, which can drop the upstream without
// signalling sub.Err()). On a stall it first polls the tip directly so sync keeps
// progressing instead of trusting a frozen cached tip as "synced", then reconnects
// to restore push delivery. It is started exactly once via subscribeReadersOnce.
func (b *EthereumRPC) tipWatchdog() {
	threshold := b.TipStaleThreshold()
	interval := threshold / 3
	if interval < tipWatchdogMinInterval {
		interval = tipWatchdogMinInterval
	}
	if interval > tipWatchdogMaxInterval {
		interval = tipWatchdogMaxInterval
	}
	glog.Infof("rpc: tip watchdog started, stall threshold %s, sampling every %s", threshold, interval)
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
// it is unit-testable with an injected threshold and a fake client (no 30s wait).
func (b *EthereumRPC) tipWatchdogTick(threshold time.Duration) {
	// Heartbeat first: this Inc proves the lone watchdog goroutine is still ticking.
	// If it ever parks (e.g. a hung reconnect), the counter stops and
	// rate(blockbook_backend_subscription_events{event="watchdog_tick"}) drops to 0 —
	// the only positive liveness signal for the sole feed-liveness healer, distinct
	// from watchdog_stall/watchdog_reconnect which fire only on an already-seen stall.
	b.ObserveSubscriptionEvent("newHeads", "watchdog_tick")
	// lastSubNotifyNs is armed when subscribeEvents establishes the newHeads
	// subscription and refreshed on every tip advance, so a non-zero value means
	// "subscription is wired up". The zero guard only skips the brief window before
	// the first subscribe (i.e. before InitializeMempool runs); it must not be the
	// sole arming signal, or a feed that never advances would keep the watchdog off.
	lastNs := b.lastSubNotifyNs.Load()
	if lastNs == 0 {
		return
	}
	age := time.Since(time.Unix(0, lastNs))
	b.SetSubscriptionAgeSeconds(age.Seconds())
	if age < threshold {
		return
	}
	glog.Warningf("rpc: newHeads subscription silent for %s (threshold %s); polling tip and reconnecting", age.Truncate(time.Second), threshold)
	b.ObserveSubscriptionEvent("newHeads", "watchdog_stall")
	// Keep sync alive immediately: poll the canonical tip directly so a dead push
	// channel can no longer freeze the cached tip into a false "synced". The poll is
	// allowed to regress the tip here (unlike the hot path): after a sustained stall
	// a lower backend height is a real rollback, not the transient load-balancer lag
	// the monotonic guard filters (that lag resolves well within the stall window).
	// Without this, a genuine rollback would leave the cached tip pinned above the
	// backend and equal to the local DB tip, so resyncIndex keeps early-exiting as
	// "synced" and never reaches its fork path (db/sync.go GetBlockHash check).
	prevHeight := b.cachedTipHeight()
	if updated, err := b.refreshBestHeaderFromChain(true); err != nil {
		glog.Error("rpc: tip watchdog tip poll error ", err)
	} else if updated {
		if newHeight := b.cachedTipHeight(); newHeight < prevHeight {
			glog.Warningf("rpc: tip watchdog observed backend rollback, cached tip %d -> %d; letting sync reconcile the fork", prevHeight, newHeight)
			b.ObserveSubscriptionEvent("newHeads", "watchdog_tip_rollback")
		} else {
			b.ObserveSubscriptionEvent("newHeads", "watchdog_tip_advanced")
		}
		b.PushHandler(bchain.NotificationNewBlock)
	}
	// Restore push delivery by reconnecting the RPC and re-subscribing.
	if err := b.reconnectRPC(); err != nil {
		glog.Error("rpc: tip watchdog reconnect error ", err)
		b.ObserveSubscriptionEvent("rpc", "watchdog_reconnect_failed")
		return
	}
	b.ObserveSubscriptionEvent("rpc", "watchdog_reconnect")
	// Give the fresh subscription a full window before judging it again so a
	// flapping backend cannot trigger a reconnect storm.
	b.markSubscriptionAlive()
}

// refreshBestHeaderFromChain polls the tip over HTTP. It is the watchdog's
// fallback when the push feed is silent (no longer on the hot path).
//
// allowRegress controls the monotonic guard. Callers on the hot path pass false so
// a lagging load-balancer node cannot regress the tip and trip a spurious fork. The
// watchdog passes true: it only polls after a sustained stall (TipStaleThreshold),
// by which point transient routing lag has resolved, so a still-lower backend tip
// is a genuine rollback the cached tip must follow down — otherwise the guard pins
// the tip above the backend and resyncIndex keeps reporting a false "synced".
func (b *EthereumRPC) refreshBestHeaderFromChain(allowRegress bool) (bool, error) {
	if b.Client == nil {
		return false, errors.New("rpc client not initialized")
	}
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	h, err := b.Client.HeaderByNumber(ctx, nil)
	if err != nil {
		return false, err
	}
	if h == nil || h.Number() == nil {
		return false, errors.New("best header is nil")
	}
	return b.setBestHeader(h, !allowRegress), nil
}

// setBestHeader stores h as the cached tip and reports whether it changed (new
// height, or same-height hash change i.e. a tip reorg). When monotonic, a lower
// height is rejected so a lagging load-balancer node cannot regress the tip and
// trip a spurious fork. A sustained real rollback (the backend genuinely below the
// cached tip past TipStaleThreshold) is instead recovered by tipWatchdog, which
// re-polls with the guard lifted so the tip follows the backend down and resyncIndex
// reaches its fork path; see refreshBestHeaderFromChain.
func (b *EthereumRPC) setBestHeader(h bchain.EVMHeader, monotonic bool) bool {
	if h == nil || h.Number() == nil {
		return false
	}
	b.bestHeaderLock.Lock()
	defer b.bestHeaderLock.Unlock()
	if b.bestHeader != nil && b.bestHeader.Number() != nil {
		prevNum := b.bestHeader.Number().Uint64()
		newNum := h.Number().Uint64()
		if newNum == prevNum && b.bestHeader.Hash() == h.Hash() {
			return false // identical tip: not progress
		}
		if monotonic && newNum < prevNum {
			return false // lagging node: keep the higher tip
		}
	}
	b.bestHeader = h
	return true
}

// cachedTipHeight returns the height of the cached tip, or 0 if it is unset. The
// watchdog uses it to tell a forward advance from a rollback for logging/metrics.
func (b *EthereumRPC) cachedTipHeight() uint64 {
	b.bestHeaderLock.Lock()
	defer b.bestHeaderLock.Unlock()
	if b.bestHeader == nil || b.bestHeader.Number() == nil {
		return 0
	}
	return b.bestHeader.Number().Uint64()
}

// GetBestBlockHash returns hash of the tip of the best-block-chain
func (b *EthereumRPC) GetBestBlockHash() (string, error) {
	h, err := b.getBestHeader()
	if err != nil {
		return "", err
	}
	return h.Hash(), nil
}

// GetBestBlockHeight returns height of the tip of the best-block-chain
func (b *EthereumRPC) GetBestBlockHeight() (uint32, error) {
	h, err := b.getBestHeader()
	if err != nil {
		return 0, err
	}
	return uint32(h.Number().Uint64()), nil
}

// GetBlockHash returns hash of block in best-block-chain at given height
func (b *EthereumRPC) GetBlockHash(height uint32) (string, error) {
	var n big.Int
	n.SetUint64(uint64(height))
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	h, err := b.Client.HeaderByNumber(ctx, &n)
	if err != nil {
		if err == ethereum.NotFound || stdErrors.Is(err, bchain.ErrBlockNotFound) {
			return "", bchain.ErrBlockNotFound
		}
		return "", errors.Annotatef(err, "height %v", height)
	}
	return h.Hash(), nil
}

func (b *EthereumRPC) ethHeaderToBlockHeader(h *rpcHeader) (*bchain.BlockHeader, error) {
	height, err := ethNumber(h.Number)
	if err != nil {
		return nil, err
	}
	c, err := b.computeConfirmations(uint64(height))
	if err != nil {
		return nil, err
	}
	time, err := ethNumber(h.Time)
	if err != nil {
		return nil, err
	}
	size, err := ethNumber(h.Size)
	if err != nil {
		return nil, err
	}
	return &bchain.BlockHeader{
		Hash:          h.Hash,
		Prev:          h.ParentHash,
		Height:        uint32(height),
		Confirmations: int(c),
		Time:          time,
		Size:          int(size),
	}, nil
}

// GetBlockHeader returns header of block with given hash
func (b *EthereumRPC) GetBlockHeader(hash string) (*bchain.BlockHeader, error) {
	raw, err := b.getBlockRaw(hash, 0, false)
	if err != nil {
		return nil, errors.Annotatef(err, "hash %v", hash)
	}
	var h rpcHeader
	if err := json.Unmarshal(raw, &h); err != nil {
		return nil, errors.Annotatef(err, "hash %v", hash)
	}
	return b.ethHeaderToBlockHeader(&h)
}

func (b *EthereumRPC) computeConfirmations(n uint64) (uint32, error) {
	bh, err := b.getBestHeader()
	if err != nil {
		return 0, err
	}
	bn := bh.Number().Uint64()
	// transaction in the best block has 1 confirmation
	return uint32(bn - n + 1), nil
}

func (b *EthereumRPC) getBlockRaw(hash string, height uint32, fullTxs bool) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	var raw json.RawMessage
	var err error
	var method string
	if hash != "" {
		if hash == "pending" {
			method = "eth_getBlockByNumber"
			err = b.RPC.CallContext(ctx, &raw, method, hash, fullTxs)
		} else {
			method = "eth_getBlockByHash"
			err = b.RPC.CallContext(ctx, &raw, method, ethcommon.HexToHash(hash), fullTxs)
		}
	} else {
		method = "eth_getBlockByNumber"
		err = b.RPC.CallContext(ctx, &raw, method, fmt.Sprintf("%#x", height), fullTxs)
	}
	b.observeEthSyncRpcError(method, err)
	if err != nil {
		return nil, errors.Annotatef(err, "hash %v, height %v", hash, height)
	} else if len(raw) == 0 || (len(raw) == 4 && string(raw) == "null") {
		return nil, bchain.ErrBlockNotFound
	}
	return raw, nil
}

// GetBlockRawByHashOrHeight returns raw block JSON by hash or height.
func (b *EthereumRPC) GetBlockRawByHashOrHeight(hash string, height uint32, fullTxs bool) (json.RawMessage, error) {
	return b.getBlockRaw(hash, height, fullTxs)
}

func (b *EthereumRPC) processEventsForBlock(blockNumber string) (map[string][]*bchain.RpcLog, []bchain.AddressAliasRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	var logs []rpcLogWithTxHash
	var ensRecords []bchain.AddressAliasRecord
	var method = "eth_getLogs"
	err := b.RPC.CallContext(ctx, &logs, method, map[string]interface{}{
		"fromBlock": blockNumber,
		"toBlock":   blockNumber,
	})
	b.observeEthSyncRpcError(method, err)
	if err != nil {
		return nil, nil, errors.Annotatef(err, "%s blockNumber %v", method, blockNumber)
	}
	r := make(map[string][]*bchain.RpcLog)
	for i := range logs {
		l := &logs[i]
		r[l.Hash] = append(r[l.Hash], &l.RpcLog)
		ens := getEnsRecord(l)
		if ens != nil {
			ensRecords = append(ensRecords, *ens)
		}
	}
	return r, ensRecords, nil
}

type rpcCallTrace struct {
	// CREATE, CREATE2, SELFDESTRUCT, CALL, CALLCODE, DELEGATECALL, STATICCALL
	Type   string         `json:"type"`
	From   string         `json:"from"`
	To     string         `json:"to"`
	Value  string         `json:"value"`
	Error  string         `json:"error"`
	Output string         `json:"output"`
	Calls  []rpcCallTrace `json:"calls"`
}

type rpcTraceResult struct {
	Result rpcCallTrace `json:"result"`
}

func (b *EthereumRPC) getCreationContractInfo(contract string, height uint32) *bchain.ContractInfo {
	// do not fetch fetchContractInfo in sync, it slows it down
	// the contract will be fetched only when asked by a client
	// ci, err := b.fetchContractInfo(contract)
	// if ci == nil || err != nil {
	ci := &bchain.ContractInfo{
		Contract: contract,
	}
	// }
	ci.Standard = bchain.UnhandledTokenStandard
	ci.Type = bchain.UnhandledTokenStandard
	ci.CreatedInBlock = height
	return ci
}

func (b *EthereumRPC) processCallTrace(call *rpcCallTrace, d *bchain.EthereumInternalData, contracts []bchain.ContractInfo, blockHeight uint32) []bchain.ContractInfo {
	value, err := hexutil.DecodeBig(call.Value)
	if err != nil {
		value = new(big.Int)
	}
	if call.Type == "CREATE" || call.Type == "CREATE2" {
		d.Transfers = append(d.Transfers, bchain.EthereumInternalTransfer{
			Type:  bchain.CREATE,
			Value: *value,
			From:  call.From,
			To:    call.To, // new contract address
		})
		contracts = append(contracts, *b.getCreationContractInfo(call.To, blockHeight))
	} else if call.Type == "SELFDESTRUCT" {
		d.Transfers = append(d.Transfers, bchain.EthereumInternalTransfer{
			Type:  bchain.SELFDESTRUCT,
			Value: *value,
			From:  call.From, // destroyed contract address
			To:    call.To,
		})
		contracts = append(contracts, bchain.ContractInfo{Contract: call.From, DestructedInBlock: blockHeight})
	} else if call.Type == "DELEGATECALL" {
		// ignore DELEGATECALL (geth v1.11 the changed tracer behavior)
		// 	https://github.com/ethereum/go-ethereum/issues/26726
	} else if err == nil && (value.BitLen() > 0 || b.ChainConfig.ProcessZeroInternalTransactions) {
		d.Transfers = append(d.Transfers, bchain.EthereumInternalTransfer{
			Value: *value,
			From:  call.From,
			To:    call.To,
		})
	}
	if call.Error != "" {
		d.Error = call.Error
	}
	for i := range call.Calls {
		contracts = b.processCallTrace(&call.Calls[i], d, contracts, blockHeight)
	}
	return contracts
}

// getInternalDataForBlock fetches debug trace using callTracer, extracts internal transfers/creations/destructions; ctx controls cancellation.
func (b *EthereumRPC) getInternalDataForBlock(ctx context.Context, blockHash string, blockHeight uint32, transactions []bchain.RpcTransaction) ([]bchain.EthereumInternalData, []bchain.ContractInfo, error) {
	if b.InternalDataProvider != nil {
		return b.InternalDataProvider.GetInternalDataForBlock(blockHash, blockHeight, transactions)
	}

	data := make([]bchain.EthereumInternalData, len(transactions))
	contracts := make([]bchain.ContractInfo, 0)
	if bchain.ProcessInternalTransactions {
		var trace []rpcTraceResult
		traceConfig := map[string]interface{}{"tracer": "callTracer"}
		if b.ChainConfig.TraceTimeout != "" {
			traceConfig["timeout"] = b.ChainConfig.TraceTimeout
		}
		err := b.RPC.CallContext(ctx, &trace, "debug_traceBlockByHash", blockHash, traceConfig) // Use caller-provided ctx for timeout/cancel.
		b.observeEthSyncRpcError("debug_traceBlockByHash", err)
		if err != nil {
			glog.Error("debug_traceBlockByHash block ", blockHash, ", error ", err)
			return data, contracts, err
		}
		if len(trace) != len(data) {
			if len(trace) < len(data) {
				for i := range transactions {
					tx := &transactions[i]
					// bridging transactions in Polygon do not create trace and cause mismatch between the trace size and block size, it is necessary to adjust the trace size
					// bridging transaction that from and to zero address
					if tx.To == "0x0000000000000000000000000000000000000000" && tx.From == "0x0000000000000000000000000000000000000000" {
						if i >= len(trace) {
							trace = append(trace, rpcTraceResult{})
						} else {
							trace = append(trace[:i+1], trace[i:]...)
							trace[i] = rpcTraceResult{}
						}
					}
				}
			}
			if len(trace) != len(data) {
				e := fmt.Sprint("trace length does not match block length ", len(trace), "!=", len(data))
				glog.Error("debug_traceBlockByHash block ", blockHash, ", error: ", e)
				return data, contracts, errors.New(e)
			} else {
				glog.Warning("debug_traceBlockByHash block ", blockHash, ", trace adjusted to match the number of transactions in block")
			}
		}
		for i, result := range trace {
			r := &result.Result
			d := &data[i]
			if r.Type == "CREATE" || r.Type == "CREATE2" {
				d.Type = bchain.CREATE
				d.Contract = r.To
				contracts = append(contracts, *b.getCreationContractInfo(d.Contract, blockHeight))
			} else if r.Type == "SELFDESTRUCT" {
				d.Type = bchain.SELFDESTRUCT
			}
			for j := range r.Calls {
				contracts = b.processCallTrace(&r.Calls[j], d, contracts, blockHeight)
			}
			if r.Error != "" {
				baseError := PackInternalTransactionError(r.Error)
				if len(baseError) > 1 {
					// n, _ := ethNumber(transactions[i].BlockNumber)
					// glog.Infof("Internal Data Error %d %s: unknown base error %s", n, transactions[i].Hash, baseError)
					baseError = strings.ToUpper(baseError[:1]) + baseError[1:] + ". "
				}
				outputError := ParseErrorFromOutput(r.Output)
				if len(outputError) > 0 {
					d.Error = baseError + strings.ToUpper(outputError[:1]) + outputError[1:]
				} else {
					traceError := PackInternalTransactionError(d.Error)
					if traceError == baseError {
						d.Error = baseError
					} else {
						d.Error = baseError + traceError
					}
				}
				// n, _ := ethNumber(transactions[i].BlockNumber)
				// glog.Infof("Internal Data Error %d %s: %s", n, transactions[i].Hash, UnpackInternalTransactionError([]byte(d.Error)))
			}
		}
	}
	return data, contracts, nil
}

// GetBlock returns block with given hash or height, hash has precedence if both passed
func (b *EthereumRPC) GetBlock(hash string, height uint32) (*bchain.Block, error) {
	raw, err := b.getBlockRaw(hash, height, true)
	if err != nil {
		return nil, err
	}
	var block struct {
		rpcHeader            // Embed to unmarshal header and txs in one pass.
		rpcBlockTransactions // Embed to avoid a second JSON decode.
	}
	if err := json.Unmarshal(raw, &block); err != nil { // Single decode to reduce CPU overhead.
		return nil, errors.Annotatef(err, "hash %v, height %v", hash, height)
	}
	head := block.rpcHeader
	body := block.rpcBlockTransactions
	bbh, err := b.ethHeaderToBlockHeader(&head)
	if err != nil {
		return nil, errors.Annotatef(err, "hash %v, height %v", hash, height)
	}
	// Run event/log processing and internal data extraction in parallel; allow early return on log failure.
	ctxInternal, cancelInternal := context.WithTimeout(context.Background(), b.Timeout) // Cancel trace RPC on log error or timeout.
	defer cancelInternal()                                                              // Ensure timer resources are released on any return path.
	type logsResult struct {                                                            // Bundles processEventsForBlock outputs for channel return.
		logs map[string][]*bchain.RpcLog
		ens  []bchain.AddressAliasRecord
		err  error
	}
	type internalResult struct { // Bundles getInternalDataForBlock outputs for channel return.
		data      []bchain.EthereumInternalData
		contracts []bchain.ContractInfo
		err       error
	}
	logsCh := make(chan logsResult, 1)         // Buffered so send won't block if we return early.
	internalCh := make(chan internalResult, 1) // Buffered to avoid goroutine leak on early return.
	go func() {
		logs, ens, err := b.processEventsForBlock(head.Number)
		logsCh <- logsResult{logs: logs, ens: ens, err: err} // Send result without shared state.
	}()
	go func() {
		data, contracts, err := b.getInternalDataForBlock(ctxInternal, head.Hash, bbh.Height, body.Transactions) // ctxInternal allows cancellation on log errors.
		internalCh <- internalResult{data: data, contracts: contracts, err: err}                                 // Send result without shared state.
	}()
	logsRes := <-logsCh
	if logsRes.err != nil {
		// Short-circuit on log failure to preserve existing error behavior.
		return nil, logsRes.err
	}
	internalRes := <-internalCh
	// Rebind results to keep downstream logic unchanged.
	logs := logsRes.logs
	ens := logsRes.ens
	internalData := internalRes.data
	contracts := internalRes.contracts
	internalErr := internalRes.err
	// error fetching internal data does not stop the block processing
	var blockSpecificData *bchain.EthereumBlockSpecificData
	// pass internalData error and ENS records in blockSpecificData to be stored
	if internalErr != nil || len(ens) > 0 || len(contracts) > 0 {
		blockSpecificData = &bchain.EthereumBlockSpecificData{}
		if internalErr != nil {
			blockSpecificData.InternalDataError = internalErr.Error()
			// glog.Info("InternalDataError ", bbh.Height, ": ", internalErr.Error())
		}
		if len(ens) > 0 {
			blockSpecificData.AddressAliasRecords = ens
			// glog.Info("ENS", ens)
		}
		if len(contracts) > 0 {
			blockSpecificData.Contracts = contracts
			// glog.Info("Contracts", contracts)
		}
	}

	btxs := make([]bchain.Tx, len(body.Transactions))
	for i := range body.Transactions {
		tx := &body.Transactions[i]
		btx, err := b.Parser.EthTxToTx(tx, &bchain.RpcReceipt{Logs: logs[tx.Hash]}, &internalData[i], bbh.Time, uint32(bbh.Confirmations), true)
		if err != nil {
			return nil, errors.Annotatef(err, "hash %v, height %v, txid %v", hash, height, tx.Hash)
		}
		btxs[i] = *btx
		b.removeTransactionFromMempool(tx.Hash)
	}
	bbk := bchain.Block{
		BlockHeader:      *bbh,
		Txs:              btxs,
		CoinSpecificData: blockSpecificData,
	}
	return &bbk, nil
}

// GetBlockInfo returns extended header (more info than in bchain.BlockHeader) with a list of txids
func (b *EthereumRPC) GetBlockInfo(hash string) (*bchain.BlockInfo, error) {
	raw, err := b.getBlockRaw(hash, 0, false)
	if err != nil {
		return nil, err
	}
	var head rpcHeader
	var txs rpcBlockTxids
	if err := json.Unmarshal(raw, &head); err != nil {
		return nil, errors.Annotatef(err, "hash %v", hash)
	}
	if err = json.Unmarshal(raw, &txs); err != nil {
		return nil, err
	}
	bch, err := b.ethHeaderToBlockHeader(&head)
	if err != nil {
		return nil, err
	}
	return &bchain.BlockInfo{
		BlockHeader: *bch,
		Difficulty:  common.JSONNumber(head.Difficulty),
		Nonce:       common.JSONNumber(head.Nonce),
		Txids:       txs.Transactions,
	}, nil
}

// GetTransactionForMempool returns a transaction by the transaction ID.
// It could be optimized for mempool, i.e. without block time and confirmations
func (b *EthereumRPC) GetTransactionForMempool(txid string) (*bchain.Tx, error) {
	return b.GetTransaction(txid)
}

func (b *EthereumRPC) removeTransactionFromMempool(txid string) {
	// remove tx from mempool
	if b.mempoolInitialized {
		b.Mempool.RemoveTransactionFromMempool(txid)
	}
	// remove tx from mempool txs fetched by alternative method
	if b.alternativeSendTxProvider != nil {
		b.alternativeSendTxProvider.RemoveTransaction(txid)
	}
}

// GetTransaction returns a transaction by the transaction ID.
func (b *EthereumRPC) GetTransaction(txid string) (*bchain.Tx, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	var tx *bchain.RpcTransaction
	var txFound bool
	var err error
	hash := ethcommon.HexToHash(txid)
	if b.alternativeSendTxProvider != nil {
		tx, txFound = b.alternativeSendTxProvider.GetTransaction(txid)
	}
	if !txFound {
		tx = &bchain.RpcTransaction{}
		err = b.RPC.CallContext(ctx, tx, "eth_getTransactionByHash", hash)
		if err != nil {
			return nil, err
		}
	}
	if *tx == (bchain.RpcTransaction{}) {
		b.removeTransactionFromMempool(txid)
		return nil, bchain.ErrTxNotFound
	}
	var btx *bchain.Tx
	if tx.BlockNumber == "" {
		// mempool tx
		btx, err = b.Parser.EthTxToTx(tx, nil, nil, 0, 0, true)
		if err != nil {
			return nil, errors.Annotatef(err, "txid %v", txid)
		}
	} else {
		// non mempool tx - read the block header to get the block time
		raw, err := b.getBlockRaw(tx.BlockHash, 0, false)
		if err != nil {
			return nil, err
		}
		var ht struct {
			Time          string `json:"timestamp"`
			BaseFeePerGas string `json:"baseFeePerGas"`
		}
		if err := json.Unmarshal(raw, &ht); err != nil {
			return nil, errors.Annotatef(err, "hash %v", hash)
		}
		var time int64
		if time, err = ethNumber(ht.Time); err != nil {
			return nil, errors.Annotatef(err, "txid %v", txid)
		}
		tx.BaseFeePerGas = ht.BaseFeePerGas
		receipt, err := b.EthereumTypeGetTransactionReceipt(txid)
		if err != nil {
			return nil, errors.Annotatef(err, "txid %v", txid)
		}
		n, err := ethNumber(tx.BlockNumber)
		if err != nil {
			return nil, errors.Annotatef(err, "txid %v", txid)
		}
		confirmations, err := b.computeConfirmations(uint64(n))
		if err != nil {
			return nil, errors.Annotatef(err, "txid %v", txid)
		}
		btx, err = b.Parser.EthTxToTx(tx, receipt, nil, time, confirmations, true)
		if err != nil {
			return nil, errors.Annotatef(err, "txid %v", txid)
		}
		b.removeTransactionFromMempool(txid)
	}
	return btx, nil
}

// GetTransactionSpecific returns json as returned by backend, with all coin specific data
func (b *EthereumRPC) GetTransactionSpecific(tx *bchain.Tx) (json.RawMessage, error) {
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
	m, err := json.Marshal(&csd)
	return json.RawMessage(m), err
}

// GetMempoolTransactions returns transactions in mempool
func (b *EthereumRPC) GetMempoolTransactions() ([]string, error) {
	raw, err := b.getBlockRaw("pending", 0, false)
	if err != nil {
		return nil, err
	}
	var body rpcBlockTxids
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &body); err != nil {
			return nil, err
		}
	}
	return body.Transactions, nil
}

// EstimateFee returns fee estimation
func (b *EthereumRPC) EstimateFee(blocks int) (big.Int, error) {
	return b.EstimateSmartFee(blocks, true)
}

// EstimateSmartFee returns fee estimation
func (b *EthereumRPC) EstimateSmartFee(blocks int, conservative bool) (big.Int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	var r big.Int
	gp, err := b.Client.SuggestGasPrice(ctx)
	if err == nil && b != nil {
		r = *gp
	}
	return r, err
}

// GetStringFromMap attempts to return the value for a specific key in a map as a string if valid,
// otherwise returns an empty string with false indicating there was no key found, or the value was not a string
func GetStringFromMap(p string, params map[string]interface{}) (string, bool) {
	v, ok := params[p]
	if ok {
		s, ok := v.(string)
		return s, ok
	}
	return "", false
}

// EthereumTypeEstimateGas returns estimation of gas consumption for given transaction parameters
func (b *EthereumRPC) EthereumTypeEstimateGas(params map[string]interface{}) (uint64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	msg := ethereum.CallMsg{}
	if s, ok := GetStringFromMap("from", params); ok && len(s) > 0 {
		msg.From = ethcommon.HexToAddress(s)
	}
	if s, ok := GetStringFromMap("to", params); ok && len(s) > 0 {
		a := ethcommon.HexToAddress(s)
		msg.To = &a
	}
	if s, ok := GetStringFromMap("data", params); ok && len(s) > 0 {
		msg.Data = ethcommon.FromHex(s)
	}
	if s, ok := GetStringFromMap("value", params); ok && len(s) > 0 {
		msg.Value, _ = hexutil.DecodeBig(s)
	}
	if s, ok := GetStringFromMap("gas", params); ok && len(s) > 0 {
		msg.Gas, _ = hexutil.DecodeUint64(s)
	}
	if s, ok := GetStringFromMap("gasPrice", params); ok && len(s) > 0 {
		msg.GasPrice, _ = hexutil.DecodeBig(s)
	}

	if b.alternativeSendTxProvider != nil {
		result, err := b.alternativeSendTxProvider.callHttpStringResult(
			b.alternativeSendTxProvider.urls[0],
			"eth_estimateGas",
			params,
		)
		if err == nil {
			return hexutil.DecodeUint64(result)
		}
	}
	return b.Client.EstimateGas(ctx, msg)
}

// EthereumTypeGetEip1559Fees retrieves Eip1559Fees, if supported
func (b *EthereumRPC) EthereumTypeGetEip1559Fees() (*bchain.Eip1559Fees, error) {
	if !b.ChainConfig.Eip1559Fees {
		return nil, nil
	}
	// if there is an alternative provider, use it
	if b.alternativeFeeProvider != nil {
		fees, err := b.alternativeFeeProvider.GetEip1559Fees()
		if err != nil {
			return nil, err
		}
		if fees != nil {
			return fees, nil
		}
		// Fall back to on-chain estimation when the alternative provider is unsupported/stale/unready,
		// so configured networks still return EIP-1559 fees instead of nil, which resolves to empty fees.
	}

	// otherwise use algorithm from here https://docs.alchemy.com/docs/how-to-build-a-gas-fee-estimator-using-eip-1559
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()

	var maxPriorityFeePerGas hexutil.Big
	err := b.RPC.CallContext(ctx, &maxPriorityFeePerGas, "eth_maxPriorityFeePerGas")
	if err != nil {
		return nil, err
	}

	var fees bchain.Eip1559Fees

	type history struct {
		OldestBlock   string     `json:"oldestBlock"`
		Reward        [][]string `json:"reward"`
		BaseFeePerGas []string   `json:"baseFeePerGas"`
		GasUsedRatio  []float64  `json:"gasUsedRatio"`
	}
	var h history
	percentiles := []int{
		20, // low
		70, // medium
		90, // high
		99, // instant
	}
	blocks := 4

	err = b.RPC.CallContext(ctx, &h, "eth_feeHistory", blocks, "pending", percentiles)
	if err != nil {
		return nil, err
	}
	if len(h.BaseFeePerGas) < blocks {
		return nil, nil
	}

	hs, _ := json.Marshal(h)
	baseFee, _ := hexutil.DecodeUint64(h.BaseFeePerGas[blocks-1])
	fees.BaseFeePerGas = big.NewInt(int64(baseFee))
	maxBasePriorityFee := maxPriorityFeePerGas.ToInt().Int64()
	glog.Info("eth_maxPriorityFeePerGas ", maxPriorityFeePerGas)
	glog.Info("eth_feeHistory ", string(hs))

	for i := 0; i < 4; i++ {
		var f bchain.Eip1559Fee
		priorityFee := int64(0)
		for j := 0; j < len(h.Reward); j++ {
			p, _ := hexutil.DecodeUint64(h.Reward[j][i])
			priorityFee += int64(p)
		}
		priorityFee = priorityFee / int64(len(h.Reward))
		f.MaxFeePerGas = big.NewInt(priorityFee)
		f.MaxPriorityFeePerGas = big.NewInt(maxBasePriorityFee)
		maxBasePriorityFee *= 2
		switch i {
		case 0:
			fees.Low = &f
		case 1:
			fees.Medium = &f
		case 2:
			fees.High = &f
		default:
			fees.Instant = &f
		}
	}
	return &fees, err
}

// SendRawTransaction sends raw transaction
func (b *EthereumRPC) SendRawTransaction(hex string, disableAlternativeRPC bool) (string, error) {
	var txid string
	var retErr error

	if !disableAlternativeRPC && b.alternativeSendTxProvider != nil {
		txid, retErr = b.alternativeSendTxProvider.SendRawTransaction(hex)
		if retErr == nil {
			return txid, nil
		}
		if b.alternativeSendTxProvider.UseOnlyAlternativeProvider() {
			return txid, retErr
		}
	}

	txid, retErr = b.callRpcStringResult("eth_sendRawTransaction", hex)
	if b.ChainConfig.DisableMempoolSync {
		// add transactions submitted by us to mempool if sync is disabled
		b.Mempool.AddTransactionToMempool(txid)
	}
	return txid, retErr
}

// EthereumTypeGetRawTransaction gets raw transaction in hex format
func (b *EthereumRPC) EthereumTypeGetRawTransaction(txid string) (string, error) {
	return b.callRpcStringResult("eth_getRawTransactionByHash", txid)
}

// Helper function for calling ETH RPC with parameters and getting string result
func (b *EthereumRPC) callRpcStringResult(rpcMethod string, args ...interface{}) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	var raw json.RawMessage
	err := b.RPC.CallContext(ctx, &raw, rpcMethod, args...)
	if err != nil {
		return "", err
	} else if len(raw) == 0 {
		return "", errors.New(rpcMethod + " : failed")
	}
	var result string
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", errors.Annotatef(err, "raw result %v", raw)
	}
	if result == "" {
		return "", errors.New(rpcMethod + " : failed, empty result")
	}
	return result, nil
}

// EthereumTypeGetTransactionReceipt returns the transaction receipt by the transaction ID.
func (b *EthereumRPC) EthereumTypeGetTransactionReceipt(txid string) (*bchain.RpcReceipt, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	var r *bchain.RpcReceipt
	err := b.RPC.CallContext(ctx, &r, "eth_getTransactionReceipt", ethcommon.HexToHash(txid))
	return r, err
}

// EthereumTypeGetBalance returns current balance of an address
func (b *EthereumRPC) EthereumTypeGetBalance(addrDesc bchain.AddressDescriptor) (*big.Int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	return b.Client.BalanceAt(ctx, addrDesc, nil)
}

// EthereumTypeGetNonce returns current balance of an address
func (b *EthereumRPC) EthereumTypeGetNonce(addrDesc bchain.AddressDescriptor) (uint64, error) {
	var result string
	var err error
	var usedAlternative bool

	ethAddress := ethcommon.BytesToAddress(addrDesc)

	if b.alternativeSendTxProvider != nil {
		result, err = b.alternativeSendTxProvider.callHttpStringResult(
			b.alternativeSendTxProvider.urls[0],
			"eth_getTransactionCount",
			ethAddress,
			"pending",
		)
		if err == nil && result != "" {
			usedAlternative = true
		} else {
			glog.Errorf("Alternative provider failed for eth_getTransactionCount: %v, falling back to primary RPC", err)
		}
	}

	if !usedAlternative {
		result, err = b.callRpcStringResult("eth_getTransactionCount", ethAddress, "pending")
		if err != nil {
			glog.Errorf("Primary RPC failed for eth_getTransactionCount: %v", err)
			return 0, err
		}
	}

	nonce, err := hexutil.DecodeUint64(result)
	if err != nil {
		glog.Errorf("Failed to parse nonce result '%s': %v", result, err)
		return 0, err
	}

	return nonce, nil
}

// GetChainParser returns ethereum BlockChainParser
func (b *EthereumRPC) GetChainParser() bchain.BlockChainParser {
	return b.Parser
}

// ENS helper: namehash per ENS spec
func ensNameHash(name string) string {
	node := make([]byte, 32)
	if name != "" {
		labels := strings.Split(name, ".")
		for i := len(labels) - 1; i >= 0; i-- {
			labelHash := keccak256([]byte(labels[i]))
			node = keccak256(append(node, labelHash...))
		}
	}
	return "0x" + hex.EncodeToString(node)
}

func keccak256(data []byte) []byte {
	hash := sha3.NewLegacyKeccak256()
	hash.Write(data)
	return hash.Sum(nil)
}

func parseENSAddressFromResult(result string) (string, error) {
	if len(result) < 2 || result[:2] != "0x" {
		return "", errors.New("invalid hex result")
	}
	hexData := result[2:]
	if len(hexData) < 64 {
		return "", errors.New("result too short")
	}
	addressHex := hexData[len(hexData)-EthereumAddressHexLength:]
	return "0x" + addressHex, nil
}

func (b *EthereumRPC) ensContracts() (string, string, error) {
	if b.Testnet || b.MainNetChainID != MainNet {
		// ENS contracts are mainnet-only here; avoid calling empty/uninitialized addresses on other networks.
		return "", "", errors.New("ENS contracts not configured for this network")
	}
	return ENSRegistryAddress, ENSBaseRegistrarAddress, nil
}

// ResolveENS resolves ENS domain name to Ethereum address
func (b *EthereumRPC) ResolveENS(name string) (*bchain.ENSResolution, error) {
	glog.Infof("ResolveENS: Starting resolution for %s", name)

	name = strings.ToLower(strings.TrimSpace(name))
	if !strings.HasSuffix(name, ".eth") {
		glog.Errorf("ResolveENS: Invalid ENS name %s", name)
		return &bchain.ENSResolution{Name: name, Error: "invalid ENS name"}, errors.New("invalid ENS name")
	}

	// Calculate the namehash for this domain
	node := ensNameHash(name)
	glog.Infof("ResolveENS: Generated node hash %s for %s", node, name)

	registry, _, err := b.ensContracts()
	if err != nil {
		// This avoids empty eth_call targets on L2s while keeping mainnet behavior unchanged
		return &bchain.ENSResolution{Name: name, Error: "ENS not supported on this network"}, err
	}

	// Call resolver(bytes32) on the ENS registry
	callData := map[string]string{
		"to":   registry,
		"data": ENSResolverFunctionSelector + node[2:],
	}
	// Call the resolver function on the ENS registry
	result, err := b.callRpcStringResult("eth_call", callData, "latest")
	if err != nil {
		glog.Errorf("ResolveENS: Registry call failed: %v", err)
		return &bchain.ENSResolution{Name: name, Error: "failed to query ENS registry"}, err
	}
	glog.Infof("ResolveENS: Registry result: %s", result)

	// Parse the resolver address from the result
	//The result is ABI-encoded, we need to extract the address from the last 40 hex characters
	resolverAddr, err := parseENSAddressFromResult(result)
	if err != nil {
		glog.Errorf("ResolveENS: Failed to parse resolver address: %v", err)
		return &bchain.ENSResolution{Name: name, Error: "failed to parse resolver"}, err
	}
	glog.Infof("ResolveENS: Resolver address: %s", resolverAddr)

	if resolverAddr == EthereumZeroAddress {
		glog.Errorf("ResolveENS: No resolver set for %s", name)
		return &bchain.ENSResolution{Name: name, Error: "no resolver set"}, errors.New("no resolver set")
	}

	// Call the addr(bytes32) function on the resolver
	callData = map[string]string{
		"to":   resolverAddr,
		"data": ENSAddrFunctionSelector + node[2:],
	}

	result, err = b.callRpcStringResult("eth_call", callData, "latest")
	if err != nil {
		glog.Errorf("ResolveENS: Resolver call failed: %v", err)
		return &bchain.ENSResolution{Name: name, Error: "failed to query resolver"}, err
	}
	glog.Infof("ResolveENS: Resolver result: %s", result)

	address, err := parseENSAddressFromResult(result)
	if err != nil {
		glog.Errorf("ResolveENS: Failed to parse address: %v", err)
		return &bchain.ENSResolution{Name: name, Error: "failed to parse address"}, err
	}

	if address == EthereumZeroAddress {
		glog.Errorf("ResolveENS: ENS name %s not found", name)
		return &bchain.ENSResolution{Name: name, Error: "ENS name not found"}, errors.New("ENS name not found")
	}

	glog.Infof("ResolveENS: Successfully resolved %s to %s", name, address)
	return &bchain.ENSResolution{Name: name, Address: address}, nil
}

// CheckENSExpiration checks if an ENS domain is expired
func (b *EthereumRPC) CheckENSExpiration(name string) (bool, error) {
	name = strings.ToLower(strings.TrimSpace(name))

	// Only check expiration for .eth domains
	if !strings.HasSuffix(name, ".eth") {
		glog.Infof("CheckENSExpiration: %s is not a .eth domain, skipping expiration check", name)
		return false, nil
	}

	// Extract the label (part before .eth)
	label := strings.TrimSuffix(name, ".eth")
	if strings.Contains(label, ".") {
		// Base Registrar tracks only second-level .eth names; for subdomains, check the parent label.
		parts := strings.Split(label, ".")
		label = parts[len(parts)-1]
	}

	_, registrar, err := b.ensContracts()
	if err != nil {
		return false, err
	}

	// Calculate token ID: keccak256(label)
	labelHash := keccak256([]byte(label))
	tokenID := new(big.Int).SetBytes(labelHash)

	glog.Infof("CheckENSExpiration: Checking expiration for %s (label: %s, tokenID: %s)", name, label, tokenID.String())

	// Pad token ID to 32 bytes (64 hex chars) with leading zeros
	tokenIDHex := hex.EncodeToString(tokenID.Bytes())
	tokenIDPadded := strings.Repeat("0", 64-len(tokenIDHex)) + tokenIDHex

	// Call nameExpires(uint256 id) on the Base Registrar
	callData := map[string]string{
		"to":   registrar,
		"data": ENSExpirationFunctionSelector + tokenIDPadded,
	}

	result, err := b.callRpcStringResult("eth_call", callData, "latest")
	if err != nil {
		glog.Errorf("CheckENSExpiration: RPC call failed for %s: %v", name, err)
		return false, err
	}

	// Parse the expiration timestamp from the result
	if len(result) < 2 || result[:2] != "0x" {
		return false, errors.New("invalid hex result")
	}

	expiration, err := hexutil.DecodeBig(result)
	if err != nil {
		glog.Errorf("CheckENSExpiration: Failed to decode expiration for %s: %v", name, err)
		return false, err
	}

	// Check if expired (current timestamp > expiration timestamp)
	currentTime := big.NewInt(time.Now().Unix())
	isExpired := currentTime.Cmp(expiration) > 0

	expirationTime := time.Unix(expiration.Int64(), 0)
	glog.Infof("CheckENSExpiration: %s expires at %s (expired: %v)", name, expirationTime.String(), isExpired)

	return isExpired, nil
}
