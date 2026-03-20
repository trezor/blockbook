//go:build unittest

package db

import (
	"reflect"
	"testing"
	"time"

	"github.com/linxGnu/grocksdb"
	"github.com/trezor/blockbook/common"
)

func TestRocksTickers(t *testing.T) {
	d := setupRocksDB(t, &testBitcoinParser{
		BitcoinParser: bitcoinTestnetParser(),
	})
	defer closeAndDestroyRocksDB(t, d)

	// Test storing & finding tickers
	pastKey, _ := time.Parse(FiatRatesTimeFormat, "20190627000000")
	futureKey, _ := time.Parse(FiatRatesTimeFormat, "20190630000000")

	ts1, _ := time.Parse(FiatRatesTimeFormat, "20190628000000")
	ticker1 := &common.CurrencyRatesTicker{
		Timestamp: ts1,
		Rates: map[string]float32{
			"usd": 20000,
			"eur": 18000,
		},
		TokenRates: map[string]float32{
			"0x6B175474E89094C44Da98b954EedeAC495271d0F": 17.2,
		},
	}

	ts2, _ := time.Parse(FiatRatesTimeFormat, "20190629000000")
	ticker2 := &common.CurrencyRatesTicker{
		Timestamp: ts2,
		Rates: map[string]float32{
			"usd": 30000,
		},
		TokenRates: map[string]float32{
			"0x82dF128257A7d7556262E1AB7F1f639d9775B85E": 13.1,
			"0x6B175474E89094C44Da98b954EedeAC495271d0F": 17.5,
		},
	}

	wb := grocksdb.NewWriteBatch()
	defer wb.Destroy()
	err := d.FiatRatesStoreTicker(wb, ticker1)
	if err != nil {
		t.Errorf("Error storing ticker! %v", err)
	}
	err = d.FiatRatesStoreTicker(wb, ticker2)
	if err != nil {
		t.Errorf("Error storing ticker! %v", err)
	}
	err = d.WriteBatch(wb)
	if err != nil {
		t.Errorf("Error storing ticker! %v", err)
	}

	// test FiatRatesGetTicker with ticker that should be in DB
	t1, err := d.FiatRatesGetTicker(&ts1)
	if err != nil || t1 == nil {
		t.Fatalf("FiatRatesGetTicker t1 %v", err)
	}
	if !reflect.DeepEqual(t1, ticker1) {
		t.Fatalf("FiatRatesGetTicker(t1) = %v, want %v", *t1, *ticker1)
	}
	// test FiatRatesGetTicker with ticker that is not  in DB
	t2, err := d.FiatRatesGetTicker(&pastKey)
	if err != nil || t2 != nil {
		t.Fatalf("FiatRatesGetTicker t2 %v, %v", err, t2)
	}

	ticker, err := d.FiatRatesFindTicker(&pastKey, "", "") // should find the closest key (ticker1)
	if err != nil {
		t.Errorf("TestRocksTickers err: %+v", err)
	} else if ticker == nil {
		t.Errorf("Ticker not found")
	} else if ticker.Timestamp.Format(FiatRatesTimeFormat) != ticker1.Timestamp.Format(FiatRatesTimeFormat) {
		t.Errorf("Incorrect ticker found. Expected: %v, found: %+v", ticker1.Timestamp, ticker.Timestamp)
	}

	ticker, err = d.FiatRatesFindLastTicker("", "") // should find the last key (ticker2)
	if err != nil {
		t.Errorf("TestRocksTickers err: %+v", err)
	} else if ticker == nil {
		t.Errorf("Ticker not found")
	} else if ticker.Timestamp.Format(FiatRatesTimeFormat) != ticker2.Timestamp.Format(FiatRatesTimeFormat) {
		t.Errorf("Incorrect ticker found. Expected: %v, found: %+v", ticker1.Timestamp, ticker.Timestamp)
	}

	ticker, err = d.FiatRatesFindTicker(&futureKey, "", "") // should not find anything
	if err != nil {
		t.Errorf("TestRocksTickers err: %+v", err)
	} else if ticker != nil {
		t.Errorf("Ticker found, but the timestamp is older than the last ticker entry.")
	}

	ticker, err = d.FiatRatesFindTicker(&pastKey, "", "0x6B175474E89094C44Da98b954EedeAC495271d0F") // should find the closest key (ticker1)
	if err != nil {
		t.Errorf("TestRocksTickers err: %+v", err)
	} else if ticker == nil {
		t.Errorf("Ticker not found")
	} else if ticker.Timestamp.Format(FiatRatesTimeFormat) != ticker1.Timestamp.Format(FiatRatesTimeFormat) {
		t.Errorf("Incorrect ticker found. Expected: %v, found: %+v", ticker1.Timestamp, ticker.Timestamp)
	}

	ticker, err = d.FiatRatesFindTicker(&pastKey, "", "0x82dF128257A7d7556262E1AB7F1f639d9775B85E") // should find the last key (ticker2)
	if err != nil {
		t.Errorf("TestRocksTickers err: %+v", err)
	} else if ticker == nil {
		t.Errorf("Ticker not found")
	} else if ticker.Timestamp.Format(FiatRatesTimeFormat) != ticker2.Timestamp.Format(FiatRatesTimeFormat) {
		t.Errorf("Incorrect ticker found. Expected: %v, found: %+v", ticker2.Timestamp, ticker.Timestamp)
	}

	ticker, err = d.FiatRatesFindLastTicker("eur", "") // should find the closest key (ticker1)
	if err != nil {
		t.Errorf("TestRocksTickers err: %+v", err)
	} else if ticker == nil {
		t.Errorf("Ticker not found")
	} else if ticker.Timestamp.Format(FiatRatesTimeFormat) != ticker1.Timestamp.Format(FiatRatesTimeFormat) {
		t.Errorf("Incorrect ticker found. Expected: %v, found: %+v", ticker1.Timestamp, ticker.Timestamp)
	}

	ticker, err = d.FiatRatesFindLastTicker("usd", "") // should find the last key (ticker2)
	if err != nil {
		t.Errorf("TestRocksTickers err: %+v", err)
	} else if ticker == nil {
		t.Errorf("Ticker not found")
	} else if ticker.Timestamp.Format(FiatRatesTimeFormat) != ticker2.Timestamp.Format(FiatRatesTimeFormat) {
		t.Errorf("Incorrect ticker found. Expected: %v, found: %+v", ticker2.Timestamp, ticker.Timestamp)
	}

	ticker, err = d.FiatRatesFindLastTicker("aud", "") // should not find any key
	if err != nil {
		t.Errorf("TestRocksTickers err: %+v", err)
	} else if ticker != nil {
		t.Errorf("Ticker %v found unexpectedly for aud vsCurrency", ticker)
	}

	queries := []struct {
		name       string
		vsCurrency string
		token      string
	}{
		{name: "base", vsCurrency: "", token: ""},
		{name: "eur", vsCurrency: "eur", token: ""},
		{name: "token", vsCurrency: "", token: "0x6B175474E89094C44Da98b954EedeAC495271d0F"},
	}
	timestamps := []int64{
		pastKey.Unix(),
		ts1.Unix(),
		ts1.Unix() + 3600,
		ts2.Unix(),
		futureKey.Unix(),
	}
	for _, q := range queries {
		got, err := d.FiatRatesFindTickers(timestamps, q.vsCurrency, q.token)
		if err != nil {
			t.Fatalf("FiatRatesFindTickers(%s) returned error: %v", q.name, err)
		}
		if len(got) != len(timestamps) {
			t.Fatalf("FiatRatesFindTickers(%s) returned %d items, want %d", q.name, len(got), len(timestamps))
		}
		for i, ts := range timestamps {
			tsTime := time.Unix(ts, 0).UTC()
			want, err := d.FiatRatesFindTicker(&tsTime, q.vsCurrency, q.token)
			if err != nil {
				t.Fatalf("FiatRatesFindTicker(%s) returned error: %v", q.name, err)
			}
			if !reflect.DeepEqual(got[i], want) {
				t.Fatalf("FiatRatesFindTickers(%s) mismatch at index %d: got %+v, want %+v", q.name, i, got[i], want)
			}
		}
	}

}

