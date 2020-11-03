package fiat

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/golang/glog"
	"github.com/trezor/blockbook/db"
)

// OnNewFiatRatesTicker is used to send notification about a new FiatRates ticker
type OnNewFiatRatesTicker func(ticker *db.CurrencyRatesTicker)

// RatesDownloaderInterface provides method signatures for specific fiat rates downloaders
type RatesDownloaderInterface interface {
	getTicker(timestamp *time.Time) (*db.CurrencyRatesTicker, error)
	marketDataExists(timestamp *time.Time) (bool, error)
}

// RatesDownloader stores FiatRates API parameters
type RatesDownloader struct {
	periodSeconds       time.Duration
	db                  *db.RocksDB
	startTime           *time.Time // a starting timestamp for tests to be deterministic (time.Now() for production)
	timeFormat          string
	callbackOnNewTicker OnNewFiatRatesTicker
	downloader          RatesDownloaderInterface
}

// NewFiatRatesDownloader initiallizes the downloader for FiatRates API.
// If the startTime is nil, the downloader will start from the beginning.
func NewFiatRatesDownloader(db *db.RocksDB, apiType string, params string, startTime *time.Time, callback OnNewFiatRatesTicker) (*RatesDownloader, error) {
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
	rd.timeFormat = "02-01-2006"                                           // Layout string for FiatRates date formatting (DD-MM-YYYY)
	rd.periodSeconds = time.Duration(rdParams.PeriodSeconds) * time.Second // Time period for syncing the latest market data
	rd.db = db
	rd.callbackOnNewTicker = callback
	if startTime == nil {
		timeNow := time.Now().UTC()
		rd.startTime = &timeNow
	} else {
		rd.startTime = startTime // If startTime is nil, time.Now() will be used
	}
	if apiType == "coingecko" {
		rd.downloader = NewCoinGeckoDownloader(rdParams.URL, rdParams.Coin, rd.timeFormat)
	} else {
		return nil, fmt.Errorf("NewFiatRatesDownloader: incorrect API type %q", apiType)
	}
	return rd, nil
}

// Run starts the FiatRates downloader. If there are tickers available, it continues from the last record.
// If there are no tickers, it finds the earliest market data available on API and downloads historical data.
// When historical data is downloaded, it continues to fetch the latest ticker prices.
func (rd *RatesDownloader) Run() error {
	var timestamp *time.Time

	// Check if there are any tickers stored in database
	glog.Infof("Finding last available ticker...")
	ticker, err := rd.db.FiatRatesFindLastTicker()
	if err != nil {
		glog.Errorf("RatesDownloader FindTicker error: %v", err)
		return err
	}

	if ticker == nil {
		// If no tickers found, start downloading from the beginning
		glog.Infof("No tickers found! Looking up the earliest market data available on API and downloading from there.")
		timestamp, err = rd.findEarliestMarketData()
		if err != nil {
			glog.Errorf("Error looking up earliest market data: %v", err)
			return err
		}
	} else {
		// If found, continue downloading data from the next day of the last available record
		glog.Infof("Last available ticker: %v", ticker.Timestamp)
		timestamp = ticker.Timestamp
	}
	err = rd.syncHistorical(timestamp)
	if err != nil {
		glog.Errorf("RatesDownloader syncHistorical error: %v", err)
		return err
	}
	if err := rd.syncLatest(); err != nil {
		glog.Errorf("RatesDownloader syncLatest error: %v", err)
		return err
	}
	return nil
}

// FindEarliestMarketData uses binary search to find the oldest market data available on API.
func (rd *RatesDownloader) findEarliestMarketData() (*time.Time, error) {
	minDateString := "03-01-2009"
	minDate, err := time.Parse(rd.timeFormat, minDateString)
	if err != nil {
		glog.Error("Error parsing date: ", err)
		return nil, err
	}
	maxDate := rd.startTime.Add(time.Duration(-24) * time.Hour) // today's historical tickers may not be ready yet, so set to yesterday
	currentDate := maxDate
	for {
		var dataExists bool = false
		for {
			dataExists, err = rd.downloader.marketDataExists(&currentDate)
			if err != nil {
				glog.Errorf("Error checking if market data exists for date %v. Error: %v. Retrying in %v seconds.", currentDate, err, rd.periodSeconds)
				timer := time.NewTimer(rd.periodSeconds)
				<-timer.C
			}
			break
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

// syncLatest downloads the latest FiatRates data every rd.PeriodSeconds
func (rd *RatesDownloader) syncLatest() error {
	timer := time.NewTimer(rd.periodSeconds)
	var lastTickerRates map[string]float64
	sameTickerCounter := 0
	for {
		ticker, err := rd.downloader.getTicker(nil)
		if err != nil {
			// Do not exit on GET error, log it, wait and try again
			glog.Errorf("syncLatest GetData error: %v", err)
			<-timer.C
			timer.Reset(rd.periodSeconds)
			continue
		}

		if sameTickerCounter < 5 && reflect.DeepEqual(ticker.Rates, lastTickerRates) {
			// If rates are the same as previous, do not store them
			glog.Infof("syncLatest: ticker rates for %v are the same as previous, skipping...", ticker.Timestamp)
			<-timer.C
			timer.Reset(rd.periodSeconds)
			sameTickerCounter++
			continue
		}
		lastTickerRates = ticker.Rates
		sameTickerCounter = 0

		glog.Infof("syncLatest: storing ticker for %v", ticker.Timestamp)
		err = rd.db.FiatRatesStoreTicker(ticker)
		if err != nil {
			// If there's an error storing ticker (like missing rates), log it, wait and try again
			glog.Errorf("syncLatest StoreTicker error: %v", err)
		} else if rd.callbackOnNewTicker != nil {
			rd.callbackOnNewTicker(ticker)
		}
		<-timer.C
		timer.Reset(rd.periodSeconds)
	}
}

// syncHistorical downloads all the historical data since the specified timestamp till today,
// then continues to download the latest rates
func (rd *RatesDownloader) syncHistorical(timestamp *time.Time) error {
	period := time.Duration(1) * time.Second
	timer := time.NewTimer(period)
	for {
		if rd.startTime.Sub(*timestamp) < time.Duration(time.Hour*24) {
			break
		}

		ticker, err := rd.downloader.getTicker(timestamp)
		if err != nil {
			// Do not exit on GET error, log it, wait and try again
			glog.Errorf("syncHistorical GetData error: %v", err)
			<-timer.C
			timer.Reset(rd.periodSeconds)
			continue
		}

		glog.Infof("syncHistorical: storing ticker for %v", ticker.Timestamp)
		err = rd.db.FiatRatesStoreTicker(ticker)
		if err != nil {
			// If there's an error storing ticker (like missing rates), log it and continue to the next day
			glog.Errorf("syncHistorical error storing ticker for %v: %v", timestamp, err)
		}

		*timestamp = timestamp.Add(time.Hour * 24) // go to the next day

		<-timer.C
		timer.Reset(period)
	}
	return nil
}
