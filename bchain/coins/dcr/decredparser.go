package dcr

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math"
	"math/big"
	"strconv"

	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"blockbook/bchain/coins/utils"

	cfg "github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/hdkeychain"
	"github.com/decred/dcrd/txscript"
	"github.com/juju/errors"
	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil/base58"
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
	MainNetParams.Base58CksumHasher = base58.Blake256D

	TestNet3Params = chaincfg.TestNet3Params
	TestNet3Params.Net = TestnetMagic
	TestNet3Params.PubKeyHashAddrID = []byte{0x0f, 0x21}
	TestNet3Params.ScriptHashAddrID = []byte{0x0e, 0xfc}
	TestNet3Params.Base58CksumHasher = base58.Blake256D
}

// DecredParser handle
type DecredParser struct {
	*btc.BitcoinParser
	baseParser *bchain.BaseParser
	netConfig  *cfg.Params
}

// NewDecredParser returns new DecredParser instance
func NewDecredParser(params *chaincfg.Params, c *btc.Configuration) *DecredParser {
	d := &DecredParser{
		BitcoinParser: btc.NewBitcoinParser(params, c),
		baseParser:    &bchain.BaseParser{},
	}

	if d.BitcoinParser.Params.Name == "testnet3" {
		d.netConfig = &cfg.TestNet3Params
	} else {
		d.netConfig = &cfg.MainNetParams
	}
	return d
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
		hexData := bchain.ScriptSig{}
		if input.ScriptSig != nil {
			hexData.Hex = input.ScriptSig.Hex
		}

		vins[index] = bchain.Vin{
			Coinbase:  input.Coinbase,
			Txid:      input.Txid,
			Vout:      input.Vout,
			ScriptSig: hexData,
			Sequence:  input.Sequence,
			// Addresses: []string{},
		}
	}

	vouts := make([]bchain.Vout, len(getTxResult.Result.Vout))
	for index, output := range getTxResult.Result.Vout {
		addr := output.ScriptPubKey.Addresses
		// If nulldata type found make asm field the address data.
		if output.ScriptPubKey.Type == "nulldata" {
			addr = []string{output.ScriptPubKey.Asm}
		}

		vouts[index] = bchain.Vout{
			ValueSat: *big.NewInt(int64(math.Round(output.Value * 1e8))),
			N:        output.N,
			ScriptPubKey: bchain.ScriptPubKey{
				Hex:       output.ScriptPubKey.Hex,
				Addresses: addr,
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
		Time:          getTxResult.Result.Time,
		Blocktime:     getTxResult.Result.Blocktime,
	}

	tx.CoinSpecificData = getTxResult.Result.TxExtraInfo

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

	scriptClass, addresses, _, err := txscript.ExtractPkScriptAddrs(txscript.DefaultScriptVersion, script, p.netConfig)
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

// *************************************************//
func (p *DecredParser) addrDescFromExtKey(extKey *hdkeychain.ExtendedKey, version uint32) (bchain.AddressDescriptor, error) {
	var a dcrutil.Address
	var err error

	pbKey, err := extKey.ECPubKey()
	if err != nil {
		return nil, err
	}

	pubKey := pbKey.Serialize()

	if version == p.XPubMagicSegwitP2sh {
		// redeemScript <witness version: OP_0><len pubKeyHash: 20><20-byte-pubKeyHash>
		pubKeyHash := dcrutil.Hash160(pubKey)
		redeemScript := make([]byte, len(pubKeyHash)+2)
		redeemScript[0] = 0
		redeemScript[1] = byte(len(pubKeyHash))
		copy(redeemScript[2:], pubKeyHash)
		hash := dcrutil.Hash160(redeemScript)
		a, err = dcrutil.NewAddressScriptHashFromHash(hash, p.netConfig)
	} else if version == p.XPubMagicSegwitNative {
		// a, err = dcrutil.NewAddressWitnessPubKeyHash(btcutil.BlakeHash160(pubKey), p.Params)
	} else {
		// default to P2PKH address
		a, err = extKey.Address(p.netConfig)
	}
	if err != nil {
		return nil, err
	}
	return txscript.PayToAddrScript(a)
}

// DeriveAddressDescriptors derives address descriptors from given xpub for listed indexes
func (p *DecredParser) DeriveAddressDescriptors(xpub string, change uint32, indexes []uint32) ([]bchain.AddressDescriptor, error) {
	extKey, err := hdkeychain.NewKeyFromString(xpub)
	if err != nil {
		return nil, err
	}
	changeExtKey, err := extKey.Child(change)
	if err != nil {
		return nil, err
	}

	version, _, _ := p.decodeXpub(xpub)

	ad := make([]bchain.AddressDescriptor, len(indexes))
	for i, index := range indexes {
		indexExtKey, err := changeExtKey.Child(index)
		if err != nil {
			return nil, err
		}
		ad[i], err = p.addrDescFromExtKey(indexExtKey, version)
		if err != nil {
			return nil, err
		}
	}
	return ad, nil
}

// DeriveAddressDescriptorsFromTo derives address descriptors from given xpub for addresses in index range
func (p *DecredParser) DeriveAddressDescriptorsFromTo(xpub string, change uint32, fromIndex uint32, toIndex uint32) ([]bchain.AddressDescriptor, error) {
	if toIndex <= fromIndex {
		return nil, errors.New("toIndex<=fromIndex")
	}
	extKey, err := hdkeychain.NewKeyFromString(xpub)
	if err != nil {
		return nil, err
	}
	changeExtKey, err := extKey.Child(change)
	if err != nil {
		return nil, err
	}

	version, _, _ := p.decodeXpub(xpub)

	ad := make([]bchain.AddressDescriptor, toIndex-fromIndex)
	for index := fromIndex; index < toIndex; index++ {
		indexExtKey, err := changeExtKey.Child(index)
		if err != nil {
			return nil, err
		}
		ad[index-fromIndex], err = p.addrDescFromExtKey(indexExtKey, version)
		if err != nil {
			return nil, err
		}
	}
	return ad, nil
}

// DerivationBasePath returns base path of xpub
func (p *DecredParser) DerivationBasePath(xpub string) (string, error) {
	var c, bip string
	version, cn, depth := p.decodeXpub(xpub)

	if cn >= 0x80000000 {
		cn -= 0x80000000
		c = "'"
	}

	c = strconv.Itoa(int(cn)) + c
	if depth != 3 {
		return "unknown/" + c, nil
	}

	if version == p.XPubMagicSegwitP2sh {
		bip = "49"
	} else if version == p.XPubMagicSegwitNative {
		bip = "84"
	} else {
		bip = "44"
	}
	return "m/" + bip + "'/" + strconv.Itoa(int(p.Slip44)) + "'/" + c, nil
}

func (p *DecredParser) decodeXpub(xpub string) (version, childNum uint32, depth uint16) {
	decoded := base58.Decode(xpub)
	payload := decoded[:len(decoded)-4]
	version = binary.BigEndian.Uint32(payload[:4])
	depth = uint16(payload[4:5][0])
	childNum = binary.BigEndian.Uint32(payload[9:13])

	return
}