func TestFiatRatesFindTickersSparseTokenGaps(t *testing.T) {
	d := setupRocksDB(t, &testBitcoinParser{
		BitcoinParser: bitcoinTestnetParser(),
	})
	defer closeAndDestroyRocksDB(t, d)

	ts1, _ := time.Parse(FiatRatesTimeFormat, "20190628000000")
	ts2, _ := time.Parse(FiatRatesTimeFormat, "20190629000000")
	ts3, _ := time.Parse(FiatRatesTimeFormat, "20190630000000")

	token := "0x82dF128257A7d7556262E1AB7F1f639d9775B85E"

	ticker1 := &common.CurrencyRatesTicker{
		Timestamp: ts1,
		Rates: map[string]float32{
			"usd": 20000,
		},
		TokenRates: map[string]float32{
			"0x6B175474E89094C44Da98b954EedeAC495271d0F": 17.2,
		},
	}
	ticker2 := &common.CurrencyRatesTicker{
		Timestamp: ts2,
		Rates: map[string]float32{
			"usd": 30000,
		},
	}
	ticker3 := &common.CurrencyRatesTicker{
		Timestamp: ts3,
		Rates: map[string]float32{
			"usd": 40000,
		},
		TokenRates: map[string]float32{
			token: 13.1,
		},
	}

	wb := grocksdb.NewWriteBatch()
	defer wb.Destroy()
	if err := d.FiatRatesStoreTicker(wb, ticker1); err != nil {
		t.Fatalf("failed storing ticker1: %v", err)
	}
	if err := d.FiatRatesStoreTicker(wb, ticker2); err != nil {
		t.Fatalf("failed storing ticker2: %v", err)
	}
	if err := d.FiatRatesStoreTicker(wb, ticker3); err != nil {
		t.Fatalf("failed storing ticker3: %v", err)
	}
	if err := d.WriteBatch(wb); err != nil {
		t.Fatalf("failed writing batch: %v", err)
	}

	timestamps := []int64{
		ts1.Unix() - 1,
		ts1.Unix(),
		ts1.Unix() + 3600,
		ts2.Unix(),
		ts2.Unix() + 3600,
		ts3.Unix(),
		ts3.Unix() + 3600,
	}

	got, err := d.FiatRatesFindTickers(timestamps, "", token)
	if err != nil {
		t.Fatalf("FiatRatesFindTickers returned error: %v", err)
	}
	if len(got) != len(timestamps) {
		t.Fatalf("FiatRatesFindTickers returned %d items, want %d", len(got), len(timestamps))
	}

	for i := 0; i < len(timestamps)-1; i++ {
		if got[i] == nil {
			t.Fatalf("expected ticker at index %d, got nil", i)
		}
		if got[i].Timestamp.Unix() != ts3.Unix() {
			t.Fatalf("unexpected timestamp at index %d: got %d, want %d", i, got[i].Timestamp.Unix(), ts3.Unix())
		}
		if got[i].TokenRates[token] != 13.1 {
			t.Fatalf("unexpected token rate at index %d: got %v, want %v", i, got[i].TokenRates[token], float32(13.1))
		}
	}
	if got[len(got)-1] != nil {
		t.Fatalf("expected nil for timestamp after last suitable ticker, got %+v", got[len(got)-1])
	}

	// Keep parity with single-item lookup semantics.
	for i, ts := range timestamps {
		tsTime := time.Unix(ts, 0).UTC()
		want, err := d.FiatRatesFindTicker(&tsTime, "", token)
		if err != nil {
			t.Fatalf("FiatRatesFindTicker returned error at index %d: %v", i, err)
		}
		if !reflect.DeepEqual(got[i], want) {
			t.Fatalf("FiatRatesFindTickers mismatch at index %d: got %+v, want %+v", i, got[i], want)
		}
	}
}

