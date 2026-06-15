package fiat

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/linxGnu/grocksdb"
	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/db"
)

const (
	DefaultHTTPTimeout                = 15 * time.Second
	DefaultThrottleDelayMs            = 100
	coingeckoHistoryDaysLimit         = 365
	coingeckoKeylessRequestsPerMinute = 10
	coingeckoRangeHistorical          = "historical"
	coingeckoRangeTip                 = "tip"
	coingeckoRangeCapped              = "capped"
	coingeckoRangeBackfill            = "backfill"
	// coingeckoTipWindowDays is the only gap still served from the rate-limited tip
	// endpoint (the chain tip); anything larger is high-volume backfill routed to the CDN.
	coingeckoTipWindowDays = 1
	coingeckoBootstrapURL             = "https://cdn.trezor.io/dynamic/coingecko/api/v3"
	coingeckoProURL                   = "https://pro-api.coingecko.com/api/v3"
	coingeckoFreeURL                  = "https://api.coingecko.com/api/v3"
	coingeckoPlanFree                 = "free"
	coingeckoPlanPro                  = "pro"
	coingeckoAPIKeyEnv                = "COINGECKO_API_KEY"
	coingeckoAPIKeyEnvSuffix          = "_" + coingeckoAPIKeyEnv
)

// Phase labels for the fiat-rate fetch metrics (fiat_rates_fetched_units_total,
// fiat_rates_fetched_tokens_total, fiat_rates_unable_total).
const (
	fiatPhaseTip       = "tip"       // chain tip, gap == 1 day, Free tier
	fiatPhaseBackfill  = "backfill"  // gap > 1 day or first-seen series, via CDN
	fiatPhaseBootstrap = "bootstrap" // full-history population, via CDN
	fiatPhaseReconcile = "reconcile" // startup self-healing pass, via CDN
)

// Reason labels for fiat_rates_unable_total.
const (
	fiatUnableGapTooLarge  = "gap_too_large"  // series stale >= reconcile guard, probable bug
	fiatUnableFetchFailed  = "fetch_failed"   // HTTP/parse error fetching the range
	fiatUnableProviderBan  = "provider_banned" // Cloudflare 1015 IP ban
	fiatUnableNoBaseTicker = "no_base_ticker" // token day without an existing base-currency ticker
)

// used when retry-after header is missing
var coingeckoThrottleBackoff = time.Minute

var coingeckoLowPriorityPollInterval = time.Second

// reqPriority determines which requests reclaim request slots first while a
// shared throttle window (429) is open.
type reqPriority int

const (
	priorityHigh reqPriority = iota // current tickers + high-granularity fetches
	priorityLow                     // historical (daily) ticker updates
)

var errCoingeckoHistoricalTokenUpdateInProgress = errors.New("coingecko historical token update already in progress")

// Coingecko is a structure that implements RatesDownloaderInterface
type Coingecko struct {
	tipURL                   string
	bootstrapURL             string
	apiKey                   string
	coin                     string
	platformIdentifier       string
	platformVsCurrency       string
	allowedVsCurrencies      map[string]struct{}
	httpTimeout              time.Duration
	timeFormat               string
	httpClient               *http.Client
	db                       *db.RocksDB
	updatingTokens           bool
	metrics                  *common.Metrics
	plan                     string
	minHttpRequestInterval   time.Duration
	bootstrapRequestInterval time.Duration
	reqMu                    sync.Mutex
	lastRequestAt            time.Time
	throttledUntil           time.Time
	highPriorityInFlight     int
	cacheMu                  sync.Mutex
	vsCurrencies             []string
	platformIds              []string
	platformIdsToTokens      map[string]string
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

func coinGeckoScopedAPIKeyEnvNames(network string, coinShortcut string) []string {
	prefixes := []string{network, coinShortcut}
	seen := make(map[string]struct{}, len(prefixes))
	envNames := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		normalized := strings.ToUpper(strings.TrimSpace(prefix))
		if normalized == "" {
			continue
		}
		envName := normalized + coingeckoAPIKeyEnvSuffix
		if _, exists := seen[envName]; exists {
			continue
		}
		seen[envName] = struct{}{}
		envNames = append(envNames, envName)
	}
	return envNames
}

