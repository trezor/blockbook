//go:build unittest

package api

import (
	"reflect"
	"testing"

	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/fiat"
)

func TestSetFiatRateToBalanceHistories_BatchesTickerLookup(t *testing.T) {
	histories := BalanceHistories{
		{Time: 100},
		{Time: 200},
		{Time: 300},
	}
	w := &Worker{
		fiatRates: &fiat.FiatRates{Enabled: true},
	}
	originalGetter := getTickersForTimestamps
	defer func() {
		getTickersForTimestamps = originalGetter
	}()

	calls := 0
	var gotTimestamps []int64
	getTickersForTimestamps = func(_ *fiat.FiatRates, timestamps []int64, _, _ string) (*[]*common.CurrencyRatesTicker, error) {
		calls++
		gotTimestamps = append([]int64(nil), timestamps...)
		tickers := []*common.CurrencyRatesTicker{
			{Rates: map[string]float32{"usd": 11, "eur": 22}},
			nil,
			{Rates: map[string]float32{"usd": 33}},
		}
		return &tickers, nil
	}

	err := w.setFiatRateToBalanceHistories(histories, []string{"USD", "eur", "cad"})
	if err != nil {
		t.Fatalf("setFiatRateToBalanceHistories returned error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 ticker lookup call, got %d", calls)
	}
	if !reflect.DeepEqual(gotTimestamps, []int64{100, 200, 300}) {
		t.Fatalf("unexpected timestamps: got %v", gotTimestamps)
	}
	if !reflect.DeepEqual(histories[0].FiatRates, map[string]float32{"usd": 11, "eur": 22, "cad": -1}) {
		t.Fatalf("unexpected rates for histories[0]: %v", histories[0].FiatRates)
	}
	if histories[1].FiatRates != nil {
		t.Fatalf("expected nil rates for histories[1], got %v", histories[1].FiatRates)
	}
	if !reflect.DeepEqual(histories[2].FiatRates, map[string]float32{"usd": 33, "eur": -1, "cad": -1}) {
		t.Fatalf("unexpected rates for histories[2]: %v", histories[2].FiatRates)
	}
}

func TestSetFiatRateToBalanceHistories_AllRatesWhenCurrenciesNotSpecified(t *testing.T) {
	histories := BalanceHistories{
		{Time: 100},
	}
	w := &Worker{
		fiatRates: &fiat.FiatRates{Enabled: true},
	}
	originalGetter := getTickersForTimestamps
	defer func() {
		getTickersForTimestamps = originalGetter
	}()

	getTickersForTimestamps = func(_ *fiat.FiatRates, _ []int64, _, _ string) (*[]*common.CurrencyRatesTicker, error) {
		tickers := []*common.CurrencyRatesTicker{
			{Rates: map[string]float32{"usd": 11, "eur": 22}},
		}
		return &tickers, nil
	}

	err := w.setFiatRateToBalanceHistories(histories, nil)
	if err != nil {
		t.Fatalf("setFiatRateToBalanceHistories returned error: %v", err)
	}
	if !reflect.DeepEqual(histories[0].FiatRates, map[string]float32{"usd": 11, "eur": 22}) {
		t.Fatalf("unexpected rates for histories[0]: %v", histories[0].FiatRates)
	}
}

func TestSetFiatRateToBalanceHistories_BatchFailureFallsBackToPerPoint(t *testing.T) {
	histories := BalanceHistories{
		{Time: 100},
		{Time: 200},
		{Time: 300},
	}
	w := &Worker{
		fiatRates: &fiat.FiatRates{Enabled: true},
	}
	originalGetter := getTickersForTimestamps
	defer func() {
		getTickersForTimestamps = originalGetter
	}()

	calls := 0
	var gotCalls [][]int64
	getTickersForTimestamps = func(_ *fiat.FiatRates, timestamps []int64, _, _ string) (*[]*common.CurrencyRatesTicker, error) {
		calls++
		gotCalls = append(gotCalls, append([]int64(nil), timestamps...))
		if len(timestamps) > 1 {
			return nil, assertError("batch error")
		}
		switch timestamps[0] {
		case 100:
			tickers := []*common.CurrencyRatesTicker{
				{Rates: map[string]float32{"usd": 11}},
			}
			return &tickers, nil
		case 200:
			return nil, assertError("point error")
		case 300:
			tickers := []*common.CurrencyRatesTicker{
				{Rates: map[string]float32{"usd": 33}},
			}
			return &tickers, nil
		default:
			tickers := []*common.CurrencyRatesTicker{}
			return &tickers, nil
		}
	}

	err := w.setFiatRateToBalanceHistories(histories, []string{"usd"})
	if err != nil {
		t.Fatalf("setFiatRateToBalanceHistories returned error: %v", err)
	}
	if calls != 4 {
		t.Fatalf("expected 4 ticker lookup calls (1 batch + 3 point), got %d", calls)
	}
	wantCalls := [][]int64{
		{100, 200, 300},
		{100},
		{200},
		{300},
	}
	if !reflect.DeepEqual(gotCalls, wantCalls) {
		t.Fatalf("unexpected lookup calls: got %v, want %v", gotCalls, wantCalls)
	}
	if !reflect.DeepEqual(histories[0].FiatRates, map[string]float32{"usd": 11}) {
		t.Fatalf("unexpected rates for histories[0]: %v", histories[0].FiatRates)
	}
	if histories[1].FiatRates != nil {
		t.Fatalf("expected nil rates for histories[1], got %v", histories[1].FiatRates)
	}
	if !reflect.DeepEqual(histories[2].FiatRates, map[string]float32{"usd": 33}) {
		t.Fatalf("unexpected rates for histories[2]: %v", histories[2].FiatRates)
	}
}

func TestSetFiatRateToBalanceHistories_SkipsLookupForEmptyHistory(t *testing.T) {
	w := &Worker{
		fiatRates: &fiat.FiatRates{Enabled: true},
	}
	originalGetter := getTickersForTimestamps
	defer func() {
		getTickersForTimestamps = originalGetter
	}()

	calls := 0
	getTickersForTimestamps = func(_ *fiat.FiatRates, _ []int64, _, _ string) (*[]*common.CurrencyRatesTicker, error) {
		calls++
		tickers := []*common.CurrencyRatesTicker{}
		return &tickers, nil
	}

	err := w.setFiatRateToBalanceHistories(BalanceHistories{}, []string{"usd"})
	if err != nil {
		t.Fatalf("setFiatRateToBalanceHistories returned error: %v", err)
	}
	if calls != 0 {
		t.Fatalf("expected 0 ticker lookup calls, got %d", calls)
	}
}

type assertError string

func (e assertError) Error() string {
	return string(e)
}
