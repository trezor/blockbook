package server

import (
	"encoding/json"

	"github.com/trezor/blockbook/api"
)

// WsReq represents a generic WebSocket request with an ID, method, and raw parameters.
type WsReq struct {
	ID     string          `json:"id" ts_doc:"Unique request identifier."`
	Method string          `json:"method" ts_type:"'getAccountInfo' | 'getInfo' | 'getBlockHash'| 'getBlock' | 'getAccountUtxo' | 'getBalanceHistory' | 'getTransaction' | 'getTransactionSpecific' | 'estimateFee' | 'sendTransaction' | 'subscribeNewBlock' | 'unsubscribeNewBlock' | 'subscribeNewTransaction' | 'unsubscribeNewTransaction' | 'subscribeAddresses' | 'unsubscribeAddresses' | 'subscribeFiatRates' | 'unsubscribeFiatRates' | 'ping' | 'getCurrentFiatRates' | 'getFiatRatesForTimestamps' | 'getFiatRatesTickersList' | 'getMempoolFilters'" ts_doc:"Requested method name."`
	Params json.RawMessage `json:"params" ts_type:"any" ts_doc:"Parameters for the requested method in raw JSON format."`
}

// WsRes represents a generic WebSocket response with an ID and arbitrary data.
type WsRes struct {
	ID   string      `json:"id" ts_doc:"Corresponding request identifier."`
	Data interface{} `json:"data" ts_doc:"Payload of the response, structure depends on the request."`
}

// WsAccountInfoReq carries parameters for the 'getAccountInfo' method.
type WsAccountInfoReq struct {
	Descriptor        string `json:"descriptor" ts_doc:"Address or XPUB descriptor to query."`
	Details           string `json:"details,omitempty" ts_type:"'basic' | 'tokens' | 'tokenBalances' | 'txids' | 'txslight' | 'txs'" ts_doc:"Level of detail to retrieve about the account."`
	Tokens            string `json:"tokens,omitempty" ts_type:"'derived' | 'used' | 'nonzero'" ts_doc:"Which tokens to include in the account info."`
	PageSize          int    `json:"pageSize,omitempty" ts_doc:"Number of items per page, if paging is used."`
	Page              int    `json:"page,omitempty" ts_doc:"Requested page index, if paging is used."`
	FromHeight        int    `json:"from,omitempty" ts_doc:"Starting block height for transaction filtering."`
	ToHeight          int    `json:"to,omitempty" ts_doc:"Ending block height for transaction filtering."`
	ContractFilter    string `json:"contractFilter,omitempty" ts_doc:"Filter by specific contract address (for token data)."`
	SecondaryCurrency string `json:"secondaryCurrency,omitempty" ts_doc:"Currency code to convert values into (e.g. 'USD')."`
	Gap               int    `json:"gap,omitempty" ts_doc:"Gap limit for XPUB scanning, if relevant."`
}

// WsBackendInfo holds extended info about the connected backend node.
type WsBackendInfo struct {
	Version          string      `json:"version,omitempty" ts_doc:"Backend version string."`
	Subversion       string      `json:"subversion,omitempty" ts_doc:"Backend sub-version string."`
	ConsensusVersion string      `json:"consensus_version,omitempty" ts_doc:"Consensus protocol version in use."`
	Consensus        interface{} `json:"consensus,omitempty" ts_doc:"Additional consensus details, structure depends on blockchain."`
}

// WsInfoRes is returned by 'getInfo' requests, containing basic blockchain info.
type WsInfoRes struct {
	Name       string        `json:"name" ts_doc:"Human-readable blockchain name."`
	Shortcut   string        `json:"shortcut" ts_doc:"Short code for the blockchain (e.g. BTC, ETH)."`
	Network    string        `json:"network" ts_doc:"Network identifier (e.g. mainnet, testnet)."`
	Decimals   int           `json:"decimals" ts_doc:"Number of decimals in the base unit of the coin."`
	Version    string        `json:"version" ts_doc:"Version of the blockbook or backend service."`
	BestHeight int           `json:"bestHeight" ts_doc:"Current best chain height according to the backend."`
	BestHash   string        `json:"bestHash" ts_doc:"Block hash of the best (latest) block."`
	Block0Hash string        `json:"block0Hash" ts_doc:"Genesis block hash or identifier."`
	Testnet    bool          `json:"testnet" ts_doc:"Indicates if this is a test network."`
	Backend    WsBackendInfo `json:"backend" ts_doc:"Additional backend-related information."`
}

