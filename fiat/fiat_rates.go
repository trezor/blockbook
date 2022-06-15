package fiat

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/golang/glog"
	"github.com/trezor/blockbook/db"
)

// OnNewFiatRatesTicker is used to send notification about a new FiatRates ticker
type OnNewFiatRatesTicker func(ticker *db.CurrencyRatesTicker)

// RatesDownloaderInterface provides method signatures for specific fiat rates downloaders
type RatesDownloaderInterface interface {
	CurrentTickers() (*db.CurrencyRatesTicker, error)
	UpdateHistoricalTickers() error
	UpdateHistoricalTokenTickers() error
}

// RatesDownloader stores FiatRates API parameters
type RatesDownloader struct {
	periodSeconds       int64
	db                  *db.RocksDB
	timeFormat          string
	callbackOnNewTicker OnNewFiatRatesTicker
	downloader          RatesDownloaderInterface
}

// NewFiatRatesDownloader initializes the downloader for FiatRates API.
func NewFiatRatesDownloader(db *db.RocksDB, apiType string, params string, callback OnNewFiatRatesTicker) (*RatesDownloader, error) {
	var rd = &RatesDownloader{}
	type fiatRatesParams struct {
		URL                string `json:"url"`
		Coin               string `json:"coin"`
		PlatformIdentifier string `json:"platformIdentifier"`
		PlatformVsCurrency string `json:"platformVsCurrency"`
		PeriodSeconds      int64  `json:"periodSeconds"`
	}
	rdParams := &fiatRatesParams{}
	err := json.Unmarshal([]byte(params), &rdParams)
	if err != nil {
		return nil, err
	}
	if rdParams.URL == "" || rdParams.PeriodSeconds == 0 {
		return nil, errors.New("Missing parameters")
	}
	rd.timeFormat = "02-01-2006"              // Layout string for FiatRates date formatting (DD-MM-YYYY)
	rd.periodSeconds = rdParams.PeriodSeconds // Time period for syncing the latest market data
	if rd.periodSeconds < 60 {                // minimum is one minute
		rd.periodSeconds = 60
	}
	rd.db = db
	rd.callbackOnNewTicker = callback
	if apiType == "coingecko" {
		throttlingDelayMs := 50
		if callback == nil {
			// a small hack - in tests the callback is not used, therefore there is no delay slowing the test
			throttlingDelayMs = 0
		}
		rd.downloader = NewCoinGeckoDownloader(db, rdParams.URL, rdParams.Coin, rdParams.PlatformIdentifier, rdParams.PlatformVsCurrency, rd.timeFormat, throttlingDelayMs)
	} else {
		return nil, fmt.Errorf("NewFiatRatesDownloader: incorrect API type %q", apiType)
	}
	return rd, nil
}

// Run periodically downloads current (every 15 minutes) and historical (once a day) tickers
func (rd *RatesDownloader) Run() error {
	var lastHistoricalTickers time.Time

	for {
		tickers, err := rd.downloader.CurrentTickers()
		if err != nil && tickers != nil {
			glog.Error("FiatRatesDownloader: CurrentTickers error ", err)
		} else {
			rd.db.FiatRatesSetCurrentTicker(tickers)
			glog.Info("FiatRatesDownloader: CurrentTickers updated")
		}
		if time.Now().UTC().YearDay() != lastHistoricalTickers.YearDay() || time.Now().UTC().Year() != lastHistoricalTickers.Year() {
			err = rd.downloader.UpdateHistoricalTickers()
			if err != nil {
				glog.Error("FiatRatesDownloader: UpdateHistoricalTickers error ", err)
			} else {
				lastHistoricalTickers = time.Now().UTC()
				glog.Info("FiatRatesDownloader: UpdateHistoricalTickers finished")
			}
			// UpdateHistoricalTokenTickers in a goroutine, it can take quite some time as there may be many tokens
			go func() {
				err := rd.downloader.UpdateHistoricalTokenTickers()
				if err != nil {
					glog.Error("FiatRatesDownloader: UpdateHistoricalTokenTickers error ", err)
				} else {
					lastHistoricalTickers = time.Now().UTC()
					glog.Info("FiatRatesDownloader: UpdateHistoricalTokenTickers finished")
				}
			}()
		}
		// next run on the
		now := time.Now().Unix()
		next := now + rd.periodSeconds
		next -= next % rd.periodSeconds
		next += int64(rand.Intn(12))
		time.Sleep(time.Duration(next-now) * time.Second)
	}
}
