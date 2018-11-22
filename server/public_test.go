// +build unittest

package server

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"blockbook/common"
	"blockbook/db"
	"blockbook/tests/dbtestdata"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang/glog"
	"github.com/jakm/btcutil/chaincfg"
	"github.com/martinboehm/golang-socketio"
	"github.com/martinboehm/golang-socketio/transport"
)

func TestMain(m *testing.M) {
	// set the current directory to blockbook root so that ./static/ works
	if err := os.Chdir(".."); err != nil {
		glog.Fatal("Chdir error:", err)
	}
	c := m.Run()
	chaincfg.ResetParams()
	os.Exit(c)
}

func setupRocksDB(t *testing.T, parser bchain.BlockChainParser) (*db.RocksDB, *common.InternalState, string) {
	tmp, err := ioutil.TempDir("", "testdb")
	if err != nil {
		t.Fatal(err)
	}
	d, err := db.NewRocksDB(tmp, 100000, -1, parser, nil)
	if err != nil {
		t.Fatal(err)
	}
	is, err := d.LoadInternalState("fakecoin")
	if err != nil {
		t.Fatal(err)
	}
	d.SetInternalState(is)
	// import data
	if err := d.ConnectBlock(dbtestdata.GetTestUTXOBlock1(parser)); err != nil {
		t.Fatal(err)
	}
	if err := d.ConnectBlock(dbtestdata.GetTestUTXOBlock2(parser)); err != nil {
		t.Fatal(err)
	}
	return d, is, tmp
}

func setupPublicHTTPServer(t *testing.T) (*PublicServer, string) {
	parser := btc.NewBitcoinParser(
		btc.GetChainParams("test"),
		&btc.Configuration{BlockAddressesToKeep: 1})

	d, is, path := setupRocksDB(t, parser)
	// setup internal state and match BestHeight to test data
	is.Coin = "Fakecoin"
	is.CoinLabel = "Fake Coin"
	is.CoinShortcut = "FAKE"
	is.BestHeight = 225494

	metrics, err := common.GetMetrics("Fakecoin")
	if err != nil {
		glog.Fatal("metrics: ", err)
	}

	chain, err := dbtestdata.NewFakeBlockChain(parser)
	if err != nil {
		glog.Fatal("fakechain: ", err)
	}

	// caching is switched off because test transactions do not have hex data
	txCache, err := db.NewTxCache(d, chain, metrics, is, false)
	if err != nil {
		glog.Fatal("txCache: ", err)
	}

	// s.Run is never called, binding can be to any port
	s, err := NewPublicServer("localhost:12345", "", d, chain, txCache, "", metrics, is, false)
	if err != nil {
		t.Fatal(err)
	}
	return s, path
}

func closeAndDestroyPublicServer(t *testing.T, s *PublicServer, dbpath string) {
	// destroy db
	if err := s.db.Close(); err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(dbpath)
}

func newGetRequest(u string) *http.Request {
	r, err := http.NewRequest("GET", u, nil)
	if err != nil {
		glog.Fatal(err)
	}
	return r
}

func newPostFormRequest(u string, formdata ...string) *http.Request {
	form := url.Values{}
	for i := 0; i < len(formdata)-1; i += 2 {
		form.Add(formdata[i], formdata[i+1])
	}
	r, err := http.NewRequest("POST", u, strings.NewReader(form.Encode()))
	if err != nil {
		glog.Fatal(err)
	}
	r.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	return r
}

func newPostRequest(u string, body string) *http.Request {
	r, err := http.NewRequest("POST", u, strings.NewReader(body))
	if err != nil {
		glog.Fatal(err)
	}
	r.Header.Add("Content-Type", "application/octet-stream")
	return r
}

