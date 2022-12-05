package avax

import (
	"context"
	"math/big"

	"github.com/ava-labs/coreth/core/types"
	"github.com/ava-labs/coreth/ethclient"
	avax "github.com/ava-labs/coreth/interfaces"
	"github.com/ethereum/go-ethereum/common"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
)

// AvalancheClient wraps the go-ethereum ethclient to conform with the EVMClient interface
type AvalancheClient struct {
	ethclient.Client
}

func (c *AvalancheClient) HeaderByNumber(ctx context.Context, number *big.Int) (eth.Header, error) {
	h, err := c.Client.HeaderByNumber(ctx, number)
	if err != nil {
		return nil, err
	}

	return &AvalancheHeader{Header: h}, nil
}

func (c *AvalancheClient) EstimateGas(ctx context.Context, msg interface{}) (uint64, error) {
	return c.Client.EstimateGas(ctx, msg.(avax.CallMsg))
}

func (c *AvalancheClient) BalanceAt(ctx context.Context, addrDesc bchain.AddressDescriptor, blockNumber *big.Int) (*big.Int, error) {
	return c.Client.BalanceAt(ctx, common.BytesToAddress(addrDesc), nil)
}

func (c *AvalancheClient) NonceAt(ctx context.Context, addrDesc bchain.AddressDescriptor, blockNumber *big.Int) (uint64, error) {
	return c.Client.NonceAt(ctx, common.BytesToAddress(addrDesc), nil)
}

type AvalancheHeader struct {
	*types.Header
}

func (h *AvalancheHeader) Hash() string {
	return h.Header.Hash().Hex()
}

func (h *AvalancheHeader) Number() *big.Int {
	return h.Header.Number
}

func (h *AvalancheHeader) Difficulty() *big.Int {
	return h.Header.Difficulty
}
