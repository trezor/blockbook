package fiat

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/linxGnu/grocksdb"
	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/db"
)

const (
	DefaultHTTPTimeout     = 15 * time.Second
	DefaultThrottleDelayMs = 100 // 100 ms delay between requests
)

// Coingecko is a structure that implements RatesDownloaderInterface
type Coingecko struct {
	url                 string
	apiKey              string
	coin                string
	platformIdentifier  string
	platformVsCurrency  string
	allowedVsCurrencies map[string]struct{}
	httpTimeout         time.Duration
	throttlingDelay     time.Duration
	timeFormat          string
	httpClient          *http.Client
	db                  *db.RocksDB
	updatingCurrent     bool
	updatingTokens      bool
	metrics             *common.Metrics
}

// simpleSupportedVSCurrencies https://api.coingecko.com/api/v3/simple/supported_vs_currencies
type simpleSupportedVSCurrencies []string

type coinsListItem struct {
	ID        string            `json:"id"`
	Symbol    string            `json:"symbol"`
	Name      string            `json:"name"`
	Platforms map[string]string `json:"platforms"`
}

// coinList https://api.coingecko.com/api/v3/coins/list
type coinList []coinsListItem

type marketPoint [2]float64
type marketChartPrices struct {
	Prices []marketPoint `json:"prices"`
}

// NewCoinGeckoDownloader creates a coingecko structure that implements the RatesDownloaderInterface
func NewCoinGeckoDownloader(db *db.RocksDB, coinShortcut string, url string, coin string, platformIdentifier string, platformVsCurrency string, allowedVsCurrencies string, timeFormat string, metrics *common.Metrics, throttleDown bool) RatesDownloaderInterface {
	throttlingDelayMs := 0 // No delay by default
	if throttleDown {
		throttlingDelayMs = DefaultThrottleDelayMs
	}

	allowedVsCurrenciesMap := getAllowedVsCurrenciesMap(allowedVsCurrencies)

	apiKey := os.Getenv(strings.ToUpper(coinShortcut) + "_COINGECKO_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("COINGECKO_API_KEY")
	}

	// use default address if not overridden, with respect to existence of apiKey
	if url == "" {
		if apiKey != "" {
			url = "https://pro-api.coingecko.com/api/v3/"
		} else {
			url = "https://api.coingecko.com/api/v3"
		}
	}
	glog.Info("Coingecko downloader url ", url)

	return &Coingecko{
		url:                 url,
		apiKey:              apiKey,
		coin:                coin,
		platformIdentifier:  platformIdentifier,
		platformVsCurrency:  platformVsCurrency,
		allowedVsCurrencies: allowedVsCurrenciesMap,
		httpTimeout:         DefaultHTTPTimeout,
		timeFormat:          timeFormat,
		httpClient: &http.Client{
			Timeout: DefaultHTTPTimeout,
		},
		db:              db,
		throttlingDelay: time.Duration(throttlingDelayMs) * time.Millisecond,
		metrics:         metrics,
	}
}

// getAllowedVsCurrenciesMap returns a map of allowed vs currencies
func getAllowedVsCurrenciesMap(currenciesString string) map[string]struct{} {
	allowedVsCurrenciesMap := make(map[string]struct{})
	if len(currenciesString) > 0 {
		for _, c := range strings.Split(strings.ToLower(currenciesString), ",") {
			allowedVsCurrenciesMap[c] = struct{}{}
		}
	}
	return allowedVsCurrenciesMap
}

// doReq HTTP client
func doReq(req *http.Request, client *http.Client) ([]byte, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s", body)
	}
	return body, nil
}

// makeReq HTTP request helper - will retry the call after 1 minute on error
func (cg *Coingecko) makeReq(url string, endpoint string) ([]byte, error) {
	for {
		// glog.Infof("Coingecko makeReq %v", url)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		if cg.apiKey != "" {
			req.Header.Set("x-cg-pro-api-key", cg.apiKey)
		}
		resp, err := doReq(req, cg.httpClient)
		if err == nil {
			if cg.metrics != nil {
				cg.metrics.CoingeckoRequests.With(common.Labels{"endpoint": endpoint, "status": "success"}).Inc()
			}
			return resp, err
		}
		if err.Error() != "error code: 1015" && !strings.Contains(strings.ToLower(err.Error()), "exceeded the rate limit") && !strings.Contains(strings.ToLower(err.Error()), "throttled") {
			if cg.metrics != nil {
				cg.metrics.CoingeckoRequests.With(common.Labels{"endpoint": endpoint, "status": "error"}).Inc()
			}
			glog.Errorf("Coingecko makeReq %v error %v", url, err)
			return nil, err
		}
		if cg.metrics != nil {
			cg.metrics.CoingeckoRequests.With(common.Labels{"endpoint": endpoint, "status": "throttle"}).Inc()
		}
		// if there is a throttling error, wait 60 seconds and retry
		glog.Warningf("Coingecko makeReq %v error %v, will retry in 60 seconds", url, err)
		time.Sleep(60 * time.Second)
	}
}