// WsBlockHashReq holds a single integer for querying the block hash at that height.
type WsBlockHashReq struct {
	Height int `json:"height" ts_doc:"Block height for which the hash is requested."`
}

// WsBlockHashRes returns the block hash for a requested height.
type WsBlockHashRes struct {
	Hash string `json:"hash" ts_doc:"Block hash at the requested height."`
}

// WsBlockReq is used to request details of a block (by ID) with paging options.
type WsBlockReq struct {
	Id       string `json:"id" ts_doc:"Block identifier (hash)."`
	PageSize int    `json:"pageSize,omitempty" ts_doc:"Number of transactions per page in the block."`
	Page     int    `json:"page,omitempty" ts_doc:"Page index to retrieve if multiple pages of transactions are available."`
}

// WsAccountUtxoReq is used to request unspent transaction outputs (UTXOs) for a given xpub/address.
type WsAccountUtxoReq struct {
	Descriptor string `json:"descriptor" ts_doc:"Address or XPUB descriptor to retrieve UTXOs for."`
}

// WsBalanceHistoryReq is used to retrieve a historical balance chart or intervals for an account.
type WsBalanceHistoryReq struct {
	Descriptor string   `json:"descriptor" ts_doc:"Address or XPUB descriptor to query history for."`
	From       int64    `json:"from,omitempty" ts_doc:"Unix timestamp from which to start the history."`
	To         int64    `json:"to,omitempty" ts_doc:"Unix timestamp at which to end the history."`
	Currencies []string `json:"currencies,omitempty" ts_doc:"List of currency codes for which to fetch exchange rates at each interval."`
	Gap        int      `json:"gap,omitempty" ts_doc:"Gap limit for XPUB scanning, if relevant."`
	GroupBy    uint32   `json:"groupBy,omitempty" ts_doc:"Size of each aggregated time window in seconds."`
}

// WsTransactionReq requests details for a specific transaction by its txid.
type WsTransactionReq struct {
	Txid string `json:"txid" ts_doc:"Transaction ID to retrieve details for."`
}

// WsMempoolFiltersReq requests mempool filters for scripts of a specific type, after a given timestamp.
type WsMempoolFiltersReq struct {
	ScriptType    string `json:"scriptType" ts_doc:"Type of script we are filtering for (e.g., P2PKH, P2SH)."`
	FromTimestamp uint32 `json:"fromTimestamp" ts_doc:"Only retrieve filters for mempool txs after this timestamp."`
	ParamM        uint64 `json:"M,omitempty" ts_doc:"Optional parameter for certain filter logic (e.g., n-bloom)."`
}

// WsBlockFilterReq requests a filter for a given block hash and script type.
type WsBlockFilterReq struct {
	ScriptType string `json:"scriptType" ts_doc:"Type of script filter (e.g., P2PKH, P2SH)."`
	BlockHash  string `json:"blockHash" ts_doc:"Block hash for which we want the filter."`
	ParamM     uint64 `json:"M,omitempty" ts_doc:"Optional parameter for certain filter logic."`
}

// WsBlockFiltersBatchReq is used to request batch filters for consecutive blocks.
type WsBlockFiltersBatchReq struct {
	ScriptType string `json:"scriptType" ts_doc:"Type of script filter (e.g., P2PKH, P2SH)."`
	BlockHash  string `json:"bestKnownBlockHash" ts_doc:"Hash of the latest known block. Filters will be retrieved backward from here."`
	PageSize   int    `json:"pageSize,omitempty" ts_doc:"Number of block filters per request."`
	ParamM     uint64 `json:"M,omitempty" ts_doc:"Optional parameter for certain filter logic."`
}

// WsTransactionSpecificReq requests blockchain-specific transaction info that might go beyond standard fields.
type WsTransactionSpecificReq struct {
	Txid string `json:"txid" ts_doc:"Transaction ID for the detailed blockchain-specific data."`
}

