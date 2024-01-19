package bchain

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"

	"github.com/trezor/blockbook/common"
)

// ChainType is type of the blockchain
type ChainType int

const (
	// ChainBitcoinType is blockchain derived from bitcoin
	ChainBitcoinType = ChainType(iota)
	// ChainEthereumType is blockchain derived from ethereum
	ChainEthereumType
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
	// ErrTxNotFound is returned if transaction was not found
	ErrTxNotFound = errors.New("Tx not found")
)

// Outpoint is txid together with output (or input) index
type Outpoint struct {
	Txid string
	Vout int32
}

// ScriptSig contains data about input script
type ScriptSig struct {
	// Asm string `json:"asm"`
	Hex string `json:"hex"`
}

// Vin contains data about tx input
type Vin struct {
	Coinbase  string    `json:"coinbase"`
	Txid      string    `json:"txid"`
	Vout      uint32    `json:"vout"`
	ScriptSig ScriptSig `json:"scriptSig"`
	Sequence  uint32    `json:"sequence"`
	Addresses []string  `json:"addresses"`
	Witness   [][]byte  `json:"-"`
}

// ScriptPubKey contains data about output script
type ScriptPubKey struct {
	// Asm       string   `json:"asm"`
	Hex string `json:"hex,omitempty"`
	// Type      string   `json:"type"`
	Addresses []string `json:"addresses"`
}

// Vout contains data about tx output
type Vout struct {
	ValueSat     big.Int
	JsonValue    common.JSONNumber `json:"value"`
	N            uint32            `json:"n"`
	ScriptPubKey ScriptPubKey      `json:"scriptPubKey"`
}

// Tx is blockchain transaction
// unnecessary fields are commented out to avoid overhead
type Tx struct {
	Hex         string `json:"hex"`
	Txid        string `json:"txid"`
	Version     int32  `json:"version"`
	LockTime    uint32 `json:"locktime"`
	VSize       int64  `json:"vsize,omitempty"`
	Vin         []Vin  `json:"vin"`
	Vout        []Vout `json:"vout"`
	BlockHeight uint32 `json:"blockHeight,omitempty"`
	// BlockHash     string `json:"blockhash,omitempty"`
	Confirmations    uint32      `json:"confirmations,omitempty"`
	Time             int64       `json:"time,omitempty"`
	Blocktime        int64       `json:"blocktime,omitempty"`
	CoinSpecificData interface{} `json:"-"`
}

// MempoolVin contains data about tx input
type MempoolVin struct {
	Vin
	AddrDesc AddressDescriptor `json:"-"`
	ValueSat big.Int
}

// MempoolTx is blockchain transaction in mempool
// optimized for onNewTx notification
type MempoolTx struct {
	Hex              string         `json:"hex"`
	Txid             string         `json:"txid"`
	Version          int32          `json:"version"`
	LockTime         uint32         `json:"locktime"`
	VSize            int64          `json:"vsize,omitempty"`
	Vin              []MempoolVin   `json:"vin"`
	Vout             []Vout         `json:"vout"`
	Blocktime        int64          `json:"blocktime,omitempty"`
	TokenTransfers   TokenTransfers `json:"-"`
	CoinSpecificData interface{}    `json:"-"`
}

// TokenType - type of token
type TokenType int

// TokenType enumeration
const (
	FungibleToken    = TokenType(iota) // ERC20/BEP20
	NonFungibleToken                   // ERC721/BEP721
	MultiToken                         // ERC1155/BEP1155
)

// TokenTypeName specifies type of token
type TokenTypeName string

// Token types
const (
	UnknownTokenType TokenTypeName = ""

	// XPUBAddressTokenType is address derived from xpub
	XPUBAddressTokenType TokenTypeName = "XPUBAddress"
)

// TokenTransfers is array of TokenTransfer
type TokenTransfers []*TokenTransfer

func (a TokenTransfers) Len() int      { return len(a) }
func (a TokenTransfers) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a TokenTransfers) Less(i, j int) bool {
	return a[i].Type < a[j].Type
}

// Block is block header and list of transactions
type Block struct {
	BlockHeader
	Txs              []Tx        `json:"tx"`
	CoinSpecificData interface{} `json:"-"`
}

// BlockHeader contains limited data (as needed for indexing) from backend block header
type BlockHeader struct {
	Hash          string `json:"hash"`
	Prev          string `json:"previousblockhash"`
	Next          string `json:"nextblockhash"`
	Height        uint32 `json:"height"`
	Confirmations int    `json:"confirmations"`
	Size          int    `json:"size"`
	Time          int64  `json:"time,omitempty"`
}

// BlockInfo contains extended block header data and a list of block txids
type BlockInfo struct {
	BlockHeader
	Version    common.JSONNumber `json:"version"`
	MerkleRoot string            `json:"merkleroot"`
	Nonce      common.JSONNumber `json:"nonce"`
	Bits       string            `json:"bits"`
	Difficulty common.JSONNumber `json:"difficulty"`
	Txids      []string          `json:"tx,omitempty"`
}

