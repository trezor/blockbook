package fiat

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/db"
)

const CurrentTickersKey = "CurrentTickers"
const HourlyTickersKey = "HourlyTickers"
const FiveMinutesTickersKey = "FiveMinutesTickers"

// OnNewFiatRatesTicker is used to send notification about a new FiatRates ticker
type OnNewFiatRatesTicker func(ticker *common.CurrencyRatesTicker)

// RatesDownloaderInterface provides method signatures for specific fiat rates downloaders
type RatesDownloaderInterface interface {
	CurrentTickers() (*common.CurrencyRatesTicker, error)
	HourlyTickers() (*[]common.CurrencyRatesTicker, error)
	FiveMinutesTickers() (*[]common.CurrencyRatesTicker, error)
	UpdateHistoricalTickers() error
	UpdateHistoricalTokenTickers() error
}

// FiatRates stores FiatRates API parameters
type FiatRates struct {
	Enabled                bool
	periodSeconds          int64
	db                     *db.RocksDB
	timeFormat             string
	callbackOnNewTicker    OnNewFiatRatesTicker
	downloader             RatesDownloaderInterface
	downloadTokens         bool
	provider               string
	allowedVsCurrencies    string
	mux                    sync.RWMutex
	currentTicker          *common.CurrencyRatesTicker
	hourlyTickers          map[int64]*common.CurrencyRatesTicker
	hourlyTickersFrom      int64
	hourlyTickersTo        int64
	fiveMinutesTickers     map[int64]*common.CurrencyRatesTicker
	fiveMinutesTickersFrom int64
	fiveMinutesTickersTo   int64
	dailyTickers           map[int64]*common.CurrencyRatesTicker
	dailyTickersFrom       int64
	dailyTickersTo         int64
}

func tickersToMap(tickers *[]common.CurrencyRatesTicker, granularitySeconds int64) (map[int64]*common.CurrencyRatesTicker, int64, int64) {
	if tickers == nil || len(*tickers) == 0 {
		return nil, 0, 0
	}
	halfGranularity := granularitySeconds / 2
	m := make(map[int64]*common.CurrencyRatesTicker, len(*tickers))
	from := ((*tickers)[0].Timestamp.UTC().Unix() + halfGranularity) % granularitySeconds
	to := ((*tickers)[len(*tickers)-1].Timestamp.UTC().Unix() + halfGranularity) % granularitySeconds
	return m, from, to
}

