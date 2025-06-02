package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/db"
)

const maxUint32 = ^uint32(0)
const maxInt = int(^uint(0) >> 1)
const maxInt64 = int64(^uint64(0) >> 1)

// AccountDetails specifies what data returns GetAddress and GetXpub calls
type AccountDetails int

const (
	// AccountDetailsBasic - only that address is indexed and some basic info
	AccountDetailsBasic AccountDetails = iota
	// AccountDetailsTokens - basic info + tokens
	AccountDetailsTokens
	// AccountDetailsTokenBalances - basic info + token with balance
	AccountDetailsTokenBalances
	// AccountDetailsTxidHistory - basic + token balances + txids, subject to paging
	AccountDetailsTxidHistory
	// AccountDetailsTxHistoryLight - basic + tokens + easily obtained tx data (not requiring requests to backend), subject to paging
	AccountDetailsTxHistoryLight
	// AccountDetailsTxHistory - basic + tokens + full tx data, subject to paging
	AccountDetailsTxHistory
)

// ErrUnsupportedXpub is returned when coin type does not support xpub address derivation or provided string is not an xpub
var ErrUnsupportedXpub = errors.New("XPUB not supported")

// APIError extends error by information if the error details should be returned to the end user
type APIError struct {
	Text   string `ts_doc:"Human-readable error message describing the issue."`
	Public bool   `ts_doc:"Whether the error message can safely be shown to the end user."`
}

func (e *APIError) Error() string {
	return e.Text
}

// NewAPIError creates ApiError
func NewAPIError(s string, public bool) error {
	return &APIError{
		Text:   s,
		Public: public,
	}
}

// Amount is a datatype holding amounts
type Amount big.Int

// IsZeroBigInt checks if big int has zero value
func IsZeroBigInt(b *big.Int) bool {
	return len(b.Bits()) == 0
}

// Compare returns an integer comparing two Amounts. The result will be 0 if a == b, -1 if a < b, and +1 if a > b.
// Nil Amount is always less then non-nil amount, two nil Amounts are equal
func (a *Amount) Compare(b *Amount) int {
	if b == nil {
		if a == nil {
			return 0
		}
		return 1
	}
	if a == nil {
		return -1
	}
	return (*big.Int)(a).Cmp((*big.Int)(b))
}

// MarshalJSON Amount serialization
func (a *Amount) MarshalJSON() (out []byte, err error) {
	if a == nil {
		return []byte(`"0"`), nil
	}
	return []byte(`"` + (*big.Int)(a).String() + `"`), nil
}

func (a *Amount) UnmarshalJSON(data []byte) error {
	s := strings.Trim(string(data), "\"")
	if len(s) > 0 {
		bigValue, parsed := new(big.Int).SetString(s, 10)
		if !parsed {
			return fmt.Errorf("couldn't parse number: %s", s)
		}
		*a = Amount(*bigValue)
	} else {
		// assuming empty string means zero
		*a = Amount{}
	}
	return nil
}

func (a *Amount) String() string {
	if a == nil {
		return ""
	}
	return (*big.Int)(a).String()
}

// DecimalString returns amount with decimal point placed according to parameter d
func (a *Amount) DecimalString(d int) string {
	return bchain.AmountToDecimalString((*big.Int)(a), d)
}

// AsBigInt returns big.Int type for the Amount (empty if Amount is nil)
func (a *Amount) AsBigInt() big.Int {
	if a == nil {
		return *new(big.Int)
	}
	return big.Int(*a)
}

// AsInt64 returns Amount as int64 (0 if Amount is nil).
// It is used only for legacy interfaces (socket.io)
// and generally not recommended to use for possible loss of precision.
func (a *Amount) AsInt64() int64 {
	if a == nil {
		return 0
	}
	return (*big.Int)(a).Int64()
}

// Vin contains information about single transaction input
type Vin struct {
	Txid      string                   `json:"txid,omitempty" ts_doc:"ID/hash of the originating transaction (where the UTXO comes from)."`
	Vout      uint32                   `json:"vout,omitempty" ts_doc:"Index of the output in the referenced transaction."`
	Sequence  int64                    `json:"sequence,omitempty" ts_doc:"Sequence number for this input (e.g. 4294967293)."`
	N         int                      `json:"n" ts_doc:"Relative index of this input within the transaction."`
	AddrDesc  bchain.AddressDescriptor `json:"-" ts_doc:"Internal address descriptor for backend usage (not exposed via JSON)."`
	Addresses []string                 `json:"addresses,omitempty" ts_doc:"List of addresses associated with this input."`
	IsAddress bool                     `json:"isAddress" ts_doc:"Indicates if this input is from a known address."`
	IsOwn     bool                     `json:"isOwn,omitempty" ts_doc:"Indicates if this input belongs to the wallet in context."`
	ValueSat  *Amount                  `json:"value,omitempty" ts_doc:"Amount (in satoshi or base units) of the input."`
	Hex       string                   `json:"hex,omitempty" ts_doc:"Raw script hex data for this input."`
	Asm       string                   `json:"asm,omitempty" ts_doc:"Disassembled script for this input."`
	Coinbase  string                   `json:"coinbase,omitempty" ts_doc:"Data for coinbase inputs (when mining)."`
}

