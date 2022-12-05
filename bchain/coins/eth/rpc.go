package eth

import (
	"context"

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

type EthereumClientSubscription struct {
	*rpc.ClientSubscription
}

func (c *EthereumClientSubscription) Unsubscribe() {
	c.ClientSubscription.Unsubscribe()
}
