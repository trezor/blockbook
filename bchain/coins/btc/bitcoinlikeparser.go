package btc

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	vlq "github.com/bsm/go-vlq"
	"github.com/juju/errors"
	"github.com/martinboehm/btcd/blockchain"
	"github.com/martinboehm/btcd/btcec"
	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil"
	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/martinboehm/btcutil/hdkeychain"
	"github.com/martinboehm/btcutil/txscript"
	"github.com/trezor/blockbook/bchain"
)

// OutputScriptToAddressesFunc converts ScriptPubKey to bitcoin addresses
type OutputScriptToAddressesFunc func(script []byte) ([]string, bool, error)

// BitcoinLikeParser handle
type BitcoinLikeParser struct {
	*bchain.BaseParser
	Params                       *chaincfg.Params
	OutputScriptToAddressesFunc  OutputScriptToAddressesFunc
	XPubMagic                    uint32
	XPubMagicSegwitP2sh          uint32
	XPubMagicSegwitNative        uint32
	Slip44                       uint32
	VSizeSupport                 bool
	minimumCoinbaseConfirmations int
}

// NewBitcoinLikeParser returns new BitcoinLikeParser instance
func NewBitcoinLikeParser(params *chaincfg.Params, c *Configuration) *BitcoinLikeParser {
	p := &BitcoinLikeParser{
		BaseParser: &bchain.BaseParser{
			BlockAddressesToKeep: c.BlockAddressesToKeep,
			AmountDecimalPoint:   8,
			AddressAliases:       c.AddressAliases,
		},
		Params:                       params,
		XPubMagic:                    c.XPubMagic,
		XPubMagicSegwitP2sh:          c.XPubMagicSegwitP2sh,
		XPubMagicSegwitNative:        c.XPubMagicSegwitNative,
		Slip44:                       c.Slip44,
		minimumCoinbaseConfirmations: c.MinimumCoinbaseConfirmations,
	}
	p.OutputScriptToAddressesFunc = p.outputScriptToAddresses
	return p
}

// GetAddrDescFromVout returns internal address representation (descriptor) of given transaction output
func (p *BitcoinLikeParser) GetAddrDescFromVout(output *bchain.Vout) (bchain.AddressDescriptor, error) {
	ad, err := hex.DecodeString(output.ScriptPubKey.Hex)
	if err != nil {
		return ad, err
	}
	// convert possible P2PK script to P2PKH
	// so that all transactions by given public key are indexed together
	return txscript.ConvertP2PKtoP2PKH(p.Params.Base58CksumHasher, ad)
}

// GetAddrDescFromAddress returns internal address representation (descriptor) of given address
func (p *BitcoinLikeParser) GetAddrDescFromAddress(address string) (bchain.AddressDescriptor, error) {
	return p.addressToOutputScript(address)
}

// GetAddressesFromAddrDesc returns addresses for given address descriptor with flag if the addresses are searchable
func (p *BitcoinLikeParser) GetAddressesFromAddrDesc(addrDesc bchain.AddressDescriptor) ([]string, bool, error) {
	return p.OutputScriptToAddressesFunc(addrDesc)
}

// GetScriptFromAddrDesc returns output script for given address descriptor
func (p *BitcoinLikeParser) GetScriptFromAddrDesc(addrDesc bchain.AddressDescriptor) ([]byte, error) {
	return addrDesc, nil
}

// IsAddrDescIndexable returns true if AddressDescriptor should be added to index
// empty or OP_RETURN scripts are not indexed
func (p *BitcoinLikeParser) IsAddrDescIndexable(addrDesc bchain.AddressDescriptor) bool {
	if len(addrDesc) == 0 || addrDesc[0] == txscript.OP_RETURN {
		return false
	}
	return true
}

// addressToOutputScript converts bitcoin address to ScriptPubKey
func (p *BitcoinLikeParser) addressToOutputScript(address string) ([]byte, error) {
	da, err := btcutil.DecodeAddress(address, p.Params)
	if err != nil {
		return nil, err
	}
	script, err := txscript.PayToAddrScript(da)
	if err != nil {
		return nil, err
	}
	return script, nil
}

