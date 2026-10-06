package eth

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/trezor/blockbook/bchain"
)

// EthereumClient wraps a client to implement the EVMClient interface
type EthereumClient struct {
	*ethclient.Client
}

// HeaderByNumber returns a block header that implements the EVMHeader interface
func (c *EthereumClient) HeaderByNumber(ctx context.Context, number *big.Int) (bchain.EVMHeader, error) {
	// ethclient's header drops the node's hash, so decode the same call into EthereumHeader
	var h *EthereumHeader
	err := c.Client.Client().CallContext(ctx, &h, "eth_getBlockByNumber", ToBlockNumArg(number), false)
	if err == nil && h == nil {
		err = ethereum.NotFound
	}
	if err != nil {
		return nil, err
	}
	return h, nil
}

// ToBlockNumArg formats a block number as an eth_getBlockByNumber argument, nil meaning latest
func ToBlockNumArg(number *big.Int) string {
	if number == nil {
		return "latest"
	}
	if number.Sign() >= 0 {
		return hexutil.EncodeBig(number)
	}
	if number.IsInt64() {
		return rpc.BlockNumber(number.Int64()).String()
	}
	return fmt.Sprintf("<invalid %d>", number)
}

// EstimateGas returns the current estimated gas cost for executing a transaction
func (c *EthereumClient) EstimateGas(ctx context.Context, msg interface{}) (uint64, error) {
	return c.Client.EstimateGas(ctx, msg.(ethereum.CallMsg))
}

// BalanceAt returns the balance for the given account at a specific block, or latest known block if no block number is provided
func (c *EthereumClient) BalanceAt(ctx context.Context, addrDesc bchain.AddressDescriptor, blockNumber *big.Int) (*big.Int, error) {
	return c.Client.BalanceAt(ctx, common.BytesToAddress(addrDesc), blockNumber)
}

// NonceAt returns the nonce for the given account at a specific block, or latest known block if no block number is provided
func (c *EthereumClient) NonceAt(ctx context.Context, addrDesc bchain.AddressDescriptor, blockNumber *big.Int) (uint64, error) {
	return c.Client.NonceAt(ctx, common.BytesToAddress(addrDesc), blockNumber)
}

// EthereumRPCClient wraps an rpc client to implement the EVMRPCClient interface
type EthereumRPCClient struct {
	*rpc.Client
}

// DualRPCClient routes calls over HTTP and subscriptions over WebSocket.
type DualRPCClient struct {
	CallClient *rpc.Client
	SubClient  *rpc.Client
}

// CallContext forwards JSON-RPC calls to the HTTP client.
func (c *DualRPCClient) CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error {
	return c.CallClient.CallContext(ctx, result, method, args...)
}

// BatchCallContext forwards batch JSON-RPC calls to the HTTP client.
func (c *DualRPCClient) BatchCallContext(ctx context.Context, batch []rpc.BatchElem) error {
	return c.CallClient.BatchCallContext(ctx, batch)
}

// EthSubscribe forwards subscriptions to the WebSocket client.
func (c *DualRPCClient) EthSubscribe(ctx context.Context, channel interface{}, args ...interface{}) (bchain.EVMClientSubscription, error) {
	sub, err := c.SubClient.EthSubscribe(ctx, channel, args...)
	if err != nil {
		return nil, err
	}
	return &EthereumClientSubscription{ClientSubscription: sub}, nil
}

// Close shuts down both underlying clients.
func (c *DualRPCClient) Close() {
	if c.SubClient != nil {
		c.SubClient.Close()
	}
	if c.CallClient != nil && c.CallClient != c.SubClient {
		c.CallClient.Close()
	}
}

// EthSubscribe subscribes to events and returns a client subscription that implements the EVMClientSubscription interface
func (c *EthereumRPCClient) EthSubscribe(ctx context.Context, channel interface{}, args ...interface{}) (bchain.EVMClientSubscription, error) {
	sub, err := c.Client.EthSubscribe(ctx, channel, args...)
	if err != nil {
		return nil, err
	}

	return &EthereumClientSubscription{ClientSubscription: sub}, nil
}

// EthereumHeader wraps a block header to implement the EVMHeader interface
type EthereumHeader struct {
	*types.Header
	// hash reported by the node; types.Header.Hash() misses header fields unknown to go-ethereum
	BlockHash common.Hash
}

// Hash returns the node-reported block hash, falling back to the locally computed one
func (h *EthereumHeader) Hash() string {
	if h.BlockHash != (common.Hash{}) {
		return h.BlockHash.Hex()
	}
	return h.Header.Hash().Hex()
}

// UnmarshalJSON decodes the header and keeps the node-reported hash
func (h *EthereumHeader) UnmarshalJSON(data []byte) error {
	var head types.Header
	if err := json.Unmarshal(data, &head); err != nil {
		return err
	}
	var hash struct {
		Hash common.Hash `json:"hash"`
	}
	if err := json.Unmarshal(data, &hash); err != nil {
		return err
	}
	h.Header, h.BlockHash = &head, hash.Hash
	return nil
}

// ParentHash returns the parent block hash as a hex string
func (h *EthereumHeader) ParentHash() string {
	return h.Header.ParentHash.Hex()
}

// Number returns the block number
func (h *EthereumHeader) Number() *big.Int {
	return h.Header.Number
}

// Difficulty returns the block difficulty
func (h *EthereumHeader) Difficulty() *big.Int {
	return h.Header.Difficulty
}

// EthereumHash wraps a transaction hash to implement the EVMHash interface
type EthereumHash struct {
	common.Hash
}

// EthereumClientSubscription wraps a client subcription to implement the EVMClientSubscription interface
type EthereumClientSubscription struct {
	*rpc.ClientSubscription
}

// EthereumNewBlock wraps a block header channel to implement the EVMNewBlockSubscriber interface
type EthereumNewBlock struct {
	channel chan *EthereumHeader
}

// NewEthereumNewBlock returns an initialized EthereumNewBlock struct
func NewEthereumNewBlock() *EthereumNewBlock {
	return &EthereumNewBlock{channel: make(chan *EthereumHeader)}
}

// Channel returns the underlying channel as an empty interface
func (s *EthereumNewBlock) Channel() interface{} {
	return s.channel
}

// Read from the underlying channel and return a block header that implements the EVMHeader interface
func (s *EthereumNewBlock) Read() (bchain.EVMHeader, bool) {
	h, ok := <-s.channel
	if !ok {
		return nil, false
	}
	return h, true
}

// Close the underlying channel
func (s *EthereumNewBlock) Close() {
	close(s.channel)
}

// EthereumNewTx wraps a transaction hash channel to implement the EVMNewTxSubscriber interface
type EthereumNewTx struct {
	channel chan common.Hash
}

// NewEthereumNewTx returns an initialized EthereumNewTx struct
func NewEthereumNewTx() *EthereumNewTx {
	return &EthereumNewTx{channel: make(chan common.Hash)}
}

// Channel returns the underlying channel as an empty interface
func (s *EthereumNewTx) Channel() interface{} {
	return s.channel
}

// Read from the underlying channel and return a transaction hash that implements the EVMHash interface
func (s *EthereumNewTx) Read() (bchain.EVMHash, bool) {
	h, ok := <-s.channel
	return &EthereumHash{Hash: h}, ok
}

// Close the underlying channel
func (s *EthereumNewTx) Close() {
	close(s.channel)
}
