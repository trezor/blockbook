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
	Txid string `ts_doc:"Transaction ID of the referenced outpoint."`
	Vout int32  `ts_doc:"Index of the specific output in the transaction."`
}

// ScriptSig contains data about input script
type ScriptSig struct {
	// Asm string `json:"asm"`
	Hex string `json:"hex" ts_doc:"Hex-encoded representation of the scriptSig."`
}

// Vin contains data about tx input
type Vin struct {
	Coinbase  string    `json:"coinbase" ts_doc:"Coinbase data if this is a coinbase input."`
	Txid      string    `json:"txid" ts_doc:"Transaction ID of the input being spent."`
	Vout      uint32    `json:"vout" ts_doc:"Output index in the referenced transaction."`
	ScriptSig ScriptSig `json:"scriptSig" ts_doc:"scriptSig object containing the spending script data."`
	Sequence  uint32    `json:"sequence" ts_doc:"Sequence number for the input."`
	Addresses []string  `json:"addresses" ts_doc:"Addresses derived from this input's script (if known)."`
	Witness   [][]byte  `json:"-" ts_doc:"Witness data for SegWit inputs (not exposed via JSON)."`
}

// ScriptPubKey contains data about output script
type ScriptPubKey struct {
	// Asm       string   `json:"asm"`
	Hex string `json:"hex,omitempty" ts_doc:"Hex-encoded representation of the scriptPubKey."`
	// Type      string   `json:"type"`
	Addresses []string `json:"addresses" ts_doc:"Addresses derived from this output's script (if known)."`
}

// Vout contains data about tx output
type Vout struct {
	ValueSat     big.Int           `ts_doc:"Amount (in satoshi or base unit) for this output."`
	JsonValue    common.JSONNumber `json:"value" ts_doc:"String-based amount for JSON usage."`
	N            uint32            `json:"n" ts_doc:"Index of this output in the transaction."`
	ScriptPubKey ScriptPubKey      `json:"scriptPubKey" ts_doc:"scriptPubKey object containing the output script data."`
}

// Tx is blockchain transaction
// unnecessary fields are commented out to avoid overhead
type Tx struct {
	Hex         string `json:"hex" ts_doc:"Hex-encoded transaction data."`
	Txid        string `json:"txid" ts_doc:"Transaction ID (hash)."`
	Version     int32  `json:"version" ts_doc:"Transaction version number."`
	LockTime    uint32 `json:"locktime" ts_doc:"Locktime specifying earliest time/block a tx can be mined."`
	VSize       int64  `json:"vsize,omitempty" ts_doc:"Virtual size of the transaction (for SegWit-based networks)."`
	Vin         []Vin  `json:"vin" ts_doc:"List of inputs."`
	Vout        []Vout `json:"vout" ts_doc:"List of outputs."`
	BlockHeight uint32 `json:"blockHeight,omitempty" ts_doc:"Block height in which this transaction was included."`
	// BlockHash     string `json:"blockhash,omitempty"`
	Confirmations    uint32      `json:"confirmations,omitempty" ts_doc:"Number of confirmations the transaction has."`
	Time             int64       `json:"time,omitempty" ts_doc:"Timestamp when the transaction was broadcast or included in a block."`
	Blocktime        int64       `json:"blocktime,omitempty" ts_doc:"Timestamp of the block in which the transaction was mined."`
	CoinSpecificData interface{} `json:"-" ts_doc:"Additional chain-specific data (not exposed via JSON)."`
}

// MempoolVin contains data about tx input specifically in mempool
type MempoolVin struct {
	Vin
	AddrDesc AddressDescriptor `json:"-" ts_doc:"Internal descriptor for the input address (not exposed)."`
	ValueSat big.Int           `ts_doc:"Amount (in satoshi or base unit) of the input."`
}

// MempoolTx is blockchain transaction in mempool
// optimized for onNewTx notification
type MempoolTx struct {
	Hex              string         `json:"hex" ts_doc:"Hex-encoded transaction data."`
	Txid             string         `json:"txid" ts_doc:"Transaction ID (hash)."`
	Version          int32          `json:"version" ts_doc:"Transaction version number."`
	LockTime         uint32         `json:"locktime" ts_doc:"Locktime specifying earliest time/block a tx can be mined."`
	VSize            int64          `json:"vsize,omitempty" ts_doc:"Virtual size of the transaction (if applicable)."`
	Vin              []MempoolVin   `json:"vin" ts_doc:"List of inputs in this mempool transaction."`
	Vout             []Vout         `json:"vout" ts_doc:"List of outputs in this mempool transaction."`
	Blocktime        int64          `json:"blocktime,omitempty" ts_doc:"Timestamp for the block in which tx might eventually be mined, if known."`
	TokenTransfers   TokenTransfers `json:"-" ts_doc:"Token transfers discovered in this mempool transaction (not exposed by default)."`
	CoinSpecificData interface{}    `json:"-" ts_doc:"Additional chain-specific data (not exposed via JSON)."`
}

