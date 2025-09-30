package gnosis

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/trezor/blockbook/bchain"
)

// GnosisClient wraps a client to implement the EVMClient interface
type GnosisClient struct {
	*ethclient.Client
	*GnosisRPCClient
}

// HeaderByNumber returns a block header that implements the EVMHeader interface
func (c *GnosisClient) HeaderByNumber(ctx context.Context, number *big.Int) (bchain.EVMHeader, error) {
	var head *Header
	err := c.GnosisRPCClient.CallContext(ctx, &head, "eth_getBlockByNumber", bchain.ToBlockNumArg(number), false)
	if err == nil && head == nil {
		err = ethereum.NotFound
	}
	return &GnosisHeader{Header: head}, err
}

// EstimateGas returns the current estimated gas cost for executing a transaction
func (c *GnosisClient) EstimateGas(ctx context.Context, msg interface{}) (uint64, error) {
	return c.Client.EstimateGas(ctx, msg.(ethereum.CallMsg))
}

// BalanceAt returns the balance for the given account at a specific block, or latest known block if no block number is provided
func (c *GnosisClient) BalanceAt(ctx context.Context, addrDesc bchain.AddressDescriptor, blockNumber *big.Int) (*big.Int, error) {
	return c.Client.BalanceAt(ctx, common.BytesToAddress(addrDesc), blockNumber)
}

// NonceAt returns the nonce for the given account at a specific block, or latest known block if no block number is provided
func (c *GnosisClient) NonceAt(ctx context.Context, addrDesc bchain.AddressDescriptor, blockNumber *big.Int) (uint64, error) {
	return c.Client.NonceAt(ctx, common.BytesToAddress(addrDesc), blockNumber)
}

// GnosisRPCClient wraps an rpc client to implement the EVMRPCClient interface
type GnosisRPCClient struct {
	*rpc.Client
}

// EthSubscribe subscribes to events and returns a client subscription that implements the EVMClientSubscription interface
func (c *GnosisRPCClient) EthSubscribe(ctx context.Context, channel interface{}, args ...interface{}) (bchain.EVMClientSubscription, error) {
	sub, err := c.Client.EthSubscribe(ctx, channel, args...)
	if err != nil {
		return nil, err
	}

	return &GnosisClientSubscription{ClientSubscription: sub}, nil
}

// GnosisHeader wraps a block header to implement the EVMHeader interface
type GnosisHeader struct {
	*Header
}

// Hash returns the block hash as a hex string
func (h *GnosisHeader) Hash() string {
	return h.Header.Hash().Hex()
}

// Number returns the block number
func (h *GnosisHeader) Number() *big.Int {
	return h.Header.Number
}

// Difficulty returns the block difficulty
func (h *GnosisHeader) Difficulty() *big.Int {
	return h.Header.Difficulty
}

// GnosisHash wraps a transaction hash to implement the EVMHash interface
type GnosisHash struct {
	common.Hash
}

// GnosisClientSubscription wraps a client subcription to implement the EVMClientSubscription interface
type GnosisClientSubscription struct {
	*rpc.ClientSubscription
}

// GnosisNewBlock wraps a block header channel to implement the EVMNewBlockSubscriber interface
type GnosisNewBlock struct {
	channel chan *Header
}

// NewGnosisNewBlock returns an initialized GnosisNewBlock struct
func NewGnosisNewBlock() *GnosisNewBlock {
	return &GnosisNewBlock{channel: make(chan *Header)}
}

// Channel returns the underlying channel as an empty interface
func (s *GnosisNewBlock) Channel() interface{} {
	return s.channel
}

// Read from the underlying channel and return a block header that implements the EVMHeader interface
func (s *GnosisNewBlock) Read() (bchain.EVMHeader, bool) {
	h, ok := <-s.channel
	return &GnosisHeader{Header: h}, ok
}

// Close the underlying channel
func (s *GnosisNewBlock) Close() {
	close(s.channel)
}

// GnosisNewTx wraps a transaction hash channel to implement the EVMNewTxSubscriber interface
type GnosisNewTx struct {
	channel chan common.Hash
}

// NewGnosisNewTx returns an initialized GnosisNewTx struct
func NewGnosisNewTx() *GnosisNewTx {
	return &GnosisNewTx{channel: make(chan common.Hash)}
}

// Channel returns the underlying channel as an empty interface
func (s *GnosisNewTx) Channel() interface{} {
	return s.channel
}

// Read from the underlying channel and return a transaction hash that implements the EVMHash interface
func (s *GnosisNewTx) Read() (bchain.EVMHash, bool) {
	h, ok := <-s.channel
	return &GnosisHash{Hash: h}, ok
}

// Close the underlying channel
func (s *GnosisNewTx) Close() {
	close(s.channel)
}
