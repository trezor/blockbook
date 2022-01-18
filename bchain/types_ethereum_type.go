package bchain

import "math/big"

// EthereumType specific

// EthereumInternalTransfer contains data about internal transfer
type EthereumInternalTransfer struct {
	Type  EthereumInternalTransactionType `json:"type"`
	From  string                          `json:"from"`
	To    string                          `json:"to"`
	Value big.Int                         `json:"value"`
}

// EthereumInternalTransactionType - type of ethereum transaction from internal data
type EthereumInternalTransactionType int

// EthereumInternalTransactionType enumeration
const (
	CALL = EthereumInternalTransactionType(iota)
	CREATE
	SELFDESTRUCT
)

// TokenTransferType - type of token transfer
type TokenTransferType int

// TokenTransferType enumeration
const (
	ERC20 = TokenTransferType(iota)
	ERC721
	ERC1155
)

// EthereumInternalTransaction contains internal transfers
type EthereumInternalData struct {
	Type      EthereumInternalTransactionType `json:"type"`
	Contract  string                          `json:"contract,omitempty"`
	Transfers []EthereumInternalTransfer      `json:"transfers,omitempty"`
	Error     string
}

// Erc20Contract contains info about ERC20 contract
type Erc20Contract struct {
	Contract string `json:"contract"`
	Name     string `json:"name"`
	Symbol   string `json:"symbol"`
	Decimals int    `json:"decimals"`
}

type TokenTransferIdValue struct {
	Id    big.Int
	Value big.Int
}

// TokenTransfer contains a single ERC20/ERC721/ERC1155 token transfer
type TokenTransfer struct {
	Type     TokenTransferType
	Contract string
	From     string
	To       string
	Value    big.Int
	IdValues []TokenTransferIdValue
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

type EthereumSpecificData struct {
	Tx           *RpcTransaction       `json:"tx"`
	InternalData *EthereumInternalData `json:"internalData,omitempty"`
	Receipt      *RpcReceipt           `json:"receipt,omitempty"`
}

type EthereumBlockSpecificData struct {
	InternalDataError string
}