// SimpleSupportedVSCurrencies /simple/supported_vs_currencies
func (cg *Coingecko) simpleSupportedVSCurrencies() (simpleSupportedVSCurrencies, error) {
	url := cg.url + "/simple/supported_vs_currencies"
	resp, err := cg.makeReq(url, "supported_vs_currencies")
	if err != nil {
		return nil, err
	}
	var data simpleSupportedVSCurrencies
	err = json.Unmarshal(resp, &data)
	if err != nil {
		return nil, err
	}
	if len(cg.allowedVsCurrencies) == 0 {
		return data, nil
	}
	filtered := make([]string, 0, len(cg.allowedVsCurrencies))
	for _, c := range data {
		if _, found := cg.allowedVsCurrencies[c]; found {
			filtered = append(filtered, c)
		}
	}
	return filtered, nil
}

// SimplePrice /simple/price Multiple ID and Currency (ids, vs_currencies)
func (cg *Coingecko) simplePrice(ids []string, vsCurrencies []string) (*map[string]map[string]float32, error) {
	params := url.Values{}
	idsParam := strings.Join(ids, ",")
	vsCurrenciesParam := strings.Join(vsCurrencies, ",")

	params.Add("ids", idsParam)
	params.Add("vs_currencies", vsCurrenciesParam)

	url := fmt.Sprintf("%s/simple/price?%s", cg.url, params.Encode())
	resp, err := cg.makeReq(url, "simple/price")
	if err != nil {
		return nil, err
	}

	t := make(map[string]map[string]float32)
	err = json.Unmarshal(resp, &t)
	if err != nil {
		return nil, err
	}

	return &t, nil
}

