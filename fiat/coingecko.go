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

// RatesDownloader stores FiatRates API parameters
type RatesDownloader struct {
	url                string
	coin               string
	periodSeconds      int
	db                 *db.RocksDB
	timeFormat         string
	httpTimeoutSeconds time.Duration
	test               bool
}

// NewFiatRatesDownloader initiallizes the downloader for FiatRates API.
// If the "test" flag is true, then downloader will sync only the last few days instead of the whole history.
func NewFiatRatesDownloader(db *db.RocksDB, params string, test bool) (*RatesDownloader, error) {
	var rd = &RatesDownloader{}
	type fiatRatesParams struct {
		URL           string `json:"url"`
		Coin          string `json:"coin"`
		PeriodSeconds int    `json:"periodSeconds"`
	}
	rdParams := &fiatRatesParams{}
	err := json.Unmarshal([]byte(params), &rdParams)
	if err != nil {
		return nil, err
	}
	if rdParams.URL == "" || rdParams.PeriodSeconds == 0 {
		return nil, errors.New("Missing parameters")
	}
	rd.timeFormat = "02-01-2006" // Layout string for FiatRates date formatting (DD-MM-YYYY)
	rd.httpTimeoutSeconds = 15 * time.Second
	rd.url = rdParams.URL
	rd.coin = rdParams.Coin
	rd.periodSeconds = rdParams.PeriodSeconds // Time period for syncing the latest market data
	rd.db = db
	rd.test = test
	return rd, err
}

// GetMarketData retrieves the response from fiatRates API at the specified date.
// If timestamp is nil, it fetches the latest market data available.
func (rd *RatesDownloader) GetMarketData(timestamp *time.Time) ([]byte, error) {
	requestURL := rd.url + "/coins/" + rd.coin
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
		timestampFormatted := timestamp.Format(rd.timeFormat)
		q.Add("date", timestampFormatted)
	}
	req.URL.RawQuery = q.Encode()

	client := &http.Client{
		Timeout: rd.httpTimeoutSeconds,
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

// GetData gets fiat rates from API at the specified date and returns JSON string.
// If timestamp is nil, it will download the latest market data.
func (rd *RatesDownloader) GetData(timestamp *time.Time) (string, error) {
	bodyBytes, err := rd.GetMarketData(timestamp)
	if err != nil {
		return "", err
	}

	type FiatRatesResponse struct {
		MarketData struct {
			Prices map[string]interface{} `json:"current_price"`
		} `json:"market_data"`
	}

	var data FiatRatesResponse
	err = json.Unmarshal(bodyBytes, &data)
	if err != nil {
		glog.Errorf("Error parsing FiatRates response: %v", err)
		return "", err
	}

	jsonString, err := json.Marshal(data.MarketData.Prices)
	if err != nil {
		glog.Errorf("Error marshalling FiatRates prices: %v", err)
		return "", err
	}
	return string(jsonString), nil
}

// MarketDataExists checks if there's data available for the specific timestamp.
func (rd *RatesDownloader) MarketDataExists(timestamp *time.Time) (bool, error) {
	resp, err := rd.GetMarketData(timestamp)
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
		glog.Errorf("Error parsing FiatRates response: %v", err)
		return false, err
	}
	return len(data.MarketData.Prices) != 0, nil
}

// FindEarliestMarketData uses binary search to find the oldest market data available on API.
func (rd *RatesDownloader) FindEarliestMarketData() (*time.Time, error) {
	minDateString := "03-01-2009"
	minDate, err := time.Parse(rd.timeFormat, minDateString)
	if err != nil {
		glog.Error("Error parsing date: ", err)
		return nil, err
	}
	maxDate := time.Now().Add(time.Duration(-24) * time.Hour) // today's historical tickers may not be ready yet, so set to yesterday
	currentDate := maxDate
	for {
		dataExists, err := rd.MarketDataExists(&currentDate)
		if err != nil {
			return nil, err
		}
		dateDiff := currentDate.Sub(minDate)
		if dataExists {
			if dateDiff < time.Hour*24 {
				maxDate := time.Date(maxDate.Year(), maxDate.Month(), maxDate.Day(), 0, 0, 0, 0, maxDate.Location()) // truncate time to day
				return &maxDate, nil
			}
			maxDate = currentDate
			currentDate = currentDate.Add(-1 * dateDiff / 2)
		} else {
			minDate = currentDate
			currentDate = currentDate.Add(maxDate.Sub(currentDate) / 2)
		}
	}
}

// SyncLatest downloads the latest data every rd.PeriodSeconds
func (rd *RatesDownloader) SyncLatest() error {
	period := time.Duration(rd.periodSeconds) * time.Second
	if rd.test {
		// Use lesser period for tests
		period = time.Duration(2) * time.Second
	}
	timer := time.NewTimer(period)
	for {
		currentTime := time.Now()
		data, err := rd.GetData(nil)
		if err != nil {
			// Do not exit on GET error, log it, wait and try again
			glog.Errorf("Sync GetData error: %v", err)
			<-timer.C
			timer.Reset(period)
			continue
		}

		err = rd.db.StoreTicker(currentTime, data)
		if err != nil {
			glog.Errorf("Sync StoreTicker error for time %v", currentTime)
			return err
		}
		if rd.test {
			break
		}
		<-timer.C
		timer.Reset(period)
	}
	return nil
}

// Sync downloads all the historical data since the specified timestamp till today,
// then continues to download the latest rates
func (rd *RatesDownloader) Sync(timestamp *time.Time) error {
	period := time.Duration(1) * time.Second
	timer := time.NewTimer(period)
	for {
		data, err := rd.GetData(timestamp)
		if err != nil {
			glog.Errorf("SyncHistorical GetData error: %v", err)
			return err
		}

		err = rd.db.StoreTicker(*timestamp, data)
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
	return rd.SyncLatest()
}

// Run starts the FiatRates downloader. If there are tickers available, it continues from the last record.
// If there are no tickers, it finds the earliest market data available on API and downloads historical data.
// When historical data is downloaded, it continues to fetch the latest ticker prices.
func (rd *RatesDownloader) Run() error {
	var timestamp *time.Time

	// Check if there are any tickers stored in database
	ticker, err := rd.db.FindLastTicker()
	if err != nil {
		glog.Errorf("RatesDownloader FindTicker error: %v", err)
		return err
	}

	if len(ticker.Rates) == 0 {
		// If no tickers found, start downloading from the beginning
		timestamp, err = rd.FindEarliestMarketData()
		if err != nil {
			glog.Errorf("Error looking up earliest market data: %v", err)
			return err
		}
		if rd.test {
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
	return rd.Sync(timestamp)
}
