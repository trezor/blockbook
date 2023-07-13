package xcb

import (
	"context"
	"math/big"

	"github.com/cryptohub-digital/blockbook-fork/bchain"
)

// CoreCoinInternalTransactionType - type of core coin transaction from internal data
type CoreCoinInternalTransactionType int

// CoreCoinInternalTransactionType enumeration
const (
	CALL = CoreCoinInternalTransactionType(iota)
	CREATE
	SELFDESTRUCT
)

// CoreCoinInternalTransfer contains data about internal transfer
type CoreCoinInternalTransfer struct {
	Type  CoreCoinInternalTransactionType `json:"type"`
	From  string                          `json:"from"`
	To    string                          `json:"to"`
	Value big.Int                         `json:"value"`
}

// RpcLog is returned by xcb_getLogs
type RpcLog struct {
	Address string   `json:"address"`
	Topics  []string `json:"topics"`
	Data    string   `json:"data"`
}

// RpcLog is returned by xcb_getTransactionReceipt
type RpcReceipt struct {
	EnergyUsed string    `json:"energyUsed"`
	Status     string    `json:"status"`
	Logs       []*RpcLog `json:"logs"`
}

// CoreCoinSpecificData contains data specific to Core Coin transactions
type CoreCoinSpecificData struct {
	Tx      *RpcTransaction `json:"tx"`
	Receipt *RpcReceipt     `json:"receipt,omitempty"`
}

// RpcTransaction is returned by xcb_getTransactionByHash
type RpcTransaction struct {
	AccountNonce     string `json:"nonce"`
	EnergyPrice      string `json:"energyPrice"`
	EnergyLimit      string `json:"energy"`
	To               string `json:"to"` // nil means contract creation
	Value            string `json:"value"`
	Payload          string `json:"input"`
	Hash             string `json:"hash"`
	BlockNumber      string `json:"blockNumber"`
	BlockHash        string `json:"blockHash,omitempty"`
	From             string `json:"from"`
	TransactionIndex string `json:"transactionIndex"`
}

// CVMClient provides the necessary client functionality for cvm chain sync
type CVMClient interface {
	NetworkID(ctx context.Context) (*big.Int, error)
	HeaderByNumber(ctx context.Context, number *big.Int) (CVMHeader, error)
	SuggestEnergyPrice(ctx context.Context) (*big.Int, error)
	EstimateEnergy(ctx context.Context, msg interface{}) (uint64, error)
	BalanceAt(ctx context.Context, addrDesc bchain.AddressDescriptor, blockNumber *big.Int) (*big.Int, error)
	NonceAt(ctx context.Context, addrDesc bchain.AddressDescriptor, blockNumber *big.Int) (uint64, error)
}

// CVMRPCClient provides the necessary rpc functionality for cvm chain sync
type CVMRPCClient interface {
	XcbSubscribe(ctx context.Context, channel interface{}, args ...interface{}) (CVMClientSubscription, error)
	CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error
	Close()
}

// CVMHeader provides access to the necessary header data for cvm chain sync
type CVMHeader interface {
	Hash() string
	Number() *big.Int
	Difficulty() *big.Int
}

// CVMHash provides access to the necessary hash data for cvm chain sync
type CVMHash interface {
	Hex() string
}

// CVMClientSubscription provides interaction with an cvm client subscription
type CVMClientSubscription interface {
	Err() <-chan error
	Unsubscribe()
}

// CVMSubscriber provides interaction with a subscription channel
type CVMSubscriber interface {
	Channel() interface{}
	Close()
}

// CVMNewBlockSubscriber provides interaction with a new block subscription channel
type CVMNewBlockSubscriber interface {
	CVMSubscriber
	Read() (CVMHeader, bool)
}

// CVMNewBlockSubscriber provides interaction with a new tx subscription channel
type CVMNewTxSubscriber interface {
	CVMSubscriber
	Read() (CVMHash, bool)
}

// CoreCoinBlockSpecificData contain data specific for Ethereum block
type CoreCoinBlockSpecificData struct {
	Contracts []bchain.ContractInfo
}

const (
	// crc token type names
	CRC20TokenType  bchain.TokenTypeName = "CRC20"
	CRC721TokenType bchain.TokenTypeName = "CRC721"
)