// Vout contains information about single transaction output
type Vout struct {
	ValueSat    *Amount                  `json:"value,omitempty" ts_doc:"Amount (in satoshi or base units) of the output."`
	N           int                      `json:"n" ts_doc:"Relative index of this output within the transaction."`
	Spent       bool                     `json:"spent,omitempty" ts_doc:"Indicates whether this output has been spent."`
	SpentTxID   string                   `json:"spentTxId,omitempty" ts_doc:"Transaction ID in which this output was spent."`
	SpentIndex  int                      `json:"spentIndex,omitempty" ts_doc:"Index of the input that spent this output."`
	SpentHeight int                      `json:"spentHeight,omitempty" ts_doc:"Block height at which this output was spent."`
	Hex         string                   `json:"hex,omitempty" ts_doc:"Raw script hex data for this output - aka ScriptPubKey."`
	Asm         string                   `json:"asm,omitempty" ts_doc:"Disassembled script for this output."`
	AddrDesc    bchain.AddressDescriptor `json:"-" ts_doc:"Internal address descriptor for backend usage (not exposed via JSON)."`
	Addresses   []string                 `json:"addresses" ts_doc:"List of addresses associated with this output."`
	IsAddress   bool                     `json:"isAddress" ts_doc:"Indicates whether this output is owned by valid address."`
	IsOwn       bool                     `json:"isOwn,omitempty" ts_doc:"Indicates if this output belongs to the wallet in context."`
	Type        string                   `json:"type,omitempty" ts_doc:"Output script type (e.g., 'P2PKH', 'P2SH')."`
}

// MultiTokenValue contains values for contracts with multiple token IDs
type MultiTokenValue struct {
	Id    *Amount `json:"id,omitempty" ts_doc:"Token ID (for ERC1155)."`
	Value *Amount `json:"value,omitempty" ts_doc:"Amount of that specific token ID."`
}

// Token contains info about tokens held by an address
type Token struct {
	// Deprecated: Use Standard instead.
	Type             bchain.TokenStandardName `json:"type" ts_type:"'' | 'XPUBAddress' | 'ERC20' | 'ERC721' | 'ERC1155' | 'BEP20' | 'BEP721' | 'BEP1155'" ts_doc:"@deprecated: Use standard instead."`
	Standard         bchain.TokenStandardName `json:"standard" ts_type:"'' | 'XPUBAddress' | 'ERC20' | 'ERC721' | 'ERC1155' | 'BEP20' | 'BEP721' | 'BEP1155'"`
	Name             string                   `json:"name" ts_doc:"Readable name of the token."`
	Path             string                   `json:"path,omitempty" ts_doc:"Derivation path if this token is derived from an XPUB-based address."`
	Contract         string                   `json:"contract,omitempty" ts_doc:"Contract address on-chain."`
	Transfers        int                      `json:"transfers" ts_doc:"Total number of token transfers for this address."`
	Symbol           string                   `json:"symbol,omitempty" ts_doc:"Symbol for the token (e.g., 'ETH', 'USDT')."`
	Decimals         int                      `json:"decimals,omitempty" ts_doc:"Number of decimals for this token."`
	BalanceSat       *Amount                  `json:"balance,omitempty" ts_doc:"Current token balance (in minimal base units)."`
	BaseValue        float64                  `json:"baseValue,omitempty" ts_doc:"Value in the base currency (e.g. ETH for ERC20 tokens)."`
	SecondaryValue   float64                  `json:"secondaryValue,omitempty" ts_doc:"Value in a secondary currency (e.g. fiat), if available."`
	Ids              []Amount                 `json:"ids,omitempty" ts_doc:"List of token IDs (for ERC721, each ID is a unique collectible)."`
	MultiTokenValues []MultiTokenValue        `json:"multiTokenValues,omitempty" ts_doc:"Multiple ERC1155 token balances (id + value)."`
	TotalReceivedSat *Amount                  `json:"totalReceived,omitempty" ts_doc:"Total amount of tokens received."`
	TotalSentSat     *Amount                  `json:"totalSent,omitempty" ts_doc:"Total amount of tokens sent."`
	ContractIndex    string                   `json:"-"`
}

// Tokens is array of Token
type Tokens []Token

func (a Tokens) Len() int      { return len(a) }
func (a Tokens) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a Tokens) Less(i, j int) bool {
	ti := &a[i]
	tj := &a[j]
	// sort by BaseValue descending  and then Name and then by Contract
	if ti.BaseValue < tj.BaseValue {
		return false
	} else if ti.BaseValue > tj.BaseValue {
		return true
	}
	if ti.Name == "" {
		if tj.Name != "" {
			return false
		}
	} else {
		if tj.Name == "" {
			return true
		}
		return ti.Name < tj.Name
	}
	return ti.Contract < tj.Contract
}

