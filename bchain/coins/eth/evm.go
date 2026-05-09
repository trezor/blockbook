package eth

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/trezor/blockbook/bchain"
)

// stickyKey is the unexported context-value key used to mark calls that must
// follow the WebSocket connection (see WithSyncRoute / isSyncRoute). Marked
// calls hit the WS-pinned backend; unmarked calls go via the HTTP pool.
type stickyKey struct{}

// WithSyncRoute returns a child context tagged so DualRPCClient.CallContext and
// EthereumClient methods route the call over the WebSocket connection. Used by
// the sync facade (ethSyncView) and by refreshBestHeaderFromChain to keep the
// chain-tip block fetch sticky to the node that delivered newHeads.
func WithSyncRoute(ctx context.Context) context.Context {
	return context.WithValue(ctx, stickyKey{}, true)
}

// isSyncRoute reports whether ctx carries the sync-route tag set by WithSyncRoute.
func isSyncRoute(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(stickyKey{}).(bool)
	return v
}

// IsSyncRoute is the exported form of isSyncRoute, intended for forked EVM
// coin packages (e.g. avalanche) that have their own DualRPCClient / Client
// equivalents and need to dispatch on the same tag the eth package sets via
// WithSyncRoute. Keeping the context key (stickyKey) unexported and routing
// reads through this helper guarantees interop without exposing the key
// itself.
func IsSyncRoute(ctx context.Context) bool {
	return isSyncRoute(ctx)
}

// EthereumClient wraps both an HTTP-backed and a WebSocket-backed *ethclient.Client.
// Each method dispatches based on isSyncRoute(ctx): tagged calls go to wsClient,
// untagged calls go to httpClient. When the same underlying *rpc.Client serves
// both, wsClient is aliased to httpClient.
type EthereumClient struct {
	httpClient *ethclient.Client
	wsClient   *ethclient.Client
}

func (c *EthereumClient) pick(ctx context.Context) *ethclient.Client {
	if isSyncRoute(ctx) && c.wsClient != nil {
		return c.wsClient
	}
	return c.httpClient
}

// HeaderByNumber returns a block header that implements the EVMHeader interface
func (c *EthereumClient) HeaderByNumber(ctx context.Context, number *big.Int) (bchain.EVMHeader, error) {
	h, err := c.pick(ctx).HeaderByNumber(ctx, number)
	if err != nil {
		return nil, err
	}

	return &EthereumHeader{Header: h}, nil
}

// EstimateGas returns the current estimated gas cost for executing a transaction
func (c *EthereumClient) EstimateGas(ctx context.Context, msg interface{}) (uint64, error) {
	return c.pick(ctx).EstimateGas(ctx, msg.(ethereum.CallMsg))
}

// BalanceAt returns the balance for the given account at a specific block, or latest known block if no block number is provided
func (c *EthereumClient) BalanceAt(ctx context.Context, addrDesc bchain.AddressDescriptor, blockNumber *big.Int) (*big.Int, error) {
	return c.pick(ctx).BalanceAt(ctx, common.BytesToAddress(addrDesc), blockNumber)
}

// NonceAt returns the nonce for the given account at a specific block, or latest known block if no block number is provided
func (c *EthereumClient) NonceAt(ctx context.Context, addrDesc bchain.AddressDescriptor, blockNumber *big.Int) (uint64, error) {
	return c.pick(ctx).NonceAt(ctx, common.BytesToAddress(addrDesc), blockNumber)
}

// NetworkID returns the chain id reported by the backend.
func (c *EthereumClient) NetworkID(ctx context.Context) (*big.Int, error) {
	return c.pick(ctx).NetworkID(ctx)
}

// SuggestGasPrice asks the backend for a gas-price suggestion.
func (c *EthereumClient) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	return c.pick(ctx).SuggestGasPrice(ctx)
}

// EthereumRPCClient wraps an rpc client to implement the EVMRPCClient interface
type EthereumRPCClient struct {
	*rpc.Client
}

// DualRPCClient holds an HTTP-backed CallClient and a WebSocket-backed SubClient.
// CallContext routes by isSyncRoute(ctx): tagged calls go over WS to keep
// chain-tip RPC traffic sticky to the announcer node; untagged calls go over
// HTTP so bulk/parallel sync and public API traffic can fan out across the LB
// pool. BatchCallContext stays on HTTP unconditionally — batching is HTTP-shaped
// in this codebase. EthSubscribe always uses WS.
type DualRPCClient struct {
	CallClient *rpc.Client
	SubClient  *rpc.Client
}