// TokenStandard - standard of token
type TokenStandard int

// TokenStandard enumeration
const (
	FungibleToken    = TokenStandard(iota) // ERC20/BEP20
	NonFungibleToken                       // ERC721/BEP721
	MultiToken                             // ERC1155/BEP1155
)

// TokenStandardName specifies standard of token
type TokenStandardName string

// Token standards
const (
	UnknownTokenStandard   TokenStandardName = ""
	UnhandledTokenStandard TokenStandardName = "-"

	// XPUBAddressStandard is address derived from xpub
	XPUBAddressStandard TokenStandardName = "XPUBAddress"
)

// TokenTransfers is array of TokenTransfer
type TokenTransfers []*TokenTransfer

func (a TokenTransfers) Len() int      { return len(a) }
func (a TokenTransfers) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a TokenTransfers) Less(i, j int) bool {
	return a[i].Standard < a[j].Standard
}

// Block is block header and list of transactions
type Block struct {
	BlockHeader
	Txs              []Tx        `json:"tx" ts_doc:"List of full transactions included in this block."`
	CoinSpecificData interface{} `json:"-" ts_doc:"Additional chain-specific data (not exposed via JSON)."`
}

// BlockHeader contains limited data (as needed for indexing) from backend block header
type BlockHeader struct {
	Hash          string `json:"hash" ts_doc:"Block hash."`
	Prev          string `json:"previousblockhash" ts_doc:"Hash of the previous block in the chain."`
	Next          string `json:"nextblockhash" ts_doc:"Hash of the next block, if known."`
	Height        uint32 `json:"height" ts_doc:"Block height (0-based index in the chain)."`
	Confirmations int    `json:"confirmations" ts_doc:"Number of confirmations (distance from best chain tip)."`
	Size          int    `json:"size" ts_doc:"Block size in bytes."`
	Time          int64  `json:"time,omitempty" ts_doc:"Timestamp of when this block was mined."`
}

// BlockInfo contains extended block header data and a list of block txids
type BlockInfo struct {
	BlockHeader
	Version    common.JSONNumber `json:"version" ts_doc:"Block version (chain-specific meaning)."`
	MerkleRoot string            `json:"merkleroot" ts_doc:"Merkle root of the block's transactions."`
	Nonce      common.JSONNumber `json:"nonce" ts_doc:"Nonce used in the mining process."`
	Bits       string            `json:"bits" ts_doc:"Compact representation of the target threshold."`
	Difficulty common.JSONNumber `json:"difficulty" ts_doc:"Difficulty target for mining this block."`
	Txids      []string          `json:"tx,omitempty" ts_doc:"List of transaction IDs included in this block."`
}

// MempoolEntry is used to get data about mempool entry
type MempoolEntry struct {
	Size            uint32            `json:"size" ts_doc:"Size of the transaction in bytes, as stored in mempool."`
	FeeSat          big.Int           `ts_doc:"Transaction fee in satoshi/base units."`
	Fee             common.JSONNumber `json:"fee" ts_doc:"String-based fee for JSON usage."`
	ModifiedFeeSat  big.Int           `ts_doc:"Modified fee in satoshi/base units after priority adjustments."`
	ModifiedFee     common.JSONNumber `json:"modifiedfee" ts_doc:"String-based modified fee for JSON usage."`
	Time            uint64            `json:"time" ts_doc:"Unix timestamp when the tx entered the mempool."`
	Height          uint32            `json:"height" ts_doc:"Block height when the tx entered the mempool."`
	DescendantCount uint32            `json:"descendantcount" ts_doc:"Number of descendant transactions in mempool."`
	DescendantSize  uint32            `json:"descendantsize" ts_doc:"Total size of all descendant transactions in bytes."`
	DescendantFees  uint32            `json:"descendantfees" ts_doc:"Combined fees of all descendant transactions."`
	AncestorCount   uint32            `json:"ancestorcount" ts_doc:"Number of ancestor transactions in mempool."`
	AncestorSize    uint32            `json:"ancestorsize" ts_doc:"Total size of all ancestor transactions in bytes."`
	AncestorFees    uint32            `json:"ancestorfees" ts_doc:"Combined fees of all ancestor transactions."`
	Depends         []string          `json:"depends" ts_doc:"List of txids this transaction depends on."`
}