// MempoolEntry is used to get data about mempool entry
type MempoolEntry struct {
	Size            uint32 `json:"size"`
	FeeSat          big.Int
	Fee             common.JSONNumber `json:"fee"`
	ModifiedFeeSat  big.Int
	ModifiedFee     common.JSONNumber `json:"modifiedfee"`
	Time            uint64            `json:"time"`
	Height          uint32            `json:"height"`
	DescendantCount uint32            `json:"descendantcount"`
	DescendantSize  uint32            `json:"descendantsize"`
	DescendantFees  uint32            `json:"descendantfees"`
	AncestorCount   uint32            `json:"ancestorcount"`
	AncestorSize    uint32            `json:"ancestorsize"`
	AncestorFees    uint32            `json:"ancestorfees"`
	Depends         []string          `json:"depends"`
}

// ChainInfo is used to get information about blockchain
type ChainInfo struct {
	Chain            string      `json:"chain"`
	Blocks           int         `json:"blocks"`
	Headers          int         `json:"headers"`
	Bestblockhash    string      `json:"bestblockhash"`
	Difficulty       string      `json:"difficulty"`
	SizeOnDisk       int64       `json:"size_on_disk"`
	Version          string      `json:"version"`
	Subversion       string      `json:"subversion"`
	ProtocolVersion  string      `json:"protocolversion"`
	Timeoffset       float64     `json:"timeoffset"`
	Warnings         string      `json:"warnings"`
	ConsensusVersion string      `json:"consensus_version,omitempty"`
	Consensus        interface{} `json:"consensus,omitempty"`
}

// RPCError defines rpc error returned by backend
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("%d: %s", e.Code, e.Message)
}

// AddressDescriptor is an opaque type obtained by parser.GetAddrDesc* methods
type AddressDescriptor []byte

func (ad AddressDescriptor) String() string {
	return "ad:" + hex.EncodeToString(ad)
}

func (ad AddressDescriptor) IsTaproot() bool {
	if len(ad) == 34 && ad[0] == 0x51 && ad[1] == 0x20 {
		return true
	}
	return false
}

// AddressDescriptorFromString converts string created by AddressDescriptor.String to AddressDescriptor
func AddressDescriptorFromString(s string) (AddressDescriptor, error) {
	if len(s) > 3 && s[0:3] == "ad:" {
		return hex.DecodeString(s[3:])
	}
	return nil, errors.New("invalid address descriptor")
}

// MempoolTxidEntry contains mempool txid with first seen time
type MempoolTxidEntry struct {
	Txid string
	Time uint32
}

// ScriptType - type of output script parsed from xpub (descriptor)
type ScriptType int

// ScriptType enumeration
const (
	P2PK = ScriptType(iota)
	P2PKH
	P2SHWPKH
	P2WPKH
	P2TR
)

// XpubDescriptor contains parsed data from xpub descriptor
type XpubDescriptor struct {
	XpubDescriptor string // The whole descriptor
	Xpub           string // Xpub part of the descriptor
	Type           ScriptType
	Bip            string
	ChangeIndexes  []uint32
	ExtKey         interface{} // extended key parsed from xpub, usually of type *hdkeychain.ExtendedKey
}

// MempoolTxidEntries is array of MempoolTxidEntry
type MempoolTxidEntries []MempoolTxidEntry

// MempoolTxidFilterEntries is a map of txids to mempool golomb filters
// Also contains a flag whether constant zeroed key was used when calculating the filters
type MempoolTxidFilterEntries struct {
	Entries       map[string]string `json:"entries,omitempty"`
	UsedZeroedKey bool              `json:"usedZeroedKey,omitempty"`
}

// OnNewBlockFunc is used to send notification about a new block
type OnNewBlockFunc func(hash string, height uint32)

// OnNewTxAddrFunc is used to send notification about a new transaction/address
type OnNewTxAddrFunc func(tx *Tx, desc AddressDescriptor)

// OnNewTxFunc is used to send notification about a new transaction/address
type OnNewTxFunc func(tx *MempoolTx)

// AddrDescForOutpointFunc returns address descriptor and value for given outpoint or nil if outpoint not found
type AddrDescForOutpointFunc func(outpoint Outpoint) (AddressDescriptor, *big.Int)

