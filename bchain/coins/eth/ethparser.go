package eth

import (
	"blockbook/bchain"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"strconv"

	"github.com/ethereum/go-ethereum/common/hexutil"

	proto "github.com/golang/protobuf/proto"
	"github.com/juju/errors"

	ethcommon "github.com/ethereum/go-ethereum/common"
)

type rpcTransaction struct {
	AccountNonce     string          `json:"nonce"    gencodec:"required"`
	Price            string          `json:"gasPrice" gencodec:"required"`
	GasLimit         string          `json:"gas"      gencodec:"required"`
	To               string          `json:"to"       rlp:"nil"` // nil means contract creation
	Value            string          `json:"value"    gencodec:"required"`
	Payload          string          `json:"input"    gencodec:"required"`
	Hash             ethcommon.Hash  `json:"hash" rlp:"-"`
	BlockNumber      string          `json:"blockNumber"`
	BlockHash        *ethcommon.Hash `json:"blockHash,omitempty"`
	From             string          `json:"from"`
	TransactionIndex string          `json:"transactionIndex"`
	// Signature values
	V string `json:"v" gencodec:"required"`
	R string `json:"r" gencodec:"required"`
	S string `json:"s" gencodec:"required"`
}

type rpcBlock struct {
	Hash         ethcommon.Hash   `json:"hash"`
	Transactions []rpcTransaction `json:"transactions"`
	UncleHashes  []ethcommon.Hash `json:"uncles"`
}

func ethHashToHash(h ethcommon.Hash) string {
	return h.Hex()[2:]
}

func ethNumber(n string) (int64, error) {
	if len(n) > 2 {
		return strconv.ParseInt(n[2:], 16, 64)
	}
	return 0, errors.Errorf("Not a number: '%v'", n)
}

func ethTxToTx(tx *rpcTransaction, blocktime int64, confirmations uint32) (*bchain.Tx, error) {
	txid := ethHashToHash(tx.Hash)
	var fa, ta []string
	if len(tx.From) > 2 {
		fa = []string{tx.From}
	}
	if len(tx.To) > 2 {
		ta = []string{tx.To}
	}
	// temporarily, the complete rpcTransaction without BlockHash is marshalled and hex encoded to bchain.Tx.Hex
	bh := tx.BlockHash
	tx.BlockHash = nil
	b, err := json.Marshal(tx)
	if err != nil {
		return nil, err
	}
	tx.BlockHash = bh
	h := hex.EncodeToString(b)
	return &bchain.Tx{
		Blocktime:     blocktime,
		Confirmations: confirmations,
		Hex:           h,
		// LockTime
		Time: blocktime,
		Txid: txid,
		Vin: []bchain.Vin{
			{
				Addresses: fa,
				// Coinbase
				// ScriptSig
				// Sequence
				// Txid
				// Vout
			},
		},
		Vout: []bchain.Vout{
			{
				N: 0, // there is always up to one To address
				// Value - cannot set, it does not fit precisely to float64
				ScriptPubKey: bchain.ScriptPubKey{
					// Hex
					Addresses: ta,
				},
			},
		},
	}, nil
}

type EthereumParser struct {
}

func (p *EthereumParser) GetAddrIDFromVout(output *bchain.Vout) ([]byte, error) {
	if len(output.ScriptPubKey.Addresses) != 1 {
		return nil, bchain.ErrAddressMissing
	}
	return p.GetAddrIDFromAddress(output.ScriptPubKey.Addresses[0])
}

func (p *EthereumParser) GetAddrIDFromAddress(address string) ([]byte, error) {
	// github.com/ethereum/go-ethereum/common.HexToAddress does not handle address errors, using own decoding
	if len(address) > 1 {
		if address[0:2] == "0x" || address[0:2] == "0X" {
			address = address[2:]
		}
	} else {
		if len(address) == 0 {
			return nil, bchain.ErrAddressMissing
		}
		return nil, errors.Errorf("Invalid address '%v'", address)
	}
	if len(address)&1 == 1 {
		address = "0" + address
	}
	return hex.DecodeString(address)
}

func (p *EthereumParser) AddressToOutputScript(address string) ([]byte, error) {
	return nil, errors.New("AddressToOutputScript: not implemented")
}

func (p *EthereumParser) OutputScriptToAddresses(script []byte) ([]string, error) {
	return nil, errors.New("OutputScriptToAddresses: not implemented")
}

func (p *EthereumParser) ParseTx(b []byte) (*bchain.Tx, error) {
	return nil, errors.New("ParseTx: not implemented")
}

