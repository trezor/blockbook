package tron

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
)

const (
	MainnetGenesisHash     = "0x2b6653dc"
	NileTestnetGenesisHash = "0xcd8690dc"
)

// TronClient wraps the original go-ethereum Client and adds Tron-specific methods
type TronClient struct {
	*ethclient.Client
	rpcClient *TronRPCClient
}

// EstimateGas returns the current estimated gas cost for executing a transaction
func (c *TronClient) EstimateGas(ctx context.Context, msg interface{}) (uint64, error) {
	return c.Client.EstimateGas(ctx, msg.(ethereum.CallMsg))
}

// BalanceAt returns the balance for the given account at a specific block, or latest known block if no block number is provided
// IMPORTANT: Tron RPC only supports 'latest' block parameter. The blockNumber parameter is ignored.
func (c *TronClient) BalanceAt(ctx context.Context, addrDesc bchain.AddressDescriptor, blockNumber *big.Int) (*big.Int, error) {
	var result hexutil.Big
	err := c.rpcClient.CallContext(ctx, &result, "eth_getBalance", common.BytesToAddress(addrDesc), "latest")
	return (*big.Int)(&result), err
}

// NonceAt is not supported by Tron RPC
func (c *TronClient) NonceAt(ctx context.Context, addrDesc bchain.AddressDescriptor, blockNumber *big.Int) (uint64, error) {
	return 0, nil
}

// TronHash wraps a transaction hash to implement the EVMHash interface
type TronHash struct {
	common.Hash
}

type TronClientSubscription struct {
	*rpc.ClientSubscription
}

// TronNewBlock wraps a block header channel to implement the EVMNewBlockSubscriber interface
type TronNewBlock struct {
	channel chan *eth.EthereumHeader
}

// Close the underlying channel
func (s *TronNewBlock) Close() {
	close(s.channel)
}

// Channel returns the underlying channel as an empty interface
func (s *TronNewBlock) Channel() interface{} {
	return s.channel
}

// Read from the underlying channel and return a block header that implements the EVMHeader interface
func (s *TronNewBlock) Read() (bchain.EVMHeader, bool) {
	h, ok := <-s.channel
	if !ok {
		return nil, false
	}
	return h, true
}

// TronNewTx wraps a transaction hash channel to conform with the EVMNewTxSubscriber interface
type TronNewTx struct {
	channel chan common.Hash
}

// Channel returns the underlying channel as an empty interface
func (s *TronNewTx) Channel() interface{} {
	return s.channel
}

// Read from the underlying channel and return a transaction hash that implements the EVMHash interface
func (s *TronNewTx) Read() (bchain.EVMHash, bool) {
	h, ok := <-s.channel
	return &TronHash{Hash: h}, ok
}

// Close the underlying channel
func (s *TronNewTx) Close() {
	close(s.channel)
}

// TronRPCClient wraps an rpc client to implement the EVMRPCClient interface
type TronRPCClient struct {
	*rpc.Client
}

// EthSubscribe subscribes to events and returns a client subscription that implements the EVMClientSubscription interface
func (c *TronRPCClient) EthSubscribe(ctx context.Context, channel interface{}, args ...interface{}) (bchain.EVMClientSubscription, error) {
	sub, err := c.Client.EthSubscribe(ctx, channel, args...)
	if err != nil {
		return nil, err
	}

	return &TronClientSubscription{ClientSubscription: sub}, nil
}

func (c *TronClient) Close() {
	c.Client.Close()
}

func (c *TronClient) HeaderByNumber(ctx context.Context, number *big.Int) (bchain.EVMHeader, error) {
	h, err := c.rpcClient.HeaderByNumber(ctx, number)
	if err != nil {
		return nil, err
	}

	return h, nil
}

// NetworkID returns the network ID for this client.
// Tron RPC returns genesis block
func (c *TronClient) NetworkID(ctx context.Context) (*big.Int, error) {
	var ver string

	if err := c.rpcClient.CallContext(ctx, &ver, "net_version"); err != nil {
		return nil, err
	}

	switch ver {
	case MainnetGenesisHash:
		return big.NewInt(int64(MainNet)), nil
	case NileTestnetGenesisHash:
		return big.NewInt(int64(TestNetNile)), nil
	default:
		return nil, fmt.Errorf("invalid net_version result %q", ver)
	}
}

// HeaderByNumber returns the canonical header at number, or the latest one if number is nil
func (c *TronRPCClient) HeaderByNumber(ctx context.Context, number *big.Int) (*eth.EthereumHeader, error) {
	var head *eth.EthereumHeader
	err := c.CallContext(ctx, &head, "eth_getBlockByNumber", eth.ToBlockNumArg(number), false)
	if err == nil && head == nil {
		err = ethereum.NotFound
	}
	return head, err
}

func (c *TronRPCClient) CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error {
	var rawData json.RawMessage

	if err := c.Client.CallContext(ctx, &rawData, method, args...); err != nil {
		return err
	}

	// Clean up the response for Tron-specific (Tron has wrong stateRoot as '0x')
	// Skip when returning raw JSON to avoid an extra marshal/unmarshal cycle.
	if method == "eth_getBlockByHash" || method == "eth_getBlockByNumber" {
		if _, ok := result.(*json.RawMessage); !ok {
			rawData = fixStateRoot(rawData)
		}
	}

	return json.Unmarshal(rawData, result)
}

// fixStateRoot works around Tron JSON-RPC returning stateRoot in a format incompatible with go-ethereum
// Issue: Tron returns stateRoot as "0x" (empty) or with incorrect length, which causes go-ethereum
// deserialization to fail since it expects a valid 32-byte hash (66 chars: "0x" + 64 hex digits)
//
// This is likely because Tron uses a different state storage mechanism than Ethereum's MPT (Merkle Patricia Tree),
// but still tries to maintain API compatibility. The stateRoot field may not have the same meaning in Tron.
//
// Workaround: Replace invalid stateRoot with a zero hash to allow successful parsing by go-ethereum library
// Reference: https://github.com/tronprotocol/java-tron/issues/5518
func fixStateRoot(data []byte) []byte {
	const (
		stateRootBad  = `"stateRoot":"0x"`
		stateRootGood = `"stateRoot":"0x0000000000000000000000000000000000000000000000000000000000000000"`
	)

	if !bytes.Contains(data, []byte(stateRootBad)) {
		return data
	}

	return bytes.Replace(data, []byte(stateRootBad), []byte(stateRootGood), 1)
}
