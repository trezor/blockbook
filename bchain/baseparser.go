package bchain

import (
	"encoding/hex"
	"encoding/json"
	"math/big"
	"strings"

	"github.com/gogo/protobuf/proto"
	"github.com/juju/errors"
)

type AddressFactoryFunc func(string) (Address, error)

// BaseParser implements data parsing/handling functionality base for all other parsers
type BaseParser struct {
	AddressFactory       AddressFactoryFunc
	BlockAddressesToKeep int
	AmountDecimalPoint   int
}

// AddressToOutputScript converts address to ScriptPubKey - currently not implemented
func (p *BaseParser) AddressToOutputScript(address string) ([]byte, error) {
	return nil, errors.New("AddressToOutputScript: not implemented")
}

// OutputScriptToAddresses converts ScriptPubKey to addresses - currently not implemented
func (p *BaseParser) OutputScriptToAddresses(script []byte) ([]string, error) {
	return nil, errors.New("OutputScriptToAddresses: not implemented")
}

// ParseBlock parses raw block to our Block struct - currently not implemented
func (p *BaseParser) ParseBlock(b []byte) (*Block, error) {
	return nil, errors.New("ParseBlock: not implemented")
}

// ParseTx parses byte array containing transaction and returns Tx struct - currently not implemented
func (p *BaseParser) ParseTx(b []byte) (*Tx, error) {
	return nil, errors.New("ParseTx: not implemented")
}

const zeros = "0000000000000000000000000000000000000000"

// AmountToBigInt converts amount in json.Number (string) to big.Int
// it uses string operations to avoid problems with rounding
func (p *BaseParser) AmountToBigInt(n json.Number) (big.Int, error) {
	var r big.Int
	s := string(n)
	i := strings.IndexByte(s, '.')
	if i == -1 {
		s = s + zeros[:p.AmountDecimalPoint]
	} else {
		z := p.AmountDecimalPoint - len(s) + i + 1
		if z > 0 {
			s = s[:i] + s[i+1:] + zeros[:z]
		} else {
			s = s[:i] + s[i+1:len(s)+z]
		}
	}
	if _, ok := r.SetString(s, 10); !ok {
		return r, errors.New("AmountToBigInt: failed to convert")
	}
	return r, nil
}

// AmountToDecimalString converts amount in big.Int to string with decimal point in the correct place
func (p *BaseParser) AmountToDecimalString(a *big.Int) string {
	n := a.String()
	var s string
	if n[0] == '-' {
		n = n[1:]
		s = "-"
	}
	if len(n) <= p.AmountDecimalPoint {
		n = zeros[:p.AmountDecimalPoint-len(n)+1] + n
	}
	i := len(n) - p.AmountDecimalPoint
	ad := strings.TrimRight(n[i:], "0")
	if len(ad) > 0 {
		n = n[:i] + "." + ad
	} else {
		n = n[:i]
	}
	return s + n
}

// ParseTxFromJson parses JSON message containing transaction and returns Tx struct
func (p *BaseParser) ParseTxFromJson(msg json.RawMessage) (*Tx, error) {
	var tx Tx
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
		if len(vout.ScriptPubKey.Addresses) == 1 {
			a, err := p.AddressFactory(vout.ScriptPubKey.Addresses[0])
			if err != nil {
				return nil, err
			}
			vout.Address = a
		}
	}

	return &tx, nil
}

// PackedTxidLen returns length in bytes of packed txid
func (p *BaseParser) PackedTxidLen() int {
	return 32
}

// KeepBlockAddresses returns number of blocks which are to be kept in blockaddresses column
func (p *BaseParser) KeepBlockAddresses() int {
	return p.BlockAddressesToKeep
}

// PackTxid packs txid to byte array
func (p *BaseParser) PackTxid(txid string) ([]byte, error) {
	if txid == "" {
		return nil, ErrTxidMissing
	}
	return hex.DecodeString(txid)
}

// UnpackTxid unpacks byte array to txid
func (p *BaseParser) UnpackTxid(buf []byte) (string, error) {
	return hex.EncodeToString(buf), nil
}

// PackBlockHash packs block hash to byte array
func (p *BaseParser) PackBlockHash(hash string) ([]byte, error) {
	return hex.DecodeString(hash)
}

// UnpackBlockHash unpacks byte array to block hash
func (p *BaseParser) UnpackBlockHash(buf []byte) (string, error) {
	return hex.EncodeToString(buf), nil
}

// IsUTXOChain returns true if the block chain is UTXO type, otherwise false
func (p *BaseParser) IsUTXOChain() bool {
	return true
}