func (p *EthereumParser) ParseBlock(b []byte) (*bchain.Block, error) {
	return nil, errors.New("ParseBlock: not implemented")
}

func hexDecode(s string) ([]byte, error) {
	b, err := hexutil.Decode(s)
	if err != nil && err != hexutil.ErrEmptyString {
		return nil, err
	}
	return b, nil
}

func hexDecodeBig(s string) ([]byte, error) {
	b, err := hexutil.DecodeBig(s)
	if err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func hexEncodeBig(b []byte) string {
	var i big.Int
	i.SetBytes(b)
	return hexutil.EncodeBig(&i)
}

func (p *EthereumParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
	b, err := hex.DecodeString(tx.Hex)
	if err != nil {
		return nil, err
	}
	var r rpcTransaction
	var n uint64
	err = json.Unmarshal(b, &r)
	if err != nil {
		return nil, err
	}
	pt := &ProtoTransaction{}
	if pt.AccountNonce, err = hexutil.DecodeUint64(r.AccountNonce); err != nil {
		return nil, errors.Annotatef(err, "AccountNonce %v", r.AccountNonce)
	}
	if n, err = hexutil.DecodeUint64(r.BlockNumber); err != nil {
		return nil, errors.Annotatef(err, "BlockNumber %v", r.BlockNumber)
	}
	pt.BlockNumber = uint32(n)
	pt.BlockTime = uint64(blockTime)
	if pt.From, err = hexDecode(r.From); err != nil {
		return nil, errors.Annotatef(err, "From %v", r.From)
	}
	if pt.GasLimit, err = hexutil.DecodeUint64(r.GasLimit); err != nil {
		return nil, errors.Annotatef(err, "GasLimit %v", r.GasLimit)
	}
	pt.Hash = r.Hash.Bytes()
	if pt.Payload, err = hexDecode(r.Payload); err != nil {
		return nil, errors.Annotatef(err, "Payload %v", r.Payload)
	}
	if pt.Price, err = hexDecodeBig(r.Price); err != nil {
		return nil, errors.Annotatef(err, "Price %v", r.Price)
	}
	if pt.R, err = hexDecodeBig(r.R); err != nil {
		return nil, errors.Annotatef(err, "R %v", r.R)
	}
	if pt.S, err = hexDecodeBig(r.S); err != nil {
		return nil, errors.Annotatef(err, "S %v", r.S)
	}
	if pt.V, err = hexDecodeBig(r.V); err != nil {
		return nil, errors.Annotatef(err, "V %v", r.V)
	}
	if pt.To, err = hexDecode(r.To); err != nil {
		return nil, errors.Annotatef(err, "To %v", r.To)
	}
	if n, err = hexutil.DecodeUint64(r.TransactionIndex); err != nil {
		return nil, errors.Annotatef(err, "TransactionIndex %v", r.TransactionIndex)
	}
	pt.TransactionIndex = uint32(n)
	if pt.Value, err = hexDecodeBig(r.Value); err != nil {
		return nil, errors.Annotatef(err, "Value %v", r.Value)
	}
	return proto.Marshal(pt)
}

func (p *EthereumParser) UnpackTx(buf []byte) (*bchain.Tx, uint32, error) {
	var pt ProtoTransaction
	err := proto.Unmarshal(buf, &pt)
	if err != nil {
		return nil, 0, err
	}
	r := rpcTransaction{
		AccountNonce:     hexutil.EncodeUint64(pt.AccountNonce),
		BlockNumber:      hexutil.EncodeUint64(uint64(pt.BlockNumber)),
		From:             hexutil.Encode(pt.From),
		GasLimit:         hexutil.EncodeUint64(pt.GasLimit),
		Hash:             ethcommon.BytesToHash(pt.Hash),
		Payload:          hexutil.Encode(pt.Payload),
		Price:            hexEncodeBig(pt.Price),
		R:                hexEncodeBig(pt.R),
		S:                hexEncodeBig(pt.S),
		V:                hexEncodeBig(pt.V),
		To:               hexutil.Encode(pt.To),
		TransactionIndex: hexutil.EncodeUint64(uint64(pt.TransactionIndex)),
		Value:            hexEncodeBig(pt.Value),
	}
	tx, err := ethTxToTx(&r, int64(pt.BlockTime), 0)
	if err != nil {
		return nil, 0, err
	}
	return tx, pt.BlockNumber, nil
}

func (p *EthereumParser) IsUTXOChain() bool {
	return false
}
