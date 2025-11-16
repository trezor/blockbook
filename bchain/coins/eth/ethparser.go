package eth

import (
	"encoding/hex"
	"math/big"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"golang.org/x/crypto/sha3"
	"google.golang.org/protobuf/proto"
)

// EthereumTypeAddressDescriptorLen - the AddressDescriptor of EthereumType has fixed length
const EthereumTypeAddressDescriptorLen = 20

// EthereumTypeTxidLen - the length of Txid
const EthereumTypeTxidLen = 32

// EtherAmountDecimalPoint defines number of decimal points in Ether amounts
const EtherAmountDecimalPoint = 18

// EthereumParser handle
type EthereumParser struct {
	*bchain.BaseParser
	EnsSuffix string
}

// NewEthereumParser returns new EthereumParser instance
func NewEthereumParser(b int, addressAliases bool) *EthereumParser {
	return &EthereumParser{
		BaseParser: &bchain.BaseParser{
			BlockAddressesToKeep: b,
			AmountDecimalPoint:   EtherAmountDecimalPoint,
			AddressAliases:       addressAliases,
		},
		EnsSuffix: ".eth",
	}
}

type rpcHeader struct {
	Hash       string `json:"hash"`
	ParentHash string `json:"parentHash"`
	Difficulty string `json:"difficulty"`
	Number     string `json:"number"`
	Time       string `json:"timestamp"`
	Size       string `json:"size"`
	Nonce      string `json:"nonce"`
}

type rpcLogWithTxHash struct {
	bchain.RpcLog
	Hash string `json:"transactionHash"`
}

