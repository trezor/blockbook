package eth

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
)

// RPCClient interface for evm chains
type RPCClient interface {
	EthSubscribe(ctx context.Context, channel interface{}, args ...interface{}) (ClientSubscription, error)
	CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error
	Close()
}

// EthereumRPCClient wraps the go-ethereum rpc client to conform with the RPCClient interface
type EthereumRPCClient struct {
	*rpc.Client
}

func (c *EthereumRPCClient) EthSubscribe(ctx context.Context, channel interface{}, args ...interface{}) (ClientSubscription, error) {
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

// ClientSubscription interface for evm chains
type ClientSubscription interface {
	Err() <-chan error
	Unsubscribe()
}

// EthereumClientSubscription wraps an ethereum client subcription to conform with the ClientSubscription interface
type EthereumClientSubscription struct {
	*rpc.ClientSubscription
}

func (c *EthereumClientSubscription) Unsubscribe() {
	c.ClientSubscription.Unsubscribe()
}

// Subscriber interface for evm chains
type Subscriber interface {
	Channel() interface{}
	Close()
}

// NewBlockSubscriber interface for evm chains
type NewBlockSubscriber interface {
	Subscriber
	Read() (Header, bool)
}

// EthereumNewBlock wraps an ethereum header channel to conform with the Subscriber interface
type EthereumNewBlock struct {
	channel chan *types.Header
}

func (s *EthereumNewBlock) Channel() interface{} {
	return s.channel
}

func (s *EthereumNewBlock) Read() (Header, bool) {
	h, ok := <-s.channel
	return &EthereumHeader{Header: h}, ok
}

func (s *EthereumNewBlock) Close() {
	close(s.channel)
}

// NewTxSubscriber interface for evm chains
type NewTxSubscriber interface {
	Subscriber
	Read() (Hash, bool)
}

// EthereumNewTx wraps an ethereum transaction hash channel to conform with the Subscriber interface
type EthereumNewTx struct {
	channel chan common.Hash
}

func (s *EthereumNewTx) Channel() interface{} {
	return s.channel
}

func (s *EthereumNewTx) Read() (Hash, bool) {
	h, ok := <-s.channel
	return &EthereumHash{Hash: h}, ok
}

func (s *EthereumNewTx) Close() {
	close(s.channel)
}
