package bchain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
)

// errors with specific meaning returned by blockchain rpc
var (
	// ErrBlockNotFound is returned when block is not found
	// either unknown hash or too high height
	// can be returned from GetBlockHash, GetBlockHeader, GetBlock
	ErrBlockNotFound = errors.New("Block not found")
	// ErrAddressMissing is returned if address is not specified
	// for example To address in ethereum can be missing in case of contract transaction
	ErrAddressMissing = errors.New("Address missing")
	// ErrTxidMissing is returned if txid is not specified
	// for example coinbase transactions in Bitcoin
	ErrTxidMissing = errors.New("Txid missing")
)

type ScriptSig struct {
	// Asm string `json:"asm"`
	Hex string `json:"hex"`
}

type Vin struct {
	Coinbase  string    `json:"coinbase"`
	Txid      string    `json:"txid"`
	Vout      uint32    `json:"vout"`
	ScriptSig ScriptSig `json:"scriptSig"`
	Sequence  uint32    `json:"sequence"`
	Addresses []string  `json:"addresses"`
}

type ScriptPubKey struct {
	// Asm       string   `json:"asm"`
	Hex string `json:"hex,omitempty"`
	// Type      string   `json:"type"`
	Addresses []string `json:"addresses"`
}

type Address interface {
	String() string
	AreEqual(addr string) bool
	InSlice(addrs []string) bool
}

type Vout struct {
	ValueSat     big.Int
	JsonValue    json.Number  `json:"value"`
	N            uint32       `json:"n"`
	ScriptPubKey ScriptPubKey `json:"scriptPubKey"`
	Address      Address
}

// Tx is blockchain transaction
// unnecessary fields are commented out to avoid overhead
type Tx struct {
	Hex      string `json:"hex"`
	Txid     string `json:"txid"`
	Version  int32  `json:"version"`
	LockTime uint32 `json:"locktime"`
	Vin      []Vin  `json:"vin"`
	Vout     []Vout `json:"vout"`
	// BlockHash     string `json:"blockhash,omitempty"`
	Confirmations uint32 `json:"confirmations,omitempty"`
	Time          int64  `json:"time,omitempty"`
	Blocktime     int64  `json:"blocktime,omitempty"`
}

type Block struct {
	BlockHeader
	Txs []Tx `json:"tx"`
}

type ThinBlock struct {
	BlockHeader
	Txids []string `json:"tx"`
}

type BlockHeader struct {
	Hash          string `json:"hash"`
	Prev          string `json:"previousblockhash"`
	Next          string `json:"nextblockhash"`
	Height        uint32 `json:"height"`
	Confirmations int    `json:"confirmations"`
}

type MempoolEntry struct {
	Size            uint32   `json:"size"`
	Fee             big.Int  `json:"fee"`
	ModifiedFee     big.Int  `json:"modifiedfee"`
	Time            uint64   `json:"time"`
	Height          uint32   `json:"height"`
	DescendantCount uint32   `json:"descendantcount"`
	DescendantSize  uint32   `json:"descendantsize"`
	DescendantFees  uint32   `json:"descendantfees"`
	AncestorCount   uint32   `json:"ancestorcount"`
	AncestorSize    uint32   `json:"ancestorsize"`
	AncestorFees    uint32   `json:"ancestorfees"`
	Depends         []string `json:"depends"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("%d: %s", e.Code, e.Message)
}

// BlockChain defines common interface to block chain daemon
type BlockChain interface {
	// life-cycle methods
	Initialize() error
	Shutdown(ctx context.Context) error
	// chain info
	IsTestnet() bool
	GetNetworkName() string
	GetSubversion() string
	GetCoinName() string
	// requests
	GetBlockChainInfo() (string, error)
	GetBestBlockHash() (string, error)
	GetBestBlockHeight() (uint32, error)
	GetBlockHash(height uint32) (string, error)
	GetBlockHeader(hash string) (*BlockHeader, error)
	GetBlock(hash string, height uint32) (*Block, error)
	GetMempool() ([]string, error)
	GetTransaction(txid string) (*Tx, error)
	GetTransactionForMempool(txid string) (*Tx, error)
	EstimateSmartFee(blocks int, conservative bool) (big.Int, error)
	EstimateFee(blocks int) (big.Int, error)
	SendRawTransaction(tx string) (string, error)
	// mempool
	ResyncMempool(onNewTxAddr func(txid string, addr string)) (int, error)
	GetMempoolTransactions(address string) ([]string, error)
	GetMempoolEntry(txid string) (*MempoolEntry, error)
	// parser
	GetChainParser() BlockChainParser
}

// BlockChainParser defines common interface to parsing and conversions of block chain data
type BlockChainParser interface {
	// chain configuration description
	// UTXO chains need "inputs" column in db, that map transactions to transactions that spend them
	// non UTXO chains have mapping of address to input and output transactions directly in "outputs" column in db
	IsUTXOChain() bool
	// KeepBlockAddresses returns number of blocks which are to be kept in blockaddresses column
	// and used in case of fork
	// if 0 the blockaddresses column is not used at all (usually non UTXO chains)
	KeepBlockAddresses() int
	// AmountToDecimalString converts amount in big.Int to string with decimal point in the correct place
	AmountToDecimalString(a *big.Int) string
	// AmountToBigInt converts amount in json.Number (string) to big.Int
	// it uses string operations to avoid problems with rounding
	AmountToBigInt(n json.Number) (big.Int, error)
	// address id conversions
	GetAddrIDFromVout(output *Vout) ([]byte, error)
	GetAddrIDFromAddress(address string) ([]byte, error)
	// address to output script conversions
	AddressToOutputScript(address string) ([]byte, error)
	OutputScriptToAddresses(script []byte) ([]string, error)
	// transactions
	PackedTxidLen() int
	PackTxid(txid string) ([]byte, error)
	UnpackTxid(buf []byte) (string, error)
	ParseTx(b []byte) (*Tx, error)
	ParseTxFromJson(json.RawMessage) (*Tx, error)
	PackTx(tx *Tx, height uint32, blockTime int64) ([]byte, error)
	UnpackTx(buf []byte) (*Tx, uint32, error)
	// blocks
	PackBlockHash(hash string) ([]byte, error)
	UnpackBlockHash(buf []byte) (string, error)
	ParseBlock(b []byte) (*Block, error)
}