type rpcBlockTransactions struct {
	Transactions []bchain.RpcTransaction `json:"transactions"`
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

func (p *EthereumParser) ethTxToTx(tx *bchain.RpcTransaction, receipt *bchain.RpcReceipt, internalData *bchain.EthereumInternalData, blocktime int64, confirmations uint32, fixEIP55 bool) (*bchain.Tx, error) {
	txid := tx.Hash
	var (
		fa, ta []string
		err    error
	)
	if len(tx.From) > 2 {
		if fixEIP55 {
			tx.From = EIP55AddressFromAddress(tx.From)
		}
		fa = []string{tx.From}
	}
	if len(tx.To) > 2 {
		if fixEIP55 {
			tx.To = EIP55AddressFromAddress(tx.To)
		}
		ta = []string{tx.To}
	}
	if fixEIP55 && receipt != nil && receipt.Logs != nil {
		for _, l := range receipt.Logs {
			if len(l.Address) > 2 {
				l.Address = EIP55AddressFromAddress(l.Address)
			}
		}
	}
	if internalData != nil {
		// ignore empty internal data
		if internalData.Type == bchain.CALL && len(internalData.Transfers) == 0 && len(internalData.Error) == 0 {
			internalData = nil
		} else {
			if fixEIP55 {
				for i := range internalData.Transfers {
					it := &internalData.Transfers[i]
					it.From = EIP55AddressFromAddress(it.From)
					it.To = EIP55AddressFromAddress(it.To)
				}
			}
		}
	}
	ct := bchain.EthereumSpecificData{
		Tx:           tx,
		InternalData: internalData,
		Receipt:      receipt,
	}
	vs, err := hexutil.DecodeBig(tx.Value)
	if err != nil {
		return nil, err
	}
	return &bchain.Tx{
		Blocktime:     blocktime,
		Confirmations: confirmations,
		// Hex
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
	if len(address) != EthereumTypeAddressDescriptorLen*2 {
		return nil, bchain.ErrAddressMissing
	}
	return hex.DecodeString(address)
}

// EIP55Address returns an EIP55-compliant hex string representation of the address
func EIP55Address(addrDesc bchain.AddressDescriptor) string {
	raw := hexutil.Encode(addrDesc)
	if len(raw) != 42 {
		return raw
	}
	sha := sha3.NewLegacyKeccak256()
	result := []byte(raw)
	sha.Write(result[2:])
	hash := sha.Sum(nil)

	for i := 2; i < len(result); i++ {
		hashByte := hash[(i-2)>>1]
		if i%2 == 0 {
			hashByte = hashByte >> 4
		} else {
			hashByte &= 0xf
		}
		if result[i] > '9' && hashByte > 7 {
			result[i] -= 32
		}
	}
	return string(result)
}

// EIP55AddressFromAddress returns an EIP55-compliant hex string representation of the address
func EIP55AddressFromAddress(address string) string {
	if has0xPrefix(address) {
		address = address[2:]
	}
	b, err := hex.DecodeString(address)
	if err != nil {
		return address
	}
	return EIP55Address(b)
}

// GetAddressesFromAddrDesc returns addresses for given address descriptor with flag if the addresses are searchable
func (p *EthereumParser) GetAddressesFromAddrDesc(addrDesc bchain.AddressDescriptor) ([]string, bool, error) {
	return []string{EIP55Address(addrDesc)}, true, nil
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
// completeTransaction.InternalData are not packed, they are stored in a different table
func (p *EthereumParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
	var err error
	var n uint64
	r, ok := tx.CoinSpecificData.(bchain.EthereumSpecificData)
	if !ok {
		return nil, errors.New("Missing CoinSpecificData")
	}
	pt := &ProtoCompleteTransaction{}
	pt.Tx = &ProtoCompleteTransaction_TxType{}
	if pt.Tx.AccountNonce, err = hexutil.DecodeUint64(r.Tx.AccountNonce); err != nil {
		return nil, errors.Annotatef(err, "AccountNonce %v", r.Tx.AccountNonce)
	}
	// pt.BlockNumber = height
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
	if len(r.Tx.MaxPriorityFeePerGas) > 0 {
		if pt.Tx.MaxPriorityFeePerGas, err = hexDecodeBig(r.Tx.MaxPriorityFeePerGas); err != nil {
			return nil, errors.Annotatef(err, "MaxPriorityFeePerGas %v", r.Tx.MaxPriorityFeePerGas)
		}
	}
	if len(r.Tx.MaxFeePerGas) > 0 {
		if pt.Tx.MaxFeePerGas, err = hexDecodeBig(r.Tx.MaxFeePerGas); err != nil {
			return nil, errors.Annotatef(err, "MaxFeePerGas %v", r.Tx.MaxFeePerGas)
		}
	}
	if len(r.Tx.BaseFeePerGas) > 0 {
		if pt.Tx.BaseFeePerGas, err = hexDecodeBig(r.Tx.BaseFeePerGas); err != nil {
			return nil, errors.Annotatef(err, "BaseFeePerGas %v", r.Tx.BaseFeePerGas)
		}
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
		if r.Receipt.Status != "" {
			if pt.Receipt.Status, err = hexDecodeBig(r.Receipt.Status); err != nil {
				return nil, errors.Annotatef(err, "Status %v", r.Receipt.Status)
			}
		} else {
			// unknown status, use 'U' as status bytes
			// there is a potential for conflict with value 0x55 but this is not used by any chain at this moment
			pt.Receipt.Status = []byte{'U'}
		}
		ptLogs := make([]*ProtoCompleteTransaction_ReceiptType_LogType, len(r.Receipt.Logs))
		for i, l := range r.Receipt.Logs {
			a, err := hexutil.Decode(l.Address)
			if err != nil {
				return nil, errors.Annotatef(err, "Address cannot be decoded %v", l)
			}
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
				Address: a,
				Data:    d,
				Topics:  t,
			}

		}
		pt.Receipt.Log = ptLogs
		if r.Receipt.L1Fee != "" {
			if pt.Receipt.L1Fee, err = hexDecodeBig(r.Receipt.L1Fee); err != nil {
				return nil, errors.Annotatef(err, "L1Fee %v", r.Receipt.L1Fee)
			}
		}
		if r.Receipt.L1FeeScalar != "" {
			pt.Receipt.L1FeeScalar = []byte(r.Receipt.L1FeeScalar)
		}
		if r.Receipt.L1GasPrice != "" {
			if pt.Receipt.L1GasPrice, err = hexDecodeBig(r.Receipt.L1GasPrice); err != nil {
				return nil, errors.Annotatef(err, "L1GasPrice %v", r.Receipt.L1GasPrice)
			}
		}
		if r.Receipt.L1GasUsed != "" {
			if pt.Receipt.L1GasUsed, err = hexDecodeBig(r.Receipt.L1GasUsed); err != nil {
				return nil, errors.Annotatef(err, "L1GasUsed %v", r.Receipt.L1GasUsed)
			}
		}
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
	rt := bchain.RpcTransaction{
		AccountNonce: hexutil.EncodeUint64(pt.Tx.AccountNonce),
		BlockNumber:  hexutil.EncodeUint64(uint64(pt.BlockNumber)),
		From:         EIP55Address(pt.Tx.From),
		GasLimit:     hexutil.EncodeUint64(pt.Tx.GasLimit),
		Hash:         hexutil.Encode(pt.Tx.Hash),
		Payload:      hexutil.Encode(pt.Tx.Payload),
		GasPrice:     hexEncodeBig(pt.Tx.GasPrice),
		// R:                hexEncodeBig(pt.R),
		// S:                hexEncodeBig(pt.S),
		// V:                hexEncodeBig(pt.V),
		To:               EIP55Address(pt.Tx.To),
		TransactionIndex: hexutil.EncodeUint64(uint64(pt.Tx.TransactionIndex)),
		Value:            hexEncodeBig(pt.Tx.Value),
	}
	if len(pt.Tx.MaxPriorityFeePerGas) > 0 {
		rt.MaxPriorityFeePerGas = hexEncodeBig(pt.Tx.MaxPriorityFeePerGas)
	}
	if len(pt.Tx.MaxFeePerGas) > 0 {
		rt.MaxFeePerGas = hexEncodeBig(pt.Tx.MaxFeePerGas)
	}
	if len(pt.Tx.BaseFeePerGas) > 0 {
		rt.BaseFeePerGas = hexEncodeBig(pt.Tx.BaseFeePerGas)
	}
	var rr *bchain.RpcReceipt
	if pt.Receipt != nil {
		rr = &bchain.RpcReceipt{
			GasUsed: hexEncodeBig(pt.Receipt.GasUsed),
			Status:  "",
			Logs:    make([]*bchain.RpcLog, len(pt.Receipt.Log)),
		}
		for i, l := range pt.Receipt.Log {
			topics := make([]string, len(l.Topics))
			for j, t := range l.Topics {
				topics[j] = hexutil.Encode(t)
			}
			rr.Logs[i] = &bchain.RpcLog{
				Address: EIP55Address(l.Address),
				Data:    hexutil.Encode(l.Data),
				Topics:  topics,
			}
		}
		// handle a special value []byte{'U'} as unknown state
		if len(pt.Receipt.Status) != 1 || pt.Receipt.Status[0] != 'U' {
			rr.Status = hexEncodeBig(pt.Receipt.Status)
		}
		if len(pt.Receipt.L1Fee) > 0 {
			rr.L1Fee = hexEncodeBig(pt.Receipt.L1Fee)
		}
		if len(pt.Receipt.L1FeeScalar) > 0 {
			rr.L1FeeScalar = string(pt.Receipt.L1FeeScalar)
		}
		if len(pt.Receipt.L1GasPrice) > 0 {
			rr.L1GasPrice = hexEncodeBig(pt.Receipt.L1GasPrice)
		}
		if len(pt.Receipt.L1GasUsed) > 0 {
			rr.L1GasUsed = hexEncodeBig(pt.Receipt.L1GasUsed)
		}
	}
	// TODO handle internal transactions
	tx, err := p.ethTxToTx(&rt, rr, nil, int64(pt.BlockTime), 0, false)
	if err != nil {
		return nil, 0, err
	}
	return tx, pt.BlockNumber, nil
}

// PackedTxidLen returns length in bytes of packed txid
func (p *EthereumParser) PackedTxidLen() int {
	return EthereumTypeTxidLen
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
	var bn string
	csd, ok := tx.CoinSpecificData.(bchain.EthereumSpecificData)
	if !ok {
		return 0, errors.New("Missing CoinSpecificData")
	}
	bn = csd.Tx.BlockNumber
	n, err := hexutil.DecodeUint64(bn)
	if err != nil {
		return 0, errors.Annotatef(err, "BlockNumber %v", bn)
	}
	return uint32(n), nil
}

// EthereumTypeGetTokenTransfersFromTx returns contract transfers from bchain.Tx
func (p *EthereumParser) EthereumTypeGetTokenTransfersFromTx(tx *bchain.Tx) (bchain.TokenTransfers, error) {
	var r bchain.TokenTransfers
	var err error
	csd, ok := tx.CoinSpecificData.(bchain.EthereumSpecificData)
	if ok {
		if csd.Receipt != nil {
			r, err = contractGetTransfersFromLog(csd.Receipt.Logs)
		} else {
			r, err = contractGetTransfersFromTx(csd.Tx)
		}
		if err != nil {
			return nil, err
		}
	}
	return r, nil
}

// FormatAddressAlias adds .eth to a name alias
func (p *EthereumParser) FormatAddressAlias(address string, name string) string {
	return name + p.EnsSuffix
}

// TxStatus is status of transaction
type TxStatus int

// statuses of transaction
const (
	TxStatusUnknown = TxStatus(iota - 2)
	TxStatusPending
	TxStatusFailure
	TxStatusOK
)

// EthereumTxData contains ethereum specific transaction data
type EthereumTxData struct {
	Status               TxStatus `json:"status"` // 1 OK, 0 Fail, -1 pending, -2 unknown
	Nonce                uint64   `json:"nonce"`
	GasLimit             *big.Int `json:"gaslimit"`
	GasUsed              *big.Int `json:"gasused"`
	GasPrice             *big.Int `json:"gasprice"`
	MaxPriorityFeePerGas *big.Int `json:"maxPriorityFeePerGas,omitempty"`
	MaxFeePerGas         *big.Int `json:"maxFeePerGas,omitempty"`
	BaseFeePerGas        *big.Int `json:"baseFeePerGas,omitempty"`
	L1Fee                *big.Int `json:"l1Fee,omitempty"`
	L1FeeScalar          string   `json:"l1FeeScalar,omitempty"`
	L1GasPrice           *big.Int `json:"l1GasPrice,omitempty"`
	L1GasUsed            *big.Int `json:"L1GasUsed,omitempty"`
	Data                 string   `json:"data"`
}

// GetEthereumTxData returns EthereumTxData from bchain.Tx
func GetEthereumTxData(tx *bchain.Tx) *EthereumTxData {
	return GetEthereumTxDataFromSpecificData(tx.CoinSpecificData)
}

// GetEthereumTxDataFromSpecificData returns EthereumTxData from coinSpecificData
func GetEthereumTxDataFromSpecificData(coinSpecificData interface{}) *EthereumTxData {
	etd := EthereumTxData{Status: TxStatusPending}
	csd, ok := coinSpecificData.(bchain.EthereumSpecificData)
	if ok {
		if csd.Tx != nil {
			etd.Nonce, _ = hexutil.DecodeUint64(csd.Tx.AccountNonce)
			etd.GasLimit, _ = hexutil.DecodeBig(csd.Tx.GasLimit)
			etd.GasPrice, _ = hexutil.DecodeBig(csd.Tx.GasPrice)
			etd.MaxPriorityFeePerGas, _ = hexutil.DecodeBig(csd.Tx.MaxPriorityFeePerGas)
			etd.MaxFeePerGas, _ = hexutil.DecodeBig(csd.Tx.MaxFeePerGas)
			etd.BaseFeePerGas, _ = hexutil.DecodeBig(csd.Tx.BaseFeePerGas)
			etd.Data = csd.Tx.Payload
		}
		if csd.Receipt != nil {
			switch csd.Receipt.Status {
			case "0x1":
				etd.Status = TxStatusOK
			case "": // old transactions did not set status
				etd.Status = TxStatusUnknown
			default:
				etd.Status = TxStatusFailure
			}
			etd.GasUsed, _ = hexutil.DecodeBig(csd.Receipt.GasUsed)
			etd.L1Fee, _ = hexutil.DecodeBig(csd.Receipt.L1Fee)
			etd.L1GasPrice, _ = hexutil.DecodeBig(csd.Receipt.L1GasPrice)
			etd.L1GasUsed, _ = hexutil.DecodeBig(csd.Receipt.L1GasUsed)
			etd.L1FeeScalar = csd.Receipt.L1FeeScalar
		}
	}
	return &etd
}

const errorOutputSignature = "08c379a0"

// ParseErrorFromOutput takes output field from internal transaction data and extracts an error message from it
// the output must have errorOutputSignature to be parsed
func ParseErrorFromOutput(output string) string {
	if has0xPrefix(output) {
		output = output[2:]
	}
	if len(output) < 8+64+64+64 || output[:8] != errorOutputSignature {
		return ""
	}
	return parseSimpleStringProperty(output[8:])
}

// PackInternalTransactionError packs common error messages to single byte to save DB space
func PackInternalTransactionError(e string) string {
	if e == "execution reverted" {
		return "\x01"
	}
	if e == "out of gas" {
		return "\x02"
	}
	if e == "contract creation code storage out of gas" {
		return "\x03"
	}
	if e == "max code size exceeded" {
		return "\x04"
	}

	return e
}

// UnpackInternalTransactionError unpacks common error messages packed by PackInternalTransactionError
func UnpackInternalTransactionError(data []byte) string {
	e := string(data)
	e = strings.ReplaceAll(e, "\x01", "Reverted. ")
	e = strings.ReplaceAll(e, "\x02", "Out of gas. ")
	e = strings.ReplaceAll(e, "\x03", "Contract creation code storage out of gas. ")
	e = strings.ReplaceAll(e, "\x04", "Max code size exceeded. ")
	return strings.TrimSpace(e)
}
