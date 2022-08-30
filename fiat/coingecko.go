package fiat

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/linxGnu/grocksdb"
	"github.com/trezor/blockbook/db"
)

// Coingecko is a structure that implements RatesDownloaderInterface
type Coingecko struct {
	url                string
	coin               string
	platformIdentifier string
	platformVsCurrency string
	httpTimeoutSeconds time.Duration
	throttlingDelay    time.Duration
	timeFormat         string
	httpClient         *http.Client
	db                 *db.RocksDB
	updatingCurrent    bool
	updatingTokens     bool
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
func NewCoinGeckoDownloader(db *db.RocksDB, url string, coin string, platformIdentifier string, platformVsCurrency string, timeFormat string, throttleDown bool) RatesDownloaderInterface {
	var throttlingDelayMs int
	if throttleDown {
		throttlingDelayMs = 100
	}
	httpTimeoutSeconds := 15 * time.Second
	return &Coingecko{
		url:                url,
		coin:               coin,
		platformIdentifier: platformIdentifier,
		platformVsCurrency: platformVsCurrency,
		httpTimeoutSeconds: httpTimeoutSeconds,
		timeFormat:         timeFormat,
		httpClient: &http.Client{
			Timeout: httpTimeoutSeconds,
		},
		db:              db,
		throttlingDelay: time.Duration(throttlingDelayMs) * time.Millisecond,
	}
}

// doReq HTTP client
func doReq(req *http.Request, client *http.Client) ([]byte, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s", body)
	}
	return body, nil
}

// makeReq HTTP request helper - will retry the call after 1 minute on error
func (cg *Coingecko) makeReq(url string) ([]byte, error) {
	for {
		// glog.Infof("Coingecko makeReq %v", url)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := doReq(req, cg.httpClient)
		if err == nil {
			return resp, err
		}
		if err.Error() != "error code: 1015" && !strings.Contains(strings.ToLower(err.Error()), "exceeded the rate limit") {
			glog.Errorf("Coingecko makeReq %v error %v", url, err)
			return nil, err
		}
		// if there is a throttling error, wait 60 seconds and retry
		glog.Errorf("Coingecko makeReq %v error %v, will retry in 60 seconds", url, err)
		time.Sleep(60 * time.Second)
	}
}

// SimpleSupportedVSCurrencies /simple/supported_vs_currencies
func (cg *Coingecko) simpleSupportedVSCurrencies() (simpleSupportedVSCurrencies, error) {
	url := cg.url + "/simple/supported_vs_currencies"
	resp, err := cg.makeReq(url)
	if err != nil {
		return nil, err
	}
	var data simpleSupportedVSCurrencies
	err = json.Unmarshal(resp, &data)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// SimplePrice /simple/price Multiple ID and Currency (ids, vs_currencies)
func (cg *Coingecko) simplePrice(ids []string, vsCurrencies []string) (*map[string]map[string]float32, error) {
	params := url.Values{}
	idsParam := strings.Join(ids, ",")
	vsCurrenciesParam := strings.Join(vsCurrencies, ",")

	params.Add("ids", idsParam)
	params.Add("vs_currencies", vsCurrenciesParam)

	url := fmt.Sprintf("%s/simple/price?%s", cg.url, params.Encode())
	resp, err := cg.makeReq(url)
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
	resp, err := cg.makeReq(url)
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
func (cg *Coingecko) coinMarketChart(id string, vs_currency string, days string) (*marketChartPrices, error) {
	if len(id) == 0 || len(vs_currency) == 0 || len(days) == 0 {
		return nil, fmt.Errorf("id, vs_currency, and days is required")
	}

	params := url.Values{}
	params.Add("interval", "daily")
	params.Add("vs_currency", vs_currency)
	params.Add("days", days)

	url := fmt.Sprintf("%s/coins/%s/market_chart?%s", cg.url, id, params.Encode())
	resp, err := cg.makeReq(url)
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

func (cg *Coingecko) CurrentTickers() (*db.CurrencyRatesTicker, error) {
	cg.updatingCurrent = true
	defer func() { cg.updatingCurrent = false }()

	var newTickers = db.CurrencyRatesTicker{}

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
		const platformIdsGroup = 200
		for from := 0; from < len(platformIds); from += platformIdsGroup {
			to := from + platformIdsGroup
			if to > len(platformIds) {
				to = len(platformIds)
			}
			tokenPrices, err := cg.simplePrice(platformIds[from:to], []string{cg.platformVsCurrency})
			if err != nil || tokenPrices == nil {
				return nil, err
			}
			for id, v := range *tokenPrices {
				t, found := platformIdsToTokens[id]
				if found {
					newTickers.TokenRates[t] = v[cg.platformVsCurrency]
				}
			}
		}
	}
	newTickers.Timestamp = time.Now().UTC()
	return &newTickers, nil
}

func (cg *Coingecko) getHistoricalTicker(tickersToUpdate map[uint]*db.CurrencyRatesTicker, coinId string, vsCurrency string, token string) (bool, error) {
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
	mc, err := cg.coinMarketChart(coinId, vsCurrency, days)
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
			var ticker *db.CurrencyRatesTicker
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
					ticker = &db.CurrencyRatesTicker{
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

func (cg *Coingecko) storeTickers(tickersToUpdate map[uint]*db.CurrencyRatesTicker) error {
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
	tickersToUpdate := make(map[uint]*db.CurrencyRatesTicker)

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
	tickersToUpdate := make(map[uint]*db.CurrencyRatesTicker)

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
				tickersToUpdate = make(map[uint]*db.CurrencyRatesTicker)
				glog.Infof("Coingecko updated %d of %d token tickers", count, len(platformIds))
			}
			if req {
				cg.throttleHistoricalDownload()
			}
		}
	}

	return cg.storeTickers(tickersToUpdate)
}