// TokenTransfer contains info about a token transfer done in a transaction
type TokenTransfer struct {
	// Deprecated: Use Standard instead.
	Type             bchain.TokenStandardName `json:"type" ts_type:"'' | 'XPUBAddress' | 'ERC20' | 'ERC721' | 'ERC1155' | 'BEP20' | 'BEP721' | 'BEP1155'" ts_doc:"@deprecated: Use standard instead."`
	Standard         bchain.TokenStandardName `json:"standard" ts_type:"'' | 'XPUBAddress' | 'ERC20' | 'ERC721' | 'ERC1155' | 'BEP20' | 'BEP721' | 'BEP1155'"`
	From             string                   `json:"from" ts_doc:"Source address of the token transfer."`
	To               string                   `json:"to" ts_doc:"Destination address of the token transfer."`
	Contract         string                   `json:"contract" ts_doc:"Contract address of the token."`
	Name             string                   `json:"name,omitempty" ts_doc:"Token name."`
	Symbol           string                   `json:"symbol,omitempty" ts_doc:"Token symbol."`
	Decimals         int                      `json:"decimals,omitempty" ts_doc:"Number of decimals for this token (if applicable)."`
	Value            *Amount                  `json:"value,omitempty" ts_doc:"Amount (in base units) of tokens transferred."`
	MultiTokenValues []MultiTokenValue        `json:"multiTokenValues,omitempty" ts_doc:"List of multiple ID-value pairs for ERC1155 transfers."`
}

// EthereumInternalTransfer represents internal transaction data in Ethereum-like blockchains
type EthereumInternalTransfer struct {
	Type  bchain.EthereumInternalTransactionType `json:"type" ts_doc:"Type of internal transfer (CALL, CREATE, etc.)."`
	From  string                                 `json:"from" ts_doc:"Address from which the transfer originated."`
	To    string                                 `json:"to" ts_doc:"Address to which the transfer was sent."`
	Value *Amount                                `json:"value" ts_doc:"Value transferred internally (in Wei or base units)."`
}

// EthereumSpecific contains ethereum-specific transaction data
type EthereumSpecific struct {
	Type                 bchain.EthereumInternalTransactionType `json:"type,omitempty" ts_doc:"High-level type of the Ethereum tx (e.g., 'call', 'create')."`
	CreatedContract      string                                 `json:"createdContract,omitempty" ts_doc:"Address of contract created by this transaction, if any."`
	Status               eth.TxStatus                           `json:"status" ts_doc:"Execution status of the transaction (1: success, 0: fail, -1: pending)."`
	Error                string                                 `json:"error,omitempty" ts_doc:"Error encountered during execution, if any."`
	Nonce                uint64                                 `json:"nonce" ts_doc:"Transaction nonce (sequential number from the sender)."`
	GasLimit             *big.Int                               `json:"gasLimit" ts_doc:"Maximum gas allowed by the sender for this transaction."`
	GasUsed              *big.Int                               `json:"gasUsed,omitempty" ts_doc:"Actual gas consumed by the transaction execution."`
	GasPrice             *Amount                                `json:"gasPrice,omitempty" ts_doc:"Price (in Wei or base units) per gas unit."`
	MaxPriorityFeePerGas *Amount                                `json:"maxPriorityFeePerGas,omitempty"`
	MaxFeePerGas         *Amount                                `json:"maxFeePerGas,omitempty"`
	BaseFeePerGas        *Amount                                `json:"baseFeePerGas,omitempty"`
	L1Fee                *big.Int                               `json:"l1Fee,omitempty" ts_doc:"Fee used for L1 part in rollups (e.g. Optimism)."`
	L1FeeScalar          string                                 `json:"l1FeeScalar,omitempty" ts_doc:"Scaling factor for L1 fees in certain Layer 2 solutions."`
	L1GasPrice           *Amount                                `json:"l1GasPrice,omitempty" ts_doc:"Gas price for L1 component, if applicable."`
	L1GasUsed            *big.Int                               `json:"l1GasUsed,omitempty" ts_doc:"Amount of gas used in L1 for this tx, if applicable."`
	Data                 string                                 `json:"data,omitempty" ts_doc:"Hex-encoded input data for the transaction."`
	ParsedData           *bchain.EthereumParsedInputData        `json:"parsedData,omitempty" ts_doc:"Decoded transaction data (function name, params, etc.)."`
	InternalTransfers    []EthereumInternalTransfer             `json:"internalTransfers,omitempty" ts_doc:"List of internal (sub-call) transfers."`
}

// AddressAlias holds a specialized alias for an address
type AddressAlias struct {
	Type  string `ts_doc:"Type of alias, e.g., user-defined name or contract name."`
	Alias string `ts_doc:"Alias string for the address."`
}

