package pivx

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"

	"github.com/juju/errors"
	"github.com/martinboehm/btcd/blockchain"
	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
)

// magic numbers
const (
	MainnetMagic wire.BitcoinNet = 0xe9fdc490
	TestnetMagic wire.BitcoinNet = 0xba657645

	// Zerocoin op codes
	OP_ZEROCOINMINT  = 0xc1
	OP_ZEROCOINSPEND = 0xc2
)

// chain parameters
var (
	MainNetParams chaincfg.Params
	TestNetParams chaincfg.Params
)

func init() {
	// PIVX mainnet Address encoding magics
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = MainnetMagic
	MainNetParams.PubKeyHashAddrID = []byte{30} // starting with 'D'
	MainNetParams.ScriptHashAddrID = []byte{13}
	MainNetParams.PrivateKeyID = []byte{212}

	// PIVX testnet Address encoding magics
	TestNetParams = chaincfg.TestNet3Params
	TestNetParams.Net = TestnetMagic
	TestNetParams.PubKeyHashAddrID = []byte{139} // starting with 'x' or 'y'
	TestNetParams.ScriptHashAddrID = []byte{19}
	TestNetParams.PrivateKeyID = []byte{239}
}

// PivXParser handle
type PivXParser struct {
	*btc.BitcoinLikeParser
	baseparser                         *bchain.BaseParser
	BitcoinOutputScriptToAddressesFunc btc.OutputScriptToAddressesFunc
}

// NewPivXParser returns new PivXParser instance
func NewPivXParser(params *chaincfg.Params, c *btc.Configuration) *PivXParser {
	p := &PivXParser{
		BitcoinLikeParser: btc.NewBitcoinLikeParser(params, c),
		baseparser:        &bchain.BaseParser{},
	}
	p.BitcoinOutputScriptToAddressesFunc = p.OutputScriptToAddressesFunc
	p.OutputScriptToAddressesFunc = p.outputScriptToAddresses
	return p
}

