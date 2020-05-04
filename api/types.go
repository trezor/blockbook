package api

import (
	"blockbook/bchain"
	"blockbook/common"
	"encoding/json"
	"errors"
	"math/big"
	"sort"
	"time"
)

const maxUint32 = ^uint32(0)
const maxInt = int(^uint(0) >> 1)
const maxInt64 = int64(^uint64(0) >> 1)

// AccountDetails specifies what data returns GetAddress and GetXpub calls
type AccountDetails int

const (
	// AccountDetailsBasic - only that address is indexed and some basic info
	AccountDetailsBasic AccountDetails = iota
	// AccountDetailsTokens - basic info + tokens
	AccountDetailsTokens
	// AccountDetailsTokenBalances - basic info + token with balance
	AccountDetailsTokenBalances
	// AccountDetailsTxidHistory - basic + token balances + txids, subject to paging
	AccountDetailsTxidHistory
	// AccountDetailsTxHistoryLight - basic + tokens + easily obtained tx data (not requiring requests to backend), subject to paging
	AccountDetailsTxHistoryLight
	// AccountDetailsTxHistory - basic + tokens + full tx data, subject to paging
	AccountDetailsTxHistory
)

// ErrUnsupportedXpub is returned when coin type does not support xpub address derivation or provided string is not an xpub
var ErrUnsupportedXpub = errors.New("XPUB not supported")

// APIError extends error by information if the error details should be returned to the end user
type APIError struct {
	Text   string
	Public bool
}

func (e *APIError) Error() string {
	return e.Text
}

// NewAPIError creates ApiError
func NewAPIError(s string, public bool) error {
	return &APIError{
		Text:   s,
		Public: public,
	}
}


// IsZeroBigInt if big int has zero value
func IsZeroBigInt(b *big.Int) bool {
	return len(b.Bits()) == 0
}


// Vin contains information about single transaction input
type Vin struct {
	Txid      string                   `json:"txid,omitempty"`
	Vout      uint32                   `json:"vout,omitempty"`
	Sequence  int64                    `json:"sequence,omitempty"`
	N         int                      `json:"n"`
	AddrDesc  bchain.AddressDescriptor `json:"-"`
	Addresses []string                 `json:"addresses,omitempty"`
	IsAddress bool                     `json:"isAddress"`
	ValueSat  *bchain.Amount                  `json:"value,omitempty"`
	Hex       string                   `json:"hex,omitempty"`
	Asm       string                   `json:"asm,omitempty"`
	Coinbase  string                   `json:"coinbase,omitempty"`
	AssetInfo bchain.AssetInfo		   `json:"assetInfo,omitempty"`
}

// Vout contains information about single transaction output
type Vout struct {
	ValueSat    *bchain.Amount                  `json:"value,omitempty"`
	N           int                      `json:"n"`
	Spent       bool                     `json:"spent,omitempty"`
	SpentTxID   string                   `json:"spentTxId,omitempty"`
	SpentIndex  int                      `json:"spentIndex,omitempty"`
	SpentHeight int                      `json:"spentHeight,omitempty"`
	Hex         string                   `json:"hex,omitempty"`
	Asm         string                   `json:"asm,omitempty"`
	AddrDesc    bchain.AddressDescriptor `json:"-"`
	Addresses   []string                 `json:"addresses"`
	IsAddress   bool                     `json:"isAddress"`
	Type        string                   `json:"type,omitempty"`
	AssetInfo 	bchain.AssetInfo		 `json:"assetInfo,omitempty"`
}

// Contains SyscoinSpecific asset information hex decoded and pertinent to API display
type AssetSpecific struct {
	AssetGuid 		uint32
	AddrStr    	    string
	Contract 		string
	Symbol 			string
	PubData 		map[string]interface{}
	Balance 		*bchain.Amount
	TotalSupply 	*bchain.Amount
	MaxSupply 		*bchain.Amount
	Decimals 		int
	UpdateFlags 	uint8
}

// Contains SyscoinSpecific assets information when searching for assets
type AssetsSpecific struct {
	AssetGuid 		uint32
	AddrStr    	    string
	Contract 		string
	Symbol 			string
	PubData 		map[string]interface{}
	TotalSupply 	*bchain.Amount
	Decimals 		int
	Txs				int
}

// EthereumSpecific contains ethereum specific transaction data
type EthereumSpecific struct {
	Status   int      `json:"status"` // 1 OK, 0 Fail, -1 pending
	Nonce    uint64   `json:"nonce"`
	GasLimit *big.Int `json:"gasLimit"`
	GasUsed  *big.Int `json:"gasUsed"`
	GasPrice *bchain.Amount  `json:"gasPrice"`
}

