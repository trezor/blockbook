package avalanche

import (
	"context"
	"math/big"
	"strings"

	"github.com/ava-labs/coreth/core/types"
	"github.com/ava-labs/coreth/ethclient"
	avax "github.com/ava-labs/coreth/interfaces"
	"github.com/ava-labs/coreth/rpc"
	"github.com/ethereum/go-ethereum/common"
	"github.com/trezor/blockbook/bchain"
)

// AvalancheClient wraps the go-ethereum ethclient to conform with the EVMClient interface
type AvalancheClient struct {
	ethclient.Client
}

func (c *AvalancheClient) HeaderByNumber(ctx context.Context, number *big.Int) (bchain.EVMHeader, error) {
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

// AvalancheRPCClient wraps the go-ethereum rpc client to conform with the Client interface
type AvalancheRPCClient struct {
	*rpc.Client
}

func (c *AvalancheRPCClient) EthSubscribe(ctx context.Context, channel interface{}, args ...interface{}) (bchain.EVMClientSubscription, error) {
	sub, err := c.Client.EthSubscribe(ctx, channel, args...)
	if err != nil {
		return nil, err
	}

	return &AvalancheClientSubscription{ClientSubscription: sub}, nil
}

func (c *AvalancheRPCClient) CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error {
	err := c.Client.CallContext(ctx, result, method, args...)
	// unfinalized data cannot be queried error returned when trying to query a block height greater than last finalized block
	// do not throw rpc error and instead treat as ErrBlockNotFound
	// https://docs.avax.network/quickstart/exchanges/integrate-exchange-with-avalanche#determining-finality
	if err != nil && strings.Contains(err.Error(), "cannot query unfinalized data") {
		err = nil
	}
	return err
}

// AvalancheHeader wraps the avalanche header to conform with the Header interface
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

// AvalancheHash wraps the avalanche hash to conform with the Hash interface
type AvalancheHash struct {
	common.Hash
}

// AvalancheClientSubscription wraps an avalanche client subcription to conform with the ClientSubscription interface
type AvalancheClientSubscription struct {
	*rpc.ClientSubscription
}

// AvalancheNewBlock wraps an avalanche header channel to conform with the Subscriber interface
type AvalancheNewBlock struct {
	channel chan *types.Header
}

func (s *AvalancheNewBlock) Channel() interface{} {
	return s.channel
}

func (s *AvalancheNewBlock) Read() (bchain.EVMHeader, bool) {
	h, ok := <-s.channel
	return &AvalancheHeader{Header: h}, ok
}

func (s *AvalancheNewBlock) Close() {
	close(s.channel)
}

// AvalancheNewTx wraps an ethereum transaction hash channel to conform with the Subscriber interface
type AvalancheNewTx struct {
	channel chan common.Hash
}

func (s *AvalancheNewTx) Channel() interface{} {
	return s.channel
}

func (s *AvalancheNewTx) Read() (bchain.EVMHash, bool) {
	h, ok := <-s.channel
	return &AvalancheHash{Hash: h}, ok
}

func (s *AvalancheNewTx) Close() {
	close(s.channel)
}