// WsEstimateFeeReq requests an estimation of transaction fees for a set of blocks or with specific parameters.
type WsEstimateFeeReq struct {
	Blocks   []int                  `json:"blocks,omitempty" ts_doc:"Block confirmations targets for which fees should be estimated."`
	Specific map[string]interface{} `json:"specific,omitempty" ts_type:"{conservative?: boolean; txsize?: number; from?: string; to?: string; data?: string; value?: string;}" ts_doc:"Additional chain-specific parameters (e.g. for Ethereum)."`
}

// WsEstimateFeeRes is returned in response to a fee estimation request.
type WsEstimateFeeRes struct {
	FeePerTx   string           `json:"feePerTx,omitempty" ts_doc:"Estimated total fee per transaction, if relevant."`
	FeePerUnit string           `json:"feePerUnit,omitempty" ts_doc:"Estimated fee per unit (sat/byte, Wei/gas, etc.)."`
	FeeLimit   string           `json:"feeLimit,omitempty" ts_doc:"Max fee limit for blockchains like Ethereum."`
	Eip1559    *api.Eip1559Fees `json:"eip1559,omitempty"`
}

// WsLongTermFeeRateRes is returned in response to a long term fee rate request.
type WsLongTermFeeRateRes struct {
	FeePerUnit string `json:"feePerUnit" ts_doc:"Long term fee rate (in sat/kByte)."`
	Blocks     uint64 `json:"blocks" ts_doc:"Amount of blocks used for the long term fee rate estimation."`
}

// WsSendTransactionReq is used to broadcast a transaction to the network.
type WsSendTransactionReq struct {
	Hex                   string `json:"hex,omitempty" ts_doc:"Hex-encoded transaction data to broadcast (string format)."`
	DisableAlternativeRPC bool   `json:"disableAlternativeRpc" ts_doc:"Use alternative RPC method to broadcast transaction."`
}

// WsSubscribeAddressesReq is used to subscribe to updates on a list of addresses.
type WsSubscribeAddressesReq struct {
	Addresses []string `json:"addresses" ts_doc:"List of addresses to subscribe for updates (e.g., new transactions)."`
}

// WsSubscribeFiatRatesReq subscribes to updates of fiat rates for a specific currency or set of tokens.
type WsSubscribeFiatRatesReq struct {
	Currency string   `json:"currency,omitempty" ts_doc:"Fiat currency code (e.g. 'USD')."`
	Tokens   []string `json:"tokens,omitempty" ts_doc:"List of token symbols or IDs to get fiat rates for."`
}

// WsCurrentFiatRatesReq requests the current fiat rates for specified currencies (and optionally a token).
type WsCurrentFiatRatesReq struct {
	Currencies []string `json:"currencies,omitempty" ts_doc:"List of fiat currencies, e.g. ['USD','EUR']."`
	Token      string   `json:"token,omitempty" ts_doc:"Token symbol or ID if asking for token fiat rates (e.g. 'ETH')."`
}

// WsFiatRatesForTimestampsReq requests historical fiat rates for given timestamps.
type WsFiatRatesForTimestampsReq struct {
	Timestamps []int64  `json:"timestamps" ts_doc:"List of Unix timestamps for which to retrieve fiat rates."`
	Currencies []string `json:"currencies,omitempty" ts_doc:"List of fiat currencies, e.g. ['USD','EUR']."`
	Token      string   `json:"token,omitempty" ts_doc:"Token symbol or ID if asking for token fiat rates."`
}

// WsFiatRatesTickersListReq requests a list of tickers for a given timestamp (and possibly a token).
type WsFiatRatesTickersListReq struct {
	Timestamp int64  `json:"timestamp,omitempty" ts_doc:"Timestamp for which the list of available tickers is needed."`
	Token     string `json:"token,omitempty" ts_doc:"Token symbol or ID if asking for token-specific fiat rates."`
}

// WsRpcCallReq is used for raw RPC calls (for example, on an Ethereum-like backend).
type WsRpcCallReq struct {
	From string `json:"from,omitempty" ts_doc:"Address from which the RPC call is originated (if relevant)."`
	To   string `json:"to" ts_doc:"Contract or address to which the RPC call is made."`
	Data string `json:"data" ts_doc:"Hex-encoded call data (function signature + parameters)."`
}

// WsRpcCallRes returns the result of an RPC call in hex form.
type WsRpcCallRes struct {
	Data string `json:"data" ts_doc:"Hex-encoded return data from the call."`
}
