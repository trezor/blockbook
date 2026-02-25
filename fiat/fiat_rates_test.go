//go:build unittest

package fiat

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/golang/glog"
	"github.com/linxGnu/grocksdb"
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

func setupRocksDB(t *testing.T, parser bchain.BlockChainParser, config *common.Config) (*db.RocksDB, *common.InternalState, string) {
	tmp, err := os.MkdirTemp("", "testdb")
	if err != nil {
		t.Fatal(err)
	}
	d, err := db.NewRocksDB(tmp, 100000, -1, parser, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	// Force synchronous block-times initialization in tests.
	// For non-"coin-unittest" names, LoadInternalState starts a background
	// goroutine that can race with test DB teardown.
	loadConfig := *config
	loadConfig.CoinName = "coin-unittest"
	is, err := d.LoadInternalState(&loadConfig)
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

	// config with mocked CoinGecko API
	config := common.Config{
		CoinName:        "fakecoin",
		FiatRates:       "coingecko",
		FiatRatesParams: `{"url": "` + mockServer.URL + `", "coin": "ethereum","platformIdentifier": "ethereum","platformVsCurrency": "eth","periodSeconds": 60}`,
	}

	d, _, tmp := setupRocksDB(t, &testBitcoinParser{
		BitcoinParser: bitcoinTestnetParser(),
	}, &config)
	defer closeAndDestroyRocksDB(t, d, tmp)

	fiatRates, err := NewFiatRates(d, &config, nil, nil)
	if err != nil {
		t.Fatalf("FiatRates init error: %v", err)
	}
	// In the current model, FiatRatesParams.url is bootstrap URL only.
	// Point tip/current calls to the mock explicitly to keep this test isolated.
	coingeckoDownloader, ok := fiatRates.downloader.(*Coingecko)
	if !ok {
		t.Fatalf("unexpected downloader type: %T", fiatRates.downloader)
	}
	coingeckoDownloader.tipURL = mockServer.URL

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

func TestGetTickersForTimestamps_UsesGranularityAndFallback(t *testing.T) {
	fr := &FiatRates{
		Enabled: true,
		currentTicker: &common.CurrencyRatesTicker{
			Timestamp: time.Unix(123456, 0).UTC(),
			Rates:     map[string]float32{"usd": 4},
		},
		fiveMinutesTickers: map[int64]*common.CurrencyRatesTicker{
			600: {
				Timestamp: time.Unix(600, 0).UTC(),
				Rates:     map[string]float32{"usd": 1},
			},
		},
		fiveMinutesTickersFrom: 600,
		fiveMinutesTickersTo:   600,
		hourlyTickers: map[int64]*common.CurrencyRatesTicker{
			3600: {
				Timestamp: time.Unix(3600, 0).UTC(),
				Rates:     map[string]float32{"usd": 2},
			},
		},
		hourlyTickersFrom: 3600,
		hourlyTickersTo:   3600,
		dailyTickers: map[int64]*common.CurrencyRatesTicker{
			86400: {
				Timestamp: time.Unix(86400, 0).UTC(),
				Rates:     map[string]float32{"usd": 3},
			},
		},
		dailyTickersFrom: 86400,
		dailyTickersTo:   86400,
	}

	tickers, err := fr.GetTickersForTimestamps([]int64{600, 3600, 86400, 90000}, "usd", "")
	if err != nil {
		t.Fatalf("GetTickersForTimestamps returned error: %v", err)
	}
	if tickers == nil || len(*tickers) != 4 {
		t.Fatalf("unexpected ticker result shape: %+v", tickers)
	}

	got := []float32{
		(*tickers)[0].Rates["usd"],
		(*tickers)[1].Rates["usd"],
		(*tickers)[2].Rates["usd"],
		(*tickers)[3].Rates["usd"],
	}
	want := []float32{1, 2, 3, 4}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected rates: got %v, want %v", got, want)
	}
}

func TestGetTickersForTimestamps_ConcurrentReadersAndWriters(t *testing.T) {
	fr := &FiatRates{Enabled: true}

	const (
		writers      = 2
		readers      = 8
		testDuration = 1200 * time.Millisecond
		waitTimeout  = 3 * time.Second
	)

	stop := make(chan struct{})
	errCh := make(chan error, readers)
	readerCalls := make([]int, readers)
	var wg sync.WaitGroup

	setState := func(counter int64) {
		currentTicker := &common.CurrencyRatesTicker{
			Timestamp: time.Unix(123456+counter, 0).UTC(),
			Rates:     map[string]float32{"usd": float32(100 + counter%100)},
		}
		fr.mux.Lock()
		fr.currentTicker = currentTicker
		fr.fiveMinutesTickers = map[int64]*common.CurrencyRatesTicker{
			600: {
				Timestamp: time.Unix(600, 0).UTC(),
				Rates:     map[string]float32{"usd": float32(1 + counter%10)},
			},
		}
		fr.fiveMinutesTickersFrom = 600
		fr.fiveMinutesTickersTo = 600
		fr.hourlyTickers = map[int64]*common.CurrencyRatesTicker{
			3600: {
				Timestamp: time.Unix(3600, 0).UTC(),
				Rates:     map[string]float32{"usd": float32(10 + counter%10)},
			},
		}
		fr.hourlyTickersFrom = 3600
		fr.hourlyTickersTo = 3600
		fr.dailyTickers = map[int64]*common.CurrencyRatesTicker{
			86400: {
				Timestamp: time.Unix(86400, 0).UTC(),
				Rates:     map[string]float32{"usd": float32(20 + counter%10)},
			},
		}
		fr.dailyTickersFrom = 86400
		fr.dailyTickersTo = 86400
		fr.mux.Unlock()
	}

	// Seed cache state before readers start.
	setState(0)

	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()

			counter := int64(seed)
			for {
				select {
				case <-stop:
					return
				default:
				}

				setState(counter)

				counter++
				time.Sleep(100 * time.Microsecond)
			}
		}(w)
	}

	for r := 0; r < readers; r++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			timestamps := []int64{600, 3600, 86400, 90000}
			calls := 0
			for {
				select {
				case <-stop:
					readerCalls[idx] = calls
					return
				default:
				}

				tickers, err := fr.GetTickersForTimestamps(timestamps, "usd", "")
				if err != nil {
					errCh <- fmt.Errorf("reader %d returned error: %w", idx, err)
					readerCalls[idx] = calls
					return
				}
				if tickers == nil || len(*tickers) != len(timestamps) {
					errCh <- fmt.Errorf("reader %d unexpected ticker shape: %+v", idx, tickers)
					readerCalls[idx] = calls
					return
				}
				for i, ticker := range *tickers {
					if ticker == nil {
						errCh <- fmt.Errorf("reader %d got nil ticker at index %d", idx, i)
						readerCalls[idx] = calls
						return
					}
					if _, found := ticker.Rates["usd"]; !found {
						errCh <- fmt.Errorf("reader %d ticker at index %d missing usd rate", idx, i)
						readerCalls[idx] = calls
						return
					}
				}
				calls++
			}
		}(r)
	}

	time.Sleep(testDuration)
	close(stop)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(waitTimeout):
		t.Fatal("concurrent fiat readers/writers did not finish in time")
	}

	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}

	totalCalls := 0
	for i, calls := range readerCalls {
		if calls == 0 {
			t.Fatalf("reader %d did not make any successful calls", i)
		}
		totalCalls += calls
	}
	if totalCalls < readers {
		t.Fatalf("too few reader calls made: got %d", totalCalls)
	}
}