// TryParseOPReturn tries to process OP_RETURN script and return its string representation
func (p *BitcoinLikeParser) TryParseOPReturn(script []byte) string {
	if len(script) > 1 && script[0] == txscript.OP_RETURN {
		// trying 2 variants of OP_RETURN data
		// 1) OP_RETURN OP_PUSHDATA1 <datalen> <data>
		// 2) OP_RETURN <datalen> <data>
		// 3) OP_RETURN OP_PUSHDATA2 <datalenlow> <datalenhigh> <data>
		var data []byte
		var l int
		if script[1] == txscript.OP_PUSHDATA1 && len(script) > 2 {
			l = int(script[2])
			data = script[3:]
			if l != len(data) {
				l = int(script[1])
				data = script[2:]
			}
		} else if script[1] == txscript.OP_PUSHDATA2 && len(script) > 3 {
			l = int(script[2]) + int(script[3])<<8
			data = script[4:]
		} else {
			l = int(script[1])
			data = script[2:]
		}
		if l == len(data) {
			var ed string

			ed = p.tryParseOmni(data)
			if ed != "" {
				return ed
			}

			if utf8.Valid(data) {
				ed = "(" + string(data) + ")"
			} else {
				ed = hex.EncodeToString(data)
			}
			return "OP_RETURN " + ed
		}
	}
	return ""
}

var omniCurrencyMap = map[uint32]string{
	1:  "Omni",
	2:  "Test Omni",
	31: "TetherUS",
}

// tryParseOmni tries to extract Omni simple send transaction from script
func (p *BitcoinLikeParser) tryParseOmni(data []byte) string {

	// currently only simple send transaction version 0 is supported, see
	// https://github.com/OmniLayer/spec#transfer-coins-simple-send
	if len(data) != 20 || data[0] != 'o' {
		return ""
	}
	// omni (4) <tx_version> (2) <tx_type> (2)
	omniHeader := []byte{'o', 'm', 'n', 'i', 0, 0, 0, 0}
	if !bytes.Equal(data[0:8], omniHeader) {
		return ""
	}

	currencyID := binary.BigEndian.Uint32(data[8:12])
	currency, ok := omniCurrencyMap[currencyID]
	if !ok {
		return ""
	}
	amount := new(big.Int)
	amount.SetBytes(data[12:])
	amountStr := p.AmountToDecimalString(amount)

	ed := "OMNI Simple Send: " + amountStr + " " + currency + " (#" + strconv.Itoa(int(currencyID)) + ")"
	return ed
}

// outputScriptToAddresses converts ScriptPubKey to addresses with a flag that the addresses are searchable
func (p *BitcoinLikeParser) outputScriptToAddresses(script []byte) ([]string, bool, error) {
	sc, addresses, _, err := txscript.ExtractPkScriptAddrs(script, p.Params)
	if err != nil {
		return nil, false, err
	}
	rv := make([]string, len(addresses))
	for i, a := range addresses {
		rv[i] = a.EncodeAddress()
	}
	var s bool
	if sc == txscript.PubKeyHashTy || sc == txscript.WitnessV0PubKeyHashTy || sc == txscript.ScriptHashTy || sc == txscript.WitnessV0ScriptHashTy || sc == txscript.WitnessV1TaprootTy {
		s = true
	} else if len(rv) == 0 {
		or := p.TryParseOPReturn(script)
		if or != "" {
			rv = []string{or}
		}
	}
	return rv, s, nil
}

// TxFromMsgTx converts bitcoin wire Tx to bchain.Tx
func (p *BitcoinLikeParser) TxFromMsgTx(t *wire.MsgTx, parseAddresses bool) bchain.Tx {
	var vSize int64
	if p.VSizeSupport {
		baseSize := t.SerializeSizeStripped()
		totalSize := t.SerializeSize()
		weight := int64((baseSize * (blockchain.WitnessScaleFactor - 1)) + totalSize)
		vSize = (weight + (blockchain.WitnessScaleFactor - 1)) / blockchain.WitnessScaleFactor
	}

	vin := make([]bchain.Vin, len(t.TxIn))
	for i, in := range t.TxIn {
		if blockchain.IsCoinBaseTx(t) {
			vin[i] = bchain.Vin{
				Coinbase: hex.EncodeToString(in.SignatureScript),
				Sequence: in.Sequence,
			}
			break
		}
		s := bchain.ScriptSig{
			Hex: hex.EncodeToString(in.SignatureScript),
			// missing: Asm,
		}
		vin[i] = bchain.Vin{
			Txid:      in.PreviousOutPoint.Hash.String(),
			Vout:      in.PreviousOutPoint.Index,
			Sequence:  in.Sequence,
			ScriptSig: s,
		}
	}
	vout := make([]bchain.Vout, len(t.TxOut))
	for i, out := range t.TxOut {
		addrs := []string{}
		if parseAddresses {
			addrs, _, _ = p.OutputScriptToAddressesFunc(out.PkScript)
		}
		s := bchain.ScriptPubKey{
			Hex:       hex.EncodeToString(out.PkScript),
			Addresses: addrs,
			// missing: Asm,
			// missing: Type,
		}
		var vs big.Int
		vs.SetInt64(out.Value)
		vout[i] = bchain.Vout{
			ValueSat:     vs,
			N:            uint32(i),
			ScriptPubKey: s,
		}
	}
	tx := bchain.Tx{
		Txid:     t.TxHash().String(),
		Version:  t.Version,
		LockTime: t.LockTime,
		VSize:    vSize,
		Vin:      vin,
		Vout:     vout,
		// skip: BlockHash,
		// skip: Confirmations,
		// skip: Time,
		// skip: Blocktime,
	}
	return tx
}

