package eth

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/trezor/blockbook/bchain"
)

// Client interface for evm chains
type Client interface {
	NetworkID(ctx context.Context) (*big.Int, error)
	HeaderByNumber(ctx context.Context, number *big.Int) (Header, error)
	SuggestGasPrice(ctx context.Context) (*big.Int, error)
	EstimateGas(ctx context.Context, msg interface{}) (uint64, error)
	BalanceAt(ctx context.Context, addrDesc bchain.AddressDescriptor, blockNumber *big.Int) (*big.Int, error)
	NonceAt(ctx context.Context, addrDesc bchain.AddressDescriptor, blockNumber *big.Int) (uint64, error)
}

// EthereumClient wraps the go-ethereum ethclient to conform with the Client interface
type EthereumClient struct {
	*ethclient.Client
}

func (c *EthereumClient) HeaderByNumber(ctx context.Context, number *big.Int) (Header, error) {
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

// Header interface for evm chains
type Header interface {
	Hash() string
	Number() *big.Int
	Difficulty() *big.Int
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

// Hash interface for evm chains
type Hash interface {
	Hex() string
}

// EthereumHash wraps the ethereum hash to conform with the Hash interface
type EthereumHash struct {
	common.Hash
}
