package api

import (
	"blockbook/bchain"
	"blockbook/common"
	"blockbook/db"
	"math/big"
	"time"
)

const BlockbookAbout = "Blockbook - blockchain indexer for TREZOR wallet https://trezor.io/. Do not use for any other purpose."

type ApiError struct {
	Text   string
	Public bool
}

func (e *ApiError) Error() string {
	return e.Text
}

func NewApiError(s string, public bool) error {
	return &ApiError{
		Text:   s,
		Public: public,
	}
}

type ScriptSig struct {
	Hex string `json:"hex"`
	Asm string `json:"asm,omitempty"`
}

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

type ScriptPubKey struct {
	Hex        string                   `json:"hex"`
	Asm        string                   `json:"asm,omitempty"`
	AddrDesc   bchain.AddressDescriptor `json:"-"`
	Addresses  []string                 `json:"addresses"`
	Searchable bool                     `json:"-"`
	Type       string                   `json:"type,omitempty"`
}
type Vout struct {
	Value        string       `json:"value"`
	ValueSat     big.Int      `json:"-"`
	N            int          `json:"n"`
	ScriptPubKey ScriptPubKey `json:"scriptPubKey"`
	Spent        bool         `json:"-"`
	SpentTxID    string       `json:"spentTxId,omitempty"`
	SpentIndex   int          `json:"spentIndex,omitempty"`
	SpentHeight  int          `json:"spentHeight,omitempty"`
}

type Tx struct {
	Txid          string `json:"txid"`
	Version       int32  `json:"version,omitempty"`
	Locktime      uint32 `json:"locktime,omitempty"`
	Vin           []Vin  `json:"vin"`
	Vout          []Vout `json:"vout"`
	Blockhash     string `json:"blockhash,omitempty"`
	Blockheight   int    `json:"blockheight"`
	Confirmations uint32 `json:"confirmations"`
	Time          int64  `json:"time,omitempty"`
	Blocktime     int64  `json:"blocktime"`
	ValueOut      string `json:"valueOut"`
	Size          int    `json:"size,omitempty"`
	ValueIn       string `json:"valueIn"`
	Fees          string `json:"fees"`
	Hex           string `json:"hex"`
}

type Paging struct {
	Page        int `json:"page"`
	TotalPages  int `json:"totalPages"`
	ItemsOnPage int `json:"itemsOnPage"`
}

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

type Blocks struct {
	Paging
	Blocks []db.BlockInfo `json:"blocks"`
}

type Block struct {
	Paging
	bchain.BlockInfo
	TxCount      int   `json:"TxCount"`
	Transactions []*Tx `json:"txs,omitempty"`
}

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

type SystemInfo struct {
	Blockbook *BlockbookInfo    `json:"blockbook"`
	Backend   *bchain.ChainInfo `json:"backend"`
}
