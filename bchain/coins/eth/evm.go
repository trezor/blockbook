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

// EthereumClient wraps a client to implement the EVMClient interface
type EthereumClient struct {
	*ethclient.Client
}

// HeaderByNumber returns a block header that implements the EVMHeader interface
func (c *EthereumClient) HeaderByNumber(ctx context.Context, number *big.Int) (bchain.EVMHeader, error) {
	h, err := c.Client.HeaderByNumber(ctx, number)
	if err != nil {
		return nil, err
	}

	return &EthereumHeader{Header: h}, nil
}

// EstimateGas returns the current estimated gas cost for executing a transaction
func (c *EthereumClient) EstimateGas(ctx context.Context, msg interface{}) (uint64, error) {
	return c.Client.EstimateGas(ctx, msg.(ethereum.CallMsg))
}

// BalanceAt returns the balance for the given account at a specific block, or latest known block if no block number is provided
func (c *EthereumClient) BalanceAt(ctx context.Context, addrDesc bchain.AddressDescriptor, blockNumber *big.Int) (*big.Int, error) {
	return c.Client.BalanceAt(ctx, common.BytesToAddress(addrDesc), blockNumber)
}

// NonceAt returns the nonce for the given account at a specific block, or latest known block if no block number is provided
func (c *EthereumClient) NonceAt(ctx context.Context, addrDesc bchain.AddressDescriptor, blockNumber *big.Int) (uint64, error) {
	return c.Client.NonceAt(ctx, common.BytesToAddress(addrDesc), blockNumber)
}

// EthereumRPCClient wraps an rpc client to implement the EVMRPCClient interface
type EthereumRPCClient struct {
	*rpc.Client
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