// AddressAliasesMap is a map of address strings to their alias definitions
type AddressAliasesMap map[string]AddressAlias

// Tx holds information about a transaction
type Tx struct {
	Txid                   string            `json:"txid" ts_doc:"Transaction ID (hash)."`
	Version                int32             `json:"version,omitempty" ts_doc:"Version of the transaction (if applicable)."`
	Locktime               uint32            `json:"lockTime,omitempty" ts_doc:"Locktime indicating earliest time/height transaction can be mined."`
	Vin                    []Vin             `json:"vin" ts_doc:"Array of inputs for this transaction."`
	Vout                   []Vout            `json:"vout" ts_doc:"Array of outputs for this transaction."`
	Blockhash              string            `json:"blockHash,omitempty" ts_doc:"Hash of the block containing this transaction."`
	Blockheight            int               `json:"blockHeight" ts_doc:"Block height in which this transaction was included."`
	Confirmations          uint32            `json:"confirmations" ts_doc:"Number of confirmations (blocks mined after this tx's block)."`
	ConfirmationETABlocks  uint32            `json:"confirmationETABlocks,omitempty" ts_doc:"Estimated blocks remaining until confirmation (if unconfirmed)."`
	ConfirmationETASeconds int64             `json:"confirmationETASeconds,omitempty" ts_doc:"Estimated seconds remaining until confirmation (if unconfirmed)."`
	Blocktime              int64             `json:"blockTime" ts_doc:"Unix timestamp of the block in which this transaction was included. 0 if unconfirmed."`
	Size                   int               `json:"size,omitempty" ts_doc:"Transaction size in bytes."`
	VSize                  int               `json:"vsize,omitempty" ts_doc:"Virtual size in bytes, for SegWit-enabled chains."`
	ValueOutSat            *Amount           `json:"value" ts_doc:"Total value of all outputs (in satoshi or base units)."`
	ValueInSat             *Amount           `json:"valueIn,omitempty" ts_doc:"Total value of all inputs (in satoshi or base units)."`
	FeesSat                *Amount           `json:"fees,omitempty" ts_doc:"Transaction fee (inputs - outputs)."`
	Hex                    string            `json:"hex,omitempty" ts_doc:"Raw hex-encoded transaction data."`
	Rbf                    bool              `json:"rbf,omitempty" ts_doc:"Indicates if this transaction is replace-by-fee (RBF) enabled."`
	CoinSpecificData       json.RawMessage   `json:"coinSpecificData,omitempty" ts_type:"any" ts_doc:"Blockchain-specific extended data."`
	TokenTransfers         []TokenTransfer   `json:"tokenTransfers,omitempty" ts_doc:"List of token transfers that occurred in this transaction."`
	EthereumSpecific       *EthereumSpecific `json:"ethereumSpecific,omitempty" ts_doc:"Ethereum-like blockchain specific data (if applicable)."`
	AddressAliases         AddressAliasesMap `json:"addressAliases,omitempty" ts_doc:"Aliases for addresses involved in this transaction."`
}

// FeeStats contains detailed block fee statistics
type FeeStats struct {
	TxCount         int       `json:"txCount" ts_doc:"Number of transactions in the given block."`
	TotalFeesSat    *Amount   `json:"totalFeesSat" ts_doc:"Sum of all fees in satoshi or base units."`
	AverageFeePerKb int64     `json:"averageFeePerKb" ts_doc:"Average fee per kilobyte in satoshi or base units."`
	DecilesFeePerKb [11]int64 `json:"decilesFeePerKb" ts_doc:"Fee distribution deciles (0%..100%) in satoshi or base units per kB."`
}

// Paging contains information about paging for address, blocks and block
type Paging struct {
	Page        int `json:"page,omitempty" ts_doc:"Current page index."`
	TotalPages  int `json:"totalPages,omitempty" ts_doc:"Total number of pages available."`
	ItemsOnPage int `json:"itemsOnPage,omitempty" ts_doc:"Number of items returned on this page."`
}

// TokensToReturn specifies what tokens are returned by GetAddress and GetXpubAddress
type TokensToReturn int

const (
	// AddressFilterVoutOff disables filtering of transactions by vout
	AddressFilterVoutOff = -1
	// AddressFilterVoutInputs specifies that only txs where the address is as input are returned
	AddressFilterVoutInputs = -2
	// AddressFilterVoutOutputs specifies that only txs where the address is as output are returned
	AddressFilterVoutOutputs = -3
	// AddressFilterVoutQueryNotNecessary signals that query for transactions is not necessary as there are no transactions for specified contract filter
	AddressFilterVoutQueryNotNecessary = -4

	// TokensToReturnNonzeroBalance - return only tokens with nonzero balance
	TokensToReturnNonzeroBalance TokensToReturn = 0
	// TokensToReturnUsed - return tokens with some transfers (even if they have zero balance now)
	TokensToReturnUsed TokensToReturn = 1
	// TokensToReturnDerived - return all derived tokens
	TokensToReturnDerived TokensToReturn = 2
)