func TestGetTokenTickersForTimestamps_QueriesUniqueSortedTimestamps(t *testing.T) {
	originalFindTickers := fiatRatesFindTickers
	defer func() {
		fiatRatesFindTickers = originalFindTickers
	}()

	lookupCalls := make([]int64, 0)
	batchCalls := 0
	fiatRatesFindTickers = func(_ *db.RocksDB, timestamps []int64, _, _ string) ([]*common.CurrencyRatesTicker, error) {
		batchCalls++
		lookupCalls = append(lookupCalls, timestamps...)
		tickers := make([]*common.CurrencyRatesTicker, len(timestamps))
		for i, ts := range timestamps {
			tickers[i] = &common.CurrencyRatesTicker{
				Timestamp:  time.Unix(ts, 0).UTC(),
				Rates:      map[string]float32{"usd": float32(ts)},
				TokenRates: map[string]float32{"token": 1},
			}
		}
		return tickers, nil
	}

	fr := &FiatRates{
		currentTicker: &common.CurrencyRatesTicker{
			Timestamp:  time.Unix(999, 0).UTC(),
			Rates:      map[string]float32{"usd": 1},
			TokenRates: map[string]float32{"token": 1},
		},
	}
	input := []int64{300, 100, 200, 100, 250}
	tickers, err := fr.getTokenTickersForTimestamps(input, "", "token")
	if err != nil {
		t.Fatalf("getTokenTickersForTimestamps returned error: %v", err)
	}
	if tickers == nil {
		t.Fatal("expected non-nil tickers")
	}

	if !reflect.DeepEqual(lookupCalls, []int64{100, 200, 250, 300}) {
		t.Fatalf("unexpected DB lookup order: got %v", lookupCalls)
	}
	if batchCalls != 1 {
		t.Fatalf("unexpected number of batch DB calls: got %d, want %d", batchCalls, 1)
	}

	got := make([]float32, len(input))
	for i := range input {
		if (*tickers)[i] == nil {
			t.Fatalf("ticker at index %d is nil", i)
		}
		got[i] = (*tickers)[i].Rates["usd"]
	}
	want := []float32{300, 100, 200, 100, 250}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected returned rates: got %v, want %v", got, want)
	}
}

