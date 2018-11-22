package api

import (
	"blockbook/bchain"
	"blockbook/common"
	"blockbook/db"
	"math/big"
	"time"
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

// ScriptSig contains input script
type ScriptSig struct {
	Hex string `json:"hex"`
	Asm string `json:"asm,omitempty"`
}

// Vin contains information about single transaction input
type Vin struct {
	Txid       string                   `json:"txid"`
	Vout       uint32                   `json:"vout"`
	Sequence   int64                    `json:"sequence,omitempty"`
	N          int                      `json:"n"`
	ScriptSig  ScriptSig                `json:"scriptSig"`
	AddrDesc   bchain.AddressDescriptor `json:"-"`
	Addresses  []string                 `json:"addresses"`
	Searchable bool                     `json:"-"`
	Value      string                   `json:"value"`
	ValueSat   big.Int                  `json:"-"`
}

// ScriptPubKey contains output script and addresses derived from it
type ScriptPubKey struct {
	Hex        string                   `json:"hex"`
	Asm        string                   `json:"asm,omitempty"`
	AddrDesc   bchain.AddressDescriptor `json:"-"`
	Addresses  []string                 `json:"addresses"`
	Searchable bool                     `json:"-"`
	Type       string                   `json:"type,omitempty"`
}

// Vout contains information about single transaction output
type Vout struct {
	Value        string       `json:"value"`
	ValueSat     big.Int      `json:"-"`
	N            int          `json:"n"`
	ScriptPubKey ScriptPubKey `json:"scriptPubKey"`
	Spent        bool         `json:"spent"`
	SpentTxID    string       `json:"spentTxId,omitempty"`
	SpentIndex   int          `json:"spentIndex,omitempty"`
	SpentHeight  int          `json:"spentHeight,omitempty"`
}

// Tx holds information about a transaction
type Tx struct {
	Txid          string  `json:"txid"`
	Version       int32   `json:"version,omitempty"`
	Locktime      uint32  `json:"locktime,omitempty"`
	Vin           []Vin   `json:"vin"`
	Vout          []Vout  `json:"vout"`
	Blockhash     string  `json:"blockhash,omitempty"`
	Blockheight   int     `json:"blockheight"`
	Confirmations uint32  `json:"confirmations"`
	Time          int64   `json:"time,omitempty"`
	Blocktime     int64   `json:"blocktime"`
	ValueOut      string  `json:"valueOut"`
	ValueOutSat   big.Int `json:"-"`
	Size          int     `json:"size,omitempty"`
	ValueIn       string  `json:"valueIn"`
	ValueInSat    big.Int `json:"-"`
	Fees          string  `json:"fees"`
	FeesSat       big.Int `json:"-"`
	Hex           string  `json:"hex"`
}

// Paging contains information about paging for address, blocks and block
type Paging struct {
	Page        int `json:"page"`
	TotalPages  int `json:"totalPages"`
	ItemsOnPage int `json:"itemsOnPage"`
}

// Address holds information about address and its transactions
type Address struct {
	Paging
	AddrStr                 string   `json:"addrStr"`
	Balance                 string   `json:"balance"`
	TotalReceived           string   `json:"totalReceived"`
	TotalSent               string   `json:"totalSent"`
	UnconfirmedBalance      string   `json:"unconfirmedBalance"`
	UnconfirmedTxApperances int      `json:"unconfirmedTxApperances"`
	TxApperances            int      `json:"txApperances"`
	Transactions            []*Tx    `json:"txs,omitempty"`
	Txids                   []string `json:"transactions,omitempty"`
}

// AddressUtxo holds information about address and its transactions
type AddressUtxo struct {
	Txid          string  `json:"txid"`
	Vout          uint32  `json:"vout"`
	Amount        string  `json:"amount"`
	AmountSat     big.Int `json:"satoshis"`
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
