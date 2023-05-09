//go:build unittest

package server

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang/glog"
	"github.com/gorilla/websocket"
	"github.com/linxGnu/grocksdb"
	"github.com/martinboehm/btcutil/chaincfg"
	gosocketio "github.com/martinboehm/golang-socketio"
	"github.com/martinboehm/golang-socketio/transport"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/db"
	"github.com/trezor/blockbook/fiat"
	"github.com/trezor/blockbook/tests/dbtestdata"
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

func setupRocksDB(parser bchain.BlockChainParser, chain bchain.BlockChain, t *testing.T, extendedIndex bool) (*db.RocksDB, *common.InternalState, string) {
	tmp, err := ioutil.TempDir("", "testdb")
	if err != nil {
		t.Fatal(err)
	}
	d, err := db.NewRocksDB(tmp, 100000, -1, parser, nil, extendedIndex)
	if err != nil {
		t.Fatal(err)
	}
	is, err := d.LoadInternalState("fakecoin")
	if err != nil {
		t.Fatal(err)
	}
	d.SetInternalState(is)
	// there are 2 simulated block, of height bestBlockHeight-1 and bestBlockHeight
	bestHeight, err := chain.GetBestBlockHeight()
	if err != nil {
		t.Fatal(err)
	}
	block1, err := chain.GetBlock("", bestHeight-1)
	if err != nil {
		t.Fatal(err)
	}
	// setup internal state BlockTimes
	for i := uint32(0); i < block1.Height; i++ {
		is.BlockTimes = append(is.BlockTimes, 0)
	}
	// import data
	if err := d.ConnectBlock(block1); err != nil {
		t.Fatal(err)
	}
	block2, err := chain.GetBlock("", bestHeight)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.ConnectBlock(block2); err != nil {
		t.Fatal(err)
	}
	is.FinishedSync(block2.Height)
	if parser.GetChainType() == bchain.ChainEthereumType {
		if err := initTestFiatRatesEthereumType(d); err != nil {
			t.Fatal(err)
		}
		if err := initEthereumTypeDB(d); err != nil {
			t.Fatal(err)
		}
	} else {
		if err := initTestFiatRates(d); err != nil {
			t.Fatal(err)
		}
	}
	return d, is, tmp
}

var metrics *common.Metrics

func setupPublicHTTPServer(parser bchain.BlockChainParser, chain bchain.BlockChain, t *testing.T, extendedIndex bool) (*PublicServer, string) {
	d, is, path := setupRocksDB(parser, chain, t, extendedIndex)
	// setup internal state and match BestHeight to test data
	is.Coin = "Fakecoin"
	is.CoinLabel = "Fake Coin"
	is.CoinShortcut = "FAKE"

	var err error
	// metrics can be setup only once
	if metrics == nil {
		metrics, err = common.GetMetrics("Fakecoin" + strconv.FormatBool(extendedIndex))
		if err != nil {
			glog.Fatal("metrics: ", err)
		}
	}

	mempool, err := chain.CreateMempool(chain)
	if err != nil {
		glog.Fatal("mempool: ", err)
	}

	// caching is switched off because test transactions do not have hex data
	txCache, err := db.NewTxCache(d, chain, metrics, is, false)
	if err != nil {
		glog.Fatal("txCache: ", err)
	}

	// mocked CoinGecko API
	configJSON := `{"fiat_rates": "coingecko", "fiat_rates_params": "{\"url\": \"none\", \"coin\": \"ethereum\",\"platformIdentifier\":\"ethereum\",\"platformVsCurrency\": \"usd\",\"periodSeconds\": 60}"}`
	fiatRates, err := fiat.NewFiatRates(d, []byte(configJSON), nil, nil)
	if err != nil {
		glog.Fatal("fiatRates ", err)
	}

	// s.Run is never called, binding can be to any port
	s, err := NewPublicServer("localhost:12345", "", d, chain, mempool, txCache, "", metrics, is, fiatRates, false)
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

func insertFiatRate(date string, rates map[string]float32, tokenRates map[string]float32, d *db.RocksDB) error {
	convertedDate, err := time.Parse("20060102150405", date)
	if err != nil {
		return err
	}
	ticker := &common.CurrencyRatesTicker{
		Timestamp:  convertedDate,
		Rates:      rates,
		TokenRates: tokenRates,
	}
	wb := grocksdb.NewWriteBatch()
	defer wb.Destroy()
	if err := d.FiatRatesStoreTicker(wb, ticker); err != nil {
		return err
	}
	return d.WriteBatch(wb)
}

// initTestFiatRates initializes test data for /api/v2/tickers endpoint
func initTestFiatRates(d *db.RocksDB) error {
	if err := insertFiatRate("20180320000000", map[string]float32{
		"usd": 2000.0,
		"eur": 1300.0,
	}, nil, d); err != nil {
		return err
	}
	if err := insertFiatRate("20180321000000", map[string]float32{
		"usd": 2001.0,
		"eur": 1301.0,
	}, nil, d); err != nil {
		return err
	}
	if err := insertFiatRate("20180322000000", map[string]float32{
		"usd": 2002.0,
		"eur": 1302.0,
	}, nil, d); err != nil {
		return err
	}
	if err := insertFiatRate("20180324000000", map[string]float32{
		"usd": 2003.0,
		"eur": 1303.0,
	}, nil, d); err != nil {
		return err
	}
	if err := insertFiatRate("20191121000000", map[string]float32{
		"usd": 7814.5,
		"eur": 7100.0,
	}, nil, d); err != nil {
		return err
	}
	return insertFiatRate("20191122000000", map[string]float32{
		"usd": 7914.5,
		"eur": 7134.1,
	}, nil, d)
}

type httpTests struct {
	name        string
	r           *http.Request
	status      int
	contentType string
	body        []string
}

func performHttpTests(tests []httpTests, t *testing.T, ts *httptest.Server) {
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
					t.Errorf("got\n%v\nwant to contain %v", b, c)
					break
				}
			}
		})
	}
}

