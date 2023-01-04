package optimism

import (
	"context"
	"math/big"

	optimism "github.com/ethereum-optimism/optimism/l2geth"
	"github.com/ethereum-optimism/optimism/l2geth/common"
	"github.com/ethereum-optimism/optimism/l2geth/core/types"
	"github.com/ethereum-optimism/optimism/l2geth/ethclient"
	"github.com/ethereum-optimism/optimism/l2geth/rpc"
	"github.com/trezor/blockbook/bchain"
)

// OptimismClient wraps a client to implement the EVMClient interface
type OptimismClient struct {
	*ethclient.Client
}

// HeaderByNumber returns a block header that implements the EVMHeader interface
func (c *OptimismClient) HeaderByNumber(ctx context.Context, number *big.Int) (bchain.EVMHeader, error) {
	h, err := c.Client.HeaderByNumber(ctx, number)
	if err != nil {
		return nil, err
	}

	return &OptimismHeader{Header: h}, nil
}

// EstimateGas returns the current estimated gas cost for executing a transaction
func (c *OptimismClient) EstimateGas(ctx context.Context, msg interface{}) (uint64, error) {
	return c.Client.EstimateGas(ctx, msg.(optimism.CallMsg))
}

// BalanceAt returns the balance for the given account at a specific block, or latest known block if no block number is provided
func (c *OptimismClient) BalanceAt(ctx context.Context, addrDesc bchain.AddressDescriptor, blockNumber *big.Int) (*big.Int, error) {
	return c.Client.BalanceAt(ctx, common.BytesToAddress(addrDesc), blockNumber)
}

// NonceAt returns the nonce for the given account at a specific block, or latest known block if no block number is provided
func (c *OptimismClient) NonceAt(ctx context.Context, addrDesc bchain.AddressDescriptor, blockNumber *big.Int) (uint64, error) {
	return c.Client.NonceAt(ctx, common.BytesToAddress(addrDesc), blockNumber)
}

// OptimismRPCClient wraps an rpc client to implement the EVMRPCClient interface
type OptimismRPCClient struct {
	*rpc.Client
}

// EthSubscribe subscribes to events and returns a client subscription that implements the EVMClientSubscription interface
func (c *OptimismRPCClient) EthSubscribe(ctx context.Context, channel interface{}, args ...interface{}) (bchain.EVMClientSubscription, error) {
	sub, err := c.Client.EthSubscribe(ctx, channel, args...)
	if err != nil {
		return nil, err
	}

	return &OptimismClientSubscription{ClientSubscription: sub}, nil
}

// OptimismHeader wraps a block header to implement the EVMHeader interface
type OptimismHeader struct {
	*types.Header
}

// Hash returns the block hash as a hex string
func (h *OptimismHeader) Hash() string {
	return h.Header.Hash().Hex()
}

// Number returns the block number
func (h *OptimismHeader) Number() *big.Int {
	return h.Header.Number
}

// Difficulty returns the block difficulty
func (h *OptimismHeader) Difficulty() *big.Int {
	return h.Header.Difficulty
}

// OptimismHash wraps a transaction hash to implement the EVMHash interface
type OptimismHash struct {
	common.Hash
}

// OptimismClientSubscription wraps a client subcription to implement the EVMClientSubscription interface
type OptimismClientSubscription struct {
	*rpc.ClientSubscription
}

// OptimismNewBlock wraps a block header channel to implement the EVMNewBlockSubscriber interface
type OptimismNewBlock struct {
	channel chan *types.Header
}

// Channel returns the underlying channel as an empty interface
func (s *OptimismNewBlock) Channel() interface{} {
	return s.channel
}

// Read from the underlying channel and return a block header that implements the EVMHeader interface
func (s *OptimismNewBlock) Read() (bchain.EVMHeader, bool) {
	h, ok := <-s.channel
	return &OptimismHeader{Header: h}, ok
}

// Close the underlying channel
func (s *OptimismNewBlock) Close() {
	close(s.channel)
}

// OptimismNewTx wraps a transaction hash channel to conform with the EVMNewTxSubscriber interface
type OptimismNewTx struct {
	channel chan common.Hash
}

// Channel returns the underlying channel as an empty interface
func (s *OptimismNewTx) Channel() interface{} {
	return s.channel
}

// Read from the underlying channel and return a transaction hash that implements the EVMHash interface
func (s *OptimismNewTx) Read() (bchain.EVMHash, bool) {
	h, ok := <-s.channel
	return &OptimismHash{Hash: h}, ok
}

// Close the underlying channel
func (s *OptimismNewTx) Close() {
	close(s.channel)
}