func TestGetTokenTickersForTimestamps_SkipsDBLookupWhenCurrentTickerHasNoToken(t *testing.T) {
	originalFindTickers := fiatRatesFindTickers
	defer func() {
		fiatRatesFindTickers = originalFindTickers
	}()

	lookupCalls := 0
	fiatRatesFindTickers = func(_ *db.RocksDB, _ []int64, _, _ string) ([]*common.CurrencyRatesTicker, error) {
		lookupCalls++
		return nil, nil
	}

	fr := &FiatRates{
		currentTicker: &common.CurrencyRatesTicker{
			Timestamp:  time.Unix(999, 0).UTC(),
			Rates:      map[string]float32{"usd": 1},
			TokenRates: map[string]float32{"another-token": 1},
		},
	}
	tickers, err := fr.getTokenTickersForTimestamps([]int64{100, 200}, "", "token")
	if err != nil {
		t.Fatalf("getTokenTickersForTimestamps returned error: %v", err)
	}
	if lookupCalls != 0 {
		t.Fatalf("expected 0 DB lookups, got %d", lookupCalls)
	}
	if tickers == nil || len(*tickers) != 2 {
		t.Fatalf("unexpected ticker result shape: %+v", tickers)
	}
	if (*tickers)[0] != nil || (*tickers)[1] != nil {
		t.Fatalf("expected nil tickers when current ticker does not include token, got %+v", *tickers)
	}
}

