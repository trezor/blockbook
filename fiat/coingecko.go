package fiat

import (
	"blockbook/db"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	"github.com/golang/glog"
)

// Coingecko is a structure that implements RatesDownloaderInterface
type Coingecko struct {
	url                string
	coin               string
	httpTimeoutSeconds time.Duration
	timeFormat         string
}

// NewCoinGeckoDownloader creates a coingecko structure that implements the RatesDownloaderInterface
func NewCoinGeckoDownloader(url string, coin string, timeFormat string) RatesDownloaderInterface {
	return &Coingecko{
		url:                url,
		coin:               coin,
		httpTimeoutSeconds: 15 * time.Second,
		timeFormat:         timeFormat,
	}
}

// makeRequest retrieves the response from Coingecko API at the specified date.
// If timestamp is nil, it fetches the latest market data available.
func (cg *Coingecko) makeRequest(timestamp *time.Time) ([]byte, error) {
	requestURL := cg.url + "/coins/" + cg.coin
	if timestamp != nil {
		requestURL += "/history"
	}

	req, err := http.NewRequest("GET", requestURL, nil)
	if err != nil {
		glog.Errorf("Error creating a new request for %v: %v", requestURL, err)
		return nil, err
	}
	req.Close = true
	req.Header.Set("Content-Type", "application/json")

	// Add query parameters
	q := req.URL.Query()

	// Add a unix timestamp to query parameters to get uncached responses
	currentTimestamp := strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
	q.Add("current_timestamp", currentTimestamp)

	if timestamp == nil {
		q.Add("market_data", "true")
		q.Add("localization", "false")
		q.Add("tickers", "false")
		q.Add("community_data", "false")
		q.Add("developer_data", "false")
	} else {
		timestampFormatted := timestamp.Format(cg.timeFormat)
		q.Add("date", timestampFormatted)
	}
	req.URL.RawQuery = q.Encode()

	client := &http.Client{
		Timeout: cg.httpTimeoutSeconds,
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("Invalid response status: " + string(resp.Status))
	}
	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return bodyBytes, nil
}

// GetData gets fiat rates from API at the specified date and returns a CurrencyRatesTicker
// If timestamp is nil, it will download the current fiat rates.
func (cg *Coingecko) getTicker(timestamp *time.Time) (*db.CurrencyRatesTicker, error) {
	dataTimestamp := timestamp
	if timestamp == nil {
		timeNow := time.Now()
		dataTimestamp = &timeNow
	}
	dataTimestampUTC := dataTimestamp.UTC()
	ticker := &db.CurrencyRatesTicker{Timestamp: &dataTimestampUTC}
	bodyBytes, err := cg.makeRequest(timestamp)
	if err != nil {
		return nil, err
	}

	type FiatRatesResponse struct {
		MarketData struct {
			Prices map[string]float64 `json:"current_price"`
		} `json:"market_data"`
	}

	var data FiatRatesResponse
	err = json.Unmarshal(bodyBytes, &data)
	if err != nil {
		glog.Errorf("Error parsing FiatRates response: %v", err)
		return nil, err
	}
	ticker.Rates = data.MarketData.Prices
	return ticker, nil
}

// MarketDataExists checks if there's data available for the specific timestamp.
func (cg *Coingecko) marketDataExists(timestamp *time.Time) (bool, error) {
	resp, err := cg.makeRequest(timestamp)
	if err != nil {
		glog.Error("Error getting market data: ", err)
		return false, err
	}
	type FiatRatesResponse struct {
		MarketData struct {
			Prices map[string]interface{} `json:"current_price"`
		} `json:"market_data"`
	}
	var data FiatRatesResponse
	err = json.Unmarshal(resp, &data)
	if err != nil {
		glog.Errorf("Error parsing Coingecko response: %v", err)
		return false, err
	}
	return len(data.MarketData.Prices) != 0, nil
}