// ChainInfo is used to get information about blockchain
type ChainInfo struct {
	Chain            string      `json:"chain" ts_doc:"Name of the chain (e.g. 'main')."`
	Blocks           int         `json:"blocks" ts_doc:"Number of fully verified blocks in the chain."`
	Headers          int         `json:"headers" ts_doc:"Number of block headers in the chain (can be ahead of full blocks)."`
	Bestblockhash    string      `json:"bestblockhash" ts_doc:"Hash of the best (latest) block."`
	Difficulty       string      `json:"difficulty" ts_doc:"Current difficulty of the network."`
	SizeOnDisk       int64       `json:"size_on_disk" ts_doc:"Size of the blockchain data on disk in bytes."`
	Version          string      `json:"version" ts_doc:"Version of the blockchain backend."`
	Subversion       string      `json:"subversion" ts_doc:"Subversion string of the blockchain backend."`
	ProtocolVersion  string      `json:"protocolversion" ts_doc:"Protocol version for this chain node."`
	Timeoffset       float64     `json:"timeoffset" ts_doc:"Time offset (in seconds) reported by the node."`
	Warnings         string      `json:"warnings" ts_doc:"Any warnings generated by the node regarding the chain state."`
	ConsensusVersion string      `json:"consensus_version,omitempty" ts_doc:"Version of the chain's consensus protocol, if available."`
	Consensus        interface{} `json:"consensus,omitempty" ts_doc:"Additional consensus details, structure depends on chain."`
}

// LongTermFeeRate gets information about the fee rate over longer period of time.
type LongTermFeeRate struct {
	FeePerUnit big.Int `json:"feePerUnit" ts_doc:"Long term fee rate (in sat/kByte)."`
	Blocks     uint64  `json:"blocks" ts_doc:"Amount of blocks used for the long term fee rate estimation."`
}

// RPCError defines rpc error returned by backend
type RPCError struct {
	Code    int    `json:"code" ts_doc:"Error code returned by the backend RPC."`
	Message string `json:"message" ts_doc:"Human-readable error message."`
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
	Txid string `ts_doc:"Transaction ID (hash) of the mempool entry."`
	Time uint32 `ts_doc:"Unix timestamp when the transaction was first seen in the mempool."`
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
	XpubDescriptor string      `ts_doc:"Full descriptor string including xpub and script type."`
	Xpub           string      `ts_doc:"The xpub part itself extracted from the descriptor."`
	Type           ScriptType  `ts_doc:"Parsed script type (P2PKH, P2WPKH, etc.)."`
	Bip            string      `ts_doc:"BIP standard (e.g. BIP44) inferred from the descriptor."`
	ChangeIndexes  []uint32    `ts_doc:"Indexes designated as change addresses."`
	ExtKey         interface{} `ts_doc:"Extended key object parsed from xpub (implementation-specific)."`
}

// MempoolTxidEntries is array of MempoolTxidEntry
type MempoolTxidEntries []MempoolTxidEntry

// MempoolTxidFilterEntries is a map of txids to mempool golomb filters
// Also contains a flag whether constant zeroed key was used when calculating the filters
type MempoolTxidFilterEntries struct {
	Entries       map[string]string `json:"entries,omitempty" ts_doc:"Map of txid to filter data (hex-encoded)."`
	UsedZeroedKey bool              `json:"usedZeroedKey,omitempty" ts_doc:"Indicates if a zeroed key was used in filter calculation."`
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
	LongTermFeeRate() (*LongTermFeeRate, error)
	SendRawTransaction(tx string, disableAlternativeRPC bool) (string, error)
	GetMempoolEntry(txid string) (*MempoolEntry, error)
	GetContractInfo(contractDesc AddressDescriptor) (*ContractInfo, error)
	// parser
	GetChainParser() BlockChainParser
	// EthereumType specific
	EthereumTypeGetBalance(addrDesc AddressDescriptor) (*big.Int, error)
	EthereumTypeGetNonce(addrDesc AddressDescriptor) (uint64, error)
	EthereumTypeEstimateGas(params map[string]interface{}) (uint64, error)
	EthereumTypeGetEip1559Fees() (*Eip1559Fees, error)
	EthereumTypeGetErc20ContractBalance(addrDesc, contractDesc AddressDescriptor) (*big.Int, error)
	EthereumTypeGetSupportedStakingPools() []string
	EthereumTypeGetStakingPoolsData(addrDesc AddressDescriptor) ([]StakingPoolData, error)
	EthereumTypeRpcCall(data, to, from string) (string, error)
	EthereumTypeGetRawTransaction(txid string) (string, error)
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
