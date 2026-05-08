package bchain

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
)

// EVMClient provides the necessary client functionality for evm chain sync
type EVMClient interface {
	NetworkID(ctx context.Context) (*big.Int, error)
	HeaderByNumber(ctx context.Context, number *big.Int) (EVMHeader, error)
	SuggestGasPrice(ctx context.Context) (*big.Int, error)
	EstimateGas(ctx context.Context, msg interface{}) (uint64, error)
	BalanceAt(ctx context.Context, addrDesc AddressDescriptor, blockNumber *big.Int) (*big.Int, error)
	NonceAt(ctx context.Context, addrDesc AddressDescriptor, blockNumber *big.Int) (uint64, error)
}

// EVMRPCClient provides the necessary rpc functionality for evm chain sync
type EVMRPCClient interface {
	EthSubscribe(ctx context.Context, channel interface{}, args ...interface{}) (EVMClientSubscription, error)
	CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error
	CallContextWithIntent(ctx context.Context, intent EVMRPCIntent, result interface{}, method string, args ...interface{}) error
	Close()
}

// EVMRPCIntent describes why a JSON-RPC call is being made, so routing can be
// explicit without changing the public BlockChain methods.
type EVMRPCIntent int

const (
	EVMRPCIntentDefault EVMRPCIntent = iota
	EVMRPCIntentChainTip
)

// EVMBatchRPCClient provides batch JSON-RPC calls in addition to the base RPC client.
type EVMBatchRPCClient interface {
	EVMRPCClient
	BatchCallContext(ctx context.Context, batch []rpc.BatchElem) error
}

// DualEVMRPCClient routes ordinary calls over one RPC client and subscriptions/chain-tip calls over another.
type DualEVMRPCClient struct {
	CallClient EVMBatchRPCClient
	SubClient  EVMRPCClient
}

// CallContext forwards JSON-RPC calls to the ordinary call client.
func (c *DualEVMRPCClient) CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error {
	return c.CallClient.CallContext(ctx, result, method, args...)
}

// CallContextWithIntent forwards chain-tip JSON-RPC calls to the subscription client.
func (c *DualEVMRPCClient) CallContextWithIntent(ctx context.Context, intent EVMRPCIntent, result interface{}, method string, args ...interface{}) error {
	if intent == EVMRPCIntentChainTip {
		return c.SubClient.CallContextWithIntent(ctx, intent, result, method, args...)
	}
	return c.CallClient.CallContextWithIntent(ctx, intent, result, method, args...)
}

// BatchCallContext forwards batch JSON-RPC calls to the ordinary call client.
func (c *DualEVMRPCClient) BatchCallContext(ctx context.Context, batch []rpc.BatchElem) error {
	return c.CallClient.BatchCallContext(ctx, batch)
}

// EthSubscribe forwards subscriptions to the subscription client.
func (c *DualEVMRPCClient) EthSubscribe(ctx context.Context, channel interface{}, args ...interface{}) (EVMClientSubscription, error) {
	return c.SubClient.EthSubscribe(ctx, channel, args...)
}

// Close shuts down both underlying clients.
func (c *DualEVMRPCClient) Close() {
	if c.SubClient != nil {
		c.SubClient.Close()
	}
	if c.CallClient != nil && c.CallClient != c.SubClient {
		c.CallClient.Close()
	}
}

// EVMHeader provides access to the necessary header data for evm chain sync
type EVMHeader interface {
	Hash() string
	Number() *big.Int
	Difficulty() *big.Int
}

// EVMHash provides access to the necessary hash data for evm chain sync
type EVMHash interface {
	Hex() string
}

// EVMClientSubscription provides interaction with an evm client subscription
type EVMClientSubscription interface {
	Err() <-chan error
	Unsubscribe()
}

// EVMSubscriber provides interaction with a subscription channel
type EVMSubscriber interface {
	Channel() interface{}
	Close()
}

// EVMNewBlockSubscriber provides interaction with a new block subscription channel
type EVMNewBlockSubscriber interface {
	EVMSubscriber
	Read() (EVMHeader, bool)
}

// EVMNewBlockSubscriber provides interaction with a new tx subscription channel
type EVMNewTxSubscriber interface {
	EVMSubscriber
	Read() (EVMHash, bool)
}

// ToBlockNumArg converts a big.Int to an appropriate string representation of the number if possible
// - valid return values: (hex string, "latest", "pending", "earliest", "finalized", or "safe")
// - invalid return value: "invalid<number>"
func ToBlockNumArg(number *big.Int) string {
	if number == nil {
		return "latest"
	}
	if number.Sign() >= 0 {
		return hexutil.EncodeBig(number)
	}
	// It's negative.
	if number.IsInt64() {
		return rpc.BlockNumber(number.Int64()).String()
	}
	// It's negative and large, which is invalid.
	return fmt.Sprintf("<invalid %d>", number)
}
