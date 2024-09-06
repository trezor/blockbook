package btc

import (
	"encoding/json"
	"math/big"

	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
)

// temp params for signet(wait btcd commit)
// magic numbers
const (
	Testnet4Magic wire.BitcoinNet = 0x283f161c
)

// chain parameters
var (
	TestNet4Params chaincfg.Params
)

func init() {
	TestNet4Params = chaincfg.TestNet3Params
	TestNet4Params.Net = Testnet4Magic
}

// BitcoinParser handle
type BitcoinParser struct {
	*BitcoinLikeParser
}

// NewBitcoinParser returns new BitcoinParser instance
func NewBitcoinParser(params *chaincfg.Params, c *Configuration) *BitcoinParser {
	p := &BitcoinParser{
		BitcoinLikeParser: NewBitcoinLikeParser(params, c),
	}
	p.VSizeSupport = true
	return p
}

// GetChainParams contains network parameters for the main Bitcoin network,
// the regression test Bitcoin network, the test Bitcoin network and
// the simulation test Bitcoin network, in this order
func GetChainParams(chain string) *chaincfg.Params {
	if !chaincfg.IsRegistered(&chaincfg.MainNetParams) {
		chaincfg.RegisterBitcoinParams()
	}
	switch chain {
	case "test":
		return &chaincfg.TestNet3Params
	case "testnet4":
		return &TestNet4Params
	case "regtest":
		return &chaincfg.RegressionNetParams
	case "signet":
		return &chaincfg.SigNetParams
	}
	return &chaincfg.MainNetParams
}

// ScriptPubKey contains data about output script
type ScriptPubKey struct {
	// Asm       string   `json:"asm"`
	Hex string `json:"hex,omitempty"`
	// Type      string   `json:"type"`
	Addresses []string `json:"addresses"` // removed from Bitcoind 22.0.0
	Address   string   `json:"address"`   // used in Bitcoind 22.0.0
}

// Vout contains data about tx output
type Vout struct {
	ValueSat     big.Int
	JsonValue    common.JSONNumber `json:"value"`
	N            uint32            `json:"n"`
	ScriptPubKey ScriptPubKey      `json:"scriptPubKey"`
}

// Tx is blockchain transaction
// unnecessary fields are commented out to avoid overhead
type Tx struct {
	Hex         string       `json:"hex"`
	Txid        string       `json:"txid"`
	Version     int32        `json:"version"`
	LockTime    uint32       `json:"locktime"`
	VSize       int64        `json:"vsize,omitempty"`
	Vin         []bchain.Vin `json:"vin"`
	Vout        []Vout       `json:"vout"`
	BlockHeight uint32       `json:"blockHeight,omitempty"`
	// BlockHash     string `json:"blockhash,omitempty"`
	Confirmations    uint32      `json:"confirmations,omitempty"`
	Time             int64       `json:"time,omitempty"`
	Blocktime        int64       `json:"blocktime,omitempty"`
	CoinSpecificData interface{} `json:"-"`
}

// ParseTxFromJson parses JSON message containing transaction and returns Tx struct
// Bitcoind version 22.0.0 removed ScriptPubKey.Addresses from the API and replaced it by a single Address
func (p *BitcoinParser) ParseTxFromJson(msg json.RawMessage) (*bchain.Tx, error) {
	var bitcoinTx Tx
	var tx bchain.Tx
	err := json.Unmarshal(msg, &bitcoinTx)
	if err != nil {
		return nil, err
	}

	// it is necessary to copy bitcoinTx to Tx to make it compatible
	tx.Hex = bitcoinTx.Hex
	tx.Txid = bitcoinTx.Txid
	tx.Version = bitcoinTx.Version
	tx.LockTime = bitcoinTx.LockTime
	tx.VSize = bitcoinTx.VSize
	tx.Vin = bitcoinTx.Vin
	tx.BlockHeight = bitcoinTx.BlockHeight
	tx.Confirmations = bitcoinTx.Confirmations
	tx.Time = bitcoinTx.Time
	tx.Blocktime = bitcoinTx.Blocktime
	tx.CoinSpecificData = bitcoinTx.CoinSpecificData
	tx.Vout = make([]bchain.Vout, len(bitcoinTx.Vout))

	for i := range bitcoinTx.Vout {
		bitcoinVout := &bitcoinTx.Vout[i]
		vout := &tx.Vout[i]
		// convert vout.JsonValue to big.Int and clear it, it is only temporary value used for unmarshal
		vout.ValueSat, err = p.AmountToBigInt(bitcoinVout.JsonValue)
		if err != nil {
			return nil, err
		}
		vout.N = bitcoinVout.N
		vout.ScriptPubKey.Hex = bitcoinVout.ScriptPubKey.Hex
		// convert single Address to Addresses if Addresses are empty
		if len(bitcoinVout.ScriptPubKey.Addresses) == 0 {
			vout.ScriptPubKey.Addresses = []string{bitcoinVout.ScriptPubKey.Address}
		} else {
			vout.ScriptPubKey.Addresses = bitcoinVout.ScriptPubKey.Addresses
		}
	}

	return &tx, nil
}
