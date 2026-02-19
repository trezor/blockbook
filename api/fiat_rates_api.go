package api

import (
	"fmt"
	"sort"
	"strings"

	"github.com/golang/glog"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
)

// removeEmpty removes empty strings from a slice.
func removeEmpty(stringSlice []string) []string {
	ret := make([]string, 0, len(stringSlice))
	for _, str := range stringSlice {
		if str != "" {
			ret = append(ret, str)
		}
	}
	return ret
}

func copyTickerRates(rates map[string]float32) map[string]float32 {
	copied := make(map[string]float32, len(rates))
	for k, v := range rates {
		copied[k] = v
	}
	return copied
}

// getFiatRatesResult checks if CurrencyRatesTicker contains all necessary data and returns formatted result.
func (w *Worker) getFiatRatesResult(currencies []string, ticker *common.CurrencyRatesTicker, token string) (*FiatTicker, error) {
	if token != "" {
		capacity := len(currencies)
		if capacity == 0 {
			capacity = len(ticker.Rates)
		}
		rates := make(map[string]float32, capacity)
		if len(currencies) == 0 {
			for currency := range ticker.Rates {
				currency = strings.ToLower(currency)
				rate := ticker.TokenRateInCurrency(token, currency)
				if rate <= 0 {
					rate = -1
				}
				rates[currency] = rate
			}
		} else {
			for _, currency := range currencies {
				currency = strings.ToLower(currency)
				rate := ticker.TokenRateInCurrency(token, currency)
				if rate <= 0 {
					rate = -1
				}
				rates[currency] = rate
			}
		}
		return &FiatTicker{
			Timestamp: ticker.Timestamp.UTC().Unix(),
			Rates:     rates,
		}, nil
	}
	if len(currencies) == 0 {
		// Return all available ticker rates.
		return &FiatTicker{
			Timestamp: ticker.Timestamp.UTC().Unix(),
			Rates:     copyTickerRates(ticker.Rates),
		}, nil
	}
	// Check if currencies from the list are available in the ticker rates.
	rates := make(map[string]float32, len(currencies))
	for _, currency := range currencies {
		currency = strings.ToLower(currency)
		if rate, found := ticker.Rates[currency]; found {
			rates[currency] = rate
		} else {
			rates[currency] = -1
		}
	}
	return &FiatTicker{
		Timestamp: ticker.Timestamp.UTC().Unix(),
		Rates:     rates,
	}, nil
}

// GetCurrentFiatRates returns last available fiat rates.
func (w *Worker) GetCurrentFiatRates(currencies []string, token string) (*FiatTicker, error) {
	vsCurrency := ""
	currencies = removeEmpty(currencies)
	if len(currencies) == 1 {
		vsCurrency = currencies[0]
	}
	ticker := getCurrentTicker(w.fiatRates, vsCurrency, token)
	var err error
	if ticker == nil {
		if token == "" {
			// fallback - get last fiat rate from db if not in current ticker
			// not for tokens, many tokens do not have fiat rates at all and it is very costly
			// to do DB search for token without an exchange rate
			ticker, err = w.db.FiatRatesFindLastTicker(vsCurrency, token)
		}
		if err != nil {
			return nil, NewAPIError(fmt.Sprintf("Error finding ticker: %v", err), false)
		} else if ticker == nil {
			return nil, NewAPIError("No tickers found!", true)
		}
	}
	result, err := w.getFiatRatesResult(currencies, ticker, token)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// makeErrorRates returns a map of currencies, with each value equal to -1
// used when there was an error finding ticker.
func makeErrorRates(currencies []string) map[string]float32 {
	rates := make(map[string]float32, len(currencies))
	for _, currency := range currencies {
		rates[strings.ToLower(currency)] = -1
	}
	return rates
}

// GetFiatRatesForTimestamps returns fiat rates for each of the provided dates.
func (w *Worker) GetFiatRatesForTimestamps(timestamps []int64, currencies []string, token string) (*FiatTickers, error) {
	if len(timestamps) == 0 {
		return nil, NewAPIError("No timestamps provided", true)
	}
	vsCurrency := ""
	currencies = removeEmpty(currencies)
	if len(currencies) == 1 {
		vsCurrency = currencies[0]
	}
	tickers, err := getTickersForTimestamps(w.fiatRates, timestamps, vsCurrency, token)
	if err != nil {
		return nil, err
	}
	if tickers == nil {
		return nil, NewAPIError("No tickers found", true)
	}
	if len(*tickers) != len(timestamps) {
		glog.Error("GetFiatRatesForTimestamps: number of tickers does not match timestamps ", len(*tickers), ", ", len(timestamps))
		return nil, NewAPIError("No tickers found", false)
	}
	fiatTickers := make([]FiatTicker, len(*tickers))
	for i, t := range *tickers {
		if t == nil {
			fiatTickers[i] = FiatTicker{Timestamp: timestamps[i], Rates: makeErrorRates(currencies)}
			continue
		}
		result, err := w.getFiatRatesResult(currencies, t, token)
		if err != nil {
			if apiErr, ok := err.(*APIError); ok {
				if apiErr.Public {
					return nil, err
				}
			}
			fiatTickers[i] = FiatTicker{Timestamp: timestamps[i], Rates: makeErrorRates(currencies)}
			continue
		}
		fiatTickers[i] = *result
	}
	return &FiatTickers{Tickers: fiatTickers}, nil
}

// GetFiatRatesForBlockID returns fiat rates for block height or block hash.
func (w *Worker) GetFiatRatesForBlockID(blockID string, currencies []string, token string) (*FiatTicker, error) {
	bi, err := w.getBlockInfoFromBlockID(blockID)
	if err != nil {
		if err == bchain.ErrBlockNotFound {
			return nil, NewAPIError(fmt.Sprintf("Block %v not found", blockID), true)
		}
		return nil, NewAPIError(fmt.Sprintf("Block %v not found, error: %v", blockID, err), false)
	}
	tickers, err := w.GetFiatRatesForTimestamps([]int64{bi.Time}, currencies, token)
	if err != nil || tickers == nil || len(tickers.Tickers) == 0 {
		return nil, err
	}
	return &tickers.Tickers[0], nil
}

// GetAvailableVsCurrencies returns the list of available versus currencies for exchange rates.
func (w *Worker) GetAvailableVsCurrencies(timestamp int64, token string) (*AvailableVsCurrencies, error) {
	tickers, err := getTickersForTimestamps(w.fiatRates, []int64{timestamp}, "", token)
	if err != nil {
		return nil, NewAPIError(fmt.Sprintf("Error finding ticker: %v", err), false)
	}
	if tickers == nil || len(*tickers) == 0 {
		return nil, NewAPIError("No tickers found", true)
	}
	ticker := (*tickers)[0]
	if ticker == nil {
		return nil, NewAPIError("No tickers found", true)
	}
	keys := make([]string, 0, len(ticker.Rates))
	for k := range ticker.Rates {
		keys = append(keys, k)
	}
	sort.Strings(keys) // sort to get deterministic results

	return &AvailableVsCurrencies{
		Timestamp: ticker.Timestamp.Unix(),
		Tickers:   keys,
	}, nil
}
