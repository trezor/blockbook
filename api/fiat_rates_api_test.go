//go:build unittest

package api

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/fiat"
)

func requireAPIError(t *testing.T, err error, wantPublic bool) *APIError {
	t.Helper()
	if err == nil {
		t.Fatal("expected API error, got nil")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T (%v)", err, err)
	}
	if apiErr.Public != wantPublic {
		t.Fatalf("unexpected API error visibility: got %v, want %v", apiErr.Public, wantPublic)
	}
	return apiErr
}

func TestRemoveEmpty(t *testing.T) {
	got := removeEmpty([]string{"usd", "", "eur", "", ""})
	want := []string{"usd", "eur"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected filtered currencies: got %v, want %v", got, want)
	}
}

func TestMakeErrorRates(t *testing.T) {
	got := makeErrorRates([]string{"USD", "eur", "Usd"})
	want := map[string]float32{
		"usd": -1,
		"eur": -1,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected error rates: got %v, want %v", got, want)
	}
}

func TestGetFiatRatesResult_NonTokenSelectedCurrencies(t *testing.T) {
	w := &Worker{}
	ticker := &common.CurrencyRatesTicker{
		Timestamp: time.Unix(1700000000, 0),
		Rates: map[string]float32{
			"usd": 1.23,
			"eur": 0.99,
		},
	}

	got, err := w.getFiatRatesResult([]string{"USD", "gbp"}, ticker, "")
	if err != nil {
		t.Fatalf("getFiatRatesResult returned error: %v", err)
	}

	want := &FiatTicker{
		Timestamp: ticker.Timestamp.UTC().Unix(),
		Rates: map[string]float32{
			"usd": 1.23,
			"gbp": -1,
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected fiat ticker: got %+v, want %+v", got, want)
	}
}

func TestGetFiatRatesResult_NonTokenAllCurrenciesReturnsCopy(t *testing.T) {
	w := &Worker{}
	ticker := &common.CurrencyRatesTicker{
		Timestamp: time.Unix(1700000001, 0),
		Rates: map[string]float32{
			"usd": 1.5,
			"eur": 1.2,
		},
	}

	got, err := w.getFiatRatesResult(nil, ticker, "")
	if err != nil {
		t.Fatalf("getFiatRatesResult returned error: %v", err)
	}
	if !reflect.DeepEqual(got.Rates, ticker.Rates) {
		t.Fatalf("unexpected all-rates result: got %v, want %v", got.Rates, ticker.Rates)
	}

	got.Rates["usd"] = 999
	if ticker.Rates["usd"] == 999 {
		t.Fatalf("ticker rates were modified through result map")
	}
}

func TestGetFiatRatesResult_TokenRates(t *testing.T) {
	w := &Worker{}
	ticker := &common.CurrencyRatesTicker{
		Timestamp: time.Unix(1700000002, 0),
		Rates: map[string]float32{
			"usd": 2,
			"eur": 3,
		},
		TokenRates: map[string]float32{
			"0xtoken": 4,
		},
	}

	got, err := w.getFiatRatesResult([]string{"USD", "EUR", "JPY"}, ticker, "0xToken")
	if err != nil {
		t.Fatalf("getFiatRatesResult returned error: %v", err)
	}
	want := map[string]float32{
		"usd": 8,
		"eur": 12,
		"jpy": -1,
	}
	if !reflect.DeepEqual(got.Rates, want) {
		t.Fatalf("unexpected token rates: got %v, want %v", got.Rates, want)
	}
}

func TestGetCurrentFiatRates_UsesGetterAndCurrencyFilter(t *testing.T) {
	w := &Worker{fiatRates: &fiat.FiatRates{}}
	originalGetter := getCurrentTicker
	defer func() {
		getCurrentTicker = originalGetter
	}()

	ticker := &common.CurrencyRatesTicker{
		Timestamp: time.Unix(1700000003, 0),
		Rates:     map[string]float32{"usd": 1.01},
	}
	calls := 0
	gotVsCurrency := ""
	gotToken := ""
	getCurrentTicker = func(_ *fiat.FiatRates, vsCurrency string, token string) *common.CurrencyRatesTicker {
		calls++
		gotVsCurrency = vsCurrency
		gotToken = token
		return ticker
	}

	got, err := w.GetCurrentFiatRates([]string{"", "USD"}, "")
	if err != nil {
		t.Fatalf("GetCurrentFiatRates returned error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected one ticker call, got %d", calls)
	}
	if gotVsCurrency != "USD" {
		t.Fatalf("unexpected vsCurrency: got %q, want %q", gotVsCurrency, "USD")
	}
	if gotToken != "" {
		t.Fatalf("unexpected token: got %q, want empty", gotToken)
	}
	wantRates := map[string]float32{"usd": 1.01}
	if !reflect.DeepEqual(got.Rates, wantRates) {
		t.Fatalf("unexpected rates: got %v, want %v", got.Rates, wantRates)
	}
}

func TestGetCurrentFiatRates_TokenWithoutTickerReturnsPublicError(t *testing.T) {
	w := &Worker{fiatRates: &fiat.FiatRates{}}
	originalGetter := getCurrentTicker
	defer func() {
		getCurrentTicker = originalGetter
	}()

	getCurrentTicker = func(_ *fiat.FiatRates, _, _ string) *common.CurrencyRatesTicker {
		return nil
	}

	_, err := w.GetCurrentFiatRates(nil, "0xtoken")
	apiErr := requireAPIError(t, err, true)
	if apiErr.Text != "No tickers found!" {
		t.Fatalf("unexpected error text: got %q", apiErr.Text)
	}
}

func TestGetFiatRatesForTimestamps_EmptyInput(t *testing.T) {
	w := &Worker{}
	_, err := w.GetFiatRatesForTimestamps(nil, []string{"usd"}, "")
	apiErr := requireAPIError(t, err, true)
	if apiErr.Text != "No timestamps provided" {
		t.Fatalf("unexpected error text: got %q", apiErr.Text)
	}
}

func TestGetFiatRatesForTimestamps_LenMismatchReturnsNonPublicError(t *testing.T) {
	w := &Worker{fiatRates: &fiat.FiatRates{}}
	originalGetter := getTickersForTimestamps
	defer func() {
		getTickersForTimestamps = originalGetter
	}()

	getTickersForTimestamps = func(_ *fiat.FiatRates, _ []int64, _, _ string) (*[]*common.CurrencyRatesTicker, error) {
		tickers := []*common.CurrencyRatesTicker{
			{Timestamp: time.Unix(1700000004, 0), Rates: map[string]float32{"usd": 1}},
		}
		return &tickers, nil
	}

	_, err := w.GetFiatRatesForTimestamps([]int64{1, 2}, []string{"usd"}, "")
	apiErr := requireAPIError(t, err, false)
	if apiErr.Text != "No tickers found" {
		t.Fatalf("unexpected error text: got %q", apiErr.Text)
	}
}

func TestGetFiatRatesForTimestamps_NilTickerEntryFallsBackToErrorRates(t *testing.T) {
	w := &Worker{fiatRates: &fiat.FiatRates{}}
	originalGetter := getTickersForTimestamps
	defer func() {
		getTickersForTimestamps = originalGetter
	}()

	getTickersForTimestamps = func(_ *fiat.FiatRates, timestamps []int64, vsCurrency, token string) (*[]*common.CurrencyRatesTicker, error) {
		if !reflect.DeepEqual(timestamps, []int64{100, 200}) {
			t.Fatalf("unexpected timestamps: got %v", timestamps)
		}
		if vsCurrency != "" || token != "" {
			t.Fatalf("unexpected lookup args: vsCurrency=%q token=%q", vsCurrency, token)
		}
		tickers := []*common.CurrencyRatesTicker{
			{Timestamp: time.Unix(1700000005, 0), Rates: map[string]float32{"usd": 1.5}},
			nil,
		}
		return &tickers, nil
	}

	got, err := w.GetFiatRatesForTimestamps([]int64{100, 200}, []string{"USD", "EUR"}, "")
	if err != nil {
		t.Fatalf("GetFiatRatesForTimestamps returned error: %v", err)
	}
	if len(got.Tickers) != 2 {
		t.Fatalf("unexpected ticker count: got %d, want 2", len(got.Tickers))
	}
	if !reflect.DeepEqual(got.Tickers[0].Rates, map[string]float32{"usd": 1.5, "eur": -1}) {
		t.Fatalf("unexpected first ticker rates: %v", got.Tickers[0].Rates)
	}
	if got.Tickers[1].Timestamp != 200 {
		t.Fatalf("unexpected fallback timestamp: got %d, want 200", got.Tickers[1].Timestamp)
	}
	if !reflect.DeepEqual(got.Tickers[1].Rates, map[string]float32{"usd": -1, "eur": -1}) {
		t.Fatalf("unexpected fallback rates: %v", got.Tickers[1].Rates)
	}
}

func TestGetAvailableVsCurrencies_SortedAndDeterministic(t *testing.T) {
	w := &Worker{fiatRates: &fiat.FiatRates{}}
	originalGetter := getTickersForTimestamps
	defer func() {
		getTickersForTimestamps = originalGetter
	}()

	getTickersForTimestamps = func(_ *fiat.FiatRates, timestamps []int64, vsCurrency, token string) (*[]*common.CurrencyRatesTicker, error) {
		if !reflect.DeepEqual(timestamps, []int64{123}) {
			t.Fatalf("unexpected timestamps: got %v", timestamps)
		}
		if vsCurrency != "" || token != "0xtoken" {
			t.Fatalf("unexpected lookup args: vsCurrency=%q token=%q", vsCurrency, token)
		}
		tickers := []*common.CurrencyRatesTicker{
			{
				Timestamp: time.Unix(1700000006, 0),
				Rates: map[string]float32{
					"usd": 1,
					"cad": 2,
					"eur": 3,
				},
			},
		}
		return &tickers, nil
	}

	got, err := w.GetAvailableVsCurrencies(123, "0xtoken")
	if err != nil {
		t.Fatalf("GetAvailableVsCurrencies returned error: %v", err)
	}
	if !reflect.DeepEqual(got.Tickers, []string{"cad", "eur", "usd"}) {
		t.Fatalf("unexpected sorted tickers: got %v", got.Tickers)
	}
	if got.Timestamp != 1700000006 {
		t.Fatalf("unexpected timestamp: got %d", got.Timestamp)
	}
}

func TestGetAvailableVsCurrencies_PropagatesProviderErrorAsNonPublic(t *testing.T) {
	w := &Worker{fiatRates: &fiat.FiatRates{}}
	originalGetter := getTickersForTimestamps
	defer func() {
		getTickersForTimestamps = originalGetter
	}()

	getTickersForTimestamps = func(_ *fiat.FiatRates, _ []int64, _, _ string) (*[]*common.CurrencyRatesTicker, error) {
		return nil, fiatRatesTestError("provider failure")
	}

	_, err := w.GetAvailableVsCurrencies(123, "")
	apiErr := requireAPIError(t, err, false)
	if !strings.Contains(apiErr.Text, "provider failure") {
		t.Fatalf("unexpected error text: got %q", apiErr.Text)
	}
}

func TestGetAvailableVsCurrencies_NilFirstTickerReturnsPublicError(t *testing.T) {
	w := &Worker{fiatRates: &fiat.FiatRates{}}
	originalGetter := getTickersForTimestamps
	defer func() {
		getTickersForTimestamps = originalGetter
	}()

	getTickersForTimestamps = func(_ *fiat.FiatRates, _ []int64, _, _ string) (*[]*common.CurrencyRatesTicker, error) {
		tickers := []*common.CurrencyRatesTicker{nil}
		return &tickers, nil
	}

	_, err := w.GetAvailableVsCurrencies(123, "0xtoken")
	apiErr := requireAPIError(t, err, true)
	if apiErr.Text != "No tickers found" {
		t.Fatalf("unexpected error text: got %q", apiErr.Text)
	}
}

type fiatRatesTestError string

func (e fiatRatesTestError) Error() string {
	return string(e)
}
