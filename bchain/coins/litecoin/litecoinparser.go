package litecoin

import (
	"encoding/json"

	"github.com/golang/glog"
	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
)

// magic numbers
const (
	MainnetMagic wire.BitcoinNet = 0xdbb6c0fb
	TestnetMagic wire.BitcoinNet = 0xf1c8d2fd
	RegtestMagic wire.BitcoinNet = 0xdab5bffa
)

// chain parameters
var (
	MainNetParams chaincfg.Params
	TestNetParams chaincfg.Params
)

func init() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic
	MainNetParams.PubKeyHashAddrID = []byte{48}
	MainNetParams.ScriptHashAddrID = []byte{50}
	MainNetParams.Bech32HRPSegwit = "ltc"

	TestNetParams = chaincfg.TestNet3Params
	TestNetParams.Net = TestnetMagic
	TestNetParams.PubKeyHashAddrID = []byte{111}
	TestNetParams.ScriptHashAddrID = []byte{58}
	TestNetParams.Bech32HRPSegwit = "tltc"
}

// LitecoinParser handle
type LitecoinParser struct {
	*btc.BitcoinLikeParser
	baseparser *bchain.BaseParser
}

// NewLitecoinParser returns new LitecoinParser instance
func NewLitecoinParser(params *chaincfg.Params, c *btc.Configuration) *LitecoinParser {
	return &LitecoinParser{
		BitcoinLikeParser: btc.NewBitcoinLikeParser(params, c),
		baseparser:        &bchain.BaseParser{},
	}
}

// GetChainParams contains network parameters for the main Litecoin network,
// and the test Litecoin network
func GetChainParams(chain string) *chaincfg.Params {
	// register bitcoin parameters in addition to litecoin parameters
	// litecoin has dual standard of addresses and we want to be able to
	// parse both standards
	if !chaincfg.IsRegistered(&chaincfg.MainNetParams) {
		chaincfg.RegisterBitcoinParams()
	}
	if !chaincfg.IsRegistered(&MainNetParams) {
		err := chaincfg.Register(&MainNetParams)
		if err == nil {
			err = chaincfg.Register(&TestNetParams)
		}
		if err != nil {
			panic(err)
		}
	}
	switch chain {
	case "test":
		return &TestNetParams
	default:
		return &MainNetParams
	}
}

// fallbackTx is used to handle situation when Litecoin mainnet returns
// for certain transactions Version 4294967295 instead of -1, which causes json unmarshal error
type fallbackTx struct {
	Hex         string        `json:"hex"`
	Txid        string        `json:"txid"`
	Version     uint32        `json:"version"`
	LockTime    uint32        `json:"locktime"`
	Vin         []bchain.Vin  `json:"vin"`
	Vout        []bchain.Vout `json:"vout"`
	BlockHeight uint32        `json:"blockHeight,omitempty"`
	// BlockHash     string `json:"blockhash,omitempty"`
	Confirmations uint32 `json:"confirmations,omitempty"`
	Time          int64  `json:"time,omitempty"`
	Blocktime     int64  `json:"blocktime,omitempty"`
}

// ParseTxFromJson parses JSON message containing transaction and returns Tx struct
func (p *LitecoinParser) ParseTxFromJson(msg json.RawMessage) (*bchain.Tx, error) {
	var tx bchain.Tx
	err := json.Unmarshal(msg, &tx)
	if err != nil {
		var fTx fallbackTx
		fErr := json.Unmarshal(msg, &fTx)
		// log warning with Txid possibly parsed using fallbackTx
		glog.Warningf("ParseTxFromJson txid %s to bchain.Tx error %v, using fallback method", fTx.Txid, err)
		if fErr != nil {
			return nil, fErr
		}
		tx.Hex = fTx.Hex
		tx.Txid = fTx.Txid
		tx.Version = int32(fTx.Version)
		tx.LockTime = fTx.LockTime
		tx.Vin = fTx.Vin
		tx.Vout = fTx.Vout
		tx.BlockHeight = fTx.BlockHeight
		tx.Confirmations = fTx.Confirmations
		tx.Time = fTx.Time
		tx.Blocktime = fTx.Blocktime
	}

	for i := range tx.Vout {
		vout := &tx.Vout[i]
		// convert vout.JsonValue to big.Int and clear it, it is only temporary value used for unmarshal
		vout.ValueSat, err = p.AmountToBigInt(vout.JsonValue)
		if err != nil {
			return nil, err
		}
		vout.JsonValue = ""
	}

	return &tx, nil
}

// PackTx packs transaction to byte array using protobuf
func (p *LitecoinParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
	return p.baseparser.PackTx(tx, height, blockTime)
}

// UnpackTx unpacks transaction from protobuf byte array
func (p *LitecoinParser) UnpackTx(buf []byte) (*bchain.Tx, uint32, error) {
	return p.baseparser.UnpackTx(buf)
}