func httpTestsBitcoinType(t *testing.T, ts *httptest.Server) {
	tests := []httpTests{
		{
			name:        "explorerTx",
			r:           newGetRequest(ts.URL + "/tx/fdd824a780cbb718eeb766eb05d83fdefc793a27082cd5e67f856d69798cf7db"),
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body: []string{
				`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1.0,shrink-to-fit=no"><link href="https://cdn.jsdelivr.net/npm/bootstrap@5.2.2/dist/css/bootstrap.min.css" rel="stylesheet" integrity="sha384-Zenh87qX5JnK2Jl0vWa8Ck2rdkQ2Bzep5IDxbcnCeuOxjzrPF/et3URy9Bv1WTRi" crossorigin="anonymous"><link rel="stylesheet" href="/static/css/main.min.3.css"><script>var hasSecondary=false;</script><script src="https://cdn.jsdelivr.net/npm/bootstrap@5.2.2/dist/js/bootstrap.bundle.min.js" integrity="sha384-OERcA2EqjJCMA+/3y+gxIOqMEjwtxJY7qPCqsdltbNJuaOe923+mo//f6V8Qbsw3" crossorigin="anonymous"></script><script src="/static/js/main.min.3.js"></script><meta http-equiv="X-UA-Compatible" content="IE=edge"><meta name="description" content="Trezor Fake Coin Explorer"><title>Trezor Fake Coin Explorer</title></head><body><header id="header"><nav class="navbar navbar-expand-lg"><div class="container"><a class="navbar-brand" href="/" title="Home"><span class="trezor-logo"></span><span style="padding-left: 140px;">Fake Coin Explorer</span></a><button class="navbar-toggler" type="button" data-bs-toggle="collapse" data-bs-target="#navbarSupportedContent" aria-controls="navbarSupportedContent" aria-expanded="false" aria-label="Toggle navigation"><span class="navbar-toggler-icon"></span></button><div class="collapse navbar-collapse" id="navbarSupportedContent"><ul class="navbar-nav m-md-auto"><li class="nav-item pe-xl-4"><a href="/blocks" class="nav-link">Blocks</a></li><li class="nav-item"><a href="/" class="nav-link">Status</a></li></ul><span class="navbar-form"><form class="d-flex" id="search" action="/search" method="get"><input name="q" type="text" class="form-control form-control-lg" placeholder="Search for block, transaction, address or xpub" focus="true"><button class="btn" type="submit"><span class="search-icon"></span></button></form></span></div></div></nav></header><main id="wrap"><div class="container"><div class="row"><h1 class="col-12">Transaction</h1></div><div class="row pt-3"><h5 class="col-12 d-flex h-data"><span class="ellipsis copyable">fdd824a780cbb718eeb766eb05d83fdefc793a27082cd5e67f856d69798cf7db</span></h5></div><table class="table data-table info-table"><tbody><tr><td>Mined Time</td><td><span tt="2018-03-21 01:27:58">1639 days 11 hours ago</span></td></tr><tr><td>In Block</td><td class="ellipsis" style="max-width: 280px">00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6</td></tr><tr><td>In Block Height</td><td><a href="/block/225494">225<span class="ns">494</span></a></td></tr><tr><td>Total Input</td><td><span class="amt copyable" cc="0 FAKE"><span class="prim-amt">0 FAKE</span></span></td></tr><tr><td>Total Output</td><td><span class="amt copyable" cc="13.60030331 FAKE"><span class="prim-amt">13.<span class="amt-dec">60<span class="ns">030</span><span class="ns">331</span></span> FAKE</span></span></td></tr><tr><td>Fees</td><td><span class="amt copyable" cc="0 FAKE"><span class="prim-amt">0 FAKE</span></span></td></tr></tbody></table><div class="pt-1"><div class="tx-detail"><div class="row head"><div class="col-xs-7 col-md-8"><a href="/tx/fdd824a780cbb718eeb766eb05d83fdefc793a27082cd5e67f856d69798cf7db" class="ellipsis copyable txid">fdd824a780cbb718eeb766eb05d83fdefc793a27082cd5e67f856d69798cf7db</a></div><div class="col-xs-5 col-md-4 text-end">mined <span class="txvalue ms-1"><span tt="2018-03-21 01:27:58">1639 days 11 hours ago</span></span></div></div><div class="row body"><div class="col-md-5"><div class="row tx-in"><div class="col-12">No Inputs (Newly Generated Coins)</div></div></div><div class="col-md-1 col-xs-12 text-center">&nbsp;<span class="octicon"></span></div><div class="col-md-6"><div class="row tx-out"><div class="col-12"><span class="ellipsis copyable"><a href="/address/mzVznVsCHkVHX9UN8WPFASWUUHtxnNn4Jj">mzVznVsCHkVHX9UN8WPFASWUUHtxnNn4Jj</a></span><span class="tx-amt"><span class="amt copyable" cc="13.60030331 FAKE"><span class="prim-amt">13.<span class="amt-dec">60<span class="ns">030</span><span class="ns">331</span></span> FAKE</span></span><span class="unspent" tt="Unspent">×</span></span></td></div><div class="col-12">Unparsed address<span class="tx-amt"><span class="amt copyable" cc="0 FAKE"><span class="prim-amt">0 FAKE</span></span><span class="unspent" tt="Unspent">×</span></span></td></div></div></div></div><div class="row footer"><div class="col-sm-12 col-md-4">Fee <span class="amt txvalue copyable ms-3" cc="0 FAKE"><span class="prim-amt">0 FAKE</span></span></div><div class="col-sm-12 col-md-8 text-end"><span class="me-4"><span class="txvalue">1</span> confirmations</span><span class="amt txvalue copyable" cc="13.60030331 FAKE"><span class="prim-amt">13.<span class="amt-dec">60<span class="ns">030</span><span class="ns">331</span></span> FAKE</span></span></div></div></div></div><div class="pt-4"><h5>Raw Transaction</h5><div class="json"><pre id="raw"></pre></div><script type="text/javascript">var raw = {"hex":"","txid":"fdd824a780cbb718eeb766eb05d83fdefc793a27082cd5e67f856d69798cf7db","version":0,"locktime":0,"vin":[{"coinbase":"03bf1e1504aede765b726567696f6e312f50726f6a65637420425443506f6f6c2f01000001bf7e000000000000","txid":"","vout":0,"scriptSig":{"hex":""},"sequence":0,"addresses":null}],"vout":[{"ValueSat":1360030331,"value":0,"n":0,"scriptPubKey":{"hex":"76a914d03c0d863d189b23b061a95ad32940b65837609f88ac","addresses":null}},{"ValueSat":0,"value":0,"n":1,"scriptPubKey":{"addresses":null}}],"confirmations":1,"time":1521595678,"blocktime":1521595678,"size":300};document.getElementById('raw').innerHTML = syntaxHighlight(raw);</script></div></div></main><footer id="footer"><div class="container"><nav class="navbar navbar-dark"><span class="navbar-nav"><a class="nav-link" href="https://satoshilabs.com/" target="_blank" rel="noopener noreferrer">Created by SatoshiLabs</a></span><span class="navbar-nav ml-md-auto"><a class="nav-link" href="https://trezor.io/terms-of-use" target="_blank" rel="noopener noreferrer">Terms of Use</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/" target="_blank" rel="noopener noreferrer">Trezor</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/trezor-suite" target="_blank" rel="noopener noreferrer">Suite</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/support" target="_blank" rel="noopener noreferrer">Support</a></span><span class="navbar-nav ml-md-auto"><a class="nav-link" href="/sendtx">Send Transaction</a></span><span class="navbar-nav ml-md-auto d-lg-flex d-none"><a class="nav-link" href="https://trezor.io/compare" target="_blank" rel="noopener noreferrer">Don't have a Trezor? Get one!</a></span></nav></div></footer></body></html>`,
			},
		},
		{
			name:        "explorerAddress",
			r:           newGetRequest(ts.URL + "/address/mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz"),
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body: []string{
				`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1.0,shrink-to-fit=no"><link href="https://cdn.jsdelivr.net/npm/bootstrap@5.2.2/dist/css/bootstrap.min.css" rel="stylesheet" integrity="sha384-Zenh87qX5JnK2Jl0vWa8Ck2rdkQ2Bzep5IDxbcnCeuOxjzrPF/et3URy9Bv1WTRi" crossorigin="anonymous"><link rel="stylesheet" href="/static/css/main.min.3.css"><script>var hasSecondary=false;</script><script src="https://cdn.jsdelivr.net/npm/bootstrap@5.2.2/dist/js/bootstrap.bundle.min.js" integrity="sha384-OERcA2EqjJCMA+/3y+gxIOqMEjwtxJY7qPCqsdltbNJuaOe923+mo//f6V8Qbsw3" crossorigin="anonymous"></script><script src="/static/js/main.min.3.js"></script><meta http-equiv="X-UA-Compatible" content="IE=edge"><meta name="description" content="Trezor Fake Coin Explorer"><title>Trezor Fake Coin Explorer</title></head><body><header id="header"><nav class="navbar navbar-expand-lg"><div class="container"><a class="navbar-brand" href="/" title="Home"><span class="trezor-logo"></span><span style="padding-left: 140px;">Fake Coin Explorer</span></a><button class="navbar-toggler" type="button" data-bs-toggle="collapse" data-bs-target="#navbarSupportedContent" aria-controls="navbarSupportedContent" aria-expanded="false" aria-label="Toggle navigation"><span class="navbar-toggler-icon"></span></button><div class="collapse navbar-collapse" id="navbarSupportedContent"><ul class="navbar-nav m-md-auto"><li class="nav-item pe-xl-4"><a href="/blocks" class="nav-link">Blocks</a></li><li class="nav-item"><a href="/" class="nav-link">Status</a></li></ul><span class="navbar-form"><form class="d-flex" id="search" action="/search" method="get"><input name="q" type="text" class="form-control form-control-lg" placeholder="Search for block, transaction, address or xpub" focus="true"><button class="btn" type="submit"><span class="search-icon"></span></button></form></span></div></div></nav></header><main id="wrap"><div class="container"><div class="row g-0 ms-2 ms-lg-0"><div class="col-md-10 order-2 order-md-1"><h1>Address </h1><h5 class="col-12 d-flex h-data pb-2"><span class="ellipsis copyable">mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz</span></h5><h4 class="row"><div class="col-lg-6"><span class="copyable">0.<span class="amt-dec">000<span class="ns">123</span><span class="ns">45</span></span> FAKE</span></div></h4></div><div class="col-md-2 order-1 order-md-2 d-flex justify-content-center justify-content-md-end mb-3 mb-md-0"><div id="qrcode"></div><script type="text/javascript" src="/static/js/qrcode.min.js"></script><script type="text/javascript">new QRCode(document.getElementById("qrcode"), { text: "mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz", width: 120, height: 120 });</script></div></div><table class="table data-table info-table"><tbody><tr><td style="white-space: nowrap;"><h5>Confirmed</h5></td><td></td></tr><tr><td style="width: 25%;">Total Received</td><td><span class="amt copyable" cc="0.0002469 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">024</span><span class="ns">690</span></span> FAKE</span></span></td></tr><tr><td>Total Sent</td><td><span class="amt copyable" cc="0.00012345 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">012</span><span class="ns">345</span></span> FAKE</span></span></td></tr><tr><td>Final Balance</td><td><span class="amt copyable" cc="0.00012345 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">012</span><span class="ns">345</span></span> FAKE</span></span></td></tr><tr><td>No. Transactions</td><td>2</td></tr></tbody></table><div class="row pt-3 pb-1"><h3 class="col-sm-6 col-lg-3 m-0 align-self-center">Transactions</h3><div class="col-sm-6 col-lg-3 my-2 my-lg-0 align-self-center"><select  class="w-100" onchange="self.location='?filter='+options[selectedIndex].value"><option>All</option><option  value="inputs">Address on input side</option><option  value="outputs">Address on output side</option></select></div><div class="col-lg-6"></div></div><div><div class="tx-detail"><div class="row head"><div class="col-xs-7 col-md-8"><a href="/tx/7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25" class="ellipsis copyable txid">7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25</a></div><div class="col-xs-5 col-md-4 text-end">mined <span class="txvalue ms-1"><span tt="2018-03-21 01:27:58">1639 days 11 hours ago</span></span></div></div><div class="row body"><div class="col-md-5"><div class="row tx-in"><div class="col-12"><span class="ellipsis copyable"><a href="/address/mv9uLThosiEnGRbVPS7Vhyw6VssbVRsiAw">mv9uLThosiEnGRbVPS7Vhyw6VssbVRsiAw</a></span><span class="amt tx-amt copyable" cc="12345.67890123 FAKE"><span class="prim-amt">12<span class="nc">345</span>.<span class="amt-dec">67<span class="ns">890</span><span class="ns">123</span></span> FAKE</span></span></div><div class="col-12 tx-own"><span class="ellipsis copyable">mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz</span><span class="amt tx-amt copyable" cc="0.00012345 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">012</span><span class="ns">345</span></span> FAKE</span></span></div></div></div><div class="col-md-1 col-xs-12 text-center">&nbsp;<span class="octicon"></span></div><div class="col-md-6"><div class="row tx-out"><div class="col-12"><span class="ellipsis copyable"><a href="/address/mzB8cYrfRwFRFAGTDzV8LkUQy5BQicxGhX">mzB8cYrfRwFRFAGTDzV8LkUQy5BQicxGhX</a></span><span class="tx-amt"><span class="amt copyable" cc="3172.83951061 FAKE"><span class="prim-amt">3<span class="nc">172</span>.<span class="amt-dec">83<span class="ns">951</span><span class="ns">061</span></span> FAKE</span></span><a class="spent" href="/spending/7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25/0" tt="Spent">→</a></span></td></div><div class="col-12"><span class="ellipsis copyable"><a href="/address/mtR97eM2HPWVM6c8FGLGcukgaHHQv7THoL">mtR97eM2HPWVM6c8FGLGcukgaHHQv7THoL</a></span><span class="tx-amt"><span class="amt copyable" cc="9172.83951061 FAKE"><span class="prim-amt">9<span class="nc">172</span>.<span class="amt-dec">83<span class="ns">951</span><span class="ns">061</span></span> FAKE</span></span><span class="unspent" tt="Unspent">×</span></span></td></div><div class="col-12"><span class="ellipsis copyable">OP_RETURN 2020f1686f6a20</span><span class="tx-amt"><span class="amt copyable" cc="0 FAKE"><span class="prim-amt">0 FAKE</span></span><span class="unspent" tt="Unspent">×</span></span></td></div></div></div></div><div class="row footer"><div class="col-sm-12 col-md-4">Fee <span class="amt txvalue copyable ms-3" cc="0.00000346 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">000</span><span class="ns">346</span></span> FAKE</span></span></div><div class="col-sm-12 col-md-8 text-end"><span class="me-4"><span class="txvalue">1</span> confirmations</span><span class="amt txvalue copyable" cc="12345.67902122 FAKE"><span class="prim-amt">12<span class="nc">345</span>.<span class="amt-dec">67<span class="ns">902</span><span class="ns">122</span></span> FAKE</span></span></div></div></div><div class="tx-detail"><div class="row head"><div class="col-xs-7 col-md-8"><a href="/tx/00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa3840" class="ellipsis copyable txid">00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa3840</a></div><div class="col-xs-5 col-md-4 text-end">mined <span class="txvalue ms-1"><span tt="2018-03-20 03:03:46">1640 days 9 hours ago</span></span></div></div><div class="row body"><div class="col-md-5"><div class="row tx-in"><div class="col-12">No Inputs</div></div></div><div class="col-md-1 col-xs-12 text-center">&nbsp;<span class="octicon"></span></div><div class="col-md-6"><div class="row tx-out"><div class="col-12"><span class="ellipsis copyable"><a href="/address/mfcWp7DB6NuaZsExybTTXpVgWz559Np4Ti">mfcWp7DB6NuaZsExybTTXpVgWz559Np4Ti</a></span><span class="tx-amt"><span class="amt copyable" cc="1 FAKE"><span class="prim-amt">1.<span class="amt-dec">00<span class="ns">000</span><span class="ns">000</span></span> FAKE</span></span><span class="unspent" tt="Unspent">×</span></span></td></div><div class="col-12 tx-own"><span class="ellipsis copyable">mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz</span><span class="tx-amt"><span class="amt copyable" cc="0.00012345 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">012</span><span class="ns">345</span></span> FAKE</span></span><a class="spent" href="/spending/00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa3840/1" tt="Spent">→</a></span></td></div><div class="col-12 tx-own"><span class="ellipsis copyable">mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz</span><span class="tx-amt"><span class="amt copyable" cc="0.00012345 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">012</span><span class="ns">345</span></span> FAKE</span></span><span class="unspent" tt="Unspent">×</span></span></td></div></div></div></div><div class="row footer"><div class="col-sm-12 col-md-4">Fee <span class="amt txvalue copyable ms-3" cc="0 FAKE"><span class="prim-amt">0 FAKE</span></span></div><div class="col-sm-12 col-md-8 text-end"><span class="me-4"><span class="txvalue">2</span> confirmations</span><span class="amt txvalue copyable" cc="1.0002469 FAKE"><span class="prim-amt">1.<span class="amt-dec">00<span class="ns">024</span><span class="ns">690</span></span> FAKE</span></span></div></div></div></div></div></main><footer id="footer"><div class="container"><nav class="navbar navbar-dark"><span class="navbar-nav"><a class="nav-link" href="https://satoshilabs.com/" target="_blank" rel="noopener noreferrer">Created by SatoshiLabs</a></span><span class="navbar-nav ml-md-auto"><a class="nav-link" href="https://trezor.io/terms-of-use" target="_blank" rel="noopener noreferrer">Terms of Use</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/" target="_blank" rel="noopener noreferrer">Trezor</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/trezor-suite" target="_blank" rel="noopener noreferrer">Suite</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/support" target="_blank" rel="noopener noreferrer">Support</a></span><span class="navbar-nav ml-md-auto"><a class="nav-link" href="/sendtx">Send Transaction</a></span><span class="navbar-nav ml-md-auto d-lg-flex d-none"><a class="nav-link" href="https://trezor.io/compare" target="_blank" rel="noopener noreferrer">Don't have a Trezor? Get one!</a></span></nav></div></footer></body></html>`,
			},
		},
		{
			name:        "explorerSpendingTx",
			r:           newGetRequest(ts.URL + "/spending/7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25/0"),
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body: []string{
				`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1.0,shrink-to-fit=no"><link href="https://cdn.jsdelivr.net/npm/bootstrap@5.2.2/dist/css/bootstrap.min.css" rel="stylesheet" integrity="sha384-Zenh87qX5JnK2Jl0vWa8Ck2rdkQ2Bzep5IDxbcnCeuOxjzrPF/et3URy9Bv1WTRi" crossorigin="anonymous"><link rel="stylesheet" href="/static/css/main.min.3.css"><script>var hasSecondary=false;</script><script src="https://cdn.jsdelivr.net/npm/bootstrap@5.2.2/dist/js/bootstrap.bundle.min.js" integrity="sha384-OERcA2EqjJCMA+/3y+gxIOqMEjwtxJY7qPCqsdltbNJuaOe923+mo//f6V8Qbsw3" crossorigin="anonymous"></script><script src="/static/js/main.min.3.js"></script><meta http-equiv="X-UA-Compatible" content="IE=edge"><meta name="description" content="Trezor Fake Coin Explorer"><title>Trezor Fake Coin Explorer</title></head><body><header id="header"><nav class="navbar navbar-expand-lg"><div class="container"><a class="navbar-brand" href="/" title="Home"><span class="trezor-logo"></span><span style="padding-left: 140px;">Fake Coin Explorer</span></a><button class="navbar-toggler" type="button" data-bs-toggle="collapse" data-bs-target="#navbarSupportedContent" aria-controls="navbarSupportedContent" aria-expanded="false" aria-label="Toggle navigation"><span class="navbar-toggler-icon"></span></button><div class="collapse navbar-collapse" id="navbarSupportedContent"><ul class="navbar-nav m-md-auto"><li class="nav-item pe-xl-4"><a href="/blocks" class="nav-link">Blocks</a></li><li class="nav-item"><a href="/" class="nav-link">Status</a></li></ul><span class="navbar-form"><form class="d-flex" id="search" action="/search" method="get"><input name="q" type="text" class="form-control form-control-lg" placeholder="Search for block, transaction, address or xpub" focus="true"><button class="btn" type="submit"><span class="search-icon"></span></button></form></span></div></div></nav></header><main id="wrap"><div class="container"><div class="row"><h1 class="col-12">Transaction</h1></div><div class="row pt-3"><h5 class="col-12 d-flex h-data"><span class="ellipsis copyable">3d90d15ed026dc45e19ffb52875ed18fa9e8012ad123d7f7212176e2b0ebdb71</span></h5></div><table class="table data-table info-table"><tbody><tr><td>Mined Time</td><td><span tt="2018-03-21 01:27:58">1639 days 11 hours ago</span></td></tr><tr><td>In Block</td><td class="ellipsis" style="max-width: 280px">00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6</td></tr><tr><td>In Block Height</td><td><a href="/block/225494">225<span class="ns">494</span></a></td></tr><tr><td>Total Input</td><td><span class="amt copyable" cc="3172.83951062 FAKE"><span class="prim-amt">3<span class="nc">172</span>.<span class="amt-dec">83<span class="ns">951</span><span class="ns">062</span></span> FAKE</span></span></td></tr><tr><td>Total Output</td><td><span class="amt copyable" cc="3172.83951 FAKE"><span class="prim-amt">3<span class="nc">172</span>.<span class="amt-dec">83<span class="ns">951</span><span class="ns">000</span></span> FAKE</span></span></td></tr><tr><td>Fees</td><td><span class="amt copyable" cc="0.00000062 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">000</span><span class="ns">062</span></span> FAKE</span></span></td></tr></tbody></table><div class="pt-1"><div class="tx-detail"><div class="row head"><div class="col-xs-7 col-md-8"><a href="/tx/3d90d15ed026dc45e19ffb52875ed18fa9e8012ad123d7f7212176e2b0ebdb71" class="ellipsis copyable txid">3d90d15ed026dc45e19ffb52875ed18fa9e8012ad123d7f7212176e2b0ebdb71</a></div><div class="col-xs-5 col-md-4 text-end">mined <span class="txvalue ms-1"><span tt="2018-03-21 01:27:58">1639 days 11 hours ago</span></span></div></div><div class="row body"><div class="col-md-5"><div class="row tx-in"><div class="col-12"><span class="ellipsis copyable"><a href="/address/mzB8cYrfRwFRFAGTDzV8LkUQy5BQicxGhX">mzB8cYrfRwFRFAGTDzV8LkUQy5BQicxGhX</a></span><a class="outpoint" href="/tx/7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25" tt="Outpoint 7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25,0">←</a><span class="amt tx-amt copyable" cc="3172.83951061 FAKE"><span class="prim-amt">3<span class="nc">172</span>.<span class="amt-dec">83<span class="ns">951</span><span class="ns">061</span></span> FAKE</span></span></div><div class="col-12"><span class="ellipsis copyable"><a href="/address/2MzmAKayJmja784jyHvRUW1bXPget1csRRG">2MzmAKayJmja784jyHvRUW1bXPget1csRRG</a></span><a class="outpoint" href="/tx/effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75" tt="Outpoint effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75,1">←</a><span class="amt tx-amt copyable" cc="0.00000001 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">000</span><span class="ns">001</span></span> FAKE</span></span></div></div></div><div class="col-md-1 col-xs-12 text-center">&nbsp;<span class="octicon"></span></div><div class="col-md-6"><div class="row tx-out"><div class="col-12"><span class="ellipsis copyable"><a href="/address/2N6utyMZfPNUb1Bk8oz7p2JqJrXkq83gegu">2N6utyMZfPNUb1Bk8oz7p2JqJrXkq83gegu</a></span><span class="tx-amt"><span class="amt copyable" cc="1186.419755 FAKE"><span class="prim-amt">1<span class="nc">186</span>.<span class="amt-dec">41<span class="ns">975</span><span class="ns">500</span></span> FAKE</span></span><span class="unspent" tt="Unspent">×</span></span></td></div><div class="col-12"><span class="ellipsis copyable"><a href="/address/mmJx9Y8ayz9h14yd9fgCW1bUKoEpkBAquP">mmJx9Y8ayz9h14yd9fgCW1bUKoEpkBAquP</a></span><span class="tx-amt"><span class="amt copyable" cc="1986.419755 FAKE"><span class="prim-amt">1<span class="nc">986</span>.<span class="amt-dec">41<span class="ns">975</span><span class="ns">500</span></span> FAKE</span></span><span class="unspent" tt="Unspent">×</span></span></td></div></div></div></div><div class="row footer"><div class="col-sm-12 col-md-4">Fee <span class="amt txvalue copyable ms-3" cc="0.00000062 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">000</span><span class="ns">062</span></span> FAKE</span></span></div><div class="col-sm-12 col-md-8 text-end"><span class="me-4"><span class="txvalue">1</span> confirmations</span><span class="amt txvalue copyable" cc="3172.83951 FAKE"><span class="prim-amt">3<span class="nc">172</span>.<span class="amt-dec">83<span class="ns">951</span><span class="ns">000</span></span> FAKE</span></span></div></div></div></div><div class="pt-4"><h5>Raw Transaction</h5><div class="json"><pre id="raw"></pre></div><script type="text/javascript">var raw = {"hex":"","txid":"3d90d15ed026dc45e19ffb52875ed18fa9e8012ad123d7f7212176e2b0ebdb71","version":0,"locktime":0,"vin":[{"coinbase":"","txid":"7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25","vout":0,"scriptSig":{"hex":""},"sequence":0,"addresses":null},{"coinbase":"","txid":"effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75","vout":1,"scriptSig":{"hex":""},"sequence":0,"addresses":null}],"vout":[{"ValueSat":118641975500,"value":0,"n":0,"scriptPubKey":{"hex":"a91495e9fbe306449c991d314afe3c3567d5bf78efd287","addresses":null}},{"ValueSat":198641975500,"value":0,"n":1,"scriptPubKey":{"hex":"76a9143f8ba3fda3ba7b69f5818086e12223c6dd25e3c888ac","addresses":null}}],"confirmations":1,"time":1521595678,"blocktime":1521595678,"vsize":400};document.getElementById('raw').innerHTML = syntaxHighlight(raw);</script></div></div></main><footer id="footer"><div class="container"><nav class="navbar navbar-dark"><span class="navbar-nav"><a class="nav-link" href="https://satoshilabs.com/" target="_blank" rel="noopener noreferrer">Created by SatoshiLabs</a></span><span class="navbar-nav ml-md-auto"><a class="nav-link" href="https://trezor.io/terms-of-use" target="_blank" rel="noopener noreferrer">Terms of Use</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/" target="_blank" rel="noopener noreferrer">Trezor</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/trezor-suite" target="_blank" rel="noopener noreferrer">Suite</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/support" target="_blank" rel="noopener noreferrer">Support</a></span><span class="navbar-nav ml-md-auto"><a class="nav-link" href="/sendtx">Send Transaction</a></span><span class="navbar-nav ml-md-auto d-lg-flex d-none"><a class="nav-link" href="https://trezor.io/compare" target="_blank" rel="noopener noreferrer">Don't have a Trezor? Get one!</a></span></nav></div></footer></body></html>`,
			},
		},
		{
			name:        "explorerSpendingTx - not found",
			r:           newGetRequest(ts.URL + "/spending/123be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25/0"),
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body: []string{
				`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1.0,shrink-to-fit=no"><link href="https://cdn.jsdelivr.net/npm/bootstrap@5.2.2/dist/css/bootstrap.min.css" rel="stylesheet" integrity="sha384-Zenh87qX5JnK2Jl0vWa8Ck2rdkQ2Bzep5IDxbcnCeuOxjzrPF/et3URy9Bv1WTRi" crossorigin="anonymous"><link rel="stylesheet" href="/static/css/main.min.3.css"><script>var hasSecondary=false;</script><script src="https://cdn.jsdelivr.net/npm/bootstrap@5.2.2/dist/js/bootstrap.bundle.min.js" integrity="sha384-OERcA2EqjJCMA+/3y+gxIOqMEjwtxJY7qPCqsdltbNJuaOe923+mo//f6V8Qbsw3" crossorigin="anonymous"></script><script src="/static/js/main.min.3.js"></script><meta http-equiv="X-UA-Compatible" content="IE=edge"><meta name="description" content="Trezor Fake Coin Explorer"><title>Trezor Fake Coin Explorer</title></head><body><header id="header"><nav class="navbar navbar-expand-lg"><div class="container"><a class="navbar-brand" href="/" title="Home"><span class="trezor-logo"></span><span style="padding-left: 140px;">Fake Coin Explorer</span></a><button class="navbar-toggler" type="button" data-bs-toggle="collapse" data-bs-target="#navbarSupportedContent" aria-controls="navbarSupportedContent" aria-expanded="false" aria-label="Toggle navigation"><span class="navbar-toggler-icon"></span></button><div class="collapse navbar-collapse" id="navbarSupportedContent"><ul class="navbar-nav m-md-auto"><li class="nav-item pe-xl-4"><a href="/blocks" class="nav-link">Blocks</a></li><li class="nav-item"><a href="/" class="nav-link">Status</a></li></ul><span class="navbar-form"><form class="d-flex" id="search" action="/search" method="get"><input name="q" type="text" class="form-control form-control-lg" placeholder="Search for block, transaction, address or xpub" focus="true"><button class="btn" type="submit"><span class="search-icon"></span></button></form></span></div></div></nav></header><main id="wrap"><div class="container"><h1>Error</h1><h4>Transaction not found</h4></div></main><footer id="footer"><div class="container"><nav class="navbar navbar-dark"><span class="navbar-nav"><a class="nav-link" href="https://satoshilabs.com/" target="_blank" rel="noopener noreferrer">Created by SatoshiLabs</a></span><span class="navbar-nav ml-md-auto"><a class="nav-link" href="https://trezor.io/terms-of-use" target="_blank" rel="noopener noreferrer">Terms of Use</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/" target="_blank" rel="noopener noreferrer">Trezor</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/trezor-suite" target="_blank" rel="noopener noreferrer">Suite</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/support" target="_blank" rel="noopener noreferrer">Support</a></span><span class="navbar-nav ml-md-auto"><a class="nav-link" href="/sendtx">Send Transaction</a></span><span class="navbar-nav ml-md-auto d-lg-flex d-none"><a class="nav-link" href="https://trezor.io/compare" target="_blank" rel="noopener noreferrer">Don't have a Trezor? Get one!</a></span></nav></div></footer></body></html>`,
			},
		},
		{
			name:        "explorerBlocks",
			r:           newGetRequest(ts.URL + "/blocks"),
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body: []string{
				`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1.0,shrink-to-fit=no"><link href="https://cdn.jsdelivr.net/npm/bootstrap@5.2.2/dist/css/bootstrap.min.css" rel="stylesheet" integrity="sha384-Zenh87qX5JnK2Jl0vWa8Ck2rdkQ2Bzep5IDxbcnCeuOxjzrPF/et3URy9Bv1WTRi" crossorigin="anonymous"><link rel="stylesheet" href="/static/css/main.min.3.css"><script>var hasSecondary=false;</script><script src="https://cdn.jsdelivr.net/npm/bootstrap@5.2.2/dist/js/bootstrap.bundle.min.js" integrity="sha384-OERcA2EqjJCMA+/3y+gxIOqMEjwtxJY7qPCqsdltbNJuaOe923+mo//f6V8Qbsw3" crossorigin="anonymous"></script><script src="/static/js/main.min.3.js"></script><meta http-equiv="X-UA-Compatible" content="IE=edge"><meta name="description" content="Trezor Fake Coin Explorer"><title>Trezor Fake Coin Explorer</title></head><body><header id="header"><nav class="navbar navbar-expand-lg"><div class="container"><a class="navbar-brand" href="/" title="Home"><span class="trezor-logo"></span><span style="padding-left: 140px;">Fake Coin Explorer</span></a><button class="navbar-toggler" type="button" data-bs-toggle="collapse" data-bs-target="#navbarSupportedContent" aria-controls="navbarSupportedContent" aria-expanded="false" aria-label="Toggle navigation"><span class="navbar-toggler-icon"></span></button><div class="collapse navbar-collapse" id="navbarSupportedContent"><ul class="navbar-nav m-md-auto"><li class="nav-item pe-xl-4"><a href="/blocks" class="nav-link">Blocks</a></li><li class="nav-item"><a href="/" class="nav-link">Status</a></li></ul><span class="navbar-form"><form class="d-flex" id="search" action="/search" method="get"><input name="q" type="text" class="form-control form-control-lg" placeholder="Search for block, transaction, address or xpub" focus="true"><button class="btn" type="submit"><span class="search-icon"></span></button></form></span></div></div></nav></header><main id="wrap"><div class="container"><div class="row"><div class="col-md-6"><h1>Blocks</h1></div><div class="col-md-6"><nav class="paging justify-content-end"><a class="btn btn-paging" href="?page=1">Previous</a><div class="paging-group mx-2"><div class="bb-group"><a class="btn active" href="?page=1">1</a><a class="btn" href="?page=2">2</a><a class="btn" href="?page=3">3</a><a class="btn" href="?page=4">4</a><span>...</span><a class="btn" href="?page=4510">4510</a></div></div><a class="btn btn-paging" href="?page=2">Next</a></nav></div></row><div><table class="table table-hover data-table"><thead><tr><th>Height</th><th class="col-md-6">Hash</th><th class="col-md-2">Timestamp</span></th><th class="text-end">Transactions</th><th class="text-end">Size</th></tr></thead><tbody><tr><td><a href="/block/225494">225<span class="ns">494</span></a></td><td class="ellipsis">00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6</td><td><span tt="2018-03-21 01:27:58">1639 days 11 hours ago</span></td><td class="text-end">4</td><td class="text-end">2<span class="ns">345</span><span class="ns">678</span></td></tr><tr><td><a href="/block/225493">225<span class="ns">493</span></a></td><td class="ellipsis">0000000076fbbed90fd75b0e18856aa35baa984e9c9d444cf746ad85e94e2997</td><td><span tt="2018-03-20 03:03:46">1640 days 9 hours ago</span></td><td class="text-end">2</td><td class="text-end">1<span class="ns">234</span><span class="ns">567</span></td></tr></tbody></table></div><nav class="paging justify-content-end"><a class="btn btn-paging" href="?page=1">Previous</a><div class="paging-group mx-2"><div class="bb-group"><a class="btn active" href="?page=1">1</a><a class="btn" href="?page=2">2</a><a class="btn" href="?page=3">3</a><a class="btn" href="?page=4">4</a><span>...</span><a class="btn" href="?page=4510">4510</a></div></div><a class="btn btn-paging" href="?page=2">Next</a></nav></div></main><footer id="footer"><div class="container"><nav class="navbar navbar-dark"><span class="navbar-nav"><a class="nav-link" href="https://satoshilabs.com/" target="_blank" rel="noopener noreferrer">Created by SatoshiLabs</a></span><span class="navbar-nav ml-md-auto"><a class="nav-link" href="https://trezor.io/terms-of-use" target="_blank" rel="noopener noreferrer">Terms of Use</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/" target="_blank" rel="noopener noreferrer">Trezor</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/trezor-suite" target="_blank" rel="noopener noreferrer">Suite</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/support" target="_blank" rel="noopener noreferrer">Support</a></span><span class="navbar-nav ml-md-auto"><a class="nav-link" href="/sendtx">Send Transaction</a></span><span class="navbar-nav ml-md-auto d-lg-flex d-none"><a class="nav-link" href="https://trezor.io/compare" target="_blank" rel="noopener noreferrer">Don't have a Trezor? Get one!</a></span></nav></div></footer></body></html>`,
			},
		},
		{
			name:        "explorerBlock",
			r:           newGetRequest(ts.URL + "/block/225494"),
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body: []string{
				`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1.0,shrink-to-fit=no"><link href="https://cdn.jsdelivr.net/npm/bootstrap@5.2.2/dist/css/bootstrap.min.css" rel="stylesheet" integrity="sha384-Zenh87qX5JnK2Jl0vWa8Ck2rdkQ2Bzep5IDxbcnCeuOxjzrPF/et3URy9Bv1WTRi" crossorigin="anonymous"><link rel="stylesheet" href="/static/css/main.min.3.css"><script>var hasSecondary=false;</script><script src="https://cdn.jsdelivr.net/npm/bootstrap@5.2.2/dist/js/bootstrap.bundle.min.js" integrity="sha384-OERcA2EqjJCMA+/3y+gxIOqMEjwtxJY7qPCqsdltbNJuaOe923+mo//f6V8Qbsw3" crossorigin="anonymous"></script><script src="/static/js/main.min.3.js"></script><meta http-equiv="X-UA-Compatible" content="IE=edge"><meta name="description" content="Trezor Fake Coin Explorer"><title>Trezor Fake Coin Explorer</title></head><body><header id="header"><nav class="navbar navbar-expand-lg"><div class="container"><a class="navbar-brand" href="/" title="Home"><span class="trezor-logo"></span><span style="padding-left: 140px;">Fake Coin Explorer</span></a><button class="navbar-toggler" type="button" data-bs-toggle="collapse" data-bs-target="#navbarSupportedContent" aria-controls="navbarSupportedContent" aria-expanded="false" aria-label="Toggle navigation"><span class="navbar-toggler-icon"></span></button><div class="collapse navbar-collapse" id="navbarSupportedContent"><ul class="navbar-nav m-md-auto"><li class="nav-item pe-xl-4"><a href="/blocks" class="nav-link">Blocks</a></li><li class="nav-item"><a href="/" class="nav-link">Status</a></li></ul><span class="navbar-form"><form class="d-flex" id="search" action="/search" method="get"><input name="q" type="text" class="form-control form-control-lg" placeholder="Search for block, transaction, address or xpub" focus="true"><button class="btn" type="submit"><span class="search-icon"></span></button></form></span></div></div></nav></header><main id="wrap"><div class="container"><div class="row"><h1 class="col-sm-6">Block</h1><nav class="col-sm-6 paging justify-content-end pb-1"><a class="btn btn-paging" href="/block/0000000076fbbed90fd75b0e18856aa35baa984e9c9d444cf746ad85e94e2997">Previous Block</a><span class="btn btn-paging ms-2">Next Block</span></nav></div><div class="row pt-3"><h5 class="col-lg-3 h-data"><span class="ellipsis copyable">225<span class="ns">494</span></span></h5><h5 class="col-lg-9 justify-content-end d-flex h-data"><span class="copyable ellipsis">00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6</span></h5></div><div class="row"><div class="col-md-6"><table class="table data-table info-table"><tbody><tr><td>Transactions</td><td>4</td></tr><tr><td>Height</td><td>225<span class="ns">494</span></td></tr><tr><td>Confirmations</td><td>1</td></tr><tr><td>Timestamp</td><td><span tt="2018-03-21 01:27:58">1639 days 11 hours ago</span></td></tr><tr><td>Size (bytes)</td><td>2<span class="ns">345</span><span class="ns">678</span></td></tr></tbody></table></div><div class="col-md-6"><table class="table data-table info-table"><tbody><tr><td>Version</td><td class="ellipsis"></td></tr><tr><td>Merkle&nbsp;Root</td><td style="max-width: 280px;" class="ellipsis"></td></tr><tr><td>Nonce</td><td style="max-width: 280px;" class="ellipsis"></td></tr><tr><td>Bits</td><td class="ellipsis"></td></tr><tr><td>Difficulty</td><td class="ellipsis"></td></tr></tbody></table></div></div><div class="row pt-3 pb-1"><h3 class="col-md-6 align-self-center">Transactions</h3><div class="col-md-6"></div></div><div><div class="tx-detail"><div class="row head"><div class="col-xs-7 col-md-8"><a href="/tx/7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25" class="ellipsis copyable txid">7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25</a></div><div class="col-xs-5 col-md-4 text-end">mined <span class="txvalue ms-1"><span tt="2018-03-21 01:27:58">1639 days 11 hours ago</span></span></div></div><div class="row body"><div class="col-md-5"><div class="row tx-in"><div class="col-12"><span class="ellipsis copyable"><a href="/address/mv9uLThosiEnGRbVPS7Vhyw6VssbVRsiAw">mv9uLThosiEnGRbVPS7Vhyw6VssbVRsiAw</a></span><span class="amt tx-amt copyable" cc="12345.67890123 FAKE"><span class="prim-amt">12<span class="nc">345</span>.<span class="amt-dec">67<span class="ns">890</span><span class="ns">123</span></span> FAKE</span></span></div><div class="col-12"><span class="ellipsis copyable"><a href="/address/mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz">mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz</a></span><span class="amt tx-amt copyable" cc="0.00012345 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">012</span><span class="ns">345</span></span> FAKE</span></span></div></div></div><div class="col-md-1 col-xs-12 text-center">&nbsp;<span class="octicon"></span></div><div class="col-md-6"><div class="row tx-out"><div class="col-12"><span class="ellipsis copyable"><a href="/address/mzB8cYrfRwFRFAGTDzV8LkUQy5BQicxGhX">mzB8cYrfRwFRFAGTDzV8LkUQy5BQicxGhX</a></span><span class="tx-amt"><span class="amt copyable" cc="3172.83951061 FAKE"><span class="prim-amt">3<span class="nc">172</span>.<span class="amt-dec">83<span class="ns">951</span><span class="ns">061</span></span> FAKE</span></span><a class="spent" href="/spending/7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25/0" tt="Spent">→</a></span></td></div><div class="col-12"><span class="ellipsis copyable"><a href="/address/mtR97eM2HPWVM6c8FGLGcukgaHHQv7THoL">mtR97eM2HPWVM6c8FGLGcukgaHHQv7THoL</a></span><span class="tx-amt"><span class="amt copyable" cc="9172.83951061 FAKE"><span class="prim-amt">9<span class="nc">172</span>.<span class="amt-dec">83<span class="ns">951</span><span class="ns">061</span></span> FAKE</span></span><span class="unspent" tt="Unspent">×</span></span></td></div><div class="col-12"><span class="ellipsis copyable">OP_RETURN 2020f1686f6a20</span><span class="tx-amt"><span class="amt copyable" cc="0 FAKE"><span class="prim-amt">0 FAKE</span></span><span class="unspent" tt="Unspent">×</span></span></td></div></div></div></div><div class="row footer"><div class="col-sm-12 col-md-4">Fee <span class="amt txvalue copyable ms-3" cc="0.00000346 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">000</span><span class="ns">346</span></span> FAKE</span></span></div><div class="col-sm-12 col-md-8 text-end"><span class="me-4"><span class="txvalue">1</span> confirmations</span><span class="amt txvalue copyable" cc="12345.67902122 FAKE"><span class="prim-amt">12<span class="nc">345</span>.<span class="amt-dec">67<span class="ns">902</span><span class="ns">122</span></span> FAKE</span></span></div></div></div><div class="tx-detail"><div class="row head"><div class="col-xs-7 col-md-8"><a href="/tx/3d90d15ed026dc45e19ffb52875ed18fa9e8012ad123d7f7212176e2b0ebdb71" class="ellipsis copyable txid">3d90d15ed026dc45e19ffb52875ed18fa9e8012ad123d7f7212176e2b0ebdb71</a></div><div class="col-xs-5 col-md-4 text-end">mined <span class="txvalue ms-1"><span tt="2018-03-21 01:27:58">1639 days 11 hours ago</span></span></div></div><div class="row body"><div class="col-md-5"><div class="row tx-in"><div class="col-12"><span class="ellipsis copyable"><a href="/address/mzB8cYrfRwFRFAGTDzV8LkUQy5BQicxGhX">mzB8cYrfRwFRFAGTDzV8LkUQy5BQicxGhX</a></span><span class="amt tx-amt copyable" cc="3172.83951061 FAKE"><span class="prim-amt">3<span class="nc">172</span>.<span class="amt-dec">83<span class="ns">951</span><span class="ns">061</span></span> FAKE</span></span></div><div class="col-12"><span class="ellipsis copyable"><a href="/address/2MzmAKayJmja784jyHvRUW1bXPget1csRRG">2MzmAKayJmja784jyHvRUW1bXPget1csRRG</a></span><span class="amt tx-amt copyable" cc="0.00000001 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">000</span><span class="ns">001</span></span> FAKE</span></span></div></div></div><div class="col-md-1 col-xs-12 text-center">&nbsp;<span class="octicon"></span></div><div class="col-md-6"><div class="row tx-out"><div class="col-12"><span class="ellipsis copyable"><a href="/address/2N6utyMZfPNUb1Bk8oz7p2JqJrXkq83gegu">2N6utyMZfPNUb1Bk8oz7p2JqJrXkq83gegu</a></span><span class="tx-amt"><span class="amt copyable" cc="1186.419755 FAKE"><span class="prim-amt">1<span class="nc">186</span>.<span class="amt-dec">41<span class="ns">975</span><span class="ns">500</span></span> FAKE</span></span><span class="unspent" tt="Unspent">×</span></span></td></div><div class="col-12"><span class="ellipsis copyable"><a href="/address/mmJx9Y8ayz9h14yd9fgCW1bUKoEpkBAquP">mmJx9Y8ayz9h14yd9fgCW1bUKoEpkBAquP</a></span><span class="tx-amt"><span class="amt copyable" cc="1986.419755 FAKE"><span class="prim-amt">1<span class="nc">986</span>.<span class="amt-dec">41<span class="ns">975</span><span class="ns">500</span></span> FAKE</span></span><span class="unspent" tt="Unspent">×</span></span></td></div></div></div></div><div class="row footer"><div class="col-sm-12 col-md-4">Fee <span class="amt txvalue copyable ms-3" cc="0.00000062 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">000</span><span class="ns">062</span></span> FAKE</span></span></div><div class="col-sm-12 col-md-8 text-end"><span class="me-4"><span class="txvalue">1</span> confirmations</span><span class="amt txvalue copyable" cc="3172.83951 FAKE"><span class="prim-amt">3<span class="nc">172</span>.<span class="amt-dec">83<span class="ns">951</span><span class="ns">000</span></span> FAKE</span></span></div></div></div><div class="tx-detail"><div class="row head"><div class="col-xs-7 col-md-8"><a href="/tx/05e2e48aeabdd9b75def7b48d756ba304713c2aba7b522bf9dbc893fc4231b07" class="ellipsis copyable txid">05e2e48aeabdd9b75def7b48d756ba304713c2aba7b522bf9dbc893fc4231b07</a></div><div class="col-xs-5 col-md-4 text-end">mined <span class="txvalue ms-1"><span tt="2018-03-21 01:27:58">1639 days 11 hours ago</span></span></div></div><div class="row body"><div class="col-md-5"><div class="row tx-in"><div class="col-12"><span class="ellipsis copyable"><a href="/address/2NEVv9LJmAnY99W1pFoc5UJjVdypBqdnvu1">2NEVv9LJmAnY99W1pFoc5UJjVdypBqdnvu1</a></span><span class="amt tx-amt copyable" cc="0.00009876 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">009</span><span class="ns">876</span></span> FAKE</span></span></div></div></div><div class="col-md-1 col-xs-12 text-center">&nbsp;<span class="octicon"></span></div><div class="col-md-6"><div class="row tx-out"><div class="col-12"><span class="ellipsis copyable"><a href="/address/2NEVv9LJmAnY99W1pFoc5UJjVdypBqdnvu1">2NEVv9LJmAnY99W1pFoc5UJjVdypBqdnvu1</a></span><span class="tx-amt"><span class="amt copyable" cc="0.00009 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">009</span><span class="ns">000</span></span> FAKE</span></span><span class="unspent" tt="Unspent">×</span></span></td></div></div></div></div><div class="row footer"><div class="col-sm-12 col-md-4">Fee <span class="amt txvalue copyable ms-3" cc="0.00000876 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">000</span><span class="ns">876</span></span> FAKE</span></span></div><div class="col-sm-12 col-md-8 text-end"><span class="me-4"><span class="txvalue">1</span> confirmations</span><span class="amt txvalue copyable" cc="0.00009 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">009</span><span class="ns">000</span></span> FAKE</span></span></div></div></div><div class="tx-detail"><div class="row head"><div class="col-xs-7 col-md-8"><a href="/tx/fdd824a780cbb718eeb766eb05d83fdefc793a27082cd5e67f856d69798cf7db" class="ellipsis copyable txid">fdd824a780cbb718eeb766eb05d83fdefc793a27082cd5e67f856d69798cf7db</a></div><div class="col-xs-5 col-md-4 text-end">mined <span class="txvalue ms-1"><span tt="2018-03-21 01:27:58">1639 days 11 hours ago</span></span></div></div><div class="row body"><div class="col-md-5"><div class="row tx-in"><div class="col-12">No Inputs (Newly Generated Coins)</div></div></div><div class="col-md-1 col-xs-12 text-center">&nbsp;<span class="octicon"></span></div><div class="col-md-6"><div class="row tx-out"><div class="col-12"><span class="ellipsis copyable"><a href="/address/mzVznVsCHkVHX9UN8WPFASWUUHtxnNn4Jj">mzVznVsCHkVHX9UN8WPFASWUUHtxnNn4Jj</a></span><span class="tx-amt"><span class="amt copyable" cc="13.60030331 FAKE"><span class="prim-amt">13.<span class="amt-dec">60<span class="ns">030</span><span class="ns">331</span></span> FAKE</span></span><span class="unspent" tt="Unspent">×</span></span></td></div><div class="col-12">Unparsed address<span class="tx-amt"><span class="amt copyable" cc="0 FAKE"><span class="prim-amt">0 FAKE</span></span><span class="unspent" tt="Unspent">×</span></span></td></div></div></div></div><div class="row footer"><div class="col-sm-12 col-md-4">Fee <span class="amt txvalue copyable ms-3" cc="0 FAKE"><span class="prim-amt">0 FAKE</span></span></div><div class="col-sm-12 col-md-8 text-end"><span class="me-4"><span class="txvalue">1</span> confirmations</span><span class="amt txvalue copyable" cc="13.60030331 FAKE"><span class="prim-amt">13.<span class="amt-dec">60<span class="ns">030</span><span class="ns">331</span></span> FAKE</span></span></div></div></div></div></div></main><footer id="footer"><div class="container"><nav class="navbar navbar-dark"><span class="navbar-nav"><a class="nav-link" href="https://satoshilabs.com/" target="_blank" rel="noopener noreferrer">Created by SatoshiLabs</a></span><span class="navbar-nav ml-md-auto"><a class="nav-link" href="https://trezor.io/terms-of-use" target="_blank" rel="noopener noreferrer">Terms of Use</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/" target="_blank" rel="noopener noreferrer">Trezor</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/trezor-suite" target="_blank" rel="noopener noreferrer">Suite</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/support" target="_blank" rel="noopener noreferrer">Support</a></span><span class="navbar-nav ml-md-auto"><a class="nav-link" href="/sendtx">Send Transaction</a></span><span class="navbar-nav ml-md-auto d-lg-flex d-none"><a class="nav-link" href="https://trezor.io/compare" target="_blank" rel="noopener noreferrer">Don't have a Trezor? Get one!</a></span></nav></div></footer></body></html>`,
			},
		},
		{
			name:        "explorerIndex",
			r:           newGetRequest(ts.URL + "/"),
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body: []string{
				`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1.0,shrink-to-fit=no"><link href="https://cdn.jsdelivr.net/npm/bootstrap@5.2.2/dist/css/bootstrap.min.css" rel="stylesheet" integrity="sha384-Zenh87qX5JnK2Jl0vWa8Ck2rdkQ2Bzep5IDxbcnCeuOxjzrPF/et3URy9Bv1WTRi" crossorigin="anonymous"><link rel="stylesheet" href="/static/css/main.min.3.css"><script>var hasSecondary=false;</script><script src="https://cdn.jsdelivr.net/npm/bootstrap@5.2.2/dist/js/bootstrap.bundle.min.js" integrity="sha384-OERcA2EqjJCMA+/3y+gxIOqMEjwtxJY7qPCqsdltbNJuaOe923+mo//f6V8Qbsw3" crossorigin="anonymous"></script><script src="/static/js/main.min.3.js"></script><meta http-equiv="X-UA-Compatible" content="IE=edge"><meta name="description" content="Trezor Fake Coin Explorer"><title>Trezor Fake Coin Explorer</title></head><body><header id="header"><nav class="navbar navbar-expand-lg"><div class="container"><a class="navbar-brand" href="/" title="Home"><span class="trezor-logo"></span><span style="padding-left: 140px;">Fake Coin Explorer</span></a><button class="navbar-toggler" type="button" data-bs-toggle="collapse" data-bs-target="#navbarSupportedContent" aria-controls="navbarSupportedContent" aria-expanded="false" aria-label="Toggle navigation"><span class="navbar-toggler-icon"></span></button><div class="collapse navbar-collapse" id="navbarSupportedContent"><ul class="navbar-nav m-md-auto"><li class="nav-item pe-xl-4"><a href="/blocks" class="nav-link">Blocks</a></li><li class="nav-item"><a href="/" class="nav-link">Status</a></li></ul><span class="navbar-form"><form class="d-flex" id="search" action="/search" method="get"><input name="q" type="text" class="form-control form-control-lg" placeholder="Search for block, transaction, address or xpub" focus="true"><button class="btn" type="submit"><span class="search-icon"></span></button></form></span></div></div></nav></header><main id="wrap"><div class="container"><h1>Application status</h1><h3><span class="badge bg-warning text-white p-3 w-100" style="white-space: break-spaces;">Synchronization with backend is disabled, the state of index is not up to date.</span></h3><div class="row"><div class="col-lg-6"><table class="table data-table info-table"><tbody><tr><td style="white-space: nowrap;"><h3>Blockbook</h3></td><td></td></tr><tr><td>Coin</td><td>Fakecoin</td></tr><tr><td>Host</td><td></td></tr><tr><td>Version / Commit / Build</td><td>unknown / <a href="https://github.com/trezor/blockbook/commit/unknown" target="_blank" rel="noopener noreferrer">unknown</a> / unknown</td></tr><tr><td>Synchronized</td><td><h6 class="badge bg-success">true</h6></td></tr><tr><td>Last Block</td><td><a href="/block/225494">225<span class="ns">494</span></a></td></tr><tr><td>Last Block Update</td><td>`,
				`</td></tr><tr><td>Mempool in Sync</td><td><h6 class="badge bg-danger">false</h6></td></tr><tr><td>Last Mempool Update</td><td></td></tr><tr><td>Transactions in Mempool</td><td><a href="/mempool">0</a></td></tr><tr><td>Current Fiat rates</td>`,
				`</td></tr></tbody></table></div><div class="col-lg-6"><table class="table data-table info-table"><tbody><tr><td style="white-space: nowrap;"><h3>Backend</h3></td><td></td></tr><tr><td>Chain</td><td>fakecoin</td></tr><tr><td>Version</td><td>001001</td></tr><tr><td>Subversion</td><td>/Fakecoin:0.0.1/</td></tr><tr><td>Last Block</td><td>2</td></tr><tr><td>Difficulty</td><td></td></tr></tbody></table></div></div><span class="text-muted">Blockbook - blockchain indexer for Trezor Suite https://trezor.io/trezor-suite. Do not use for any other purpose.</span></div></main><footer id="footer"><div class="container"><nav class="navbar navbar-dark"><span class="navbar-nav"><a class="nav-link" href="https://satoshilabs.com/" target="_blank" rel="noopener noreferrer">Created by SatoshiLabs</a></span><span class="navbar-nav ml-md-auto"><a class="nav-link" href="https://trezor.io/terms-of-use" target="_blank" rel="noopener noreferrer">Terms of Use</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/" target="_blank" rel="noopener noreferrer">Trezor</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/trezor-suite" target="_blank" rel="noopener noreferrer">Suite</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/support" target="_blank" rel="noopener noreferrer">Support</a></span><span class="navbar-nav ml-md-auto"><a class="nav-link" href="/sendtx">Send Transaction</a></span><span class="navbar-nav ml-md-auto d-lg-flex d-none"><a class="nav-link" href="https://trezor.io/compare" target="_blank" rel="noopener noreferrer">Don't have a Trezor? Get one!</a></span></nav></div></footer></body></html>`,
			},
		},
		{
			name:        "explorerSearch block height",
			r:           newGetRequest(ts.URL + "/search?q=225494"),
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body: []string{
				`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1.0,shrink-to-fit=no"><link href="https://cdn.jsdelivr.net/npm/bootstrap@5.2.2/dist/css/bootstrap.min.css" rel="stylesheet" integrity="sha384-Zenh87qX5JnK2Jl0vWa8Ck2rdkQ2Bzep5IDxbcnCeuOxjzrPF/et3URy9Bv1WTRi" crossorigin="anonymous"><link rel="stylesheet" href="/static/css/main.min.3.css"><script>var hasSecondary=false;</script><script src="https://cdn.jsdelivr.net/npm/bootstrap@5.2.2/dist/js/bootstrap.bundle.min.js" integrity="sha384-OERcA2EqjJCMA+/3y+gxIOqMEjwtxJY7qPCqsdltbNJuaOe923+mo//f6V8Qbsw3" crossorigin="anonymous"></script><script src="/static/js/main.min.3.js"></script><meta http-equiv="X-UA-Compatible" content="IE=edge"><meta name="description" content="Trezor Fake Coin Explorer"><title>Trezor Fake Coin Explorer</title></head><body><header id="header"><nav class="navbar navbar-expand-lg"><div class="container"><a class="navbar-brand" href="/" title="Home"><span class="trezor-logo"></span><span style="padding-left: 140px;">Fake Coin Explorer</span></a><button class="navbar-toggler" type="button" data-bs-toggle="collapse" data-bs-target="#navbarSupportedContent" aria-controls="navbarSupportedContent" aria-expanded="false" aria-label="Toggle navigation"><span class="navbar-toggler-icon"></span></button><div class="collapse navbar-collapse" id="navbarSupportedContent"><ul class="navbar-nav m-md-auto"><li class="nav-item pe-xl-4"><a href="/blocks" class="nav-link">Blocks</a></li><li class="nav-item"><a href="/" class="nav-link">Status</a></li></ul><span class="navbar-form"><form class="d-flex" id="search" action="/search" method="get"><input name="q" type="text" class="form-control form-control-lg" placeholder="Search for block, transaction, address or xpub" focus="true"><button class="btn" type="submit"><span class="search-icon"></span></button></form></span></div></div></nav></header><main id="wrap"><div class="container"><div class="row"><h1 class="col-sm-6">Block</h1><nav class="col-sm-6 paging justify-content-end pb-1"><a class="btn btn-paging" href="/block/0000000076fbbed90fd75b0e18856aa35baa984e9c9d444cf746ad85e94e2997">Previous Block</a><span class="btn btn-paging ms-2">Next Block</span></nav></div><div class="row pt-3"><h5 class="col-lg-3 h-data"><span class="ellipsis copyable">225<span class="ns">494</span></span></h5><h5 class="col-lg-9 justify-content-end d-flex h-data"><span class="copyable ellipsis">00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6</span></h5></div><div class="row"><div class="col-md-6"><table class="table data-table info-table"><tbody><tr><td>Transactions</td><td>4</td></tr><tr><td>Height</td><td>225<span class="ns">494</span></td></tr><tr><td>Confirmations</td><td>1</td></tr><tr><td>Timestamp</td><td><span tt="2018-03-21 01:27:58">1639 days 11 hours ago</span></td></tr><tr><td>Size (bytes)</td><td>2<span class="ns">345</span><span class="ns">678</span></td></tr></tbody></table></div><div class="col-md-6"><table class="table data-table info-table"><tbody><tr><td>Version</td><td class="ellipsis"></td></tr><tr><td>Merkle&nbsp;Root</td><td style="max-width: 280px;" class="ellipsis"></td></tr><tr><td>Nonce</td><td style="max-width: 280px;" class="ellipsis"></td></tr><tr><td>Bits</td><td class="ellipsis"></td></tr><tr><td>Difficulty</td><td class="ellipsis"></td></tr></tbody></table></div></div><div class="row pt-3 pb-1"><h3 class="col-md-6 align-self-center">Transactions</h3><div class="col-md-6"></div></div><div><div class="tx-detail"><div class="row head"><div class="col-xs-7 col-md-8"><a href="/tx/7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25" class="ellipsis copyable txid">7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25</a></div><div class="col-xs-5 col-md-4 text-end">mined <span class="txvalue ms-1"><span tt="2018-03-21 01:27:58">1639 days 11 hours ago</span></span></div></div><div class="row body"><div class="col-md-5"><div class="row tx-in"><div class="col-12"><span class="ellipsis copyable"><a href="/address/mv9uLThosiEnGRbVPS7Vhyw6VssbVRsiAw">mv9uLThosiEnGRbVPS7Vhyw6VssbVRsiAw</a></span><span class="amt tx-amt copyable" cc="12345.67890123 FAKE"><span class="prim-amt">12<span class="nc">345</span>.<span class="amt-dec">67<span class="ns">890</span><span class="ns">123</span></span> FAKE</span></span></div><div class="col-12"><span class="ellipsis copyable"><a href="/address/mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz">mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz</a></span><span class="amt tx-amt copyable" cc="0.00012345 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">012</span><span class="ns">345</span></span> FAKE</span></span></div></div></div><div class="col-md-1 col-xs-12 text-center">&nbsp;<span class="octicon"></span></div><div class="col-md-6"><div class="row tx-out"><div class="col-12"><span class="ellipsis copyable"><a href="/address/mzB8cYrfRwFRFAGTDzV8LkUQy5BQicxGhX">mzB8cYrfRwFRFAGTDzV8LkUQy5BQicxGhX</a></span><span class="tx-amt"><span class="amt copyable" cc="3172.83951061 FAKE"><span class="prim-amt">3<span class="nc">172</span>.<span class="amt-dec">83<span class="ns">951</span><span class="ns">061</span></span> FAKE</span></span><a class="spent" href="/spending/7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25/0" tt="Spent">→</a></span></td></div><div class="col-12"><span class="ellipsis copyable"><a href="/address/mtR97eM2HPWVM6c8FGLGcukgaHHQv7THoL">mtR97eM2HPWVM6c8FGLGcukgaHHQv7THoL</a></span><span class="tx-amt"><span class="amt copyable" cc="9172.83951061 FAKE"><span class="prim-amt">9<span class="nc">172</span>.<span class="amt-dec">83<span class="ns">951</span><span class="ns">061</span></span> FAKE</span></span><span class="unspent" tt="Unspent">×</span></span></td></div><div class="col-12"><span class="ellipsis copyable">OP_RETURN 2020f1686f6a20</span><span class="tx-amt"><span class="amt copyable" cc="0 FAKE"><span class="prim-amt">0 FAKE</span></span><span class="unspent" tt="Unspent">×</span></span></td></div></div></div></div><div class="row footer"><div class="col-sm-12 col-md-4">Fee <span class="amt txvalue copyable ms-3" cc="0.00000346 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">000</span><span class="ns">346</span></span> FAKE</span></span></div><div class="col-sm-12 col-md-8 text-end"><span class="me-4"><span class="txvalue">1</span> confirmations</span><span class="amt txvalue copyable" cc="12345.67902122 FAKE"><span class="prim-amt">12<span class="nc">345</span>.<span class="amt-dec">67<span class="ns">902</span><span class="ns">122</span></span> FAKE</span></span></div></div></div><div class="tx-detail"><div class="row head"><div class="col-xs-7 col-md-8"><a href="/tx/3d90d15ed026dc45e19ffb52875ed18fa9e8012ad123d7f7212176e2b0ebdb71" class="ellipsis copyable txid">3d90d15ed026dc45e19ffb52875ed18fa9e8012ad123d7f7212176e2b0ebdb71</a></div><div class="col-xs-5 col-md-4 text-end">mined <span class="txvalue ms-1"><span tt="2018-03-21 01:27:58">1639 days 11 hours ago</span></span></div></div><div class="row body"><div class="col-md-5"><div class="row tx-in"><div class="col-12"><span class="ellipsis copyable"><a href="/address/mzB8cYrfRwFRFAGTDzV8LkUQy5BQicxGhX">mzB8cYrfRwFRFAGTDzV8LkUQy5BQicxGhX</a></span><span class="amt tx-amt copyable" cc="3172.83951061 FAKE"><span class="prim-amt">3<span class="nc">172</span>.<span class="amt-dec">83<span class="ns">951</span><span class="ns">061</span></span> FAKE</span></span></div><div class="col-12"><span class="ellipsis copyable"><a href="/address/2MzmAKayJmja784jyHvRUW1bXPget1csRRG">2MzmAKayJmja784jyHvRUW1bXPget1csRRG</a></span><span class="amt tx-amt copyable" cc="0.00000001 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">000</span><span class="ns">001</span></span> FAKE</span></span></div></div></div><div class="col-md-1 col-xs-12 text-center">&nbsp;<span class="octicon"></span></div><div class="col-md-6"><div class="row tx-out"><div class="col-12"><span class="ellipsis copyable"><a href="/address/2N6utyMZfPNUb1Bk8oz7p2JqJrXkq83gegu">2N6utyMZfPNUb1Bk8oz7p2JqJrXkq83gegu</a></span><span class="tx-amt"><span class="amt copyable" cc="1186.419755 FAKE"><span class="prim-amt">1<span class="nc">186</span>.<span class="amt-dec">41<span class="ns">975</span><span class="ns">500</span></span> FAKE</span></span><span class="unspent" tt="Unspent">×</span></span></td></div><div class="col-12"><span class="ellipsis copyable"><a href="/address/mmJx9Y8ayz9h14yd9fgCW1bUKoEpkBAquP">mmJx9Y8ayz9h14yd9fgCW1bUKoEpkBAquP</a></span><span class="tx-amt"><span class="amt copyable" cc="1986.419755 FAKE"><span class="prim-amt">1<span class="nc">986</span>.<span class="amt-dec">41<span class="ns">975</span><span class="ns">500</span></span> FAKE</span></span><span class="unspent" tt="Unspent">×</span></span></td></div></div></div></div><div class="row footer"><div class="col-sm-12 col-md-4">Fee <span class="amt txvalue copyable ms-3" cc="0.00000062 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">000</span><span class="ns">062</span></span> FAKE</span></span></div><div class="col-sm-12 col-md-8 text-end"><span class="me-4"><span class="txvalue">1</span> confirmations</span><span class="amt txvalue copyable" cc="3172.83951 FAKE"><span class="prim-amt">3<span class="nc">172</span>.<span class="amt-dec">83<span class="ns">951</span><span class="ns">000</span></span> FAKE</span></span></div></div></div><div class="tx-detail"><div class="row head"><div class="col-xs-7 col-md-8"><a href="/tx/05e2e48aeabdd9b75def7b48d756ba304713c2aba7b522bf9dbc893fc4231b07" class="ellipsis copyable txid">05e2e48aeabdd9b75def7b48d756ba304713c2aba7b522bf9dbc893fc4231b07</a></div><div class="col-xs-5 col-md-4 text-end">mined <span class="txvalue ms-1"><span tt="2018-03-21 01:27:58">1639 days 11 hours ago</span></span></div></div><div class="row body"><div class="col-md-5"><div class="row tx-in"><div class="col-12"><span class="ellipsis copyable"><a href="/address/2NEVv9LJmAnY99W1pFoc5UJjVdypBqdnvu1">2NEVv9LJmAnY99W1pFoc5UJjVdypBqdnvu1</a></span><span class="amt tx-amt copyable" cc="0.00009876 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">009</span><span class="ns">876</span></span> FAKE</span></span></div></div></div><div class="col-md-1 col-xs-12 text-center">&nbsp;<span class="octicon"></span></div><div class="col-md-6"><div class="row tx-out"><div class="col-12"><span class="ellipsis copyable"><a href="/address/2NEVv9LJmAnY99W1pFoc5UJjVdypBqdnvu1">2NEVv9LJmAnY99W1pFoc5UJjVdypBqdnvu1</a></span><span class="tx-amt"><span class="amt copyable" cc="0.00009 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">009</span><span class="ns">000</span></span> FAKE</span></span><span class="unspent" tt="Unspent">×</span></span></td></div></div></div></div><div class="row footer"><div class="col-sm-12 col-md-4">Fee <span class="amt txvalue copyable ms-3" cc="0.00000876 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">000</span><span class="ns">876</span></span> FAKE</span></span></div><div class="col-sm-12 col-md-8 text-end"><span class="me-4"><span class="txvalue">1</span> confirmations</span><span class="amt txvalue copyable" cc="0.00009 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">009</span><span class="ns">000</span></span> FAKE</span></span></div></div></div><div class="tx-detail"><div class="row head"><div class="col-xs-7 col-md-8"><a href="/tx/fdd824a780cbb718eeb766eb05d83fdefc793a27082cd5e67f856d69798cf7db" class="ellipsis copyable txid">fdd824a780cbb718eeb766eb05d83fdefc793a27082cd5e67f856d69798cf7db</a></div><div class="col-xs-5 col-md-4 text-end">mined <span class="txvalue ms-1"><span tt="2018-03-21 01:27:58">1639 days 11 hours ago</span></span></div></div><div class="row body"><div class="col-md-5"><div class="row tx-in"><div class="col-12">No Inputs (Newly Generated Coins)</div></div></div><div class="col-md-1 col-xs-12 text-center">&nbsp;<span class="octicon"></span></div><div class="col-md-6"><div class="row tx-out"><div class="col-12"><span class="ellipsis copyable"><a href="/address/mzVznVsCHkVHX9UN8WPFASWUUHtxnNn4Jj">mzVznVsCHkVHX9UN8WPFASWUUHtxnNn4Jj</a></span><span class="tx-amt"><span class="amt copyable" cc="13.60030331 FAKE"><span class="prim-amt">13.<span class="amt-dec">60<span class="ns">030</span><span class="ns">331</span></span> FAKE</span></span><span class="unspent" tt="Unspent">×</span></span></td></div><div class="col-12">Unparsed address<span class="tx-amt"><span class="amt copyable" cc="0 FAKE"><span class="prim-amt">0 FAKE</span></span><span class="unspent" tt="Unspent">×</span></span></td></div></div></div></div><div class="row footer"><div class="col-sm-12 col-md-4">Fee <span class="amt txvalue copyable ms-3" cc="0 FAKE"><span class="prim-amt">0 FAKE</span></span></div><div class="col-sm-12 col-md-8 text-end"><span class="me-4"><span class="txvalue">1</span> confirmations</span><span class="amt txvalue copyable" cc="13.60030331 FAKE"><span class="prim-amt">13.<span class="amt-dec">60<span class="ns">030</span><span class="ns">331</span></span> FAKE</span></span></div></div></div></div></div></main><footer id="footer"><div class="container"><nav class="navbar navbar-dark"><span class="navbar-nav"><a class="nav-link" href="https://satoshilabs.com/" target="_blank" rel="noopener noreferrer">Created by SatoshiLabs</a></span><span class="navbar-nav ml-md-auto"><a class="nav-link" href="https://trezor.io/terms-of-use" target="_blank" rel="noopener noreferrer">Terms of Use</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/" target="_blank" rel="noopener noreferrer">Trezor</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/trezor-suite" target="_blank" rel="noopener noreferrer">Suite</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/support" target="_blank" rel="noopener noreferrer">Support</a></span><span class="navbar-nav ml-md-auto"><a class="nav-link" href="/sendtx">Send Transaction</a></span><span class="navbar-nav ml-md-auto d-lg-flex d-none"><a class="nav-link" href="https://trezor.io/compare" target="_blank" rel="noopener noreferrer">Don't have a Trezor? Get one!</a></span></nav></div></footer></body></html>`,
			},
		},
		{
			name:        "explorerSearch block hash",
			r:           newGetRequest(ts.URL + "/search?q=00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6"),
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body: []string{
				`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1.0,shrink-to-fit=no"><link href="https://cdn.jsdelivr.net/npm/bootstrap@5.2.2/dist/css/bootstrap.min.css" rel="stylesheet" integrity="sha384-Zenh87qX5JnK2Jl0vWa8Ck2rdkQ2Bzep5IDxbcnCeuOxjzrPF/et3URy9Bv1WTRi" crossorigin="anonymous"><link rel="stylesheet" href="/static/css/main.min.3.css"><script>var hasSecondary=false;</script><script src="https://cdn.jsdelivr.net/npm/bootstrap@5.2.2/dist/js/bootstrap.bundle.min.js" integrity="sha384-OERcA2EqjJCMA+/3y+gxIOqMEjwtxJY7qPCqsdltbNJuaOe923+mo//f6V8Qbsw3" crossorigin="anonymous"></script><script src="/static/js/main.min.3.js"></script><meta http-equiv="X-UA-Compatible" content="IE=edge"><meta name="description" content="Trezor Fake Coin Explorer"><title>Trezor Fake Coin Explorer</title></head><body><header id="header"><nav class="navbar navbar-expand-lg"><div class="container"><a class="navbar-brand" href="/" title="Home"><span class="trezor-logo"></span><span style="padding-left: 140px;">Fake Coin Explorer</span></a><button class="navbar-toggler" type="button" data-bs-toggle="collapse" data-bs-target="#navbarSupportedContent" aria-controls="navbarSupportedContent" aria-expanded="false" aria-label="Toggle navigation"><span class="navbar-toggler-icon"></span></button><div class="collapse navbar-collapse" id="navbarSupportedContent"><ul class="navbar-nav m-md-auto"><li class="nav-item pe-xl-4"><a href="/blocks" class="nav-link">Blocks</a></li><li class="nav-item"><a href="/" class="nav-link">Status</a></li></ul><span class="navbar-form"><form class="d-flex" id="search" action="/search" method="get"><input name="q" type="text" class="form-control form-control-lg" placeholder="Search for block, transaction, address or xpub" focus="true"><button class="btn" type="submit"><span class="search-icon"></span></button></form></span></div></div></nav></header><main id="wrap"><div class="container"><div class="row"><h1 class="col-sm-6">Block</h1><nav class="col-sm-6 paging justify-content-end pb-1"><a class="btn btn-paging" href="/block/0000000076fbbed90fd75b0e18856aa35baa984e9c9d444cf746ad85e94e2997">Previous Block</a><span class="btn btn-paging ms-2">Next Block</span></nav></div><div class="row pt-3"><h5 class="col-lg-3 h-data"><span class="ellipsis copyable">225<span class="ns">494</span></span></h5><h5 class="col-lg-9 justify-content-end d-flex h-data"><span class="copyable ellipsis">00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6</span></h5></div><div class="row"><div class="col-md-6"><table class="table data-table info-table"><tbody><tr><td>Transactions</td><td>4</td></tr><tr><td>Height</td><td>225<span class="ns">494</span></td></tr><tr><td>Confirmations</td><td>1</td></tr><tr><td>Timestamp</td><td><span tt="2018-03-21 01:27:58">1639 days 11 hours ago</span></td></tr><tr><td>Size (bytes)</td><td>2<span class="ns">345</span><span class="ns">678</span></td></tr></tbody></table></div><div class="col-md-6"><table class="table data-table info-table"><tbody><tr><td>Version</td><td class="ellipsis"></td></tr><tr><td>Merkle&nbsp;Root</td><td style="max-width: 280px;" class="ellipsis"></td></tr><tr><td>Nonce</td><td style="max-width: 280px;" class="ellipsis"></td></tr><tr><td>Bits</td><td class="ellipsis"></td></tr><tr><td>Difficulty</td><td class="ellipsis"></td></tr></tbody></table></div></div><div class="row pt-3 pb-1"><h3 class="col-md-6 align-self-center">Transactions</h3><div class="col-md-6"></div></div><div><div class="tx-detail"><div class="row head"><div class="col-xs-7 col-md-8"><a href="/tx/7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25" class="ellipsis copyable txid">7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25</a></div><div class="col-xs-5 col-md-4 text-end">mined <span class="txvalue ms-1"><span tt="2018-03-21 01:27:58">1639 days 11 hours ago</span></span></div></div><div class="row body"><div class="col-md-5"><div class="row tx-in"><div class="col-12"><span class="ellipsis copyable"><a href="/address/mv9uLThosiEnGRbVPS7Vhyw6VssbVRsiAw">mv9uLThosiEnGRbVPS7Vhyw6VssbVRsiAw</a></span><span class="amt tx-amt copyable" cc="12345.67890123 FAKE"><span class="prim-amt">12<span class="nc">345</span>.<span class="amt-dec">67<span class="ns">890</span><span class="ns">123</span></span> FAKE</span></span></div><div class="col-12"><span class="ellipsis copyable"><a href="/address/mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz">mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz</a></span><span class="amt tx-amt copyable" cc="0.00012345 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">012</span><span class="ns">345</span></span> FAKE</span></span></div></div></div><div class="col-md-1 col-xs-12 text-center">&nbsp;<span class="octicon"></span></div><div class="col-md-6"><div class="row tx-out"><div class="col-12"><span class="ellipsis copyable"><a href="/address/mzB8cYrfRwFRFAGTDzV8LkUQy5BQicxGhX">mzB8cYrfRwFRFAGTDzV8LkUQy5BQicxGhX</a></span><span class="tx-amt"><span class="amt copyable" cc="3172.83951061 FAKE"><span class="prim-amt">3<span class="nc">172</span>.<span class="amt-dec">83<span class="ns">951</span><span class="ns">061</span></span> FAKE</span></span><a class="spent" href="/spending/7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25/0" tt="Spent">→</a></span></td></div><div class="col-12"><span class="ellipsis copyable"><a href="/address/mtR97eM2HPWVM6c8FGLGcukgaHHQv7THoL">mtR97eM2HPWVM6c8FGLGcukgaHHQv7THoL</a></span><span class="tx-amt"><span class="amt copyable" cc="9172.83951061 FAKE"><span class="prim-amt">9<span class="nc">172</span>.<span class="amt-dec">83<span class="ns">951</span><span class="ns">061</span></span> FAKE</span></span><span class="unspent" tt="Unspent">×</span></span></td></div><div class="col-12"><span class="ellipsis copyable">OP_RETURN 2020f1686f6a20</span><span class="tx-amt"><span class="amt copyable" cc="0 FAKE"><span class="prim-amt">0 FAKE</span></span><span class="unspent" tt="Unspent">×</span></span></td></div></div></div></div><div class="row footer"><div class="col-sm-12 col-md-4">Fee <span class="amt txvalue copyable ms-3" cc="0.00000346 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">000</span><span class="ns">346</span></span> FAKE</span></span></div><div class="col-sm-12 col-md-8 text-end"><span class="me-4"><span class="txvalue">1</span> confirmations</span><span class="amt txvalue copyable" cc="12345.67902122 FAKE"><span class="prim-amt">12<span class="nc">345</span>.<span class="amt-dec">67<span class="ns">902</span><span class="ns">122</span></span> FAKE</span></span></div></div></div><div class="tx-detail"><div class="row head"><div class="col-xs-7 col-md-8"><a href="/tx/3d90d15ed026dc45e19ffb52875ed18fa9e8012ad123d7f7212176e2b0ebdb71" class="ellipsis copyable txid">3d90d15ed026dc45e19ffb52875ed18fa9e8012ad123d7f7212176e2b0ebdb71</a></div><div class="col-xs-5 col-md-4 text-end">mined <span class="txvalue ms-1"><span tt="2018-03-21 01:27:58">1639 days 11 hours ago</span></span></div></div><div class="row body"><div class="col-md-5"><div class="row tx-in"><div class="col-12"><span class="ellipsis copyable"><a href="/address/mzB8cYrfRwFRFAGTDzV8LkUQy5BQicxGhX">mzB8cYrfRwFRFAGTDzV8LkUQy5BQicxGhX</a></span><span class="amt tx-amt copyable" cc="3172.83951061 FAKE"><span class="prim-amt">3<span class="nc">172</span>.<span class="amt-dec">83<span class="ns">951</span><span class="ns">061</span></span> FAKE</span></span></div><div class="col-12"><span class="ellipsis copyable"><a href="/address/2MzmAKayJmja784jyHvRUW1bXPget1csRRG">2MzmAKayJmja784jyHvRUW1bXPget1csRRG</a></span><span class="amt tx-amt copyable" cc="0.00000001 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">000</span><span class="ns">001</span></span> FAKE</span></span></div></div></div><div class="col-md-1 col-xs-12 text-center">&nbsp;<span class="octicon"></span></div><div class="col-md-6"><div class="row tx-out"><div class="col-12"><span class="ellipsis copyable"><a href="/address/2N6utyMZfPNUb1Bk8oz7p2JqJrXkq83gegu">2N6utyMZfPNUb1Bk8oz7p2JqJrXkq83gegu</a></span><span class="tx-amt"><span class="amt copyable" cc="1186.419755 FAKE"><span class="prim-amt">1<span class="nc">186</span>.<span class="amt-dec">41<span class="ns">975</span><span class="ns">500</span></span> FAKE</span></span><span class="unspent" tt="Unspent">×</span></span></td></div><div class="col-12"><span class="ellipsis copyable"><a href="/address/mmJx9Y8ayz9h14yd9fgCW1bUKoEpkBAquP">mmJx9Y8ayz9h14yd9fgCW1bUKoEpkBAquP</a></span><span class="tx-amt"><span class="amt copyable" cc="1986.419755 FAKE"><span class="prim-amt">1<span class="nc">986</span>.<span class="amt-dec">41<span class="ns">975</span><span class="ns">500</span></span> FAKE</span></span><span class="unspent" tt="Unspent">×</span></span></td></div></div></div></div><div class="row footer"><div class="col-sm-12 col-md-4">Fee <span class="amt txvalue copyable ms-3" cc="0.00000062 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">000</span><span class="ns">062</span></span> FAKE</span></span></div><div class="col-sm-12 col-md-8 text-end"><span class="me-4"><span class="txvalue">1</span> confirmations</span><span class="amt txvalue copyable" cc="3172.83951 FAKE"><span class="prim-amt">3<span class="nc">172</span>.<span class="amt-dec">83<span class="ns">951</span><span class="ns">000</span></span> FAKE</span></span></div></div></div><div class="tx-detail"><div class="row head"><div class="col-xs-7 col-md-8"><a href="/tx/05e2e48aeabdd9b75def7b48d756ba304713c2aba7b522bf9dbc893fc4231b07" class="ellipsis copyable txid">05e2e48aeabdd9b75def7b48d756ba304713c2aba7b522bf9dbc893fc4231b07</a></div><div class="col-xs-5 col-md-4 text-end">mined <span class="txvalue ms-1"><span tt="2018-03-21 01:27:58">1639 days 11 hours ago</span></span></div></div><div class="row body"><div class="col-md-5"><div class="row tx-in"><div class="col-12"><span class="ellipsis copyable"><a href="/address/2NEVv9LJmAnY99W1pFoc5UJjVdypBqdnvu1">2NEVv9LJmAnY99W1pFoc5UJjVdypBqdnvu1</a></span><span class="amt tx-amt copyable" cc="0.00009876 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">009</span><span class="ns">876</span></span> FAKE</span></span></div></div></div><div class="col-md-1 col-xs-12 text-center">&nbsp;<span class="octicon"></span></div><div class="col-md-6"><div class="row tx-out"><div class="col-12"><span class="ellipsis copyable"><a href="/address/2NEVv9LJmAnY99W1pFoc5UJjVdypBqdnvu1">2NEVv9LJmAnY99W1pFoc5UJjVdypBqdnvu1</a></span><span class="tx-amt"><span class="amt copyable" cc="0.00009 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">009</span><span class="ns">000</span></span> FAKE</span></span><span class="unspent" tt="Unspent">×</span></span></td></div></div></div></div><div class="row footer"><div class="col-sm-12 col-md-4">Fee <span class="amt txvalue copyable ms-3" cc="0.00000876 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">000</span><span class="ns">876</span></span> FAKE</span></span></div><div class="col-sm-12 col-md-8 text-end"><span class="me-4"><span class="txvalue">1</span> confirmations</span><span class="amt txvalue copyable" cc="0.00009 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">009</span><span class="ns">000</span></span> FAKE</span></span></div></div></div><div class="tx-detail"><div class="row head"><div class="col-xs-7 col-md-8"><a href="/tx/fdd824a780cbb718eeb766eb05d83fdefc793a27082cd5e67f856d69798cf7db" class="ellipsis copyable txid">fdd824a780cbb718eeb766eb05d83fdefc793a27082cd5e67f856d69798cf7db</a></div><div class="col-xs-5 col-md-4 text-end">mined <span class="txvalue ms-1"><span tt="2018-03-21 01:27:58">1639 days 11 hours ago</span></span></div></div><div class="row body"><div class="col-md-5"><div class="row tx-in"><div class="col-12">No Inputs (Newly Generated Coins)</div></div></div><div class="col-md-1 col-xs-12 text-center">&nbsp;<span class="octicon"></span></div><div class="col-md-6"><div class="row tx-out"><div class="col-12"><span class="ellipsis copyable"><a href="/address/mzVznVsCHkVHX9UN8WPFASWUUHtxnNn4Jj">mzVznVsCHkVHX9UN8WPFASWUUHtxnNn4Jj</a></span><span class="tx-amt"><span class="amt copyable" cc="13.60030331 FAKE"><span class="prim-amt">13.<span class="amt-dec">60<span class="ns">030</span><span class="ns">331</span></span> FAKE</span></span><span class="unspent" tt="Unspent">×</span></span></td></div><div class="col-12">Unparsed address<span class="tx-amt"><span class="amt copyable" cc="0 FAKE"><span class="prim-amt">0 FAKE</span></span><span class="unspent" tt="Unspent">×</span></span></td></div></div></div></div><div class="row footer"><div class="col-sm-12 col-md-4">Fee <span class="amt txvalue copyable ms-3" cc="0 FAKE"><span class="prim-amt">0 FAKE</span></span></div><div class="col-sm-12 col-md-8 text-end"><span class="me-4"><span class="txvalue">1</span> confirmations</span><span class="amt txvalue copyable" cc="13.60030331 FAKE"><span class="prim-amt">13.<span class="amt-dec">60<span class="ns">030</span><span class="ns">331</span></span> FAKE</span></span></div></div></div></div></div></main><footer id="footer"><div class="container"><nav class="navbar navbar-dark"><span class="navbar-nav"><a class="nav-link" href="https://satoshilabs.com/" target="_blank" rel="noopener noreferrer">Created by SatoshiLabs</a></span><span class="navbar-nav ml-md-auto"><a class="nav-link" href="https://trezor.io/terms-of-use" target="_blank" rel="noopener noreferrer">Terms of Use</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/" target="_blank" rel="noopener noreferrer">Trezor</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/trezor-suite" target="_blank" rel="noopener noreferrer">Suite</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/support" target="_blank" rel="noopener noreferrer">Support</a></span><span class="navbar-nav ml-md-auto"><a class="nav-link" href="/sendtx">Send Transaction</a></span><span class="navbar-nav ml-md-auto d-lg-flex d-none"><a class="nav-link" href="https://trezor.io/compare" target="_blank" rel="noopener noreferrer">Don't have a Trezor? Get one!</a></span></nav></div></footer></body></html>`,
			},
		},
		{
			name:        "explorerSearch tx",
			r:           newGetRequest(ts.URL + "/search?q=fdd824a780cbb718eeb766eb05d83fdefc793a27082cd5e67f856d69798cf7db"),
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body: []string{
				`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1.0,shrink-to-fit=no"><link href="https://cdn.jsdelivr.net/npm/bootstrap@5.2.2/dist/css/bootstrap.min.css" rel="stylesheet" integrity="sha384-Zenh87qX5JnK2Jl0vWa8Ck2rdkQ2Bzep5IDxbcnCeuOxjzrPF/et3URy9Bv1WTRi" crossorigin="anonymous"><link rel="stylesheet" href="/static/css/main.min.3.css"><script>var hasSecondary=false;</script><script src="https://cdn.jsdelivr.net/npm/bootstrap@5.2.2/dist/js/bootstrap.bundle.min.js" integrity="sha384-OERcA2EqjJCMA+/3y+gxIOqMEjwtxJY7qPCqsdltbNJuaOe923+mo//f6V8Qbsw3" crossorigin="anonymous"></script><script src="/static/js/main.min.3.js"></script><meta http-equiv="X-UA-Compatible" content="IE=edge"><meta name="description" content="Trezor Fake Coin Explorer"><title>Trezor Fake Coin Explorer</title></head><body><header id="header"><nav class="navbar navbar-expand-lg"><div class="container"><a class="navbar-brand" href="/" title="Home"><span class="trezor-logo"></span><span style="padding-left: 140px;">Fake Coin Explorer</span></a><button class="navbar-toggler" type="button" data-bs-toggle="collapse" data-bs-target="#navbarSupportedContent" aria-controls="navbarSupportedContent" aria-expanded="false" aria-label="Toggle navigation"><span class="navbar-toggler-icon"></span></button><div class="collapse navbar-collapse" id="navbarSupportedContent"><ul class="navbar-nav m-md-auto"><li class="nav-item pe-xl-4"><a href="/blocks" class="nav-link">Blocks</a></li><li class="nav-item"><a href="/" class="nav-link">Status</a></li></ul><span class="navbar-form"><form class="d-flex" id="search" action="/search" method="get"><input name="q" type="text" class="form-control form-control-lg" placeholder="Search for block, transaction, address or xpub" focus="true"><button class="btn" type="submit"><span class="search-icon"></span></button></form></span></div></div></nav></header><main id="wrap"><div class="container"><div class="row"><h1 class="col-12">Transaction</h1></div><div class="row pt-3"><h5 class="col-12 d-flex h-data"><span class="ellipsis copyable">fdd824a780cbb718eeb766eb05d83fdefc793a27082cd5e67f856d69798cf7db</span></h5></div><table class="table data-table info-table"><tbody><tr><td>Mined Time</td><td><span tt="2018-03-21 01:27:58">1639 days 11 hours ago</span></td></tr><tr><td>In Block</td><td class="ellipsis" style="max-width: 280px">00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6</td></tr><tr><td>In Block Height</td><td><a href="/block/225494">225<span class="ns">494</span></a></td></tr><tr><td>Total Input</td><td><span class="amt copyable" cc="0 FAKE"><span class="prim-amt">0 FAKE</span></span></td></tr><tr><td>Total Output</td><td><span class="amt copyable" cc="13.60030331 FAKE"><span class="prim-amt">13.<span class="amt-dec">60<span class="ns">030</span><span class="ns">331</span></span> FAKE</span></span></td></tr><tr><td>Fees</td><td><span class="amt copyable" cc="0 FAKE"><span class="prim-amt">0 FAKE</span></span></td></tr></tbody></table><div class="pt-1"><div class="tx-detail"><div class="row head"><div class="col-xs-7 col-md-8"><a href="/tx/fdd824a780cbb718eeb766eb05d83fdefc793a27082cd5e67f856d69798cf7db" class="ellipsis copyable txid">fdd824a780cbb718eeb766eb05d83fdefc793a27082cd5e67f856d69798cf7db</a></div><div class="col-xs-5 col-md-4 text-end">mined <span class="txvalue ms-1"><span tt="2018-03-21 01:27:58">1639 days 11 hours ago</span></span></div></div><div class="row body"><div class="col-md-5"><div class="row tx-in"><div class="col-12">No Inputs (Newly Generated Coins)</div></div></div><div class="col-md-1 col-xs-12 text-center">&nbsp;<span class="octicon"></span></div><div class="col-md-6"><div class="row tx-out"><div class="col-12"><span class="ellipsis copyable"><a href="/address/mzVznVsCHkVHX9UN8WPFASWUUHtxnNn4Jj">mzVznVsCHkVHX9UN8WPFASWUUHtxnNn4Jj</a></span><span class="tx-amt"><span class="amt copyable" cc="13.60030331 FAKE"><span class="prim-amt">13.<span class="amt-dec">60<span class="ns">030</span><span class="ns">331</span></span> FAKE</span></span><span class="unspent" tt="Unspent">×</span></span></td></div><div class="col-12">Unparsed address<span class="tx-amt"><span class="amt copyable" cc="0 FAKE"><span class="prim-amt">0 FAKE</span></span><span class="unspent" tt="Unspent">×</span></span></td></div></div></div></div><div class="row footer"><div class="col-sm-12 col-md-4">Fee <span class="amt txvalue copyable ms-3" cc="0 FAKE"><span class="prim-amt">0 FAKE</span></span></div><div class="col-sm-12 col-md-8 text-end"><span class="me-4"><span class="txvalue">1</span> confirmations</span><span class="amt txvalue copyable" cc="13.60030331 FAKE"><span class="prim-amt">13.<span class="amt-dec">60<span class="ns">030</span><span class="ns">331</span></span> FAKE</span></span></div></div></div></div><div class="pt-4"><h5>Raw Transaction</h5><div class="json"><pre id="raw"></pre></div><script type="text/javascript">var raw = {"hex":"","txid":"fdd824a780cbb718eeb766eb05d83fdefc793a27082cd5e67f856d69798cf7db","version":0,"locktime":0,"vin":[{"coinbase":"03bf1e1504aede765b726567696f6e312f50726f6a65637420425443506f6f6c2f01000001bf7e000000000000","txid":"","vout":0,"scriptSig":{"hex":""},"sequence":0,"addresses":null}],"vout":[{"ValueSat":1360030331,"value":0,"n":0,"scriptPubKey":{"hex":"76a914d03c0d863d189b23b061a95ad32940b65837609f88ac","addresses":null}},{"ValueSat":0,"value":0,"n":1,"scriptPubKey":{"addresses":null}}],"confirmations":1,"time":1521595678,"blocktime":1521595678,"size":300};document.getElementById('raw').innerHTML = syntaxHighlight(raw);</script></div></div></main><footer id="footer"><div class="container"><nav class="navbar navbar-dark"><span class="navbar-nav"><a class="nav-link" href="https://satoshilabs.com/" target="_blank" rel="noopener noreferrer">Created by SatoshiLabs</a></span><span class="navbar-nav ml-md-auto"><a class="nav-link" href="https://trezor.io/terms-of-use" target="_blank" rel="noopener noreferrer">Terms of Use</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/" target="_blank" rel="noopener noreferrer">Trezor</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/trezor-suite" target="_blank" rel="noopener noreferrer">Suite</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/support" target="_blank" rel="noopener noreferrer">Support</a></span><span class="navbar-nav ml-md-auto"><a class="nav-link" href="/sendtx">Send Transaction</a></span><span class="navbar-nav ml-md-auto d-lg-flex d-none"><a class="nav-link" href="https://trezor.io/compare" target="_blank" rel="noopener noreferrer">Don't have a Trezor? Get one!</a></span></nav></div></footer></body></html>`,
			},
		},
		{
			name:        "explorerSearch address",
			r:           newGetRequest(ts.URL + "/search?q=mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz"),
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body: []string{
				`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1.0,shrink-to-fit=no"><link href="https://cdn.jsdelivr.net/npm/bootstrap@5.2.2/dist/css/bootstrap.min.css" rel="stylesheet" integrity="sha384-Zenh87qX5JnK2Jl0vWa8Ck2rdkQ2Bzep5IDxbcnCeuOxjzrPF/et3URy9Bv1WTRi" crossorigin="anonymous"><link rel="stylesheet" href="/static/css/main.min.3.css"><script>var hasSecondary=false;</script><script src="https://cdn.jsdelivr.net/npm/bootstrap@5.2.2/dist/js/bootstrap.bundle.min.js" integrity="sha384-OERcA2EqjJCMA+/3y+gxIOqMEjwtxJY7qPCqsdltbNJuaOe923+mo//f6V8Qbsw3" crossorigin="anonymous"></script><script src="/static/js/main.min.3.js"></script><meta http-equiv="X-UA-Compatible" content="IE=edge"><meta name="description" content="Trezor Fake Coin Explorer"><title>Trezor Fake Coin Explorer</title></head><body><header id="header"><nav class="navbar navbar-expand-lg"><div class="container"><a class="navbar-brand" href="/" title="Home"><span class="trezor-logo"></span><span style="padding-left: 140px;">Fake Coin Explorer</span></a><button class="navbar-toggler" type="button" data-bs-toggle="collapse" data-bs-target="#navbarSupportedContent" aria-controls="navbarSupportedContent" aria-expanded="false" aria-label="Toggle navigation"><span class="navbar-toggler-icon"></span></button><div class="collapse navbar-collapse" id="navbarSupportedContent"><ul class="navbar-nav m-md-auto"><li class="nav-item pe-xl-4"><a href="/blocks" class="nav-link">Blocks</a></li><li class="nav-item"><a href="/" class="nav-link">Status</a></li></ul><span class="navbar-form"><form class="d-flex" id="search" action="/search" method="get"><input name="q" type="text" class="form-control form-control-lg" placeholder="Search for block, transaction, address or xpub" focus="true"><button class="btn" type="submit"><span class="search-icon"></span></button></form></span></div></div></nav></header><main id="wrap"><div class="container"><div class="row g-0 ms-2 ms-lg-0"><div class="col-md-10 order-2 order-md-1"><h1>Address </h1><h5 class="col-12 d-flex h-data pb-2"><span class="ellipsis copyable">mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz</span></h5><h4 class="row"><div class="col-lg-6"><span class="copyable">0.<span class="amt-dec">000<span class="ns">123</span><span class="ns">45</span></span> FAKE</span></div></h4></div><div class="col-md-2 order-1 order-md-2 d-flex justify-content-center justify-content-md-end mb-3 mb-md-0"><div id="qrcode"></div><script type="text/javascript" src="/static/js/qrcode.min.js"></script><script type="text/javascript">new QRCode(document.getElementById("qrcode"), { text: "mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz", width: 120, height: 120 });</script></div></div><table class="table data-table info-table"><tbody><tr><td style="white-space: nowrap;"><h5>Confirmed</h5></td><td></td></tr><tr><td style="width: 25%;">Total Received</td><td><span class="amt copyable" cc="0.0002469 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">024</span><span class="ns">690</span></span> FAKE</span></span></td></tr><tr><td>Total Sent</td><td><span class="amt copyable" cc="0.00012345 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">012</span><span class="ns">345</span></span> FAKE</span></span></td></tr><tr><td>Final Balance</td><td><span class="amt copyable" cc="0.00012345 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">012</span><span class="ns">345</span></span> FAKE</span></span></td></tr><tr><td>No. Transactions</td><td>2</td></tr></tbody></table><div class="row pt-3 pb-1"><h3 class="col-sm-6 col-lg-3 m-0 align-self-center">Transactions</h3><div class="col-sm-6 col-lg-3 my-2 my-lg-0 align-self-center"><select  class="w-100" onchange="self.location='?filter='+options[selectedIndex].value"><option>All</option><option  value="inputs">Address on input side</option><option  value="outputs">Address on output side</option></select></div><div class="col-lg-6"></div></div><div><div class="tx-detail"><div class="row head"><div class="col-xs-7 col-md-8"><a href="/tx/7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25" class="ellipsis copyable txid">7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25</a></div><div class="col-xs-5 col-md-4 text-end">mined <span class="txvalue ms-1"><span tt="2018-03-21 01:27:58">1639 days 11 hours ago</span></span></div></div><div class="row body"><div class="col-md-5"><div class="row tx-in"><div class="col-12"><span class="ellipsis copyable"><a href="/address/mv9uLThosiEnGRbVPS7Vhyw6VssbVRsiAw">mv9uLThosiEnGRbVPS7Vhyw6VssbVRsiAw</a></span><span class="amt tx-amt copyable" cc="12345.67890123 FAKE"><span class="prim-amt">12<span class="nc">345</span>.<span class="amt-dec">67<span class="ns">890</span><span class="ns">123</span></span> FAKE</span></span></div><div class="col-12 tx-own"><span class="ellipsis copyable">mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz</span><span class="amt tx-amt copyable" cc="0.00012345 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">012</span><span class="ns">345</span></span> FAKE</span></span></div></div></div><div class="col-md-1 col-xs-12 text-center">&nbsp;<span class="octicon"></span></div><div class="col-md-6"><div class="row tx-out"><div class="col-12"><span class="ellipsis copyable"><a href="/address/mzB8cYrfRwFRFAGTDzV8LkUQy5BQicxGhX">mzB8cYrfRwFRFAGTDzV8LkUQy5BQicxGhX</a></span><span class="tx-amt"><span class="amt copyable" cc="3172.83951061 FAKE"><span class="prim-amt">3<span class="nc">172</span>.<span class="amt-dec">83<span class="ns">951</span><span class="ns">061</span></span> FAKE</span></span><a class="spent" href="/spending/7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25/0" tt="Spent">→</a></span></td></div><div class="col-12"><span class="ellipsis copyable"><a href="/address/mtR97eM2HPWVM6c8FGLGcukgaHHQv7THoL">mtR97eM2HPWVM6c8FGLGcukgaHHQv7THoL</a></span><span class="tx-amt"><span class="amt copyable" cc="9172.83951061 FAKE"><span class="prim-amt">9<span class="nc">172</span>.<span class="amt-dec">83<span class="ns">951</span><span class="ns">061</span></span> FAKE</span></span><span class="unspent" tt="Unspent">×</span></span></td></div><div class="col-12"><span class="ellipsis copyable">OP_RETURN 2020f1686f6a20</span><span class="tx-amt"><span class="amt copyable" cc="0 FAKE"><span class="prim-amt">0 FAKE</span></span><span class="unspent" tt="Unspent">×</span></span></td></div></div></div></div><div class="row footer"><div class="col-sm-12 col-md-4">Fee <span class="amt txvalue copyable ms-3" cc="0.00000346 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">000</span><span class="ns">346</span></span> FAKE</span></span></div><div class="col-sm-12 col-md-8 text-end"><span class="me-4"><span class="txvalue">1</span> confirmations</span><span class="amt txvalue copyable" cc="12345.67902122 FAKE"><span class="prim-amt">12<span class="nc">345</span>.<span class="amt-dec">67<span class="ns">902</span><span class="ns">122</span></span> FAKE</span></span></div></div></div><div class="tx-detail"><div class="row head"><div class="col-xs-7 col-md-8"><a href="/tx/00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa3840" class="ellipsis copyable txid">00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa3840</a></div><div class="col-xs-5 col-md-4 text-end">mined <span class="txvalue ms-1"><span tt="2018-03-20 03:03:46">1640 days 9 hours ago</span></span></div></div><div class="row body"><div class="col-md-5"><div class="row tx-in"><div class="col-12">No Inputs</div></div></div><div class="col-md-1 col-xs-12 text-center">&nbsp;<span class="octicon"></span></div><div class="col-md-6"><div class="row tx-out"><div class="col-12"><span class="ellipsis copyable"><a href="/address/mfcWp7DB6NuaZsExybTTXpVgWz559Np4Ti">mfcWp7DB6NuaZsExybTTXpVgWz559Np4Ti</a></span><span class="tx-amt"><span class="amt copyable" cc="1 FAKE"><span class="prim-amt">1.<span class="amt-dec">00<span class="ns">000</span><span class="ns">000</span></span> FAKE</span></span><span class="unspent" tt="Unspent">×</span></span></td></div><div class="col-12 tx-own"><span class="ellipsis copyable">mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz</span><span class="tx-amt"><span class="amt copyable" cc="0.00012345 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">012</span><span class="ns">345</span></span> FAKE</span></span><a class="spent" href="/spending/00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa3840/1" tt="Spent">→</a></span></td></div><div class="col-12 tx-own"><span class="ellipsis copyable">mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz</span><span class="tx-amt"><span class="amt copyable" cc="0.00012345 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">012</span><span class="ns">345</span></span> FAKE</span></span><span class="unspent" tt="Unspent">×</span></span></td></div></div></div></div><div class="row footer"><div class="col-sm-12 col-md-4">Fee <span class="amt txvalue copyable ms-3" cc="0 FAKE"><span class="prim-amt">0 FAKE</span></span></div><div class="col-sm-12 col-md-8 text-end"><span class="me-4"><span class="txvalue">2</span> confirmations</span><span class="amt txvalue copyable" cc="1.0002469 FAKE"><span class="prim-amt">1.<span class="amt-dec">00<span class="ns">024</span><span class="ns">690</span></span> FAKE</span></span></div></div></div></div></div></main><footer id="footer"><div class="container"><nav class="navbar navbar-dark"><span class="navbar-nav"><a class="nav-link" href="https://satoshilabs.com/" target="_blank" rel="noopener noreferrer">Created by SatoshiLabs</a></span><span class="navbar-nav ml-md-auto"><a class="nav-link" href="https://trezor.io/terms-of-use" target="_blank" rel="noopener noreferrer">Terms of Use</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/" target="_blank" rel="noopener noreferrer">Trezor</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/trezor-suite" target="_blank" rel="noopener noreferrer">Suite</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/support" target="_blank" rel="noopener noreferrer">Support</a></span><span class="navbar-nav ml-md-auto"><a class="nav-link" href="/sendtx">Send Transaction</a></span><span class="navbar-nav ml-md-auto d-lg-flex d-none"><a class="nav-link" href="https://trezor.io/compare" target="_blank" rel="noopener noreferrer">Don't have a Trezor? Get one!</a></span></nav></div></footer></body></html>`,
			},
		},
		{
			name:        "explorerSearch xpub",
			r:           newGetRequest(ts.URL + "/search?q=" + dbtestdata.Xpub),
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body: []string{
				`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1.0,shrink-to-fit=no"><link href="https://cdn.jsdelivr.net/npm/bootstrap@5.2.2/dist/css/bootstrap.min.css" rel="stylesheet" integrity="sha384-Zenh87qX5JnK2Jl0vWa8Ck2rdkQ2Bzep5IDxbcnCeuOxjzrPF/et3URy9Bv1WTRi" crossorigin="anonymous"><link rel="stylesheet" href="/static/css/main.min.3.css"><script>var hasSecondary=false;</script><script src="https://cdn.jsdelivr.net/npm/bootstrap@5.2.2/dist/js/bootstrap.bundle.min.js" integrity="sha384-OERcA2EqjJCMA+/3y+gxIOqMEjwtxJY7qPCqsdltbNJuaOe923+mo//f6V8Qbsw3" crossorigin="anonymous"></script><script src="/static/js/main.min.3.js"></script><meta http-equiv="X-UA-Compatible" content="IE=edge"><meta name="description" content="Trezor Fake Coin Explorer"><title>Trezor Fake Coin Explorer</title></head><body><header id="header"><nav class="navbar navbar-expand-lg"><div class="container"><a class="navbar-brand" href="/" title="Home"><span class="trezor-logo"></span><span style="padding-left: 140px;">Fake Coin Explorer</span></a><button class="navbar-toggler" type="button" data-bs-toggle="collapse" data-bs-target="#navbarSupportedContent" aria-controls="navbarSupportedContent" aria-expanded="false" aria-label="Toggle navigation"><span class="navbar-toggler-icon"></span></button><div class="collapse navbar-collapse" id="navbarSupportedContent"><ul class="navbar-nav m-md-auto"><li class="nav-item pe-xl-4"><a href="/blocks" class="nav-link">Blocks</a></li><li class="nav-item"><a href="/" class="nav-link">Status</a></li></ul><span class="navbar-form"><form class="d-flex" id="search" action="/search" method="get"><input name="q" type="text" class="form-control form-control-lg" placeholder="Search for block, transaction, address or xpub" focus="true"><button class="btn" type="submit"><span class="search-icon"></span></button></form></span></div></div></nav></header><main id="wrap"><div class="container"><div class="row"><div class="col-md-10 order-2 order-md-1"><h1>XPUB</h1><h5 class="col-12 d-flex h-data pb-2"><span class="ellipsis copyable">upub5E1xjDmZ7Hhej6LPpS8duATdKXnRYui7bDYj6ehfFGzWDZtmCmQkZhc3Zb7kgRLtHWd16QFxyP86JKL3ShZEBFX88aciJ3xyocuyhZZ8g6q</span></h5><h4 class="row"><div class="col-lg-6"><span class="copyable">1<span class="nc">186</span>.<span class="amt-dec">419<span class="ns">755</span></span> FAKE</span></div></h4></div><div class="col-md-2 order-1 order-md-2 d-flex justify-content-center justify-content-md-end mb-3 mb-md-0"><div id="qrcode"></div><script type="text/javascript" src="/static/js/qrcode.min.js"></script><script type="text/javascript">new QRCode(document.getElementById("qrcode"), { text: "upub5E1xjDmZ7Hhej6LPpS8duATdKXnRYui7bDYj6ehfFGzWDZtmCmQkZhc3Zb7kgRLtHWd16QFxyP86JKL3ShZEBFX88aciJ3xyocuyhZZ8g6q", width: 120, height: 120 });</script></div></div><table class="table data-table info-table"><tbody><tr><td style="white-space: nowrap;"><h5>Confirmed</h5></td><td></td></tr><tr><td style="width: 25%;">Total Received</td><td><span class="amt copyable" cc="1186.41975501 FAKE"><span class="prim-amt">1<span class="nc">186</span>.<span class="amt-dec">41<span class="ns">975</span><span class="ns">501</span></span> FAKE</span></span></td></tr><tr><td>Total Sent</td><td><span class="amt copyable" cc="0.00000001 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">000</span><span class="ns">001</span></span> FAKE</span></span></td></tr><tr><td>Final Balance</td><td><span class="amt copyable" cc="1186.419755 FAKE"><span class="prim-amt">1<span class="nc">186</span>.<span class="amt-dec">41<span class="ns">975</span><span class="ns">500</span></span> FAKE</span></span></td></tr><tr><td>No. Transactions</td><td>2</td></tr><tr><td>Used XPUB Addresses</td><td>2</td></tr></tbody></table><table class="table data-table"><tbody><tr><td style="white-space: nowrap; width: 50%;"><h5>XPUB Addresses with Balance</h5></td><td colspan="3"></td></tr><tr><th>Address</th><th>Balance</th><th>Txs</th><th>Path</th></tr><tr><td class="ellipsis"><a href="/address/2N6utyMZfPNUb1Bk8oz7p2JqJrXkq83gegu" class="copyable">2N6utyMZfPNUb1Bk8oz7p2JqJrXkq83gegu</a></td><td><span class="amt copyable" cc="1186.419755 FAKE"><span class="prim-amt">1<span class="nc">186</span>.<span class="amt-dec">41<span class="ns">975</span><span class="ns">500</span></span> FAKE</span></span></td><td>1</td><td>m/49&#39;/1&#39;/33&#39;/1/3</td></tr></tbody></table><div class="row mb-4"><div class="col-12"><a href="?tokens=used" class="ms-3 me-3">Show used XPUB addresses</a><a href="?tokens=derived">Show all derived XPUB addresses</a></div></div><div class="row pt-3 pb-1"><h3 class="col-sm-6 col-lg-3 m-0 align-self-center">Transactions</h3><div class="col-sm-6 col-lg-3 my-2 my-lg-0 align-self-center"><select  class="w-100" onchange="self.location='?filter='+options[selectedIndex].value"><option>All</option><option  value="inputs">XPUB addresses on input side</option><option  value="outputs">XPUB addresses on output side</option></select></div><div class="col-lg-6"></div></div><div><div class="tx-detail"><div class="row head"><div class="col-xs-7 col-md-8"><a href="/tx/3d90d15ed026dc45e19ffb52875ed18fa9e8012ad123d7f7212176e2b0ebdb71" class="ellipsis copyable txid">3d90d15ed026dc45e19ffb52875ed18fa9e8012ad123d7f7212176e2b0ebdb71</a></div><div class="col-xs-5 col-md-4 text-end">mined <span class="txvalue ms-1"><span tt="2018-03-21 01:27:58">1639 days 11 hours ago</span></span></div></div><div class="row body"><div class="col-md-5"><div class="row tx-in"><div class="col-12"><span class="ellipsis copyable"><a href="/address/mzB8cYrfRwFRFAGTDzV8LkUQy5BQicxGhX">mzB8cYrfRwFRFAGTDzV8LkUQy5BQicxGhX</a></span><span class="amt tx-amt copyable" cc="3172.83951061 FAKE"><span class="prim-amt">3<span class="nc">172</span>.<span class="amt-dec">83<span class="ns">951</span><span class="ns">061</span></span> FAKE</span></span></div><div class="col-12 tx-own"><span class="ellipsis copyable"><a href="/address/2MzmAKayJmja784jyHvRUW1bXPget1csRRG">2MzmAKayJmja784jyHvRUW1bXPget1csRRG</a></span><span class="amt tx-amt copyable" cc="0.00000001 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">000</span><span class="ns">001</span></span> FAKE</span></span></div></div></div><div class="col-md-1 col-xs-12 text-center">&nbsp;<span class="octicon"></span></div><div class="col-md-6"><div class="row tx-out"><div class="col-12 tx-own"><span class="ellipsis copyable"><a href="/address/2N6utyMZfPNUb1Bk8oz7p2JqJrXkq83gegu">2N6utyMZfPNUb1Bk8oz7p2JqJrXkq83gegu</a></span><span class="tx-amt"><span class="amt copyable" cc="1186.419755 FAKE"><span class="prim-amt">1<span class="nc">186</span>.<span class="amt-dec">41<span class="ns">975</span><span class="ns">500</span></span> FAKE</span></span><span class="unspent" tt="Unspent">×</span></span></td></div><div class="col-12"><span class="ellipsis copyable"><a href="/address/mmJx9Y8ayz9h14yd9fgCW1bUKoEpkBAquP">mmJx9Y8ayz9h14yd9fgCW1bUKoEpkBAquP</a></span><span class="tx-amt"><span class="amt copyable" cc="1986.419755 FAKE"><span class="prim-amt">1<span class="nc">986</span>.<span class="amt-dec">41<span class="ns">975</span><span class="ns">500</span></span> FAKE</span></span><span class="unspent" tt="Unspent">×</span></span></td></div></div></div></div><div class="row footer"><div class="col-sm-12 col-md-4">Fee <span class="amt txvalue copyable ms-3" cc="0.00000062 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">000</span><span class="ns">062</span></span> FAKE</span></span></div><div class="col-sm-12 col-md-8 text-end"><span class="me-4"><span class="txvalue">1</span> confirmations</span><span class="amt txvalue copyable" cc="3172.83951 FAKE"><span class="prim-amt">3<span class="nc">172</span>.<span class="amt-dec">83<span class="ns">951</span><span class="ns">000</span></span> FAKE</span></span></div></div></div><div class="tx-detail"><div class="row head"><div class="col-xs-7 col-md-8"><a href="/tx/effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75" class="ellipsis copyable txid">effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75</a></div><div class="col-xs-5 col-md-4 text-end">mined <span class="txvalue ms-1"><span tt="2018-03-20 03:03:46">1640 days 9 hours ago</span></span></div></div><div class="row body"><div class="col-md-5"><div class="row tx-in"><div class="col-12">No Inputs</div></div></div><div class="col-md-1 col-xs-12 text-center">&nbsp;<span class="octicon"></span></div><div class="col-md-6"><div class="row tx-out"><div class="col-12"><span class="ellipsis copyable"><a href="/address/mv9uLThosiEnGRbVPS7Vhyw6VssbVRsiAw">mv9uLThosiEnGRbVPS7Vhyw6VssbVRsiAw</a></span><span class="tx-amt"><span class="amt copyable" cc="12345.67890123 FAKE"><span class="prim-amt">12<span class="nc">345</span>.<span class="amt-dec">67<span class="ns">890</span><span class="ns">123</span></span> FAKE</span></span><a class="spent" href="/spending/effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75/0" tt="Spent">→</a></span></td></div><div class="col-12 tx-own"><span class="ellipsis copyable"><a href="/address/2MzmAKayJmja784jyHvRUW1bXPget1csRRG">2MzmAKayJmja784jyHvRUW1bXPget1csRRG</a></span><span class="tx-amt"><span class="amt copyable" cc="0.00000001 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">000</span><span class="ns">001</span></span> FAKE</span></span><a class="spent" href="/spending/effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75/1" tt="Spent">→</a></span></td></div><div class="col-12"><span class="ellipsis copyable"><a href="/address/2NEVv9LJmAnY99W1pFoc5UJjVdypBqdnvu1">2NEVv9LJmAnY99W1pFoc5UJjVdypBqdnvu1</a></span><span class="tx-amt"><span class="amt copyable" cc="0.00009876 FAKE"><span class="prim-amt">0.<span class="amt-dec">00<span class="ns">009</span><span class="ns">876</span></span> FAKE</span></span><a class="spent" href="/spending/effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75/2" tt="Spent">→</a></span></td></div></div></div></div><div class="row footer"><div class="col-sm-12 col-md-4">Fee <span class="amt txvalue copyable ms-3" cc="0 FAKE"><span class="prim-amt">0 FAKE</span></span></div><div class="col-sm-12 col-md-8 text-end"><span class="me-4"><span class="txvalue">2</span> confirmations</span><span class="amt txvalue copyable" cc="12345.679 FAKE"><span class="prim-amt">12<span class="nc">345</span>.<span class="amt-dec">67<span class="ns">900</span><span class="ns">000</span></span> FAKE</span></span></div></div></div></div></div></main><footer id="footer"><div class="container"><nav class="navbar navbar-dark"><span class="navbar-nav"><a class="nav-link" href="https://satoshilabs.com/" target="_blank" rel="noopener noreferrer">Created by SatoshiLabs</a></span><span class="navbar-nav ml-md-auto"><a class="nav-link" href="https://trezor.io/terms-of-use" target="_blank" rel="noopener noreferrer">Terms of Use</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/" target="_blank" rel="noopener noreferrer">Trezor</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/trezor-suite" target="_blank" rel="noopener noreferrer">Suite</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/support" target="_blank" rel="noopener noreferrer">Support</a></span><span class="navbar-nav ml-md-auto"><a class="nav-link" href="/sendtx">Send Transaction</a></span><span class="navbar-nav ml-md-auto d-lg-flex d-none"><a class="nav-link" href="https://trezor.io/compare" target="_blank" rel="noopener noreferrer">Don't have a Trezor? Get one!</a></span></nav></div></footer></body></html>`,
			},
		},
		{
			name:        "explorerSearch taproot descriptor",
			r:           newGetRequest(ts.URL + "/search?q=" + url.QueryEscape(dbtestdata.TaprootDescriptor)),
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body: []string{
				`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1.0,shrink-to-fit=no"><link href="https://cdn.jsdelivr.net/npm/bootstrap@5.2.2/dist/css/bootstrap.min.css" rel="stylesheet" integrity="sha384-Zenh87qX5JnK2Jl0vWa8Ck2rdkQ2Bzep5IDxbcnCeuOxjzrPF/et3URy9Bv1WTRi" crossorigin="anonymous"><link rel="stylesheet" href="/static/css/main.min.3.css"><script>var hasSecondary=false;</script><script src="https://cdn.jsdelivr.net/npm/bootstrap@5.2.2/dist/js/bootstrap.bundle.min.js" integrity="sha384-OERcA2EqjJCMA+/3y+gxIOqMEjwtxJY7qPCqsdltbNJuaOe923+mo//f6V8Qbsw3" crossorigin="anonymous"></script><script src="/static/js/main.min.3.js"></script><meta http-equiv="X-UA-Compatible" content="IE=edge"><meta name="description" content="Trezor Fake Coin Explorer"><title>Trezor Fake Coin Explorer</title></head><body><header id="header"><nav class="navbar navbar-expand-lg"><div class="container"><a class="navbar-brand" href="/" title="Home"><span class="trezor-logo"></span><span style="padding-left: 140px;">Fake Coin Explorer</span></a><button class="navbar-toggler" type="button" data-bs-toggle="collapse" data-bs-target="#navbarSupportedContent" aria-controls="navbarSupportedContent" aria-expanded="false" aria-label="Toggle navigation"><span class="navbar-toggler-icon"></span></button><div class="collapse navbar-collapse" id="navbarSupportedContent"><ul class="navbar-nav m-md-auto"><li class="nav-item pe-xl-4"><a href="/blocks" class="nav-link">Blocks</a></li><li class="nav-item"><a href="/" class="nav-link">Status</a></li></ul><span class="navbar-form"><form class="d-flex" id="search" action="/search" method="get"><input name="q" type="text" class="form-control form-control-lg" placeholder="Search for block, transaction, address or xpub" focus="true"><button class="btn" type="submit"><span class="search-icon"></span></button></form></span></div></div></nav></header><main id="wrap"><div class="container"><div class="row"><div class="col-md-10 order-2 order-md-1"><h1>XPUB</h1><h5 class="col-12 d-flex h-data pb-2"><span class="ellipsis copyable">tr([5c9e228d/86&#39;/1&#39;/0&#39;]tpubDC88gkaZi5HvJGxGDNLADkvtdpni3mLmx6vr2KnXmWMG8zfkBRggsxHVBkUpgcwPe2KKpkyvTJCdXHb1UHEWE64vczyyPQfHr1skBcsRedN/{0,1}/*)#4rqwxvej</span></h5><h4 class="row"><div class="col-lg-6"><span class="copyable">0 FAKE</span></div></h4></div><div class="col-md-2 order-1 order-md-2 d-flex justify-content-center justify-content-md-end mb-3 mb-md-0"><div id="qrcode"></div><script type="text/javascript" src="/static/js/qrcode.min.js"></script><script type="text/javascript">new QRCode(document.getElementById("qrcode"), { text: "tr([5c9e228d\/86\u0027\/1\u0027\/0\u0027]tpubDC88gkaZi5HvJGxGDNLADkvtdpni3mLmx6vr2KnXmWMG8zfkBRggsxHVBkUpgcwPe2KKpkyvTJCdXHb1UHEWE64vczyyPQfHr1skBcsRedN\/{0,1}\/*)#4rqwxvej", width: 120, height: 120 });</script></div></div><table class="table data-table info-table"><tbody><tr><td style="white-space: nowrap;"><h5>Confirmed</h5></td><td></td></tr><tr><td style="width: 25%;">Total Received</td><td><span class="amt copyable" cc="0 FAKE"><span class="prim-amt">0 FAKE</span></span></td></tr><tr><td>Total Sent</td><td><span class="amt copyable" cc="0 FAKE"><span class="prim-amt">0 FAKE</span></span></td></tr><tr><td>Final Balance</td><td><span class="amt copyable" cc="0 FAKE"><span class="prim-amt">0 FAKE</span></span></td></tr><tr><td>No. Transactions</td><td>0</td></tr><tr><td>Used XPUB Addresses</td><td>0</td></tr></tbody></table><table class="table data-table"><tbody><tr><td style="white-space: nowrap; width: 50%;"><h5>XPUB Addresses with Balance</h5></td><td colspan="3"></td></tr><tr><td colspan="4">No addresses</td></tr></tbody></table><div class="row mb-4"><div class="col-12"><a href="?tokens=used" class="ms-3 me-3">Show used XPUB addresses</a><a href="?tokens=derived">Show all derived XPUB addresses</a></div></div></div></main><footer id="footer"><div class="container"><nav class="navbar navbar-dark"><span class="navbar-nav"><a class="nav-link" href="https://satoshilabs.com/" target="_blank" rel="noopener noreferrer">Created by SatoshiLabs</a></span><span class="navbar-nav ml-md-auto"><a class="nav-link" href="https://trezor.io/terms-of-use" target="_blank" rel="noopener noreferrer">Terms of Use</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/" target="_blank" rel="noopener noreferrer">Trezor</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/trezor-suite" target="_blank" rel="noopener noreferrer">Suite</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/support" target="_blank" rel="noopener noreferrer">Support</a></span><span class="navbar-nav ml-md-auto"><a class="nav-link" href="/sendtx">Send Transaction</a></span><span class="navbar-nav ml-md-auto d-lg-flex d-none"><a class="nav-link" href="https://trezor.io/compare" target="_blank" rel="noopener noreferrer">Don't have a Trezor? Get one!</a></span></nav></div></footer></body></html>`,
			},
		},
		{
			name:        "explorerSearch not found",
			r:           newGetRequest(ts.URL + "/search?q=1234"),
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body: []string{
				`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1.0,shrink-to-fit=no"><link href="https://cdn.jsdelivr.net/npm/bootstrap@5.2.2/dist/css/bootstrap.min.css" rel="stylesheet" integrity="sha384-Zenh87qX5JnK2Jl0vWa8Ck2rdkQ2Bzep5IDxbcnCeuOxjzrPF/et3URy9Bv1WTRi" crossorigin="anonymous"><link rel="stylesheet" href="/static/css/main.min.3.css"><script>var hasSecondary=false;</script><script src="https://cdn.jsdelivr.net/npm/bootstrap@5.2.2/dist/js/bootstrap.bundle.min.js" integrity="sha384-OERcA2EqjJCMA+/3y+gxIOqMEjwtxJY7qPCqsdltbNJuaOe923+mo//f6V8Qbsw3" crossorigin="anonymous"></script><script src="/static/js/main.min.3.js"></script><meta http-equiv="X-UA-Compatible" content="IE=edge"><meta name="description" content="Trezor Fake Coin Explorer"><title>Trezor Fake Coin Explorer</title></head><body><header id="header"><nav class="navbar navbar-expand-lg"><div class="container"><a class="navbar-brand" href="/" title="Home"><span class="trezor-logo"></span><span style="padding-left: 140px;">Fake Coin Explorer</span></a><button class="navbar-toggler" type="button" data-bs-toggle="collapse" data-bs-target="#navbarSupportedContent" aria-controls="navbarSupportedContent" aria-expanded="false" aria-label="Toggle navigation"><span class="navbar-toggler-icon"></span></button><div class="collapse navbar-collapse" id="navbarSupportedContent"><ul class="navbar-nav m-md-auto"><li class="nav-item pe-xl-4"><a href="/blocks" class="nav-link">Blocks</a></li><li class="nav-item"><a href="/" class="nav-link">Status</a></li></ul><span class="navbar-form"><form class="d-flex" id="search" action="/search" method="get"><input name="q" type="text" class="form-control form-control-lg" placeholder="Search for block, transaction, address or xpub" focus="true"><button class="btn" type="submit"><span class="search-icon"></span></button></form></span></div></div></nav></header><main id="wrap"><div class="container"><h1>Error</h1><h4>No matching records found for &#39;1234&#39;</h4></div></main><footer id="footer"><div class="container"><nav class="navbar navbar-dark"><span class="navbar-nav"><a class="nav-link" href="https://satoshilabs.com/" target="_blank" rel="noopener noreferrer">Created by SatoshiLabs</a></span><span class="navbar-nav ml-md-auto"><a class="nav-link" href="https://trezor.io/terms-of-use" target="_blank" rel="noopener noreferrer">Terms of Use</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/" target="_blank" rel="noopener noreferrer">Trezor</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/trezor-suite" target="_blank" rel="noopener noreferrer">Suite</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/support" target="_blank" rel="noopener noreferrer">Support</a></span><span class="navbar-nav ml-md-auto"><a class="nav-link" href="/sendtx">Send Transaction</a></span><span class="navbar-nav ml-md-auto d-lg-flex d-none"><a class="nav-link" href="https://trezor.io/compare" target="_blank" rel="noopener noreferrer">Don't have a Trezor? Get one!</a></span></nav></div></footer></body></html>`,
			},
		},
		{
			name:        "explorerSendTx",
			r:           newGetRequest(ts.URL + "/sendtx"),
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body: []string{
				`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1.0,shrink-to-fit=no"><link href="https://cdn.jsdelivr.net/npm/bootstrap@5.2.2/dist/css/bootstrap.min.css" rel="stylesheet" integrity="sha384-Zenh87qX5JnK2Jl0vWa8Ck2rdkQ2Bzep5IDxbcnCeuOxjzrPF/et3URy9Bv1WTRi" crossorigin="anonymous"><link rel="stylesheet" href="/static/css/main.min.3.css"><script>var hasSecondary=false;</script><script src="https://cdn.jsdelivr.net/npm/bootstrap@5.2.2/dist/js/bootstrap.bundle.min.js" integrity="sha384-OERcA2EqjJCMA+/3y+gxIOqMEjwtxJY7qPCqsdltbNJuaOe923+mo//f6V8Qbsw3" crossorigin="anonymous"></script><script src="/static/js/main.min.3.js"></script><meta http-equiv="X-UA-Compatible" content="IE=edge"><meta name="description" content="Trezor Fake Coin Explorer"><title>Trezor Fake Coin Explorer</title></head><body><header id="header"><nav class="navbar navbar-expand-lg"><div class="container"><a class="navbar-brand" href="/" title="Home"><span class="trezor-logo"></span><span style="padding-left: 140px;">Fake Coin Explorer</span></a><button class="navbar-toggler" type="button" data-bs-toggle="collapse" data-bs-target="#navbarSupportedContent" aria-controls="navbarSupportedContent" aria-expanded="false" aria-label="Toggle navigation"><span class="navbar-toggler-icon"></span></button><div class="collapse navbar-collapse" id="navbarSupportedContent"><ul class="navbar-nav m-md-auto"><li class="nav-item pe-xl-4"><a href="/blocks" class="nav-link">Blocks</a></li><li class="nav-item"><a href="/" class="nav-link">Status</a></li></ul><span class="navbar-form"><form class="d-flex" id="search" action="/search" method="get"><input name="q" type="text" class="form-control form-control-lg" placeholder="Search for block, transaction, address or xpub" focus="true"><button class="btn" type="submit"><span class="search-icon"></span></button></form></span></div></div></nav></header><main id="wrap"><div class="container"><h1>Send Raw Transaction</h1><form method="POST" action="/sendtx"><div class="form-group"><label for="exampleFormControlTextarea1">Raw transaction data</label><textarea class="form-control" rows="8" name="hex"></textarea></div><div class="form-group mt-3"><button type="submit" class="btn btn-outline-secondary">Send</button></div></form></div></main><footer id="footer"><div class="container"><nav class="navbar navbar-dark"><span class="navbar-nav"><a class="nav-link" href="https://satoshilabs.com/" target="_blank" rel="noopener noreferrer">Created by SatoshiLabs</a></span><span class="navbar-nav ml-md-auto"><a class="nav-link" href="https://trezor.io/terms-of-use" target="_blank" rel="noopener noreferrer">Terms of Use</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/" target="_blank" rel="noopener noreferrer">Trezor</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/trezor-suite" target="_blank" rel="noopener noreferrer">Suite</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/support" target="_blank" rel="noopener noreferrer">Support</a></span><span class="navbar-nav ml-md-auto"><a class="nav-link" href="/sendtx">Send Transaction</a></span><span class="navbar-nav ml-md-auto d-lg-flex d-none"><a class="nav-link" href="https://trezor.io/compare" target="_blank" rel="noopener noreferrer">Don't have a Trezor? Get one!</a></span></nav></div></footer></body></html>`,
			},
		},
		{
			name:        "explorerSendTx POST",
			r:           newPostFormRequest(ts.URL+"/sendtx", "hex", "12341234"),
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body: []string{
				`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1.0,shrink-to-fit=no"><link href="https://cdn.jsdelivr.net/npm/bootstrap@5.2.2/dist/css/bootstrap.min.css" rel="stylesheet" integrity="sha384-Zenh87qX5JnK2Jl0vWa8Ck2rdkQ2Bzep5IDxbcnCeuOxjzrPF/et3URy9Bv1WTRi" crossorigin="anonymous"><link rel="stylesheet" href="/static/css/main.min.3.css"><script>var hasSecondary=false;</script><script src="https://cdn.jsdelivr.net/npm/bootstrap@5.2.2/dist/js/bootstrap.bundle.min.js" integrity="sha384-OERcA2EqjJCMA+/3y+gxIOqMEjwtxJY7qPCqsdltbNJuaOe923+mo//f6V8Qbsw3" crossorigin="anonymous"></script><script src="/static/js/main.min.3.js"></script><meta http-equiv="X-UA-Compatible" content="IE=edge"><meta name="description" content="Trezor Fake Coin Explorer"><title>Trezor Fake Coin Explorer</title></head><body><header id="header"><nav class="navbar navbar-expand-lg"><div class="container"><a class="navbar-brand" href="/" title="Home"><span class="trezor-logo"></span><span style="padding-left: 140px;">Fake Coin Explorer</span></a><button class="navbar-toggler" type="button" data-bs-toggle="collapse" data-bs-target="#navbarSupportedContent" aria-controls="navbarSupportedContent" aria-expanded="false" aria-label="Toggle navigation"><span class="navbar-toggler-icon"></span></button><div class="collapse navbar-collapse" id="navbarSupportedContent"><ul class="navbar-nav m-md-auto"><li class="nav-item pe-xl-4"><a href="/blocks" class="nav-link">Blocks</a></li><li class="nav-item"><a href="/" class="nav-link">Status</a></li></ul><span class="navbar-form"><form class="d-flex" id="search" action="/search" method="get"><input name="q" type="text" class="form-control form-control-lg" placeholder="Search for block, transaction, address or xpub" focus="true"><button class="btn" type="submit"><span class="search-icon"></span></button></form></span></div></div></nav></header><main id="wrap"><div class="container"><h1>Send Raw Transaction</h1><form method="POST" action="/sendtx"><div class="form-group"><label for="exampleFormControlTextarea1">Raw transaction data</label><textarea class="form-control" rows="8" name="hex">12341234</textarea></div><div class="form-group mt-3"><button type="submit" class="btn btn-outline-secondary">Send</button></div></form><div class="alert alert-danger mt-3">Invalid data</div></div></main><footer id="footer"><div class="container"><nav class="navbar navbar-dark"><span class="navbar-nav"><a class="nav-link" href="https://satoshilabs.com/" target="_blank" rel="noopener noreferrer">Created by SatoshiLabs</a></span><span class="navbar-nav ml-md-auto"><a class="nav-link" href="https://trezor.io/terms-of-use" target="_blank" rel="noopener noreferrer">Terms of Use</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/" target="_blank" rel="noopener noreferrer">Trezor</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/trezor-suite" target="_blank" rel="noopener noreferrer">Suite</a></span><span class="navbar-nav ml-md-auto d-md-flex d-none"><a class="nav-link" href="https://trezor.io/support" target="_blank" rel="noopener noreferrer">Support</a></span><span class="navbar-nav ml-md-auto"><a class="nav-link" href="/sendtx">Send Transaction</a></span><span class="navbar-nav ml-md-auto d-lg-flex d-none"><a class="nav-link" href="https://trezor.io/compare" target="_blank" rel="noopener noreferrer">Don't have a Trezor? Get one!</a></span></nav></div></footer></body></html>`,
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
				`"decimals":8`,
				`"backend":{"chain":"fakecoin","blocks":2,"headers":2,"bestBlockHash":"00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6"`,
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
			name:        "apiTx v1",
			r:           newGetRequest(ts.URL + "/api/v1/tx/05e2e48aeabdd9b75def7b48d756ba304713c2aba7b522bf9dbc893fc4231b07"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"txid":"05e2e48aeabdd9b75def7b48d756ba304713c2aba7b522bf9dbc893fc4231b07","vin":[{"txid":"effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75","vout":2,"n":0,"scriptSig":{},"addresses":["2NEVv9LJmAnY99W1pFoc5UJjVdypBqdnvu1"],"value":"0.00009876"}],"vout":[{"value":"0.00009","n":0,"scriptPubKey":{"hex":"a914e921fc4912a315078f370d959f2c4f7b6d2a683c87","addresses":["2NEVv9LJmAnY99W1pFoc5UJjVdypBqdnvu1"]},"spent":false}],"blockhash":"00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6","blockheight":225494,"confirmations":1,"time":1521595678,"blocktime":1521595678,"valueOut":"0.00009","valueIn":"0.00009876","fees":"0.00000876","hex":""}`,
			},
		},
		{
			name:        "apiTx - not found v1",
			r:           newGetRequest(ts.URL + "/api/v1/tx/1232e48aeabdd9b75def7b48d756ba304713c2aba7b522bf9dbc893fc4231b07"),
			status:      http.StatusBadRequest,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"error":"Transaction '1232e48aeabdd9b75def7b48d756ba304713c2aba7b522bf9dbc893fc4231b07' not found"}`,
			},
		},
		{
			name:        "apiTx v2",
			r:           newGetRequest(ts.URL + "/api/v2/tx/05e2e48aeabdd9b75def7b48d756ba304713c2aba7b522bf9dbc893fc4231b07"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"txid":"05e2e48aeabdd9b75def7b48d756ba304713c2aba7b522bf9dbc893fc4231b07","vin":[{"txid":"effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75","vout":2,"n":0,"addresses":["2NEVv9LJmAnY99W1pFoc5UJjVdypBqdnvu1"],"isAddress":true,"value":"9876"}],"vout":[{"value":"9000","n":0,"hex":"a914e921fc4912a315078f370d959f2c4f7b6d2a683c87","addresses":["2NEVv9LJmAnY99W1pFoc5UJjVdypBqdnvu1"],"isAddress":true}],"blockHash":"00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6","blockHeight":225494,"confirmations":1,"blockTime":1521595678,"value":"9000","valueIn":"9876","fees":"876"}`,
			},
		},
		{
			name:        "apiTx - not found v2",
			r:           newGetRequest(ts.URL + "/api/v2/tx/1232e48aeabdd9b75def7b48d756ba304713c2aba7b522bf9dbc893fc4231b07"),
			status:      http.StatusBadRequest,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"error":"Transaction '1232e48aeabdd9b75def7b48d756ba304713c2aba7b522bf9dbc893fc4231b07' not found"}`,
			},
		},
		{
			name:        "apiTxSpecific",
			r:           newGetRequest(ts.URL + "/api/tx-specific/00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa3840"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"hex":"","txid":"00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa3840","version":0,"locktime":0,"vin":[],"vout":[{"ValueSat":100000000,"value":0,"n":0,"scriptPubKey":{"hex":"76a914010d39800f86122416e28f485029acf77507169288ac","addresses":null}},{"ValueSat":12345,"value":0,"n":1,"scriptPubKey":{"hex":"76a9148bdf0aa3c567aa5975c2e61321b8bebbe7293df688ac","addresses":null}},{"ValueSat":12345,"value":0,"n":2,"scriptPubKey":{"hex":"76a9148bdf0aa3c567aa5975c2e61321b8bebbe7293df688ac","addresses":null}}],"confirmations":2,"time":1521515026,"blocktime":1521515026}`,
			},
		},
		{
			name:        "apiFeeStats",
			r:           newGetRequest(ts.URL + "/api/v2/feestats/225494"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"txCount":3,"totalFeesSat":"1284","averageFeePerKb":1398,"decilesFeePerKb":[155,155,155,155,1679,1679,1679,2361,2361,2361,2361]}`,
			},
		},
		{
			name:        "apiFiatRates all currencies",
			r:           newGetRequest(ts.URL + "/api/v2/tickers"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"ts":1574380800,"rates":{"eur":7134.1,"usd":7914.5}}`,
			},
		},
		{
			name:        "apiFiatRates get last rate",
			r:           newGetRequest(ts.URL + "/api/v2/tickers?currency=usd"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"ts":1574380800,"rates":{"usd":7914.5}}`,
			},
		},
		{
			name:        "apiFiatRates get rate by exact timestamp",
			r:           newGetRequest(ts.URL + "/api/v2/tickers?currency=usd&timestamp=1521545531"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"ts":1521590400,"rates":{"usd":2001}}`,
			},
		},
		{
			name:        "apiFiatRates incorrect timestamp",
			r:           newGetRequest(ts.URL + "/api/v2/tickers?currency=usd&timestamp=yesterday"),
			status:      http.StatusBadRequest,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"error":"Parameter 'timestamp' is not a valid Unix timestamp."}`,
			},
		},
		{
			name:        "apiFiatRates future timestamp",
			r:           newGetRequest(ts.URL + "/api/v2/tickers?currency=usd&timestamp=7980386400"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"ts":7980386400,"rates":{"usd":-1}}`,
			},
		},
		{
			name:        "apiFiatRates future timestamp, all currencies",
			r:           newGetRequest(ts.URL + "/api/v2/tickers?timestamp=7980386400"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"ts":7980386400,"rates":{}}`,
			},
		},
		{
			name:        "apiFiatRates get EUR rate (exact timestamp)",
			r:           newGetRequest(ts.URL + "/api/v2/tickers?timestamp=1574344800&currency=eur"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"ts":1574380800,"rates":{"eur":7134.1}`,
			},
		},
		{
			name:        "apiMultiFiatRates all currencies",
			r:           newGetRequest(ts.URL + "/api/v2/multi-tickers?timestamp=1574344800,1521677000"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`[{"ts":1574380800,"rates":{"eur":7134.1,"usd":7914.5}},{"ts":1521849600,"rates":{"eur":1303,"usd":2003}}]`,
			},
		},
		{
			name:        "apiMultiFiatRates get EUR rate",
			r:           newGetRequest(ts.URL + "/api/v2/multi-tickers?timestamp=1521545531,1574346615&currency=eur"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`[{"ts":1521590400,"rates":{"eur":1301}},{"ts":1574380800,"rates":{"eur":7134.1}}]`,
			},
		},
		{
			name:        "apiFiatRates get closest rate",
			r:           newGetRequest(ts.URL + "/api/v2/tickers?timestamp=1357045200&currency=usd"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"ts":1521504000,"rates":{"usd":2000}}`,
			},
		},
		{
			name:        "apiFiatRates get rate by block height",
			r:           newGetRequest(ts.URL + "/api/v2/tickers?block=225494&currency=usd"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"ts":1521676800,"rates":{"usd":2002}}`,
			},
		},
		{
			name:        "apiFiatRates get rate for EUR",
			r:           newGetRequest(ts.URL + "/api/v2/tickers?timestamp=1574346615&currency=eur"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"ts":1574380800,"rates":{"eur":7134.1}}`,
			},
		},
		{
			name:        "apiFiatRates get exact rate for an incorrect currency",
			r:           newGetRequest(ts.URL + "/api/v2/tickers?timestamp=1574346615&currency=does_not_exist"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"ts":1574346615,"rates":{"does_not_exist":-1}}`,
			},
		},
		{
			name:        "apiTickerList",
			r:           newGetRequest(ts.URL + "/api/v2/tickers-list?timestamp=1574346615"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"ts":1574380800,"available_currencies":["eur","usd"]}`,
			},
		},
		{
			name:        "apiAddress v1",
			r:           newGetRequest(ts.URL + "/api/v1/address/mv9uLThosiEnGRbVPS7Vhyw6VssbVRsiAw"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"page":1,"totalPages":1,"itemsOnPage":1000,"addrStr":"mv9uLThosiEnGRbVPS7Vhyw6VssbVRsiAw","balance":"0","totalReceived":"12345.67890123","totalSent":"12345.67890123","unconfirmedBalance":"0","unconfirmedTxApperances":0,"txApperances":2,"transactions":["7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25","effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75"]}`,
			},
		},
		{
			name:        "apiAddress v2",
			r:           newGetRequest(ts.URL + "/api/v2/address/mv9uLThosiEnGRbVPS7Vhyw6VssbVRsiAw"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"page":1,"totalPages":1,"itemsOnPage":1000,"address":"mv9uLThosiEnGRbVPS7Vhyw6VssbVRsiAw","balance":"0","totalReceived":"1234567890123","totalSent":"1234567890123","unconfirmedBalance":"0","unconfirmedTxs":0,"txs":2,"txids":["7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25","effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75"]}`,
			},
		},
		{
			name:        "apiAddress v2 details=basic",
			r:           newGetRequest(ts.URL + "/api/v2/address/mv9uLThosiEnGRbVPS7Vhyw6VssbVRsiAw?details=basic"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"address":"mv9uLThosiEnGRbVPS7Vhyw6VssbVRsiAw","balance":"0","totalReceived":"1234567890123","totalSent":"1234567890123","unconfirmedBalance":"0","unconfirmedTxs":0,"txs":2}`,
			},
		},
		{
			name:        "apiAddress v2 details=txs",
			r:           newGetRequest(ts.URL + "/api/v2/address/mv9uLThosiEnGRbVPS7Vhyw6VssbVRsiAw?details=txs"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"page":1,"totalPages":1,"itemsOnPage":1000,"address":"mv9uLThosiEnGRbVPS7Vhyw6VssbVRsiAw","balance":"0","totalReceived":"1234567890123","totalSent":"1234567890123","unconfirmedBalance":"0","unconfirmedTxs":0,"txs":2,"transactions":[{"txid":"7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25","vin":[{"txid":"effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75","n":0,"addresses":["mv9uLThosiEnGRbVPS7Vhyw6VssbVRsiAw"],"isAddress":true,"isOwn":true,"value":"1234567890123"},{"txid":"00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa3840","vout":1,"n":1,"addresses":["mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz"],"isAddress":true,"value":"12345"}],"vout":[{"value":"317283951061","n":0,"spent":true,"hex":"76a914ccaaaf374e1b06cb83118453d102587b4273d09588ac","addresses":["mzB8cYrfRwFRFAGTDzV8LkUQy5BQicxGhX"],"isAddress":true},{"value":"917283951061","n":1,"hex":"76a9148d802c045445df49613f6a70ddd2e48526f3701f88ac","addresses":["mtR97eM2HPWVM6c8FGLGcukgaHHQv7THoL"],"isAddress":true},{"value":"0","n":2,"hex":"6a072020f1686f6a20","addresses":["OP_RETURN 2020f1686f6a20"],"isAddress":false}],"blockHash":"00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6","blockHeight":225494,"confirmations":1,"blockTime":1521595678,"value":"1234567902122","valueIn":"1234567902468","fees":"346"},{"txid":"effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75","vin":[],"vout":[{"value":"1234567890123","n":0,"spent":true,"hex":"76a914a08eae93007f22668ab5e4a9c83c8cd1c325e3e088ac","addresses":["mv9uLThosiEnGRbVPS7Vhyw6VssbVRsiAw"],"isAddress":true,"isOwn":true},{"value":"1","n":1,"spent":true,"hex":"a91452724c5178682f70e0ba31c6ec0633755a3b41d987","addresses":["2MzmAKayJmja784jyHvRUW1bXPget1csRRG"],"isAddress":true},{"value":"9876","n":2,"spent":true,"hex":"a914e921fc4912a315078f370d959f2c4f7b6d2a683c87","addresses":["2NEVv9LJmAnY99W1pFoc5UJjVdypBqdnvu1"],"isAddress":true}],"blockHash":"0000000076fbbed90fd75b0e18856aa35baa984e9c9d444cf746ad85e94e2997","blockHeight":225493,"confirmations":2,"blockTime":1521515026,"value":"1234567900000","valueIn":"0","fees":"0"}]}`,
			},
		},
		{
			name:        "apiAddress v2 missing address",
			r:           newGetRequest(ts.URL + "/api/v2/address/"),
			status:      http.StatusBadRequest,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"error":"Missing address"}`,
			},
		},
		{
			name:        "apiXpub v2 default",
			r:           newGetRequest(ts.URL + "/api/v2/xpub/" + dbtestdata.Xpub),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"page":1,"totalPages":1,"itemsOnPage":1000,"address":"upub5E1xjDmZ7Hhej6LPpS8duATdKXnRYui7bDYj6ehfFGzWDZtmCmQkZhc3Zb7kgRLtHWd16QFxyP86JKL3ShZEBFX88aciJ3xyocuyhZZ8g6q","balance":"118641975500","totalReceived":"118641975501","totalSent":"1","unconfirmedBalance":"0","unconfirmedTxs":0,"txs":2,"addrTxCount":3,"txids":["3d90d15ed026dc45e19ffb52875ed18fa9e8012ad123d7f7212176e2b0ebdb71","effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75"],"usedTokens":2,"tokens":[{"type":"XPUBAddress","name":"2N6utyMZfPNUb1Bk8oz7p2JqJrXkq83gegu","path":"m/49'/1'/33'/1/3","transfers":1,"decimals":8,"balance":"118641975500","totalReceived":"118641975500","totalSent":"0"}]}`,
			},
		},
		{
			name:        "apiXpub v2 tokens=used",
			r:           newGetRequest(ts.URL + "/api/v2/xpub/" + dbtestdata.Xpub + "?tokens=used"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"page":1,"totalPages":1,"itemsOnPage":1000,"address":"upub5E1xjDmZ7Hhej6LPpS8duATdKXnRYui7bDYj6ehfFGzWDZtmCmQkZhc3Zb7kgRLtHWd16QFxyP86JKL3ShZEBFX88aciJ3xyocuyhZZ8g6q","balance":"118641975500","totalReceived":"118641975501","totalSent":"1","unconfirmedBalance":"0","unconfirmedTxs":0,"txs":2,"addrTxCount":3,"txids":["3d90d15ed026dc45e19ffb52875ed18fa9e8012ad123d7f7212176e2b0ebdb71","effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75"],"usedTokens":2,"tokens":[{"type":"XPUBAddress","name":"2MzmAKayJmja784jyHvRUW1bXPget1csRRG","path":"m/49'/1'/33'/0/0","transfers":2,"decimals":8,"balance":"0","totalReceived":"1","totalSent":"1"},{"type":"XPUBAddress","name":"2N6utyMZfPNUb1Bk8oz7p2JqJrXkq83gegu","path":"m/49'/1'/33'/1/3","transfers":1,"decimals":8,"balance":"118641975500","totalReceived":"118641975500","totalSent":"0"}]}`,
			},
		},
		{
			name:        "apiXpub v2 tokens=derived",
			r:           newGetRequest(ts.URL + "/api/v2/xpub/" + dbtestdata.Xpub + "?tokens=derived"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"page":1,"totalPages":1,"itemsOnPage":1000,"address":"upub5E1xjDmZ7Hhej6LPpS8duATdKXnRYui7bDYj6ehfFGzWDZtmCmQkZhc3Zb7kgRLtHWd16QFxyP86JKL3ShZEBFX88aciJ3xyocuyhZZ8g6q","balance":"118641975500","totalReceived":"118641975501","totalSent":"1","unconfirmedBalance":"0","unconfirmedTxs":0,"txs":2,"addrTxCount":3,"txids":["3d90d15ed026dc45e19ffb52875ed18fa9e8012ad123d7f7212176e2b0ebdb71","effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75"],"usedTokens":2,"tokens":[{"type":"XPUBAddress","name":"2MzmAKayJmja784jyHvRUW1bXPget1csRRG","path":"m/49'/1'/33'/0/0","transfers":2,"decimals":8,"balance":"0","totalReceived":"1","totalSent":"1"},{"type":"XPUBAddress","name":"2MsYfbi6ZdVXLDNrYAQ11ja9Sd3otMk4Pmj","path":"m/49'/1'/33'/0/1","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2MuAZNAjLSo6RLFad2fvHSfgqBD7BoEVy4T","path":"m/49'/1'/33'/0/2","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2NEqKzw3BosGnBE9by5uaDy5QgwjHac4Zbg","path":"m/49'/1'/33'/0/3","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2Mw7vJNC8zUK6VNN4CEjtoTYmuNPLewxZzV","path":"m/49'/1'/33'/0/4","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N1kvo97NFASPXiwephZUxE9PRXunjTxEc4","path":"m/49'/1'/33'/0/5","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2MuWrWMzoBt8VDFNvPmpJf42M1GTUs85fPx","path":"m/49'/1'/33'/0/6","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2MuVZ2Ca6Da9zmYynt49Rx7uikAgubGcymF","path":"m/49'/1'/33'/0/7","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2MzRGWDUmrPP9HwYu4B43QGCTLwoop5cExa","path":"m/49'/1'/33'/0/8","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N5C9EEWJzyBXhpyPHqa3UNed73Amsi5b3L","path":"m/49'/1'/33'/0/9","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2MzNawz2zjwq1L85GDE3YydEJGJYfXxaWkk","path":"m/49'/1'/33'/0/10","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N7NdeuAMgL57WE7QCeV2gTWi2Um8iAu5dA","path":"m/49'/1'/33'/0/11","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N8JQEP6DSHEZHNsSDPA1gHMUq9YFndhkfV","path":"m/49'/1'/33'/0/12","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2Mvbn3YXqKZVpQKugaoQrfjSYPvz76RwZkC","path":"m/49'/1'/33'/0/13","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N8MRNxCfwUY9TSW27X9ooGYtqgrGCfLRHx","path":"m/49'/1'/33'/0/14","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N6HvwrHC113KYZAmCtJ9XJNWgaTcnFunCM","path":"m/49'/1'/33'/0/15","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2NEo3oNyHUoi7rmRWee7wki37jxPWsWCopJ","path":"m/49'/1'/33'/0/16","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2Mzm5KY8qdFbDHsQfy4akXbFvbR3FAwDuVo","path":"m/49'/1'/33'/0/17","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2NGMwftmQCogp6XZNGvgiybz3WZysvsJzqC","path":"m/49'/1'/33'/0/18","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N3fJrrefndYjLGycvFFfYgevpZtcRKCkRD","path":"m/49'/1'/33'/0/19","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N1T7TnHBwfdpBoyw53EGUL7vuJmb2mU6jF","path":"m/49'/1'/33'/0/20","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2MzSBtRWHbBjeUcu3H5VRDqkvz5sfmDxJKo","path":"m/49'/1'/33'/1/0","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2MtShtAJYb1afWduUTwF1SixJjan7urZKke","path":"m/49'/1'/33'/1/1","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N3cP668SeqyBEr9gnB4yQEmU3VyxeRYith","path":"m/49'/1'/33'/1/2","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N6utyMZfPNUb1Bk8oz7p2JqJrXkq83gegu","path":"m/49'/1'/33'/1/3","transfers":1,"decimals":8,"balance":"118641975500","totalReceived":"118641975500","totalSent":"0"},{"type":"XPUBAddress","name":"2NEzatauNhf9kPTwwj6ZfYKjUdy52j4hVUL","path":"m/49'/1'/33'/1/4","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N4RjsDp4LBpkNqyF91aNjgpF9CwDwBkJZq","path":"m/49'/1'/33'/1/5","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N8XygTmQc4NoBBPEy3yybnfCYhsxFtzPDY","path":"m/49'/1'/33'/1/6","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N5BjBomZvb48sccK2vwLMiQ5ETKp1fdPVn","path":"m/49'/1'/33'/1/7","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2MybMwbZRPCGU3SMWPwQCpDkbcQFw5Hbwen","path":"m/49'/1'/33'/1/8","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N7HexL4dyAQc7Th4iqcCW4hZuyiZsLWf74","path":"m/49'/1'/33'/1/9","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2NF6X5FDGWrQj4nQrfP6hA77zB5WAc1DGup","path":"m/49'/1'/33'/1/10","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N4ZRPdvc7BVioBTohy4F6QtxreqcjNj26b","path":"m/49'/1'/33'/1/11","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2Mtfho1rLmevh4qTnkYWxZEFCWteDMtTcUF","path":"m/49'/1'/33'/1/12","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2NFUCphKYvmMcNZRZrF261mRX6iADVB9Qms","path":"m/49'/1'/33'/1/13","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N5kBNMB8qgxE4Y4f8J19fScsE49J4aNvoJ","path":"m/49'/1'/33'/1/14","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2NANWCaefhCKdXMcW8NbZnnrFRDvhJN2wPy","path":"m/49'/1'/33'/1/15","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2NFHw7Yo2Bz8D2wGAYHW9qidbZFLpfJ72qB","path":"m/49'/1'/33'/1/16","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2NBDSsBgy5PpFniLCb1eAFHcSxgxwPSDsZa","path":"m/49'/1'/33'/1/17","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2NDWCSQHogc7sCuc2WoYt9PX2i2i6a5k6dX","path":"m/49'/1'/33'/1/18","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N8vNyDP7iSDjm3BKpXrbDjAxyphqfvnJz8","path":"m/49'/1'/33'/1/19","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N4tFKLurSbMusAyq1tv4tzymVjveAFV1Vb","path":"m/49'/1'/33'/1/20","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2NBx5WwjAr2cH6Yqrp3Vsf957HtRKwDUVdX","path":"m/49'/1'/33'/1/21","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2NBu1seHTaFhQxbcW5L5BkZzqFLGmZqpxsa","path":"m/49'/1'/33'/1/22","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2NCDLoea22jGsXuarfT1n2QyCUh6RFhAPnT","path":"m/49'/1'/33'/1/23","transfers":0,"decimals":8}]}`,
			},
		},
		{
			name:        "apiXpub v2 taproot descriptor tokens=derived",
			r:           newGetRequest(ts.URL + "/api/v2/xpub/" + url.QueryEscape(dbtestdata.TaprootDescriptor) + "?tokens=derived&gap=2"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"page":1,"totalPages":1,"itemsOnPage":1000,"address":"tr([5c9e228d/86'/1'/0']tpubDC88gkaZi5HvJGxGDNLADkvtdpni3mLmx6vr2KnXmWMG8zfkBRggsxHVBkUpgcwPe2KKpkyvTJCdXHb1UHEWE64vczyyPQfHr1skBcsRedN/{0,1}/*)#4rqwxvej","balance":"0","totalReceived":"0","totalSent":"0","unconfirmedBalance":"0","unconfirmedTxs":0,"txs":0,"tokens":[{"type":"XPUBAddress","name":"tb1pswrqtykue8r89t9u4rprjs0gt4qzkdfuursfnvqaa3f2yql07zmq8s8a5u","path":"m/86'/1'/0'/0/0","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"tb1p8tvmvsvhsee73rhym86wt435qrqm92psfsyhy6a3n5gw455znnpqm8wald","path":"m/86'/1'/0'/0/1","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"tb1p537ddhyuydg5c2v75xxmn6ac64yz4xns2x0gpdcwj5vzzzgrywlqlqwk43","path":"m/86'/1'/0'/0/2","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"tb1pn2d0yjeedavnkd8z8lhm566p0f2utm3lgvxrsdehnl94y34txmts5s7t4c","path":"m/86'/1'/0'/1/0","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"tb1p0pnd6ue5vryymvd28aeq3kdz6rmsdjqrq6eespgtg8wdgnxjzjksujhq4u","path":"m/86'/1'/0'/1/1","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"tb1p29gpmd96hhgf7wj2vs03ca7x2xx39g8t6e0p55h2d5ssqs4fsj8qtx00wc","path":"m/86'/1'/0'/1/2","transfers":0,"decimals":8}]}`,
			},
		},
		{
			name:        "apiXpub v2 details=basic",
			r:           newGetRequest(ts.URL + "/api/v2/xpub/" + dbtestdata.Xpub + "?details=basic"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"address":"upub5E1xjDmZ7Hhej6LPpS8duATdKXnRYui7bDYj6ehfFGzWDZtmCmQkZhc3Zb7kgRLtHWd16QFxyP86JKL3ShZEBFX88aciJ3xyocuyhZZ8g6q","balance":"118641975500","totalReceived":"118641975501","totalSent":"1","unconfirmedBalance":"0","unconfirmedTxs":0,"txs":3,"addrTxCount":3,"usedTokens":2}`,
			},
		},
		{
			name:        "apiXpub v2 details=tokens?tokens=used",
			r:           newGetRequest(ts.URL + "/api/v2/xpub/" + dbtestdata.Xpub + "?details=tokens&tokens=used"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"address":"upub5E1xjDmZ7Hhej6LPpS8duATdKXnRYui7bDYj6ehfFGzWDZtmCmQkZhc3Zb7kgRLtHWd16QFxyP86JKL3ShZEBFX88aciJ3xyocuyhZZ8g6q","balance":"118641975500","totalReceived":"118641975501","totalSent":"1","unconfirmedBalance":"0","unconfirmedTxs":0,"txs":3,"addrTxCount":3,"usedTokens":2,"tokens":[{"type":"XPUBAddress","name":"2MzmAKayJmja784jyHvRUW1bXPget1csRRG","path":"m/49'/1'/33'/0/0","transfers":2,"decimals":8},{"type":"XPUBAddress","name":"2N6utyMZfPNUb1Bk8oz7p2JqJrXkq83gegu","path":"m/49'/1'/33'/1/3","transfers":1,"decimals":8}]}`,
			},
		},
		{
			name:        "apiXpub v2 details=tokenBalances",
			r:           newGetRequest(ts.URL + "/api/v2/xpub/" + dbtestdata.Xpub + "?details=tokenBalances"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"address":"upub5E1xjDmZ7Hhej6LPpS8duATdKXnRYui7bDYj6ehfFGzWDZtmCmQkZhc3Zb7kgRLtHWd16QFxyP86JKL3ShZEBFX88aciJ3xyocuyhZZ8g6q","balance":"118641975500","totalReceived":"118641975501","totalSent":"1","unconfirmedBalance":"0","unconfirmedTxs":0,"txs":3,"addrTxCount":3,"usedTokens":2,"tokens":[{"type":"XPUBAddress","name":"2N6utyMZfPNUb1Bk8oz7p2JqJrXkq83gegu","path":"m/49'/1'/33'/1/3","transfers":1,"decimals":8,"balance":"118641975500","totalReceived":"118641975500","totalSent":"0"}]}`,
			},
		},
		{
			name:        "apiXpub v2 details=txs&tokens=derived&gap=5&from=225494&to=225494&pageSize=3",
			r:           newGetRequest(ts.URL + "/api/v2/xpub/" + dbtestdata.Xpub + "?details=txs&tokens=derived&gap=5&from=225494&to=225494&pageSize=3"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"page":1,"totalPages":1,"itemsOnPage":3,"address":"upub5E1xjDmZ7Hhej6LPpS8duATdKXnRYui7bDYj6ehfFGzWDZtmCmQkZhc3Zb7kgRLtHWd16QFxyP86JKL3ShZEBFX88aciJ3xyocuyhZZ8g6q","balance":"118641975500","totalReceived":"118641975501","totalSent":"1","unconfirmedBalance":"0","unconfirmedTxs":0,"txs":2,"addrTxCount":3,"transactions":[{"txid":"3d90d15ed026dc45e19ffb52875ed18fa9e8012ad123d7f7212176e2b0ebdb71","vin":[{"txid":"7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25","n":0,"addresses":["mzB8cYrfRwFRFAGTDzV8LkUQy5BQicxGhX"],"isAddress":true,"value":"317283951061"},{"txid":"effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75","vout":1,"n":1,"addresses":["2MzmAKayJmja784jyHvRUW1bXPget1csRRG"],"isAddress":true,"isOwn":true,"value":"1"}],"vout":[{"value":"118641975500","n":0,"hex":"a91495e9fbe306449c991d314afe3c3567d5bf78efd287","addresses":["2N6utyMZfPNUb1Bk8oz7p2JqJrXkq83gegu"],"isAddress":true,"isOwn":true},{"value":"198641975500","n":1,"hex":"76a9143f8ba3fda3ba7b69f5818086e12223c6dd25e3c888ac","addresses":["mmJx9Y8ayz9h14yd9fgCW1bUKoEpkBAquP"],"isAddress":true}],"blockHash":"00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6","blockHeight":225494,"confirmations":1,"blockTime":1521595678,"value":"317283951000","valueIn":"317283951062","fees":"62"}],"usedTokens":2,"tokens":[{"type":"XPUBAddress","name":"2MzmAKayJmja784jyHvRUW1bXPget1csRRG","path":"m/49'/1'/33'/0/0","transfers":2,"decimals":8,"balance":"0","totalReceived":"1","totalSent":"1"},{"type":"XPUBAddress","name":"2MsYfbi6ZdVXLDNrYAQ11ja9Sd3otMk4Pmj","path":"m/49'/1'/33'/0/1","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2MuAZNAjLSo6RLFad2fvHSfgqBD7BoEVy4T","path":"m/49'/1'/33'/0/2","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2NEqKzw3BosGnBE9by5uaDy5QgwjHac4Zbg","path":"m/49'/1'/33'/0/3","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2Mw7vJNC8zUK6VNN4CEjtoTYmuNPLewxZzV","path":"m/49'/1'/33'/0/4","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N1kvo97NFASPXiwephZUxE9PRXunjTxEc4","path":"m/49'/1'/33'/0/5","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2MzSBtRWHbBjeUcu3H5VRDqkvz5sfmDxJKo","path":"m/49'/1'/33'/1/0","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2MtShtAJYb1afWduUTwF1SixJjan7urZKke","path":"m/49'/1'/33'/1/1","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N3cP668SeqyBEr9gnB4yQEmU3VyxeRYith","path":"m/49'/1'/33'/1/2","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N6utyMZfPNUb1Bk8oz7p2JqJrXkq83gegu","path":"m/49'/1'/33'/1/3","transfers":1,"decimals":8,"balance":"118641975500","totalReceived":"118641975500","totalSent":"0"},{"type":"XPUBAddress","name":"2NEzatauNhf9kPTwwj6ZfYKjUdy52j4hVUL","path":"m/49'/1'/33'/1/4","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N4RjsDp4LBpkNqyF91aNjgpF9CwDwBkJZq","path":"m/49'/1'/33'/1/5","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N8XygTmQc4NoBBPEy3yybnfCYhsxFtzPDY","path":"m/49'/1'/33'/1/6","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N5BjBomZvb48sccK2vwLMiQ5ETKp1fdPVn","path":"m/49'/1'/33'/1/7","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2MybMwbZRPCGU3SMWPwQCpDkbcQFw5Hbwen","path":"m/49'/1'/33'/1/8","transfers":0,"decimals":8}]}`,
			},
		},
		{
			name:        "apiXpub v2 missing xpub",
			r:           newGetRequest(ts.URL + "/api/v2/xpub/"),
			status:      http.StatusBadRequest,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"error":"Missing xpub"}`,
			},
		},
		{
			name:        "apiUtxo v1",
			r:           newGetRequest(ts.URL + "/api/v1/utxo/mtR97eM2HPWVM6c8FGLGcukgaHHQv7THoL"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`[{"txid":"7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25","vout":1,"amount":"9172.83951061","satoshis":917283951061,"height":225494,"confirmations":1}]`,
			},
		},
		{
			name:        "apiUtxo v2",
			r:           newGetRequest(ts.URL + "/api/v2/utxo/mtR97eM2HPWVM6c8FGLGcukgaHHQv7THoL"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`[{"txid":"7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25","vout":1,"value":"917283951061","height":225494,"confirmations":1}]`,
			},
		},
		{
			name:        "apiUtxo v2 xpub",
			r:           newGetRequest(ts.URL + "/api/v2/utxo/" + dbtestdata.Xpub),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`[{"txid":"3d90d15ed026dc45e19ffb52875ed18fa9e8012ad123d7f7212176e2b0ebdb71","vout":0,"value":"118641975500","height":225494,"confirmations":1,"address":"2N6utyMZfPNUb1Bk8oz7p2JqJrXkq83gegu","path":"m/49'/1'/33'/1/3"}]`,
			},
		},
		{
			name:        "apiUtxo v2 xpub",
			r:           newGetRequest(ts.URL + "/api/v2/utxo/" + url.QueryEscape(dbtestdata.TaprootDescriptor)),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`[]`,
			},
		},
		{
			name:        "apiBalanceHistory Addr2 v2",
			r:           newGetRequest(ts.URL + "/api/v2/balancehistory/mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`[{"time":1521514800,"txs":1,"received":"24690","sent":"0","sentToSelf":"0","rates":{"eur":1301,"usd":2001}},{"time":1521594000,"txs":1,"received":"0","sent":"12345","sentToSelf":"0","rates":{"eur":1302,"usd":2002}}]`,
			},
		},
		{
			name:        "apiBalanceHistory Addr5 v2",
			r:           newGetRequest(ts.URL + "/api/v2/balancehistory/2NEVv9LJmAnY99W1pFoc5UJjVdypBqdnvu1"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`[{"time":1521514800,"txs":1,"received":"9876","sent":"0","sentToSelf":"0","rates":{"eur":1301,"usd":2001}},{"time":1521594000,"txs":1,"received":"9000","sent":"9876","sentToSelf":"9000","rates":{"eur":1302,"usd":2002}}]`,
			},
		},
		{
			name:        "apiBalanceHistory Addr5 v2 fiatcurrency=eur",
			r:           newGetRequest(ts.URL + "/api/v2/balancehistory/2NEVv9LJmAnY99W1pFoc5UJjVdypBqdnvu1?fiatcurrency=eur"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`[{"time":1521514800,"txs":1,"received":"9876","sent":"0","sentToSelf":"0","rates":{"eur":1301}},{"time":1521594000,"txs":1,"received":"9000","sent":"9876","sentToSelf":"9000","rates":{"eur":1302}}]`,
			},
		},
		{
			name:        "apiBalanceHistory Addr2 v2 from=1521504000&to=1521590400",
			r:           newGetRequest(ts.URL + "/api/v2/balancehistory/mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz?from=1521504000&to=1521590400"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`[{"time":1521514800,"txs":1,"received":"24690","sent":"0","sentToSelf":"0","rates":{"eur":1301,"usd":2001}}]`,
			},
		},
		{
			name:        "apiBalanceHistory xpub v2",
			r:           newGetRequest(ts.URL + "/api/v2/balancehistory/" + dbtestdata.Xpub),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`[{"time":1521514800,"txs":1,"received":"1","sent":"0","sentToSelf":"0","rates":{"eur":1301,"usd":2001}},{"time":1521594000,"txs":1,"received":"118641975500","sent":"1","sentToSelf":"118641975500","rates":{"eur":1302,"usd":2002}}]`,
			},
		},
		{
			name:        "apiBalanceHistory xpub v2 from=1521504000&to=1521590400",
			r:           newGetRequest(ts.URL + "/api/v2/balancehistory/" + dbtestdata.Xpub + "?from=1521504000&to=1521590400"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`[{"time":1521514800,"txs":1,"received":"1","sent":"0","sentToSelf":"0","rates":{"eur":1301,"usd":2001}}]`,
			},
		},
		{
			name:        "apiBalanceHistory xpub v2 from=1521504000&to=1521590400&fiatcurrency=usd",
			r:           newGetRequest(ts.URL + "/api/v2/balancehistory/" + dbtestdata.Xpub + "?from=1521504000&to=1521590400&fiatcurrency=usd"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`[{"time":1521514800,"txs":1,"received":"1","sent":"0","sentToSelf":"0","rates":{"usd":2001}}]`,
			},
		},
		{
			name:        "apiBalanceHistory xpub v2 from=1521590400",
			r:           newGetRequest(ts.URL + "/api/v2/balancehistory/" + dbtestdata.Xpub + "?from=1521590400"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`[{"time":1521594000,"txs":1,"received":"118641975500","sent":"1","sentToSelf":"118641975500","rates":{"eur":1302,"usd":2002}}]`,
			},
		},
		{
			name:        "apiSendTx",
			r:           newGetRequest(ts.URL + "/api/v2/sendtx/1234567890"),
			status:      http.StatusBadRequest,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"error":"Invalid data"}`,
			},
		},
		{
			name:        "apiSendTx POST",
			r:           newPostRequest(ts.URL+"/api/v2/sendtx/", "123456"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"result":"9876"}`,
			},
		},
		{
			name:        "apiSendTx POST empty",
			r:           newPostRequest(ts.URL+"/api/v2/sendtx", ""),
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
		{
			name:        "apiGetBlock",
			r:           newGetRequest(ts.URL + "/api/v2/block/225493"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"page":1,"totalPages":1,"itemsOnPage":1000,"hash":"0000000076fbbed90fd75b0e18856aa35baa984e9c9d444cf746ad85e94e2997","nextBlockHash":"00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6","height":225493,"confirmations":2,"size":1234567,"time":1521515026,"version":0,"merkleRoot":"","nonce":"","bits":"","difficulty":"","txCount":2,"txs":[{"txid":"00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa3840","vin":[],"vout":[{"value":"100000000","n":0,"addresses":["mfcWp7DB6NuaZsExybTTXpVgWz559Np4Ti"],"isAddress":true},{"value":"12345","n":1,"spent":true,"addresses":["mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz"],"isAddress":true},{"value":"12345","n":2,"addresses":["mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz"],"isAddress":true}],"blockHash":"0000000076fbbed90fd75b0e18856aa35baa984e9c9d444cf746ad85e94e2997","blockHeight":225493,"confirmations":2,"blockTime":1521515026,"value":"100024690","valueIn":"0","fees":"0"},{"txid":"effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75","vin":[],"vout":[{"value":"1234567890123","n":0,"spent":true,"addresses":["mv9uLThosiEnGRbVPS7Vhyw6VssbVRsiAw"],"isAddress":true},{"value":"1","n":1,"spent":true,"addresses":["2MzmAKayJmja784jyHvRUW1bXPget1csRRG"],"isAddress":true},{"value":"9876","n":2,"spent":true,"addresses":["2NEVv9LJmAnY99W1pFoc5UJjVdypBqdnvu1"],"isAddress":true}],"blockHash":"0000000076fbbed90fd75b0e18856aa35baa984e9c9d444cf746ad85e94e2997","blockHeight":225493,"confirmations":2,"blockTime":1521515026,"value":"1234567900000","valueIn":"0","fees":"0"}]}`,
			},
		},
		{
			name:        "apiGetRawBlock",
			r:           newGetRequest(ts.URL + "/api/v2/rawblock/225493"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"hex":"00e0ff3fd42677a86f1515bafcf9802c1765e02226655a9b97fd44132602000000000000"}`,
			},
		},
	}
	performHttpTests(tests, t, ts)
}

func socketioTestsBitcoinType(t *testing.T, ts *httptest.Server) {
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
			name: "socketio getInfo",
			req:  socketioReq{"getInfo", []interface{}{}},
			want: `{"result":{"blocks":225494,"testnet":true,"network":"fakecoin","subversion":"/Fakecoin:0.0.1/","coin_name":"Fakecoin","about":"Blockbook - blockchain indexer for Trezor Suite https://trezor.io/trezor-suite. Do not use for any other purpose."}}`,
		},
		{
			name: "socketio estimateFee",
			req:  socketioReq{"estimateFee", []interface{}{17}},
			want: `{"result":0.000034}`,
		},
		{
			name: "socketio estimateSmartFee",
			req:  socketioReq{"estimateSmartFee", []interface{}{19, true}},
			want: `{"result":0.000019}`,
		},
		{
			name: "socketio getAddressTxids",
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
			name: "socketio getAddressTxids limited range",
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
			name: "socketio getAddressHistory",
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
			want: `{"result":{"totalCount":2,"items":[{"addresses":{"mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz":{"inputIndexes":[1],"outputIndexes":[]}},"satoshis":-12345,"confirmations":1,"tx":{"hex":"","height":225494,"blockTimestamp":1521595678,"version":0,"hash":"7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25","inputs":[{"txid":"effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75","outputIndex":0,"script":"","sequence":0,"address":"mv9uLThosiEnGRbVPS7Vhyw6VssbVRsiAw","satoshis":1234567890123},{"txid":"00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa3840","outputIndex":1,"script":"","sequence":0,"address":"mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz","satoshis":12345}],"inputSatoshis":1234567902468,"outputs":[{"satoshis":317283951061,"script":"76a914ccaaaf374e1b06cb83118453d102587b4273d09588ac","address":"mzB8cYrfRwFRFAGTDzV8LkUQy5BQicxGhX"},{"satoshis":917283951061,"script":"76a9148d802c045445df49613f6a70ddd2e48526f3701f88ac","address":"mtR97eM2HPWVM6c8FGLGcukgaHHQv7THoL"},{"satoshis":0,"script":"6a072020f1686f6a20","address":"OP_RETURN 2020f1686f6a20"}],"outputSatoshis":1234567902122,"feeSatoshis":346}},{"addresses":{"mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz":{"inputIndexes":[],"outputIndexes":[1,2]}},"satoshis":24690,"confirmations":2,"tx":{"hex":"","height":225493,"blockTimestamp":1521515026,"version":0,"hash":"00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa3840","inputs":[],"outputs":[{"satoshis":100000000,"script":"76a914010d39800f86122416e28f485029acf77507169288ac","address":"mfcWp7DB6NuaZsExybTTXpVgWz559Np4Ti"},{"satoshis":12345,"script":"76a9148bdf0aa3c567aa5975c2e61321b8bebbe7293df688ac","address":"mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz"},{"satoshis":12345,"script":"76a9148bdf0aa3c567aa5975c2e61321b8bebbe7293df688ac","address":"mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz"}],"outputSatoshis":100024690}}]}}`,
		},
		{
			name: "socketio getBlockHeader",
			req:  socketioReq{"getBlockHeader", []interface{}{225493}},
			want: `{"result":{"hash":"0000000076fbbed90fd75b0e18856aa35baa984e9c9d444cf746ad85e94e2997","version":0,"confirmations":0,"height":0,"chainWork":"","nextHash":"","merkleRoot":"","time":0,"medianTime":0,"nonce":0,"bits":"","difficulty":0}}`,
		},
		{
			name: "socketio getDetailedTransaction",
			req:  socketioReq{"getDetailedTransaction", []interface{}{"3d90d15ed026dc45e19ffb52875ed18fa9e8012ad123d7f7212176e2b0ebdb71"}},
			want: `{"result":{"hex":"","height":225494,"blockTimestamp":1521595678,"version":0,"hash":"3d90d15ed026dc45e19ffb52875ed18fa9e8012ad123d7f7212176e2b0ebdb71","inputs":[{"txid":"7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25","outputIndex":0,"script":"","sequence":0,"address":"mzB8cYrfRwFRFAGTDzV8LkUQy5BQicxGhX","satoshis":317283951061},{"txid":"effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75","outputIndex":1,"script":"","sequence":0,"address":"2MzmAKayJmja784jyHvRUW1bXPget1csRRG","satoshis":1}],"inputSatoshis":317283951062,"outputs":[{"satoshis":118641975500,"script":"a91495e9fbe306449c991d314afe3c3567d5bf78efd287","address":"2N6utyMZfPNUb1Bk8oz7p2JqJrXkq83gegu"},{"satoshis":198641975500,"script":"76a9143f8ba3fda3ba7b69f5818086e12223c6dd25e3c888ac","address":"mmJx9Y8ayz9h14yd9fgCW1bUKoEpkBAquP"}],"outputSatoshis":317283951000,"feeSatoshis":62}}`,
		},
		{
			name: "socketio sendTransaction",
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
				t.Errorf("got %v, want %v", resp, tt.want)
			}
		})
	}
}

func websocketTestsBitcoinType(t *testing.T, ts *httptest.Server) {
	type websocketReq struct {
		ID     string      `json:"id"`
		Method string      `json:"method"`
		Params interface{} `json:"params,omitempty"`
	}
	type websocketResp struct {
		ID string `json:"id"`
	}
	url := strings.Replace(ts.URL, "http://", "ws://", 1) + "/websocket"
	s, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	tests := []struct {
		name string
		req  websocketReq
		want string
	}{
		{
			name: "websocket getInfo",
			req: websocketReq{
				Method: "getInfo",
			},
			want: `{"id":"0","data":{"name":"Fakecoin","shortcut":"FAKE","decimals":8,"version":"unknown","bestHeight":225494,"bestHash":"00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6","block0Hash":"","testnet":true,"backend":{"version":"001001","subversion":"/Fakecoin:0.0.1/"}}}`,
		},
		{
			name: "websocket getBlockHash",
			req: websocketReq{
				Method: "getBlockHash",
				Params: map[string]interface{}{
					"height": 225494,
				},
			},
			want: `{"id":"1","data":{"hash":"00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6"}}`,
		},
		{
			name: "websocket getAccountInfo xpub txs",
			req: websocketReq{
				Method: "getAccountInfo",
				Params: map[string]interface{}{
					"descriptor": dbtestdata.Xpub,
					"details":    "txs",
				},
			},
			want: `{"id":"2","data":{"page":1,"totalPages":1,"itemsOnPage":25,"address":"upub5E1xjDmZ7Hhej6LPpS8duATdKXnRYui7bDYj6ehfFGzWDZtmCmQkZhc3Zb7kgRLtHWd16QFxyP86JKL3ShZEBFX88aciJ3xyocuyhZZ8g6q","balance":"118641975500","totalReceived":"118641975501","totalSent":"1","unconfirmedBalance":"0","unconfirmedTxs":0,"txs":2,"addrTxCount":3,"transactions":[{"txid":"3d90d15ed026dc45e19ffb52875ed18fa9e8012ad123d7f7212176e2b0ebdb71","vin":[{"txid":"7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25","n":0,"addresses":["mzB8cYrfRwFRFAGTDzV8LkUQy5BQicxGhX"],"isAddress":true,"value":"317283951061"},{"txid":"effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75","vout":1,"n":1,"addresses":["2MzmAKayJmja784jyHvRUW1bXPget1csRRG"],"isAddress":true,"isOwn":true,"value":"1"}],"vout":[{"value":"118641975500","n":0,"hex":"a91495e9fbe306449c991d314afe3c3567d5bf78efd287","addresses":["2N6utyMZfPNUb1Bk8oz7p2JqJrXkq83gegu"],"isAddress":true,"isOwn":true},{"value":"198641975500","n":1,"hex":"76a9143f8ba3fda3ba7b69f5818086e12223c6dd25e3c888ac","addresses":["mmJx9Y8ayz9h14yd9fgCW1bUKoEpkBAquP"],"isAddress":true}],"blockHash":"00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6","blockHeight":225494,"confirmations":1,"blockTime":1521595678,"value":"317283951000","valueIn":"317283951062","fees":"62"},{"txid":"effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75","vin":[],"vout":[{"value":"1234567890123","n":0,"spent":true,"hex":"76a914a08eae93007f22668ab5e4a9c83c8cd1c325e3e088ac","addresses":["mv9uLThosiEnGRbVPS7Vhyw6VssbVRsiAw"],"isAddress":true},{"value":"1","n":1,"spent":true,"hex":"a91452724c5178682f70e0ba31c6ec0633755a3b41d987","addresses":["2MzmAKayJmja784jyHvRUW1bXPget1csRRG"],"isAddress":true,"isOwn":true},{"value":"9876","n":2,"spent":true,"hex":"a914e921fc4912a315078f370d959f2c4f7b6d2a683c87","addresses":["2NEVv9LJmAnY99W1pFoc5UJjVdypBqdnvu1"],"isAddress":true}],"blockHash":"0000000076fbbed90fd75b0e18856aa35baa984e9c9d444cf746ad85e94e2997","blockHeight":225493,"confirmations":2,"blockTime":1521515026,"value":"1234567900000","valueIn":"0","fees":"0"}],"usedTokens":2,"tokens":[{"type":"XPUBAddress","name":"2MzmAKayJmja784jyHvRUW1bXPget1csRRG","path":"m/49'/1'/33'/0/0","transfers":2,"decimals":8,"balance":"0","totalReceived":"1","totalSent":"1"},{"type":"XPUBAddress","name":"2MsYfbi6ZdVXLDNrYAQ11ja9Sd3otMk4Pmj","path":"m/49'/1'/33'/0/1","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2MuAZNAjLSo6RLFad2fvHSfgqBD7BoEVy4T","path":"m/49'/1'/33'/0/2","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2NEqKzw3BosGnBE9by5uaDy5QgwjHac4Zbg","path":"m/49'/1'/33'/0/3","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2Mw7vJNC8zUK6VNN4CEjtoTYmuNPLewxZzV","path":"m/49'/1'/33'/0/4","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N1kvo97NFASPXiwephZUxE9PRXunjTxEc4","path":"m/49'/1'/33'/0/5","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2MuWrWMzoBt8VDFNvPmpJf42M1GTUs85fPx","path":"m/49'/1'/33'/0/6","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2MuVZ2Ca6Da9zmYynt49Rx7uikAgubGcymF","path":"m/49'/1'/33'/0/7","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2MzRGWDUmrPP9HwYu4B43QGCTLwoop5cExa","path":"m/49'/1'/33'/0/8","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N5C9EEWJzyBXhpyPHqa3UNed73Amsi5b3L","path":"m/49'/1'/33'/0/9","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2MzNawz2zjwq1L85GDE3YydEJGJYfXxaWkk","path":"m/49'/1'/33'/0/10","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N7NdeuAMgL57WE7QCeV2gTWi2Um8iAu5dA","path":"m/49'/1'/33'/0/11","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N8JQEP6DSHEZHNsSDPA1gHMUq9YFndhkfV","path":"m/49'/1'/33'/0/12","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2Mvbn3YXqKZVpQKugaoQrfjSYPvz76RwZkC","path":"m/49'/1'/33'/0/13","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N8MRNxCfwUY9TSW27X9ooGYtqgrGCfLRHx","path":"m/49'/1'/33'/0/14","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N6HvwrHC113KYZAmCtJ9XJNWgaTcnFunCM","path":"m/49'/1'/33'/0/15","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2NEo3oNyHUoi7rmRWee7wki37jxPWsWCopJ","path":"m/49'/1'/33'/0/16","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2Mzm5KY8qdFbDHsQfy4akXbFvbR3FAwDuVo","path":"m/49'/1'/33'/0/17","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2NGMwftmQCogp6XZNGvgiybz3WZysvsJzqC","path":"m/49'/1'/33'/0/18","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N3fJrrefndYjLGycvFFfYgevpZtcRKCkRD","path":"m/49'/1'/33'/0/19","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N1T7TnHBwfdpBoyw53EGUL7vuJmb2mU6jF","path":"m/49'/1'/33'/0/20","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2MzSBtRWHbBjeUcu3H5VRDqkvz5sfmDxJKo","path":"m/49'/1'/33'/1/0","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2MtShtAJYb1afWduUTwF1SixJjan7urZKke","path":"m/49'/1'/33'/1/1","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N3cP668SeqyBEr9gnB4yQEmU3VyxeRYith","path":"m/49'/1'/33'/1/2","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N6utyMZfPNUb1Bk8oz7p2JqJrXkq83gegu","path":"m/49'/1'/33'/1/3","transfers":1,"decimals":8,"balance":"118641975500","totalReceived":"118641975500","totalSent":"0"},{"type":"XPUBAddress","name":"2NEzatauNhf9kPTwwj6ZfYKjUdy52j4hVUL","path":"m/49'/1'/33'/1/4","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N4RjsDp4LBpkNqyF91aNjgpF9CwDwBkJZq","path":"m/49'/1'/33'/1/5","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N8XygTmQc4NoBBPEy3yybnfCYhsxFtzPDY","path":"m/49'/1'/33'/1/6","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N5BjBomZvb48sccK2vwLMiQ5ETKp1fdPVn","path":"m/49'/1'/33'/1/7","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2MybMwbZRPCGU3SMWPwQCpDkbcQFw5Hbwen","path":"m/49'/1'/33'/1/8","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N7HexL4dyAQc7Th4iqcCW4hZuyiZsLWf74","path":"m/49'/1'/33'/1/9","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2NF6X5FDGWrQj4nQrfP6hA77zB5WAc1DGup","path":"m/49'/1'/33'/1/10","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N4ZRPdvc7BVioBTohy4F6QtxreqcjNj26b","path":"m/49'/1'/33'/1/11","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2Mtfho1rLmevh4qTnkYWxZEFCWteDMtTcUF","path":"m/49'/1'/33'/1/12","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2NFUCphKYvmMcNZRZrF261mRX6iADVB9Qms","path":"m/49'/1'/33'/1/13","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N5kBNMB8qgxE4Y4f8J19fScsE49J4aNvoJ","path":"m/49'/1'/33'/1/14","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2NANWCaefhCKdXMcW8NbZnnrFRDvhJN2wPy","path":"m/49'/1'/33'/1/15","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2NFHw7Yo2Bz8D2wGAYHW9qidbZFLpfJ72qB","path":"m/49'/1'/33'/1/16","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2NBDSsBgy5PpFniLCb1eAFHcSxgxwPSDsZa","path":"m/49'/1'/33'/1/17","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2NDWCSQHogc7sCuc2WoYt9PX2i2i6a5k6dX","path":"m/49'/1'/33'/1/18","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N8vNyDP7iSDjm3BKpXrbDjAxyphqfvnJz8","path":"m/49'/1'/33'/1/19","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N4tFKLurSbMusAyq1tv4tzymVjveAFV1Vb","path":"m/49'/1'/33'/1/20","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2NBx5WwjAr2cH6Yqrp3Vsf957HtRKwDUVdX","path":"m/49'/1'/33'/1/21","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2NBu1seHTaFhQxbcW5L5BkZzqFLGmZqpxsa","path":"m/49'/1'/33'/1/22","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2NCDLoea22jGsXuarfT1n2QyCUh6RFhAPnT","path":"m/49'/1'/33'/1/23","transfers":0,"decimals":8}]}}`,
		},
		{
			name: "websocket getAccountInfo address",
			req: websocketReq{
				Method: "getAccountInfo",
				Params: map[string]interface{}{
					"descriptor": dbtestdata.Addr4,
					"details":    "txids",
				},
			},
			want: `{"id":"3","data":{"page":1,"totalPages":1,"itemsOnPage":25,"address":"2MzmAKayJmja784jyHvRUW1bXPget1csRRG","balance":"0","totalReceived":"1","totalSent":"1","unconfirmedBalance":"0","unconfirmedTxs":0,"txs":2,"txids":["3d90d15ed026dc45e19ffb52875ed18fa9e8012ad123d7f7212176e2b0ebdb71","effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75"]}}`,
		},
		{
			name: "websocket getAccountInfo xpub gap",
			req: websocketReq{
				Method: "getAccountInfo",
				Params: map[string]interface{}{
					"descriptor": dbtestdata.Xpub,
					"details":    "tokens",
					"tokens":     "derived",
					"gap":        10,
				},
			},
			want: `{"id":"4","data":{"address":"upub5E1xjDmZ7Hhej6LPpS8duATdKXnRYui7bDYj6ehfFGzWDZtmCmQkZhc3Zb7kgRLtHWd16QFxyP86JKL3ShZEBFX88aciJ3xyocuyhZZ8g6q","balance":"118641975500","totalReceived":"118641975501","totalSent":"1","unconfirmedBalance":"0","unconfirmedTxs":0,"txs":3,"addrTxCount":3,"usedTokens":2,"tokens":[{"type":"XPUBAddress","name":"2MzmAKayJmja784jyHvRUW1bXPget1csRRG","path":"m/49'/1'/33'/0/0","transfers":2,"decimals":8},{"type":"XPUBAddress","name":"2MsYfbi6ZdVXLDNrYAQ11ja9Sd3otMk4Pmj","path":"m/49'/1'/33'/0/1","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2MuAZNAjLSo6RLFad2fvHSfgqBD7BoEVy4T","path":"m/49'/1'/33'/0/2","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2NEqKzw3BosGnBE9by5uaDy5QgwjHac4Zbg","path":"m/49'/1'/33'/0/3","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2Mw7vJNC8zUK6VNN4CEjtoTYmuNPLewxZzV","path":"m/49'/1'/33'/0/4","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N1kvo97NFASPXiwephZUxE9PRXunjTxEc4","path":"m/49'/1'/33'/0/5","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2MuWrWMzoBt8VDFNvPmpJf42M1GTUs85fPx","path":"m/49'/1'/33'/0/6","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2MuVZ2Ca6Da9zmYynt49Rx7uikAgubGcymF","path":"m/49'/1'/33'/0/7","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2MzRGWDUmrPP9HwYu4B43QGCTLwoop5cExa","path":"m/49'/1'/33'/0/8","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N5C9EEWJzyBXhpyPHqa3UNed73Amsi5b3L","path":"m/49'/1'/33'/0/9","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2MzNawz2zjwq1L85GDE3YydEJGJYfXxaWkk","path":"m/49'/1'/33'/0/10","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2MzSBtRWHbBjeUcu3H5VRDqkvz5sfmDxJKo","path":"m/49'/1'/33'/1/0","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2MtShtAJYb1afWduUTwF1SixJjan7urZKke","path":"m/49'/1'/33'/1/1","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N3cP668SeqyBEr9gnB4yQEmU3VyxeRYith","path":"m/49'/1'/33'/1/2","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N6utyMZfPNUb1Bk8oz7p2JqJrXkq83gegu","path":"m/49'/1'/33'/1/3","transfers":1,"decimals":8},{"type":"XPUBAddress","name":"2NEzatauNhf9kPTwwj6ZfYKjUdy52j4hVUL","path":"m/49'/1'/33'/1/4","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N4RjsDp4LBpkNqyF91aNjgpF9CwDwBkJZq","path":"m/49'/1'/33'/1/5","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N8XygTmQc4NoBBPEy3yybnfCYhsxFtzPDY","path":"m/49'/1'/33'/1/6","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N5BjBomZvb48sccK2vwLMiQ5ETKp1fdPVn","path":"m/49'/1'/33'/1/7","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2MybMwbZRPCGU3SMWPwQCpDkbcQFw5Hbwen","path":"m/49'/1'/33'/1/8","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N7HexL4dyAQc7Th4iqcCW4hZuyiZsLWf74","path":"m/49'/1'/33'/1/9","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2NF6X5FDGWrQj4nQrfP6hA77zB5WAc1DGup","path":"m/49'/1'/33'/1/10","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2N4ZRPdvc7BVioBTohy4F6QtxreqcjNj26b","path":"m/49'/1'/33'/1/11","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2Mtfho1rLmevh4qTnkYWxZEFCWteDMtTcUF","path":"m/49'/1'/33'/1/12","transfers":0,"decimals":8},{"type":"XPUBAddress","name":"2NFUCphKYvmMcNZRZrF261mRX6iADVB9Qms","path":"m/49'/1'/33'/1/13","transfers":0,"decimals":8}]}}`,
		},
		{
			name: "websocket getAccountUtxo",
			req: websocketReq{
				Method: "getAccountUtxo",
				Params: map[string]interface{}{
					"descriptor": dbtestdata.Addr1,
				},
			},
			want: `{"id":"5","data":[{"txid":"00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa3840","vout":0,"value":"100000000","height":225493,"confirmations":2}]}`,
		},
		{
			name: "websocket getAccountUtxo",
			req: websocketReq{
				Method: "getAccountUtxo",
				Params: map[string]interface{}{
					"descriptor": dbtestdata.Addr4,
				},
			},
			want: `{"id":"6","data":[]}`,
		},
		{
			name: "websocket getTransaction",
			req: websocketReq{
				Method: "getTransaction",
				Params: map[string]interface{}{
					"txid": dbtestdata.TxidB2T2,
				},
			},
			want: `{"id":"7","data":{"txid":"3d90d15ed026dc45e19ffb52875ed18fa9e8012ad123d7f7212176e2b0ebdb71","vin":[{"txid":"7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25","n":0,"addresses":["mzB8cYrfRwFRFAGTDzV8LkUQy5BQicxGhX"],"isAddress":true,"value":"317283951061"},{"txid":"effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75","vout":1,"n":1,"addresses":["2MzmAKayJmja784jyHvRUW1bXPget1csRRG"],"isAddress":true,"value":"1"}],"vout":[{"value":"118641975500","n":0,"hex":"a91495e9fbe306449c991d314afe3c3567d5bf78efd287","addresses":["2N6utyMZfPNUb1Bk8oz7p2JqJrXkq83gegu"],"isAddress":true},{"value":"198641975500","n":1,"hex":"76a9143f8ba3fda3ba7b69f5818086e12223c6dd25e3c888ac","addresses":["mmJx9Y8ayz9h14yd9fgCW1bUKoEpkBAquP"],"isAddress":true}],"blockHash":"00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6","blockHeight":225494,"confirmations":1,"blockTime":1521595678,"value":"317283951000","valueIn":"317283951062","fees":"62"}}`,
		},
		{
			name: "websocket getTransaction",
			req: websocketReq{
				Method: "getTransaction",
				Params: map[string]interface{}{
					"txid": "not a tx",
				},
			},
			want: `{"id":"8","data":{"error":{"message":"Transaction 'not a tx' not found"}}}`,
		},
		{
			name: "websocket getTransactionSpecific",
			req: websocketReq{
				Method: "getTransactionSpecific",
				Params: map[string]interface{}{
					"txid": dbtestdata.TxidB2T2,
				},
			},
			want: `{"id":"9","data":{"hex":"","txid":"3d90d15ed026dc45e19ffb52875ed18fa9e8012ad123d7f7212176e2b0ebdb71","version":0,"locktime":0,"vin":[{"coinbase":"","txid":"7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25","vout":0,"scriptSig":{"hex":""},"sequence":0,"addresses":null},{"coinbase":"","txid":"effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75","vout":1,"scriptSig":{"hex":""},"sequence":0,"addresses":null}],"vout":[{"ValueSat":118641975500,"value":0,"n":0,"scriptPubKey":{"hex":"a91495e9fbe306449c991d314afe3c3567d5bf78efd287","addresses":null}},{"ValueSat":198641975500,"value":0,"n":1,"scriptPubKey":{"hex":"76a9143f8ba3fda3ba7b69f5818086e12223c6dd25e3c888ac","addresses":null}}],"confirmations":1,"time":1521595678,"blocktime":1521595678,"vsize":400}}`,
		},
		{
			name: "websocket estimateFee",
			req: websocketReq{
				Method: "estimateFee",
				Params: map[string]interface{}{
					"blocks": []int{2, 5, 10, 20},
					"specific": map[string]interface{}{
						"conservative": false,
						"txsize":       1234,
					},
				},
			},
			want: `{"id":"10","data":[{"feePerTx":"246","feePerUnit":"199"},{"feePerTx":"616","feePerUnit":"499"},{"feePerTx":"1233","feePerUnit":"999"},{"feePerTx":"2467","feePerUnit":"1999"}]}`,
		},
		{
			name: "websocket estimateFee second time, from cache",
			req: websocketReq{
				Method: "estimateFee",
				Params: map[string]interface{}{
					"blocks": []int{2, 5, 10, 20},
					"specific": map[string]interface{}{
						"conservative": false,
						"txsize":       1234,
					},
				},
			},
			want: `{"id":"11","data":[{"feePerTx":"246","feePerUnit":"199"},{"feePerTx":"616","feePerUnit":"499"},{"feePerTx":"1233","feePerUnit":"999"},{"feePerTx":"2467","feePerUnit":"1999"}]}`,
		},
		{
			name: "websocket sendTransaction",
			req: websocketReq{
				Method: "sendTransaction",
				Params: map[string]interface{}{
					"hex": "123456",
				},
			},
			want: `{"id":"12","data":{"result":"9876"}}`,
		},
		{
			name: "websocket subscribeNewBlock",
			req: websocketReq{
				Method: "subscribeNewBlock",
			},
			want: `{"id":"13","data":{"subscribed":true}}`,
		},
		{
			name: "websocket unsubscribeNewBlock",
			req: websocketReq{
				Method: "unsubscribeNewBlock",
			},
			want: `{"id":"14","data":{"subscribed":false}}`,
		},
		{
			name: "websocket subscribeAddresses",
			req: websocketReq{
				Method: "subscribeAddresses",
				Params: map[string]interface{}{
					"addresses": []string{dbtestdata.Addr1, dbtestdata.Addr2},
				},
			},
			want: `{"id":"15","data":{"subscribed":true}}`,
		},
		{
			name: "websocket unsubscribeAddresses",
			req: websocketReq{
				Method: "unsubscribeAddresses",
			},
			want: `{"id":"16","data":{"subscribed":false}}`,
		},
		{
			name: "websocket ping",
			req: websocketReq{
				Method: "ping",
			},
			want: `{"id":"17","data":{}}`,
		},
		{
			name: "websocket getCurrentFiatRates all currencies",
			req: websocketReq{
				Method: "getCurrentFiatRates",
				Params: map[string]interface{}{
					"currencies": []string{},
				},
			},
			want: `{"id":"18","data":{"ts":1574380800,"rates":{"eur":7134.1,"usd":7914.5}}}`,
		},
		{
			name: "websocket getCurrentFiatRates usd",
			req: websocketReq{
				Method: "getCurrentFiatRates",
				Params: map[string]interface{}{
					"currencies": []string{"usd"},
				},
			},
			want: `{"id":"19","data":{"ts":1574380800,"rates":{"usd":7914.5}}}`,
		},
		{
			name: "websocket getCurrentFiatRates eur",
			req: websocketReq{
				Method: "getCurrentFiatRates",
				Params: map[string]interface{}{
					"currencies": []string{"eur"},
				},
			},
			want: `{"id":"20","data":{"ts":1574380800,"rates":{"eur":7134.1}}}`,
		},
		{
			name: "websocket getCurrentFiatRates incorrect currency",
			req: websocketReq{
				Method: "getCurrentFiatRates",
				Params: map[string]interface{}{
					"currencies": []string{"does-not-exist"},
				},
			},
			want: `{"id":"21","data":{"error":{"message":"No tickers found!"}}}`,
		},
		{
			name: "websocket getFiatRatesForTimestamps missing date",
			req: websocketReq{
				Method: "getFiatRatesForTimestamps",
				Params: map[string]interface{}{
					"currencies": []string{"usd"},
				},
			},
			want: `{"id":"22","data":{"error":{"message":"No timestamps provided"}}}`,
		},
		{
			name: "websocket getFiatRatesForTimestamps incorrect date",
			req: websocketReq{
				Method: "getFiatRatesForTimestamps",
				Params: map[string]interface{}{
					"currencies": []string{"usd"},
					"timestamps": []string{"yesterday"},
				},
			},
			want: `{"id":"23","data":{"error":{"message":"json: cannot unmarshal string into Go struct field WsFiatRatesForTimestampsReq.timestamps of type int64"}}}`,
		},
		{
			name: "websocket getFiatRatesForTimestamps empty currency",
			req: websocketReq{
				Method: "getFiatRatesForTimestamps",
				Params: map[string]interface{}{
					"timestamps": []int64{7885693815},
					"currencies": []string{""},
				},
			},
			want: `{"id":"24","data":{"tickers":[{"ts":7885693815,"rates":{}}]}}`,
		},
		{
			name: "websocket getFiatRatesForTimestamps incorrect (future) date",
			req: websocketReq{
				Method: "getFiatRatesForTimestamps",
				Params: map[string]interface{}{
					"currencies": []string{"usd"},
					"timestamps": []int64{7885693815},
				},
			},
			want: `{"id":"25","data":{"tickers":[{"ts":7885693815,"rates":{"usd":-1}}]}}`,
		},
		{
			name: "websocket getFiatRatesForTimestamps exact date",
			req: websocketReq{
				Method: "getFiatRatesForTimestamps",
				Params: map[string]interface{}{
					"currencies": []string{"usd"},
					"timestamps": []int64{1574380800},
				},
			},
			want: `{"id":"26","data":{"tickers":[{"ts":1574380800,"rates":{"usd":7914.5}}]}}`,
		},
		{
			name: "websocket getFiatRatesForTimestamps closest date, eur",
			req: websocketReq{
				Method: "getFiatRatesForTimestamps",
				Params: map[string]interface{}{
					"currencies": []string{"eur"},
					"timestamps": []int64{1521507600},
				},
			},
			want: `{"id":"27","data":{"tickers":[{"ts":1521590400,"rates":{"eur":1301}}]}}`,
		},
		{
			name: "websocket getFiatRatesForTimestamps multiple timestamps usd",
			req: websocketReq{
				Method: "getFiatRatesForTimestamps",
				Params: map[string]interface{}{
					"currencies": []string{"usd"},
					"timestamps": []int64{1570346615, 1574346615},
				},
			},
			want: `{"id":"28","data":{"tickers":[{"ts":1574294400,"rates":{"usd":7814.5}},{"ts":1574380800,"rates":{"usd":7914.5}}]}}`,
		},
		{
			name: "websocket getFiatRatesForTimestamps multiple timestamps eur",
			req: websocketReq{
				Method: "getFiatRatesForTimestamps",
				Params: map[string]interface{}{
					"currencies": []string{"eur"},
					"timestamps": []int64{1570346615, 1574346615},
				},
			},
			want: `{"id":"29","data":{"tickers":[{"ts":1574294400,"rates":{"eur":7100}},{"ts":1574380800,"rates":{"eur":7134.1}}]}}`,
		},
		{
			name: "websocket getFiatRatesForTimestamps multiple timestamps with an error",
			req: websocketReq{
				Method: "getFiatRatesForTimestamps",
				Params: map[string]interface{}{
					"currencies": []string{"usd"},
					"timestamps": []int64{1570346615, 1574346615, 2000000000},
				},
			},
			want: `{"id":"30","data":{"tickers":[{"ts":1574294400,"rates":{"usd":7814.5}},{"ts":1574380800,"rates":{"usd":7914.5}},{"ts":2000000000,"rates":{"usd":-1}}]}}`,
		},
		{
			name: "websocket getFiatRatesForTimestamps multiple errors",
			req: websocketReq{
				Method: "getFiatRatesForTimestamps",
				Params: map[string]interface{}{
					"currencies": []string{"usd"},
					"timestamps": []int64{7832854800, 2000000000},
				},
			},
			want: `{"id":"31","data":{"tickers":[{"ts":7832854800,"rates":{"usd":-1}},{"ts":2000000000,"rates":{"usd":-1}}]}}`,
		},
		{
			name: "websocket getTickersList",
			req: websocketReq{
				Method: "getFiatRatesTickersList",
				Params: map[string]interface{}{
					"timestamp": 1570346615,
				},
			},
			want: `{"id":"32","data":{"ts":1574294400,"available_currencies":["eur","usd"]}}`,
		},
		{
			name: "websocket getBalanceHistory Addr2",
			req: websocketReq{
				Method: "getBalanceHistory",
				Params: map[string]interface{}{
					"descriptor": "mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz",
				},
			},
			want: `{"id":"33","data":[{"time":1521514800,"txs":1,"received":"24690","sent":"0","sentToSelf":"0","rates":{"eur":1301,"usd":2001}},{"time":1521594000,"txs":1,"received":"0","sent":"12345","sentToSelf":"0","rates":{"eur":1302,"usd":2002}}]}`,
		},
		{
			name: "websocket getBalanceHistory xpub",
			req: websocketReq{
				Method: "getBalanceHistory",
				Params: map[string]interface{}{
					"descriptor": dbtestdata.Xpub,
				},
			},
			want: `{"id":"34","data":[{"time":1521514800,"txs":1,"received":"1","sent":"0","sentToSelf":"0","rates":{"eur":1301,"usd":2001}},{"time":1521594000,"txs":1,"received":"118641975500","sent":"1","sentToSelf":"118641975500","rates":{"eur":1302,"usd":2002}}]}`,
		},
		{
			name: "websocket getBalanceHistory xpub from=1521504000&to=1521590400 currencies=[usd]",
			req: websocketReq{
				Method: "getBalanceHistory",
				Params: map[string]interface{}{
					"descriptor": dbtestdata.Xpub,
					"from":       1521504000,
					"to":         1521590400,
					"currencies": []string{"usd"},
				},
			},
			want: `{"id":"35","data":[{"time":1521514800,"txs":1,"received":"1","sent":"0","sentToSelf":"0","rates":{"usd":2001}}]}`,
		},
		{
			name: "websocket getBalanceHistory xpub from=1521504000&to=1521590400 currencies=[usd, eur, incorrect]",
			req: websocketReq{
				Method: "getBalanceHistory",
				Params: map[string]interface{}{
					"descriptor": dbtestdata.Xpub,
					"from":       1521504000,
					"to":         1521590400,
					"currencies": []string{"usd", "eur", "incorrect"},
				},
			},
			want: `{"id":"36","data":[{"time":1521514800,"txs":1,"received":"1","sent":"0","sentToSelf":"0","rates":{"eur":1301,"incorrect":-1,"usd":2001}}]}`,
		},
		{
			name: "websocket getBalanceHistory xpub from=1521504000&to=1521590400 currencies=[]",
			req: websocketReq{
				Method: "getBalanceHistory",
				Params: map[string]interface{}{
					"descriptor": dbtestdata.Xpub,
					"from":       1521504000,
					"to":         1521590400,
					"currencies": []string{},
				},
			},
			want: `{"id":"37","data":[{"time":1521514800,"txs":1,"received":"1","sent":"0","sentToSelf":"0","rates":{"eur":1301,"usd":2001}}]}`,
		},
		{
			name: "websocket subscribeNewTransaction",
			req: websocketReq{
				Method: "subscribeNewTransaction",
			},
			want: `{"id":"38","data":{"subscribed":false,"message":"subscribeNewTransaction not enabled, use -enablesubnewtx flag to enable."}}`,
		},
		{
			name: "websocket unsubscribeNewTransaction",
			req: websocketReq{
				Method: "unsubscribeNewTransaction",
			},
			want: `{"id":"39","data":{"subscribed":false,"message":"unsubscribeNewTransaction not enabled, use -enablesubnewtx flag to enable."}}`,
		},
		{
			name: "websocket getBlock",
			req: websocketReq{
				Method: "getBlock",
				Params: map[string]interface{}{
					"id": "00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6",
				},
			},
			want: `{"id":"40","data":{"error":{"message":"Not supported"}}}`,
		},
		{
			name: "websocket getMempoolFilters",
			req: websocketReq{
				Method: "getMempoolFilters",
				Params: map[string]interface{}{
					"scriptType": "",
				},
			},
			want: `{"id":"41","data":{}}`,
		},
		{
			name: "websocket getMempoolFilters invalid type",
			req: websocketReq{
				Method: "getMempoolFilters",
				Params: map[string]interface{}{
					"scriptType": "invalid",
				},
			},
			want: `{"id":"42","data":{"error":{"message":"Unsupported script filter invalid"}}}`,
		},
	}

	// send all requests at once
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.req.ID = strconv.Itoa(i)
			err = s.WriteJSON(tt.req)
			if err != nil {
				t.Fatal(err)
			}
		})
	}

	// wait for all responses
	done := make(chan struct{})

	go func() {
		defer close(done)
		for i := 0; i < len(tests); i++ {
			_, message, err := s.ReadMessage()
			if err != nil {
				t.Fatal(err)
			}
			var resp websocketResp
			err = json.Unmarshal(message, &resp)
			if err != nil {
				t.Fatal(err)
			}
			id, err := strconv.Atoi(resp.ID)
			if err != nil {
				t.Fatal(err)
			}
			got := strings.TrimSpace(string(message))
			if got != tests[id].want {
				t.Errorf("%s: got %v, want %v", tests[id].name, got, tests[id].want)
			} else {
				tests[id].want = "already checked, should not check twice"
			}
		}
	}()

	select {
	case <-done:
		break
	case <-time.After(time.Second * 10):
		t.Error("Timeout while waiting for websocket responses")
	}
}

// fixedTimeNow returns always 2022-09-15 12:43:56 UTC
func fixedTimeNow() time.Time {
	return time.Date(2022, 9, 15, 12, 43, 56, 0, time.UTC)
}

func setupChain(t *testing.T) (bchain.BlockChainParser, bchain.BlockChain) {
	timeNow = fixedTimeNow
	parser := btc.NewBitcoinParser(
		btc.GetChainParams("test"),
		&btc.Configuration{
			BlockAddressesToKeep:  1,
			XPubMagic:             70617039,
			XPubMagicSegwitP2sh:   71979618,
			XPubMagicSegwitNative: 73342198,
			Slip44:                1,
		})

	chain, err := dbtestdata.NewFakeBlockChain(parser)
	if err != nil {
		glog.Fatal("fakechain: ", err)
	}
	return parser, chain
}

func Test_PublicServer_BitcoinType(t *testing.T) {
	parser, chain := setupChain(t)

	s, dbpath := setupPublicHTTPServer(parser, chain, t, false)
	defer closeAndDestroyPublicServer(t, s, dbpath)
	s.ConnectFullPublicInterface()
	// take the handler of the public server and pass it to the test server
	ts := httptest.NewServer(s.https.Handler)
	defer ts.Close()

	httpTestsBitcoinType(t, ts)
	socketioTestsBitcoinType(t, ts)
	websocketTestsBitcoinType(t, ts)
}

func httpTestsExtendedIndex(t *testing.T, ts *httptest.Server) {
	tests := []struct {
		name        string
		r           *http.Request
		status      int
		contentType string
		body        []string
	}{
		{
			name:        "apiTx v2",
			r:           newGetRequest(ts.URL + "/api/v2/tx/7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"txid":"7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25","vin":[{"txid":"effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75","n":0,"addresses":["mv9uLThosiEnGRbVPS7Vhyw6VssbVRsiAw"],"isAddress":true,"value":"1234567890123"},{"txid":"00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa3840","vout":1,"n":1,"addresses":["mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz"],"isAddress":true,"value":"12345"}],"vout":[{"value":"317283951061","n":0,"spent":true,"spentTxId":"3d90d15ed026dc45e19ffb52875ed18fa9e8012ad123d7f7212176e2b0ebdb71","spentHeight":225494,"hex":"76a914ccaaaf374e1b06cb83118453d102587b4273d09588ac","addresses":["mzB8cYrfRwFRFAGTDzV8LkUQy5BQicxGhX"],"isAddress":true},{"value":"917283951061","n":1,"hex":"76a9148d802c045445df49613f6a70ddd2e48526f3701f88ac","addresses":["mtR97eM2HPWVM6c8FGLGcukgaHHQv7THoL"],"isAddress":true},{"value":"0","n":2,"hex":"6a072020f1686f6a20","addresses":["OP_RETURN 2020f1686f6a20"],"isAddress":false}],"blockHash":"00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6","blockHeight":225494,"confirmations":1,"blockTime":1521595678,"value":"1234567902122","valueIn":"1234567902468","fees":"346"}`,
			},
		},
		{
			name:        "apiAddress v2 details=txs",
			r:           newGetRequest(ts.URL + "/api/v2/address/mv9uLThosiEnGRbVPS7Vhyw6VssbVRsiAw?details=txs"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"page":1,"totalPages":1,"itemsOnPage":1000,"address":"mv9uLThosiEnGRbVPS7Vhyw6VssbVRsiAw","balance":"0","totalReceived":"1234567890123","totalSent":"1234567890123","unconfirmedBalance":"0","unconfirmedTxs":0,"txs":2,"transactions":[{"txid":"7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25","vin":[{"txid":"effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75","n":0,"addresses":["mv9uLThosiEnGRbVPS7Vhyw6VssbVRsiAw"],"isAddress":true,"isOwn":true,"value":"1234567890123"},{"txid":"00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa3840","vout":1,"n":1,"addresses":["mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz"],"isAddress":true,"value":"12345"}],"vout":[{"value":"317283951061","n":0,"spent":true,"spentTxId":"3d90d15ed026dc45e19ffb52875ed18fa9e8012ad123d7f7212176e2b0ebdb71","spentHeight":225494,"hex":"76a914ccaaaf374e1b06cb83118453d102587b4273d09588ac","addresses":["mzB8cYrfRwFRFAGTDzV8LkUQy5BQicxGhX"],"isAddress":true},{"value":"917283951061","n":1,"hex":"76a9148d802c045445df49613f6a70ddd2e48526f3701f88ac","addresses":["mtR97eM2HPWVM6c8FGLGcukgaHHQv7THoL"],"isAddress":true},{"value":"0","n":2,"hex":"6a072020f1686f6a20","addresses":["OP_RETURN 2020f1686f6a20"],"isAddress":false}],"blockHash":"00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6","blockHeight":225494,"confirmations":1,"blockTime":1521595678,"value":"1234567902122","valueIn":"1234567902468","fees":"346"},{"txid":"effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75","vin":[],"vout":[{"value":"1234567890123","n":0,"spent":true,"spentTxId":"7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25","spentHeight":225494,"hex":"76a914a08eae93007f22668ab5e4a9c83c8cd1c325e3e088ac","addresses":["mv9uLThosiEnGRbVPS7Vhyw6VssbVRsiAw"],"isAddress":true,"isOwn":true},{"value":"1","n":1,"spent":true,"spentTxId":"3d90d15ed026dc45e19ffb52875ed18fa9e8012ad123d7f7212176e2b0ebdb71","spentIndex":1,"spentHeight":225494,"hex":"a91452724c5178682f70e0ba31c6ec0633755a3b41d987","addresses":["2MzmAKayJmja784jyHvRUW1bXPget1csRRG"],"isAddress":true},{"value":"9876","n":2,"spent":true,"spentTxId":"05e2e48aeabdd9b75def7b48d756ba304713c2aba7b522bf9dbc893fc4231b07","spentHeight":225494,"hex":"a914e921fc4912a315078f370d959f2c4f7b6d2a683c87","addresses":["2NEVv9LJmAnY99W1pFoc5UJjVdypBqdnvu1"],"isAddress":true}],"blockHash":"0000000076fbbed90fd75b0e18856aa35baa984e9c9d444cf746ad85e94e2997","blockHeight":225493,"confirmations":2,"blockTime":1521515026,"value":"1234567900000","valueIn":"0","fees":"0"}]}`,
			},
		},
		{
			name:        "apiGetBlock",
			r:           newGetRequest(ts.URL + "/api/v2/block/225493"),
			status:      http.StatusOK,
			contentType: "application/json; charset=utf-8",
			body: []string{
				`{"page":1,"totalPages":1,"itemsOnPage":1000,"hash":"0000000076fbbed90fd75b0e18856aa35baa984e9c9d444cf746ad85e94e2997","nextBlockHash":"00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6","height":225493,"confirmations":2,"size":1234567,"time":1521515026,"version":0,"merkleRoot":"","nonce":"","bits":"","difficulty":"","txCount":2,"txs":[{"txid":"00b2c06055e5e90e9c82bd4181fde310104391a7fa4f289b1704e5d90caa3840","vin":[],"vout":[{"value":"100000000","n":0,"addresses":["mfcWp7DB6NuaZsExybTTXpVgWz559Np4Ti"],"isAddress":true},{"value":"12345","n":1,"spent":true,"spentTxId":"7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25","spentIndex":1,"spentHeight":225494,"addresses":["mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz"],"isAddress":true},{"value":"12345","n":2,"addresses":["mtGXQvBowMkBpnhLckhxhbwYK44Gs9eEtz"],"isAddress":true}],"blockHash":"0000000076fbbed90fd75b0e18856aa35baa984e9c9d444cf746ad85e94e2997","blockHeight":225493,"confirmations":2,"blockTime":1521515026,"value":"100024690","valueIn":"0","fees":"0"},{"txid":"effd9ef509383d536b1c8af5bf434c8efbf521a4f2befd4022bbd68694b4ac75","vin":[],"vout":[{"value":"1234567890123","n":0,"spent":true,"spentTxId":"7c3be24063f268aaa1ed81b64776798f56088757641a34fb156c4f51ed2e9d25","spentHeight":225494,"addresses":["mv9uLThosiEnGRbVPS7Vhyw6VssbVRsiAw"],"isAddress":true},{"value":"1","n":1,"spent":true,"spentTxId":"3d90d15ed026dc45e19ffb52875ed18fa9e8012ad123d7f7212176e2b0ebdb71","spentIndex":1,"spentHeight":225494,"addresses":["2MzmAKayJmja784jyHvRUW1bXPget1csRRG"],"isAddress":true},{"value":"9876","n":2,"spent":true,"spentTxId":"05e2e48aeabdd9b75def7b48d756ba304713c2aba7b522bf9dbc893fc4231b07","spentHeight":225494,"addresses":["2NEVv9LJmAnY99W1pFoc5UJjVdypBqdnvu1"],"isAddress":true}],"blockHash":"0000000076fbbed90fd75b0e18856aa35baa984e9c9d444cf746ad85e94e2997","blockHeight":225493,"confirmations":2,"blockTime":1521515026,"value":"1234567900000","valueIn":"0","fees":"0"}]}`,
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
					t.Errorf("got %v, want to contain %v", b, c)
					break
				}
			}
		})
	}
}

func Test_PublicServer_BitcoinType_ExtendedIndex(t *testing.T) {
	parser, chain := setupChain(t)

	s, dbpath := setupPublicHTTPServer(parser, chain, t, true)
	defer closeAndDestroyPublicServer(t, s, dbpath)
	s.ConnectFullPublicInterface()
	// take the handler of the public server and pass it to the test server
	ts := httptest.NewServer(s.https.Handler)
	defer ts.Close()

	httpTestsExtendedIndex(t, ts)
}
