//go:build integration

package api

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/trezor/blockbook/bchain"
)

const (
	httpTimeout         = 30 * time.Second
	wsDialTimeout       = 10 * time.Second
	wsMessageTimeout    = 15 * time.Second
	txSearchWindow      = 12
	blockPageSize       = 1
	sampleBlockPageSize = 3
	sampleBlockProbeMax = 3
	sciNotationWindow   = 40
	sciNotationTxLimit  = 8
)

type testCapability uint8

const (
	capabilityNone testCapability = 0
	capabilityUTXO testCapability = 1 << iota
	capabilityEVM
)

type testDefinition struct {
	fn       func(t *testing.T, th *TestHandler)
	required testCapability
	group    string
}

var commonTests = map[string]func(t *testing.T, th *TestHandler){
	"Status":                          testStatus,
	"GetBlockIndex":                   testGetBlockIndex,
	"GetBlockByHeight":                testGetBlockByHeight,
	"GetBlock":                        testGetBlock,
	"GetTransaction":                  testGetTransaction,
	"GetTransactionSpecific":          testGetTransactionSpecific,
	"GetAddress":                      testGetAddress,
	"GetAddressTxids":                 testGetAddressTxids,
	"GetAddressTxs":                   testGetAddressTxs,
	"GetAddressTxsScientificNotation": testGetAddressTxsScientificNotation,
	"GetCurrentFiatRates":             testGetCurrentFiatRates,
	"GetTickersList":                  testGetTickersList,
	"GetMultiTickers":                 testGetMultiTickers,
}

var utxoOnlyTests = map[string]func(t *testing.T, th *TestHandler){
	"GetUtxo":                testGetUtxo,
	"GetUtxoConfirmedFilter": testGetUtxoConfirmedFilter,
}

var evmOnlyTests = map[string]func(t *testing.T, th *TestHandler){
	"GetAddressBasicEVM":                  testGetAddressBasicEVM,
	"GetAddressTokensEVM":                 testGetAddressTokensEVM,
	"GetAddressTokenBalances":             testGetAddressTokenBalances,
	"GetAddressIncludeErc4626EVM":         testGetAddressIncludeErc4626EVM,
	"GetAddressTxidsPaginationEVM":        testGetAddressTxidsPaginationEVM,
	"GetAddressTxsPaginationEVM":          testGetAddressTxsPaginationEVM,
	"GetAddressContractFilterEVM":         testGetAddressContractFilterEVM,
	"GetTransactionEVMShape":              testGetTransactionEVMShape,
	"WsGetAccountInfoBasicEVM":            testWsGetAccountInfoBasicEVM,
	"WsGetAccountInfoEVM":                 testWsGetAccountInfoEVM,
	"WsGetAccountInfoTxidsConsistencyEVM": testWsGetAccountInfoTxidsConsistencyEVM,
	"WsGetAccountInfoTxsConsistencyEVM":   testWsGetAccountInfoTxsConsistencyEVM,
	"WsGetAccountInfoContractFilterEVM":   testWsGetAccountInfoContractFilterEVM,
	"WsGetAccountInfoIncludeErc4626EVM":   testWsGetAccountInfoIncludeErc4626EVM,
}

var wsOnlyTests = map[string]func(t *testing.T, th *TestHandler){
	"WsGetInfo":        testWsGetInfo,
	"WsGetBlockHash":   testWsGetBlockHash,
	"WsGetTransaction": testWsGetTransaction,
	"WsGetAccountInfo": testWsGetAccountInfo,
	"WsPing":           testWsPing,
}

var wsUTXOTests = map[string]func(t *testing.T, th *TestHandler){
	"WsGetAccountUtxo": testWsGetAccountUtxo,
}

var testRegistry = buildTestRegistry()

