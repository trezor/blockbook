package api

import (
	"math/big"

	"github.com/trezor/blockbook/bchain"
)

// ScriptSigV1 is used for legacy api v1
type ScriptSigV1 struct {
	Hex string `json:"hex,omitempty"`
	Asm string `json:"asm,omitempty"`
}

// VinV1 is used for legacy api v1
type VinV1 struct {
	Txid      string                   `json:"txid"`
	Vout      uint32                   `json:"vout"`
	Sequence  int64                    `json:"sequence,omitempty"`
	N         int                      `json:"n"`
	ScriptSig ScriptSigV1              `json:"scriptSig"`
	AddrDesc  bchain.AddressDescriptor `json:"-"`
	Addresses []string                 `json:"addresses"`
	IsAddress bool                     `json:"-"`
	Value     string                   `json:"value"`
	ValueSat  big.Int                  `json:"-"`
}

// ScriptPubKeyV1 is used for legacy api v1
type ScriptPubKeyV1 struct {
	Hex       string                   `json:"hex,omitempty"`
	Asm       string                   `json:"asm,omitempty"`
	AddrDesc  bchain.AddressDescriptor `json:"-"`
	Addresses []string                 `json:"addresses"`
	IsAddress bool                     `json:"-"`
	Type      string                   `json:"type,omitempty"`
}

// VoutV1 is used for legacy api v1
type VoutV1 struct {
	Value        string         `json:"value"`
	ValueSat     big.Int        `json:"-"`
	N            int            `json:"n"`
	ScriptPubKey ScriptPubKeyV1 `json:"scriptPubKey"`
	Spent        bool           `json:"spent"`
	SpentTxID    string         `json:"spentTxId,omitempty"`
	SpentIndex   int            `json:"spentIndex,omitempty"`
	SpentHeight  int            `json:"spentHeight,omitempty"`
}

// TxV1 is used for legacy api v1
type TxV1 struct {
	Txid          string   `json:"txid"`
	Version       int32    `json:"version,omitempty"`
	Locktime      uint32   `json:"locktime,omitempty"`
	Vin           []VinV1  `json:"vin"`
	Vout          []VoutV1 `json:"vout"`
	Blockhash     string   `json:"blockhash,omitempty"`
	Blockheight   int      `json:"blockheight"`
	Confirmations uint32   `json:"confirmations"`
	Time          int64    `json:"time,omitempty"`
	Blocktime     int64    `json:"blocktime"`
	ValueOut      string   `json:"valueOut"`
	ValueOutSat   big.Int  `json:"-"`
	Size          int      `json:"size,omitempty"`
	ValueIn       string   `json:"valueIn"`
	ValueInSat    big.Int  `json:"-"`
	Fees          string   `json:"fees"`
	FeesSat       big.Int  `json:"-"`
	Hex           string   `json:"hex"`
}

// AddressV1 is used for legacy api v1
type AddressV1 struct {
	Paging
	AddrStr                 string   `json:"addrStr"`
	Balance                 string   `json:"balance"`
	TotalReceived           string   `json:"totalReceived"`
	TotalSent               string   `json:"totalSent"`
	UnconfirmedBalance      string   `json:"unconfirmedBalance"`
	UnconfirmedTxApperances int      `json:"unconfirmedTxApperances"`
	TxApperances            int      `json:"txApperances"`
	Transactions            []*TxV1  `json:"txs,omitempty"`
	Txids                   []string `json:"transactions,omitempty"`
}

// AddressUtxoV1 is used for legacy api v1
type AddressUtxoV1 struct {
	Txid          string  `json:"txid"`
	Vout          uint32  `json:"vout"`
	Amount        string  `json:"amount"`
	AmountSat     big.Int `json:"satoshis"`
	Height        int     `json:"height,omitempty"`
	Confirmations int     `json:"confirmations"`
}

// BlockV1 contains information about block
type BlockV1 struct {
	Paging
	BlockInfo
	TxCount      int     `json:"txCount"`
	Transactions []*TxV1 `json:"txs,omitempty"`
}