// AddressFilter is used to filter data returned from GetAddress api method
type AddressFilter struct {
	Vout           int            `ts_doc:"Specifies which output index we are interested in filtering (or use the special constants)."`
	Contract       string         `ts_doc:"Contract address to filter by, if applicable."`
	FromHeight     uint32         `ts_doc:"Starting block height for filtering transactions."`
	ToHeight       uint32         `ts_doc:"Ending block height for filtering transactions."`
	TokensToReturn TokensToReturn `ts_doc:"Which tokens to include in the result set."`
	// OnlyConfirmed set to true will ignore mempool transactions; mempool is also ignored if FromHeight/ToHeight filter is specified
	OnlyConfirmed bool `ts_doc:"If true, ignores mempool (unconfirmed) transactions."`
}

// StakingPool holds data about address participation in a staking pool contract
type StakingPool struct {
	Contract                string  `json:"contract" ts_doc:"Staking pool contract address on-chain."`
	Name                    string  `json:"name" ts_doc:"Name of the staking pool contract."`
	PendingBalance          *Amount `json:"pendingBalance" ts_doc:"Balance pending deposit or withdrawal, if any."`
	PendingDepositedBalance *Amount `json:"pendingDepositedBalance" ts_doc:"Any pending deposit that is not yet finalized."`
	DepositedBalance        *Amount `json:"depositedBalance" ts_doc:"Currently deposited/staked balance."`
	WithdrawTotalAmount     *Amount `json:"withdrawTotalAmount" ts_doc:"Total amount withdrawn from this pool by the address."`
	ClaimableAmount         *Amount `json:"claimableAmount" ts_doc:"Rewards or principal currently claimable by the address."`
	RestakedReward          *Amount `json:"restakedReward" ts_doc:"Total rewards that have been restaked automatically."`
	AutocompoundBalance     *Amount `json:"autocompoundBalance" ts_doc:"Any balance automatically reinvested into the pool."`
}

// Address holds information about an address and its transactions
type Address struct {
	Paging
	AddrStr               string               `json:"address" ts_doc:"The address string in standard format."`
	BalanceSat            *Amount              `json:"balance" ts_doc:"Current confirmed balance (in satoshi or base units)."`
	TotalReceivedSat      *Amount              `json:"totalReceived,omitempty" ts_doc:"Total amount ever received by this address."`
	TotalSentSat          *Amount              `json:"totalSent,omitempty" ts_doc:"Total amount ever sent by this address."`
	UnconfirmedBalanceSat *Amount              `json:"unconfirmedBalance" ts_doc:"Unconfirmed balance for this address."`
	UnconfirmedTxs        int                  `json:"unconfirmedTxs" ts_doc:"Number of unconfirmed transactions for this address."`
	UnconfirmedSending    *Amount              `json:"unconfirmedSending,omitempty" ts_doc:"Unconfirmed outgoing balance for this address."`
	UnconfirmedReceiving  *Amount              `json:"unconfirmedReceiving,omitempty" ts_doc:"Unconfirmed incoming balance for this address."`
	Txs                   int                  `json:"txs" ts_doc:"Number of transactions for this address (including confirmed)."`
	AddrTxCount           int                  `json:"addrTxCount,omitempty" ts_doc:"Historical total count of transactions, if known."`
	NonTokenTxs           int                  `json:"nonTokenTxs,omitempty" ts_doc:"Number of transactions not involving tokens (pure coin transfers)."`
	InternalTxs           int                  `json:"internalTxs,omitempty" ts_doc:"Number of internal transactions (e.g., Ethereum calls)."`
	Transactions          []*Tx                `json:"transactions,omitempty" ts_doc:"List of transaction details (if requested)."`
	Txids                 []string             `json:"txids,omitempty" ts_doc:"List of transaction IDs (if detailed data is not requested)."`
	Nonce                 string               `json:"nonce,omitempty" ts_doc:"Current transaction nonce for Ethereum-like addresses."`
	UsedTokens            int                  `json:"usedTokens,omitempty" ts_doc:"Number of tokens with any historical usage at this address."`
	Tokens                Tokens               `json:"tokens,omitempty" ts_doc:"List of tokens associated with this address."`
	SecondaryValue        float64              `json:"secondaryValue,omitempty" ts_doc:"Total value of the address in secondary currency (e.g. fiat)."`
	TokensBaseValue       float64              `json:"tokensBaseValue,omitempty" ts_doc:"Sum of token values in base currency."`
	TokensSecondaryValue  float64              `json:"tokensSecondaryValue,omitempty" ts_doc:"Sum of token values in secondary currency (fiat)."`
	TotalBaseValue        float64              `json:"totalBaseValue,omitempty" ts_doc:"Address's entire value in base currency, including tokens."`
	TotalSecondaryValue   float64              `json:"totalSecondaryValue,omitempty" ts_doc:"Address's entire value in secondary currency, including tokens."`
	ContractInfo          *bchain.ContractInfo `json:"contractInfo,omitempty" ts_doc:"Extra info if the address is a contract (ABI, type)."`
	// Deprecated: replaced by ContractInfo
	Erc20Contract  *bchain.ContractInfo `json:"erc20Contract,omitempty" ts_doc:"@deprecated: replaced by contractInfo"`
	AddressAliases AddressAliasesMap    `json:"addressAliases,omitempty" ts_doc:"Aliases assigned to this address."`
	StakingPools   []StakingPool        `json:"stakingPools,omitempty" ts_doc:"List of staking pool data if address interacts with staking."`
	// helpers for explorer
	Filter        string              `json:"-" ts_doc:"Filter used internally for data retrieval."`
	XPubAddresses map[string]struct{} `json:"-" ts_doc:"Set of derived XPUB addresses (internal usage)."`
}