type TestHandler struct {
	Coin      string
	HTTPBase  string
	WSURL     string
	HTTP      *http.Client
	status    *statusBlockbook
	nextWSReq int

	blockHashByHeight map[int]string
	blockByHash       map[string]*blockSummary
	txByID            map[string]*txDetailResponse

	sampleTxResolved       bool
	sampleTxID             string
	sampleAddrResolved     bool
	sampleAddress          string
	sampleIndexResolved    bool
	sampleIndexHeight      int
	sampleIndexHash        string
	sampleBlockResolved    bool
	sampleBlockHeight      int
	sampleBlockHash        string
	sampleContractResolved bool
	sampleContract         string
	sampleFiatResolved     bool
	sampleFiatAvailable    bool
	sampleFiatTicker       fiatTickerResponse
	sampleSciAddrResolved  bool
	sampleSciAddress       string
	sampleSciTxID          string
	sampleSciHeight        int

	capabilitiesResolved bool
	supportsUTXO         bool
	utxoProbeMessage     string
	supportsEVM          bool
	evmProbeMessage      string
}

type statusEnvelope struct {
	Blockbook json.RawMessage `json:"blockbook"`
	Backend   json.RawMessage `json:"backend"`
}

type statusBlockbook struct {
	BestHeight           int        `json:"bestHeight"`
	HasFiatRates         bool       `json:"hasFiatRates"`
	CurrentFiatRatesTime *time.Time `json:"currentFiatRatesTime"`
}

type blockIndexResponse struct {
	BlockHash string `json:"blockHash"`
}

type blockResponse struct {
	Hash   string            `json:"hash"`
	Height int               `json:"height"`
	Txs    []json.RawMessage `json:"txs"`
}

type blockSummary struct {
	Hash       string
	Height     int
	HasTxField bool
	TxIDs      []string
}

type txPart struct {
	Addresses []string `json:"addresses"`
}

type txDetailResponse struct {
	Txid string   `json:"txid"`
	Vin  []txPart `json:"vin"`
	Vout []txPart `json:"vout"`
}

type addressResponse struct {
	Address string `json:"address"`
}

type addressTxidsResponse struct {
	Address     string   `json:"address"`
	Page        int      `json:"page"`
	ItemsOnPage int      `json:"itemsOnPage"`
	TotalPages  int      `json:"totalPages"`
	Txs         int      `json:"txs"`
	Txids       []string `json:"txids"`
}

type addressTxsResponse struct {
	Address      string             `json:"address"`
	Page         int                `json:"page"`
	ItemsOnPage  int                `json:"itemsOnPage"`
	TotalPages   int                `json:"totalPages"`
	Txs          int                `json:"txs"`
	Transactions []txDetailResponse `json:"transactions"`
}

type utxoResponse struct {
	Txid          string `json:"txid"`
	Vout          int    `json:"vout"`
	Value         string `json:"value"`
	Confirmations int    `json:"confirmations"`
	Height        int    `json:"height"`
}

type fiatTickerResponse struct {
	Timestamp int64              `json:"ts"`
	Rates     map[string]float32 `json:"rates"`
}

type availableVsCurrenciesResponse struct {
	Timestamp int64    `json:"ts"`
	Tickers   []string `json:"available_currencies"`
}

type wsRequest struct {
	ID     string      `json:"id"`
	Method string      `json:"method"`
	Params interface{} `json:"params"`
}

type wsResponse struct {
	ID   string          `json:"id"`
	Data json.RawMessage `json:"data"`
}

type wsInfoResponse struct {
	BestHeight int    `json:"bestHeight"`
	BestHash   string `json:"bestHash"`
}

type wsBlockHashResponse struct {
	Hash string `json:"hash"`
}

type evmAddressTokenBalanceResponse struct {
	Address     string             `json:"address"`
	Balance     string             `json:"balance"`
	Nonce       string             `json:"nonce"`
	Txs         int                `json:"txs"`
	NonTokenTxs int                `json:"nonTokenTxs"`
	Tokens      []evmTokenResponse `json:"tokens"`
}

type evmTokenResponse struct {
	Type             string               `json:"type"`
	Standard         string               `json:"standard"`
	Contract         string               `json:"contract"`
	Balance          string               `json:"balance"`
	IDs              []string             `json:"ids"`
	MultiTokenValues []evmMultiTokenValue `json:"multiTokenValues"`
	Erc4626          *evmErc4626Response  `json:"erc4626,omitempty"`
}

type evmMultiTokenValue struct {
	ID    string `json:"id"`
	Value string `json:"value"`
}

