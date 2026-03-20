//go:build unittest

package api

import (
	"testing"

	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/fiat"
)

func TestGetSecondaryTicker_SkipsLookupWithoutSecondaryCurrency(t *testing.T) {
	w := &Worker{
		fiatRates: &fiat.FiatRates{Enabled: true},
	}
	originalGetter := getCurrentTicker
	defer func() {
		getCurrentTicker = originalGetter
	}()

	calls := 0
	getCurrentTicker = func(_ *fiat.FiatRates, _, _ string) *common.CurrencyRatesTicker {
		calls++
		return &common.CurrencyRatesTicker{}
	}

	ticker := w.getSecondaryTicker("")
	if ticker != nil {
		t.Fatalf("expected nil ticker when secondary currency is not requested, got %+v", ticker)
	}
	if calls != 0 {
		t.Fatalf("expected no ticker lookup call, got %d", calls)
	}
}

func TestGetSecondaryTicker_PerformsLookupWithSecondaryCurrency(t *testing.T) {
	w := &Worker{
		fiatRates: &fiat.FiatRates{Enabled: true},
	}
	originalGetter := getCurrentTicker
	defer func() {
		getCurrentTicker = originalGetter
	}()

	calls := 0
	expected := &common.CurrencyRatesTicker{Rates: map[string]float32{"usd": 1}}
	getCurrentTicker = func(_ *fiat.FiatRates, _, _ string) *common.CurrencyRatesTicker {
		calls++
		return expected
	}

	ticker := w.getSecondaryTicker("usd")
	if ticker != expected {
		t.Fatalf("unexpected ticker returned: got %+v, want %+v", ticker, expected)
	}
	if calls != 1 {
		t.Fatalf("expected one ticker lookup call, got %d", calls)
	}
}
