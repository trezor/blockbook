// +build unittest

package fiat

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"blockbook/common"
	"blockbook/db"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/golang/glog"
	"github.com/martinboehm/btcutil/chaincfg"
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
	return d, is, tmp
}

func closeAndDestroyRocksDB(t *testing.T, db *db.RocksDB, dbpath string) {
	// destroy db
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(dbpath)
}

type testBitcoinParser struct {
	*btc.BitcoinParser
}

func bitcoinTestnetParser() *btc.BitcoinParser {
	return btc.NewBitcoinParser(
		btc.GetChainParams("test"),
		&btc.Configuration{BlockAddressesToKeep: 1})
}

// getFiatRatesMockData reads a stub JSON response from a file and returns its content as string
func getFiatRatesMockData(dateParam string) (string, error) {
	var filename string
	if dateParam == "current" {
		filename = "fiat/mock_data/current.json"
	} else {
		filename = "fiat/mock_data/" + dateParam + ".json"
	}
	mockFile, err := os.Open(filename)
	if err != nil {
		glog.Errorf("Cannot open file %v", filename)
		return "", err
	}
	b, err := ioutil.ReadAll(mockFile)
	if err != nil {
		glog.Errorf("Cannot read file %v", filename)
		return "", err
	}
	return string(b), nil
}

func TestFiatRates(t *testing.T) {
	d, _, tmp := setupRocksDB(t, &testBitcoinParser{
		BitcoinParser: bitcoinTestnetParser(),
	})
	defer closeAndDestroyRocksDB(t, d, tmp)

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		var mockData string

		if r.URL.Path == "/ping" {
			w.WriteHeader(200)
		} else if r.URL.Path == "/coins/bitcoin/history" {
			date := r.URL.Query()["date"][0]
			mockData, err = getFiatRatesMockData(date) // get stub rates by date
		} else if r.URL.Path == "/coins/bitcoin" {
			mockData, err = getFiatRatesMockData("current") // get "latest" stub rates
		} else {
			t.Errorf("Unknown URL path: %v", r.URL.Path)
		}

		if err != nil {
			t.Errorf("Error loading stub data: %v", err)
		}
		fmt.Fprintln(w, mockData)
	}))
	defer mockServer.Close()

	// real CoinGecko API
	//configJSON := `{"fiat_rates": "coingecko", "fiat_rates_params": "{\"url\": \"https://api.coingecko.com/api/v3\", \"coin\": \"bitcoin\", \"periodSeconds\": 60}"}`

	// mocked CoinGecko API
	configJSON := `{"fiat_rates": "coingecko", "fiat_rates_params": "{\"url\": \"` + mockServer.URL + `\", \"coin\": \"bitcoin\", \"periodSeconds\": 60}"}`

	type fiatRatesConfig struct {
		FiatRates       string `json:"fiat_rates"`
		FiatRatesParams string `json:"fiat_rates_params"`
	}

	var config fiatRatesConfig
	err := json.Unmarshal([]byte(configJSON), &config)
	if err != nil {
		t.Errorf("Error parsing config: %v", err)
	}

	if config.FiatRates == "" || config.FiatRatesParams == "" {
		t.Errorf("Error parsing FiatRates config - empty parameter")
		return
	}
	fiatRates, err := NewFiatRatesDownloader(d, config.FiatRatesParams, true)
	if err != nil {
		t.Errorf("FiatRates init error: %v\n", err)
	}
	if config.FiatRates == "coingecko" {
		err = fiatRates.Run()
		if err != nil {
			t.Errorf("Error running FiatRatesDownloader: %v", err)
		}
	}
}
