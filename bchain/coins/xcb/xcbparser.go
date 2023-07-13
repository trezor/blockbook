package xcb

import (
	"encoding/hex"
	"math/big"
	"strconv"

	xcbcommon "github.com/core-coin/go-core/v2/common"
	"github.com/core-coin/go-core/v2/common/hexutil"
	"github.com/cryptohub-digital/blockbook-fork/bchain"
	"github.com/golang/protobuf/proto"
	"github.com/juju/errors"
)

// CoreCoinTypeAddressDescriptorLen - the AddressDescriptor of Core Coin has fixed length
const CoreCoinTypeAddressDescriptorLen = 22

// CoreCoinTypeTxidLen - the length of Txid
const CoreCoinTypeTxidLen = 32

// CoreAmountDecimalPoint defines number of decimal points in Core amounts
const CoreAmountDecimalPoint = 18

// CoreCoinParser handle
type CoreCoinParser struct {
	*bchain.BaseParser
}

// NewCoreCoinParser returns new CoreCoinParser instance
func NewCoreCoinParser(b int) *CoreCoinParser {
	return &CoreCoinParser{&bchain.BaseParser{
		BlockAddressesToKeep: b,
		AmountDecimalPoint:   CoreAmountDecimalPoint,
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

type rpcLogWithTxHash struct {
	RpcLog
	Hash string `json:"transactionHash"`
}

type rpcBlockTransactions struct {
	Transactions []RpcTransaction `json:"transactions"`
}

type rpcBlockTxids struct {
	Transactions []string `json:"transactions"`
}

func xcbNumber(n string) (int64, error) {
	if len(n) > 2 {
		return strconv.ParseInt(n[2:], 16, 64)
	}
	return 0, errors.Errorf("Not a number: '%v'", n)
}

func (p *CoreCoinParser) xcbTxToTx(tx *RpcTransaction, receipt *RpcReceipt, blocktime int64, confirmations uint32) (*bchain.Tx, error) {
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
	ct := CoreCoinSpecificData{
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
		Time:          blocktime,
		Txid:          txid,
		Vin: []bchain.Vin{
			{
				Addresses: fa,
			},
		},
		Vout: []bchain.Vout{
			{
				N:        0, // there is always up to one To address
				ValueSat: *vs,
				ScriptPubKey: bchain.ScriptPubKey{
					Addresses: ta,
				},
			},
		},
		CoinSpecificData: ct,
	}, nil
}

// GetAddrDescFromVout returns internal address representation of given transaction output
func (p *CoreCoinParser) GetAddrDescFromVout(output *bchain.Vout) (bchain.AddressDescriptor, error) {
	if len(output.ScriptPubKey.Addresses) != 1 {
		return nil, bchain.ErrAddressMissing
	}
	return p.GetAddrDescFromAddress(output.ScriptPubKey.Addresses[0])
}

func has0xPrefix(s string) bool {
	return len(s) >= 2 && s[0] == '0' && (s[1]|32) == 'x'
}

// GetAddrDescFromAddress returns internal address representation of given address
func (p *CoreCoinParser) GetAddrDescFromAddress(address string) (bchain.AddressDescriptor, error) {
	if address == "" {
		return nil, bchain.ErrAddressMissing
	}
	parsed, err := xcbcommon.HexToAddress(address)
	if err != nil {
		return nil, err
	}
	return parsed.Bytes(), nil
}

// GetAddressesFromAddrDesc returns addresse for given address descriptor
func (p *CoreCoinParser) GetAddressesFromAddrDesc(addrDesc bchain.AddressDescriptor) ([]string, bool, error) {
	addr, err := xcbcommon.HexToAddress(xcbcommon.Bytes2Hex(addrDesc))
	if err != nil {
		return []string{}, false, err
	}
	return []string{addr.Hex()}, true, nil
}

// GetScriptFromAddrDesc returns output script for given address descriptor
func (p *CoreCoinParser) GetScriptFromAddrDesc(addrDesc bchain.AddressDescriptor) ([]byte, error) {
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

func decodeAddress(addr []byte) (string, error) {
	address, err := xcbcommon.HexToAddress(xcbcommon.Bytes2Hex(addr))
	if err != nil {
		return "", err
	}
	return address.Hex(), nil
}

// PackTx packs transaction to byte array
// completeTransaction.InternalData are not packed, they are stored in a different table
func (p *CoreCoinParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
	var err error
	var n uint64
	r, ok := tx.CoinSpecificData.(CoreCoinSpecificData)
	if !ok {
		return nil, errors.New("Missing CoinSpecificData")
	}
	pt := &ProtoXCBCompleteTransaction{}
	pt.Tx = &ProtoXCBCompleteTransaction_TxType{}
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
	if pt.Tx.EnergyLimit, err = hexutil.DecodeUint64(r.Tx.EnergyLimit); err != nil {
		return nil, errors.Annotatef(err, "EnergyLimit %v", r.Tx.EnergyLimit)
	}
	if pt.Tx.Hash, err = hexDecode(r.Tx.Hash); err != nil {
		return nil, errors.Annotatef(err, "Hash %v", r.Tx.Hash)
	}
	if pt.Tx.Payload, err = hexDecode(r.Tx.Payload); err != nil {
		return nil, errors.Annotatef(err, "Payload %v", r.Tx.Payload)
	}
	if pt.Tx.EnergyPrice, err = hexDecodeBig(r.Tx.EnergyPrice); err != nil {
		return nil, errors.Annotatef(err, "EnergyPrice %v", r.Tx.EnergyPrice)
	}
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
		pt.Receipt = &ProtoXCBCompleteTransaction_ReceiptType{}
		if pt.Receipt.EnergyUsed, err = hexDecodeBig(r.Receipt.EnergyUsed); err != nil {
			return nil, errors.Annotatef(err, "EnergyUsed %v", r.Receipt.EnergyUsed)
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
		ptLogs := make([]*ProtoXCBCompleteTransaction_ReceiptType_LogType, len(r.Receipt.Logs))
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
			ptLogs[i] = &ProtoXCBCompleteTransaction_ReceiptType_LogType{
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
func (p *CoreCoinParser) UnpackTx(buf []byte) (*bchain.Tx, uint32, error) {
	var pt ProtoXCBCompleteTransaction
	err := proto.Unmarshal(buf, &pt)
	if err != nil {
		return nil, 0, err
	}
	from, err := decodeAddress(pt.Tx.From)
	if err != nil {
		return nil, 0, err
	}

	to, err := decodeAddress(pt.Tx.To)
	if err != nil {
		return nil, 0, err
	}
	rt := RpcTransaction{
		AccountNonce:     hexutil.EncodeUint64(pt.Tx.AccountNonce),
		BlockNumber:      hexutil.EncodeUint64(uint64(pt.BlockNumber)),
		From:             from,
		EnergyLimit:      hexutil.EncodeUint64(pt.Tx.EnergyLimit),
		Hash:             hexutil.Encode(pt.Tx.Hash),
		Payload:          hexutil.Encode(pt.Tx.Payload),
		EnergyPrice:      hexEncodeBig(pt.Tx.EnergyPrice),
		To:               to,
		TransactionIndex: hexutil.EncodeUint64(uint64(pt.Tx.TransactionIndex)),
		Value:            hexEncodeBig(pt.Tx.Value),
	}
	var rr *RpcReceipt
	if pt.Receipt != nil {
		logs := make([]*RpcLog, len(pt.Receipt.Log))
		for i, l := range pt.Receipt.Log {
			topics := make([]string, len(l.Topics))
			for j, t := range l.Topics {
				topics[j] = hexutil.Encode(t)
			}
			address, err := decodeAddress(l.Address)
			if err != nil {
				return nil, 0, err
			}
			logs[i] = &RpcLog{
				Address: address,
				Data:    hexutil.Encode(l.Data),
				Topics:  topics,
			}
		}
		status := ""
		// handle a special value []byte{'U'} as unknown state
		if len(pt.Receipt.Status) != 1 || pt.Receipt.Status[0] != 'U' {
			status = hexEncodeBig(pt.Receipt.Status)
		}
		rr = &RpcReceipt{
			EnergyUsed: hexEncodeBig(pt.Receipt.EnergyUsed),
			Status:     status,
			Logs:       logs,
		}
	}
	tx, err := p.xcbTxToTx(&rt, rr, int64(pt.BlockTime), 0)
	if err != nil {
		return nil, 0, err
	}
	return tx, pt.BlockNumber, nil
}

// PackedTxidLen returns length in bytes of packed txid
func (p *CoreCoinParser) PackedTxidLen() int {
	return CoreCoinTypeTxidLen
}

// PackTxid packs txid to byte array
func (p *CoreCoinParser) PackTxid(txid string) ([]byte, error) {
	if has0xPrefix(txid) {
		txid = txid[2:]
	}
	return hex.DecodeString(txid)
}

// UnpackTxid unpacks byte array to txid
func (p *CoreCoinParser) UnpackTxid(buf []byte) (string, error) {
	return hexutil.Encode(buf), nil
}

// PackBlockHash packs block hash to byte array
func (p *CoreCoinParser) PackBlockHash(hash string) ([]byte, error) {
	if has0xPrefix(hash) {
		hash = hash[2:]
	}
	return hex.DecodeString(hash)
}

// UnpackBlockHash unpacks byte array to block hash
func (p *CoreCoinParser) UnpackBlockHash(buf []byte) (string, error) {
	return hexutil.Encode(buf), nil
}

// GetChainType returns CoreCoinType
func (p *CoreCoinParser) GetChainType() bchain.ChainType {
	return bchain.ChainCoreCoinType
}

// GetHeightFromTx returns core coin specific data from bchain.Tx
func GetHeightFromTx(tx *bchain.Tx) (uint32, error) {
	var bn string
	csd, ok := tx.CoinSpecificData.(CoreCoinSpecificData)
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

// CoreCoinTypeGetTokenTransfersFromTx returns tokens data from bchain.Tx
func (p *CoreCoinParser) CoreCoinTypeGetTokenTransfersFromTx(tx *bchain.Tx) (bchain.TokenTransfers, error) {
	var r bchain.TokenTransfers
	var err error
	csd, ok := tx.CoinSpecificData.(CoreCoinSpecificData)
	if ok {
		if csd.Receipt != nil {
			r, err = getTokenTransfersFromLog(csd.Receipt.Logs)
		} else {
			r, err = getTokenTransfersFromTx(csd.Tx)
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

// CoreCoinTxData contains core coin specific transaction data
type CoreCoinTxData struct {
	Status      TxStatus `json:"status"` // 1 OK, 0 Fail, -1 pending, -2 unknown
	Nonce       uint64   `json:"nonce"`
	EnergyLimit *big.Int `json:"energylimit"`
	EnergyUsed  *big.Int `json:"energyused"`
	EnergyPrice *big.Int `json:"energyprice"`
	Data        string   `json:"data"`
}

// GetCoreCoinTxData returns CoreCoinTxData from bchain.Tx
func GetCoreCoinTxData(tx *bchain.Tx) *CoreCoinTxData {
	return GetCoreCoinTxDataFromSpecificData(tx.CoinSpecificData)
}

// GetCoreCoinTxDataFromSpecificData returns CoreCoinTxData from coinSpecificData
func GetCoreCoinTxDataFromSpecificData(coinSpecificData interface{}) *CoreCoinTxData {
	etd := CoreCoinTxData{Status: TxStatusPending}
	csd, ok := coinSpecificData.(CoreCoinSpecificData)
	if ok {
		if csd.Tx != nil {
			etd.Nonce, _ = hexutil.DecodeUint64(csd.Tx.AccountNonce)
			etd.EnergyLimit, _ = hexutil.DecodeBig(csd.Tx.EnergyLimit)
			etd.EnergyPrice, _ = hexutil.DecodeBig(csd.Tx.EnergyPrice)
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
			etd.EnergyUsed, _ = hexutil.DecodeBig(csd.Receipt.EnergyUsed)
		}
	}
	return &etd
}
