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
