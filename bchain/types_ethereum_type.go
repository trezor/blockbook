package bchain

import (
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

// EthereumInternalTransfer contains data about internal transfer
type EthereumInternalTransfer struct {
	Type  EthereumInternalTransactionType `json:"type"`
	From  string                          `json:"from"`
	To    string                          `json:"to"`
	Value big.Int                         `json:"value"`
}

// FourByteSignature contains data about about a contract function signature
type FourByteSignature struct {
	// stored in DB
	Name       string
	Parameters []string
	// processed from DB data and stored only in cache
	DecamelName      string
	Function         string
	ParsedParameters []abi.Type
}

// EthereumParsedInputParam contains data about a contract function parameter
type EthereumParsedInputParam struct {
	Type   string   `json:"type"`
	Values []string `json:"values,omitempty"`
}

// EthereumParsedInputData contains the parsed data for an input data hex payload
type EthereumParsedInputData struct {
	MethodId string                     `json:"methodId"`
	Name     string                     `json:"name"`
	Function string                     `json:"function,omitempty"`
	Params   []EthereumParsedInputParam `json:"params,omitempty"`
}

// EthereumInternalTransactionType - type of ethereum transaction from internal data
type EthereumInternalTransactionType int

// EthereumInternalTransactionType enumeration
const (
	CALL = EthereumInternalTransactionType(iota)
	CREATE
	SELFDESTRUCT
)

// EthereumInternalTransaction contains internal transfers
type EthereumInternalData struct {
	Type      EthereumInternalTransactionType `json:"type"`
	Contract  string                          `json:"contract,omitempty"`
	Transfers []EthereumInternalTransfer      `json:"transfers,omitempty"`
	Error     string
}

// ContractInfo contains info about a contract
type ContractInfo struct {
	Type              TokenTypeName `json:"type"`
	Contract          string        `json:"contract"`
	Name              string        `json:"name"`
	Symbol            string        `json:"symbol"`
	Decimals          int           `json:"decimals"`
	CreatedInBlock    uint32        `json:"createdInBlock,omitempty"`
	DestructedInBlock uint32        `json:"destructedInBlock,omitempty"`
}

// Ethereum token type names
const (
	ERC20TokenType   TokenTypeName = "ERC20"
	ERC771TokenType  TokenTypeName = "ERC721"
	ERC1155TokenType TokenTypeName = "ERC1155"
)

// EthereumTokenTypeMap maps bchain.TokenType to TokenTypeName
// the map must match all bchain.TokenType to avoid index out of range panic
var EthereumTokenTypeMap = []TokenTypeName{ERC20TokenType, ERC771TokenType, ERC1155TokenType}

type MultiTokenValue struct {
	Id    big.Int
	Value big.Int
}

// TokenTransfer contains a single token transfer
type TokenTransfer struct {
	Type             TokenType
	Contract         string
	From             string
	To               string
	Value            big.Int
	MultiTokenValues []MultiTokenValue
}

// RpcTransaction is returned by eth_getTransactionByHash
type RpcTransaction struct {
	AccountNonce     string `json:"nonce"`
	GasPrice         string `json:"gasPrice"`
	GasLimit         string `json:"gas"`
	To               string `json:"to"` // nil means contract creation
	Value            string `json:"value"`
	Payload          string `json:"input"`
	Hash             string `json:"hash"`
	BlockNumber      string `json:"blockNumber"`
	BlockHash        string `json:"blockHash,omitempty"`
	From             string `json:"from"`
	TransactionIndex string `json:"transactionIndex"`
	// Signature values - ignored
	// V string `json:"v"`
	// R string `json:"r"`
	// S string `json:"s"`
}

// RpcLog is returned by eth_getLogs
type RpcLog struct {
	Address string   `json:"address"`
	Topics  []string `json:"topics"`
	Data    string   `json:"data"`
}

// RpcLog is returned by eth_getTransactionReceipt
type RpcReceipt struct {
	GasUsed string    `json:"gasUsed"`
	Status  string    `json:"status"`
	Logs    []*RpcLog `json:"logs"`
}

// EthereumSpecificData contains data specific to Ethereum transactions
type EthereumSpecificData struct {
	Tx           *RpcTransaction       `json:"tx"`
	InternalData *EthereumInternalData `json:"internalData,omitempty"`
	Receipt      *RpcReceipt           `json:"receipt,omitempty"`
}

// AddressAliasRecord maps address to ENS name
type AddressAliasRecord struct {
	Address string
	Name    string
}

// EthereumBlockSpecificData contain data specific for Ethereum block
type EthereumBlockSpecificData struct {
	InternalDataError   string
	AddressAliasRecords []AddressAliasRecord
	Contracts           []ContractInfo
}

// StakingPool holds data about address participation in a staking pool contract
type StakingPoolData struct {
	Contract                string  `json:"contract"`
	Name                    string  `json:"name"`
	PendingBalance          big.Int `json:"pendingBalance"`          // pendingBalanceOf method
	PendingDepositedBalance big.Int `json:"pendingDepositedBalance"` // pendingDepositedBalanceOf method
	DepositedBalance        big.Int `json:"depositedBalance"`        // depositedBalanceOf method
	WithdrawTotalAmount     big.Int `json:"withdrawTotalAmount"`     // withdrawRequest method, return value [0]
	ClaimableAmount         big.Int `json:"claimableAmount"`         // withdrawRequest method, return value [1]
	RestakedReward          big.Int `json:"restakedReward"`          // restakedRewardOf method
	AutocompoundBalance     big.Int `json:"autocompoundBalance"`     // autocompoundBalanceOf method
}
