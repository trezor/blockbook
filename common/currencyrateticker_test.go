package common

import (
	"testing"
)

func TestCurrencyRatesTicker_ConvertToken(t *testing.T) {
	ticker := &CurrencyRatesTicker{
		Rates: map[string]float32{
			"usd": 2129.987654321,
			"eur": 1332.12345678,
		},
		TokenRates: map[string]float32{
			"0x82df128257a7d7556262e1ab7f1f639d9775b85e": 0.4092341123,
			"0x6b175474e89094c44da98b954eedeac495271d0f": 12.32323232323232,
			"0xdac17f958d2ee523a2206206994597c13d831ec7": 1332421341235.51234,
		},
	}
	tests := []struct {
		name       string
		value      float64
		token      string
		toCurrency string
		want       float64
	}{
		{
			name:       "usd 0x82df128257a7d7556262e1ab7f1f639d9775b85e",
			value:      10,
			token:      "0x82df128257a7d7556262e1ab7f1f639d9775b85e",
			toCurrency: "usd",
			want:       8716.635514874506,
		},
		{
			name:       "eur 0xdac17f958d2ee523a2206206994597c13d831ec7",
			value:      23.123,
			token:      "0xdac17f958d2ee523a2206206994597c13d831ec7",
			toCurrency: "eur",
			want:       4.104216071804417e+16,
		},
		{
			name:       "eur 0xdac17f958d2ee523a2206206994597c13d831ec8",
			value:      23.123,
			token:      "0xdac17f958d2ee523a2206206994597c13d831ec8",
			toCurrency: "eur",
			want:       0,
		},
		{
			name:       "eur 0xdac17f958d2ee523a2206206994597c13d831ec7",
			value:      23.123,
			token:      "0xdac17f958d2ee523a2206206994597c13d831ec7",
			toCurrency: "czk",
			want:       0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ticker.ConvertToken(tt.value, tt.token, tt.toCurrency); got != tt.want {
				t.Errorf("CurrencyRatesTicker.ConvertToken() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCurrencyRatesTicker_GetTokenRate_UsesExactMatchForCaseSensitiveTokens(t *testing.T) {
	const tronUSDT = "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t"

	ticker := &CurrencyRatesTicker{
		Rates: map[string]float32{
			"trx": 1,
		},
		TokenRates: map[string]float32{
			tronUSDT: 9,
		},
	}

	got, found := ticker.GetTokenRate(tronUSDT)
	if !found {
		t.Fatalf("expected exact-match lookup to find tron token %q", tronUSDT)
	}
	if got != 9 {
		t.Fatalf("unexpected tron token rate: got %v, want %v", got, float32(9))
	}
}

func TestCurrencyRatesTicker_GetTokenRate_FallsBackToLowercaseForHexTokens(t *testing.T) {
	ticker := &CurrencyRatesTicker{
		Rates: map[string]float32{
			"usd": 1,
		},
		TokenRates: map[string]float32{
			"0xa4dd6bc15be95af55f0447555c8b6aa3088562f3": 1.2,
		},
	}

	got, found := ticker.GetTokenRate("0xA4DD6Bc15Be95Af55f0447555c8b6aA3088562f3")
	if !found {
		t.Fatal("expected mixed-case hex token lookup to fall back to lowercase")
	}
	if got != 1.2 {
		t.Fatalf("unexpected hex token rate: got %v, want %v", got, float32(1.2))
	}
}
