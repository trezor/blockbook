package bchain

import (
	"context"
	"math/big"
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
	Close()
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
