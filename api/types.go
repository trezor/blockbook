package api

import (
	"encoding/json"
	"errors"
	"math/big"
	"sort"
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
	Text   string
	Public bool
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

// Amount is datatype holding amounts
type Amount big.Int

// IsZeroBigInt if big int has zero value
func IsZeroBigInt(b *big.Int) bool {
	return len(b.Bits()) == 0
}

// Compare returns an integer comparing two Amounts. The result will be 0 if a == b, -1 if a < b, and +1 if a > b.
// Nil Amount is always less then non nil amount, two nil Amounts are equal
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
	Txid      string                   `json:"txid,omitempty"`
	Vout      uint32                   `json:"vout,omitempty"`
	Sequence  int64                    `json:"sequence,omitempty"`
	N         int                      `json:"n"`
	AddrDesc  bchain.AddressDescriptor `json:"-"`
	Addresses []string                 `json:"addresses,omitempty"`
	IsAddress bool                     `json:"isAddress"`
	IsOwn     bool                     `json:"isOwn,omitempty"`
	ValueSat  *Amount                  `json:"value,omitempty"`
	Hex       string                   `json:"hex,omitempty"`
	Asm       string                   `json:"asm,omitempty"`
	Coinbase  string                   `json:"coinbase,omitempty"`
}

// Vout contains information about single transaction output
type Vout struct {
	ValueSat    *Amount                  `json:"value,omitempty"`
	N           int                      `json:"n"`
	Spent       bool                     `json:"spent,omitempty"`
	SpentTxID   string                   `json:"spentTxId,omitempty"`
	SpentIndex  int                      `json:"spentIndex,omitempty"`
	SpentHeight int                      `json:"spentHeight,omitempty"`
	Hex         string                   `json:"hex,omitempty"`
	Asm         string                   `json:"asm,omitempty"`
	AddrDesc    bchain.AddressDescriptor `json:"-"`
	Addresses   []string                 `json:"addresses"`
	IsAddress   bool                     `json:"isAddress"`
	IsOwn       bool                     `json:"isOwn,omitempty"`
	Type        string                   `json:"type,omitempty"`
}

// MultiTokenValue contains values for contract with id and value (like ERC1155)
type MultiTokenValue struct {
	Id    *Amount `json:"id,omitempty"`
	Value *Amount `json:"value,omitempty"`
}

// Token contains info about tokens held by an address
type Token struct {
	Type             bchain.TokenTypeName `json:"type" ts_type:"'XPUBAddress' | 'ERC20' | 'ERC721' | 'ERC1155'"`
	Name             string               `json:"name"`
	Path             string               `json:"path,omitempty"`
	Contract         string               `json:"contract,omitempty"`
	Transfers        int                  `json:"transfers"`
	Symbol           string               `json:"symbol,omitempty"`
	Decimals         int                  `json:"decimals,omitempty"`
	BalanceSat       *Amount              `json:"balance,omitempty"`
	BaseValue        float64              `json:"baseValue,omitempty"`        // value in the base currency (ETH for Ethereum)
	SecondaryValue   float64              `json:"secondaryValue,omitempty"`   // value in secondary (fiat) currency, if specified
	Ids              []Amount             `json:"ids,omitempty"`              // multiple ERC721 tokens
	MultiTokenValues []MultiTokenValue    `json:"multiTokenValues,omitempty"` // multiple ERC1155 tokens
	TotalReceivedSat *Amount              `json:"totalReceived,omitempty"`
	TotalSentSat     *Amount              `json:"totalSent,omitempty"`
	ContractIndex    string               `json:"-"`
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
	Type             bchain.TokenTypeName `json:"type"`
	From             string               `json:"from"`
	To               string               `json:"to"`
	Contract         string               `json:"contract"`
	Name             string               `json:"name"`
	Symbol           string               `json:"symbol"`
	Decimals         int                  `json:"decimals"`
	Value            *Amount              `json:"value,omitempty"`
	MultiTokenValues []MultiTokenValue    `json:"multiTokenValues,omitempty"`
}

type EthereumInternalTransfer struct {
	Type  bchain.EthereumInternalTransactionType `json:"type"`
	From  string                                 `json:"from"`
	To    string                                 `json:"to"`
	Value *Amount                                `json:"value"`
}

// EthereumSpecific contains ethereum specific transaction data
type EthereumSpecific struct {
	Type              bchain.EthereumInternalTransactionType `json:"type,omitempty"`
	CreatedContract   string                                 `json:"createdContract,omitempty"`
	Status            eth.TxStatus                           `json:"status"` // 1 OK, 0 Fail, -1 pending
	Error             string                                 `json:"error,omitempty"`
	Nonce             uint64                                 `json:"nonce"`
	GasLimit          *big.Int                               `json:"gasLimit"`
	GasUsed           *big.Int                               `json:"gasUsed,omitempty"`
	GasPrice          *Amount                                `json:"gasPrice"`
	Data              string                                 `json:"data,omitempty"`
	ParsedData        *bchain.EthereumParsedInputData        `json:"parsedData,omitempty"`
	InternalTransfers []EthereumInternalTransfer             `json:"internalTransfers,omitempty"`
}