// Tx holds information about a transaction
type Tx struct {
	Txid             string            `json:"txid"`
	Version          int32             `json:"version,omitempty"`
	Locktime         uint32            `json:"lockTime,omitempty"`
	Vin              []Vin             `json:"vin"`
	Vout             []Vout            `json:"vout"`
	Blockhash        string            `json:"blockHash,omitempty"`
	Blockheight      int               `json:"blockHeight"`
	Confirmations    uint32            `json:"confirmations"`
	Blocktime        int64             `json:"blockTime"`
	Size             int               `json:"size,omitempty"`
	ValueOutSat      *bchain.Amount           `json:"value"`
	ValueInSat       *bchain.Amount           `json:"valueIn,omitempty"`
	FeesSat          *bchain.Amount           `json:"fees,omitempty"`
	Hex              string            `json:"hex,omitempty"`
	Rbf              bool              `json:"rbf,omitempty"`
	CoinSpecificData interface{}       `json:"-"`
	CoinSpecificJSON json.RawMessage   `json:"-"`
	TokenTransferSummary   []*bchain.TokenTransferSummary   `json:"tokenTransfers,omitempty"`
	EthereumSpecific *EthereumSpecific `json:"ethereumSpecific,omitempty"`
}

// FeeStats contains detailed block fee statistics
type FeeStats struct {
	TxCount         int       `json:"txCount"`
	TotalFeesSat    *bchain.Amount   `json:"totalFeesSat"`
	AverageFeePerKb int64     `json:"averageFeePerKb"`
	DecilesFeePerKb [11]int64 `json:"decilesFeePerKb"`
}

// Paging contains information about paging for address, blocks and block
type Paging struct {
	Page        int `json:"page,omitempty"`
	TotalPages  int `json:"totalPages,omitempty"`
	ItemsOnPage int `json:"itemsOnPage,omitempty"`
}

// TokensToReturn specifies what tokens are returned by GetAddress and GetXpubAddress
type TokensToReturn int

const (
	// AddressFilterVoutOff disables filtering of transactions by vout
	AddressFilterVoutOff = -1
	// AddressFilterVoutInputs specifies that only txs where the address is as input are returned
	AddressFilterVoutInputs = -2
	// AddressFilterVoutOutputs specifies that only txs where the address is as output are returned
	AddressFilterVoutOutputs = -3

	// TokensToReturnNonzeroBalance - return only tokens with nonzero balance
	TokensToReturnNonzeroBalance TokensToReturn = 0
	// TokensToReturnUsed - return tokens with some transfers (even if they have zero balance now)
	TokensToReturnUsed TokensToReturn = 1
	// TokensToReturnDerived - return all derived tokens
	TokensToReturnDerived TokensToReturn = 2
)

// AddressFilter is used to filter data returned from GetAddress api method
type AddressFilter struct {
	Vout           int
	Contract       string
	FromHeight     uint32
	ToHeight       uint32
	TokensToReturn TokensToReturn
	// OnlyConfirmed set to true will ignore mempool transactions; mempool is also ignored if FromHeight/ToHeight filter is specified
	OnlyConfirmed bool
	AssetsMask 	   bchain.AssetsMask
}
// Address holds information about address and its transactions
type Address struct {
	Paging
	AddrStr               string                `json:"address"`
	BalanceSat            *bchain.Amount               `json:"balance"`
	TotalReceivedSat      *bchain.Amount               `json:"totalReceived,omitempty"`
	TotalSentSat          *bchain.Amount               `json:"totalSent,omitempty"`
	UnconfirmedBalanceSat *bchain.Amount               `json:"unconfirmedBalance"`
	UnconfirmedTxs        int                   `json:"unconfirmedTxs"`
	Txs                   int                   `json:"txs"`
	NonTokenTxs           int                   `json:"nonTokenTxs,omitempty"`
	Transactions          []*Tx                 `json:"transactions,omitempty"`
	Txids                 []string              `json:"txids,omitempty"`
	Nonce                 string                `json:"nonce,omitempty"`
	UsedTokens            int                   `json:"usedTokens,omitempty"`
	Tokens                bchain.Tokens         `json:"tokens,omitempty"`
	Erc20Contract         *bchain.Erc20Contract `json:"erc20Contract,omitempty"`
	// helpers for explorer
	Filter        string              `json:"-"`
	XPubAddresses map[string]struct{} `json:"-"`
}