// TxToV1 converts Tx to TxV1
func (w *Worker) TxToV1(tx *Tx) *TxV1 {
	d := w.chainParser.AmountDecimals()
	vinV1 := make([]VinV1, len(tx.Vin))
	for i := range tx.Vin {
		v := &tx.Vin[i]
		vinV1[i] = VinV1{
			AddrDesc:  v.AddrDesc,
			Addresses: v.Addresses,
			N:         v.N,
			ScriptSig: ScriptSigV1{
				Asm: v.Asm,
				Hex: v.Hex,
			},
			IsAddress: v.IsAddress,
			Sequence:  v.Sequence,
			Txid:      v.Txid,
			Value:     v.ValueSat.DecimalString(d),
			ValueSat:  v.ValueSat.AsBigInt(),
			Vout:      v.Vout,
		}
	}
	voutV1 := make([]VoutV1, len(tx.Vout))
	for i := range tx.Vout {
		v := &tx.Vout[i]
		voutV1[i] = VoutV1{
			N: v.N,
			ScriptPubKey: ScriptPubKeyV1{
				AddrDesc:  v.AddrDesc,
				Addresses: v.Addresses,
				Asm:       v.Asm,
				Hex:       v.Hex,
				IsAddress: v.IsAddress,
				Type:      v.Type,
			},
			Spent:       v.Spent,
			SpentHeight: v.SpentHeight,
			SpentIndex:  v.SpentIndex,
			SpentTxID:   v.SpentTxID,
			Value:       v.ValueSat.DecimalString(d),
			ValueSat:    v.ValueSat.AsBigInt(),
		}
	}
	return &TxV1{
		Blockhash:     tx.Blockhash,
		Blockheight:   tx.Blockheight,
		Blocktime:     tx.Blocktime,
		Confirmations: tx.Confirmations,
		Fees:          tx.FeesSat.DecimalString(d),
		FeesSat:       tx.FeesSat.AsBigInt(),
		Hex:           tx.Hex,
		Locktime:      tx.Locktime,
		Size:          tx.Size,
		Time:          tx.Blocktime,
		Txid:          tx.Txid,
		ValueIn:       tx.ValueInSat.DecimalString(d),
		ValueInSat:    tx.ValueInSat.AsBigInt(),
		ValueOut:      tx.ValueOutSat.DecimalString(d),
		ValueOutSat:   tx.ValueOutSat.AsBigInt(),
		Version:       tx.Version,
		Vin:           vinV1,
		Vout:          voutV1,
	}
}

func (w *Worker) transactionsToV1(txs []*Tx) []*TxV1 {
	v1 := make([]*TxV1, len(txs))
	for i := range txs {
		v1[i] = w.TxToV1(txs[i])
	}
	return v1
}

// AddressToV1 converts Address to AddressV1
func (w *Worker) AddressToV1(a *Address) *AddressV1 {
	d := w.chainParser.AmountDecimals()
	return &AddressV1{
		AddrStr:                 a.AddrStr,
		Balance:                 a.BalanceSat.DecimalString(d),
		Paging:                  a.Paging,
		TotalReceived:           a.TotalReceivedSat.DecimalString(d),
		TotalSent:               a.TotalSentSat.DecimalString(d),
		Transactions:            w.transactionsToV1(a.Transactions),
		TxApperances:            a.Txs,
		Txids:                   a.Txids,
		UnconfirmedBalance:      a.UnconfirmedBalanceSat.DecimalString(d),
		UnconfirmedTxApperances: a.UnconfirmedTxs,
	}
}

// AddressUtxoToV1 converts []AddressUtxo to []AddressUtxoV1
func (w *Worker) AddressUtxoToV1(au Utxos) []AddressUtxoV1 {
	d := w.chainParser.AmountDecimals()
	v1 := make([]AddressUtxoV1, len(au))
	for i := range au {
		utxo := &au[i]
		v1[i] = AddressUtxoV1{
			AmountSat:     utxo.AmountSat.AsBigInt(),
			Amount:        utxo.AmountSat.DecimalString(d),
			Confirmations: utxo.Confirmations,
			Height:        utxo.Height,
			Txid:          utxo.Txid,
			Vout:          uint32(utxo.Vout),
		}
	}
	return v1
}

// BlockToV1 converts Address to Address1
func (w *Worker) BlockToV1(b *Block) *BlockV1 {
	return &BlockV1{
		BlockInfo:    b.BlockInfo,
		Paging:       b.Paging,
		Transactions: w.transactionsToV1(b.Transactions),
		TxCount:      b.TxCount,
	}
}
