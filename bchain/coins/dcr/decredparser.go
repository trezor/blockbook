package dcr

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"math/big"

	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"blockbook/bchain/coins/utils"

	cfg "github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/txscript"
	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil/chaincfg"
)

const (
	// MainnetMagic is mainnet network constant
	MainnetMagic wire.BitcoinNet = 0xd9b400f9
	// TestnetMagic is testnet network constant
	TestnetMagic wire.BitcoinNet = 0xb194aa75
)

var (
	// MainNetParams are parser parameters for mainnet
	MainNetParams chaincfg.Params
	// TestNet3Params are parser parameters for testnet
	TestNet3Params chaincfg.Params
)

func init() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic
	MainNetParams.PubKeyHashAddrID = []byte{0x07, 0x3f}
	MainNetParams.ScriptHashAddrID = []byte{0x07, 0x1a}

	TestNet3Params = chaincfg.TestNet3Params
	TestNet3Params.Net = TestnetMagic
	TestNet3Params.PubKeyHashAddrID = []byte{0x0f, 0x21}
	TestNet3Params.ScriptHashAddrID = []byte{0x0e, 0xfc}
}

// DecredParser handle
type DecredParser struct {
	*btc.BitcoinParser
	baseParser *bchain.BaseParser
}

// NewDecredParser returns new DecredParser instance
func NewDecredParser(params *chaincfg.Params, c *btc.Configuration) *DecredParser {
	return &DecredParser{
		BitcoinParser: btc.NewBitcoinParser(params, c),
		baseParser:    &bchain.BaseParser{},
	}
}

// GetChainParams contains network parameters for the main Decred network,
// and the test Decred network.
func GetChainParams(chain string) *chaincfg.Params {
	var param *chaincfg.Params

	switch chain {
	case "testnet3":
		param = &TestNet3Params
	default:
		param = &MainNetParams
	}

	if !chaincfg.IsRegistered(param) {
		if err := chaincfg.Register(param); err != nil {
			panic(err)
		}
	}
	return param
}

// ParseBlock parses raw block to our Block struct.
func (p *DecredParser) ParseBlock(b []byte) (*bchain.Block, error) {
	r := bytes.NewReader(b)
	h := wire.BlockHeader{}
	if err := h.Deserialize(r); err != nil {
		return nil, err
	}

	if (h.Version & utils.VersionAuxpow) != 0 {
		if err := utils.SkipAuxpow(r); err != nil {
			return nil, err
		}
	}

	var w wire.MsgBlock
	if err := utils.DecodeTransactions(r, 0, wire.WitnessEncoding, &w); err != nil {
		return nil, err
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

func (p *DecredParser) ParseTxFromJson(jsonTx json.RawMessage) (*bchain.Tx, error) {
	var getTxResult GetTransactionResult
	if err := json.Unmarshal([]byte(jsonTx), &getTxResult.Result); err != nil {
		return nil, err
	}

	vins := make([]bchain.Vin, len(getTxResult.Result.Vin))
	for index, input := range getTxResult.Result.Vin {
		vins[index] = bchain.Vin{
			Coinbase:  input.Coinbase,
			Txid:      input.Txid,
			Vout:      input.Vout,
			ScriptSig: bchain.ScriptSig{},
			Sequence:  input.Sequence,
			// Addresses: []string{},
		}
	}

	vouts := make([]bchain.Vout, len(getTxResult.Result.Vout))
	for index, output := range getTxResult.Result.Vout {
		vouts[index] = bchain.Vout{
			ValueSat: *big.NewInt(int64(output.Value * 100000000)),
			N:        output.N,
			ScriptPubKey: bchain.ScriptPubKey{
				Hex:       output.ScriptPubKey.Hex,
				Addresses: output.ScriptPubKey.Addresses,
			},
		}
	}

	tx := &bchain.Tx{
		Hex:           getTxResult.Result.Hex,
		Txid:          getTxResult.Result.Txid,
		Version:       getTxResult.Result.Version,
		LockTime:      getTxResult.Result.LockTime,
		Vin:           vins,
		Vout:          vouts,
		Confirmations: uint32(getTxResult.Result.Confirmations),
		Time:          getTxResult.Result.Time / 1000,
		Blocktime:     getTxResult.Result.Blocktime,
	}
	return tx, nil
}

// GetAddrDescForUnknownInput returns nil AddressDescriptor
func (p *DecredParser) GetAddrDescForUnknownInput(tx *bchain.Tx, input int) bchain.AddressDescriptor {
	return nil
}

func (p *DecredParser) GetAddrDescFromAddress(address string) (bchain.AddressDescriptor, error) {
	addressByte := []byte(address)
	return bchain.AddressDescriptor(addressByte), nil
}

func (p *DecredParser) GetAddrDescFromVout(output *bchain.Vout) (bchain.AddressDescriptor, error) {
	script, err := hex.DecodeString(output.ScriptPubKey.Hex)
	if err != nil {
		return nil, err
	}

	var params cfg.Params
	if p.Params.Name == "mainnet" {
		params = cfg.MainNetParams
	} else {
		params = cfg.TestNet3Params
	}

	scriptClass, addresses, _, err := txscript.ExtractPkScriptAddrs(txscript.DefaultScriptVersion, script, &params)
	if err != nil {
		return nil, err
	}

	if scriptClass.String() == "nulldata" {
		if parsedOPReturn := p.BitcoinParser.TryParseOPReturn(script); parsedOPReturn != "" {
			return []byte(parsedOPReturn), nil
		}
	}

	var addressByte []byte
	for i := range addresses {
		addressByte = append(addressByte, addresses[i].String()...)
	}

	return bchain.AddressDescriptor(addressByte), nil
}

func (p *DecredParser) GetAddressesFromAddrDesc(addrDesc bchain.AddressDescriptor) ([]string, bool, error) {
	var addrs []string

	if addrDesc != nil {
		addrs = append(addrs, string(addrDesc))
	}

	return addrs, true, nil
}

// PackTx packs transaction to byte array using protobuf
func (p *DecredParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
	return p.baseParser.PackTx(tx, height, blockTime)
}

// UnpackTx unpacks transaction from protobuf byte array
func (p *DecredParser) UnpackTx(buf []byte) (*bchain.Tx, uint32, error) {
	return p.baseParser.UnpackTx(buf)
}