func Test_packUnpackCurrencyRatesTicker(t *testing.T) {
	type args struct {
	}
	tests := []struct {
		name string
		data common.CurrencyRatesTicker
	}{
		{
			name: "empty",
			data: common.CurrencyRatesTicker{},
		},
		{
			name: "rates",
			data: common.CurrencyRatesTicker{
				Rates: map[string]float32{
					"usd": 2129.2341123,
					"eur": 1332.51234,
				},
			},
		},
		{
			name: "rates&tokenrates",
			data: common.CurrencyRatesTicker{
				Rates: map[string]float32{
					"usd": 322129.987654321,
					"eur": 291332.12345678,
				},
				TokenRates: map[string]float32{
					"0x82dF128257A7d7556262E1AB7F1f639d9775B85E": 0.4092341123,
					"0x6B175474E89094C44Da98b954EedeAC495271d0F": 12.32323232323232,
					"0xdAC17F958D2ee523a2206206994597C13D831ec7": 1332421341235.51234,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packed := packCurrencyRatesTicker(&tt.data)
			got, err := unpackCurrencyRatesTicker(packed)
			if err != nil {
				t.Errorf("unpackCurrencyRatesTicker() error = %v", err)
				return
			}
			if !reflect.DeepEqual(got, &tt.data) {
				t.Errorf("unpackCurrencyRatesTicker() = %v, want %v", *got, tt.data)
			}
		})
	}
}