// ParseTx parses byte array containing transaction and returns Tx struct
func (p *BitcoinLikeParser) ParseTx(b []byte) (*bchain.Tx, error) {
	t := wire.MsgTx{}
	r := bytes.NewReader(b)
	if err := t.Deserialize(r); err != nil {
		return nil, err
	}
	tx := p.TxFromMsgTx(&t, true)
	tx.Hex = hex.EncodeToString(b)
	return &tx, nil
}

// ParseBlock parses raw block to our Block struct
func (p *BitcoinLikeParser) ParseBlock(b []byte) (*bchain.Block, error) {
	w := wire.MsgBlock{}
	r := bytes.NewReader(b)

	if err := w.Deserialize(r); err != nil {
		return nil, err
	}

	txs := make([]bchain.Tx, len(w.Transactions))
	for ti, t := range w.Transactions {
		txs[ti] = p.TxFromMsgTx(t, false)
	}

	return &bchain.Block{
		BlockHeader: bchain.BlockHeader{
			Size: len(b),
			Time: w.Header.Timestamp.Unix(),
		},
		Txs: txs,
	}, nil
}

// PackTx packs transaction to byte array
func (p *BitcoinLikeParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
	buf := make([]byte, 4+vlq.MaxLen64+len(tx.Hex)/2)
	binary.BigEndian.PutUint32(buf[0:4], height)
	vl := vlq.PutInt(buf[4:4+vlq.MaxLen64], blockTime)
	hl, err := hex.Decode(buf[4+vl:], []byte(tx.Hex))
	return buf[0 : 4+vl+hl], err
}

// UnpackTx unpacks transaction from byte array
func (p *BitcoinLikeParser) UnpackTx(buf []byte) (*bchain.Tx, uint32, error) {
	height := binary.BigEndian.Uint32(buf)
	bt, l := vlq.Int(buf[4:])
	tx, err := p.ParseTx(buf[4+l:])
	if err != nil {
		return nil, 0, err
	}
	tx.Blocktime = bt

	return tx, height, nil
}

// MinimumCoinbaseConfirmations returns minimum number of confirmations a coinbase transaction must have before it can be spent
func (p *BitcoinLikeParser) MinimumCoinbaseConfirmations() int {
	return p.minimumCoinbaseConfirmations
}

// SupportsVSize returns true if vsize of a transaction should be computed and returned by API
func (p *BitcoinLikeParser) SupportsVSize() bool {
	return p.VSizeSupport
}

var tapTweakTagHash = sha256.Sum256([]byte("TapTweak"))

func tapTweakHash(msg []byte) []byte {
	tagLen := len(tapTweakTagHash)
	m := make([]byte, tagLen*2+len(msg))
	copy(m[:tagLen], tapTweakTagHash[:])
	copy(m[tagLen:tagLen*2], tapTweakTagHash[:])
	copy(m[tagLen*2:], msg)
	h := sha256.Sum256(m)
	return h[:]
}

func (p *BitcoinLikeParser) taprootAddrFromExtKey(extKey *hdkeychain.ExtendedKey) (*btcutil.AddressWitnessTaproot, error) {
	curve := btcec.S256()
	t := new(big.Int)

	// tweak the derived pubkey to the output pub key according to https://en.bitcoin.it/wiki/BIP_0341
	// and https://github.com/bitcoin/bips/blob/master/bip-0086.mediawiki
	derived_key := extKey.PubKeyBytes()[1:]

	t.SetBytes(tapTweakHash(derived_key))
	// Fail if t >=order of the base point
	if t.Cmp(curve.N) >= 0 {
		return nil, errors.New("greater than or equal to curve order")
	}
	// Q = point_add(lift_x(int_from_bytes(pubkey)), point_mul(G, t))
	ipx, ipy, err := btcec.LiftX(derived_key)
	if err != nil {
		return nil, err
	}
	tGx, tGy := curve.ScalarBaseMult(t.Bytes())
	output_pubkey, _ := curve.Add(ipx, ipy, tGx, tGy)
	//
	b := output_pubkey.Bytes()
	// the x coordinate on the curve can be a number small enough that it does not need 32 bytes required for the output script
	if len(b) < 32 {
		b = make([]byte, 32)
		output_pubkey.FillBytes(b)
	}
	return btcutil.NewAddressWitnessTaproot(b, p.Params)
}

