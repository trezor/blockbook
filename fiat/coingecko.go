package fiat

import (
	"blockbook/db"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/golang/glog"
)

// CoingeckoDownloader stores CoinGecko API parameters
type CoingeckoDownloader struct {
	url                string
	coin               string
	periodSeconds      int
	db                 *db.RocksDB
	timeFormat         string
	httpTimeoutSeconds time.Duration
	test               bool
}

// NewCoingeckoDownloader initiallizes the downloader for CoinGecko API.
// If the "test" flag is true, then downloader will sync only the last few days instead of the whole history.
func NewCoingeckoDownloader(db *db.RocksDB, params string, test bool) (*CoingeckoDownloader, error) {
	var cg = &CoingeckoDownloader{}
	type coingeckoParams struct {
		URL           string `json:"url"`
		Coin          string `json:"coin"`
		PeriodSeconds int    `json:"periodSeconds"`
	}
	cgParams := &coingeckoParams{}
	err := json.Unmarshal([]byte(params), &cgParams)
	if err != nil {
		return nil, err
	}
	if cgParams.URL == "" || cgParams.PeriodSeconds == 0 {
		return nil, errors.New("Missing parameters")
	}
	cg.timeFormat = "02-01-2006" // Layout string for CoinGecko date formatting (DD-MM-YYYY)
	cg.httpTimeoutSeconds = 15 * time.Second
	cg.url = cgParams.URL
	cg.coin = cgParams.Coin
	cg.periodSeconds = cgParams.PeriodSeconds // Time period for syncing the latest market data
	cg.db = db
	cg.test = test
	return cg, err
}

// GetMarketData retrieves the response from coingecko API at the specified date.
// If timestamp is nil, it fetches the latest market data available.
func (cg *CoingeckoDownloader) GetMarketData(timestamp *time.Time) ([]byte, error) {
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

// GetData gets fiat rates from coingecko.com at the specified date and returns JSON string.
// If timestamp is nil, it will download the latest market data.
func (cg *CoingeckoDownloader) GetData(timestamp *time.Time) (string, error) {
	bodyBytes, err := cg.GetMarketData(timestamp)
	if err != nil {
		return "", err
	}

	type CoinGeckoResponse struct {
		MarketData struct {
			Prices map[string]interface{} `json:"current_price"`
		} `json:"market_data"`
	}

	var data CoinGeckoResponse
	err = json.Unmarshal(bodyBytes, &data)
	if err != nil {
		glog.Errorf("Error parsing CoinGecko response: %v", err)
		return "", err
	}

	jsonString, err := json.Marshal(data.MarketData.Prices)
	if err != nil {
		glog.Errorf("Error marshalling CoinGecko prices: %v", err)
		return "", err
	}
	return string(jsonString), nil
}

// MarketDataExists checks if there's data available for the specific timestamp.
func (cg *CoingeckoDownloader) MarketDataExists(timestamp *time.Time) (bool, error) {
	resp, err := cg.GetMarketData(timestamp)
	if err != nil {
		glog.Error("Error getting market data: ", err)
		return false, err
	}
	type CoinGeckoResponse struct {
		MarketData struct {
			Prices map[string]interface{} `json:"current_price"`
		} `json:"market_data"`
	}
	var data CoinGeckoResponse
	err = json.Unmarshal(resp, &data)
	if err != nil {
		glog.Errorf("Error parsing CoinGecko response: %v", err)
		return false, err
	}
	return len(data.MarketData.Prices) != 0, nil
}

// FindEarliestMarketData uses binary search to find the oldest market data available on CoinGecko.
func (cg *CoingeckoDownloader) FindEarliestMarketData() (*time.Time, error) {
	minDateString := "03-01-2009"
	minDate, err := time.Parse(cg.timeFormat, minDateString)
	if err != nil {
		glog.Error("Error parsing date: ", err)
		return nil, err
	}
	var lastDate time.Time
	maxDate := time.Now()
	for {
		dataExists, err := cg.MarketDataExists(&maxDate)
		if err != nil {
			return nil, err
		}
		if dataExists {
			lastDate = maxDate
			dateDiff := maxDate.Sub(minDate)
			if dateDiff < time.Hour*24 {
				lastDate := time.Date(lastDate.Year(), lastDate.Month(), lastDate.Day(), 0, 0, 0, 0, lastDate.Location()) // truncate time to day
				return &lastDate, nil
			}
			maxDate = maxDate.Add(-1 * dateDiff / 2)
		} else {
			minDate = maxDate
			maxDate = maxDate.Add(lastDate.Sub(maxDate) / 2)
		}
	}
}

// SyncLatest downloads the latest data every cg.PeriodSeconds
func (cg *CoingeckoDownloader) SyncLatest() error {
	period := time.Duration(cg.periodSeconds) * time.Second
	if cg.test {
		// Use lesser period for tests
		period = time.Duration(2) * time.Second
	}
	timer := time.NewTimer(period)
	for {
		currentTime := time.Now()
		data, err := cg.GetData(nil)
		if err != nil {
			// Do not exit on GET error, log it, wait and try again
			glog.Errorf("Sync GetData error: %v", err)
			<-timer.C
			timer.Reset(period)
			continue
		}

		err = cg.db.StoreTicker(currentTime, data)
		if err != nil {
			glog.Errorf("Sync StoreTicker error for time %v", currentTime)
			return err
		}
		if cg.test {
			break
		}
		<-timer.C
		timer.Reset(period)
	}
	return nil
}

// Sync downloads all the historical data since the specified timestamp till today,
// then continues to download the latest rates
func (cg *CoingeckoDownloader) Sync(timestamp *time.Time) error {
	period := time.Duration(1) * time.Second
	timer := time.NewTimer(period)
	for {
		data, err := cg.GetData(timestamp)
		if err != nil {
			glog.Errorf("SyncHistorical GetData error: %v", err)
			return err
		}

		err = cg.db.StoreTicker(*timestamp, data)
		if err != nil {
			glog.Errorf("SyncHistorical error storing ticker for %v", timestamp)
			return err
		}

		*timestamp = timestamp.Add(time.Hour * 24) // go to the next day

		if time.Now().Sub(*timestamp) < time.Duration(time.Hour*24) {
			break
		}

		<-timer.C
		timer.Reset(period)
	}
	return cg.SyncLatest()
}

// Run starts the CoinGecko downloader. If there are tickers available, it continues from the last record.
// If there are no tickers, it finds the earliest market data available on CoinGecko and downloads historical data.
// When historical data is downloaded, it continues to fetch the latest ticker prices.
func (cg *CoingeckoDownloader) Run() error {
	var timestamp *time.Time

	// Check if there are any tickers stored in database
	ticker, err := cg.db.FindLastTicker()
	if err != nil {
		glog.Errorf("CoingeckoDownloader FindTicker error: %v", err)
		return err
	}

	if len(ticker.Rates) == 0 {
		// If no tickers found, start downloading from the beginning
		timestamp, err = cg.FindEarliestMarketData()
		if err != nil {
			glog.Errorf("Error looking up earliest market data: %v", err)
			return err
		}
		if cg.test {
			// When testing, start from 2 days ago instead of the beginning (2013)
			*timestamp = time.Now().Add(time.Duration(-24*2) * time.Hour)
		}
	} else {
		// If found, continue downloading data from the last available record
		timestamp, err = db.ConvertDate(ticker.Timestamp)
		if err != nil {
			glog.Errorf("Timestamp conversion error: %v", err)
			return err
		}
	}
	return cg.Sync(timestamp)
}
