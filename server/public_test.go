// build unittest

package server

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"blockbook/common"
	"blockbook/db"
	"blockbook/tests/dbtestdata"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/golang/glog"
	"github.com/jakm/btcutil/chaincfg"
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
	is, err := d.LoadInternalState("btc-testnet")
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
	is.Coin = "Testnet"
	is.CoinLabel = "Bitcoin Testnet"
	is.CoinShortcut = "TEST"

	metrics, err := common.GetMetrics("Testnet")
	if err != nil {
		glog.Fatal("metrics: ", err)
	}

	chain, err := dbtestdata.NewFakeBlockChain(parser)
	if err != nil {
		glog.Fatal("fakechain: ", err)
	}

	txCache, err := db.NewTxCache(d, chain, metrics, is, true)
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

func newGetRequest(url string, body io.Reader) *http.Request {
	r, err := http.NewRequest("GET", url, body)
	if err != nil {
		glog.Fatal(err)
	}
	return r
}

func Test_PublicServer_UTXO(t *testing.T) {
	s, dbpath := setupPublicHTTPServer(t)
	defer closeAndDestroyPublicServer(t, s, dbpath)
	s.ConnectFullPublicInterface()
	// take the handler of the public server and pass it to the test server
	ts := httptest.NewServer(s.https.Handler)
	defer ts.Close()

	tests := []struct {
		name        string
		r           *http.Request
		status      int
		contentType string
		body        []string
	}{
		{
			name:        "explorerTx",
			r:           newGetRequest(ts.URL+"/tx/fdd824a780cbb718eeb766eb05d83fdefc793a27082cd5e67f856d69798cf7db", nil),
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body: []string{
				`<a href="/" class="nav-link">Bitcoin Testnet Explorer</a>`,
				`<h1>Transaction</h1>`,
				`<span class="data">fdd824a780cbb718eeb766eb05d83fdefc793a27082cd5e67f856d69798cf7db</span>`,
				`td class="data">0 TEST</td>`,
				`<a href="/address/mzVznVsCHkVHX9UN8WPFASWUUHtxnNn4Jj">mzVznVsCHkVHX9UN8WPFASWUUHtxnNn4Jj</a>`,
				`13.60030331 TEST`,
				`<td><span class="float-left">No Inputs (Newly Generated Coins)</span></td>`,
				`</html>`,
			},
		},
		// explorerSpendingTx
		// explorerBlocks
		{
			name:        "explorerBlock",
			r:           newGetRequest(ts.URL+"/block/225494", nil),
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body: []string{
				`<a href="/" class="nav-link">Bitcoin Testnet Explorer</a>`,
				`<h1>Block 225494</h1>`,
				`<span class="data">00000000eb0443fd7dc4a1ed5c686a8e995057805f9a161d9a5a77a95e72b7b6</span>`,
				`<td class="data">4</td>`, // number of transactions
				`<a href="/address/mtR97eM2HPWVM6c8FGLGcukgaHHQv7THoL">mtR97eM2HPWVM6c8FGLGcukgaHHQv7THoL</a>`,
				`9172.83951061 TEST`,
				`<a href="/tx/fdd824a780cbb718eeb766eb05d83fdefc793a27082cd5e67f856d69798cf7db">fdd824a780cbb718eeb766eb05d83fdefc793a27082cd5e67f856d69798cf7db</a>`,
				`</html>`,
			},
		},
		{
			name:        "explorerIndex",
			r:           newGetRequest(ts.URL+"/", nil),
			status:      http.StatusOK,
			contentType: "text/html; charset=utf-8",
			body: []string{
				`<a href="/" class="nav-link">Bitcoin Testnet Explorer</a>`,
				`<h1>Application status</h1>`,
				`<h3 class="bg-warning text-white" style="padding: 20px;">Synchronization with backend is disabled, the state of index is not up to date.</h3>`,
				`<a href="/block/225494">225494</a>`,
				`<td class="data">/Satoshi:0.16.3/</td>`,
				`</html>`,
			},
		},
		// explorerSearch
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
