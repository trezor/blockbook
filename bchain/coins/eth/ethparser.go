package eth

import (
	"blockbook/bchain"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"strconv"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/golang/protobuf/proto"
	"github.com/juju/errors"
)

// EthereumParser handle
type EthereumParser struct {
	*bchain.BaseParser
}

// NewEthereumParser returns new EthereumParser instance
func NewEthereumParser() *EthereumParser {
	return &EthereumParser{&bchain.BaseParser{
		BlockAddressesToKeep: 0,
		AmountDecimalPoint:   18,
	}}
}

type rpcHeader struct {
	Hash       string `json:"hash"`
	Difficulty string `json:"difficulty"`
	Number     string `json:"number"`
	Time       string `json:"timestamp"`
	Size       string `json:"size"`
	Nonce      string `json:"nonce"`
}

type rpcTransaction struct {
	AccountNonce     string `json:"nonce"`
	GasPrice         string `json:"gasPrice"`
	GasLimit         string `json:"gas"`
	To               string `json:"to"` // nil means contract creation
	Value            string `json:"value"`
	Payload          string `json:"input"`
	Hash             string `json:"hash"`
	BlockNumber      string `json:"blockNumber"`
	BlockHash        string `json:"blockHash,omitempty"`
	From             string `json:"from"`
	TransactionIndex string `json:"transactionIndex"`
	// Signature values - ignored
	// V string `json:"v"`
	// R string `json:"r"`
	// S string `json:"s"`
}

type rpcLog struct {
	Address ethcommon.Address `json:"address"`
	Topics  []string          `json:"topics"`
	Data    string            `json:"data"`
}

type rpcLogWithTxHash struct {
	rpcLog
	Hash string `json:"transactionHash"`
}

type rpcReceipt struct {
	GasUsed string    `json:"gasUsed"`
	Status  string    `json:"status"`
	Logs    []*rpcLog `json:"logs"`
}

type completeTransaction struct {
	Tx      *rpcTransaction `json:"tx"`
	Receipt *rpcReceipt     `json:"receipt,omitempty"`
}

type rpcBlockTransactions struct {
	Transactions []rpcTransaction `json:"transactions"`
}

type rpcBlockTxids struct {
	Transactions []string `json:"transactions"`
}

func ethNumber(n string) (int64, error) {
	if len(n) > 2 {
		return strconv.ParseInt(n[2:], 16, 64)
	}
	return 0, errors.Errorf("Not a number: '%v'", n)
}

func (p *EthereumParser) ethTxToTx(tx *rpcTransaction, receipt *rpcReceipt, blocktime int64, confirmations uint32, marshallHex bool) (*bchain.Tx, error) {
	txid := tx.Hash
	var (
		fa, ta []string
		err    error
	)
	if len(tx.From) > 2 {
		fa = []string{tx.From}
	}
	if len(tx.To) > 2 {
		ta = []string{tx.To}
	}
	ct := completeTransaction{
		Tx:      tx,
		Receipt: receipt,
	}
	var h string
	if marshallHex {
		// completeTransaction without BlockHash is marshalled and hex encoded to bchain.Tx.Hex
		bh := tx.BlockHash
		tx.BlockHash = ""
		b, err := json.Marshal(ct)
		if err != nil {
			return nil, err
		}
		tx.BlockHash = bh
		h = hex.EncodeToString(b)
	}
	vs, err := hexutil.DecodeBig(tx.Value)
	if err != nil {
		return nil, err
	}
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
				N:        0, // there is always up to one To address
				ValueSat: *vs,
				ScriptPubKey: bchain.ScriptPubKey{
					// Hex
					Addresses: ta,
				},
			},
		},
		CoinSpecificData: ct,
	}, nil
}

// GetAddrDescFromVout returns internal address representation of given transaction output
func (p *EthereumParser) GetAddrDescFromVout(output *bchain.Vout) (bchain.AddressDescriptor, error) {
	if len(output.ScriptPubKey.Addresses) != 1 {
		return nil, bchain.ErrAddressMissing
	}
	return p.GetAddrDescFromAddress(output.ScriptPubKey.Addresses[0])
}

func has0xPrefix(s string) bool {
	return len(s) >= 2 && s[0] == '0' && (s[1]|32) == 'x'
}

// GetAddrDescFromAddress returns internal address representation of given address
func (p *EthereumParser) GetAddrDescFromAddress(address string) (bchain.AddressDescriptor, error) {
	// github.com/ethereum/go-ethereum/common.HexToAddress does not handle address errors, using own decoding
	if has0xPrefix(address) {
		address = address[2:]
	}
	if len(address) == 0 {
		return nil, bchain.ErrAddressMissing
	}
	if len(address)&1 == 1 {
		address = "0" + address
	}
	return hex.DecodeString(address)
}