// BlockChain defines common interface to block chain daemon
type BlockChain interface {
	// life-cycle methods
	// initialize the block chain connector
	Initialize() error
	// create mempool but do not initialize it
	CreateMempool(BlockChain) (Mempool, error)
	// initialize mempool, create ZeroMQ (or other) subscription
	InitializeMempool(AddrDescForOutpointFunc, OnNewTxAddrFunc, OnNewTxFunc) error
	// shutdown mempool, ZeroMQ and block chain connections
	Shutdown(ctx context.Context) error
	// chain info
	IsTestnet() bool
	GetNetworkName() string
	GetSubversion() string
	GetCoinName() string
	GetChainInfo() (*ChainInfo, error)
	// requests
	GetBestBlockHash() (string, error)
	GetBestBlockHeight() (uint32, error)
	GetBlockHash(height uint32) (string, error)
	GetBlockHeader(hash string) (*BlockHeader, error)
	GetBlock(hash string, height uint32) (*Block, error)
	GetBlockInfo(hash string) (*BlockInfo, error)
	GetBlockRaw(hash string) (string, error)
	GetMempoolTransactions() ([]string, error)
	GetTransaction(txid string) (*Tx, error)
	GetTransactionForMempool(txid string) (*Tx, error)
	GetTransactionSpecific(tx *Tx) (json.RawMessage, error)
	EstimateSmartFee(blocks int, conservative bool) (big.Int, error)
	EstimateFee(blocks int) (big.Int, error)
	SendRawTransaction(tx string) (string, error)
	GetMempoolEntry(txid string) (*MempoolEntry, error)
	GetContractInfo(contractDesc AddressDescriptor) (*ContractInfo, error)
	// parser
	GetChainParser() BlockChainParser
	// EthereumType specific
	EthereumTypeGetBalance(addrDesc AddressDescriptor) (*big.Int, error)
	EthereumTypeGetNonce(addrDesc AddressDescriptor) (uint64, error)
	EthereumTypeEstimateGas(params map[string]interface{}) (uint64, error)
	EthereumTypeGetErc20ContractBalance(addrDesc, contractDesc AddressDescriptor) (*big.Int, error)
	EthereumTypeGetSupportedStakingPools() []string
	EthereumTypeGetStakingPoolsData(addrDesc AddressDescriptor) ([]StakingPoolData, error)
	GetTokenURI(contractDesc AddressDescriptor, tokenID *big.Int) (string, error)
}

// BlockChainParser defines common interface to parsing and conversions of block chain data
type BlockChainParser interface {
	// type of the blockchain
	GetChainType() ChainType
	// KeepBlockAddresses returns number of blocks which are to be kept in blockTxs column
	// to be used for rollbacks
	KeepBlockAddresses() int
	// AmountDecimals returns number of decimal places in coin amounts
	AmountDecimals() int
	// UseAddressAliases returns true if address aliases are enabled
	UseAddressAliases() bool
	// MinimumCoinbaseConfirmations returns minimum number of confirmations a coinbase transaction must have before it can be spent
	MinimumCoinbaseConfirmations() int
	// SupportsVSize returns true if vsize of a transaction should be computed and returned by API
	SupportsVSize() bool
	// AmountToDecimalString converts amount in big.Int to string with decimal point in the correct place
	AmountToDecimalString(a *big.Int) string
	// AmountToBigInt converts amount in common.JSONNumber (string) to big.Int
	// it uses string operations to avoid problems with rounding
	AmountToBigInt(n common.JSONNumber) (big.Int, error)
	// address descriptor conversions
	GetAddrDescFromVout(output *Vout) (AddressDescriptor, error)
	GetAddrDescFromAddress(address string) (AddressDescriptor, error)
	GetAddressesFromAddrDesc(addrDesc AddressDescriptor) ([]string, bool, error)
	GetScriptFromAddrDesc(addrDesc AddressDescriptor) ([]byte, error)
	IsAddrDescIndexable(addrDesc AddressDescriptor) bool
	// transactions
	PackedTxidLen() int
	PackTxid(txid string) ([]byte, error)
	UnpackTxid(buf []byte) (string, error)
	ParseTx(b []byte) (*Tx, error)
	ParseTxFromJson(json.RawMessage) (*Tx, error)
	PackTx(tx *Tx, height uint32, blockTime int64) ([]byte, error)
	UnpackTx(buf []byte) (*Tx, uint32, error)
	GetAddrDescForUnknownInput(tx *Tx, input int) AddressDescriptor
	// blocks
	PackBlockHash(hash string) ([]byte, error)
	UnpackBlockHash(buf []byte) (string, error)
	ParseBlock(b []byte) (*Block, error)
	// xpub
	ParseXpub(xpub string) (*XpubDescriptor, error)
	DerivationBasePath(descriptor *XpubDescriptor) (string, error)
	DeriveAddressDescriptors(descriptor *XpubDescriptor, change uint32, indexes []uint32) ([]AddressDescriptor, error)
	DeriveAddressDescriptorsFromTo(descriptor *XpubDescriptor, change uint32, fromIndex uint32, toIndex uint32) ([]AddressDescriptor, error)
	// EthereumType specific
	EthereumTypeGetTokenTransfersFromTx(tx *Tx) (TokenTransfers, error)
	// AddressAlias
	FormatAddressAlias(address string, name string) string
}

// Mempool defines common interface to mempool
type Mempool interface {
	Resync() (int, error)
	GetTransactions(address string) ([]Outpoint, error)
	GetAddrDescTransactions(addrDesc AddressDescriptor) ([]Outpoint, error)
	GetAllEntries() MempoolTxidEntries
	GetTransactionTime(txid string) uint32
	GetTxidFilterEntries(filterScripts string, fromTimestamp uint32) (MempoolTxidFilterEntries, error)
}
