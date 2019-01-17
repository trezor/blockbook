package api

import (
	"blockbook/bchain"
	"blockbook/common"
	"blockbook/db"
	"encoding/json"
	"math/big"
	"time"
)

const maxInt = int(^uint(0) >> 1)

// GetAddressOption specifies what data returns GetAddress api call
type GetAddressOption int

const (
	// Basic - only that address is indexed and some basic info
	Basic GetAddressOption = iota
	// Balance - only balances
	Balance
	// TxidHistory - balances and txids, subject to paging
	TxidHistory
	// TxHistoryLight - balances and easily obtained tx data (not requiring request to backend), subject to paging
	TxHistoryLight
	// TxHistory - balances and full tx data, subject to paging
	TxHistory
)

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
	Txid       string                   `json:"txid,omitempty"`
	Vout       uint32                   `json:"vout,omitempty"`
	Sequence   int64                    `json:"sequence,omitempty"`
	N          int                      `json:"n"`
	AddrDesc   bchain.AddressDescriptor `json:"-"`
	Addresses  []string                 `json:"addresses,omitempty"`
	Searchable bool                     `json:"-"`
	ValueSat   *Amount                  `json:"value,omitempty"`
	Hex        string                   `json:"hex,omitempty"`
	Asm        string                   `json:"asm,omitempty"`
	Coinbase   string                   `json:"coinbase,omitempty"`
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
	Searchable  bool                     `json:"-"`
	Type        string                   `json:"type,omitempty"`
}

// TokenType specifies type of token
type TokenType string

// ERC20TokenType is Ethereum ERC20 token
const ERC20TokenType TokenType = "ERC20"

// Token contains info about tokens held by an address
type Token struct {
	Type          TokenType `json:"type"`
	Contract      string    `json:"contract"`
	Transfers     int       `json:"transfers"`
	Name          string    `json:"name"`
	Symbol        string    `json:"symbol"`
	Decimals      int       `json:"decimals"`
	BalanceSat    *Amount   `json:"balance,omitempty"`
	ContractIndex string    `json:"-"`
}

// TokenTransfer contains info about a token transfer done in a transaction
type TokenTransfer struct {
	Type     TokenType `json:"type"`
	From     string    `json:"from"`
	To       string    `json:"to"`
	Token    string    `json:"token"`
	Name     string    `json:"name"`
	Symbol   string    `json:"symbol"`
	Decimals int       `json:"decimals"`
	Value    *Amount   `json:"value"`
}

// EthereumSpecific contains ethereum specific transaction data
type EthereumSpecific struct {
	Status   int      `json:"status"` // 1 OK, 0 Fail, -1 pending
	Nonce    uint64   `json:"nonce"`
	GasLimit *big.Int `json:"gaslimit"`
	GasUsed  *big.Int `json:"gasused"`
	GasPrice *Amount  `json:"gasprice"`
}

// Tx holds information about a transaction
type Tx struct {
	Txid             string            `json:"txid"`
	Version          int32             `json:"version,omitempty"`
	Locktime         uint32            `json:"locktime,omitempty"`
	Vin              []Vin             `json:"vin"`
	Vout             []Vout            `json:"vout"`
	Blockhash        string            `json:"blockhash,omitempty"`
	Blockheight      int               `json:"blockheight"`
	Confirmations    uint32            `json:"confirmations"`
	Blocktime        int64             `json:"blocktime"`
	Size             int               `json:"size,omitempty"`
	ValueOutSat      *Amount           `json:"value"`
	ValueInSat       *Amount           `json:"valueIn,omitempty"`
	FeesSat          *Amount           `json:"fees,omitempty"`
	Hex              string            `json:"hex,omitempty"`
	CoinSpecificData interface{}       `json:"-"`
	CoinSpecificJSON json.RawMessage   `json:"-"`
	TokenTransfers   []TokenTransfer   `json:"tokentransfers,omitempty"`
	EthereumSpecific *EthereumSpecific `json:"ethereumspecific,omitempty"`
}