// CoinsList /coins/list
func (cg *Coingecko) coinsList() (coinList, error) {
	params := url.Values{}
	platform := "false"
	if cg.platformIdentifier != "" {
		platform = "true"
	}
	params.Add("include_platform", platform)
	url := fmt.Sprintf("%s/coins/list?%s", cg.url, params.Encode())
	resp, err := cg.makeReq(url, "coins/list")
	if err != nil {
		return nil, err
	}

	var data coinList
	err = json.Unmarshal(resp, &data)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// coinMarketChart /coins/{id}/market_chart?vs_currency={usd, eur, jpy, etc.}&days={1,14,30,max}
func (cg *Coingecko) coinMarketChart(id string, vs_currency string, days string, daily bool) (*marketChartPrices, error) {
	if len(id) == 0 || len(vs_currency) == 0 || len(days) == 0 {
		return nil, fmt.Errorf("id, vs_currency, and days is required")
	}

	params := url.Values{}
	if daily {
		params.Add("interval", "daily")
	}
	params.Add("vs_currency", vs_currency)
	params.Add("days", days)

	url := fmt.Sprintf("%s/coins/%s/market_chart?%s", cg.url, id, params.Encode())
	resp, err := cg.makeReq(url, "market_chart")
	if err != nil {
		return nil, err
	}

	m := marketChartPrices{}
	err = json.Unmarshal(resp, &m)
	if err != nil {
		return &m, err
	}

	return &m, nil
}

var vsCurrencies []string
var platformIds []string
var platformIdsToTokens map[string]string

func (cg *Coingecko) platformIds() error {
	if cg.platformIdentifier == "" {
		return nil
	}
	cl, err := cg.coinsList()
	if err != nil {
		return err
	}
	idsMap := make(map[string]string, 64)
	ids := make([]string, 0, 64)
	for i := range cl {
		id, found := cl[i].Platforms[cg.platformIdentifier]
		if found && id != "" {
			idsMap[cl[i].ID] = id
			ids = append(ids, cl[i].ID)
		}
	}
	platformIds = ids
	platformIdsToTokens = idsMap
	return nil
}

// CurrentTickers returns the latest exchange rates
func (cg *Coingecko) CurrentTickers() (*common.CurrencyRatesTicker, error) {
	cg.updatingCurrent = true
	defer func() { cg.updatingCurrent = false }()

	var newTickers = common.CurrencyRatesTicker{}

	if vsCurrencies == nil {
		vs, err := cg.simpleSupportedVSCurrencies()
		if err != nil {
			return nil, err
		}
		vsCurrencies = vs
	}
	prices, err := cg.simplePrice([]string{cg.coin}, vsCurrencies)
	if err != nil || prices == nil {
		return nil, err
	}
	newTickers.Rates = make(map[string]float32, len((*prices)[cg.coin]))
	for t, v := range (*prices)[cg.coin] {
		newTickers.Rates[t] = v
	}

	if cg.platformIdentifier != "" && cg.platformVsCurrency != "" {
		if platformIdsToTokens == nil {
			err = cg.platformIds()
			if err != nil {
				return nil, err
			}
		}
		newTickers.TokenRates = make(map[string]float32)
		from := 0
		const maxRequestLen = 6000
		requestLen := 0
		for to := 0; to < len(platformIds); to++ {
			requestLen += len(platformIds[to]) + 3 // 3 characters for the comma separator %2C
			if requestLen > maxRequestLen || to+1 >= len(platformIds) {
				tokenPrices, err := cg.simplePrice(platformIds[from:to+1], []string{cg.platformVsCurrency})
				if err != nil || tokenPrices == nil {
					return nil, err
				}
				for id, v := range *tokenPrices {
					t, found := platformIdsToTokens[id]
					if found {
						newTickers.TokenRates[t] = v[cg.platformVsCurrency]
					}
				}
				from = to + 1
				requestLen = 0
			}
		}
	}
	newTickers.Timestamp = time.Now().UTC()
	return &newTickers, nil
}

func (cg *Coingecko) getHighGranularityTickers(days string) (*[]common.CurrencyRatesTicker, error) {
	mc, err := cg.coinMarketChart(cg.coin, highGranularityVsCurrency, days, false)
	if err != nil {
		return nil, err
	}
	if len(mc.Prices) < 2 {
		return nil, nil
	}
	// ignore the last point, it is not in granularity
	tickers := make([]common.CurrencyRatesTicker, len(mc.Prices)-1)
	for i, p := range mc.Prices[:len(mc.Prices)-1] {
		var timestamp uint
		timestamp = uint(p[0])
		if timestamp > 100000000000 {
			// convert timestamp from milliseconds to seconds
			timestamp /= 1000
		}
		rate := float32(p[1])
		u := time.Unix(int64(timestamp), 0).UTC()
		ticker := common.CurrencyRatesTicker{
			Timestamp: u,
			Rates:     make(map[string]float32),
		}
		ticker.Rates[highGranularityVsCurrency] = rate
		tickers[i] = ticker
	}
	return &tickers, nil
}

// HourlyTickers returns the array of the exchange rates in hourly granularity
func (cg *Coingecko) HourlyTickers() (*[]common.CurrencyRatesTicker, error) {
	return cg.getHighGranularityTickers("90")
}

// HourlyTickers returns the array of the exchange rates in five minutes granularity
func (cg *Coingecko) FiveMinutesTickers() (*[]common.CurrencyRatesTicker, error) {
	return cg.getHighGranularityTickers("1")
}

func (cg *Coingecko) getHistoricalTicker(tickersToUpdate map[uint]*common.CurrencyRatesTicker, coinId string, vsCurrency string, token string) (bool, error) {
	lastTicker, err := cg.db.FiatRatesFindLastTicker(vsCurrency, token)
	if err != nil {
		return false, err
	}
	var days string
	if lastTicker == nil {
		days = "max"
	} else {
		diff := time.Since(lastTicker.Timestamp)
		d := int(diff / (24 * 3600 * 1000000000))
		if d == 0 { // nothing to do, the last ticker exist
			return false, nil
		}
		days = strconv.Itoa(d)
	}
	mc, err := cg.coinMarketChart(coinId, vsCurrency, days, true)
	if err != nil {
		return false, err
	}
	warningLogged := false
	for _, p := range mc.Prices {
		var timestamp uint
		timestamp = uint(p[0])
		if timestamp > 100000000000 {
			// convert timestamp from milliseconds to seconds
			timestamp /= 1000
		}
		rate := float32(p[1])
		if timestamp%(24*3600) == 0 && timestamp != 0 && rate != 0 { // process only tickers for the whole day with non 0 value
			var found bool
			var ticker *common.CurrencyRatesTicker
			if ticker, found = tickersToUpdate[timestamp]; !found {
				u := time.Unix(int64(timestamp), 0).UTC()
				ticker, err = cg.db.FiatRatesGetTicker(&u)
				if err != nil {
					return false, err
				}
				if ticker == nil {
					if token != "" { // if the base currency is not found in DB, do not create ticker for the token
						if !warningLogged {
							glog.Warningf("No base currency ticker for date %v for token %s", u, token)
							warningLogged = true
						}
						continue
					}
					ticker = &common.CurrencyRatesTicker{
						Timestamp: u,
						Rates:     make(map[string]float32),
					}
				}
				tickersToUpdate[timestamp] = ticker
			}
			if token == "" {
				ticker.Rates[vsCurrency] = rate
			} else {
				if ticker.TokenRates == nil {
					ticker.TokenRates = make(map[string]float32)
				}
				ticker.TokenRates[token] = rate
			}
		}
	}
	return true, nil
}

func (cg *Coingecko) storeTickers(tickersToUpdate map[uint]*common.CurrencyRatesTicker) error {
	if len(tickersToUpdate) > 0 {
		wb := grocksdb.NewWriteBatch()
		defer wb.Destroy()
		for _, v := range tickersToUpdate {
			if err := cg.db.FiatRatesStoreTicker(wb, v); err != nil {
				return err
			}
		}
		if err := cg.db.WriteBatch(wb); err != nil {
			return err
		}
	}
	return nil
}

func (cg *Coingecko) throttleHistoricalDownload() {
	// long delay next request to avoid throttling if downloading current tickers at the same time
	delay := 1
	if cg.updatingCurrent {
		delay = 600
	}
	time.Sleep(cg.throttlingDelay * time.Duration(delay))
}

// UpdateHistoricalTickers gets historical tickers for the main crypto currency
func (cg *Coingecko) UpdateHistoricalTickers() error {
	tickersToUpdate := make(map[uint]*common.CurrencyRatesTicker)

	// reload vs_currencies
	vs, err := cg.simpleSupportedVSCurrencies()
	if err != nil {
		return err
	}
	vsCurrencies = vs

	for _, currency := range vsCurrencies {
		// get historical rates for each currency
		var err error
		var req bool
		if req, err = cg.getHistoricalTicker(tickersToUpdate, cg.coin, currency, ""); err != nil {
			// report error and continue, Coingecko may return error like "Could not find coin with the given id"
			// the rates will be updated next run
			glog.Errorf("getHistoricalTicker %s-%s %v", cg.coin, currency, err)
		}
		if req {
			cg.throttleHistoricalDownload()
		}
	}

	return cg.storeTickers(tickersToUpdate)
}

// UpdateHistoricalTokenTickers gets historical tickers for the tokens
func (cg *Coingecko) UpdateHistoricalTokenTickers() error {
	if cg.updatingTokens {
		return nil
	}
	cg.updatingTokens = true
	defer func() { cg.updatingTokens = false }()
	tickersToUpdate := make(map[uint]*common.CurrencyRatesTicker)

	if cg.platformIdentifier != "" && cg.platformVsCurrency != "" {
		//  reload platform ids
		if err := cg.platformIds(); err != nil {
			return err
		}
		glog.Infof("Coingecko returned %d %s tokens ", len(platformIds), cg.coin)
		count := 0
		// get token historical rates
		for tokenId, token := range platformIdsToTokens {
			var err error
			var req bool
			if req, err = cg.getHistoricalTicker(tickersToUpdate, tokenId, cg.platformVsCurrency, token); err != nil {
				// report error and continue, Coingecko may return error like "Could not find coin with the given id"
				// the rates will be updated next run
				glog.Errorf("getHistoricalTicker %s-%s %v", tokenId, cg.platformVsCurrency, err)
			}
			count++
			if count%100 == 0 {
				err := cg.storeTickers(tickersToUpdate)
				if err != nil {
					return err
				}
				tickersToUpdate = make(map[uint]*common.CurrencyRatesTicker)
				glog.Infof("Coingecko updated %d of %d token tickers", count, len(platformIds))
			}
			if req {
				cg.throttleHistoricalDownload()
			}
		}
	}

	return cg.storeTickers(tickersToUpdate)
}
