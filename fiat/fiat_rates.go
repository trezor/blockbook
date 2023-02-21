package fiat

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/db"
)

// OnNewFiatRatesTicker is used to send notification about a new FiatRates ticker
type OnNewFiatRatesTicker func(ticker *common.CurrencyRatesTicker)

// RatesDownloaderInterface provides method signatures for specific fiat rates downloaders
type RatesDownloaderInterface interface {
	CurrentTickers() (*common.CurrencyRatesTicker, error)
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
	downloadTokens      bool
}

// NewFiatRatesDownloader initializes the downloader for FiatRates API.
func NewFiatRatesDownloader(db *db.RocksDB, apiType string, params string, allowedVsCurrencies string, callback OnNewFiatRatesTicker) (*RatesDownloader, error) {
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
	rd.downloadTokens = rdParams.PlatformIdentifier != "" && rdParams.PlatformVsCurrency != ""
	if rd.downloadTokens {
		common.TickerRecalculateTokenRate = strings.ToLower(db.GetInternalState().CoinShortcut) != rdParams.PlatformVsCurrency
		common.TickerTokenVsCurrency = rdParams.PlatformVsCurrency
	}
	is := rd.db.GetInternalState()
	if apiType == "coingecko" {
		throttle := true
		if callback == nil {
			// a small hack - in tests the callback is not used, therefore there is no delay slowing down the test
			throttle = false
		}
		rd.downloader = NewCoinGeckoDownloader(db, rdParams.URL, rdParams.Coin, rdParams.PlatformIdentifier, rdParams.PlatformVsCurrency, allowedVsCurrencies, rd.timeFormat, throttle)
		if is != nil {
			is.HasFiatRates = true
			is.HasTokenFiatRates = rd.downloadTokens
		}

	} else {
		return nil, fmt.Errorf("NewFiatRatesDownloader: incorrect API type %q", apiType)
	}
	return rd, nil
}

// Run periodically downloads current (every 15 minutes) and historical (once a day) tickers
func (rd *RatesDownloader) Run() error {
	var lastHistoricalTickers time.Time
	is := rd.db.GetInternalState()
	tickerFromIs := is.GetCurrentTicker("", "")
	firstRun := true
	for {
		unix := time.Now().Unix()
		next := unix + rd.periodSeconds
		next -= next % rd.periodSeconds
		// skip waiting for the period for the first run if there are no tickerFromIs or they are too old
		if !firstRun || (tickerFromIs != nil && next-tickerFromIs.Timestamp.Unix() < rd.periodSeconds) {
			// wait for the next run with a slight random value to avoid too many request at the same time
			next += int64(rand.Intn(12))
			time.Sleep(time.Duration(next-unix) * time.Second)
		}
		firstRun = false
		tickers, err := rd.downloader.CurrentTickers()
		if err != nil || tickers == nil {
			glog.Error("FiatRatesDownloader: CurrentTickers error ", err)
		} else {
			is.SetCurrentTicker(tickers)
			glog.Info("FiatRatesDownloader: CurrentTickers updated")
			if rd.callbackOnNewTicker != nil {
				rd.callbackOnNewTicker(tickers)
			}
		}
		now := time.Now().UTC()
		// once a day, 1 hour after UTC midnight (to let the provider prepare historical rates) update historical tickers
		if (now.YearDay() != lastHistoricalTickers.YearDay() || now.Year() != lastHistoricalTickers.Year()) && now.Hour() > 0 {
			err = rd.downloader.UpdateHistoricalTickers()
			if err != nil {
				glog.Error("FiatRatesDownloader: UpdateHistoricalTickers error ", err)
			} else {
				lastHistoricalTickers = time.Now().UTC()
				ticker, err := rd.db.FiatRatesFindLastTicker("", "")
				if err != nil || ticker == nil {
					glog.Error("FiatRatesDownloader: FiatRatesFindLastTicker error ", err)
				} else {
					glog.Infof("FiatRatesDownloader: UpdateHistoricalTickers finished, last ticker from %v", ticker.Timestamp)
					if is != nil {
						is.HistoricalFiatRatesTime = ticker.Timestamp
					}
				}
				if rd.downloadTokens {
					// UpdateHistoricalTokenTickers in a goroutine, it can take quite some time as there are many tokens
					go func() {
						err := rd.downloader.UpdateHistoricalTokenTickers()
						if err != nil {
							glog.Error("FiatRatesDownloader: UpdateHistoricalTokenTickers error ", err)
						} else {
							glog.Info("FiatRatesDownloader: UpdateHistoricalTokenTickers finished")
							if is != nil {
								is.HistoricalTokenFiatRatesTime = time.Now()
							}
						}
					}()
				}
			}
		}
	}
}
