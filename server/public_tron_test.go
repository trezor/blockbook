//go:build unittest
// +build unittest

package server

import (
	"github.com/golang/glog"
	"github.com/trezor/blockbook/bchain/coins/tron"
	"github.com/trezor/blockbook/tests/dbtestdata"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func httpTestsTron(t *testing.T, ts *httptest.Server) {
	tests := []httpTests{
		{
			name:        "apiBlock",
			r:           newGetRequest(ts.URL + "/api/v2/block/" + strconv.Itoa(dbtestdata.Block1)),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"page":1,"totalPages":1,"itemsOnPage":1000,"hash":"0x11223344556677889900aabbccddeeff11223344556677889900aabbccddeeff","previousBlockHash":"0x0000000000000000000000000000000000000000000000000000000000000000","height":100000,"confirmations":99,"size":12345,"time":1677700000,"version":0,"merkleRoot":"","nonce":"","bits":"","difficulty":"","txCount":1,"txs":[{"txid":"0xa431984fef1d014620504d02f821f872221cf44c250a81a31e81fa4855b2b302","vin":[{"n":0,"addresses":["TZEZWXYQS44388xBoMhQdpL1HrBZFLfDpt"],"isAddress":true}],"vout":[{"value":"0","n":0,"addresses":["TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf"],"isAddress":true}],"blockHeight":-1,"confirmations":0,"blockTime":0,"value":"0","fees":"3076500","rbf":true,"coinSpecificData":{"tx":{"nonce":"0x0","gasPrice":"0xd2","gas":"0x393a","to":"TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf","value":"0x0","input":"0xa9059cbb000000000000000000000000242aa579f130bf6fea5eac12aa6b846fb8b293ab0000000000000000000000000000000000000000000000000000000000ab604e","hash":"0xa431984fef1d014620504d02f821f872221cf44c250a81a31e81fa4855b2b302","blockNumber":"0x348d2a7","from":"TZEZWXYQS44388xBoMhQdpL1HrBZFLfDpt","transactionIndex":"0x0"},"receipt":{"gasUsed":"0x393a","status":"0x1","logs":[{"address":"TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf","topics":["0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef","0x000000000000000000000000ff324071970b2b08822caa310c1bb458e63a5033","0x000000000000000000000000242aa579f130bf6fea5eac12aa6b846fb8b293ab"],"data":"0x0000000000000000000000000000000000000000000000000000000000ab604e"}]}},"tokenTransfers":[{"type":"TRC20","standard":"TRC20","from":"TZEZWXYQS44388xBoMhQdpL1HrBZFLfDpt","to":"TDGSR64oU4QDpViKfdwawSiqwyqpUB6JUD","contract":"TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf","name":"TronTestContract236","symbol":"TRC236","decimals":6,"value":"11231310"}],"ethereumSpecific":{"status":1,"nonce":0,"gasLimit":14650,"gasUsed":14650,"gasPrice":"210","data":"0xa9059cbb000000000000000000000000242aa579f130bf6fea5eac12aa6b846fb8b293ab0000000000000000000000000000000000000000000000000000000000ab604e","parsedData":{"methodId":"0xa9059cbb","name":"Transfer","function":"transfer(address, uint256)","params":[{"type":"address","values":["TDGSR64oU4QDpViKfdwawSiqwyqpUB6JUD"]},{"type":"uint256","values":["11231310"]}]}}}],"addressAliases":{"TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf":{"Type":"Contract","Alias":"TronTestContract236"}}}`,
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
				`{"txid":"0xa431984fef1d014620504d02f821f872221cf44c250a81a31e81fa4855b2b302","vin":[{"n":0,"addresses":["TZEZWXYQS44388xBoMhQdpL1HrBZFLfDpt"],"isAddress":true}],"vout":[{"value":"0","n":0,"addresses":["TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf"],"isAddress":true}],"blockHeight":-1,"confirmations":0,"blockTime":0,"value":"0","fees":"3076500","rbf":true,"coinSpecificData":{"tx":{"nonce":"0x0","gasPrice":"0xd2","gas":"0x393a","to":"TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf","value":"0x0","input":"0xa9059cbb000000000000000000000000242aa579f130bf6fea5eac12aa6b846fb8b293ab0000000000000000000000000000000000000000000000000000000000ab604e","hash":"0xa431984fef1d014620504d02f821f872221cf44c250a81a31e81fa4855b2b302","blockNumber":"0x348d2a7","from":"TZEZWXYQS44388xBoMhQdpL1HrBZFLfDpt","transactionIndex":"0x0"},"receipt":{"gasUsed":"0x393a","status":"0x1","logs":[{"address":"TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf","topics":["0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef","0x000000000000000000000000ff324071970b2b08822caa310c1bb458e63a5033","0x000000000000000000000000242aa579f130bf6fea5eac12aa6b846fb8b293ab"],"data":"0x0000000000000000000000000000000000000000000000000000000000ab604e"}]}},"tokenTransfers":[{"type":"TRC20","standard":"TRC20","from":"TZEZWXYQS44388xBoMhQdpL1HrBZFLfDpt","to":"TDGSR64oU4QDpViKfdwawSiqwyqpUB6JUD","contract":"TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf","name":"TronTestContract236","symbol":"TRC236","decimals":6,"value":"11231310"}],"ethereumSpecific":{"status":1,"nonce":0,"gasLimit":14650,"gasUsed":14650,"gasPrice":"210","data":"0xa9059cbb000000000000000000000000242aa579f130bf6fea5eac12aa6b846fb8b293ab0000000000000000000000000000000000000000000000000000000000ab604e","parsedData":{"methodId":"0xa9059cbb","name":"Transfer","function":"transfer(address, uint256)","params":[{"type":"address","values":["TDGSR64oU4QDpViKfdwawSiqwyqpUB6JUD"]},{"type":"uint256","values":["11231310"]}]}},"addressAliases":{"TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf":{"Type":"Contract","Alias":"TronTestContract236"}}}`,
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
				`{"page":1,"totalPages":1,"itemsOnPage":1000,"address":"TZEZWXYQS44388xBoMhQdpL1HrBZFLfDpt","balance":"123450255","unconfirmedBalance":"0","unconfirmedTxs":0,"txs":1,"nonTokenTxs":1,"txids":["0xa431984fef1d014620504d02f821f872221cf44c250a81a31e81fa4855b2b302"],"nonce":"255","tokens":[{"type":"TRC20","standard":"TRC20","name":"TronTestContract236","contract":"TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf","transfers":1,"symbol":"TRC236","decimals":6,"balance":"1000255236"}]}`,
			},
		},
		{
			name:        "apiAddress TronAddrTX",
			r:           newGetRequest(ts.URL + "/api/v2/address/" + dbtestdata.TronAddrTD + "?details=txs"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"page":1,"totalPages":1,"itemsOnPage":1000,"address":"TDGSR64oU4QDpViKfdwawSiqwyqpUB6JUD","balance":"123450036","unconfirmedBalance":"0","unconfirmedTxs":0,"txs":1,"transactions":[{"txid":"0xa431984fef1d014620504d02f821f872221cf44c250a81a31e81fa4855b2b302","vin":[{"n":0,"addresses":["TZEZWXYQS44388xBoMhQdpL1HrBZFLfDpt"],"isAddress":true}],"vout":[{"value":"0","n":0,"addresses":["TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf"],"isAddress":true}],"blockHeight":-1,"confirmations":0,"blockTime":0,"value":"0","fees":"3076500","rbf":true,"coinSpecificData":{"tx":{"nonce":"0x0","gasPrice":"0xd2","gas":"0x393a","to":"TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf","value":"0x0","input":"0xa9059cbb000000000000000000000000242aa579f130bf6fea5eac12aa6b846fb8b293ab0000000000000000000000000000000000000000000000000000000000ab604e","hash":"0xa431984fef1d014620504d02f821f872221cf44c250a81a31e81fa4855b2b302","blockNumber":"0x348d2a7","from":"TZEZWXYQS44388xBoMhQdpL1HrBZFLfDpt","transactionIndex":"0x0"},"receipt":{"gasUsed":"0x393a","status":"0x1","logs":[{"address":"TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf","topics":["0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef","0x000000000000000000000000ff324071970b2b08822caa310c1bb458e63a5033","0x000000000000000000000000242aa579f130bf6fea5eac12aa6b846fb8b293ab"],"data":"0x0000000000000000000000000000000000000000000000000000000000ab604e"}]}},"tokenTransfers":[{"type":"TRC20","standard":"TRC20","from":"TZEZWXYQS44388xBoMhQdpL1HrBZFLfDpt","to":"TDGSR64oU4QDpViKfdwawSiqwyqpUB6JUD","contract":"TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf","name":"TronTestContract236","symbol":"TRC236","decimals":6,"value":"11231310"}],"ethereumSpecific":{"status":1,"nonce":0,"gasLimit":14650,"gasUsed":14650,"gasPrice":"210","data":"0xa9059cbb000000000000000000000000242aa579f130bf6fea5eac12aa6b846fb8b293ab0000000000000000000000000000000000000000000000000000000000ab604e","parsedData":{"methodId":"0xa9059cbb","name":"Transfer","function":"transfer(address, uint256)","params":[{"type":"address","values":["TDGSR64oU4QDpViKfdwawSiqwyqpUB6JUD"]},{"type":"uint256","values":["11231310"]}]}}}],"nonce":"36","tokens":[{"type":"TRC20","standard":"TRC20","name":"TronTestContract236","contract":"TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf","transfers":1,"symbol":"TRC236","decimals":6,"balance":"1000036236"}],"addressAliases":{"TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf":{"Type":"Contract","Alias":"TronTestContract236"}}}`,
			},
		},
		{
			name:        "apiAddress TronAddrContractTX1",
			r:           newGetRequest(ts.URL + "/api/v2/address/" + dbtestdata.TronAddrContractTX1 + "?details=txs"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"page":1,"totalPages":1,"itemsOnPage":1000,"address":"TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf","balance":"123450236","unconfirmedBalance":"0","unconfirmedTxs":0,"txs":1,"nonTokenTxs":1,"transactions":[{"txid":"0xa431984fef1d014620504d02f821f872221cf44c250a81a31e81fa4855b2b302","vin":[{"n":0,"addresses":["TZEZWXYQS44388xBoMhQdpL1HrBZFLfDpt"],"isAddress":true}],"vout":[{"value":"0","n":0,"addresses":["TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf"],"isAddress":true,"isOwn":true}],"blockHeight":-1,"confirmations":0,"blockTime":0,"value":"0","fees":"3076500","rbf":true,"coinSpecificData":{"tx":{"nonce":"0x0","gasPrice":"0xd2","gas":"0x393a","to":"TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf","value":"0x0","input":"0xa9059cbb000000000000000000000000242aa579f130bf6fea5eac12aa6b846fb8b293ab0000000000000000000000000000000000000000000000000000000000ab604e","hash":"0xa431984fef1d014620504d02f821f872221cf44c250a81a31e81fa4855b2b302","blockNumber":"0x348d2a7","from":"TZEZWXYQS44388xBoMhQdpL1HrBZFLfDpt","transactionIndex":"0x0"},"receipt":{"gasUsed":"0x393a","status":"0x1","logs":[{"address":"TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf","topics":["0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef","0x000000000000000000000000ff324071970b2b08822caa310c1bb458e63a5033","0x000000000000000000000000242aa579f130bf6fea5eac12aa6b846fb8b293ab"],"data":"0x0000000000000000000000000000000000000000000000000000000000ab604e"}]}},"tokenTransfers":[{"type":"TRC20","standard":"TRC20","from":"TZEZWXYQS44388xBoMhQdpL1HrBZFLfDpt","to":"TDGSR64oU4QDpViKfdwawSiqwyqpUB6JUD","contract":"TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf","name":"TronTestContract236","symbol":"TRC236","decimals":6,"value":"11231310"}],"ethereumSpecific":{"status":1,"nonce":0,"gasLimit":14650,"gasUsed":14650,"gasPrice":"210","data":"0xa9059cbb000000000000000000000000242aa579f130bf6fea5eac12aa6b846fb8b293ab0000000000000000000000000000000000000000000000000000000000ab604e","parsedData":{"methodId":"0xa9059cbb","name":"Transfer","function":"transfer(address, uint256)","params":[{"type":"address","values":["TDGSR64oU4QDpViKfdwawSiqwyqpUB6JUD"]},{"type":"uint256","values":["11231310"]}]}}}],"nonce":"236","contractInfo":{"type":"TRC20","standard":"TRC20","contract":"TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf","name":"TronTestContract236","symbol":"TRC236","decimals":6,"createdInBlock":1000},"addressAliases":{"TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf":{"Type":"Contract","Alias":"TronTestContract236"}}}`,
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
				`"backend":{"chain":"fakecoin","blocks":2,"headers":2,"bestBlockHash":"0x11223344556677889900aabbccddeeff11223344556677889900aabbccddeeff"`,
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
		want: `{"id":"0","data":{"name":"Fakecoin","shortcut":"FAKE","network":"FAKE","decimals":6,"version":"unknown","bestHeight":100000,"bestHash":"0x11223344556677889900aabbccddeeff11223344556677889900aabbccddeeff","block0Hash":"","testnet":true,"backend":{"version":"tron_test_1.0","subversion":"MockTron"}}}`,
	},
	{
		name: "websocket rpcCall",
		req: websocketReq{
			Method: "rpcCall",
			Params: WsRpcCallReq{
				To:   "0xcdA9FC258358EcaA88845f19Af595e908bb7EfE9",
				Data: "0x4567",
			},
		},
		want: `{"id":"1","data":{"data":"0x4567abcd"}}`,
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
	s.ConnectFullPublicInterface()

	ts := httptest.NewServer(s.https.Handler)
	defer ts.Close()

	httpTestsTron(t, ts)
	runWebsocketTests(t, ts, websocketTestsTron)
}
