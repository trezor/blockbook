package api

import (
	"math"
	"testing"

	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/fiat"
)

func TestContractInfoIncludesProtocol(t *testing.T) {
	if !contractInfoIncludesProtocol([]string{" ERC4626 "}, contractInfoProtocolErc4626) {
		t.Fatal("expected erc4626 protocol to match case-insensitively")
	}
	if contractInfoIncludesProtocol([]string{"staking"}, contractInfoProtocolErc4626) {
		t.Fatal("unexpected erc4626 protocol match")
	}
}

func TestBuildContractInfoRates(t *testing.T) {
	originalGetter := getCurrentTicker
	defer func() {
		getCurrentTicker = originalGetter
	}()

	tickerCalls := 0
	getCurrentTicker = func(_ *fiat.FiatRates, vsCurrency string, token string) *common.CurrencyRatesTicker {
		tickerCalls++
		if vsCurrency != "usd" {
			t.Fatalf("unexpected currency lookup: got %q want %q", vsCurrency, "usd")
		}
		if token != "0xabc" {
			t.Fatalf("unexpected token lookup: got %q want %q", token, "0xabc")
		}
		return &common.CurrencyRatesTicker{
			Rates: map[string]float32{
				"usd": 2.5,
			},
			TokenRates: map[string]float32{
				"0xabc": 1.2,
			},
		}
	}

	w := &Worker{fiatRates: &fiat.FiatRates{}}
	rates := w.buildContractInfoRates("0xabc", erc4626EvmFungibleStandard(), "USD")
	if tickerCalls != 1 {
		t.Fatalf("expected one ticker lookup, got %d", tickerCalls)
	}
	if rates == nil {
		t.Fatal("expected rates")
	}
	if rates.Currency != "usd" {
		t.Fatalf("unexpected currency: %q", rates.Currency)
	}
	if math.Abs(rates.BaseRate-1.2) > 1e-6 {
		t.Fatalf("unexpected base rate: got %v want %v", rates.BaseRate, 1.2)
	}
	if math.Abs(rates.SecondaryRate-3.0) > 1e-6 {
		t.Fatalf("unexpected secondary rate: got %v want %v", rates.SecondaryRate, 3.0)
	}
}

func TestBuildContractInfoRatesSkipsUnsupportedStandards(t *testing.T) {
	originalGetter := getCurrentTicker
	defer func() {
		getCurrentTicker = originalGetter
	}()

	tickerCalls := 0
	getCurrentTicker = func(_ *fiat.FiatRates, _, _ string) *common.CurrencyRatesTicker {
		tickerCalls++
		return nil
	}

	w := &Worker{fiatRates: &fiat.FiatRates{}}
	rates := w.buildContractInfoRates("0xabc", bchain.ERC1155TokenStandard, "usd")
	if rates != nil {
		t.Fatalf("expected nil rates for unsupported standard, got %+v", rates)
	}
	if tickerCalls != 0 {
		t.Fatalf("expected no ticker lookups for unsupported standard, got %d", tickerCalls)
	}
}
