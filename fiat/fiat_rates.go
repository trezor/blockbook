package fiat

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/db"
)

const currentTickersKey = "CurrentTickers"
const hourlyTickersKey = "HourlyTickers"
const fiveMinutesTickersKey = "FiveMinutesTickers"

const highGranularityVsCurrency = "usd"

const secondsInDay = 24 * 60 * 60
const secondsInHour = 60 * 60
const secondsInFiveMinutes = 5 * 60

// OnNewFiatRatesTicker is used to send notification about a new FiatRates ticker
type OnNewFiatRatesTicker func(ticker *common.CurrencyRatesTicker)

// RatesDownloaderInterface provides method signatures for a specific fiat rates downloader
type RatesDownloaderInterface interface {
	CurrentTickers() (*common.CurrencyRatesTicker, error)
	HourlyTickers() (*[]common.CurrencyRatesTicker, error)
	FiveMinutesTickers() (*[]common.CurrencyRatesTicker, error)
	UpdateHistoricalTickers() error
	UpdateHistoricalTokenTickers() error
}

// FiatRates is used to fetch and refresh fiat rates
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

// NewFiatRates initializes the FiatRates handler
func NewFiatRates(db *db.RocksDB, config *common.Config, metrics *common.Metrics, callback OnNewFiatRatesTicker) (*FiatRates, error) {

	var fr = &FiatRates{
		provider:            config.FiatRates,
		allowedVsCurrencies: config.FiatRatesVsCurrencies,
	}

	if config.FiatRates == "" || config.FiatRatesParams == "" {
		glog.Infof("FiatRates config is empty, not downloading fiat rates")
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
	err := json.Unmarshal([]byte(config.FiatRatesParams), &rdParams)
	if err != nil {
		return nil, err
	}
	if rdParams.PeriodSeconds == 0 {
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
		fr.downloader = NewCoinGeckoDownloader(db, db.GetInternalState().GetNetwork(), rdParams.URL, rdParams.Coin, rdParams.PlatformIdentifier, rdParams.PlatformVsCurrency, fr.allowedVsCurrencies, fr.timeFormat, metrics, throttle)
		if is != nil {
			is.HasFiatRates = true
			is.HasTokenFiatRates = fr.downloadTokens
			fr.Enabled = true

			if err := fr.loadDailyTickers(); err != nil {
				return nil, err
			}

			currentTickers, err := db.FiatRatesGetSpecialTickers(currentTickersKey)
			if err != nil {
				glog.Error("FiatRatesDownloader: get CurrentTickers from DB error ", err)
			}
			if currentTickers != nil && len(*currentTickers) > 0 {
				fr.currentTicker = &(*currentTickers)[0]
			}

			hourlyTickers, err := db.FiatRatesGetSpecialTickers(hourlyTickersKey)
			if err != nil {
				glog.Error("FiatRatesDownloader: get HourlyTickers from DB error ", err)
			}
			fr.hourlyTickers, fr.hourlyTickersFrom, fr.hourlyTickersTo = fr.tickersToMap(hourlyTickers, secondsInHour)

			fiveMinutesTickers, err := db.FiatRatesGetSpecialTickers(fiveMinutesTickersKey)
			if err != nil {
				glog.Error("FiatRatesDownloader: get FiveMinutesTickers from DB error ", err)
			}
			fr.fiveMinutesTickers, fr.fiveMinutesTickersFrom, fr.fiveMinutesTickersTo = fr.tickersToMap(fiveMinutesTickers, secondsInFiveMinutes)

		}
	} else {
		return nil, fmt.Errorf("unknown provider %q", fr.provider)
	}
	fr.logTickersInfo()
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

// getTokenTickersForTimestamps returns tickers for slice of timestamps, that contain requested vsCurrency and token
func (fr *FiatRates) getTokenTickersForTimestamps(timestamps []int64, vsCurrency string, token string) (*[]*common.CurrencyRatesTicker, error) {
	currentTicker := fr.GetCurrentTicker("", token)
	tickers := make([]*common.CurrencyRatesTicker, len(timestamps))
	var prevTicker *common.CurrencyRatesTicker
	var prevTs int64
	var err error
	for i, t := range timestamps {
		// check if the token is available in the current ticker - if not, return nil ticker instead of wasting time in costly DB searches
		if currentTicker != nil {
			var ticker *common.CurrencyRatesTicker
			date := time.Unix(t, 0)
			// if previously found ticker is newer than this one (token tickers may not be in DB for every day), skip search in DB
			if prevTicker != nil && t >= prevTs && !date.After(prevTicker.Timestamp) {
				ticker = prevTicker
				prevTs = t
			} else {
				ticker, err = fr.db.FiatRatesFindTicker(&date, vsCurrency, token)
				if err != nil {
					return nil, err
				}
				prevTicker = ticker
				prevTs = t
			}
			// if ticker not found in DB, use current ticker
			if ticker == nil {
				tickers[i] = currentTicker
				prevTicker = currentTicker
				prevTs = t
			} else {
				tickers[i] = ticker
			}
		}
	}
	return &tickers, nil
}

// GetTickersForTimestamps returns tickers for slice of timestamps, that contain requested vsCurrency and token
func (fr *FiatRates) GetTickersForTimestamps(timestamps []int64, vsCurrency string, token string) (*[]*common.CurrencyRatesTicker, error) {
	if !fr.Enabled {
		return nil, nil
	}
	// token rates are not in memory, them load from DB
	if token != "" {
		return fr.getTokenTickersForTimestamps(timestamps, vsCurrency, token)
	}
	fr.mux.RLock()
	defer fr.mux.RUnlock()
	tickers := make([]*common.CurrencyRatesTicker, len(timestamps))
	var prevTicker *common.CurrencyRatesTicker
	var prevTs int64
	for i, t := range timestamps {
		dailyTs := ceilUnix(t, secondsInDay)
		// use higher granularity only for non daily timestamps
		if t != dailyTs {
			if t >= fr.fiveMinutesTickersFrom && t <= fr.fiveMinutesTickersTo {
				if ticker, found := fr.fiveMinutesTickers[ceilUnix(t, secondsInFiveMinutes)]; found && ticker != nil {
					if common.IsSuitableTicker(ticker, vsCurrency, token) {
						tickers[i] = ticker
						continue
					}
				}
			}
			if t >= fr.hourlyTickersFrom && t <= fr.hourlyTickersTo {
				if ticker, found := fr.hourlyTickers[ceilUnix(t, secondsInHour)]; found && ticker != nil {
					if common.IsSuitableTicker(ticker, vsCurrency, token) {
						tickers[i] = ticker
						continue
					}
				}
			}
		}
		if prevTicker != nil && t >= prevTs && t <= prevTicker.Timestamp.Unix() {
			tickers[i] = prevTicker
			continue
		} else {
			var found bool
			if dailyTs < fr.dailyTickersFrom {
				dailyTs = fr.dailyTickersFrom
			}
			var ticker *common.CurrencyRatesTicker
			for ; dailyTs <= fr.dailyTickersTo; dailyTs += secondsInDay {
				if ticker, found = fr.dailyTickers[dailyTs]; found && ticker != nil {
					if common.IsSuitableTicker(ticker, vsCurrency, token) {
						tickers[i] = ticker
						prevTicker = ticker
						prevTs = t
						break
					} else {
						found = false
					}
				}
			}
			if !found {
				tickers[i] = fr.currentTicker
				prevTicker = fr.currentTicker
				prevTs = t
			}
		}
	}
	return &tickers, nil
}
func (fr *FiatRates) logTickersInfo() {
	glog.Infof("fiat rates %s handler, %d (%s - %s) daily tickers, %d (%s - %s) hourly tickers, %d (%s - %s) 5 minute tickers", fr.provider,
		len(fr.dailyTickers), time.Unix(fr.dailyTickersFrom, 0).Format("2006-01-02"), time.Unix(fr.dailyTickersTo, 0).Format("2006-01-02"),
		len(fr.hourlyTickers), time.Unix(fr.hourlyTickersFrom, 0).Format("2006-01-02 15:04"), time.Unix(fr.hourlyTickersTo, 0).Format("2006-01-02 15:04"),
		len(fr.fiveMinutesTickers), time.Unix(fr.fiveMinutesTickersFrom, 0).Format("2006-01-02 15:04"), time.Unix(fr.fiveMinutesTickersTo, 0).Format("2006-01-02 15:04"))
}

func roundTimeUnix(t time.Time, granularity int64) int64 {
	return roundUnix(t.UTC().Unix(), granularity)
}

func roundUnix(t int64, granularity int64) int64 {
	unix := t + (granularity >> 1)
	return unix - unix%granularity
}

func ceilUnix(t int64, granularity int64) int64 {
	unix := t + (granularity - 1)
	return unix - unix%granularity
}

// loadDailyTickers loads daily tickers to cache
func (fr *FiatRates) loadDailyTickers() error {
	fr.mux.Lock()
	defer fr.mux.Unlock()
	fr.dailyTickers = make(map[int64]*common.CurrencyRatesTicker)
	err := fr.db.FiatRatesGetAllTickers(func(ticker *common.CurrencyRatesTicker) error {
		normalizedTime := roundTimeUnix(ticker.Timestamp, secondsInDay)
		if normalizedTime == fr.dailyTickersFrom {
			// there are multiple tickers on the first day, use only the first one
			return nil
		}
		// remove token rates from cache to save memory (tickers with token rates are hundreds of kb big)
		ticker.TokenRates = nil
		if len(fr.dailyTickers) > 0 {
			// check that there is a ticker for every day, if missing, set it from current value if missing
			prevTime := normalizedTime
			for {
				prevTime -= secondsInDay
				if _, found := fr.dailyTickers[prevTime]; found {
					break
				}
				fr.dailyTickers[prevTime] = ticker
			}
		} else {
			fr.dailyTickersFrom = normalizedTime
		}
		fr.dailyTickers[normalizedTime] = ticker
		fr.dailyTickersTo = normalizedTime
		return nil
	})
	return err
}

// setCurrentTicker sets current ticker
func (fr *FiatRates) setCurrentTicker(t *common.CurrencyRatesTicker) {
	fr.mux.Lock()
	defer fr.mux.Unlock()
	fr.currentTicker = t
	fr.db.FiatRatesStoreSpecialTickers(currentTickersKey, &[]common.CurrencyRatesTicker{*t})
}

func (fr *FiatRates) tickersToMap(tickers *[]common.CurrencyRatesTicker, granularitySeconds int64) (map[int64]*common.CurrencyRatesTicker, int64, int64) {
	if tickers == nil || len(*tickers) == 0 {
		return make(map[int64]*common.CurrencyRatesTicker), 0, 0
	}
	m := make(map[int64]*common.CurrencyRatesTicker, len(*tickers))
	from := int64(0)
	to := int64(0)
	for i := range *tickers {
		ticker := (*tickers)[i]
		normalizedTime := roundTimeUnix(ticker.Timestamp, granularitySeconds)
		dailyTime := roundTimeUnix(ticker.Timestamp, secondsInDay)
		dailyTicker, found := fr.dailyTickers[dailyTime]
		if !found {
			// if not found in historical tickers, use current ticker
			dailyTicker = fr.currentTicker
		}
		if dailyTicker != nil {
			// high granularity tickers are loaded only in one currency, add other currencies based on daily rate between fiat currencies
			vsRate, foundVs := ticker.Rates[highGranularityVsCurrency]
			dailyVsRate, foundDaily := dailyTicker.Rates[highGranularityVsCurrency]
			if foundDaily && dailyVsRate != 0 && foundVs && vsRate != 0 {
				for currency, rate := range dailyTicker.Rates {
					if currency != highGranularityVsCurrency {
						ticker.Rates[currency] = vsRate * rate / dailyVsRate
					}
				}
			}
		}
		if len(m) > 0 {
			if normalizedTime == from {
				// there are multiple normalized tickers for the first entry, skip
				continue
			}
			// check that there is a ticker for each period, set it from current value if missing
			prevTime := normalizedTime
			for {
				prevTime -= granularitySeconds
				if _, found := m[prevTime]; found {
					break
				}
				m[prevTime] = &ticker
			}
		} else {
			from = normalizedTime
		}
		m[normalizedTime] = &ticker
		to = normalizedTime
	}
	return m, from, to
}

// setHourlyTickers sets hourly tickers
func (fr *FiatRates) setHourlyTickers(t *[]common.CurrencyRatesTicker) {
	fr.db.FiatRatesStoreSpecialTickers(hourlyTickersKey, t)
	fr.mux.Lock()
	defer fr.mux.Unlock()
	fr.hourlyTickers, fr.hourlyTickersFrom, fr.hourlyTickersTo = fr.tickersToMap(t, secondsInHour)
}

// setFiveMinutesTickers sets five minutes tickers
func (fr *FiatRates) setFiveMinutesTickers(t *[]common.CurrencyRatesTicker) {
	fr.db.FiatRatesStoreSpecialTickers(fiveMinutesTickersKey, t)
	fr.mux.Lock()
	defer fr.mux.Unlock()
	fr.fiveMinutesTickers, fr.fiveMinutesTickersFrom, fr.fiveMinutesTickersTo = fr.tickersToMap(t, secondsInFiveMinutes)
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
			next += int64(rand.Intn(3))
			time.Sleep(time.Duration(next-unix) * time.Second)
		}
		firstRun = false

		// load current tickers
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

		// load hourly tickers, it is necessary to wait about 1 hour to prepare the tickers
		if time.Now().UTC().Unix() >= fr.hourlyTickersTo+secondsInHour+secondsInHour {
			hourlyTickers, err := fr.downloader.HourlyTickers()
			if err != nil || hourlyTickers == nil {
				glog.Error("FiatRatesDownloader: HourlyTickers error ", err)
			} else {
				fr.setHourlyTickers(hourlyTickers)
				glog.Info("FiatRatesDownloader: HourlyTickers updated")
			}
		}

		// load five minute tickers, it is necessary to wait about 10 minutes to prepare the tickers
		if time.Now().UTC().Unix() >= fr.fiveMinutesTickersTo+3*secondsInFiveMinutes {
			fiveMinutesTickers, err := fr.downloader.FiveMinutesTickers()
			if err != nil || fiveMinutesTickers == nil {
				glog.Error("FiatRatesDownloader: FiveMinutesTickers error ", err)
			} else {
				fr.setFiveMinutesTickers(fiveMinutesTickers)
				glog.Info("FiatRatesDownloader: FiveMinutesTickers updated")
			}
		}

		// once a day, 1 hour after UTC midnight (to let the provider prepare historical rates) update historical tickers
		now := time.Now().UTC()
		if (now.YearDay() != lastHistoricalTickers.YearDay() || now.Year() != lastHistoricalTickers.Year()) && now.Hour() > 0 {
			err = fr.downloader.UpdateHistoricalTickers()
			if err != nil {
				glog.Error("FiatRatesDownloader: UpdateHistoricalTickers error ", err)
			} else {
				lastHistoricalTickers = time.Now().UTC()
				if err = fr.loadDailyTickers(); err != nil {
					glog.Error("FiatRatesDownloader: loadDailyTickers error ", err)
				} else {
					ticker, found := fr.dailyTickers[fr.dailyTickersTo]
					if !found || ticker == nil {
						glog.Error("FiatRatesDownloader: dailyTickers not loaded")
					} else {
						glog.Infof("FiatRatesDownloader: UpdateHistoricalTickers finished, last ticker from %v", ticker.Timestamp)
						fr.logTickersInfo()
						if is != nil {
							is.HistoricalFiatRatesTime = ticker.Timestamp
						}
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