func TestNewFiatRates_AllowsBootstrapOnDefaultHistoricalURLWithoutAPIKey(t *testing.T) {
	config := common.Config{
		CoinName:        "fakecoin",
		FiatRates:       "coingecko",
		FiatRatesParams: `{"coin":"ethereum","periodSeconds":60}`,
	}
	d, is, tmp := setupRocksDB(t, &testBitcoinParser{
		BitcoinParser: bitcoinTestnetParser(),
	}, &config)
	defer closeAndDestroyRocksDB(t, d, tmp)

	// Ensure this test is deterministic even if host env has CoinGecko keys set.
	envNames := append([]string{coingeckoAPIKeyEnv}, coinGeckoScopedAPIKeyEnvNames(is.GetNetwork(), is.CoinShortcut)...)
	originalEnv := make(map[string]*string, len(envNames))
	for _, envName := range envNames {
		if v, ok := os.LookupEnv(envName); ok {
			value := v
			originalEnv[envName] = &value
		} else {
			originalEnv[envName] = nil
		}
		_ = os.Unsetenv(envName)
	}
	defer func() {
		for _, envName := range envNames {
			if v := originalEnv[envName]; v == nil {
				_ = os.Unsetenv(envName)
			} else {
				_ = os.Setenv(envName, *v)
			}
		}
	}()

	_, err := NewFiatRates(d, &config, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	complete, found, err := d.FiatRatesGetHistoricalBootstrapComplete()
	if err != nil {
		t.Fatalf("FiatRatesGetHistoricalBootstrapComplete failed: %v", err)
	}
	if !found || complete {
		t.Fatalf("unexpected bootstrap state after init: found=%v complete=%v", found, complete)
	}
}

func TestNewFiatRates_AllowsNoKeyOrURLWhenHistoricalFiatAlreadyExists(t *testing.T) {
	config := common.Config{
		CoinName:        "fakecoin",
		FiatRates:       "coingecko",
		FiatRatesParams: `{"coin":"ethereum","periodSeconds":60}`,
	}
	d, is, tmp := setupRocksDB(t, &testBitcoinParser{
		BitcoinParser: bitcoinTestnetParser(),
	}, &config)
	defer closeAndDestroyRocksDB(t, d, tmp)

	// Seed any historical fiat ticker so the instance is no longer bootstrap-empty.
	wb := grocksdb.NewWriteBatch()
	defer wb.Destroy()
	seedTicker := &common.CurrencyRatesTicker{
		Timestamp: time.Unix(1700000000, 0).UTC(),
		Rates: map[string]float32{
			"usd": 1,
		},
	}
	if err := d.FiatRatesStoreTicker(wb, seedTicker); err != nil {
		t.Fatalf("FiatRatesStoreTicker failed: %v", err)
	}
	if err := d.WriteBatch(wb); err != nil {
		t.Fatalf("WriteBatch failed: %v", err)
	}

	envNames := append([]string{coingeckoAPIKeyEnv}, coinGeckoScopedAPIKeyEnvNames(is.GetNetwork(), is.CoinShortcut)...)
	originalEnv := make(map[string]*string, len(envNames))
	for _, envName := range envNames {
		if v, ok := os.LookupEnv(envName); ok {
			value := v
			originalEnv[envName] = &value
		} else {
			originalEnv[envName] = nil
		}
		_ = os.Unsetenv(envName)
	}
	defer func() {
		for _, envName := range envNames {
			if v := originalEnv[envName]; v == nil {
				_ = os.Unsetenv(envName)
			} else {
				_ = os.Setenv(envName, *v)
			}
		}
	}()

	_, err := NewFiatRates(d, &config, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	complete, found, err := d.FiatRatesGetHistoricalBootstrapComplete()
	if err != nil {
		t.Fatalf("FiatRatesGetHistoricalBootstrapComplete failed: %v", err)
	}
	if !found || !complete {
		t.Fatalf("unexpected bootstrap state after successful init: found=%v complete=%v", found, complete)
	}
}

func TestNewFiatRates_AllowsBootstrapStateInProgressWithoutURLOrAPIKey(t *testing.T) {
	config := common.Config{
		CoinName:        "fakecoin",
		FiatRates:       "coingecko",
		FiatRatesParams: `{"coin":"ethereum","periodSeconds":60}`,
	}
	d, is, tmp := setupRocksDB(t, &testBitcoinParser{
		BitcoinParser: bitcoinTestnetParser(),
	}, &config)
	defer closeAndDestroyRocksDB(t, d, tmp)

	// Simulate interrupted bootstrap with partially populated DB.
	wb := grocksdb.NewWriteBatch()
	defer wb.Destroy()
	seedTicker := &common.CurrencyRatesTicker{
		Timestamp: time.Unix(1700000000, 0).UTC(),
		Rates: map[string]float32{
			"usd": 1,
		},
	}
	if err := d.FiatRatesStoreTicker(wb, seedTicker); err != nil {
		t.Fatalf("FiatRatesStoreTicker failed: %v", err)
	}
	if err := d.WriteBatch(wb); err != nil {
		t.Fatalf("WriteBatch failed: %v", err)
	}
	if err := d.FiatRatesSetHistoricalBootstrapComplete(false); err != nil {
		t.Fatalf("FiatRatesSetHistoricalBootstrapComplete failed: %v", err)
	}

	envNames := append([]string{coingeckoAPIKeyEnv}, coinGeckoScopedAPIKeyEnvNames(is.GetNetwork(), is.CoinShortcut)...)
	originalEnv := make(map[string]*string, len(envNames))
	for _, envName := range envNames {
		if v, ok := os.LookupEnv(envName); ok {
			value := v
			originalEnv[envName] = &value
		} else {
			originalEnv[envName] = nil
		}
		_ = os.Unsetenv(envName)
	}
	defer func() {
		for _, envName := range envNames {
			if v := originalEnv[envName]; v == nil {
				_ = os.Unsetenv(envName)
			} else {
				_ = os.Setenv(envName, *v)
			}
		}
	}()

	_, err := NewFiatRates(d, &config, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	complete, found, err := d.FiatRatesGetHistoricalBootstrapComplete()
	if err != nil {
		t.Fatalf("FiatRatesGetHistoricalBootstrapComplete failed: %v", err)
	}
	if !found || complete {
		t.Fatalf("unexpected bootstrap state after init: found=%v complete=%v", found, complete)
	}
}

func TestRegisterHistoricalBootstrapAttemptFailure_MarksBootstrapCompleteAfterThreeFailures(t *testing.T) {
	config := common.Config{
		CoinName: "fakecoin",
	}
	d, _, tmp := setupRocksDB(t, &testBitcoinParser{
		BitcoinParser: bitcoinTestnetParser(),
	}, &config)
	defer closeAndDestroyRocksDB(t, d, tmp)

	if err := d.FiatRatesSetHistoricalBootstrapComplete(false); err != nil {
		t.Fatalf("FiatRatesSetHistoricalBootstrapComplete failed: %v", err)
	}

	for i := 1; i < maxHistoricalBootstrapAttempts; i++ {
		attempts, exhausted, err := registerHistoricalBootstrapAttemptFailure(d)
		if err != nil {
			t.Fatalf("registerHistoricalBootstrapAttemptFailure failed: %v", err)
		}
		if exhausted {
			t.Fatalf("attempt %d unexpectedly exhausted", i)
		}
		if attempts != i {
			t.Fatalf("unexpected attempts value: got %d, want %d", attempts, i)
		}
		complete, found, err := d.FiatRatesGetHistoricalBootstrapComplete()
		if err != nil {
			t.Fatalf("FiatRatesGetHistoricalBootstrapComplete failed: %v", err)
		}
		if !found || complete {
			t.Fatalf("bootstrap state should remain incomplete before limit: found=%v complete=%v", found, complete)
		}
	}

	attempts, exhausted, err := registerHistoricalBootstrapAttemptFailure(d)
	if err != nil {
		t.Fatalf("registerHistoricalBootstrapAttemptFailure failed on limit: %v", err)
	}
	if !exhausted {
		t.Fatalf("expected exhausted=true on attempt limit")
	}
	if attempts != maxHistoricalBootstrapAttempts {
		t.Fatalf("unexpected attempts value on limit: got %d, want %d", attempts, maxHistoricalBootstrapAttempts)
	}

	complete, found, err := d.FiatRatesGetHistoricalBootstrapComplete()
	if err != nil {
		t.Fatalf("FiatRatesGetHistoricalBootstrapComplete failed: %v", err)
	}
	if !found || !complete {
		t.Fatalf("bootstrap should be marked complete after attempt limit: found=%v complete=%v", found, complete)
	}

	storedAttempts, found, err := d.FiatRatesGetHistoricalBootstrapAttempts()
	if err != nil {
		t.Fatalf("FiatRatesGetHistoricalBootstrapAttempts failed: %v", err)
	}
	if !found || storedAttempts != 0 {
		t.Fatalf("bootstrap attempts should be reset after exhaustion: found=%v attempts=%d", found, storedAttempts)
	}
}

func TestResetHistoricalBootstrapAttempts(t *testing.T) {
	config := common.Config{
		CoinName: "fakecoin",
	}
	d, _, tmp := setupRocksDB(t, &testBitcoinParser{
		BitcoinParser: bitcoinTestnetParser(),
	}, &config)
	defer closeAndDestroyRocksDB(t, d, tmp)

	if err := d.FiatRatesSetHistoricalBootstrapAttempts(2); err != nil {
		t.Fatalf("FiatRatesSetHistoricalBootstrapAttempts failed: %v", err)
	}
	if err := resetHistoricalBootstrapAttempts(d); err != nil {
		t.Fatalf("resetHistoricalBootstrapAttempts failed: %v", err)
	}
	attempts, found, err := d.FiatRatesGetHistoricalBootstrapAttempts()
	if err != nil {
		t.Fatalf("FiatRatesGetHistoricalBootstrapAttempts failed: %v", err)
	}
	if !found || attempts != 0 {
		t.Fatalf("unexpected attempts after reset: found=%v attempts=%d", found, attempts)
	}
}