func httpTests(t *testing.T, ts *httptest.Server) {
	tests := []struct {
		name        string
		r           *http.Request
		status      int
		contentType string
		body        []string
	}{
		{
			name:        "explorerTx",
			r:           newGetRequest(ts.URL + "/tx/fdd824a780cbb718eeb766eb05d83fdefc793a27082cd5e67f856d69798cf7db"),
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body: []string{
				`<a href="/" class="nav-link">Fake Coin Explorer</a>`,
				`<h1>Transaction</h1>`,
				`<span class="data">fdd824a780cbb718eeb766eb05d83fdefc793a27082cd5e67f856d69798cf7db</span>`,
				`td class="data">0 FAKE</td>`,
				`<a href="/address/mzVznVsCHkVHX9UN8WPFASWUUHtxnNn4Jj">mzVznVsCHkVHX9UN8WPFASWUUHtxnNn4Jj</a>`,
				`13.60030331 FAKE`,
				`<td><span class="float-left">No Inputs (Newly Generated Coins)</span></td>`,
				`</html>`,
			},
		},
		{
			name:        "explorerAddress",
			r:           newGetRequest(ts.URL + "/address/mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz"),
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body: []string{
				`<a href="/" class="nav-link">Fake Coin Explorer</a>`,
				`<h1>Address`,
				`<small class="text-muted">0 FAKE</small>`,
				`<span class="data">mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz</span>`,
				`<td class="data">0.00012345 FAKE</td>`,
				`<a href="/tx/7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25">7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25</a>`,
				`3172.83951061 FAKE <a class="text-danger" href="/spending/7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25/0" title="Spent">➡</a></span>`,
				`<td><span class="ellipsis float-left"><a href="/address/mtR97eM2HPWVM6c8FGLGcukgaHHQv7THoL">mtR97eM2HPWVM6c8FGLGcukgaHHQv7THoL</a></span><span class="float-right">`,
				`td><span class="ellipsis float-left"><a href="/address/mtR97eM2HPWVM6c8FGLGcukgaHHQv7THoL">mtR97eM2HPWVM6c8FGLGcukgaHHQv7THoL</a></span><span class="float-right">`,
				`9172.83951061 FAKE <span class="text-success" title="Unspent"> <b>×</b></span></span>`,
				`<a href="/tx/00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa3840">00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa3840</a>`,
				`</html>`,
			},
		},
		{
			name:        "explorerSpendingTx",
			r:           newGetRequest(ts.URL + "/spending/7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25/0"),
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body: []string{
				`<a href="/" class="nav-link">Fake Coin Explorer</a>`,
				`<h1>Transaction</h1>`,
				`<span class="data">3d90d15ed026dc45e19ffb52875ed18fa9e8012ad123d7f7212176e2b0ebdb71</span>`,
				`<td class="data">0.00000062 FAKE</td>`,
				`</html>`,
			},
		},
		{
			name:        "explorerSpendingTx - not found",
			r:           newGetRequest(ts.URL + "/spending/123be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25/0"),
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body: []string{
				`<a href="/" class="nav-link">Fake Coin Explorer</a>`,
				`<h1>Error</h1>`,
				`<h4>Transaction not found</h4>`,
				`</html>`,
			},
		},
		{
			name:        "explorerBlocks",
			r:           newGetRequest(ts.URL + "/blocks"),
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body: []string{
				`<a href="/" class="nav-link">Fake Coin Explorer</a>`,
				`<h1>Blocks`,
				`<td><a href="/block/225494">225494</a></td>`,
				`<td class="ellipsis">00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6</td>`,
				`<td class="ellipsis">0000000076fbbed90fd75b0e18856aa35baa984e9c9d444cf746ad85e94e2997</td>`,
				`<td class="text-right">2</td>`,
				`<td class="text-right">1234567</td>`,
				`</html>`,
			},
		},
		{
			name:        "explorerBlock",
			r:           newGetRequest(ts.URL + "/block/225494"),
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body: []string{
				`<a href="/" class="nav-link">Fake Coin Explorer</a>`,
				`<h1>Block 225494</h1>`,
				`<span class="data">00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6</span>`,
				`<td class="data">4</td>`, // number of transactions
				`<a href="/address/mtR97eM2HPWVM6c8FGLGcukgaHHQv7THoL">mtR97eM2HPWVM6c8FGLGcukgaHHQv7THoL</a>`,
				`9172.83951061 FAKE`,
				`<a href="/tx/fdd824a780cbb718eeb766eb05d83fdefc793a27082cd5e67f856d69798cf7db">fdd824a780cbb718eeb766eb05d83fdefc793a27082cd5e67f856d69798cf7db</a>`,
				`</html>`,
			},
		},
		{
			name:        "explorerIndex",
			r:           newGetRequest(ts.URL + "/"),
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body: []string{
				`<a href="/" class="nav-link">Fake Coin Explorer</a>`,
				`<h1>Application status</h1>`,
				`<h3 class="bg-warning text-white" style="padding: 20px;">Synchronization with backend is disabled, the state of index is not up to date.</h3>`,
				`<a href="/block/225494">225494</a>`,
				`<td class="data">/Fakecoin:0.0.1/</td>`,
				`</html>`,
			},
		},
		{
			name:        "explorerSearch block height",
			r:           newGetRequest(ts.URL + "/search?q=225494"),
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body: []string{
				`<a href="/" class="nav-link">Fake Coin Explorer</a>`,
				`<h1>Block 225494</h1>`,
				`<span class="data">00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6</span>`,
				`<td class="data">4</td>`, // number of transactions
				`<a href="/address/mtR97eM2HPWVM6c8FGLGcukgaHHQv7THoL">mtR97eM2HPWVM6c8FGLGcukgaHHQv7THoL</a>`,
				`9172.83951061 FAKE`,
				`<a href="/tx/fdd824a780cbb718eeb766eb05d83fdefc793a27082cd5e67f856d69798cf7db">fdd824a780cbb718eeb766eb05d83fdefc793a27082cd5e67f856d69798cf7db</a>`,
				`</html>`,
			},
		},
		{
			name:        "explorerSearch block hash",
			r:           newGetRequest(ts.URL + "/search?q=00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6"),
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body: []string{
				`<a href="/" class="nav-link">Fake Coin Explorer</a>`,
				`<h1>Block 225494</h1>`,
				`<span class="data">00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6</span>`,
				`<td class="data">4</td>`, // number of transactions
				`<a href="/address/mtR97eM2HPWVM6c8FGLGcukgaHHQv7THoL">mtR97eM2HPWVM6c8FGLGcukgaHHQv7THoL</a>`,
				`9172.83951061 FAKE`,
				`<a href="/tx/fdd824a780cbb718eeb766eb05d83fdefc793a27082cd5e67f856d69798cf7db">fdd824a780cbb718eeb766eb05d83fdefc793a27082cd5e67f856d69798cf7db</a>`,
				`</html>`,
			},
		},
		{
			name:        "explorerSearch tx",
			r:           newGetRequest(ts.URL + "/search?q=fdd824a780cbb718eeb766eb05d83fdefc793a27082cd5e67f856d69798cf7db"),
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body: []string{
				`<a href="/" class="nav-link">Fake Coin Explorer</a>`,
				`<h1>Transaction</h1>`,
				`<span class="data">fdd824a780cbb718eeb766eb05d83fdefc793a27082cd5e67f856d69798cf7db</span>`,
				`td class="data">0 FAKE</td>`,
				`<a href="/address/mzVznVsCHkVHX9UN8WPFASWUUHtxnNn4Jj">mzVznVsCHkVHX9UN8WPFASWUUHtxnNn4Jj</a>`,
				`13.60030331 FAKE`,
				`<td><span class="float-left">No Inputs (Newly Generated Coins)</span></td>`,
				`</html>`,
			},
		},
		{
			name:        "explorerSearch address",
			r:           newGetRequest(ts.URL + "/search?q=mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz"),
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body: []string{
				`<a href="/" class="nav-link">Fake Coin Explorer</a>`,
				`<h1>Address`,
				`<small class="text-muted">0 FAKE</small>`,
				`<span class="data">mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz</span>`,
				`<td class="data">0.00012345 FAKE</td>`,
				`<a href="/tx/7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25">7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25</a>`,
				`3172.83951061 FAKE <a class="text-danger" href="/spending/7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25/0" title="Spent">➡</a></span>`,
				`<td><span class="ellipsis float-left"><a href="/address/mtR97eM2HPWVM6c8FGLGcukgaHHQv7THoL">mtR97eM2HPWVM6c8FGLGcukgaHHQv7THoL</a></span><span class="float-right">`,
				`td><span class="ellipsis float-left"><a href="/address/mtR97eM2HPWVM6c8FGLGcukgaHHQv7THoL">mtR97eM2HPWVM6c8FGLGcukgaHHQv7THoL</a></span><span class="float-right">`,
				`9172.83951061 FAKE <span class="text-success" title="Unspent"> <b>×</b></span></span>`,
				`<a href="/tx/00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa3840">00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa3840</a>`,
				`</html>`,
			},
		},
		{
			name:        "explorerSearch not found",
			r:           newGetRequest(ts.URL + "/search?q=1234"),
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body: []string{
				`<a href="/" class="nav-link">Fake Coin Explorer</a>`,
				`<h1>Error</h1>`,
				`<h4>No matching records found for &#39;1234&#39;</h4>`,
				`</html>`,
			},
		},
		{
			name:        "explorerSendTx",
			r:           newGetRequest(ts.URL + "/sendtx"),
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body: []string{
				`<a href="/" class="nav-link">Fake Coin Explorer</a>`,
				`<h1>Send Raw Transaction</h1>`,
				`<textarea class="form-control" rows="8" name="hex"></textarea>`,
				`</html>`,
			},
		},
		{
			name:        "explorerSendTx POST",
			r:           newPostFormRequest(ts.URL+"/sendtx", "hex", "12341234"),
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body: []string{
				`<a href="/" class="nav-link">Fake Coin Explorer</a>`,
				`<h1>Send Raw Transaction</h1>`,
				`<textarea class="form-control" rows="8" name="hex">12341234</textarea>`,
				`<div class="alert alert-danger">Invalid data</div>`,
				`</html>`,
			},
		},
		{
			name:        "apiIndex",
			r:           newGetRequest(ts.URL + "/api"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"blockbook":{"coin":"Fakecoin"`,
				`"bestHeight":225494`,
				`"backend":{"chain":"fakecoin","blocks":2,"headers":2,"bestblockhash":"00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6"`,
				`"version":"001001","subversion":"/Fakecoin:0.0.1/"`,
			},
		},
		{
			name:        "apiBlockIndex",
			r:           newGetRequest(ts.URL + "/api/block-index/"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"blockHash":"00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6"}`,
			},
		},
		{
			name:        "apiTx",
			r:           newGetRequest(ts.URL + "/api/tx/05e2e48aeabdd9b75def7b48d756ba304713c2aba7b522bf9dbc893fc4231b07"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"txid":"05e2e48aeabdd9b75def7b48d756ba304713c2aba7b522bf9dbc893fc4231b07","vin":[{"txid":"effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75","vout":2,"n":0,"scriptSig":{"hex":""},"addresses":["2NEVv9LJmAnY99W1pFoc5UJjVdypBqdnvu1"],"value":"0.00009876"}],"vout":[{"value":"0.00009","n":0,"scriptPubKey":{"hex":"a914e921fc4912a315078f370d959f2c4f7b6d2a683c87","addresses":["2NEVv9LJmAnY99W1pFoc5UJjVdypBqdnvu1"]},"spent":false}],"blockhash":"00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6","blockheight":225494,"confirmations":1,"time":22549400002,"blocktime":22549400002,"valueOut":"0.00009","valueIn":"0.00009876","fees":"0.00000876","hex":""}`,
			},
		},
		{
			name:        "apiTx - not found",
			r:           newGetRequest(ts.URL + "/api/tx/1232e48aeabdd9b75def7b48d756ba304713c2aba7b522bf9dbc893fc4231b07"),
			status:      http.StatusBadRequest,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"error":"Tx not found, Not found"}`,
			},
		},
		{
			name:        "apiTxSpecific",
			r:           newGetRequest(ts.URL + "/api/tx-specific/00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa3840"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"hex":"","txid":"00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa3840","version":0,"locktime":0,"vin":null,"vout":[{"ValueSat":100000000,"value":0,"n":0,"scriptPubKey":{"hex":"76a914010d39800f86122416e28f485029acf77507169288ac","addresses":null}},{"ValueSat":12345,"value":0,"n":1,"scriptPubKey":{"hex":"76a9148bdf0aa3c567aa5975c2e61321b8bebbe7293df688ac","addresses":null}}],"confirmations":2,"time":22549300000,"blocktime":22549300000}`,
			},
		},
		{
			name:        "apiAddress",
			r:           newGetRequest(ts.URL + "/api/address/mv9uLThosiEnGRbVPS7Vhyw6VssbVRsiAw"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"page":1,"totalPages":1,"itemsOnPage":1000,"addrStr":"mv9uLThosiEnGRbVPS7Vhyw6VssbVRsiAw","balance":"0","totalReceived":"12345.67890123","totalSent":"12345.67890123","unconfirmedBalance":"0","unconfirmedTxApperances":0,"txApperances":2,"transactions":["7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25","effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75"]}`,
			},
		},
		{
			name:        "apiAddressUtxo",
			r:           newGetRequest(ts.URL + "/api/utxo/mtR97eM2HPWVM6c8FGLGcukgaHHQv7THoL"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`[{"txid":"7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25","vout":1,"amount":"9172.83951061","satoshis":917283951061,"height":225494,"confirmations":1}]`,
			},
		},
		{
			name:        "apiSendTx",
			r:           newGetRequest(ts.URL + "/api/sendtx/1234567890"),
			status:      http.StatusBadRequest,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"error":"Invalid data"}`,
			},
		},
		{
			name:        "apiSendTx POST",
			r:           newPostRequest(ts.URL+"/api/sendtx/", "123456"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"result":"9876"}`,
			},
		},
		{
			name:        "apiSendTx POST empty",
			r:           newPostRequest(ts.URL+"/api/sendtx", ""),
			status:      http.StatusBadRequest,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"error":"Missing tx blob"}`,
			},
		},
		{
			name:        "apiEstimateFee",
			r:           newGetRequest(ts.URL + "/api/estimatefee/123?conservative=false"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"result":"0.00012299"}`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := http.DefaultClient.Do(tt.r)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tt.status {
				t.Errorf("StatusCode = %v, want %v", resp.StatusCode, tt.status)
			}
			if resp.Header["Content-Type"][0] != tt.contentType {
				t.Errorf("Content-Type = %v, want %v", resp.Header["Content-Type"][0], tt.contentType)
			}
			bb, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			b := string(bb)
			for _, c := range tt.body {
				if !strings.Contains(b, c) {
					t.Errorf("Page body does not contain %v, body %v", c, b)
					break
				}
			}
		})
	}
}

