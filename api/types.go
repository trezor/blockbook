package api

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
	ValueSat  int64     `json:"valueSat"`
	Value     float64   `json:"value"`
}

type ScriptPubKey struct {
	Hex       string   `json:"hex"`
	Asm       string   `json:"asm,omitempty"`
	Addresses []string `json:"addresses"`
	Type      string   `json:"type,omitempty"`
}
type Vout struct {
	Value        float64      `json:"value"`
	N            int          `json:"n"`
	ScriptPubKey ScriptPubKey `json:"scriptPubKey"`
	SpentTxID    string       `json:"spentTxId,omitempty"`
	SpentIndex   int          `json:"spentIndex,omitempty"`
	SpentHeight  int          `json:"spentHeight,omitempty"`
}

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
	ValueOut      float64 `json:"valueOut"`
	Size          int     `json:"size,omitempty"`
	ValueIn       float64 `json:"valueIn"`
	Fees          float64 `json:"fees"`
	WithSpends    bool    `json:"withSpends,omitempty"`
}

type Address struct {
	AddrStr                 string  `json:"addrStr"`
	Balance                 float64 `json:"balance"`
	BalanceSat              int64   `json:"balanceSat"`
	TotalReceived           float64 `json:"totalReceived"`
	TotalReceivedSat        int64   `json:"totalReceivedSat"`
	TotalSent               float64 `json:"totalSent"`
	TotalSentSat            int64   `json:"totalSentSat"`
	UnconfirmedBalance      float64 `json:"unconfirmedBalance"`
	UnconfirmedBalanceSat   int64   `json:"unconfirmedBalanceSat"`
	UnconfirmedTxApperances int     `json:"unconfirmedTxApperances"`
	TxApperances            int     `json:"txApperances"`
	Transactions            []*Tx   `json:"transactions"`
}