func (p *BitcoinLikeParser) addrDescFromExtKey(extKey *hdkeychain.ExtendedKey, descriptor *bchain.XpubDescriptor) (bchain.AddressDescriptor, error) {
	var a btcutil.Address
	var err error
	switch descriptor.Type {
	case bchain.P2PKH:
		a, err = extKey.Address(p.Params)
	case bchain.P2SHWPKH:
		// redeemScript <witness version: OP_0><len pubKeyHash: 20><20-byte-pubKeyHash>
		pubKeyHash := btcutil.Hash160(extKey.PubKeyBytes())
		redeemScript := make([]byte, len(pubKeyHash)+2)
		redeemScript[0] = 0
		redeemScript[1] = byte(len(pubKeyHash))
		copy(redeemScript[2:], pubKeyHash)
		hash := btcutil.Hash160(redeemScript)
		a, err = btcutil.NewAddressScriptHashFromHash(hash, p.Params)
	case bchain.P2WPKH:
		a, err = btcutil.NewAddressWitnessPubKeyHash(btcutil.Hash160(extKey.PubKeyBytes()), p.Params)
	case bchain.P2TR:
		a, err = p.taprootAddrFromExtKey(extKey)
	default:
		return nil, errors.New("Unsupported xpub descriptor type")
	}
	if err != nil {
		return nil, err
	}
	return txscript.PayToAddrScript(a)
}

func (p *BitcoinLikeParser) xpubDescriptorFromXpub(xpub string) (*bchain.XpubDescriptor, error) {
	var descriptor bchain.XpubDescriptor
	extKey, err := hdkeychain.NewKeyFromString(xpub, p.Params.Base58CksumHasher)
	if err != nil {
		return nil, err
	}
	descriptor.Xpub = xpub
	descriptor.XpubDescriptor = xpub
	if extKey.Version() == p.XPubMagicSegwitP2sh {
		descriptor.Type = bchain.P2SHWPKH
		descriptor.Bip = "49"
	} else if extKey.Version() == p.XPubMagicSegwitNative {
		descriptor.Type = bchain.P2WPKH
		descriptor.Bip = "84"
	} else {
		descriptor.Type = bchain.P2PKH
		descriptor.Bip = "44"
	}
	descriptor.ChangeIndexes = []uint32{0, 1}
	descriptor.ExtKey = extKey
	return &descriptor, nil
}

var (
	xpubDesriptorRegex     *regexp.Regexp
	typeSubexpIndex        int
	bipSubexpIndex         int
	xpubSubexpIndex        int
	changeSubexpIndex      int
	changeList1SubexpIndex int
	changeList2SubexpIndex int
)

func init() {
	xpubDesriptorRegex, _ = regexp.Compile(`^(?P<type>(sh\(wpkh|wpkh|pk|pkh|wpkh|wsh|tr))\((\[\w+/(?P<bip>\d+)'/\d+'?/\d+'?\])?(?P<xpub>\w+)(/(({(?P<changelist1>\d+(,\d+)*)})|(<(?P<changelist2>\d+(;\d+)*)>)|(?P<change>\d+))/\*)?\)+`)
	typeSubexpIndex = xpubDesriptorRegex.SubexpIndex("type")
	bipSubexpIndex = xpubDesriptorRegex.SubexpIndex("bip")
	xpubSubexpIndex = xpubDesriptorRegex.SubexpIndex("xpub")
	changeList1SubexpIndex = xpubDesriptorRegex.SubexpIndex("changelist1")
	changeList2SubexpIndex = xpubDesriptorRegex.SubexpIndex("changelist2")
	changeSubexpIndex = xpubDesriptorRegex.SubexpIndex("change")
	if changeSubexpIndex < 0 {
		panic("Invalid bitcoinparser xpubDesriptorRegex")
	}
}

