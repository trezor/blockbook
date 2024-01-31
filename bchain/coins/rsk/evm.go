package rsk

import (
	"context"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/trezor/blockbook/bchain"
	"math/big"
	"strings"
)

// RskClient wraps a client to implement the EVMClient interface
type RskClient struct {
	*ethclient.Client
	*RskRPCClient
}

// HeaderByNumber returns a block header that implements the EVMHeader interface
func (c *RskClient) HeaderByNumber(ctx context.Context, number *big.Int) (bchain.EVMHeader, error) {
	h, err := rskHeaderByNumber(c.RskRPCClient, ctx, number)
	if err != nil {
		return nil, err
	}

	return h, nil
}

// EstimateGas returns the current estimated gas cost for executing a transaction
func (c *RskClient) EstimateGas(ctx context.Context, msg interface{}) (uint64, error) {
	return c.Client.EstimateGas(ctx, msg.(ethereum.CallMsg))
}

// BalanceAt returns the balance for the given account at a specific block, or latest known block if no block number is provided
func (c *RskClient) BalanceAt(ctx context.Context, addrDesc bchain.AddressDescriptor, blockNumber *big.Int) (*big.Int, error) {
	return c.Client.BalanceAt(ctx, common.BytesToAddress(addrDesc), blockNumber)
}

// NonceAt returns the nonce for the given account at a specific block, or latest known block if no block number is provided
func (c *RskClient) NonceAt(ctx context.Context, addrDesc bchain.AddressDescriptor, blockNumber *big.Int) (uint64, error) {
	return c.Client.NonceAt(ctx, common.BytesToAddress(addrDesc), blockNumber)
}

// RskRPCClient wraps a rpc client to implement the EVMRPCClient interface
type RskRPCClient struct {
	*rpc.Client
}

// EthSubscribe subscribes to events and returns a client subscription that implements the EVMClientSubscription interface
func (c *RskRPCClient) EthSubscribe(ctx context.Context, channel interface{}, args ...interface{}) (bchain.EVMClientSubscription, error) {
	sub, err := c.Client.EthSubscribe(ctx, channel, args...)
	if err != nil {
		return nil, err
	}

	return &RskClientSubscription{ClientSubscription: sub}, nil
}

// RskHeader wraps a block header to implement the EVMHeader interface
type RskHeader struct {
	RskHash     ethcommon.Hash    `json:"hash"`
	ParentHash  ethcommon.Hash    `json:"parentHash"`
	UncleHash   ethcommon.Hash    `json:"sha3Uncles"`
	Coinbase    ethcommon.Address `json:"miner"`
	Root        ethcommon.Hash    `json:"stateRoot"`
	TxHash      ethcommon.Hash    `json:"transactionsRoot"`
	ReceiptHash ethcommon.Hash    `json:"receiptsRoot"`
	//Bloom       []byte            `json:"logsBloom"`
	RskDifficulty string `json:"difficulty"`
	RskNumber     string `json:"number"`
	GasLimit      string `json:"gasLimit"`
	GasUsed       string `json:"gasUsed"`
	Time          string `json:"timestamp"`
	//Extra       []byte            `json:"extraData"`
}

// Hash returns the block hash as a hex string
func (h *RskHeader) Hash() string {
	return h.RskHash.Hex()
}

// Number returns the block number
func (h *RskHeader) Number() *big.Int {
	number, _ := big.NewInt(0).SetString(stripHex(h.RskNumber), 16)
	return number
}

// Difficulty returns the block difficulty
func (h *RskHeader) Difficulty() *big.Int {
	difficulty, _ := big.NewInt(0).SetString(stripHex(h.RskDifficulty), 16)
	return difficulty
}

// RskClientSubscription wraps a client subcription to implement the EVMClientSubscription interface
type RskClientSubscription struct {
	*rpc.ClientSubscription
}

// RskHeaderByNumber HeaderByNumber returns a RSK block header from the current canonical chain. If number is
// nil, the latest known header is returned.
func rskHeaderByNumber(b *RskRPCClient, ctx context.Context, number *big.Int) (*RskHeader, error) {
	var head *RskHeader
	err := b.Client.CallContext(ctx, &head, "eth_getBlockByNumber", toBlockNumArg(number), false)
	if err == nil && head == nil {
		err = ethereum.NotFound
	}
	return head, err
}

func toBlockNumArg(number *big.Int) string {
	if number == nil {
		return "latest"
	}
	pending := big.NewInt(-1)
	if number.Cmp(pending) == 0 {
		return "pending"
	}
	return hexutil.EncodeBig(number)
}

func stripHex(hexaString string) string {
	// replace 0x or 0X with empty String
	numberStr := strings.Replace(hexaString, "0x", "", -1)
	numberStr = strings.Replace(numberStr, "0X", "", -1)
	return numberStr
}
