package bchain

import (
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

// EthereumInternalTransfer contains data about internal transfer
type EthereumInternalTransfer struct {
	Type  EthereumInternalTransactionType `json:"type" ts_doc:"The type of internal transaction (CALL, CREATE, SELFDESTRUCT)."`
	From  string                          `json:"from" ts_doc:"Sender address of this internal transfer."`
	To    string                          `json:"to" ts_doc:"Recipient address of this internal transfer."`
	Value big.Int                         `json:"value" ts_doc:"Amount (in Wei) transferred internally."`
}

// FourByteSignature contains data about a contract function signature
type FourByteSignature struct {
	// stored in DB
	Name       string   `ts_doc:"Original function name as stored in the database."`
	Parameters []string `ts_doc:"Raw parameter type definitions (e.g. ['uint256','address'])."`
	// processed from DB data and stored only in cache
	DecamelName      string     `ts_doc:"A decamelized version of the function name for readability."`
	Function         string     `ts_doc:"Reconstructed function definition string (e.g. 'transfer(address,uint256)')."`
	ParsedParameters []abi.Type `ts_doc:"ABI-parsed parameter types (cached for efficiency)."`
}

// EthereumParsedInputParam contains data about a contract function parameter
type EthereumParsedInputParam struct {
	Type   string   `json:"type" ts_doc:"Parameter type (e.g. 'uint256')."`
	Values []string `json:"values,omitempty" ts_doc:"List of stringified parameter values."`
}

// EthereumParsedInputData contains the parsed data for an input data hex payload
type EthereumParsedInputData struct {
	MethodId string                     `json:"methodId" ts_doc:"First 4 bytes of the input data (method signature ID)."`
	Name     string                     `json:"name" ts_doc:"Parsed function name if recognized."`
	Function string                     `json:"function,omitempty" ts_doc:"Full function signature (including parameter types)."`
	Params   []EthereumParsedInputParam `json:"params,omitempty" ts_doc:"List of parsed parameters for this function call."`
}

// EthereumInternalTransactionType - type of ethereum transaction from internal data
type EthereumInternalTransactionType int

// EthereumInternalTransactionType enumeration
const (
	CALL = EthereumInternalTransactionType(iota)
	CREATE
	SELFDESTRUCT
)

// EthereumInternalData contains internal transfers
type EthereumInternalData struct {
	Type      EthereumInternalTransactionType `json:"type" ts_doc:"High-level type of the internal transaction (CALL, CREATE, etc.)."`
	Contract  string                          `json:"contract,omitempty" ts_doc:"Address of the contract involved, if any."`
	Transfers []EthereumInternalTransfer      `json:"transfers,omitempty" ts_doc:"List of internal transfers associated with this data."`
	Error     string                          `ts_doc:"Error message if something went wrong while processing."`
}

// ContractInfo contains info about a contract
type ContractInfo struct {
	// Deprecated: Use Standard instead.
	Type              TokenStandardName `json:"type" ts_type:"'' | 'XPUBAddress' | 'ERC20' | 'ERC721' | 'ERC1155' | 'BEP20' | 'BEP721' | 'BEP1155'" ts_doc:"@deprecated: Use standard instead."`
	Standard          TokenStandardName `json:"standard" ts_type:"'' | 'XPUBAddress' | 'ERC20' | 'ERC721' | 'ERC1155' | 'BEP20' | 'BEP721' | 'BEP1155'"`
	Contract          string            `json:"contract" ts_doc:"Smart contract address."`
	Name              string            `json:"name" ts_doc:"Readable name of the contract."`
	Symbol            string            `json:"symbol" ts_doc:"Symbol for tokens under this contract, if applicable."`
	Decimals          int               `json:"decimals" ts_doc:"Number of decimal places, if applicable."`
	CreatedInBlock    uint32            `json:"createdInBlock,omitempty" ts_doc:"Block height where contract was first created."`
	DestructedInBlock uint32            `json:"destructedInBlock,omitempty" ts_doc:"Block height where contract was destroyed (if any)."`
}

// Ethereum token standard names
const (
	ERC20TokenStandard   TokenStandardName = "ERC20"
	ERC771TokenStandard  TokenStandardName = "ERC721"
	ERC1155TokenStandard TokenStandardName = "ERC1155"
)

// EthereumTokenStandardMap maps bchain.TokenStandard to TokenStandardName
// the map must match all bchain.TokenStandard to avoid index out of range panic
var EthereumTokenStandardMap = []TokenStandardName{ERC20TokenStandard, ERC771TokenStandard, ERC1155TokenStandard}

// MultiTokenValue holds one ID-value pair for multi-token standards like ERC1155
type MultiTokenValue struct {
	Id    big.Int `ts_doc:"Token ID for this multi-token entry."`
	Value big.Int `ts_doc:"Amount of the token ID transferred or owned."`
}

// TokenTransfer contains a single token transfer
type TokenTransfer struct {
	Standard         TokenStandard     `ts_doc:"Integer value od the token standard."`
	Contract         string            `ts_doc:"Smart contract address for the token."`
	From             string            `ts_doc:"Sender address of the token transfer."`
	To               string            `ts_doc:"Recipient address of the token transfer."`
	Value            big.Int           `ts_doc:"Amount of tokens transferred (for fungible tokens)."`
	MultiTokenValues []MultiTokenValue `ts_doc:"List of ID-value pairs for multi-token transfers (e.g., ERC1155)."`
}

// RpcTransaction is returned by eth_getTransactionByHash
type RpcTransaction struct {
	AccountNonce         string `json:"nonce" ts_doc:"Transaction nonce from the sender's account."`
	GasPrice             string `json:"gasPrice" ts_doc:"Gas price bid by the sender in Wei."`
	MaxPriorityFeePerGas string `json:"maxPriorityFeePerGas,omitempty"`
	MaxFeePerGas         string `json:"maxFeePerGas,omitempty"`
	BaseFeePerGas        string `json:"baseFeePerGas,omitempty"`
	GasLimit             string `json:"gas" ts_doc:"Maximum gas allowed for this transaction."`
	To                   string `json:"to" ts_doc:"Recipient address if not a contract creation. Empty if it's contract creation."`
	Value                string `json:"value" ts_doc:"Amount of Ether (in Wei) sent in this transaction."`
	Payload              string `json:"input" ts_doc:"Hex-encoded input data for contract calls."`
	Hash                 string `json:"hash" ts_doc:"Transaction hash."`
	BlockNumber          string `json:"blockNumber" ts_doc:"Block number where this transaction was included, if mined."`
	BlockHash            string `json:"blockHash,omitempty" ts_doc:"Hash of the block in which this transaction was included, if mined."`
	From                 string `json:"from" ts_doc:"Sender's address derived by the backend."`
	TransactionIndex     string `json:"transactionIndex" ts_doc:"Index of the transaction within the block, if mined."`
	// Signature values - ignored
	// V string `json:"v"`
	// R string `json:"r"`
	// S string `json:"s"`
}

// RpcLog is returned by eth_getLogs
type RpcLog struct {
	Address string   `json:"address" ts_doc:"Contract or address from which this log originated."`
	Topics  []string `json:"topics" ts_doc:"Indexed event signatures and parameters."`
	Data    string   `json:"data" ts_doc:"Unindexed event data in hex form."`
}

// RpcReceipt is returned by eth_getTransactionReceipt
type RpcReceipt struct {
	GasUsed     string    `json:"gasUsed" ts_doc:"Amount of gas actually used by the transaction."`
	Status      string    `json:"status" ts_doc:"Transaction execution status (0x0 = fail, 0x1 = success)."`
	Logs        []*RpcLog `json:"logs" ts_doc:"Array of log entries generated by this transaction."`
	L1Fee       string    `json:"l1Fee,omitempty" ts_doc:"Additional Layer 1 fee, if on a rollup network."`
	L1FeeScalar string    `json:"l1FeeScalar,omitempty" ts_doc:"Fee scaling factor for L1 fees on some L2s."`
	L1GasPrice  string    `json:"l1GasPrice,omitempty" ts_doc:"Gas price used on L1 for the rollup network."`
	L1GasUsed   string    `json:"l1GasUsed,omitempty" ts_doc:"Amount of L1 gas used by the transaction, if any."`
}

// EthereumSpecificData contains data specific to Ethereum transactions
type EthereumSpecificData struct {
	Tx           *RpcTransaction       `json:"tx" ts_doc:"Raw transaction details from the blockchain node."`
	InternalData *EthereumInternalData `json:"internalData,omitempty" ts_doc:"Summary of internal calls/transfers, if any."`
	Receipt      *RpcReceipt           `json:"receipt,omitempty" ts_doc:"Transaction receipt info, including logs and gas usage."`
}

// AddressAliasRecord maps address to ENS name
type AddressAliasRecord struct {
	Address string `ts_doc:"Address whose alias is being stored."`
	Name    string `ts_doc:"The resolved name/alias (e.g. ENS domain)."`
}

// EthereumBlockSpecificData contain data specific for Ethereum block
type EthereumBlockSpecificData struct {
	InternalDataError   string               `ts_doc:"Error message for processing block internal data, if any."`
	AddressAliasRecords []AddressAliasRecord `ts_doc:"List of address-to-alias mappings discovered in this block."`
	Contracts           []ContractInfo       `ts_doc:"List of contracts created or updated in this block."`
}

// StakingPoolData holds data about address participation in a staking pool contract
type StakingPoolData struct {
	Contract                string  `json:"contract" ts_doc:"Address of the staking pool contract."`
	Name                    string  `json:"name" ts_doc:"Human-readable name of the staking pool."`
	PendingBalance          big.Int `json:"pendingBalance" ts_doc:"Amount not yet finalized in the pool (pendingBalanceOf)."`
	PendingDepositedBalance big.Int `json:"pendingDepositedBalance" ts_doc:"Amount pending deposit (pendingDepositedBalanceOf)."`
	DepositedBalance        big.Int `json:"depositedBalance" ts_doc:"Total amount currently deposited (depositedBalanceOf)."`
	WithdrawTotalAmount     big.Int `json:"withdrawTotalAmount" ts_doc:"Total amount requested for withdrawal (withdrawRequest[0])."`
	ClaimableAmount         big.Int `json:"claimableAmount" ts_doc:"Amount that can be claimed (withdrawRequest[1])."`
	RestakedReward          big.Int `json:"restakedReward" ts_doc:"Total reward that has been restaked (restakedRewardOf)."`
	AutocompoundBalance     big.Int `json:"autocompoundBalance" ts_doc:"Auto-compounded balance (autocompoundBalanceOf)."`
}

// Eip1559Fee
type Eip1559Fee struct {
	MaxFeePerGas         *big.Int `json:"maxFeePerGas"`
	MaxPriorityFeePerGas *big.Int `json:"maxPriorityFeePerGas"`
	MinWaitTimeEstimate  int      `json:"minWaitTimeEstimate,omitempty"`
	MaxWaitTimeEstimate  int      `json:"maxWaitTimeEstimate,omitempty"`
}

// Eip1559Fees
type Eip1559Fees struct {
	BaseFeePerGas              *big.Int    `json:"baseFeePerGas,omitempty"`
	Low                        *Eip1559Fee `json:"low,omitempty"`
	Medium                     *Eip1559Fee `json:"medium,omitempty"`
	High                       *Eip1559Fee `json:"high,omitempty"`
	Instant                    *Eip1559Fee `json:"instant,omitempty"`
	NetworkCongestion          float64     `json:"networkCongestion,omitempty"`
	LatestPriorityFeeRange     []*big.Int  `json:"latestPriorityFeeRange,omitempty"`
	HistoricalPriorityFeeRange []*big.Int  `json:"historicalPriorityFeeRange,omitempty"`
	HistoricalBaseFeeRange     []*big.Int  `json:"historicalBaseFeeRange,omitempty"`
	PriorityFeeTrend           string      `json:"priorityFeeTrend,omitempty"`
	BaseFeeTrend               string      `json:"baseFeeTrend,omitempty"`
}