// ParseXpub parses xpub (or xpub descriptor) and returns XpubDescriptor
func (p *BitcoinLikeParser) ParseXpub(xpub string) (*bchain.XpubDescriptor, error) {
	match := xpubDesriptorRegex.FindStringSubmatch(xpub)
	if len(match) > changeSubexpIndex {
		var descriptor bchain.XpubDescriptor
		descriptor.XpubDescriptor = xpub
		m := match[typeSubexpIndex]
		switch m {
		case "pkh":
			descriptor.Type = bchain.P2PKH
			descriptor.Bip = "44"
		case "sh(wpkh":
			descriptor.Type = bchain.P2SHWPKH
			descriptor.Bip = "49"
		case "wpkh":
			descriptor.Type = bchain.P2WPKH
			descriptor.Bip = "84"
		case "tr":
			descriptor.Type = bchain.P2TR
			descriptor.Bip = "86"
		default:
			return nil, errors.Errorf("Xpub descriptor %s is not supported", m)
		}
		if len(match[bipSubexpIndex]) > 0 {
			descriptor.Bip = match[bipSubexpIndex]
		}
		descriptor.Xpub = match[xpubSubexpIndex]
		extKey, err := hdkeychain.NewKeyFromString(descriptor.Xpub, p.Params.Base58CksumHasher)
		if err != nil {
			return nil, err
		}
		descriptor.ExtKey = extKey
		if len(match[changeSubexpIndex]) > 0 {
			change, err := strconv.ParseUint(match[changeSubexpIndex], 10, 32)
			if err != nil {
				return nil, err
			}
			descriptor.ChangeIndexes = []uint32{uint32(change)}
		} else {
			if len(match[changeList1SubexpIndex]) > 0 || len(match[changeList2SubexpIndex]) > 0 {
				var changes []string
				if len(match[changeList1SubexpIndex]) > 0 {
					changes = strings.Split(match[changeList1SubexpIndex], ",")
				} else {
					changes = strings.Split(match[changeList2SubexpIndex], ";")
				}
				if len(changes) == 0 {
					return nil, errors.New("Invalid xpub descriptor, cannot parse change")
				}
				descriptor.ChangeIndexes = make([]uint32, len(changes))
				for i, ch := range changes {
					change, err := strconv.ParseUint(ch, 10, 32)
					if err != nil {
						return nil, err
					}
					descriptor.ChangeIndexes[i] = uint32(change)

				}
			} else {
				// default to {0,1}
				descriptor.ChangeIndexes = []uint32{0, 1}
			}

		}
		return &descriptor, nil
	}
	return p.xpubDescriptorFromXpub(xpub)

}

// DeriveAddressDescriptors derives address descriptors from given xpub for listed indexes
func (p *BitcoinLikeParser) DeriveAddressDescriptors(descriptor *bchain.XpubDescriptor, change uint32, indexes []uint32) ([]bchain.AddressDescriptor, error) {
	ad := make([]bchain.AddressDescriptor, len(indexes))
	changeExtKey, err := descriptor.ExtKey.(*hdkeychain.ExtendedKey).Derive(change)
	if err != nil {
		return nil, err
	}
	for i, index := range indexes {
		indexExtKey, err := changeExtKey.Derive(index)
		if err != nil {
			return nil, err
		}
		ad[i], err = p.addrDescFromExtKey(indexExtKey, descriptor)
		if err != nil {
			return nil, err
		}
	}
	return ad, nil
}

// DeriveAddressDescriptorsFromTo derives address descriptors from given xpub for addresses in index range
func (p *BitcoinLikeParser) DeriveAddressDescriptorsFromTo(descriptor *bchain.XpubDescriptor, change uint32, fromIndex uint32, toIndex uint32) ([]bchain.AddressDescriptor, error) {
	if toIndex <= fromIndex {
		return nil, errors.New("toIndex<=fromIndex")
	}
	changeExtKey, err := descriptor.ExtKey.(*hdkeychain.ExtendedKey).Derive(change)
	if err != nil {
		return nil, err
	}
	ad := make([]bchain.AddressDescriptor, toIndex-fromIndex)
	for index := fromIndex; index < toIndex; index++ {
		indexExtKey, err := changeExtKey.Derive(index)
		if err != nil {
			return nil, err
		}
		ad[index-fromIndex], err = p.addrDescFromExtKey(indexExtKey, descriptor)
		if err != nil {
			return nil, err
		}
	}
	return ad, nil
}

// DerivationBasePath returns base path of xpub
func (p *BitcoinLikeParser) DerivationBasePath(descriptor *bchain.XpubDescriptor) (string, error) {
	var c string
	extKey := descriptor.ExtKey.(*hdkeychain.ExtendedKey)
	cn := extKey.ChildNum()
	if cn >= 0x80000000 {
		cn -= 0x80000000
		c = "'"
	}
	c = strconv.Itoa(int(cn)) + c
	if extKey.Depth() != 3 {
		return "unknown/" + c, nil
	}
	return "m/" + descriptor.Bip + "'/" + strconv.Itoa(int(p.Slip44)) + "'/" + c, nil
}
