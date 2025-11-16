package common

import (
	"strings"
	"time"
)

// CurrencyRatesTicker contains coin ticker data fetched from API
type CurrencyRatesTicker struct {
	Timestamp  time.Time          `json:"timestamp"`            // return as unix timestamp in API
	Rates      map[string]float32 `json:"rates"`                // rates of the base currency against a list of vs currencies
	TokenRates map[string]float32 `json:"tokenRates,omitempty"` // rates of the tokens (identified by the address of the contract) against the base currency
}

var (
	// TickerRecalculateTokenRate signals if it is necessary to recalculate token rate to base rate
	// this happens when token rates are downloaded in TokenVsCurrency different from the base currency
	TickerRecalculateTokenRate bool
	// TickerTokenVsCurrency is the currency in which the token rates are downloaded
	TickerTokenVsCurrency string
)

// Convert returns token rate in base currency
func (t *CurrencyRatesTicker) GetTokenRate(token string) (float32, bool) {
	if t.TokenRates != nil {
		rate, found := t.TokenRates[strings.ToLower(token)]
		if !found {
			return 0, false
		}
		if TickerRecalculateTokenRate {
			vsRate, found := t.Rates[TickerTokenVsCurrency]
			if !found || vsRate == 0 {
				return 0, false
			}
			rate = rate / vsRate
		}
		return rate, found
	}
	return 0, false
}

// Convert converts value in base currency to toCurrency
func (t *CurrencyRatesTicker) Convert(baseValue float64, toCurrency string) float64 {
	rate, found := t.Rates[toCurrency]
	if !found {
		return 0
	}
	return baseValue * float64(rate)
}

// ConvertTokenToBase converts token value to base currency
func (t *CurrencyRatesTicker) ConvertTokenToBase(value float64, token string) float64 {
	rate, found := t.GetTokenRate(token)
	if found {
		return value * float64(rate)
	}
	return 0
}

// ConvertToken converts token value to toCurrency currency
func (t *CurrencyRatesTicker) ConvertToken(value float64, token string, toCurrency string) float64 {
	baseValue := t.ConvertTokenToBase(value, token)
	if baseValue > 0 {
		return t.Convert(baseValue, toCurrency)
	}
	return 0
}

// TokenRateInCurrency return token rate in toCurrency currency
func (t *CurrencyRatesTicker) TokenRateInCurrency(token string, toCurrency string) float32 {
	rate, found := t.GetTokenRate(token)
	if found {
		baseRate, found := t.Rates[toCurrency]
		if found {
			return baseRate * rate
		}
	}
	return 0
}

// IsSuitableTicker checks if the ticker can provide data for given vsCurrency or token
func IsSuitableTicker(ticker *CurrencyRatesTicker, vsCurrency string, token string) bool {
	if vsCurrency != "" {
		if ticker.Rates == nil {
			return false
		}
		if _, found := ticker.Rates[vsCurrency]; !found {
			return false
		}
	}
	if token != "" {
		if ticker.TokenRates == nil {
			return false
		}
		if _, found := ticker.TokenRates[token]; !found {
			return false
		}
	}
	return true
}
