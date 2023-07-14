package xcb

import (
	"context"
	"math/big"

	"github.com/core-coin/go-core/v2"
	"github.com/core-coin/go-core/v2/common"
	"github.com/core-coin/go-core/v2/core/types"
	"github.com/core-coin/go-core/v2/rpc"
	"github.com/core-coin/go-core/v2/xcbclient"
	"github.com/trezor/blockbook/bchain"
)

// CoreblockchainClient wraps a client to implement the CVMClient interface
type CoreblockchainClient struct {
	*xcbclient.Client
}

// HeaderByNumber returns a block header that implements the CVMHeader interface
func (c *CoreblockchainClient) HeaderByNumber(ctx context.Context, number *big.Int) (CVMHeader, error) {
	h, err := c.Client.HeaderByNumber(ctx, number)
	if err != nil {
		return nil, err
	}

	return &CoreCoinHeader{Header: h}, nil
}

// EstimateEnergy returns the current estimated energy cost for executing a transaction
func (c *CoreblockchainClient) EstimateEnergy(ctx context.Context, msg interface{}) (uint64, error) {
	return c.Client.EstimateEnergy(ctx, msg.(core.CallMsg))
}

// BalanceAt returns the balance for the given account at a specific block, or latest known block if no block number is provided
func (c *CoreblockchainClient) BalanceAt(ctx context.Context, addrDesc bchain.AddressDescriptor, blockNumber *big.Int) (*big.Int, error) {
	return c.Client.BalanceAt(ctx, common.BytesToAddress(addrDesc), blockNumber)
}

// NonceAt returns the nonce for the given account at a specific block, or latest known block if no block number is provided
func (c *CoreblockchainClient) NonceAt(ctx context.Context, addrDesc bchain.AddressDescriptor, blockNumber *big.Int) (uint64, error) {
	return c.Client.NonceAt(ctx, common.BytesToAddress(addrDesc), blockNumber)
}

// CoreCoinRPCClient wraps an rpc client to implement the CVMRPCClient interface
type CoreCoinRPCClient struct {
	*rpc.Client
}

// XcbSubscribe subscribes to events and returns a client subscription that implements the EVMClientSubscription interface
func (c *CoreCoinRPCClient) XcbSubscribe(ctx context.Context, channel interface{}, args ...interface{}) (CVMClientSubscription, error) {
	sub, err := c.Client.XcbSubscribe(ctx, channel, args...)
	if err != nil {
		return nil, err
	}

	return &CoreCoinClientSubscription{ClientSubscription: sub}, nil
}

// CoreCoinHeader wraps a block header to implement the CVMHeader interface
type CoreCoinHeader struct {
	*types.Header
}

// Hash returns the block hash as a hex string
func (h *CoreCoinHeader) Hash() string {
	return h.Header.Hash().Hex()
}

// Number returns the block number
func (h *CoreCoinHeader) Number() *big.Int {
	return h.Header.Number
}

// Difficulty returns the block difficulty
func (h *CoreCoinHeader) Difficulty() *big.Int {
	return h.Header.Difficulty
}

// CoreCoinHash wraps a transaction hash to implement the CVMHash interface
type CoreCoinHash struct {
	common.Hash
}

// CoreCoinClientSubscription wraps a client subcription to implement the CVMClientSubscription interface
type CoreCoinClientSubscription struct {
	*rpc.ClientSubscription
}

// CoreCoinNewBlock wraps a block header channel to implement the CVMNewBlockSubscriber interface
type CoreCoinNewBlock struct {
	channel chan *types.Header
}

// NewCoreCoinNewBlock returns an initialized CoreCoinNewBlock struct
func NewCoreCoinNewBlock() *CoreCoinNewBlock {
	return &CoreCoinNewBlock{channel: make(chan *types.Header)}
}

// Channel returns the underlying channel as an empty interface
func (s *CoreCoinNewBlock) Channel() interface{} {
	return s.channel
}

// Read from the underlying channel and return a block header that implements the CVMHeader interface
func (s *CoreCoinNewBlock) Read() (CVMHeader, bool) {
	h, ok := <-s.channel
	return &CoreCoinHeader{Header: h}, ok
}

// Close the underlying channel
func (s *CoreCoinNewBlock) Close() {
	close(s.channel)
}

// CoreCoinNewTx wraps a transaction hash channel to implement the CVMNewTxSubscriber interface
type CoreCoinNewTx struct {
	channel chan common.Hash
}

// NewCoreCoinNewTx returns an initialized CoreCoinNewTx struct
func NewCoreCoinNewTx() *CoreCoinNewTx {
	return &CoreCoinNewTx{channel: make(chan common.Hash)}
}

// Channel returns the underlying channel as an empty interface
func (s *CoreCoinNewTx) Channel() interface{} {
	return s.channel
}

// Read from the underlying channel and return a transaction hash that implements the CVMHash interface
func (s *CoreCoinNewTx) Read() (CVMHash, bool) {
	h, ok := <-s.channel
	return &CoreCoinHash{Hash: h}, ok
}

// Close the underlying channel
func (s *CoreCoinNewTx) Close() {
	close(s.channel)
}
