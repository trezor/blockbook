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

// EthereumClient wraps the go-ethereum ethclient to conform with the Client interface
type EthereumClient struct {
	*ethclient.Client
}

func (c *EthereumClient) HeaderByNumber(ctx context.Context, number *big.Int) (bchain.EVMHeader, error) {
	h, err := c.Client.HeaderByNumber(ctx, number)
	if err != nil {
		return nil, err
	}

	return &EthereumHeader{Header: h}, nil
}

func (c *EthereumClient) EstimateGas(ctx context.Context, msg interface{}) (uint64, error) {
	return c.Client.EstimateGas(ctx, msg.(ethereum.CallMsg))
}

func (c *EthereumClient) BalanceAt(ctx context.Context, addrDesc bchain.AddressDescriptor, blockNumber *big.Int) (*big.Int, error) {
	return c.Client.BalanceAt(ctx, common.BytesToAddress(addrDesc), nil)
}

func (c *EthereumClient) NonceAt(ctx context.Context, addrDesc bchain.AddressDescriptor, blockNumber *big.Int) (uint64, error) {
	return c.Client.NonceAt(ctx, common.BytesToAddress(addrDesc), nil)
}

// EthereumRPCClient wraps the go-ethereum rpc client to conform with the RPCClient interface
type EthereumRPCClient struct {
	*rpc.Client
}

func (c *EthereumRPCClient) EthSubscribe(ctx context.Context, channel interface{}, args ...interface{}) (bchain.EVMClientSubscription, error) {
	sub, err := c.Client.EthSubscribe(ctx, channel, args)
	if err != nil {
		return nil, err
	}

	return &EthereumClientSubscription{ClientSubscription: sub}, nil
}

func (c *EthereumRPCClient) CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error {
	return c.Client.CallContext(ctx, result, method, args)
}

func (c *EthereumRPCClient) Close() {
	c.Client.Close()
}

// EthereumHeader wraps the ethereum header to conform with the Header interface
type EthereumHeader struct {
	*types.Header
}

func (h *EthereumHeader) Hash() string {
	return h.Header.Hash().Hex()
}

func (h *EthereumHeader) Number() *big.Int {
	return h.Header.Number
}

func (h *EthereumHeader) Difficulty() *big.Int {
	return h.Header.Difficulty
}

// EthereumHash wraps the ethereum hash to conform with the Hash interface
type EthereumHash struct {
	common.Hash
}

// EthereumClientSubscription wraps an ethereum client subcription to conform with the ClientSubscription interface
type EthereumClientSubscription struct {
	*rpc.ClientSubscription
}

func (c *EthereumClientSubscription) Unsubscribe() {
	c.ClientSubscription.Unsubscribe()
}

// EthereumNewBlock wraps an ethereum header channel to conform with the Subscriber interface
type EthereumNewBlock struct {
	channel chan *types.Header
}

func (s *EthereumNewBlock) Channel() interface{} {
	return s.channel
}

func (s *EthereumNewBlock) Read() (bchain.EVMHeader, bool) {
	h, ok := <-s.channel
	return &EthereumHeader{Header: h}, ok
}

func (s *EthereumNewBlock) Close() {
	close(s.channel)
}

// EthereumNewTx wraps an ethereum transaction hash channel to conform with the Subscriber interface
type EthereumNewTx struct {
	channel chan common.Hash
}

func (s *EthereumNewTx) Channel() interface{} {
	return s.channel
}

func (s *EthereumNewTx) Read() (bchain.EVMHash, bool) {
	h, ok := <-s.channel
	return &EthereumHash{Hash: h}, ok
}

func (s *EthereumNewTx) Close() {
	close(s.channel)
}