// Paging contains information about paging for address, blocks and block
type Paging struct {
	Page        int `json:"page,omitempty"`
	TotalPages  int `json:"totalPages,omitempty"`
	ItemsOnPage int `json:"itemsOnPage,omitempty"`
}

const (
	// AddressFilterVoutOff disables filtering of transactions by vout
	AddressFilterVoutOff = -1
	// AddressFilterVoutInputs specifies that only txs where the address is as input are returned
	AddressFilterVoutInputs = -2
	// AddressFilterVoutOutputs specifies that only txs where the address is as output are returned
	AddressFilterVoutOutputs = -3
)

// AddressFilter is used to filter data returned from GetAddress api method
type AddressFilter struct {
	Vout       int
	Contract   string
	FromHeight uint32
	ToHeight   uint32
}

// Address holds information about address and its transactions
type Address struct {
	Paging
	AddrStr               string                `json:"address"`
	BalanceSat            *Amount               `json:"balance"`
	TotalReceivedSat      *Amount               `json:"totalReceived,omitempty"`
	TotalSentSat          *Amount               `json:"totalSent,omitempty"`
	UnconfirmedBalanceSat *Amount               `json:"unconfirmedBalance"`
	UnconfirmedTxs        int                   `json:"unconfirmedTxs"`
	Txs                   int                   `json:"txs"`
	NonTokenTxs           int                   `json:"nontokenTxs,omitempty"`
	Transactions          []*Tx                 `json:"transactions,omitempty"`
	Txids                 []string              `json:"txids,omitempty"`
	Nonce                 string                `json:"nonce,omitempty"`
	Tokens                []Token               `json:"tokens,omitempty"`
	Erc20Contract         *bchain.Erc20Contract `json:"erc20contract,omitempty"`
	Filter                string                `json:"-"`
}

// AddressUtxo holds information about address and its transactions
type AddressUtxo struct {
	Txid          string  `json:"txid"`
	Vout          int32   `json:"vout"`
	AmountSat     *Amount `json:"value"`
	Height        int     `json:"height,omitempty"`
	Confirmations int     `json:"confirmations"`
}

// Blocks is list of blocks with paging information
type Blocks struct {
	Paging
	Blocks []db.BlockInfo `json:"blocks"`
}

// Block contains information about block
type Block struct {
	Paging
	bchain.BlockInfo
	TxCount      int   `json:"TxCount"`
	Transactions []*Tx `json:"txs,omitempty"`
}

// BlockbookInfo contains information about the running blockbook instance
type BlockbookInfo struct {
	Coin              string                       `json:"coin"`
	Host              string                       `json:"host"`
	Version           string                       `json:"version"`
	GitCommit         string                       `json:"gitcommit"`
	BuildTime         string                       `json:"buildtime"`
	SyncMode          bool                         `json:"syncMode"`
	InitialSync       bool                         `json:"initialsync"`
	InSync            bool                         `json:"inSync"`
	BestHeight        uint32                       `json:"bestHeight"`
	LastBlockTime     time.Time                    `json:"lastBlockTime"`
	InSyncMempool     bool                         `json:"inSyncMempool"`
	LastMempoolTime   time.Time                    `json:"lastMempoolTime"`
	MempoolSize       int                          `json:"mempoolSize"`
	Decimals          int                          `json:"decimals"`
	DbSize            int64                        `json:"dbSize"`
	DbSizeFromColumns int64                        `json:"dbSizeFromColumns,omitempty"`
	DbColumns         []common.InternalStateColumn `json:"dbColumns,omitempty"`
	About             string                       `json:"about"`
}

// SystemInfo contains information about the running blockbook and backend instance
type SystemInfo struct {
	Blockbook *BlockbookInfo    `json:"blockbook"`
	Backend   *bchain.ChainInfo `json:"backend"`
}
