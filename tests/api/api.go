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
	httpTimeout      = 20 * time.Second
	wsDialTimeout    = 10 * time.Second
	wsMessageTimeout = 15 * time.Second
	txSearchWindow   = 12
	blockPageSize    = 10
)

var testMap = map[string]func(t *testing.T, th *TestHandler){
	"Status":                 testStatus,
	"GetBlockIndex":          testGetBlockIndex,
	"GetBlockByHeight":       testGetBlockByHeight,
	"GetBlock":               testGetBlock,
	"GetTransaction":         testGetTransaction,
	"GetTransactionSpecific": testGetTransactionSpecific,
	"GetAddress":             testGetAddress,
	"GetAddressTxids":        testGetAddressTxids,
	"GetAddressTxs":          testGetAddressTxs,
	"GetUtxo":                testGetUtxo,
	"GetUtxoConfirmedFilter": testGetUtxoConfirmedFilter,
	"WsGetInfo":              testWsGetInfo,
	"WsGetBlockHash":         testWsGetBlockHash,
	"WsGetTransaction":       testWsGetTransaction,
	"WsGetAccountInfo":       testWsGetAccountInfo,
	"WsGetAccountUtxo":       testWsGetAccountUtxo,
	"WsPing":                 testWsPing,
}

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

	sampleTxResolved   bool
	sampleTxID         string
	sampleAddrResolved bool
	sampleAddress      string
}

type statusEnvelope struct {
	Blockbook json.RawMessage `json:"blockbook"`
	Backend   json.RawMessage `json:"backend"`
}

type statusBlockbook struct {
	BestHeight int `json:"bestHeight"`
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

type coinConfig struct {
	Coin struct {
		Alias string `json:"alias"`
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

	for _, test := range tests {
		if fn, found := testMap[test]; found {
			t.Run(test, func(t *testing.T) { fn(t, h) })
		} else {
			t.Errorf("%s: test not found", test)
		}
	}
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