func resolveCoinGeckoAPIKey(network string, coinShortcut string) string {
	// Preserve network-prefixed variables for backward compatibility, but also
	// support documented <coin shortcut>_COINGECKO_API_KEY as a fallback.
	for _, envName := range coinGeckoScopedAPIKeyEnvNames(network, coinShortcut) {
		if apiKey := strings.TrimSpace(os.Getenv(envName)); apiKey != "" {
			return apiKey
		}
	}
	return strings.TrimSpace(os.Getenv(coingeckoAPIKeyEnv))
}

func validateCoinGeckoAPIKeyEnv(network string, coinShortcut string) error {
	for _, envName := range coinGeckoScopedAPIKeyEnvNames(network, coinShortcut) {
		if value, exists := os.LookupEnv(envName); exists && strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is set but empty", envName)
		}
	}

	if value, exists := os.LookupEnv(coingeckoAPIKeyEnv); exists && strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is set but empty", coingeckoAPIKeyEnv)
	}

	return nil
}

func normalizeCoinGeckoPlan(plan string) string {
	normalizedPlan := strings.ToLower(strings.TrimSpace(plan))
	if normalizedPlan == coingeckoPlanPro {
		return coingeckoPlanPro
	}
	return coingeckoPlanFree
}

func coingeckoPlanRequiresAPIKey(plan string) bool {
	return normalizeCoinGeckoPlan(plan) == coingeckoPlanPro
}

func resolveCoinGeckoBootstrapURL(bootstrapURL string) string {
	trimmedURL := strings.TrimSpace(bootstrapURL)
	if trimmedURL != "" {
		return trimmedURL
	}
	return coingeckoBootstrapURL
}

// NewCoinGeckoDownloader creates a coingecko structure that implements the RatesDownloaderInterface
func NewCoinGeckoDownloader(db *db.RocksDB, network string, coinShortcut string, bootstrapURL string, coin string, platformIdentifier string, platformVsCurrency string, allowedVsCurrencies string, timeFormat string, plan string, metrics *common.Metrics, throttleDown bool) RatesDownloaderInterface {
	allowedVsCurrenciesMap := getAllowedVsCurrenciesMap(allowedVsCurrencies)

	apiKey := resolveCoinGeckoAPIKey(network, coinShortcut)
	normalizedPlan := normalizeCoinGeckoPlan(plan)

	minRequestInterval := time.Duration(0)
	bootstrapRequestInterval := time.Duration(0)
	if throttleDown {
		if normalizedPlan == coingeckoPlanFree {
			minRequestInterval = time.Minute / coingeckoKeylessRequestsPerMinute
		} else {
			minRequestInterval = DefaultThrottleDelayMs * time.Millisecond
		}
		bootstrapRequestInterval = DefaultThrottleDelayMs * time.Millisecond
	}
	resolvedBootstrapURL := resolveCoinGeckoBootstrapURL(bootstrapURL)
	tipURL := coingeckoFreeURL
	if normalizedPlan == coingeckoPlanPro {
		tipURL = coingeckoProURL
	}
	glog.Infof("Coingecko downloader bootstrap url %s, tip url %s", resolvedBootstrapURL, tipURL)

	return &Coingecko{
		tipURL:              tipURL,
		bootstrapURL:        resolvedBootstrapURL,
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
		db:                       db,
		minHttpRequestInterval:   minRequestInterval,
		bootstrapRequestInterval: bootstrapRequestInterval,
		metrics:                  metrics,
		plan:                     normalizedPlan,
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

type coingeckoHTTPError struct {
	status     int
	body       string
	retryAfter time.Duration
}

func (e *coingeckoHTTPError) Error() string {
	return e.body
}

func parseRetryAfter(value string) time.Duration {
	secs, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || secs <= 0 {
		return 0
	}
	return time.Duration(secs) * time.Second
}

// retryAfterFromError extracts the server's Retry-After hint from a coingeckoHTTPError,
// if any; zero means no hint and the caller falls back to the fixed backoff.
func retryAfterFromError(err error) time.Duration {
	var httpErr *coingeckoHTTPError
	if errors.As(err, &httpErr) {
		return httpErr.retryAfter
	}
	return 0
}

func throttleBackoffDelay(err error) time.Duration {
	delay := retryAfterFromError(err)
	if delay <= 0 {
		delay = coingeckoThrottleBackoff
	}
	if delay <= 0 {
		return 0
	}
	return delay + time.Duration(rand.Int63n(int64(delay/5)+1))
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
		return nil, &coingeckoHTTPError{
			status:     resp.StatusCode,
			body:       string(body),
			retryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
		}
	}
	return body, nil
}

func isCoingeckoThrottleError(err error) bool {
	if err == nil {
		return false
	}
	var httpErr *coingeckoHTTPError
	if errors.As(err, &httpErr) && httpErr.status == http.StatusTooManyRequests {
		return true
	}
	lowerError := strings.ToLower(err.Error())
	// Cloudflare serves rate-limit blocks as a full HTML page that merely contains
	// "error code: 1015", so match it as a substring rather than the whole body.
	return strings.Contains(lowerError, "error code: 1015") ||
		strings.Contains(lowerError, "exceeded the rate limit") ||
		strings.Contains(lowerError, "throttled")
}

func isCoingeckoCloudflareBanError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "error code: 1015")
}