// Utxo is one unspent transaction output
type Utxo struct {
	Txid          string  `json:"txid" ts_doc:"Transaction ID in which this UTXO was created."`
	Vout          int32   `json:"vout" ts_doc:"Index of the output in that transaction."`
	AmountSat     *Amount `json:"value" ts_doc:"Value of this UTXO (in satoshi or base units)."`
	Height        int     `json:"height,omitempty" ts_doc:"Block height in which the UTXO was confirmed."`
	Confirmations int     `json:"confirmations" ts_doc:"Number of confirmations for this UTXO."`
	Address       string  `json:"address,omitempty" ts_doc:"Address to which this UTXO belongs."`
	Path          string  `json:"path,omitempty" ts_doc:"Derivation path for XPUB-based wallets, if applicable."`
	Locktime      uint32  `json:"lockTime,omitempty" ts_doc:"If non-zero, locktime required before spending this UTXO."`
	Coinbase      bool    `json:"coinbase,omitempty" ts_doc:"Indicates if this UTXO originated from a coinbase transaction."`
}

// Utxos is array of Utxo
type Utxos []Utxo

func (a Utxos) Len() int      { return len(a) }
func (a Utxos) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a Utxos) Less(i, j int) bool {
	// sort in reverse order, unconfirmed (height==0) utxos on top
	hi := a[i].Height
	hj := a[j].Height
	if hi == 0 {
		hi = maxInt
	}
	if hj == 0 {
		hj = maxInt
	}
	return hi >= hj
}

// BalanceHistory contains info about one point in time of balance history
type BalanceHistory struct {
	Time          uint32             `json:"time" ts_doc:"Unix timestamp for this point in the balance history."`
	Txs           uint32             `json:"txs" ts_doc:"Number of transactions in this interval."`
	ReceivedSat   *Amount            `json:"received" ts_doc:"Amount received in this interval (in satoshi or base units)."`
	SentSat       *Amount            `json:"sent" ts_doc:"Amount sent in this interval (in satoshi or base units)."`
	SentToSelfSat *Amount            `json:"sentToSelf" ts_doc:"Amount sent to the same address (self-transfer)."`
	FiatRates     map[string]float32 `json:"rates,omitempty" ts_doc:"Exchange rates at this point in time, if available."`
	Txid          string             `json:"txid,omitempty" ts_doc:"Transaction ID if the time corresponds to a specific tx."`
}

// BalanceHistories is array of BalanceHistory
type BalanceHistories []BalanceHistory

func (a BalanceHistories) Len() int      { return len(a) }
func (a BalanceHistories) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a BalanceHistories) Less(i, j int) bool {
	ti := a[i].Time
	tj := a[j].Time
	if ti == tj {
		return a[i].Txid < a[j].Txid
	}
	return ti < tj
}

// SortAndAggregate sums BalanceHistories to groups defined by parameter groupByTime
func (a BalanceHistories) SortAndAggregate(groupByTime uint32) BalanceHistories {
	bhs := make(BalanceHistories, 0)
	if len(a) > 0 {
		bha := BalanceHistory{
			ReceivedSat:   &Amount{},
			SentSat:       &Amount{},
			SentToSelfSat: &Amount{},
		}
		sort.Sort(a)
		for i := range a {
			bh := &a[i]
			time := bh.Time - bh.Time%groupByTime
			if bha.Time != time {
				if bha.Time != 0 {
					// in aggregate, do not return txid as it could multiple of them
					bha.Txid = ""
					bhs = append(bhs, bha)
				}
				bha = BalanceHistory{
					Time:          time,
					ReceivedSat:   &Amount{},
					SentSat:       &Amount{},
					SentToSelfSat: &Amount{},
				}
			}
			if bha.Txid != bh.Txid {
				bha.Txs += bh.Txs
				bha.Txid = bh.Txid
			}
			(*big.Int)(bha.ReceivedSat).Add((*big.Int)(bha.ReceivedSat), (*big.Int)(bh.ReceivedSat))
			(*big.Int)(bha.SentSat).Add((*big.Int)(bha.SentSat), (*big.Int)(bh.SentSat))
			(*big.Int)(bha.SentToSelfSat).Add((*big.Int)(bha.SentToSelfSat), (*big.Int)(bh.SentToSelfSat))
		}
		if bha.Txs > 0 {
			bha.Txid = ""
			bhs = append(bhs, bha)
		}
	}
	return bhs
}