// NewFiatRates initializes the FiatRates handler
func NewFiatRates(db *db.RocksDB, configFile string, callback OnNewFiatRatesTicker) (*FiatRates, error) {
	var config struct {
		FiatRates             string `json:"fiat_rates"`
		FiatRatesParams       string `json:"fiat_rates_params"`
		FiatRatesVsCurrencies string `json:"fiat_rates_vs_currencies"`
	}
	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("error reading file %v, %v", configFile, err)
	}
	err = json.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("error parsing config file %v, %v", configFile, err)
	}

	var fr = &FiatRates{
		provider:            config.FiatRates,
		allowedVsCurrencies: config.FiatRatesVsCurrencies,
	}

	if config.FiatRates == "" || config.FiatRatesParams == "" {
		glog.Infof("FiatRates config (%v) is empty, not downloading fiat rates", configFile)
		fr.Enabled = false
		return fr, nil
	}

	type fiatRatesParams struct {
		URL                string `json:"url"`
		Coin               string `json:"coin"`
		PlatformIdentifier string `json:"platformIdentifier"`
		PlatformVsCurrency string `json:"platformVsCurrency"`
		PeriodSeconds      int64  `json:"periodSeconds"`
	}
	rdParams := &fiatRatesParams{}
	err = json.Unmarshal([]byte(config.FiatRatesParams), &rdParams)
	if err != nil {
		return nil, err
	}
	if rdParams.URL == "" || rdParams.PeriodSeconds == 0 {
		return nil, errors.New("missing parameters")
	}
	fr.timeFormat = "02-01-2006"              // Layout string for FiatRates date formatting (DD-MM-YYYY)
	fr.periodSeconds = rdParams.PeriodSeconds // Time period for syncing the latest market data
	if fr.periodSeconds < 60 {                // minimum is one minute
		fr.periodSeconds = 60
	}
	fr.db = db
	fr.callbackOnNewTicker = callback
	fr.downloadTokens = rdParams.PlatformIdentifier != "" && rdParams.PlatformVsCurrency != ""
	if fr.downloadTokens {
		common.TickerRecalculateTokenRate = strings.ToLower(db.GetInternalState().CoinShortcut) != rdParams.PlatformVsCurrency
		common.TickerTokenVsCurrency = rdParams.PlatformVsCurrency
	}
	is := fr.db.GetInternalState()
	if fr.provider == "coingecko" {
		throttle := true
		if callback == nil {
			// a small hack - in tests the callback is not used, therefore there is no delay slowing down the test
			throttle = false
		}
		fr.downloader = NewCoinGeckoDownloader(db, rdParams.URL, rdParams.Coin, rdParams.PlatformIdentifier, rdParams.PlatformVsCurrency, fr.allowedVsCurrencies, fr.timeFormat, throttle)
		if is != nil {
			is.HasFiatRates = true
			is.HasTokenFiatRates = fr.downloadTokens
			fr.Enabled = true

			currentTickers, err := db.FiatRatesGetSpecialTickers(CurrentTickersKey)
			if err != nil {
				glog.Error("FiatRatesDownloader: get CurrentTickers from DB error ", err)
			}
			if currentTickers != nil && len(*currentTickers) > 0 {
				fr.currentTicker = &(*currentTickers)[0]
			}

			hourlyTickers, err := db.FiatRatesGetSpecialTickers(HourlyTickersKey)
			if err != nil {
				glog.Error("FiatRatesDownloader: get HourlyTickers from DB error ", err)
			}
			fr.hourlyTickers, fr.hourlyTickersFrom, fr.hourlyTickersTo = tickersToMap(hourlyTickers, 3600)

			fiveMinutesTickers, err := db.FiatRatesGetSpecialTickers(FiveMinutesTickersKey)
			if err != nil {
				glog.Error("FiatRatesDownloader: get FiveMinutesTickers from DB error ", err)
			}
			fr.fiveMinutesTickers, fr.fiveMinutesTickersFrom, fr.fiveMinutesTickersTo = tickersToMap(fiveMinutesTickers, 5*60)

		}
	} else {
		return nil, fmt.Errorf("unknown provider %q", fr.provider)
	}
	return fr, nil
}

// GetCurrentTicker returns current ticker
func (fr *FiatRates) GetCurrentTicker(vsCurrency string, token string) *common.CurrencyRatesTicker {
	fr.mux.RLock()
	currentTicker := fr.currentTicker
	fr.mux.RUnlock()
	if currentTicker != nil && common.IsSuitableTicker(currentTicker, vsCurrency, token) {
		return currentTicker
	}
	return nil
}

// setCurrentTicker sets current ticker
func (fr *FiatRates) setCurrentTicker(t *common.CurrencyRatesTicker) {
	fr.mux.Lock()
	defer fr.mux.Unlock()
	fr.currentTicker = t
	fr.db.FiatRatesStoreSpecialTickers(CurrentTickersKey, &[]common.CurrencyRatesTicker{*t})
}

// setCurrentTicker sets hourly tickers
func (fr *FiatRates) setHourlyTickers(t *[]common.CurrencyRatesTicker) {
	fr.mux.Lock()
	defer fr.mux.Unlock()
	fr.hourlyTickers, fr.hourlyTickersFrom, fr.hourlyTickersTo = tickersToMap(t, 3600)
	fr.db.FiatRatesStoreSpecialTickers(HourlyTickersKey, t)
}

