package eth

import (
	"encoding/hex"
	"math/big"
	"strconv"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/golang/protobuf/proto"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"golang.org/x/crypto/sha3"
)

// EthereumTypeAddressDescriptorLen - in case of EthereumType, the AddressDescriptor has fixed length
const EthereumTypeAddressDescriptorLen = 20

// EtherAmountDecimalPoint defines number of decimal points in Ether amounts
const EtherAmountDecimalPoint = 18

// EthereumParser handle
type EthereumParser struct {
	*bchain.BaseParser
}

// NewEthereumParser returns new EthereumParser instance
func NewEthereumParser(b int) *EthereumParser {
	return &EthereumParser{&bchain.BaseParser{
		BlockAddressesToKeep: b,
		AmountDecimalPoint:   EtherAmountDecimalPoint,
	}}
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
	Address string   `json:"address"`
	Topics  []string `json:"topics"`
	Data    string   `json:"data"`
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

func (p *EthereumParser) ethTxToTx(tx *rpcTransaction, receipt *rpcReceipt, blocktime int64, confirmations uint32, fixEIP55 bool) (*bchain.Tx, error) {
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
	ct := completeTransaction{
		Tx:      tx,
		Receipt: receipt,
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
func (p *EthereumParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
	var err error
	var n uint64
	r, ok := tx.CoinSpecificData.(completeTransaction)
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
	var rr *rpcReceipt
	if pt.Receipt != nil {
		logs := make([]*rpcLog, len(pt.Receipt.Log))
		for i, l := range pt.Receipt.Log {
			topics := make([]string, len(l.Topics))
			for j, t := range l.Topics {
				topics[j] = hexutil.Encode(t)
			}
			logs[i] = &rpcLog{
				Address: EIP55Address(l.Address),
				Data:    hexutil.Encode(l.Data),
				Topics:  topics,
			}
		}
		status := ""
		// handle a special value []byte{'U'} as unknown state
		if len(pt.Receipt.Status) != 1 || pt.Receipt.Status[0] != 'U' {
			status = hexEncodeBig(pt.Receipt.Status)
		}
		rr = &rpcReceipt{
			GasUsed: hexEncodeBig(pt.Receipt.GasUsed),
			Status:  status,
			Logs:    logs,
		}
	}
	tx, err := p.ethTxToTx(&rt, rr, int64(pt.BlockTime), 0, false)
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
	var bn string
	csd, ok := tx.CoinSpecificData.(completeTransaction)
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

// EthereumTypeGetErc20FromTx returns Erc20 data from bchain.Tx
func (p *EthereumParser) EthereumTypeGetErc20FromTx(tx *bchain.Tx) ([]bchain.Erc20Transfer, error) {
	var r []bchain.Erc20Transfer
	var err error
	csd, ok := tx.CoinSpecificData.(completeTransaction)
	if ok {
		if csd.Receipt != nil {
			r, err = erc20GetTransfersFromLog(csd.Receipt.Logs)
		} else {
			r, err = erc20GetTransfersFromTx(csd.Tx)
		}
		if err != nil {
			return nil, err
		}
	}
	return r, nil
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
	Status   TxStatus `json:"status"` // 1 OK, 0 Fail, -1 pending, -2 unknown
	Nonce    uint64   `json:"nonce"`
	GasLimit *big.Int `json:"gaslimit"`
	GasUsed  *big.Int `json:"gasused"`
	GasPrice *big.Int `json:"gasprice"`
	Data     string   `json:"data"`
}

// GetEthereumTxData returns EthereumTxData from bchain.Tx
func GetEthereumTxData(tx *bchain.Tx) *EthereumTxData {
	return GetEthereumTxDataFromSpecificData(tx.CoinSpecificData)
}

// GetEthereumTxDataFromSpecificData returns EthereumTxData from coinSpecificData
func GetEthereumTxDataFromSpecificData(coinSpecificData interface{}) *EthereumTxData {
	etd := EthereumTxData{Status: TxStatusPending}
	csd, ok := coinSpecificData.(completeTransaction)
	if ok {
		if csd.Tx != nil {
			etd.Nonce, _ = hexutil.DecodeUint64(csd.Tx.AccountNonce)
			etd.GasLimit, _ = hexutil.DecodeBig(csd.Tx.GasLimit)
			etd.GasPrice, _ = hexutil.DecodeBig(csd.Tx.GasPrice)
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
		}
	}
	return &etd
}