func isCoingeckoHistoricalTokenUpdateInProgressError(err error) bool {
	return errors.Is(err, errCoingeckoHistoricalTokenUpdateInProgress)
}

func (cg *Coingecko) targetsBootstrapCDN(url string) bool {
	return cg.bootstrapURL != "" && strings.HasPrefix(url, cg.bootstrapURL)
}

func (cg *Coingecko) requestIntervalFor(url string) time.Duration {
	if cg.targetsBootstrapCDN(url) {
		return cg.bootstrapRequestInterval
	}
	return cg.minHttpRequestInterval
}

func (cg *Coingecko) waitForRequestSlot(priority reqPriority, minInterval time.Duration) {
	for {
		cg.reqMu.Lock()
		now := time.Now()
		if priority == priorityLow && cg.throttledUntil.After(now) && cg.highPriorityInFlight > 0 {
			cg.reqMu.Unlock()
			time.Sleep(coingeckoLowPriorityPollInterval)
			continue
		}
		slot := now
		if !cg.lastRequestAt.IsZero() {
			if next := cg.lastRequestAt.Add(minInterval); next.After(slot) {
				slot = next
			}
		}
		if cg.throttledUntil.After(slot) {
			slot = cg.throttledUntil
		}
		cg.lastRequestAt = slot
		cg.reqMu.Unlock()
		if d := time.Until(slot); d > 0 {
			time.Sleep(d)
		}
		// Another goroutine's 429 may have extended throttledUntil while we slept on an
		// already-reserved slot; firing now would be a guaranteed 429, so re-enter the gate.
		cg.reqMu.Lock()
		stillThrottled := cg.throttledUntil.After(time.Now())
		cg.reqMu.Unlock()
		if !stillThrottled {
			return
		}
	}
}

// markThrottled opens (or extends) the shared throttle window for all requests.
func (cg *Coingecko) markThrottled(delay time.Duration) {
	cg.reqMu.Lock()
	if until := time.Now().Add(delay); until.After(cg.throttledUntil) {
		cg.throttledUntil = until // extend only, never shorten
	}
	cg.reqMu.Unlock()
}

