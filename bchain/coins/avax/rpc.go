package avax

import (
	"context"

	"github.com/ava-labs/coreth/rpc"
	"github.com/trezor/blockbook/bchain/coins/eth"
)

// AvalancheRPCClient wraps the go-ethereum rpc client to conform with the EVMRPCClient interface
type AvalancheRPCClient struct {
	*rpc.Client
}

func (c *AvalancheRPCClient) EthSubscribe(ctx context.Context, channel interface{}, args ...interface{}) (eth.ClientSubscription, error) {
	sub, err := c.Client.EthSubscribe(ctx, channel, args...)
	if err != nil {
		return nil, err
	}

	return &AvalancheClientSubscription{ClientSubscription: sub}, nil
}

func (c *AvalancheRPCClient) CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error {
	return c.Client.CallContext(ctx, result, method, args...)
}

func (c *AvalancheRPCClient) Close() {
	c.Client.Close()
}

type AvalancheClientSubscription struct {
	*rpc.ClientSubscription
}

func (c *AvalancheClientSubscription) Unsubscribe() {
	c.ClientSubscription.Unsubscribe()
}
