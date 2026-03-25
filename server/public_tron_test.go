//go:build unittest
// +build unittest

package server

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/golang/glog"
	"github.com/trezor/blockbook/bchain/coins/tron"
	"github.com/trezor/blockbook/tests/dbtestdata"
)

func httpTestsTron(t *testing.T, ts *httptest.Server) {
	tests := []httpTests{
		{
			name:        "explorerAddress " + dbtestdata.TronAddrTZ,
			r:           newGetRequest(ts.URL + "/address/" + dbtestdata.TronAddrTZ),
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body: []string{
				`<span class="ellipsis copyable">TZEZWXYQS44388xBoMhQdpL1HrBZFLfDpt</span>`,
				`<h5>Resources</h5>`,
				`Bandwidth</td><td>255 / 1`,
				`Energy</td><td>25`,
			},
		},
		{
			name:        "apiBlock",
			r:           newGetRequest(ts.URL + "/api/v2/block/" + strconv.Itoa(dbtestdata.Block1)),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`"hash":"11223344556677889900aabbccddeeff11223344556677889900aabbccddeeff"`,
				`"previousBlockHash":"0000000000000000000000000000000000000000000000000000000000000000"`,
				`"txid":"a431984fef1d014620504d02f821f872221cf44c250a81a31e81fa4855b2b302"`,
				`"coinSpecificData":{"tx":{"nonce":"0x0"`,
				`"hash":"a431984fef1d014620504d02f821f872221cf44c250a81a31e81fa4855b2b302"`,
				`"chainExtraData":{"contractType":"TriggerSmartContract","operation":"contractCall","assetIssueID":"1002001","totalFee":"3076500","energyUsage":"14650","energyUsageTotal":"14650","bandwidthUsage":"345","bandwidthFee":"0","result":"SUCCESS"}`,
				`"tokenTransfers":[{"type":"TRC20"`,
				`"ethereumSpecific":{"status":1`,
				`"addressAliases":{"TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf":{"Type":"Contract","Alias":"TronTestContract236"}}`,
			},
		},
		{
			name:        "apiBlock non-existent",
			r:           newGetRequest(ts.URL + "/api/v2/block/12345678910"),
			status:      http.StatusBadRequest,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"error":"Block not found"}`,
			},
		},
		{
			name:        "apiTx",
			r:           newGetRequest(ts.URL + "/api/v2/tx/" + dbtestdata.TronTx1Id),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`"txid":"a431984fef1d014620504d02f821f872221cf44c250a81a31e81fa4855b2b302"`,
				`"coinSpecificData":{"tx":{"nonce":"0x0"`,
				`"hash":"a431984fef1d014620504d02f821f872221cf44c250a81a31e81fa4855b2b302"`,
				`"chainExtraData":{"contractType":"TriggerSmartContract","operation":"contractCall","assetIssueID":"1002001","totalFee":"3076500","energyUsage":"14650","energyUsageTotal":"14650","bandwidthUsage":"345","bandwidthFee":"0","result":"SUCCESS"}`,
				`"tokenTransfers":[{"type":"TRC20"`,
				`"ethereumSpecific":{"status":1`,
				`"addressAliases":{"TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf":{"Type":"Contract","Alias":"TronTestContract236"}}`,
			},
		},
		{
			name:        "apiTx non-existent",
			r:           newGetRequest(ts.URL + "/api/v2/tx/0x123456789"),
			status:      http.StatusBadRequest,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"error":"Transaction '0x123456789' not found"}`,
			},
		},
		{
			name:        "apiAddress TronAddrTJ",
			r:           newGetRequest(ts.URL + "/api/v2/address/" + dbtestdata.TronAddrTZ),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"page":1,"totalPages":1,"itemsOnPage":1000,"address":"TZEZWXYQS44388xBoMhQdpL1HrBZFLfDpt","balance":"123450255","unconfirmedBalance":"0","unconfirmedTxs":0,"txs":1,"nonTokenTxs":1,"internalTxs":1,"txids":["a431984fef1d014620504d02f821f872221cf44c250a81a31e81fa4855b2b302"],"nonce":"255","tokens":[{"type":"TRC20","standard":"TRC20","name":"TronTestContract236","contract":"TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf","transfers":1,"symbol":"TRC236","decimals":6,"balance":"1000255236"}],"chainExtraData":{"payloadType":"tron","payload":{"availableBandwidth":255,"totalBandwidth":1255,"availableEnergy":25500,"totalEnergy":35500}}}`,
			},
		},
		{
			name:        "apiAddress TronAddrTX",
			r:           newGetRequest(ts.URL + "/api/v2/address/" + dbtestdata.TronAddrTD + "?details=txs"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`"address":"TDGSR64oU4QDpViKfdwawSiqwyqpUB6JUD"`,
				`"transactions":[{"txid":"a431984fef1d014620504d02f821f872221cf44c250a81a31e81fa4855b2b302"`,
				`"chainExtraData":{"contractType":"TriggerSmartContract","operation":"contractCall","assetIssueID":"1002001","totalFee":"3076500","energyUsage":"14650","energyUsageTotal":"14650","bandwidthUsage":"345","bandwidthFee":"0","result":"SUCCESS"}`,
				`"nonce":"36"`,
				`"tokens":[{"type":"TRC20","standard":"TRC20","name":"TronTestContract236"`,
				`"addressAliases":{"TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf":{"Type":"Contract","Alias":"TronTestContract236"}}`,
			},
		},
		{
			name:        "apiAddress TronAddrContractTX1",
			r:           newGetRequest(ts.URL + "/api/v2/address/" + dbtestdata.TronAddrContractTX1 + "?details=txs"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`"address":"TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf"`,
				`"transactions":[{"txid":"a431984fef1d014620504d02f821f872221cf44c250a81a31e81fa4855b2b302"`,
				`"chainExtraData":{"contractType":"TriggerSmartContract","operation":"contractCall","assetIssueID":"1002001","totalFee":"3076500","energyUsage":"14650","energyUsageTotal":"14650","bandwidthUsage":"345","bandwidthFee":"0","result":"SUCCESS"}`,
				`"nonce":"236"`,
				`"contractInfo":{"type":"TRC20","standard":"TRC20","contract":"TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf","name":"TronTestContract236","symbol":"TRC236","decimals":6,"createdInBlock":1000}`,
				`"addressAliases":{"TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf":{"Type":"Contract","Alias":"TronTestContract236"}}`,
			},
		},
		{
			name:        "apiIndex",
			r:           newGetRequest(ts.URL + "/api"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"blockbook":{"coin":"Fakecoin"`,
				`"bestHeight":100000`,
				`"decimals":6`,
				`"backend":{"chain":"fakecoin","blocks":2,"headers":2,"bestBlockHash":"11223344556677889900aabbccddeeff11223344556677889900aabbccddeeff"`,
				`"version":"tron_test_1.0","subversion":"MockTron"`,
			},
		},
	}

	performHttpTests(tests, t, ts)
}

var websocketTestsTron = []websocketTest{
	{
		name: "websocket getInfo",
		req: websocketReq{
			Method: "getInfo",
		},
		want: `{"id":"0","data":{"name":"Fakecoin","shortcut":"TRX","network":"TRX","decimals":6,"version":"unknown","bestHeight":100000,"bestHash":"11223344556677889900aabbccddeeff11223344556677889900aabbccddeeff","block0Hash":"","testnet":true,"backend":{"version":"tron_test_1.0","subversion":"MockTron"}}}`,
	},
	{
		name: "websocket rpcCall",
		req: websocketReq{
			Method: "rpcCall",
			Params: WsRpcCallReq{
				To:   dbtestdata.TronAddrContractTX1,
				Data: "0x4567",
			},
		},
		want: `{"id":"1","data":{"data":"0x4567abcd"}}`,
	},
	{
		name: "websocket getAccountInfo address",
		req: websocketReq{
			Method: "getAccountInfo",
			Params: map[string]interface{}{
				"descriptor": dbtestdata.TronAddrTZ,
				"details":    "txids",
			},
		},
		want: `{"id":"2","data":{"page":1,"totalPages":1,"itemsOnPage":25,"address":"TZEZWXYQS44388xBoMhQdpL1HrBZFLfDpt","balance":"123450255","unconfirmedBalance":"0","unconfirmedTxs":0,"txs":1,"nonTokenTxs":1,"internalTxs":1,"txids":["a431984fef1d014620504d02f821f872221cf44c250a81a31e81fa4855b2b302"],"nonce":"255","tokens":[{"type":"TRC20","standard":"TRC20","name":"TronTestContract236","contract":"TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf","transfers":1,"symbol":"TRC236","decimals":6,"balance":"1000255236"}],"chainExtraData":{"payloadType":"tron","payload":{"availableBandwidth":255,"totalBandwidth":1255,"availableEnergy":25500,"totalEnergy":35500}}}}`,
	},
}

func Test_PublicServer_Tron(t *testing.T) {
	timeNow = fixedTimeNow
	parser := tron.NewTronParser(1, true)
	chain, err := dbtestdata.NewFakeBlockChainTronType(parser)
	if err != nil {
		glog.Fatal("fakechain: ", err)
	}

	s, dbpath := setupPublicHTTPServer(parser, chain, t, false)
	defer closeAndDestroyPublicServer(t, s, dbpath)
	s.is.CoinShortcut = "TRX"
	s.templates = s.parseTemplates()
	s.ConnectFullPublicInterface()

	ts := httptest.NewServer(s.https.Handler)
	defer ts.Close()

	httpTestsTron(t, ts)
	runWebsocketTests(t, ts, websocketTestsTron)
}
