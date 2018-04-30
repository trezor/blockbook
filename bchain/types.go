package bchain

import (
	"errors"
	"fmt"
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
	Addresses []string  `json:"addresses,omitempty"`
}

type ScriptPubKey struct {
	// Asm       string   `json:"asm"`
	Hex string `json:"hex,omitempty"`
	// Type      string   `json:"type"`
	Addresses []string `json:"addresses,omitempty"`
}

type AddressFormat = uint8

const (
	DefaultAddress AddressFormat = iota
	BCashAddress
)

type Address interface {
	String() string
	EncodeAddress(format AddressFormat) (string, error)
	AreEqual(addr string) (bool, error)
	InSlice(addrs []string) (bool, error)
}

type Vout struct {
	Value        float64      `json:"value"`
	N            uint32       `json:"n"`
	ScriptPubKey ScriptPubKey `json:"scriptPubKey"`
	Address      Address
}

// Tx is blockchain transaction
// unnecessary fields are commented out to avoid overhead
type Tx struct {
	Hex  string `json:"hex"`
	Txid string `json:"txid"`
	// Version  int32  `json:"version"`
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
	Fee             float64  `json:"fee"`
	ModifiedFee     float64  `json:"modifiedfee"`
	Time            float64  `json:"time"`
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
	Shutdown() error
	// chain info
	IsTestnet() bool
	GetNetworkName() string
	GetSubversion() string
	// requests
	GetBestBlockHash() (string, error)
	GetBestBlockHeight() (uint32, error)
	GetBlockHash(height uint32) (string, error)
	GetBlockHeader(hash string) (*BlockHeader, error)
	GetBlock(hash string, height uint32) (*Block, error)
	GetMempool() ([]string, error)
	GetTransaction(txid string) (*Tx, error)
	EstimateSmartFee(blocks int, conservative bool) (float64, error)
	EstimateFee(blocks int) (float64, error)
	SendRawTransaction(tx string) (string, error)
	// mempool
	ResyncMempool(onNewTxAddr func(txid string, addr string)) error
	GetMempoolTransactions(address string) ([]string, error)
	GetMempoolSpentOutput(outputTxid string, vout uint32) string
	GetMempoolEntry(txid string) (*MempoolEntry, error)
	// parser
	GetChainParser() BlockChainParser
}

// BlockChainParser defines common interface to parsing and conversions of block chain data
type BlockChainParser interface {
	// self description
	// UTXO chains need "inputs" column in db, that map transactions to transactions that spend them
	// non UTXO chains have mapping of address to input and output transactions directly in "outputs" column in db
	IsUTXOChain() bool
	// KeepBlockAddresses returns number of blocks which are to be kept in blockaddresses column
	// and used in case of fork
	// if 0 the blockaddresses column is not used at all (usually non UTXO chains)
	KeepBlockAddresses() int
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
	PackTx(tx *Tx, height uint32, blockTime int64) ([]byte, error)
	UnpackTx(buf []byte) (*Tx, uint32, error)
	// blocks
	PackBlockHash(hash string) ([]byte, error)
	UnpackBlockHash(buf []byte) (string, error)
	ParseBlock(b []byte) (*Block, error)
}
