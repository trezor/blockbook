package dcr

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"math/big"

	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"blockbook/bchain/coins/utils"

	"github.com/decred/dcrd/txscript"
	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil/chaincfg"

	dch "github.com/decred/dcrd/chaincfg"
)

const (
	MainnetMagic wire.BitcoinNet = 0xd9b400f9
)

var (
	// MainNetParams are parser parameters for mainnet
	MainNetParams chaincfg.Params
	// TestNetParams are parser parameters for testnet
	TestNetParams chaincfg.Params
)

func init() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic
	MainNetParams.PubKeyHashAddrID = []byte{0x13, 0x86}
	MainNetParams.ScriptHashAddrID = []byte{0x07, 0x1a}
}

// DecredParser handle
type DecredParser struct {
	*btc.BitcoinParser
}

// NewDecredParser returns new DecredParser instance
func NewDecredParser(params *chaincfg.Params, c *btc.Configuration) *DecredParser {
	return &DecredParser{BitcoinParser: btc.NewBitcoinParser(params, c)}
}

// GetChainParams contains network parameters for the main Decred network,
// and the test Decred network
func GetChainParams(chain string) *chaincfg.Params {
	if !chaincfg.IsRegistered(&MainNetParams) {
		err := chaincfg.Register(&MainNetParams)
		if err != nil {
			panic(err)
		}
	}
	switch chain {
	default:
		return &MainNetParams
	}
}

// ParseBlock parses raw block to our Block struct
// it has special handling for Auxpow blocks that cannot be parsed by standard btc wire parser
func (p *DecredParser) ParseBlock(b []byte) (*bchain.Block, error) {
	r := bytes.NewReader(b)
	w := wire.MsgBlock{}
	h := wire.BlockHeader{}
	err := h.Deserialize(r)
	if err != nil {
		return nil, err
	}
	if (h.Version & utils.VersionAuxpow) != 0 {
		if err = utils.SkipAuxpow(r); err != nil {
			return nil, err
		}
	}

	err = utils.DecodeTransactions(r, 0, wire.WitnessEncoding, &w)
	if err != nil {
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
	getTxResult := GetTransactionResult{}
	err := json.Unmarshal([]byte(jsonTx), &getTxResult.Result)
	if err != nil {
		return nil, err
	}

	var vins = make([]bchain.Vin, 0)
	var vouts []bchain.Vout

	for _, input := range getTxResult.Result.Vin {
		vin := bchain.Vin{
			Coinbase:  input.Coinbase,
			Txid:      input.Txid,
			Vout:      input.Vout,
			ScriptSig: bchain.ScriptSig{},
			Sequence:  input.Sequence,
			Addresses: []string{},
		}
		vins = append(vins, vin)
	}

	for _, output := range getTxResult.Result.Vout {
		valueSat := *big.NewInt(int64(output.Value * 100000000))
		vout := bchain.Vout{
			ValueSat: valueSat,
			N:        output.N,
			ScriptPubKey: bchain.ScriptPubKey{
				Hex:       output.ScriptPubKey.Hex,
				Addresses: output.ScriptPubKey.Addresses,
			},
		}
		vouts = append(vouts, vout)
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

	scriptClass, addresses, _, err := txscript.ExtractPkScriptAddrs(txscript.DefaultScriptVersion, script, &dch.TestNet3Params)
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