type AddressAlias struct {
	Type  string
	Alias string
}
type AddressAliasesMap map[string]AddressAlias

// Tx holds information about a transaction
type Tx struct {
	Txid                   string            `json:"txid"`
	Version                int32             `json:"version,omitempty"`
	Locktime               uint32            `json:"lockTime,omitempty"`
	Vin                    []Vin             `json:"vin"`
	Vout                   []Vout            `json:"vout"`
	Blockhash              string            `json:"blockHash,omitempty"`
	Blockheight            int               `json:"blockHeight"`
	Confirmations          uint32            `json:"confirmations"`
	ConfirmationETABlocks  uint32            `json:"confirmationETABlocks,omitempty"`
	ConfirmationETASeconds int64             `json:"confirmationETASeconds,omitempty"`
	Blocktime              int64             `json:"blockTime"`
	Size                   int               `json:"size,omitempty"`
	VSize                  int               `json:"vsize,omitempty"`
	ValueOutSat            *Amount           `json:"value"`
	ValueInSat             *Amount           `json:"valueIn,omitempty"`
	FeesSat                *Amount           `json:"fees,omitempty"`
	Hex                    string            `json:"hex,omitempty"`
	Rbf                    bool              `json:"rbf,omitempty"`
	CoinSpecificData       json.RawMessage   `json:"coinSpecificData,omitempty" ts_type:"any"`
	TokenTransfers         []TokenTransfer   `json:"tokenTransfers,omitempty"`
	EthereumSpecific       *EthereumSpecific `json:"ethereumSpecific,omitempty"`
	AddressAliases         AddressAliasesMap `json:"addressAliases,omitempty"`
}

// FeeStats contains detailed block fee statistics
type FeeStats struct {
	TxCount         int       `json:"txCount"`
	TotalFeesSat    *Amount   `json:"totalFeesSat"`
	AverageFeePerKb int64     `json:"averageFeePerKb"`
	DecilesFeePerKb [11]int64 `json:"decilesFeePerKb"`
}

// Paging contains information about paging for address, blocks and block
type Paging struct {
	Page        int `json:"page,omitempty"`
	TotalPages  int `json:"totalPages,omitempty"`
	ItemsOnPage int `json:"itemsOnPage,omitempty"`
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
	Vout           int
	Contract       string
	FromHeight     uint32
	ToHeight       uint32
	TokensToReturn TokensToReturn
	// OnlyConfirmed set to true will ignore mempool transactions; mempool is also ignored if FromHeight/ToHeight filter is specified
	OnlyConfirmed bool
}

// Address holds information about address and its transactions
type Address struct {
	Paging
	AddrStr               string               `json:"address"`
	BalanceSat            *Amount              `json:"balance"`
	TotalReceivedSat      *Amount              `json:"totalReceived,omitempty"`
	TotalSentSat          *Amount              `json:"totalSent,omitempty"`
	UnconfirmedBalanceSat *Amount              `json:"unconfirmedBalance"`
	UnconfirmedTxs        int                  `json:"unconfirmedTxs"`
	Txs                   int                  `json:"txs"`
	AddrTxCount           int                  `json:"addrTxCount,omitempty"`
	NonTokenTxs           int                  `json:"nonTokenTxs,omitempty"`
	InternalTxs           int                  `json:"internalTxs,omitempty"`
	Transactions          []*Tx                `json:"transactions,omitempty"`
	Txids                 []string             `json:"txids,omitempty"`
	Nonce                 string               `json:"nonce,omitempty"`
	UsedTokens            int                  `json:"usedTokens,omitempty"`
	Tokens                Tokens               `json:"tokens,omitempty"`
	SecondaryValue        float64              `json:"secondaryValue,omitempty"` // address value in secondary currency
	TokensBaseValue       float64              `json:"tokensBaseValue,omitempty"`
	TokensSecondaryValue  float64              `json:"tokensSecondaryValue,omitempty"`
	TotalBaseValue        float64              `json:"totalBaseValue,omitempty"`      // value including tokens in base currency
	TotalSecondaryValue   float64              `json:"totalSecondaryValue,omitempty"` // value including tokens in secondary currency
	ContractInfo          *bchain.ContractInfo `json:"contractInfo,omitempty"`
	Erc20Contract         *bchain.ContractInfo `json:"erc20Contract,omitempty"` // deprecated
	AddressAliases        AddressAliasesMap    `json:"addressAliases,omitempty"`
	// helpers for explorer
	Filter        string              `json:"-"`
	XPubAddresses map[string]struct{} `json:"-"`
}