// CallContext routes the JSON-RPC call to SubClient when ctx carries the
// sync-route tag, otherwise to CallClient.
func (c *DualRPCClient) CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error {
	if isSyncRoute(ctx) && c.SubClient != nil {
		return c.SubClient.CallContext(ctx, result, method, args...)
	}
	return c.CallClient.CallContext(ctx, result, method, args...)
}

// BatchCallContext forwards batch JSON-RPC calls to the HTTP client. WS is not
// used for batching: only the eth package's contract.go ERC-20 fan-out batches,
// and that path is HTTP-only by convention.
func (c *DualRPCClient) BatchCallContext(ctx context.Context, batch []rpc.BatchElem) error {
	return c.CallClient.BatchCallContext(ctx, batch)
}

// EthSubscribe forwards subscriptions to the WebSocket client.
func (c *DualRPCClient) EthSubscribe(ctx context.Context, channel interface{}, args ...interface{}) (bchain.EVMClientSubscription, error) {
	sub, err := c.SubClient.EthSubscribe(ctx, channel, args...)
	if err != nil {
		return nil, err
	}
	return &EthereumClientSubscription{ClientSubscription: sub}, nil
}

// Close shuts down both underlying clients.
func (c *DualRPCClient) Close() {
	if c.SubClient != nil {
		c.SubClient.Close()
	}
	if c.CallClient != nil && c.CallClient != c.SubClient {
		c.CallClient.Close()
	}
}

// EthSubscribe subscribes to events and returns a client subscription that implements the EVMClientSubscription interface
func (c *EthereumRPCClient) EthSubscribe(ctx context.Context, channel interface{}, args ...interface{}) (bchain.EVMClientSubscription, error) {
	sub, err := c.Client.EthSubscribe(ctx, channel, args...)
	if err != nil {
		return nil, err
	}

	return &EthereumClientSubscription{ClientSubscription: sub}, nil
}

// EthereumHeader wraps a block header to implement the EVMHeader interface
type EthereumHeader struct {
	*types.Header
}

// Hash returns the block hash as a hex string
func (h *EthereumHeader) Hash() string {
	return h.Header.Hash().Hex()
}

// Number returns the block number
func (h *EthereumHeader) Number() *big.Int {
	return h.Header.Number
}

// Difficulty returns the block difficulty
func (h *EthereumHeader) Difficulty() *big.Int {
	return h.Header.Difficulty
}

// EthereumHash wraps a transaction hash to implement the EVMHash interface
type EthereumHash struct {
	common.Hash
}

// EthereumClientSubscription wraps a client subcription to implement the EVMClientSubscription interface
type EthereumClientSubscription struct {
	*rpc.ClientSubscription
}

// EthereumNewBlock wraps a block header channel to implement the EVMNewBlockSubscriber interface
type EthereumNewBlock struct {
	channel chan *types.Header
}

// NewEthereumNewBlock returns an initialized EthereumNewBlock struct
func NewEthereumNewBlock() *EthereumNewBlock {
	return &EthereumNewBlock{channel: make(chan *types.Header)}
}

// Channel returns the underlying channel as an empty interface
func (s *EthereumNewBlock) Channel() interface{} {
	return s.channel
}

// Read from the underlying channel and return a block header that implements the EVMHeader interface
func (s *EthereumNewBlock) Read() (bchain.EVMHeader, bool) {
	h, ok := <-s.channel
	return &EthereumHeader{Header: h}, ok
}

// Close the underlying channel
func (s *EthereumNewBlock) Close() {
	close(s.channel)
}

// EthereumNewTx wraps a transaction hash channel to implement the EVMNewTxSubscriber interface
type EthereumNewTx struct {
	channel chan common.Hash
}

// NewEthereumNewTx returns an initialized EthereumNewTx struct
func NewEthereumNewTx() *EthereumNewTx {
	return &EthereumNewTx{channel: make(chan common.Hash)}
}

// Channel returns the underlying channel as an empty interface
func (s *EthereumNewTx) Channel() interface{} {
	return s.channel
}

// Read from the underlying channel and return a transaction hash that implements the EVMHash interface
func (s *EthereumNewTx) Read() (bchain.EVMHash, bool) {
	h, ok := <-s.channel
	return &EthereumHash{Hash: h}, ok
}

// Close the underlying channel
func (s *EthereumNewTx) Close() {
	close(s.channel)
}