// Asset holds information about asset and its transactions
type Asset struct {
	Paging
	AssetDetails		  *AssetSpecific		`json:"asset"`
	UnconfirmedTxs        int                   `json:"unconfirmedTxs"`
	Txs                   int                   `json:"txs"`
	Transactions          []*Tx                 `json:"transactions,omitempty"`
	Txids                 []string              `json:"txids,omitempty"`
	// helpers for explorer
	Filter        string              `json:"-"`
}

// Asset holds information about searching/filtering assets
type Assets struct {
	Paging
	AssetDetails		  []*AssetsSpecific		`json:"assets"`
	NumAssets             int                   `json:"numAssets"`
	// helpers for explorer
	Filter        string              `json:"-"`
}

// Utxo is one unspent transaction output
type Utxo struct {
	Txid          string  `json:"txid"`
	Vout          int32   `json:"vout"`
	AmountSat     *bchain.Amount `json:"value"`
	Height        int     `json:"height,omitempty"`
	Confirmations int     `json:"confirmations"`
	Address       string  `json:"address,omitempty"`
	Path          string  `json:"path,omitempty"`
	Locktime      uint32  `json:"lockTime,omitempty"`
	Coinbase      bool    `json:"coinbase,omitempty"`
	AssetInfo	  bchain.AssetInfo  `json:"assetInfo,omitempty"`
}

// Utxos is array of Utxo
type Utxos []Utxo

func (a Utxos) Len() int      { return len(a) }
func (a Utxos) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a Utxos) Less(i, j int) bool {
	// sort in reverse order, unconfirmed (height==0) utxos on top
	hi := a[i].Height
	hj := a[j].Height
	if hi == 0 {
		hi = maxInt
	}
	if hj == 0 {
		hj = maxInt
	}
	return hi >= hj
}

// history of tokens mapped to uint32 asset guid's in BalanceHistory obj
type TokenBalanceHistory struct {
	AssetGuid   uint32 `json:"assetGuid,omitempty"`
	ReceivedSat *bchain.Amount `json:"received,omitempty"`
	SentSat     *bchain.Amount `json:"sent,omitempty"`
}

// BalanceHistory contains info about one point in time of balance history
type BalanceHistory struct {
	Time        uint32             `json:"time"`
	Txs         uint32             `json:"txs"`
	ReceivedSat *bchain.Amount     `json:"received"`
	SentSat     *bchain.Amount     `json:"sent"`
	FiatRates   map[string]float64 `json:"rates,omitempty"`
	Txid        string             `json:"txid,omitempty"`
	Tokens	    []*TokenBalanceHistory `json:"tokens,omitempty"`	
}

// BalanceHistories is array of BalanceHistory
type BalanceHistories []BalanceHistory

func (a BalanceHistories) Len() int      { return len(a) }
func (a BalanceHistories) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a BalanceHistories) Less(i, j int) bool {
	ti := a[i].Time
	tj := a[j].Time
	if ti == tj {
		return a[i].Txid < a[j].Txid
	}
	return ti < tj
}

// SortAndAggregate sums BalanceHistories to groups defined by parameter groupByTime
func (a BalanceHistories) SortAndAggregate(groupByTime uint32) BalanceHistories {
	bhs := make(BalanceHistories, 0)
	var tokens map[uint32]*TokenBalanceHistory
	if len(a) > 0 {
		bha := BalanceHistory{
			SentSat:     &bchain.Amount{},
			ReceivedSat: &bchain.Amount{},
		}
		sort.Sort(a)
		for i := range a {
			bh := &a[i]
			time := bh.Time - bh.Time%groupByTime
			if bha.Time != time {
				if bha.Time != 0 {
					// in aggregate, do not return txid as it could multiple of them
					bha.Txid = ""
					if tokens != nil {
						bha.Tokens = []*TokenBalanceHistory{}
						// then flatten to array of token balances from the map
						for _, token := range tokens {
							bha.Tokens = append(bha.Tokens, token)
						}
						tokens = map[uint32]*TokenBalanceHistory{}
					}
					bhs = append(bhs, bha)
				}
				bha = BalanceHistory{
					Time:        time,
					SentSat:     &bchain.Amount{},
					ReceivedSat: &bchain.Amount{},
				}
			}
			if bha.Txid != bh.Txid {
				bha.Txs += bh.Txs
				bha.Txid = bh.Txid
			}
			if len(bh.Tokens) > 0 {
				if tokens == nil {
					tokens = map[uint32]*TokenBalanceHistory{}
				}
				// fill up map of balances for each asset guid
				for _, token := range bh.Tokens {
					bhaToken, ok := tokens[token.AssetGuid];
					if !ok {
						bhaToken = &TokenBalanceHistory{AssetGuid: token.AssetGuid, SentSat: &bchain.Amount{}, ReceivedSat: &bchain.Amount{}}
						tokens[token.AssetGuid] = bhaToken
					}
					(*big.Int)(bhaToken.SentSat).Add((*big.Int)(bhaToken.SentSat), (*big.Int)(token.SentSat))
					(*big.Int)(bhaToken.ReceivedSat).Add((*big.Int)(bhaToken.ReceivedSat), (*big.Int)(token.ReceivedSat))
				}
			}
			(*big.Int)(bha.SentSat).Add((*big.Int)(bha.SentSat), (*big.Int)(bh.SentSat))
			(*big.Int)(bha.ReceivedSat).Add((*big.Int)(bha.ReceivedSat), (*big.Int)(bh.ReceivedSat))
		}
		if bha.Txs > 0 {
			bha.Txid = ""
			if tokens != nil {
				bha.Tokens = []*TokenBalanceHistory{}
				// then flatten to array of token balances from the map
				for _, token := range tokens {
					bha.Tokens = append(bha.Tokens, token)
				}
			}
			bhs = append(bhs, bha)
		}
	}
	return bhs
}

