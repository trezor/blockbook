package bchain

import (
	"context"
	"math/big"
)

type EVMClient interface {
	NetworkID(ctx context.Context) (*big.Int, error)
	HeaderByNumber(ctx context.Context, number *big.Int) (EVMHeader, error)
	SuggestGasPrice(ctx context.Context) (*big.Int, error)
	EstimateGas(ctx context.Context, msg interface{}) (uint64, error)
	BalanceAt(ctx context.Context, addrDesc AddressDescriptor, blockNumber *big.Int) (*big.Int, error)
	NonceAt(ctx context.Context, addrDesc AddressDescriptor, blockNumber *big.Int) (uint64, error)
}

type EVMRPCClient interface {
	EthSubscribe(ctx context.Context, channel interface{}, args ...interface{}) (EVMClientSubscription, error)
	CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error
	Close()
}

type EVMHeader interface {
	Hash() string
	Number() *big.Int
	Difficulty() *big.Int
}

type EVMHash interface {
	Hex() string
}

type EVMClientSubscription interface {
	Err() <-chan error
	Unsubscribe()
}

type EVMSubscriber interface {
	Channel() interface{}
	Close()
}

type EVMNewBlockSubscriber interface {
	EVMSubscriber
	Read() (EVMHeader, bool)
}

type EVMNewTxSubscriber interface {
	EVMSubscriber
	Read() (EVMHash, bool)
}
