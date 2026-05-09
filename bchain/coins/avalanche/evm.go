package avalanche

import (
	"context"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
)

// AvalancheClient wraps both an HTTP-backed and a WebSocket-backed pair of
// inner clients (mirroring eth.EthereumClient). Methods dispatch based on
// eth.IsSyncRoute(ctx): tagged calls go to the WS-side clients, untagged
// calls go to the HTTP-side. When the same underlying *rpc.Client serves
// both, the WS fields are aliased to the HTTP fields.
type AvalancheClient struct {
	httpEC  *ethclient.Client
	wsEC    *ethclient.Client
	httpRPC *AvalancheRPCClient
	wsRPC   *AvalancheRPCClient
}

func (c *AvalancheClient) pickEC(ctx context.Context) *ethclient.Client {
	if eth.IsSyncRoute(ctx) && c.wsEC != nil {
		return c.wsEC
	}
	return c.httpEC
}

func (c *AvalancheClient) pickRPC(ctx context.Context) *AvalancheRPCClient {
	if eth.IsSyncRoute(ctx) && c.wsRPC != nil {
		return c.wsRPC
	}
	return c.httpRPC
}

// Close shuts down both inner ethclient.Clients. Aliased clients are closed
// only once. The AvalancheRPCClient pair is owned by AvalancheDualRPCClient,
// which closes them itself.
func (c *AvalancheClient) Close() {
	if c.httpEC != nil {
		c.httpEC.Close()
	}
	if c.wsEC != nil && c.wsEC != c.httpEC {
		c.wsEC.Close()
	}
}

// HeaderByNumber returns a block header that implements the EVMHeader interface
func (c *AvalancheClient) HeaderByNumber(ctx context.Context, number *big.Int) (bchain.EVMHeader, error) {
	var head *Header
	err := c.pickRPC(ctx).CallContext(ctx, &head, "eth_getBlockByNumber", bchain.ToBlockNumArg(number), false)
	if err == nil && head == nil {
		err = ethereum.NotFound
	}
	return &AvalancheHeader{Header: head}, err
}

// EstimateGas returns the current estimated gas cost for executing a transaction
func (c *AvalancheClient) EstimateGas(ctx context.Context, msg interface{}) (uint64, error) {
	return c.pickEC(ctx).EstimateGas(ctx, msg.(ethereum.CallMsg))
}

// BalanceAt returns the balance for the given account at a specific block, or latest known block if no block number is provided
func (c *AvalancheClient) BalanceAt(ctx context.Context, addrDesc bchain.AddressDescriptor, blockNumber *big.Int) (*big.Int, error) {
	return c.pickEC(ctx).BalanceAt(ctx, common.BytesToAddress(addrDesc), blockNumber)
}

// NonceAt returns the nonce for the given account at a specific block, or latest known block if no block number is provided
func (c *AvalancheClient) NonceAt(ctx context.Context, addrDesc bchain.AddressDescriptor, blockNumber *big.Int) (uint64, error) {
	return c.pickEC(ctx).NonceAt(ctx, common.BytesToAddress(addrDesc), blockNumber)
}

// NetworkID returns the chain id reported by the backend.
func (c *AvalancheClient) NetworkID(ctx context.Context) (*big.Int, error) {
	return c.pickEC(ctx).NetworkID(ctx)
}

// SuggestGasPrice asks the backend for a gas-price suggestion.
func (c *AvalancheClient) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	return c.pickEC(ctx).SuggestGasPrice(ctx)
}

// AvalancheRPCClient wraps an rpc client to implement the EVMRPCClient interface
type AvalancheRPCClient struct {
	*rpc.Client
}

// AvalancheDualRPCClient routes calls and subscriptions to separate RPC clients.
// CallContext routes by eth.IsSyncRoute(ctx): tagged calls go over WS to keep
// chain-tip RPC traffic sticky to the announcer node; untagged calls go over
// HTTP so bulk/parallel sync and public API traffic can fan out across the
// LB pool. BatchCallContext stays on HTTP unconditionally. EthSubscribe
// always uses WS.
type AvalancheDualRPCClient struct {
	CallClient *AvalancheRPCClient
	SubClient  *AvalancheRPCClient
}