type evmErc4626Response struct {
	Asset                 *evmErc4626MetadataResponse `json:"asset,omitempty"`
	Share                 *evmErc4626MetadataResponse `json:"share,omitempty"`
	TotalAssets           string                      `json:"totalAssets,omitempty"`
	ConvertToAssets1Share string                      `json:"convertToAssets1Share,omitempty"`
	ConvertToShares1Asset string                      `json:"convertToShares1Asset,omitempty"`
	PreviewDeposit1Asset  string                      `json:"previewDeposit1Asset,omitempty"`
	PreviewRedeem1Share   string                      `json:"previewRedeem1Share,omitempty"`
	Error                 string                      `json:"error,omitempty"`
}

type evmErc4626MetadataResponse struct {
	Contract string `json:"contract"`
	Name     string `json:"name"`
	Symbol   string `json:"symbol"`
	Decimals int    `json:"decimals"`
}

type evmTxShapeResponse struct {
	Txid             string          `json:"txid"`
	Vin              []txPart        `json:"vin"`
	Vout             []txPart        `json:"vout"`
	EthereumSpecific json.RawMessage `json:"ethereumSpecific"`
}

type coinConfig struct {
	Coin struct {
		Alias    string `json:"alias"`
		TestName string `json:"test_name"`
	} `json:"coin"`
	Ports struct {
		BlockbookPublic int `json:"blockbook_public"`
	} `json:"ports"`
}

type apiEndpoints struct {
	HTTP string
	WS   string
}

func IntegrationTest(t *testing.T, coin string, _ bchain.BlockChain, _ bchain.Mempool, testConfig json.RawMessage) {
	tests, err := getTests(testConfig)
	if err != nil {
		t.Fatalf("failed loading api test list: %v", err)
	}

	endpoints, err := resolveAPIEndpoints(coin)
	if err != nil {
		t.Fatalf("resolve API endpoints for %s: %v", coin, err)
	}

	h := &TestHandler{
		Coin:              coin,
		HTTPBase:          endpoints.HTTP,
		WSURL:             endpoints.WS,
		HTTP:              newHTTPClient(),
		blockHashByHeight: make(map[int]string),
		blockByHash:       make(map[string]*blockSummary),
		txByID:            make(map[string]*txDetailResponse),
	}
	// Fail fast once per coin if the API endpoint is unavailable. Without this,
	// each subtest retries independently and can make CI appear hung.
	_ = h.getStatus(t)

	for _, test := range tests {
		if td, found := testRegistry[test]; found {
			t.Run(test, func(t *testing.T) {
				if !h.requireCapabilities(t, td.required, td.group, test) {
					return
				}
				td.fn(t, h)
			})
		} else {
			t.Errorf("%s: test not found", test)
		}
	}
}

func buildTestRegistry() map[string]testDefinition {
	registry := make(map[string]testDefinition, len(commonTests)+len(utxoOnlyTests)+len(evmOnlyTests)+len(wsOnlyTests)+len(wsUTXOTests))
	addGroup := func(group string, required testCapability, tests map[string]func(t *testing.T, th *TestHandler)) {
		for name, fn := range tests {
			if _, found := registry[name]; found {
				panic("duplicate api test definition: " + name)
			}
			registry[name] = testDefinition{
				fn:       fn,
				required: required,
				group:    group,
			}
		}
	}

	addGroup("common", capabilityNone, commonTests)
	addGroup("utxo-only", capabilityUTXO, utxoOnlyTests)
	addGroup("evm-only", capabilityEVM, evmOnlyTests)
	addGroup("ws-only", capabilityNone, wsOnlyTests)
	addGroup("ws-utxo", capabilityUTXO, wsUTXOTests)
	return registry
}

func getTests(cfg json.RawMessage) ([]string, error) {
	var v []string
	if err := json.Unmarshal(cfg, &v); err != nil {
		return nil, err
	}
	if len(v) == 0 {
		return nil, errors.New("no tests declared")
	}
	return v, nil
}

func newHTTPClient() *http.Client {
	return &http.Client{
		Timeout: httpTimeout,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}
}