// makeReq HTTP request helper with unbounded retries for throttling errors.
func (cg *Coingecko) makeReq(url string, endpoint string, plan string, priority reqPriority) ([]byte, error) {
	if priority == priorityHigh {
		cg.reqMu.Lock()
		cg.highPriorityInFlight++
		cg.reqMu.Unlock()
		defer func() {
			cg.reqMu.Lock()
			cg.highPriorityInFlight--
			cg.reqMu.Unlock()
		}()
	}
	minInterval := cg.requestIntervalFor(url)
	for attempt := 0; ; attempt++ {
		// glog.Infof("Coingecko makeReq %v", url)
		cg.waitForRequestSlot(priority, minInterval)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		if cg.apiKey != "" {
			// Use the paid-tier header by default when an API key is provided.
			if plan == "free" {
				req.Header.Set("x-cg-demo-api-key", cg.apiKey)
			} else {
				req.Header.Set("x-cg-pro-api-key", cg.apiKey)
			}
		}
		resp, err := doReq(req, cg.httpClient)
		if err == nil {
			if cg.metrics != nil {
				cg.metrics.CoingeckoRequests.With(common.Labels{"endpoint": endpoint, "status": "success"}).Inc()
			}
			return resp, err
		}

		if isCoingeckoCloudflareBanError(err) {
			if cg.metrics != nil {
				cg.metrics.CoingeckoRequests.With(common.Labels{"endpoint": endpoint, "status": "banned"}).Inc()
			}
			glog.Warningf("Coingecko makeReq %v Cloudflare rate-limit ban (1015), fast-failing: %v", url, err)
			return nil, err
		}
		if !isCoingeckoThrottleError(err) {
			if cg.metrics != nil {
				cg.metrics.CoingeckoRequests.With(common.Labels{"endpoint": endpoint, "status": "error"}).Inc()
			}
			glog.Errorf("Coingecko makeReq %v error %v", url, err)
			return nil, err
		}
		if cg.metrics != nil {
			cg.metrics.CoingeckoRequests.With(common.Labels{"endpoint": endpoint, "status": "throttle"}).Inc()
		}

		delay := throttleBackoffDelay(err)
		cg.markThrottled(delay)
		glog.Warningf("Coingecko makeReq %v throttled on attempt %d, retrying in %v: %v", url, attempt+1, delay, err)
	}
}

