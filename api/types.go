package api

import "math/big"

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
	Txid      string    `json:"txid"`
	Vout      uint32    `json:"vout"`
	Sequence  int64     `json:"sequence,omitempty"`
	N         int       `json:"n"`
	ScriptSig ScriptSig `json:"scriptSig"`
	Addr      string    `json:"addr"`
	Value     string    `json:"value"`
	ValueSat  big.Int   `json:"-"`
}

type ScriptPubKey struct {
	Hex       string   `json:"hex"`
	Asm       string   `json:"asm,omitempty"`
	Addresses []string `json:"addresses"`
	Type      string   `json:"type,omitempty"`
}
type Vout struct {
	Value        string       `json:"value"`
	ValueSat     big.Int      `json:"-"`
	N            int          `json:"n"`
	ScriptPubKey ScriptPubKey `json:"scriptPubKey"`
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
	WithSpends    bool   `json:"withSpends,omitempty"`
}

type Address struct {
	AddrStr                 string `json:"addrStr"`
	Balance                 string `json:"balance"`
	TotalReceived           string `json:"totalReceived"`
	TotalSent               string `json:"totalSent"`
	UnconfirmedBalance      string `json:"unconfirmedBalance"`
	UnconfirmedTxApperances int    `json:"unconfirmedTxApperances"`
	TxApperances            int    `json:"txApperances"`
	Transactions            []*Tx  `json:"transactions"`
	Page                    int    `json:"page"`
	TotalPages              int    `json:"totalPages"`
	TxsOnPage               int    `json:"txsOnPage"`
}
