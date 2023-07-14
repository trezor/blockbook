//go:build unittest

package fiat

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/golang/glog"
	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/db"
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
	tmp, err := os.MkdirTemp("", "testdb")
	if err != nil {
		t.Fatal(err)
	}
	d, err := db.NewRocksDB(tmp, 100000, -1, parser, nil, false)
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
func getFiatRatesMockData(name string) (string, error) {
	var filename string
	filename = "fiat/mock_data/" + name + ".json"
	mockFile, err := os.Open(filename)
	if err != nil {
		glog.Errorf("Cannot open file %v", filename)
		return "", err
	}
	b, err := io.ReadAll(mockFile)
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
		} else if r.URL.Path == "/coins/list" {
			mockData, err = getFiatRatesMockData("coinlist")
		} else if r.URL.Path == "/simple/supported_vs_currencies" {
			mockData, err = getFiatRatesMockData("vs_currencies")
		} else if r.URL.Path == "/simple/price" {
			if r.URL.Query().Get("ids") == "ethereum" {
				mockData, err = getFiatRatesMockData("simpleprice_base")
			} else {
				mockData, err = getFiatRatesMockData("simpleprice_tokens")
			}
		} else if r.URL.Path == "/coins/ethereum/market_chart" {
			vsCurrency := r.URL.Query().Get("vs_currency")
			if vsCurrency == "usd" {
				days := r.URL.Query().Get("days")
				if days == "max" {
					mockData, err = getFiatRatesMockData("market_chart_eth_usd_max")
				} else {
					mockData, err = getFiatRatesMockData("market_chart_eth_usd_1")
				}
			} else {
				mockData, err = getFiatRatesMockData("market_chart_eth_other")
			}
		} else if r.URL.Path == "/coins/vendit/market_chart" || r.URL.Path == "/coins/ethereum-cash-token/market_chart" {
			mockData, err = getFiatRatesMockData("market_chart_token_other")
		} else {
			t.Fatalf("Unknown URL path: %v", r.URL.Path)
		}

		if err != nil {
			t.Fatalf("Error loading stub data: %v", err)
		}
		fmt.Fprintln(w, mockData)
	}))
	defer mockServer.Close()

	// mocked CoinGecko API
	configJSON := `{"fiat_rates": "coingecko", "fiat_rates_params": "{\"url\": \"` + mockServer.URL + `\", \"coin\": \"ethereum\",\"platformIdentifier\":\"ethereum\",\"platformVsCurrency\": \"eth\",\"periodSeconds\": 60}"}`

	fiatRates, err := NewFiatRates(d, []byte(configJSON), nil, nil)
	if err != nil {
		t.Fatalf("FiatRates init error: %v", err)
	}

	// get current tickers
	currentTickers, err := fiatRates.downloader.CurrentTickers()
	if err != nil {
		t.Fatalf("Error in CurrentTickers: %v", err)
		return
	}
	if currentTickers == nil {
		t.Fatalf("CurrentTickers returned nil value")
		return
	}

	wantCurrentTickers := common.CurrencyRatesTicker{
		Rates: map[string]float32{
			"aed": 8447.1,
			"ars": 268901,
			"aud": 3314.36,
			"btc": 0.07531005,
			"eth": 1,
			"eur": 2182.99,
			"ltc": 29.097696,
			"usd": 2299.72,
		},
		TokenRates: map[string]float32{
			"0x5e9997684d061269564f94e5d11ba6ce6fa9528c": 5.58195e-07,
			"0x906710835d1ae85275eb770f06873340ca54274b": 1.39852e-10,
		},
		Timestamp: currentTickers.Timestamp,
	}
	if !reflect.DeepEqual(currentTickers, &wantCurrentTickers) {
		t.Fatalf("CurrentTickers() = %v, want %v", *currentTickers, wantCurrentTickers)
	}

	ticker, err := fiatRates.db.FiatRatesFindLastTicker("usd", "")
	if err != nil {
		t.Fatalf("FiatRatesFindLastTicker failed with error: %v", err)
	}
	if ticker != nil {
		t.Fatalf("FiatRatesFindLastTicker found unexpected data")
	}

	// update historical tickers for the first time
	err = fiatRates.downloader.UpdateHistoricalTickers()
	if err != nil {
		t.Fatalf("UpdateHistoricalTickers 1st pass failed with error: %v", err)
	}
	err = fiatRates.downloader.UpdateHistoricalTokenTickers()
	if err != nil {
		t.Fatalf("UpdateHistoricalTokenTickers 1st pass failed with error: %v", err)
	}

	ticker, err = fiatRates.db.FiatRatesFindLastTicker("usd", "")
	if err != nil || ticker == nil {
		t.Fatalf("FiatRatesFindLastTicker failed with error: %v", err)
	}
	wantTicker := common.CurrencyRatesTicker{
		Rates: map[string]float32{
			"aed": 241272.48,
			"ars": 241272.48,
			"aud": 241272.48,
			"btc": 241272.48,
			"eth": 241272.48,
			"eur": 241272.48,
			"ltc": 241272.48,
			"usd": 1794.5397,
		},
		TokenRates: map[string]float32{
			"0x5e9997684d061269564f94e5d11ba6ce6fa9528c": 4.161734e+07,
			"0x906710835d1ae85275eb770f06873340ca54274b": 4.161734e+07,
		},
		Timestamp: time.Unix(1654732800, 0).UTC(),
	}
	if !reflect.DeepEqual(ticker, &wantTicker) {
		t.Fatalf("UpdateHistoricalTickers(usd) 1st pass = %v, want %v", *ticker, wantTicker)
	}

	ticker, err = fiatRates.db.FiatRatesFindLastTicker("eur", "")
	if err != nil || ticker == nil {
		t.Fatalf("FiatRatesFindLastTicker failed with error: %v", err)
	}
	wantTicker = common.CurrencyRatesTicker{
		Rates: map[string]float32{
			"aed": 240402.97,
			"ars": 240402.97,
			"aud": 240402.97,
			"btc": 240402.97,
			"eth": 240402.97,
			"eur": 240402.97,
			"ltc": 240402.97,
		},
		TokenRates: map[string]float32{
			"0x5e9997684d061269564f94e5d11ba6ce6fa9528c": 4.1464476e+07,
			"0x906710835d1ae85275eb770f06873340ca54274b": 4.1464476e+07,
		},
		Timestamp: time.Unix(1654819200, 0).UTC(),
	}
	if !reflect.DeepEqual(ticker, &wantTicker) {
		t.Fatalf("UpdateHistoricalTickers(eur) 1st pass = %v, want %v", *ticker, wantTicker)
	}

	// update historical tickers for the second time
	err = fiatRates.downloader.UpdateHistoricalTickers()
	if err != nil {
		t.Fatalf("UpdateHistoricalTickers 2nd pass failed with error: %v", err)
	}
	err = fiatRates.downloader.UpdateHistoricalTokenTickers()
	if err != nil {
		t.Fatalf("UpdateHistoricalTokenTickers 2nd pass failed with error: %v", err)
	}
	ticker, err = fiatRates.db.FiatRatesFindLastTicker("usd", "")
	if err != nil || ticker == nil {
		t.Fatalf("FiatRatesFindLastTicker failed with error: %v", err)
	}
	wantTicker = common.CurrencyRatesTicker{
		Rates: map[string]float32{
			"aed": 240402.97,
			"ars": 240402.97,
			"aud": 240402.97,
			"btc": 240402.97,
			"eth": 240402.97,
			"eur": 240402.97,
			"ltc": 240402.97,
			"usd": 1788.4183,
		},
		TokenRates: map[string]float32{
			"0x5e9997684d061269564f94e5d11ba6ce6fa9528c": 4.1464476e+07,
			"0x906710835d1ae85275eb770f06873340ca54274b": 4.1464476e+07,
		},
		Timestamp: time.Unix(1654819200, 0).UTC(),
	}
	if !reflect.DeepEqual(ticker, &wantTicker) {
		t.Fatalf("UpdateHistoricalTickers(usd) 2nd pass = %v, want %v", *ticker, wantTicker)
	}
	ticker, err = fiatRates.db.FiatRatesFindLastTicker("eur", "")
	if err != nil || ticker == nil {
		t.Fatalf("FiatRatesFindLastTicker failed with error: %v", err)
	}
	if !reflect.DeepEqual(ticker, &wantTicker) {
		t.Fatalf("UpdateHistoricalTickers(eur) 2nd pass = %v, want %v", *ticker, wantTicker)
	}
}