// Blocks is list of blocks with paging information
type Blocks struct {
	Paging
	Blocks []bchain.DbBlockInfo `json:"blocks"`
}

// BlockInfo contains extended block header data and a list of block txids
type BlockInfo struct {
	Hash          string      `json:"hash"`
	Prev          string      `json:"previousBlockHash,omitempty"`
	Next          string      `json:"nextBlockHash,omitempty"`
	Height        uint32      `json:"height"`
	Confirmations int         `json:"confirmations"`
	Size          int         `json:"size"`
	Time          int64       `json:"time,omitempty"`
	Version       json.Number `json:"version"`
	MerkleRoot    string      `json:"merkleRoot"`
	Nonce         string      `json:"nonce"`
	Bits          string      `json:"bits"`
	Difficulty    string      `json:"difficulty"`
	Txids         []string    `json:"tx,omitempty"`
}

// Block contains information about block
type Block struct {
	Paging
	BlockInfo
	TxCount      int   `json:"txCount"`
	Transactions []*Tx `json:"txs,omitempty"`
}

// BlockbookInfo contains information about the running blockbook instance
type BlockbookInfo struct {
	Coin              string                       `json:"coin"`
	Host              string                       `json:"host"`
	Version           string                       `json:"version"`
	GitCommit         string                       `json:"gitCommit"`
	BuildTime         string                       `json:"buildTime"`
	SyncMode          bool                         `json:"syncMode"`
	InitialSync       bool                         `json:"initialSync"`
	InSync            bool                         `json:"inSync"`
	BestHeight        uint32                       `json:"bestHeight"`
	LastBlockTime     time.Time                    `json:"lastBlockTime"`
	InSyncMempool     bool                         `json:"inSyncMempool"`
	LastMempoolTime   time.Time                    `json:"lastMempoolTime"`
	MempoolSize       int                          `json:"mempoolSize"`
	Decimals          int                          `json:"decimals"`
	DbSize            int64                        `json:"dbSize"`
	DbSizeFromColumns int64                        `json:"dbSizeFromColumns,omitempty"`
	DbColumns         []common.InternalStateColumn `json:"dbColumns,omitempty"`
	About             string                       `json:"about"`
}

// BackendInfo is used to get information about blockchain
type BackendInfo struct {
	BackendError    string  `json:"error,omitempty"`
	Chain           string  `json:"chain,omitempty"`
	Blocks          int     `json:"blocks,omitempty"`
	Headers         int     `json:"headers,omitempty"`
	BestBlockHash   string  `json:"bestBlockHash,omitempty"`
	Difficulty      string  `json:"difficulty,omitempty"`
	SizeOnDisk      int64   `json:"sizeOnDisk,omitempty"`
	Version         string  `json:"version,omitempty"`
	Subversion      string  `json:"subversion,omitempty"`
	ProtocolVersion string  `json:"protocolVersion,omitempty"`
	Timeoffset      float64 `json:"timeOffset,omitempty"`
	Warnings        string  `json:"warnings,omitempty"`
}

// SystemInfo contains information about the running blockbook and backend instance
type SystemInfo struct {
	Blockbook *BlockbookInfo `json:"blockbook"`
	Backend   *BackendInfo   `json:"backend"`
}

// MempoolTxid contains information about a transaction in mempool
type MempoolTxid struct {
	Time int64  `json:"time"`
	Txid string `json:"txid"`
}

// MempoolTxids contains a list of mempool txids with paging information
type MempoolTxids struct {
	Paging
	Mempool     []MempoolTxid `json:"mempool"`
	MempoolSize int           `json:"mempoolSize"`
}
