package monetaryunit

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"

	"github.com/juju/errors"
	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
	"github.com/trezor/blockbook/bchain/coins/utils"
)

const (
	// Net Magics
	MainnetMagic wire.BitcoinNet = 0x91c4fdea
	TestnetMagic wire.BitcoinNet = 0x477665bd
)

var (
	MainNetParams chaincfg.Params
	TestNetParams chaincfg.Params
)

func init() {
	// MonetaryUnit mainnet Address encoding magics
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic
	MainNetParams.PubKeyHashAddrID = []byte{16} // starting with '7'
	MainNetParams.ScriptHashAddrID = []byte{76}
	MainNetParams.PrivateKeyID = []byte{126}

	// MonetaryUnit testnet Address encoding magics
	TestNetParams = chaincfg.TestNet3Params
	TestNetParams.Net = TestnetMagic
	TestNetParams.PubKeyHashAddrID = []byte{139} // starting with 'x' or 'y'
	TestNetParams.ScriptHashAddrID = []byte{19}
	TestNetParams.PrivateKeyID = []byte{239}
}

// MonetaryUnitParser handle
type MonetaryUnitParser struct {
	*btc.BitcoinLikeParser
	baseparser                         *bchain.BaseParser
	BitcoinOutputScriptToAddressesFunc btc.OutputScriptToAddressesFunc
}

// NewMonetaryUnitParser returns new MonetaryUnitParser instance
func NewMonetaryUnitParser(params *chaincfg.Params, c *btc.Configuration) *MonetaryUnitParser {
	p := &MonetaryUnitParser{
		BitcoinLikeParser: btc.NewBitcoinLikeParser(params, c),
		baseparser:        &bchain.BaseParser{},
	}
	p.BitcoinOutputScriptToAddressesFunc = p.OutputScriptToAddressesFunc
	p.OutputScriptToAddressesFunc = p.outputScriptToAddresses
	return p
}

// GetChainParams contains network parameters for the main MonetaryUnit network
func GetChainParams(chain string) *chaincfg.Params {
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

// ParseBlock parses raw block to our Block struct
func (p *MonetaryUnitParser) ParseBlock(b []byte) (*bchain.Block, error) {
	r := bytes.NewReader(b)
	w := wire.MsgBlock{}
	h := wire.BlockHeader{}
	err := h.Deserialize(r)
	if err != nil {
		return nil, errors.Annotatef(err, "Deserialize")
	}

	if h.Version > 3 {
		// Skip past AccumulatorCheckpoint which was added in MonetaryUnit block version 4
		r.Seek(32, io.SeekCurrent)
	}

	err = utils.DecodeTransactions(r, 0, wire.WitnessEncoding, &w)
	if err != nil {
		return nil, errors.Annotatef(err, "DecodeTransactions")
	}

	txs := make([]bchain.Tx, len(w.Transactions))
	for ti, t := range w.Transactions {
		txs[ti] = p.TxFromMsgTx(t, false)
	}

	return &bchain.Block{
		BlockHeader: bchain.BlockHeader{
			Size: len(b),
			Time: h.Timestamp.Unix(),
		},
		Txs: txs,
	}, nil
}

// PackTx packs transaction to byte array using protobuf
func (p *MonetaryUnitParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
	return p.baseparser.PackTx(tx, height, blockTime)
}

// UnpackTx unpacks transaction from protobuf byte array
func (p *MonetaryUnitParser) UnpackTx(buf []byte) (*bchain.Tx, uint32, error) {
	return p.baseparser.UnpackTx(buf)
}

// ParseTx parses byte array containing transaction and returns Tx struct
func (p *MonetaryUnitParser) ParseTx(b []byte) (*bchain.Tx, error) {
	t := wire.MsgTx{}
	r := bytes.NewReader(b)
	if err := t.Deserialize(r); err != nil {
		return nil, err
	}
	tx := p.TxFromMsgTx(&t, true)
	tx.Hex = hex.EncodeToString(b)
	return &tx, nil
}

// ParseTxFromJson parses JSON message containing transaction and returns Tx struct
func (p *MonetaryUnitParser) ParseTxFromJson(msg json.RawMessage) (*bchain.Tx, error) {
	var tx bchain.Tx
	err := json.Unmarshal(msg, &tx)
	if err != nil {
		return nil, err
	}

	for i := range tx.Vout {
		vout := &tx.Vout[i]
		// convert vout.JsonValue to big.Int and clear it, it is only temporary value used for unmarshal
		vout.ValueSat, err = p.AmountToBigInt(vout.JsonValue)
		if err != nil {
			return nil, err
		}
		vout.JsonValue = ""

		if vout.ScriptPubKey.Addresses == nil {
			vout.ScriptPubKey.Addresses = []string{}
		}
	}

	return &tx, nil
}

// outputScriptToAddresses converts ScriptPubKey to bitcoin addresses
func (p *MonetaryUnitParser) outputScriptToAddresses(script []byte) ([]string, bool, error) {

	rv, s, _ := p.BitcoinOutputScriptToAddressesFunc(script)
	return rv, s, nil
}

func (p *MonetaryUnitParser) GetAddrDescForUnknownInput(tx *bchain.Tx, input int) bchain.AddressDescriptor {
	if len(tx.Vin) > input {
		scriptHex := tx.Vin[input].ScriptSig.Hex

		if scriptHex != "" {
			script, _ := hex.DecodeString(scriptHex)
			return script
		}
	}

	s := make([]byte, 10)
	return s
}