// PackTx packs transaction to byte array using protobuf
func (p *BaseParser) PackTx(tx *Tx, height uint32, blockTime int64) ([]byte, error) {
	var err error
	pti := make([]*ProtoTransaction_VinType, len(tx.Vin))
	for i, vi := range tx.Vin {
		hex, err := hex.DecodeString(vi.ScriptSig.Hex)
		if err != nil {
			return nil, errors.Annotatef(err, "Vin %v Hex %v", i, vi.ScriptSig.Hex)
		}
		itxid, err := p.PackTxid(vi.Txid)
		if err != nil {
			return nil, errors.Annotatef(err, "Vin %v Txid %v", i, vi.Txid)
		}
		pti[i] = &ProtoTransaction_VinType{
			Addresses:    vi.Addresses,
			Coinbase:     vi.Coinbase,
			ScriptSigHex: hex,
			Sequence:     vi.Sequence,
			Txid:         itxid,
			Vout:         vi.Vout,
		}
	}
	pto := make([]*ProtoTransaction_VoutType, len(tx.Vout))
	for i, vo := range tx.Vout {
		hex, err := hex.DecodeString(vo.ScriptPubKey.Hex)
		if err != nil {
			return nil, errors.Annotatef(err, "Vout %v Hex %v", i, vo.ScriptPubKey.Hex)
		}
		pto[i] = &ProtoTransaction_VoutType{
			Addresses:       vo.ScriptPubKey.Addresses,
			N:               vo.N,
			ScriptPubKeyHex: hex,
			ValueSat:        vo.ValueSat.Bytes(),
		}
	}
	pt := &ProtoTransaction{
		Blocktime: uint64(blockTime),
		Height:    height,
		Locktime:  tx.LockTime,
		Vin:       pti,
		Vout:      pto,
	}
	if pt.Hex, err = hex.DecodeString(tx.Hex); err != nil {
		return nil, errors.Annotatef(err, "Hex %v", tx.Hex)
	}
	if pt.Txid, err = p.PackTxid(tx.Txid); err != nil {
		return nil, errors.Annotatef(err, "Txid %v", tx.Txid)
	}
	return proto.Marshal(pt)
}

// UnpackTx unpacks transaction from protobuf byte array
func (p *BaseParser) UnpackTx(buf []byte) (*Tx, uint32, error) {
	var pt ProtoTransaction
	err := proto.Unmarshal(buf, &pt)
	if err != nil {
		return nil, 0, err
	}
	txid, err := p.UnpackTxid(pt.Txid)
	if err != nil {
		return nil, 0, err
	}
	vin := make([]Vin, len(pt.Vin))
	for i, pti := range pt.Vin {
		itxid, err := p.UnpackTxid(pti.Txid)
		if err != nil {
			return nil, 0, err
		}
		vin[i] = Vin{
			Addresses: pti.Addresses,
			Coinbase:  pti.Coinbase,
			ScriptSig: ScriptSig{
				Hex: hex.EncodeToString(pti.ScriptSigHex),
			},
			Sequence: pti.Sequence,
			Txid:     itxid,
			Vout:     pti.Vout,
		}
	}
	vout := make([]Vout, len(pt.Vout))
	for i, pto := range pt.Vout {
		var vs big.Int
		vs.SetBytes(pto.ValueSat)
		vout[i] = Vout{
			N: pto.N,
			ScriptPubKey: ScriptPubKey{
				Addresses: pto.Addresses,
				Hex:       hex.EncodeToString(pto.ScriptPubKeyHex),
			},
			ValueSat: vs,
		}
		if len(pto.Addresses) == 1 {
			a, err := p.AddressFactory(pto.Addresses[0])
			if err != nil {
				return nil, 0, err
			}
			vout[i].Address = a
		}
	}
	tx := Tx{
		Blocktime: int64(pt.Blocktime),
		Hex:       hex.EncodeToString(pt.Hex),
		LockTime:  pt.Locktime,
		Time:      int64(pt.Blocktime),
		Txid:      txid,
		Vin:       vin,
		Vout:      vout,
	}
	return &tx, pt.Height, nil
}

type baseAddress struct {
	addr string
}

func NewBaseAddress(addr string) (Address, error) {
	return &baseAddress{addr: addr}, nil
}

func (a baseAddress) String() string {
	return a.addr
}

func (a baseAddress) AreEqual(addr string) bool {
	return a.String() == addr
}

func (a baseAddress) InSlice(addrs []string) bool {
	ea := a.String()
	for _, addr := range addrs {
		if ea == addr {
			return true
		}
	}
	return false
}
