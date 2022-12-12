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

// OptimismClient wraps the go-ethereum ethclient to conform with the EVMClient interface
type OptimismClient struct {
	*ethclient.Client
}

func (c *OptimismClient) HeaderByNumber(ctx context.Context, number *big.Int) (bchain.EVMHeader, error) {
	h, err := c.Client.HeaderByNumber(ctx, number)
	if err != nil {
		return nil, err
	}

	return &OptimismHeader{Header: h}, nil
}

func (c *OptimismClient) EstimateGas(ctx context.Context, msg interface{}) (uint64, error) {
	return c.Client.EstimateGas(ctx, msg.(optimism.CallMsg))
}

func (c *OptimismClient) BalanceAt(ctx context.Context, addrDesc bchain.AddressDescriptor, blockNumber *big.Int) (*big.Int, error) {
	return c.Client.BalanceAt(ctx, common.BytesToAddress(addrDesc), nil)
}

func (c *OptimismClient) NonceAt(ctx context.Context, addrDesc bchain.AddressDescriptor, blockNumber *big.Int) (uint64, error) {
	return c.Client.NonceAt(ctx, common.BytesToAddress(addrDesc), nil)
}

// OptimismRPCClient wraps the go-ethereum rpc client to conform with the Client interface
type OptimismRPCClient struct {
	*rpc.Client
}

func (c *OptimismRPCClient) EthSubscribe(ctx context.Context, channel interface{}, args ...interface{}) (bchain.EVMClientSubscription, error) {
	sub, err := c.Client.EthSubscribe(ctx, channel, args...)
	if err != nil {
		return nil, err
	}

	return &OptimismClientSubscription{ClientSubscription: sub}, nil
}

// OptimismHeader wraps the avalanche header to conform with the Header interface
type OptimismHeader struct {
	*types.Header
}

func (h *OptimismHeader) Hash() string {
	return h.Header.Hash().Hex()
}

func (h *OptimismHeader) Number() *big.Int {
	return h.Header.Number
}

func (h *OptimismHeader) Difficulty() *big.Int {
	return h.Header.Difficulty
}

// OptimismHash wraps the avalanche hash to conform with the Hash interface
type OptimismHash struct {
	common.Hash
}

// OptimismClientSubscription wraps an avalanche client subcription to conform with the ClientSubscription interface
type OptimismClientSubscription struct {
	*rpc.ClientSubscription
}

// OptimismNewBlock wraps an avalanche header channel to conform with the Subscriber interface
type OptimismNewBlock struct {
	channel chan *types.Header
}

func (s *OptimismNewBlock) Channel() interface{} {
	return s.channel
}

func (s *OptimismNewBlock) Read() (bchain.EVMHeader, bool) {
	h, ok := <-s.channel
	return &OptimismHeader{Header: h}, ok
}

func (s *OptimismNewBlock) Close() {
	close(s.channel)
}

// OptimismNewTx wraps an ethereum transaction hash channel to conform with the Subscriber interface
type OptimismNewTx struct {
	channel chan common.Hash
}

func (s *OptimismNewTx) Channel() interface{} {
	return s.channel
}

func (s *OptimismNewTx) Read() (bchain.EVMHash, bool) {
	h, ok := <-s.channel
	return &OptimismHash{Hash: h}, ok
}

func (s *OptimismNewTx) Close() {
	close(s.channel)
}