// Blocks is list of blocks with paging information
type Blocks struct {
	Paging
	Blocks []db.BlockInfo `json:"blocks" ts_doc:"List of blocks."`
}

// BlockInfo contains extended block header data and a list of block txids
type BlockInfo struct {
	Hash          string            `json:"hash" ts_doc:"Block hash."`
	Prev          string            `json:"previousBlockHash,omitempty" ts_doc:"Hash of the previous block in the chain."`
	Next          string            `json:"nextBlockHash,omitempty" ts_doc:"Hash of the next block, if known."`
	Height        uint32            `json:"height" ts_doc:"Block height (0-based index in the chain)."`
	Confirmations int               `json:"confirmations" ts_doc:"Number of confirmations of this block (distance from best chain tip)."`
	Size          int               `json:"size" ts_doc:"Size of the block in bytes."`
	Time          int64             `json:"time,omitempty" ts_doc:"Timestamp of when this block was mined."`
	Version       common.JSONNumber `json:"version" ts_doc:"Block version (chain-specific meaning)."`
	MerkleRoot    string            `json:"merkleRoot" ts_doc:"Merkle root of the block's transactions."`
	Nonce         string            `json:"nonce" ts_doc:"Nonce used in the mining process."`
	Bits          string            `json:"bits" ts_doc:"Compact representation of the target threshold."`
	Difficulty    string            `json:"difficulty" ts_doc:"Difficulty target for mining this block."`
	Txids         []string          `json:"tx,omitempty" ts_doc:"List of transaction IDs included in this block."`
}

// Block contains information about block
type Block struct {
	Paging
	BlockInfo
	TxCount        int               `json:"txCount" ts_doc:"Total count of transactions in this block."`
	Transactions   []*Tx             `json:"txs,omitempty" ts_doc:"List of full transaction details (if requested)."`
	AddressAliases AddressAliasesMap `json:"addressAliases,omitempty" ts_doc:"Optional aliases for addresses found in this block."`
}

// BlockRaw contains raw block in hex
type BlockRaw struct {
	Hex string `json:"hex" ts_doc:"Hex-encoded block data."`
}

// BlockbookInfo contains information about the running blockbook instance
type BlockbookInfo struct {
	Coin                         string                       `json:"coin" ts_doc:"Coin name, e.g. 'Bitcoin'."`
	Network                      string                       `json:"network" ts_doc:"Network shortcut, e.g. 'BTC'."`
	Host                         string                       `json:"host" ts_doc:"Hostname of the blockbook instance, e.g. 'backend5'."`
	Version                      string                       `json:"version" ts_doc:"Running blockbook version, e.g. '0.4.0'."`
	GitCommit                    string                       `json:"gitCommit" ts_doc:"Git commit hash of the running blockbook, e.g. 'a0960c8e'."`
	BuildTime                    string                       `json:"buildTime" ts_doc:"Build time of running blockbook, e.g. '2024-08-08T12:32:50+00:00'."`
	SyncMode                     bool                         `json:"syncMode" ts_doc:"If true, blockbook is syncing from scratch or in a special sync mode."`
	InitialSync                  bool                         `json:"initialSync" ts_doc:"Indicates if blockbook is in its initial sync phase."`
	InSync                       bool                         `json:"inSync" ts_doc:"Indicates if the backend is fully synced with the blockchain."`
	BestHeight                   uint32                       `json:"bestHeight" ts_doc:"Best (latest) block height according to this instance."`
	LastBlockTime                time.Time                    `json:"lastBlockTime" ts_doc:"Timestamp of the latest block in the chain."`
	InSyncMempool                bool                         `json:"inSyncMempool" ts_doc:"Indicates if mempool info is synced as well."`
	LastMempoolTime              time.Time                    `json:"lastMempoolTime" ts_doc:"Timestamp of the last mempool update."`
	MempoolSize                  int                          `json:"mempoolSize" ts_doc:"Number of unconfirmed transactions in the mempool."`
	Decimals                     int                          `json:"decimals" ts_doc:"Number of decimals for this coin's base unit."`
	DbSize                       int64                        `json:"dbSize" ts_doc:"Size of the underlying database in bytes."`
	HasFiatRates                 bool                         `json:"hasFiatRates,omitempty" ts_doc:"Whether this instance provides fiat exchange rates."`
	HasTokenFiatRates            bool                         `json:"hasTokenFiatRates,omitempty" ts_doc:"Whether this instance provides fiat exchange rates for tokens."`
	CurrentFiatRatesTime         *time.Time                   `json:"currentFiatRatesTime,omitempty" ts_doc:"Timestamp of the latest fiat rates update."`
	HistoricalFiatRatesTime      *time.Time                   `json:"historicalFiatRatesTime,omitempty" ts_doc:"Timestamp of the latest historical fiat rates update."`
	HistoricalTokenFiatRatesTime *time.Time                   `json:"historicalTokenFiatRatesTime,omitempty" ts_doc:"Timestamp of the latest historical token fiat rates update."`
	SupportedStakingPools        []string                     `json:"supportedStakingPools,omitempty" ts_doc:"List of contract addresses supported for staking."`
	DbSizeFromColumns            int64                        `json:"dbSizeFromColumns,omitempty" ts_doc:"Optional calculated DB size from columns."`
	DbColumns                    []common.InternalStateColumn `json:"dbColumns,omitempty" ts_doc:"List of columns/tables in the DB for internal state."`
	About                        string                       `json:"about" ts_doc:"Additional human-readable info about this blockbook instance."`
}

