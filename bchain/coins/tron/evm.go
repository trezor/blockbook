package tron

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
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
	return 1, nil
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
	channel chan *types.Header
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
	return &TronHeader{Header: h}, ok
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

type TronHeader struct {
	*types.Header // Embed the original Header
	// use Hash of the block returned from RPC
	HashBlock common.Hash `json:"hash"       gencodec:"required"`
}

func (h *TronHeader) Hash() string {
	return h.HashBlock.Hex()
}

func (h *TronHeader) Number() *big.Int {
	return h.Header.Number
}

func (h *TronHeader) Difficulty() *big.Int {
	return h.Header.Difficulty
}

func (t *TronHeader) MarshalJSON() ([]byte, error) {
	type Alias TronHeader
	return json.Marshal(&struct {
		HashBlock common.Hash `json:"hash"`
		*Alias
	}{
		HashBlock: t.HashBlock,
		Alias:     (*Alias)(t),
	})
}

func (t *TronHeader) UnmarshalJSON(data []byte) error {
	// initialize Header
	if t.Header == nil {
		t.Header = &types.Header{}
	}

	var hashData struct {
		Hash string `json:"hash"`
	}
	if err := json.Unmarshal(data, &hashData); err != nil {
		return fmt.Errorf("error unmarshalling hash: %w", err)
	}

	// Decode the hash from hex string to `common.Hash`
	hashBytes, err := hexutil.Decode(hashData.Hash)
	if err != nil {
		return fmt.Errorf("invalid hash hex format: %w", err)
	}
	copy(t.HashBlock[:], hashBytes)

	// Unmarshal remaining data from Header
	if err := json.Unmarshal(data, t.Header); err != nil {
		return fmt.Errorf("error unmarshalling Header: %w", err)
	}

	return nil
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

// HeaderByNumber returns a block header from the current canonical chain. If number is
// nil, the latest known header is returned.
// overwriten so it returns TronHeader with Hash
func (c *TronRPCClient) HeaderByNumber(ctx context.Context, number *big.Int) (*TronHeader, error) {
	var head *TronHeader
	err := c.CallContext(ctx, &head, "eth_getBlockByNumber", toBlockNumArg(number), false)
	if err == nil && head == nil {
		err = ethereum.NotFound
	}
	return head, err
}

func toBlockNumArg(number *big.Int) string {
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

func (b *TronRPC) getBlockRaw(hash string, height uint32, fullTxs bool) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	var raw json.RawMessage
	var err error
	if hash != "" {
		// tron does not support 'pending', changed to "latest"
		if hash == "pending" {
			err = b.RPC.CallContext(ctx, &raw, "eth_getBlockByNumber", "latest", fullTxs)
		} else {
			err = b.RPC.CallContext(ctx, &raw, "eth_getBlockByHash", ethcommon.HexToHash(hash), fullTxs)
		}
	} else {
		err = b.RPC.CallContext(ctx, &raw, "eth_getBlockByNumber", fmt.Sprintf("%#x", height), fullTxs)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "hash %v, height %v", hash, height)
	} else if len(raw) == 0 || (len(raw) == 4 && string(raw) == "null") {
		return nil, bchain.ErrBlockNotFound
	}
	return raw, nil
}

func (c *TronRPCClient) CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error {
	var rawData json.RawMessage
	var err error

	if err := c.Client.CallContext(ctx, &rawData, method, args...); err != nil {
		return err
	}

	// Clean up the response for Tron-specific (Tron has wrong stateRoot as '0x')
	if method == "eth_getBlockByHash" || method == "eth_getBlockByNumber" {
		rawData, err = fixStateRoot(rawData)
		if err != nil {
			return err
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
func fixStateRoot(data []byte) ([]byte, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	if stateRoot, ok := raw["stateRoot"].(string); ok && (stateRoot == "0x" || len(stateRoot) != 66) {
		raw["stateRoot"] = "0x0000000000000000000000000000000000000000000000000000000000000000"
	}

	return json.Marshal(raw)
}
