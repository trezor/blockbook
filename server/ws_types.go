package server

import "encoding/json"

type WsReq struct {
	ID     string          `json:"id"`
	Method string          `json:"method" ts_type:"'getAccountInfo' | 'getInfo' | 'getBlockHash'| 'getBlock' | 'getAccountUtxo' | 'getBalanceHistory' | 'getTransaction' | 'getTransactionSpecific' | 'estimateFee' | 'sendTransaction' | 'subscribeNewBlock' | 'unsubscribeNewBlock' | 'subscribeNewTransaction' | 'unsubscribeNewTransaction' | 'subscribeAddresses' | 'unsubscribeAddresses' | 'subscribeFiatRates' | 'unsubscribeFiatRates' | 'ping' | 'getCurrentFiatRates' | 'getFiatRatesForTimestamps' | 'getFiatRatesTickersList' | 'getMempoolFilters'"`
	Params json.RawMessage `json:"params" ts_type:"any"`
}

type WsRes struct {
	ID   string      `json:"id"`
	Data interface{} `json:"data"`
}

type WsAccountInfoReq struct {
	Descriptor        string `json:"descriptor"`
	Details           string `json:"details,omitempty" ts_type:"'basic' | 'tokens' | 'tokenBalances' | 'txids' | 'txslight' | 'txs'"`
	Tokens            string `json:"tokens,omitempty" ts_type:"'derived' | 'used' | 'nonzero'"`
	PageSize          int    `json:"pageSize,omitempty"`
	Page              int    `json:"page,omitempty"`
	FromHeight        int    `json:"from,omitempty"`
	ToHeight          int    `json:"to,omitempty"`
	ContractFilter    string `json:"contractFilter,omitempty"`
	SecondaryCurrency string `json:"secondaryCurrency,omitempty"`
	Gap               int    `json:"gap,omitempty"`
}

type WsBackendInfo struct {
	Version          string      `json:"version,omitempty"`
	Subversion       string      `json:"subversion,omitempty"`
	ConsensusVersion string      `json:"consensus_version,omitempty"`
	Consensus        interface{} `json:"consensus,omitempty"`
}

type WsInfoRes struct {
	Name       string        `json:"name"`
	Shortcut   string        `json:"shortcut"`
	Decimals   int           `json:"decimals"`
	Version    string        `json:"version"`
	BestHeight int           `json:"bestHeight"`
	BestHash   string        `json:"bestHash"`
	Block0Hash string        `json:"block0Hash"`
	Testnet    bool          `json:"testnet"`
	Backend    WsBackendInfo `json:"backend"`
}

type WsBlockHashReq struct {
	Height int `json:"height"`
}

type WsBlockHashRes struct {
	Hash string `json:"hash"`
}

type WsBlockReq struct {
	Id       string `json:"id"`
	PageSize int    `json:"pageSize,omitempty"`
	Page     int    `json:"page,omitempty"`
}

type WsAccountUtxoReq struct {
	Descriptor string `json:"descriptor"`
}

type WsBalanceHistoryReq struct {
	Descriptor string   `json:"descriptor"`
	From       int64    `json:"from,omitempty"`
	To         int64    `json:"to,omitempty"`
	Currencies []string `json:"currencies,omitempty"`
	Gap        int      `json:"gap,omitempty"`
	GroupBy    uint32   `json:"groupBy,omitempty"`
}

type WsTransactionReq struct {
	Txid string `json:"txid"`
}

type WsMempoolFiltersReq struct {
	ScriptType    string `json:"scriptType"`
	FromTimestamp uint32 `json:"fromTimestamp"`
	ParamM        uint64 `json:"M,omitempty"`
}

type WsBlockFilterReq struct {
	ScriptType string `json:"scriptType"`
	BlockHash  string `json:"blockHash"`
	ParamM     uint64 `json:"M,omitempty"`
}

type WsBlockFiltersBatchReq struct {
	ScriptType string `json:"scriptType"`
	BlockHash  string `json:"bestKnownBlockHash"`
	PageSize   int    `json:"pageSize,omitempty"`
	ParamM     uint64 `json:"M,omitempty"`
}

type WsTransactionSpecificReq struct {
	Txid string `json:"txid"`
}

type WsEstimateFeeReq struct {
	Blocks   []int                  `json:"blocks,omitempty"`
	Specific map[string]interface{} `json:"specific,omitempty" ts_type:"{conservative?: boolean;txsize?: number;from?: string;to?: string;data?: string;value?: string;}"`
}

type WsEstimateFeeRes struct {
	FeePerTx   string `json:"feePerTx,omitempty"`
	FeePerUnit string `json:"feePerUnit,omitempty"`
	FeeLimit   string `json:"feeLimit,omitempty"`
}

type WsSendTransactionReq struct {
	Hex string `json:"hex"`
}

type WsSubscribeAddressesReq struct {
	Addresses []string `json:"addresses"`
}
type WsSubscribeFiatRatesReq struct {
	Currency string   `json:"currency,omitempty"`
	Tokens   []string `json:"tokens,omitempty"`
}

type WsCurrentFiatRatesReq struct {
	Currencies []string `json:"currencies,omitempty"`
	Token      string   `json:"token,omitempty"`
}

type WsFiatRatesForTimestampsReq struct {
	Timestamps []int64  `json:"timestamps"`
	Currencies []string `json:"currencies,omitempty"`
	Token      string   `json:"token,omitempty"`
}

type WsFiatRatesTickersListReq struct {
	Timestamp int64  `json:"timestamp,omitempty"`
	Token     string `json:"token,omitempty"`
}