// setCurrentTicker sets hourly tickers
func (fr *FiatRates) setFiveMinutesTickers(t *[]common.CurrencyRatesTicker) {
	fr.mux.Lock()
	defer fr.mux.Unlock()
	fr.fiveMinutesTickers, fr.fiveMinutesTickersFrom, fr.fiveMinutesTickersTo = tickersToMap(t, 5*60)
	fr.db.FiatRatesStoreSpecialTickers(FiveMinutesTickersKey, t)
}

// RunDownloader periodically downloads current (every 15 minutes) and historical (once a day) tickers
func (fr *FiatRates) RunDownloader() error {
	glog.Infof("Starting %v FiatRates downloader...", fr.provider)
	var lastHistoricalTickers time.Time
	is := fr.db.GetInternalState()
	tickerFromIs := fr.GetCurrentTicker("", "")
	firstRun := true
	for {
		unix := time.Now().Unix()
		next := unix + fr.periodSeconds
		next -= next % fr.periodSeconds
		// skip waiting for the period for the first run if there are no tickerFromIs or they are too old
		if !firstRun || (tickerFromIs != nil && next-tickerFromIs.Timestamp.Unix() < fr.periodSeconds) {
			// wait for the next run with a slight random value to avoid too many request at the same time
			next += int64(rand.Intn(12))
			time.Sleep(time.Duration(next-unix) * time.Second)
		}
		firstRun = false
		currentTicker, err := fr.downloader.CurrentTickers()
		if err != nil || currentTicker == nil {
			glog.Error("FiatRatesDownloader: CurrentTickers error ", err)
		} else {
			fr.setCurrentTicker(currentTicker)
			glog.Info("FiatRatesDownloader: CurrentTickers updated")
			if fr.callbackOnNewTicker != nil {
				fr.callbackOnNewTicker(currentTicker)
			}
		}
		hourlyTickers, err := fr.downloader.HourlyTickers()
		if err != nil || hourlyTickers == nil {
			glog.Error("FiatRatesDownloader: HourlyTickers error ", err)
		} else {
			fr.setHourlyTickers(hourlyTickers)
			glog.Info("FiatRatesDownloader: HourlyTickers updated")
		}
		fiveMinutesTickers, err := fr.downloader.FiveMinutesTickers()
		if err != nil || fiveMinutesTickers == nil {
			glog.Error("FiatRatesDownloader: FiveMinutesTickers error ", err)
		} else {
			fr.setFiveMinutesTickers(fiveMinutesTickers)
			glog.Info("FiatRatesDownloader: FiveMinutesTickers updated")
		}
		now := time.Now().UTC()
		// once a day, 1 hour after UTC midnight (to let the provider prepare historical rates) update historical tickers
		if (now.YearDay() != lastHistoricalTickers.YearDay() || now.Year() != lastHistoricalTickers.Year()) && now.Hour() > 0 {
			err = fr.downloader.UpdateHistoricalTickers()
			if err != nil {
				glog.Error("FiatRatesDownloader: UpdateHistoricalTickers error ", err)
			} else {
				lastHistoricalTickers = time.Now().UTC()
				ticker, err := fr.db.FiatRatesFindLastTicker("", "")
				if err != nil || ticker == nil {
					glog.Error("FiatRatesDownloader: FiatRatesFindLastTicker error ", err)
				} else {
					glog.Infof("FiatRatesDownloader: UpdateHistoricalTickers finished, last ticker from %v", ticker.Timestamp)
					if is != nil {
						is.HistoricalFiatRatesTime = ticker.Timestamp
					}
				}
				if fr.downloadTokens {
					// UpdateHistoricalTokenTickers in a goroutine, it can take quite some time as there are many tokens
					go func() {
						err := fr.downloader.UpdateHistoricalTokenTickers()
						if err != nil {
							glog.Error("FiatRatesDownloader: UpdateHistoricalTokenTickers error ", err)
						} else {
							glog.Info("FiatRatesDownloader: UpdateHistoricalTokenTickers finished")
							if is != nil {
								is.HistoricalTokenFiatRatesTime = time.Now().UTC()
							}
						}
					}()
				}
			}
		}
	}
}