// GetChainParams contains network parameters for the main PivX network
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
func (p *PivXParser) ParseBlock(b []byte) (*bchain.Block, error) {
	r := bytes.NewReader(b)
	w := wire.MsgBlock{}
	h := wire.BlockHeader{}
	err := h.Deserialize(r)
	if err != nil {
		return nil, errors.Annotatef(err, "Deserialize")
	}

	if h.Version > 3 && h.Version < 7 {
		// Skip past AccumulatorCheckpoint (block version 4, 5 and 6)
		r.Seek(32, io.SeekCurrent)
	}

	if h.Version > 7 {
		// Skip new hashFinalSaplingRoot (block version 8 or newer)
		r.Seek(32, io.SeekCurrent)
	}

	err = p.PivxDecodeTransactions(r, 0, &w)
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
func (p *PivXParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
	return p.baseparser.PackTx(tx, height, blockTime)
}

// UnpackTx unpacks transaction from protobuf byte array
func (p *PivXParser) UnpackTx(buf []byte) (*bchain.Tx, uint32, error) {
	return p.baseparser.UnpackTx(buf)
}

// ParseTx parses byte array containing transaction and returns Tx struct
func (p *PivXParser) ParseTx(b []byte) (*bchain.Tx, error) {
	t := wire.MsgTx{}
	r := bytes.NewReader(b)
	if err := t.Deserialize(r); err != nil {
		return nil, err
	}
	tx := p.TxFromMsgTx(&t, true)
	tx.Hex = hex.EncodeToString(b)
	return &tx, nil
}

// TxFromMsgTx parses tx and adds handling for OP_ZEROCOINSPEND inputs
func (p *PivXParser) TxFromMsgTx(t *wire.MsgTx, parseAddresses bool) bchain.Tx {
	vin := make([]bchain.Vin, len(t.TxIn))
	for i, in := range t.TxIn {

		// extra check to not confuse Tx with single OP_ZEROCOINSPEND input as a coinbase Tx
		if !isZeroCoinSpendScript(in.SignatureScript) && blockchain.IsCoinBaseTx(t) {
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

		txid := in.PreviousOutPoint.Hash.String()

		vin[i] = bchain.Vin{
			Txid:      txid,
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
		Vin:      vin,
		Vout:     vout,
		// skip: BlockHash,
		// skip: Confirmations,
		// skip: Time,
		// skip: Blocktime,
	}
	return tx
}

// ParseTxFromJson parses JSON message containing transaction and returns Tx struct
func (p *PivXParser) ParseTxFromJson(msg json.RawMessage) (*bchain.Tx, error) {
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
func (p *PivXParser) outputScriptToAddresses(script []byte) ([]string, bool, error) {
	if isZeroCoinSpendScript(script) {
		return []string{"Zerocoin Spend"}, false, nil
	}
	if isZeroCoinMintScript(script) {
		return []string{"Zerocoin Mint"}, false, nil
	}

	rv, s, _ := p.BitcoinOutputScriptToAddressesFunc(script)
	return rv, s, nil
}

func (p *PivXParser) GetAddrDescForUnknownInput(tx *bchain.Tx, input int) bchain.AddressDescriptor {
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

func (p *PivXParser) PivxDecodeTransactions(r *bytes.Reader, pver uint32, blk *wire.MsgBlock) error {
	maxTxPerBlock := uint64((wire.MaxBlockPayload / 10) + 1)

	txCount, err := wire.ReadVarInt(r, pver)
	if err != nil {
		return err
	}

	// Prevent more transactions than could possibly fit into a block.
	// It would be possible to cause memory exhaustion and panics without
	// a sane upper bound on this count.
	if txCount > maxTxPerBlock {
		str := fmt.Sprintf("too many transactions to fit into a block "+
			"[count %d, max %d]", txCount, maxTxPerBlock)
		return &wire.MessageError{Func: "utils.decodeTransactions", Description: str}
	}

	blk.Transactions = make([]*wire.MsgTx, 0, txCount)
	for i := uint64(0); i < txCount; i++ {
		tx := wire.MsgTx{}

		// read version & seek back to original state
		var version uint32 = 0
		if err = binary.Read(r, binary.LittleEndian, &version); err != nil {
			return err
		}
		if _, err = r.Seek(-4, io.SeekCurrent); err != nil {
			return err
		}

		txVersion := version & 0xffff
		enc := wire.WitnessEncoding

		// shielded transactions
		if txVersion >= 3 {
			enc = wire.BaseEncoding
		}

		err := p.PivxDecode(&tx, r, pver, enc)
		if err != nil {
			return err
		}
		blk.Transactions = append(blk.Transactions, &tx)
	}

	return nil
}

func (p *PivXParser) PivxDecode(MsgTx *wire.MsgTx, r *bytes.Reader, pver uint32, enc wire.MessageEncoding) error {
	if err := MsgTx.BtcDecode(r, pver, enc); err != nil {
		return err
	}

	// extra
	version := uint32(MsgTx.Version)
	txVersion := version & 0xffff

	if txVersion >= 3 {
		// valueBalance
		r.Seek(9, io.SeekCurrent)

		vShieldedSpend, err := wire.ReadVarInt(r, 0)
		if err != nil {
			return err
		}
		if vShieldedSpend > 0 {
			r.Seek(int64(vShieldedSpend*384), io.SeekCurrent)
		}

		vShieldOutput, err := wire.ReadVarInt(r, 0)
		if err != nil {
			return err
		}
		if vShieldOutput > 0 {
			r.Seek(int64(vShieldOutput*948), io.SeekCurrent)
		}

		// bindingSig
		r.Seek(64, io.SeekCurrent)
	}

	return nil
}

// Checks if script is OP_ZEROCOINMINT
func isZeroCoinMintScript(signatureScript []byte) bool {
	return len(signatureScript) > 1 && signatureScript[0] == OP_ZEROCOINMINT
}

// Checks if script is OP_ZEROCOINSPEND
func isZeroCoinSpendScript(signatureScript []byte) bool {
	return len(signatureScript) >= 100 && signatureScript[0] == OP_ZEROCOINSPEND
}