// SimpleSupportedVSCurrencies /simple/supported_vs_currencies
func (cg *Coingecko) simpleSupportedVSCurrenciesAt(baseURL string, priority reqPriority) (simpleSupportedVSCurrencies, error) {
	url := baseURL + "/simple/supported_vs_currencies"
	resp, err := cg.makeReq(url, "supported_vs_currencies", cg.plan, priority)
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

	url := fmt.Sprintf("%s/simple/price?%s", cg.tipURL, params.Encode())
	// only called from CurrentTickers, therefore always high priority
	resp, err := cg.makeReq(url, "simple/price", cg.plan, priorityHigh)
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
func (cg *Coingecko) coinsListAt(baseURL string, priority reqPriority) (coinList, error) {
	params := url.Values{}
	platform := "false"
	if cg.platformIdentifier != "" {
		platform = "true"
	}
	params.Add("include_platform", platform)
	url := fmt.Sprintf("%s/coins/list?%s", baseURL, params.Encode())
	resp, err := cg.makeReq(url, "coins/list", cg.plan, priority)
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
func (cg *Coingecko) coinMarketChartAt(baseURL string, id string, vs_currency string, days string, daily bool, priority reqPriority) (*marketChartPrices, error) {
	if len(id) == 0 || len(vs_currency) == 0 || len(days) == 0 {
		return nil, fmt.Errorf("id, vs_currency, and days is required")
	}

	params := url.Values{}
	if daily {
		params.Add("interval", "daily")
	}
	params.Add("vs_currency", vs_currency)
	params.Add("days", days)

	url := fmt.Sprintf("%s/coins/%s/market_chart?%s", baseURL, id, params.Encode())
	resp, err := cg.makeReq(url, "market_chart", cg.plan, priority)
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

func (cg *Coingecko) platformIdsAt(baseURL string, priority reqPriority) error {
	if cg.platformIdentifier == "" {
		return nil
	}
	cl, err := cg.coinsListAt(baseURL, priority)
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
	cg.cacheMu.Lock()
	cg.platformIds = ids
	cg.platformIdsToTokens = idsMap
	cg.cacheMu.Unlock()
	return nil
}

// CurrentTickers returns the latest exchange rates
func (cg *Coingecko) CurrentTickers() (*common.CurrencyRatesTicker, error) {
	var newTickers = common.CurrencyRatesTicker{}

	cg.cacheMu.Lock()
	vsCurrencies := cg.vsCurrencies
	cg.cacheMu.Unlock()
	if vsCurrencies == nil {
		vs, err := cg.simpleSupportedVSCurrenciesAt(cg.tipURL, priorityHigh)
		if err != nil {
			return nil, err
		}
		vsCurrencies = vs
		cg.cacheMu.Lock()
		cg.vsCurrencies = vs
		cg.cacheMu.Unlock()
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
		cg.cacheMu.Lock()
		platformIds, platformIdsToTokens := cg.platformIds, cg.platformIdsToTokens
		cg.cacheMu.Unlock()
		if platformIdsToTokens == nil {
			err = cg.platformIdsAt(cg.tipURL, priorityHigh)
			if err != nil {
				return nil, err
			}
			cg.cacheMu.Lock()
			platformIds, platformIdsToTokens = cg.platformIds, cg.platformIdsToTokens
			cg.cacheMu.Unlock()
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
	mc, err := cg.coinMarketChartAt(cg.tipURL, cg.coin, highGranularityVsCurrency, days, false, priorityHigh)
	if err != nil {
		return nil, err
	}
	if len(mc.Prices) < 2 {
		return nil, fmt.Errorf("not enough price points: %d", len(mc.Prices))
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

// FiveMinutesTickers returns the array of the exchange rates in five minutes granularity
func (cg *Coingecko) FiveMinutesTickers() (*[]common.CurrencyRatesTicker, error) {
	return cg.getHighGranularityTickers("1")
}

func coingeckoBootstrapURLAllowed(bootstrapURL string) bool {
	return strings.TrimSpace(bootstrapURL) != ""
}

func coingeckoBootstrapPreconditionError() error {
	return fmt.Errorf("coingecko bootstrap is not possible: missing bootstrap URL")
}

func (cg *Coingecko) canUseBootstrapMax() bool {
	return coingeckoBootstrapURLAllowed(cg.bootstrapURL)
}

// metadataURL returns the base URL for low-volume metadata calls
// (supported_vs_currencies, coins/list). These are cheap and stable, so prefer the CDN
// (no rate limit) whenever it is configured, keeping the Free tier reserved for the tip.
func (cg *Coingecko) metadataURL() string {
	if cg.bootstrapURL != "" {
		return cg.bootstrapURL
	}
	return cg.tipURL
}

// sourceURLForRange routes each historical range to the right backend: the rate-limited
// tip endpoint is used only for the chain tip (gap == 1 day); every higher-volume range
// (backfill, capped, full-history) goes to the CDN when configured. When no CDN URL is
// configured we fall back to the tip endpoint to preserve pre-CDN behavior.
func (cg *Coingecko) sourceURLForRange(rangeKind string) string {
	if rangeKind == coingeckoRangeTip {
		return cg.tipURL
	}
	if cg.bootstrapURL != "" {
		return cg.bootstrapURL
	}
	return cg.tipURL
}

// phaseForRange maps a resolved range kind to the metric phase label.
func phaseForRange(rangeKind string) string {
	switch rangeKind {
	case coingeckoRangeTip:
		return fiatPhaseTip
	case coingeckoRangeHistorical:
		return fiatPhaseBootstrap
	default: // backfill + capped are both high-volume CDN ranges
		return fiatPhaseBackfill
	}
}

func (cg *Coingecko) resolveHistoricalDays(lastTicker *common.CurrencyRatesTicker, allowMax bool) (string, bool, string) {
	if lastTicker == nil {
		if allowMax {
			// Bootstrap mode only: for the very first full historical population use full range.
			return "max", true, coingeckoRangeHistorical
		}
		// Non-bootstrap mode: first-seen token/vsCurrency must stay within free-plan-compatible window.
		return strconv.Itoa(coingeckoHistoryDaysLimit), true, coingeckoRangeCapped
	}
	diff := time.Since(lastTicker.Timestamp)
	d := int(diff / (24 * 3600 * 1000000000))
	if d == 0 { // nothing to do, the last ticker exist
		return "", false, ""
	}
	if d <= coingeckoTipWindowDays {
		// The chain tip: a single missing day, served from the rate-limited tip endpoint.
		return strconv.Itoa(d), true, coingeckoRangeTip
	}
	if d > coingeckoHistoryDaysLimit {
		// This happens when the latest stored ticker for a given series is older than 365 days
		// (for example after downtime, stale/partial historical data, or a newly tracked series
		// after bootstrap). We intentionally cap backfill to 365 days.
		return strconv.Itoa(coingeckoHistoryDaysLimit), true, coingeckoRangeCapped
	}
	// A multi-day gap (downtime catch-up): high-volume, routed to the CDN.
	return strconv.Itoa(d), true, coingeckoRangeBackfill
}

// fillFromMarketChart writes the whole-day points from a market_chart response into
// tickersToUpdate. With fillMissingOnly set, an already-present rate for the series is left
// intact (used by the startup reconciliation so it repairs holes without rewriting existing
// data). Returns the number of daily points written and the number of token points skipped
// because their day had no base-currency ticker yet.
func (cg *Coingecko) fillFromMarketChart(tickersToUpdate map[uint]*common.CurrencyRatesTicker, mc *marketChartPrices, vsCurrency string, token string, fillMissingOnly bool) (written int, noBaseTicker int, err error) {
	warningLogged := false
	for _, p := range mc.Prices {
		timestamp := uint(p[0])
		if timestamp > 100000000000 {
			// convert timestamp from milliseconds to seconds
			timestamp /= 1000
		}
		rate := float32(p[1])
		if timestamp%(24*3600) != 0 || timestamp == 0 || rate == 0 {
			// process only tickers for the whole day with non 0 value
			continue
		}
		ticker, found := tickersToUpdate[timestamp]
		if !found {
			u := time.Unix(int64(timestamp), 0).UTC()
			ticker, err = cg.db.FiatRatesGetTicker(&u)
			if err != nil {
				return written, noBaseTicker, err
			}
			if ticker == nil {
				if token != "" { // if the base currency is not found in DB, do not create ticker for the token
					noBaseTicker++
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
			if fillMissingOnly {
				if _, ok := ticker.Rates[vsCurrency]; ok {
					continue
				}
			}
			ticker.Rates[vsCurrency] = rate
		} else {
			if ticker.TokenRates == nil {
				ticker.TokenRates = make(map[string]float32)
			}
			if fillMissingOnly {
				if _, ok := ticker.TokenRates[token]; ok {
					continue
				}
			}
			ticker.TokenRates[token] = rate
		}
		written++
	}
	return written, noBaseTicker, nil
}

func (cg *Coingecko) getHistoricalTicker(tickersToUpdate map[uint]*common.CurrencyRatesTicker, coinId string, vsCurrency string, token string, allowMax bool) (bool, error) {
	lastTicker, err := cg.db.FiatRatesFindLastTicker(vsCurrency, token)
	if err != nil {
		return false, err
	}
	days, shouldRequest, rangeKind := cg.resolveHistoricalDays(lastTicker, allowMax)
	if !shouldRequest {
		return false, nil
	}
	if cg.metrics != nil {
		cg.metrics.CoingeckoRangeRequests.With(common.Labels{"range": rangeKind}).Inc()
	}
	// the tip (gap == 1 day) stays on the rate-limited endpoint; everything larger uses the CDN
	baseURL := cg.sourceURLForRange(rangeKind)
	phase := phaseForRange(rangeKind)
	// both callers update historical (daily) tickers, therefore always low priority
	mc, err := cg.coinMarketChartAt(baseURL, coinId, vsCurrency, days, true, priorityLow)
	if err != nil {
		return false, err
	}
	written, noBaseTicker, err := cg.fillFromMarketChart(tickersToUpdate, mc, vsCurrency, token, false)
	if err != nil {
		return false, err
	}
	cg.observeFetchedUnits(phase, written)
	if token != "" && written > 0 {
		cg.observeFetchedToken(phase)
	}
	cg.observeUnable(phase, fiatUnableNoBaseTicker, noBaseTicker)
	return true, nil
}

func (cg *Coingecko) observeFetchedUnits(phase string, n int) {
	if cg.metrics != nil && n > 0 {
		cg.metrics.FiatRatesFetchedUnits.With(common.Labels{"phase": phase}).Add(float64(n))
	}
}

func (cg *Coingecko) observeFetchedToken(phase string) {
	if cg.metrics != nil {
		cg.metrics.FiatRatesFetchedTokens.With(common.Labels{"phase": phase}).Inc()
	}
}

func (cg *Coingecko) observeUnable(phase string, reason string, n int) {
	if cg.metrics != nil && n > 0 {
		cg.metrics.FiatRatesUnable.With(common.Labels{"phase": phase, "reason": reason}).Add(float64(n))
	}
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

// UpdateHistoricalTickers gets historical tickers for the main crypto currency
func (cg *Coingecko) UpdateHistoricalTickers() error {
	tickersToUpdate := make(map[uint]*common.CurrencyRatesTicker)
	allowMax := false
	bootstrapInProgress, _, err := historicalBootstrapInProgress(cg.db)
	if err != nil {
		return err
	}
	metadataURL := cg.metadataURL()
	if bootstrapInProgress {
		if !cg.canUseBootstrapMax() {
			return coingeckoBootstrapPreconditionError()
		}
		allowMax = true
	}

	// reload vs_currencies
	vs, err := cg.simpleSupportedVSCurrenciesAt(metadataURL, priorityLow)
	if err != nil {
		return err
	}
	cg.cacheMu.Lock()
	cg.vsCurrencies = vs
	cg.cacheMu.Unlock()

	hadFailures := false
	var banErr error
	for _, currency := range vs {
		// get historical rates for each currency
		var err error
		if _, err = cg.getHistoricalTicker(tickersToUpdate, cg.coin, currency, "", allowMax); err != nil {
			hadFailures = true
			// report error and continue, Coingecko may return error like "Could not find coin with the given id"
			// the rates will be updated next run
			glog.Errorf("getHistoricalTicker %s-%s %v", cg.coin, currency, err)
			if isCoingeckoCloudflareBanError(err) {
				banErr = err
				break
			}
		}
	}
	if err := cg.storeTickers(tickersToUpdate); err != nil {
		return err
	}
	if banErr != nil {
		return banErr
	}
	if bootstrapInProgress && hadFailures {
		return fmt.Errorf("coingecko historical bootstrap incomplete: one or more currency updates failed")
	}
	return nil
}

// UpdateHistoricalTokenTickers gets historical tickers for the tokens
func (cg *Coingecko) UpdateHistoricalTokenTickers() error {
	cg.reqMu.Lock()
	if cg.updatingTokens {
		cg.reqMu.Unlock()
		return errCoingeckoHistoricalTokenUpdateInProgress
	}
	cg.updatingTokens = true
	cg.reqMu.Unlock()
	defer func() {
		cg.reqMu.Lock()
		cg.updatingTokens = false
		cg.reqMu.Unlock()
	}()
	tickersToUpdate := make(map[uint]*common.CurrencyRatesTicker)

	if cg.platformIdentifier != "" && cg.platformVsCurrency != "" {
		allowMax := false
		bootstrapInProgress, _, err := historicalBootstrapInProgress(cg.db)
		if err != nil {
			return err
		}
		metadataURL := cg.metadataURL()
		if bootstrapInProgress {
			if !cg.canUseBootstrapMax() {
				return coingeckoBootstrapPreconditionError()
			}
			allowMax = true
		}

		//  reload platform ids
		if err := cg.platformIdsAt(metadataURL, priorityLow); err != nil {
			return err
		}
		cg.cacheMu.Lock()
		platformIds, platformIdsToTokens := cg.platformIds, cg.platformIdsToTokens
		cg.cacheMu.Unlock()
		glog.Infof("Coingecko returned %d %s tokens ", len(platformIds), cg.coin)
		count := 0
		var banErr error
		// get token historical rates
		for tokenId, token := range platformIdsToTokens {
			var err error
			if _, err = cg.getHistoricalTicker(tickersToUpdate, tokenId, cg.platformVsCurrency, token, allowMax); err != nil {
				// report error and continue, Coingecko may return error like "Could not find coin with the given id"
				// the rates will be updated next run
				glog.Errorf("getHistoricalTicker %s-%s %v", tokenId, cg.platformVsCurrency, err)
				if isCoingeckoCloudflareBanError(err) {
					banErr = err
					break
				}
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
		}
		if banErr != nil {
			if err := cg.storeTickers(tickersToUpdate); err != nil {
				return err
			}
			return banErr
		}
	}

	if err := cg.storeTickers(tickersToUpdate); err != nil {
		return err
	}
	return nil
}
