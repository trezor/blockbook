package optimism

import (
	"context"

	"github.com/ethereum/go-ethereum/rpc"
	"github.com/trezor/blockbook/bchain"
)

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

// CallContext performs a JSON-RPC call with the given arguments
func (c *OptimismRPCClient) CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error {
	if err := c.Client.CallContext(ctx, result, method, args...); err != nil {
		return err
	}

	// special case to handle empty gas price for a valid rpc transaction
	// (https://goerli-optimism.etherscan.io/tx/0x9b62094073147508471e3371920b68070979beea32100acdc49c721350b69cb9)
	if r, ok := result.(*bchain.RpcTransaction); ok {
		if *r != (bchain.RpcTransaction{}) && r.GasPrice == "" {
			r.GasPrice = "0x0"
		}
	}

	return nil
}

// OptimismClientSubscription wraps a client subcription to implement the EVMClientSubscription interface
type OptimismClientSubscription struct {
	*rpc.ClientSubscription
}
