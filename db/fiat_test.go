//go:build unittest

package db

import (
	"testing"
	"time"
)

func TestRocksTickers(t *testing.T) {
	d := setupRocksDB(t, &testBitcoinParser{
		BitcoinParser: bitcoinTestnetParser(),
	})
	defer closeAndDestroyRocksDB(t, d)

	// Test valid formats
	for _, date := range []string{"20190130", "2019013012", "201901301250", "20190130125030"} {
		_, err := FiatRatesConvertDate(date)
		if err != nil {
			t.Errorf("%v", err)
		}
	}

	// Test invalid formats
	for _, date := range []string{"01102019", "10201901", "", "abc", "20190130xxx"} {
		_, err := FiatRatesConvertDate(date)
		if err == nil {
			t.Errorf("Wrongly-formatted date \"%v\" marked as valid!", date)
		}
	}

	// Test storing & finding tickers
	key, _ := time.Parse(FiatRatesTimeFormat, "20190627000000")
	futureKey, _ := time.Parse(FiatRatesTimeFormat, "20190630000000")

	ts1, _ := time.Parse(FiatRatesTimeFormat, "20190628000000")
	ticker1 := &CurrencyRatesTicker{
		Timestamp: &ts1,
		Rates: map[string]float64{
			"usd": 20000,
		},
	}

	ts2, _ := time.Parse(FiatRatesTimeFormat, "20190629000000")
	ticker2 := &CurrencyRatesTicker{
		Timestamp: &ts2,
		Rates: map[string]float64{
			"usd": 30000,
		},
	}
	err := d.FiatRatesStoreTicker(ticker1)
	if err != nil {
		t.Errorf("Error storing ticker! %v", err)
	}
	d.FiatRatesStoreTicker(ticker2)
	if err != nil {
		t.Errorf("Error storing ticker! %v", err)
	}

	ticker, err := d.FiatRatesFindTicker(&key) // should find the closest key (ticker1)
	if err != nil {
		t.Errorf("TestRocksTickers err: %+v", err)
	} else if ticker == nil {
		t.Errorf("Ticker not found")
	} else if ticker.Timestamp.Format(FiatRatesTimeFormat) != ticker1.Timestamp.Format(FiatRatesTimeFormat) {
		t.Errorf("Incorrect ticker found. Expected: %v, found: %+v", ticker1.Timestamp, ticker.Timestamp)
	}

	ticker, err = d.FiatRatesFindLastTicker() // should find the last key (ticker2)
	if err != nil {
		t.Errorf("TestRocksTickers err: %+v", err)
	} else if ticker == nil {
		t.Errorf("Ticker not found")
	} else if ticker.Timestamp.Format(FiatRatesTimeFormat) != ticker2.Timestamp.Format(FiatRatesTimeFormat) {
		t.Errorf("Incorrect ticker found. Expected: %v, found: %+v", ticker1.Timestamp, ticker.Timestamp)
	}

	ticker, err = d.FiatRatesFindTicker(&futureKey) // should not find anything
	if err != nil {
		t.Errorf("TestRocksTickers err: %+v", err)
	} else if ticker != nil {
		t.Errorf("Ticker found, but the timestamp is older than the last ticker entry.")
	}
}