// SystemInfo contains information about the running blockbook and backend instance
type SystemInfo struct {
	Blockbook *BlockbookInfo      `json:"blockbook" ts_doc:"Blockbook instance information."`
	Backend   *common.BackendInfo `json:"backend" ts_doc:"Information about the connected backend node."`
}

// MempoolTxid contains information about a transaction in mempool
type MempoolTxid struct {
	Time int64  `json:"time" ts_doc:"Timestamp when the transaction was received in the mempool."`
	Txid string `json:"txid" ts_doc:"Transaction hash for this mempool entry."`
}

// MempoolTxids contains a list of mempool txids with paging information
type MempoolTxids struct {
	Paging
	Mempool     []MempoolTxid `json:"mempool" ts_doc:"List of transactions currently in the mempool."`
	MempoolSize int           `json:"mempoolSize" ts_doc:"Number of unconfirmed transactions in the mempool."`
}

// FiatTicker contains formatted CurrencyRatesTicker data
type FiatTicker struct {
	Timestamp int64              `json:"ts,omitempty" ts_doc:"Unix timestamp for these fiat rates."`
	Rates     map[string]float32 `json:"rates" ts_doc:"Map of currency codes to their exchange rate."`
	Error     string             `json:"error,omitempty" ts_doc:"Any error message encountered while fetching rates."`
}

// FiatTickers contains a formatted CurrencyRatesTicker list
type FiatTickers struct {
	Tickers []FiatTicker `json:"tickers" ts_doc:"List of fiat tickers with timestamps and rates."`
}

// AvailableVsCurrencies contains formatted data about available versus currencies for exchange rates
type AvailableVsCurrencies struct {
	Timestamp int64    `json:"ts,omitempty" ts_doc:"Timestamp for the available currency list."`
	Tickers   []string `json:"available_currencies" ts_doc:"List of currency codes (e.g., USD, EUR) supported by the rates."`
	Error     string   `json:"error,omitempty" ts_doc:"Error message, if any, when fetching the available currencies."`
}

// Eip1559Fee
type Eip1559Fee struct {
	MaxFeePerGas         *Amount `json:"maxFeePerGas"`
	MaxPriorityFeePerGas *Amount `json:"maxPriorityFeePerGas"`
	MinWaitTimeEstimate  int     `json:"minWaitTimeEstimate,omitempty"`
	MaxWaitTimeEstimate  int     `json:"maxWaitTimeEstimate,omitempty"`
}

// Eip1559Fees
type Eip1559Fees struct {
	BaseFeePerGas              *Amount     `json:"baseFeePerGas,omitempty"`
	Low                        *Eip1559Fee `json:"low,omitempty"`
	Medium                     *Eip1559Fee `json:"medium,omitempty"`
	High                       *Eip1559Fee `json:"high,omitempty"`
	Instant                    *Eip1559Fee `json:"instant,omitempty"`
	NetworkCongestion          float64     `json:"networkCongestion,omitempty"`
	LatestPriorityFeeRange     []*Amount   `json:"latestPriorityFeeRange,omitempty"`
	HistoricalPriorityFeeRange []*Amount   `json:"historicalPriorityFeeRange,omitempty"`
	HistoricalBaseFeeRange     []*Amount   `json:"historicalBaseFeeRange,omitempty"`
	PriorityFeeTrend           string      `json:"priorityFeeTrend,omitempty" ts_type:"'up' | 'down'"`
	BaseFeeTrend               string      `json:"baseFeeTrend,omitempty" ts_type:"'up' | 'down'"`
}

type LongTermFeeRate struct {
	FeePerUnit string `json:"feePerUnit" ts_doc:"Long term fee rate (in sat/kByte)."`
	Blocks     uint64 `json:"blocks" ts_doc:"Amount of blocks used for the long term fee rate estimation."`
}
