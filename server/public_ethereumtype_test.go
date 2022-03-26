//go:build unittest
// +build unittest

package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang/glog"
	"github.com/trezor/blockbook/bchain/coins/eth"
	"github.com/trezor/blockbook/tests/dbtestdata"
)

func httpTestsEthereumType(t *testing.T, ts *httptest.Server) {
	tests := []httpTests{
		{
			name:        "apiIndex",
			r:           newGetRequest(ts.URL + "/api"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"blockbook":{"coin":"Fakecoin"`,
				`"bestHeight":4321001`,
				`"decimals":18`,
				`"backend":{"chain":"fakecoin","blocks":2,"headers":2,"bestBlockHash":"0x2b57e15e93a0ed197417a34c2498b7187df79099572c04a6b6e6ff418f74e6ee"`,
				`"version":"001001","subversion":"/Fakecoin:0.0.1/"`,
			},
		},
		{
			name:        "apiAddress EthAddr4b",
			r:           newGetRequest(ts.URL + "/api/v2/address/" + dbtestdata.EthAddr4b),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"page":1,"totalPages":1,"itemsOnPage":1000,"address":"0x4Bda106325C335dF99eab7fE363cAC8A0ba2a24D","balance":"123450075","unconfirmedBalance":"0","unconfirmedTxs":0,"txs":1,"nonTokenTxs":1,"internalTxs":1,"txids":["0xc92919ad24ffd58f760b18df7949f06e1190cf54a50a0e3745a385608ed3cbf2"],"nonce":"75","tokens":[{"type":"ERC20","name":"Contract 13","contract":"0x0d0F936Ee4c93e25944694D6C121de94D9760F11","transfers":2,"symbol":"S13","decimals":18,"balance":"1000075013"},{"type":"ERC20","name":"Contract 74","contract":"0x4af4114F73d1c1C903aC9E0361b379D1291808A2","transfers":2,"symbol":"S74","decimals":18,"balance":"1000075074"}],"erc20Contract":{"contract":"0x4Bda106325C335dF99eab7fE363cAC8A0ba2a24D","name":"Contract 75","symbol":"S75","decimals":18}}`,
			},
		},
	}

	performHttpTests(tests, t, ts)
}

func Test_PublicServer_EthereumType(t *testing.T) {
	parser := eth.NewEthereumParser(1)
	chain, err := dbtestdata.NewFakeBlockChainEthereumType(parser)
	if err != nil {
		glog.Fatal("fakechain: ", err)
	}

	s, dbpath := setupPublicHTTPServer(parser, chain, t)
	defer closeAndDestroyPublicServer(t, s, dbpath)
	s.ConnectFullPublicInterface()
	// take the handler of the public server and pass it to the test server
	ts := httptest.NewServer(s.https.Handler)
	defer ts.Close()

	httpTestsEthereumType(t, ts)
}