func socketioTests(t *testing.T, ts *httptest.Server) {
	type socketioReq struct {
		Method string        `json:"method"`
		Params []interface{} `json:"params"`
	}

	url := strings.Replace(ts.URL, "http://", "ws://", 1) + "/socket.io/"
	s, err := gosocketio.Dial(url, transport.GetDefaultWebsocketTransport())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	tests := []struct {
		name string
		req  socketioReq
		want string
	}{
		{
			name: "getInfo",
			req:  socketioReq{"getInfo", []interface{}{}},
			want: `{"result":{"blocks":225494,"testnet":true,"network":"fakecoin","subversion":"/Fakecoin:0.0.1/","coin_name":"Fakecoin","about":"Blockbook - blockchain indexer for TREZOR wallet https://trezor.io/. Do not use for any other purpose."}}`,
		},
		{
			name: "estimateFee",
			req:  socketioReq{"estimateFee", []interface{}{17}},
			want: `{"result":0.000034}`,
		},
		{
			name: "estimateSmartFee",
			req:  socketioReq{"estimateSmartFee", []interface{}{19, true}},
			want: `{"result":0.000019}`,
		},
		{
			name: "getAddressTxids",
			req: socketioReq{"getAddressTxids", []interface{}{
				[]string{"mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz"},
				map[string]interface{}{
					"start":        2000000,
					"end":          0,
					"queryMempool": false,
				},
			}},
			want: `{"result":["7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25","00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa3840"]}`,
		},
		{
			name: "getAddressTxids limited range",
			req: socketioReq{"getAddressTxids", []interface{}{
				[]string{"mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz"},
				map[string]interface{}{
					"start":        225494,
					"end":          225494,
					"queryMempool": false,
				},
			}},
			want: `{"result":["7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25"]}`,
		},
		{
			name: "getAddressHistory",
			req: socketioReq{"getAddressHistory", []interface{}{
				[]string{"mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz"},
				map[string]interface{}{
					"start":        2000000,
					"end":          0,
					"queryMempool": false,
					"from":         0,
					"to":           5,
				},
			}},
			want: `{"result":{"totalCount":2,"items":[{"addresses":{"mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz":{"inputIndexes":[1],"outputIndexes":[]}},"satoshis":-12345,"confirmations":1,"tx":{"hex":"","height":225494,"blockTimestamp":22549400000,"version":0,"hash":"7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25","inputs":[{"txid":"effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75","outputIndex":0,"script":"","sequence":0,"address":"mv9uLThosiEnGRbVPS7Vhyw6VssbVRsiAw","satoshis":1234567890123},{"txid":"00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa3840","outputIndex":1,"script":"","sequence":0,"address":"mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz","satoshis":12345}],"inputSatoshis":1234567902468,"outputs":[{"satoshis":317283951061,"script":"76a914ccaaaf374e1b06cb83118453d102587b4273d09588ac","address":"mzB8cYrfRwFRFAGTDzV8LkUQy5BQicxGhX"},{"satoshis":917283951061,"script":"76a9148d802c045445df49613f6a70ddd2e48526f3701f88ac","address":"mtR97eM2HPWVM6c8FGLGcukgaHHQv7THoL"}],"outputSatoshis":1234567902122,"feeSatoshis":346}},{"addresses":{"mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz":{"inputIndexes":[],"outputIndexes":[1]}},"satoshis":12345,"confirmations":2,"tx":{"hex":"","height":225493,"blockTimestamp":22549300000,"version":0,"hash":"00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa3840","inputs":[],"outputs":[{"satoshis":100000000,"script":"76a914010d39800f86122416e28f485029acf77507169288ac","address":"mfcWp7DB6NuaZsExybTTXpVgWz559Np4Ti"},{"satoshis":12345,"script":"76a9148bdf0aa3c567aa5975c2e61321b8bebbe7293df688ac","address":"mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz"}],"outputSatoshis":100012345}}]}}`,
		},
		{
			name: "getBlockHeader",
			req:  socketioReq{"getBlockHeader", []interface{}{225493}},
			want: `{"result":{"hash":"0000000076fbbed90fd75b0e18856aa35baa984e9c9d444cf746ad85e94e2997","version":0,"confirmations":0,"height":0,"chainWork":"","nextHash":"","merkleRoot":"","time":0,"medianTime":0,"nonce":0,"bits":"","difficulty":0}}`,
		},
		{
			name: "getDetailedTransaction",
			req:  socketioReq{"getDetailedTransaction", []interface{}{"3d90d15ed026dc45e19ffb52875ed18fa9e8012ad123d7f7212176e2b0ebdb71"}},
			want: `{"result":{"hex":"","height":225494,"blockTimestamp":22549400001,"version":0,"hash":"3d90d15ed026dc45e19ffb52875ed18fa9e8012ad123d7f7212176e2b0ebdb71","inputs":[{"txid":"7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25","outputIndex":0,"script":"","sequence":0,"address":"mzB8cYrfRwFRFAGTDzV8LkUQy5BQicxGhX","satoshis":317283951061},{"txid":"effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75","outputIndex":1,"script":"","sequence":0,"address":"2Mz1CYoppGGsLNUGF2YDhTif6J661JitALS","satoshis":1}],"inputSatoshis":317283951062,"outputs":[{"satoshis":118641975500,"script":"76a914b434eb0c1a3b7a02e8a29cc616e791ef1e0bf51f88ac","address":"mwwoKQE5Lb1G4picHSHDQKg8jw424PF9SC"},{"satoshis":198641975500,"script":"76a9143f8ba3fda3ba7b69f5818086e12223c6dd25e3c888ac","address":"mmJx9Y8ayz9h14yd9fgCW1bUKoEpkBAquP"}],"outputSatoshis":317283951000,"feeSatoshis":62}}`,
		},
		{
			name: "sendTransaction",
			req:  socketioReq{"sendTransaction", []interface{}{"010000000001019d64f0c72a0d206001decbffaa722eb1044534c"}},
			want: `{"error":{"message":"Invalid data"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := s.Ack("message", tt.req, time.Second*3)
			if err != nil {
				t.Errorf("Socketio error %v", err)
			}
			if resp != tt.want {
				t.Errorf("Socketio resp %v, want %v", resp, tt.want)
			}
		})
	}
}

func Test_PublicServer_UTXO(t *testing.T) {
	s, dbpath := setupPublicHTTPServer(t)
	defer closeAndDestroyPublicServer(t, s, dbpath)
	s.ConnectFullPublicInterface()
	// take the handler of the public server and pass it to the test server
	ts := httptest.NewServer(s.https.Handler)
	defer ts.Close()

	httpTests(t, ts)
	socketioTests(t, ts)

}