// Utxo is one unspent transaction output
type Utxo struct {
	Txid          string  `json:"txid"`
	Vout          int32   `json:"vout"`
	AmountSat     *Amount `json:"value"`
	Height        int     `json:"height,omitempty"`
	Confirmations int     `json:"confirmations"`
	Address       string  `json:"address,omitempty"`
	Path          string  `json:"path,omitempty"`
	Locktime      uint32  `json:"lockTime,omitempty"`
	Coinbase      bool    `json:"coinbase,omitempty"`
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
	Time          uint32             `json:"time"`
	Txs           uint32             `json:"txs"`
	ReceivedSat   *Amount            `json:"received"`
	SentSat       *Amount            `json:"sent"`
	SentToSelfSat *Amount            `json:"sentToSelf"`
	FiatRates     map[string]float32 `json:"rates,omitempty"`
	Txid          string             `json:"txid,omitempty"`
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
	Blocks []db.BlockInfo `json:"blocks"`
}

// BlockInfo contains extended block header data and a list of block txids
type BlockInfo struct {
	Hash          string            `json:"hash"`
	Prev          string            `json:"previousBlockHash,omitempty"`
	Next          string            `json:"nextBlockHash,omitempty"`
	Height        uint32            `json:"height"`
	Confirmations int               `json:"confirmations"`
	Size          int               `json:"size"`
	Time          int64             `json:"time,omitempty"`
	Version       common.JSONNumber `json:"version"`
	MerkleRoot    string            `json:"merkleRoot"`
	Nonce         string            `json:"nonce"`
	Bits          string            `json:"bits"`
	Difficulty    string            `json:"difficulty"`
	Txids         []string          `json:"tx,omitempty"`
}

// Block contains information about block
type Block struct {
	Paging
	BlockInfo
	TxCount        int               `json:"txCount"`
	Transactions   []*Tx             `json:"txs,omitempty"`
	AddressAliases AddressAliasesMap `json:"addressAliases,omitempty"`
}

// BlockRaw contains raw block in hex
type BlockRaw struct {
	Hex string `json:"hex"`
}

// BlockbookInfo contains information about the running blockbook instance
type BlockbookInfo struct {
	Coin                         string                       `json:"coin"`
	Host                         string                       `json:"host"`
	Version                      string                       `json:"version"`
	GitCommit                    string                       `json:"gitCommit"`
	BuildTime                    string                       `json:"buildTime"`
	SyncMode                     bool                         `json:"syncMode"`
	InitialSync                  bool                         `json:"initialSync"`
	InSync                       bool                         `json:"inSync"`
	BestHeight                   uint32                       `json:"bestHeight"`
	LastBlockTime                time.Time                    `json:"lastBlockTime"`
	InSyncMempool                bool                         `json:"inSyncMempool"`
	LastMempoolTime              time.Time                    `json:"lastMempoolTime"`
	MempoolSize                  int                          `json:"mempoolSize"`
	Decimals                     int                          `json:"decimals"`
	DbSize                       int64                        `json:"dbSize"`
	HasFiatRates                 bool                         `json:"hasFiatRates,omitempty"`
	HasTokenFiatRates            bool                         `json:"hasTokenFiatRates,omitempty"`
	CurrentFiatRatesTime         *time.Time                   `json:"currentFiatRatesTime,omitempty"`
	HistoricalFiatRatesTime      *time.Time                   `json:"historicalFiatRatesTime,omitempty"`
	HistoricalTokenFiatRatesTime *time.Time                   `json:"historicalTokenFiatRatesTime,omitempty"`
	DbSizeFromColumns            int64                        `json:"dbSizeFromColumns,omitempty"`
	DbColumns                    []common.InternalStateColumn `json:"dbColumns,omitempty"`
	About                        string                       `json:"about"`
}

// SystemInfo contains information about the running blockbook and backend instance
type SystemInfo struct {
	Blockbook *BlockbookInfo      `json:"blockbook"`
	Backend   *common.BackendInfo `json:"backend"`
}

// MempoolTxid contains information about a transaction in mempool
type MempoolTxid struct {
	Time int64  `json:"time"`
	Txid string `json:"txid"`
}

// MempoolTxids contains a list of mempool txids with paging information
type MempoolTxids struct {
	Paging
	Mempool     []MempoolTxid `json:"mempool"`
	MempoolSize int           `json:"mempoolSize"`
}

// FiatTicker contains formatted CurrencyRatesTicker data
type FiatTicker struct {
	Timestamp int64              `json:"ts,omitempty"`
	Rates     map[string]float32 `json:"rates"`
	Error     string             `json:"error,omitempty"`
}

// FiatTickers contains a formatted CurrencyRatesTicker list
type FiatTickers struct {
	Tickers []FiatTicker `json:"tickers"`
}

// AvailableVsCurrencies contains formatted data about available versus currencies for exchange rates
type AvailableVsCurrencies struct {
	Timestamp int64    `json:"ts,omitempty"`
	Tickers   []string `json:"available_currencies"`
	Error     string   `json:"error,omitempty"`
}
