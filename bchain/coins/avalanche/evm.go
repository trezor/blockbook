package avalanche

import (
	"context"
	"math/big"
	"strings"

	"github.com/ava-labs/coreth/core/types"
	"github.com/ava-labs/coreth/ethclient"
	"github.com/ava-labs/coreth/interfaces"
	"github.com/ava-labs/coreth/rpc"
	"github.com/ethereum/go-ethereum/common"
	"github.com/trezor/blockbook/bchain"
)

// AvalancheClient wraps a client to implement the EVMClient interface
type AvalancheClient struct {
	ethclient.Client
}

// HeaderByNumber returns a block header that implements the EVMHeader interface
func (c *AvalancheClient) HeaderByNumber(ctx context.Context, number *big.Int) (bchain.EVMHeader, error) {
	h, err := c.Client.HeaderByNumber(ctx, number)
	if err != nil {
		return nil, err
	}

	return &AvalancheHeader{Header: h}, nil
}

// EstimateGas returns the current estimated gas cost for executing a transaction
func (c *AvalancheClient) EstimateGas(ctx context.Context, msg interface{}) (uint64, error) {
	return c.Client.EstimateGas(ctx, msg.(interfaces.CallMsg))
}

// BalanceAt returns the balance for the given account at a specific block, or latest known block if no block number is provided
func (c *AvalancheClient) BalanceAt(ctx context.Context, addrDesc bchain.AddressDescriptor, blockNumber *big.Int) (*big.Int, error) {
	return c.Client.BalanceAt(ctx, common.BytesToAddress(addrDesc), blockNumber)
}

// NonceAt returns the nonce for the given account at a specific block, or latest known block if no block number is provided
func (c *AvalancheClient) NonceAt(ctx context.Context, addrDesc bchain.AddressDescriptor, blockNumber *big.Int) (uint64, error) {
	return c.Client.NonceAt(ctx, common.BytesToAddress(addrDesc), blockNumber)
}

// AvalancheRPCClient wraps an rpc client to implement the EVMRPCClient interface
type AvalancheRPCClient struct {
	*rpc.Client
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
	// do not throw rpc error and instead treat as ErrBlockNotFound
	// https://docs.avax.network/quickstart/exchanges/integrate-exchange-with-avalanche#determining-finality
	if err != nil && !strings.Contains(err.Error(), "cannot query unfinalized data") {
		return err
	}
	return nil
}

// AvalancheHeader wraps a block header to implement the EVMHeader interface
type AvalancheHeader struct {
	*types.Header
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
	channel chan *types.Header
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