// CallContext routes the JSON-RPC call to SubClient when ctx carries the
// sync-route tag, otherwise to CallClient. Avalanche-specific error
// translation is performed inside AvalancheRPCClient.CallContext on whichever
// side handles the call.
func (c *AvalancheDualRPCClient) CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error {
	if eth.IsSyncRoute(ctx) && c.SubClient != nil {
		return c.SubClient.CallContext(ctx, result, method, args...)
	}
	return c.CallClient.CallContext(ctx, result, method, args...)
}

// BatchCallContext forwards batch JSON-RPC calls to the HTTP client.
func (c *AvalancheDualRPCClient) BatchCallContext(ctx context.Context, batch []rpc.BatchElem) error {
	return c.CallClient.BatchCallContext(ctx, batch)
}

// EthSubscribe forwards subscriptions to the WebSocket client.
func (c *AvalancheDualRPCClient) EthSubscribe(ctx context.Context, channel interface{}, args ...interface{}) (bchain.EVMClientSubscription, error) {
	return c.SubClient.EthSubscribe(ctx, channel, args...)
}

// Close shuts down both underlying clients.
func (c *AvalancheDualRPCClient) Close() {
	if c.SubClient != nil {
		c.SubClient.Close()
	}
	if c.CallClient != nil && c.CallClient != c.SubClient {
		c.CallClient.Close()
	}
}

// EthSubscribe subscribes to events and returns a client subscription that implements the EVMClientSubscription interface
func (c *AvalancheRPCClient) EthSubscribe(ctx context.Context, channel interface{}, args ...interface{}) (bchain.EVMClientSubscription, error) {
	sub, err := c.Client.EthSubscribe(ctx, channel, args...)
	if err != nil {
		return nil, err
	}

	return &AvalancheClientSubscription{ClientSubscription: sub}, nil
}

// CallContext performs a JSON-RPC call with the given arguments
func (c *AvalancheRPCClient) CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error {
	err := c.Client.CallContext(ctx, result, method, args...)
	// unfinalized data cannot be queried error returned when trying to query a block height greater than last finalized block
	// treat as ErrBlockNotFound so sync retries instead of processing an empty result
	// https://docs.avax.network/quickstart/exchanges/integrate-exchange-with-avalanche#determining-finality
	if err != nil {
		if strings.Contains(err.Error(), "cannot query unfinalized data") {
			return bchain.ErrBlockNotFound
		}
		return err
	}
	return nil
}

// AvalancheHeader wraps a block header to implement the EVMHeader interface
type AvalancheHeader struct {
	*Header
}

// Hash returns the block hash as a hex string
func (h *AvalancheHeader) Hash() string {
	return h.Header.Hash().Hex()
}

// Number returns the block number
func (h *AvalancheHeader) Number() *big.Int {
	return h.Header.Number
}

// Difficulty returns the block difficulty
func (h *AvalancheHeader) Difficulty() *big.Int {
	return h.Header.Difficulty
}

// AvalancheHash wraps a transaction hash to implement the EVMHash interface
type AvalancheHash struct {
	common.Hash
}

// AvalancheClientSubscription wraps a client subcription to implement the EVMClientSubscription interface
type AvalancheClientSubscription struct {
	*rpc.ClientSubscription
}

// AvalancheNewBlock wraps a block header channel to implement the EVMNewBlockSubscriber interface
type AvalancheNewBlock struct {
	channel chan *Header
}

// Channel returns the underlying channel as an empty interface
func (s *AvalancheNewBlock) Channel() interface{} {
	return s.channel
}

// Read from the underlying channel and return a block header that implements the EVMHeader interface
func (s *AvalancheNewBlock) Read() (bchain.EVMHeader, bool) {
	h, ok := <-s.channel
	return &AvalancheHeader{Header: h}, ok
}

// Close the underlying channel
func (s *AvalancheNewBlock) Close() {
	close(s.channel)
}

// AvalancheNewTx wraps a transaction hash channel to conform with the EVMNewTxSubscriber interface
type AvalancheNewTx struct {
	channel chan common.Hash
}

// Channel returns the underlying channel as an empty interface
func (s *AvalancheNewTx) Channel() interface{} {
	return s.channel
}

// Read from the underlying channel and return a transaction hash that implements the EVMHash interface
func (s *AvalancheNewTx) Read() (bchain.EVMHash, bool) {
	h, ok := <-s.channel
	return &AvalancheHash{Hash: h}, ok
}

// Close the underlying channel
func (s *AvalancheNewTx) Close() {
	close(s.channel)
}