// GetAddressesFromAddrDesc returns addresses for given address descriptor with flag if the addresses are searchable
func (p *EthereumParser) GetAddressesFromAddrDesc(addrDesc bchain.AddressDescriptor) ([]string, bool, error) {
	return []string{hexutil.Encode(addrDesc)}, true, nil
}

// GetScriptFromAddrDesc returns output script for given address descriptor
func (p *EthereumParser) GetScriptFromAddrDesc(addrDesc bchain.AddressDescriptor) ([]byte, error) {
	return addrDesc, nil
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

// PackTx packs transaction to byte array
func (p *EthereumParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
	b, err := hex.DecodeString(tx.Hex)
	if err != nil {
		return nil, err
	}
	var r completeTransaction
	var n uint64
	err = json.Unmarshal(b, &r)
	if err != nil {
		return nil, err
	}
	pt := &ProtoCompleteTransaction{}
	pt.Tx = &ProtoCompleteTransaction_TxType{}
	if pt.Tx.AccountNonce, err = hexutil.DecodeUint64(r.Tx.AccountNonce); err != nil {
		return nil, errors.Annotatef(err, "AccountNonce %v", r.Tx.AccountNonce)
	}
	if n, err = hexutil.DecodeUint64(r.Tx.BlockNumber); err != nil {
		return nil, errors.Annotatef(err, "BlockNumber %v", r.Tx.BlockNumber)
	}
	pt.BlockNumber = uint32(n)
	pt.BlockTime = uint64(blockTime)
	if pt.Tx.From, err = hexDecode(r.Tx.From); err != nil {
		return nil, errors.Annotatef(err, "From %v", r.Tx.From)
	}
	if pt.Tx.GasLimit, err = hexutil.DecodeUint64(r.Tx.GasLimit); err != nil {
		return nil, errors.Annotatef(err, "GasLimit %v", r.Tx.GasLimit)
	}
	if pt.Tx.Hash, err = hexDecode(r.Tx.Hash); err != nil {
		return nil, errors.Annotatef(err, "Hash %v", r.Tx.Hash)
	}
	if pt.Tx.Payload, err = hexDecode(r.Tx.Payload); err != nil {
		return nil, errors.Annotatef(err, "Payload %v", r.Tx.Payload)
	}
	if pt.Tx.GasPrice, err = hexDecodeBig(r.Tx.GasPrice); err != nil {
		return nil, errors.Annotatef(err, "Price %v", r.Tx.GasPrice)
	}
	// if pt.R, err = hexDecodeBig(r.R); err != nil {
	// 	return nil, errors.Annotatef(err, "R %v", r.R)
	// }
	// if pt.S, err = hexDecodeBig(r.S); err != nil {
	// 	return nil, errors.Annotatef(err, "S %v", r.S)
	// }
	// if pt.V, err = hexDecodeBig(r.V); err != nil {
	// 	return nil, errors.Annotatef(err, "V %v", r.V)
	// }
	if pt.Tx.To, err = hexDecode(r.Tx.To); err != nil {
		return nil, errors.Annotatef(err, "To %v", r.Tx.To)
	}
	if n, err = hexutil.DecodeUint64(r.Tx.TransactionIndex); err != nil {
		return nil, errors.Annotatef(err, "TransactionIndex %v", r.Tx.TransactionIndex)
	}
	pt.Tx.TransactionIndex = uint32(n)
	if pt.Tx.Value, err = hexDecodeBig(r.Tx.Value); err != nil {
		return nil, errors.Annotatef(err, "Value %v", r.Tx.Value)
	}
	if r.Receipt != nil {
		pt.Receipt = &ProtoCompleteTransaction_ReceiptType{}
		if pt.Receipt.GasUsed, err = hexDecodeBig(r.Receipt.GasUsed); err != nil {
			return nil, errors.Annotatef(err, "GasUsed %v", r.Receipt.GasUsed)
		}
		if pt.Receipt.Status, err = hexDecodeBig(r.Receipt.Status); err != nil {
			return nil, errors.Annotatef(err, "Status %v", r.Receipt.Status)
		}
		ptLogs := make([]*ProtoCompleteTransaction_ReceiptType_LogType, len(r.Receipt.Logs))
		for i, l := range r.Receipt.Logs {
			d, err := hexutil.Decode(l.Data)
			if err != nil {
				return nil, errors.Annotatef(err, "Data cannot be decoded %v", l)
			}
			t := make([][]byte, len(l.Topics))
			for j, s := range l.Topics {
				t[j], err = hexutil.Decode(s)
				if err != nil {
					return nil, errors.Annotatef(err, "Topic cannot be decoded %v", l)
				}
			}
			ptLogs[i] = &ProtoCompleteTransaction_ReceiptType_LogType{
				Address: l.Address.Bytes(),
				Data:    d,
				Topics:  t,
			}

		}
		pt.Receipt.Log = ptLogs
	}
	return proto.Marshal(pt)
}

// UnpackTx unpacks transaction from byte array
func (p *EthereumParser) UnpackTx(buf []byte) (*bchain.Tx, uint32, error) {
	var pt ProtoCompleteTransaction
	err := proto.Unmarshal(buf, &pt)
	if err != nil {
		return nil, 0, err
	}
	rt := rpcTransaction{
		AccountNonce: hexutil.EncodeUint64(pt.Tx.AccountNonce),
		BlockNumber:  hexutil.EncodeUint64(uint64(pt.BlockNumber)),
		From:         hexutil.Encode(pt.Tx.From),
		GasLimit:     hexutil.EncodeUint64(pt.Tx.GasLimit),
		Hash:         hexutil.Encode(pt.Tx.Hash),
		Payload:      hexutil.Encode(pt.Tx.Payload),
		GasPrice:     hexEncodeBig(pt.Tx.GasPrice),
		// R:                hexEncodeBig(pt.R),
		// S:                hexEncodeBig(pt.S),
		// V:                hexEncodeBig(pt.V),
		To:               hexutil.Encode(pt.Tx.To),
		TransactionIndex: hexutil.EncodeUint64(uint64(pt.Tx.TransactionIndex)),
		Value:            hexEncodeBig(pt.Tx.Value),
	}
	var rr *rpcReceipt
	if pt.Receipt != nil {
		logs := make([]*rpcLog, len(pt.Receipt.Log))
		for i, l := range pt.Receipt.Log {
			topics := make([]string, len(l.Topics))
			for j, t := range l.Topics {
				topics[j] = hexutil.Encode(t)
			}
			logs[i] = &rpcLog{
				Address: ethcommon.BytesToAddress(l.Address),
				Data:    hexutil.Encode(l.Data),
				Topics:  topics,
			}
		}
		rr = &rpcReceipt{
			GasUsed: hexEncodeBig(pt.Receipt.GasUsed),
			Status:  hexEncodeBig(pt.Receipt.Status),
			Logs:    logs,
		}
	}
	tx, err := p.ethTxToTx(&rt, rr, int64(pt.BlockTime), 0, true)
	if err != nil {
		return nil, 0, err
	}
	return tx, pt.BlockNumber, nil
}

// PackedTxidLen returns length in bytes of packed txid
func (p *EthereumParser) PackedTxidLen() int {
	return 32
}

// PackTxid packs txid to byte array
func (p *EthereumParser) PackTxid(txid string) ([]byte, error) {
	if has0xPrefix(txid) {
		txid = txid[2:]
	}
	return hex.DecodeString(txid)
}

// UnpackTxid unpacks byte array to txid
func (p *EthereumParser) UnpackTxid(buf []byte) (string, error) {
	return hexutil.Encode(buf), nil
}

// PackBlockHash packs block hash to byte array
func (p *EthereumParser) PackBlockHash(hash string) ([]byte, error) {
	if has0xPrefix(hash) {
		hash = hash[2:]
	}
	return hex.DecodeString(hash)
}

// UnpackBlockHash unpacks byte array to block hash
func (p *EthereumParser) UnpackBlockHash(buf []byte) (string, error) {
	return hexutil.Encode(buf), nil
}

// GetChainType returns EthereumType
func (p *EthereumParser) GetChainType() bchain.ChainType {
	return bchain.ChainEthereumType
}

// GetHeightFromTx returns ethereum specific data from bchain.Tx
func GetHeightFromTx(tx *bchain.Tx) (uint32, error) {
	// TODO -  temporary implementation - will use bchain.Tx.SpecificData field
	b, err := hex.DecodeString(tx.Hex)
	if err != nil {
		return 0, err
	}
	var ct completeTransaction
	var n uint64
	err = json.Unmarshal(b, &ct)
	if err != nil {
		return 0, err
	}
	if n, err = hexutil.DecodeUint64(ct.Tx.BlockNumber); err != nil {
		return 0, errors.Annotatef(err, "BlockNumber %v", ct.Tx.BlockNumber)
	}
	return uint32(n), nil
}
