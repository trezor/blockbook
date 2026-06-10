//go:build unittest

package fiat

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/trezor/blockbook/common"
)

func testCoinGeckoScopedAPIKeyEnvName(prefix string) string {
	return strings.ToUpper(strings.TrimSpace(prefix)) + coingeckoAPIKeyEnvSuffix
}

func TestResolveCoinGeckoAPIKey(t *testing.T) {
	t.Run("prefers network-specific key", func(t *testing.T) {
		t.Setenv(testCoinGeckoScopedAPIKeyEnvName("OP"), "network-key")
		t.Setenv(testCoinGeckoScopedAPIKeyEnvName("ETH"), "shortcut-key")
		t.Setenv(coingeckoAPIKeyEnv, "global-key")

		got := resolveCoinGeckoAPIKey("op", "eth")
		if got != "network-key" {
			t.Fatalf("unexpected api key: got %q, want %q", got, "network-key")
		}
	})

	t.Run("falls back to shortcut key when network is unrecognized", func(t *testing.T) {
		t.Setenv(testCoinGeckoScopedAPIKeyEnvName("ETH"), "shortcut-key")
		t.Setenv(coingeckoAPIKeyEnv, "global-key")

		got := resolveCoinGeckoAPIKey("unrecognized", "eth")
		if got != "shortcut-key" {
			t.Fatalf("unexpected api key: got %q, want %q", got, "shortcut-key")
		}
	})

	t.Run("falls back to global key when prefixed keys are missing", func(t *testing.T) {
		t.Setenv(coingeckoAPIKeyEnv, "global-key")

		got := resolveCoinGeckoAPIKey("unrecognized", "unknown")
		if got != "global-key" {
			t.Fatalf("unexpected api key: got %q, want %q", got, "global-key")
		}
	})
}

func TestValidateCoinGeckoAPIKeyEnv(t *testing.T) {
	t.Run("network key set empty returns error", func(t *testing.T) {
		networkEnvName := testCoinGeckoScopedAPIKeyEnvName("OP")
		t.Setenv(networkEnvName, "")
		err := validateCoinGeckoAPIKeyEnv("op", "eth")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), networkEnvName) {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("shortcut key set empty returns error when network key unset", func(t *testing.T) {
		shortcutEnvName := testCoinGeckoScopedAPIKeyEnvName("ETH")
		t.Setenv(shortcutEnvName, "   ")
		err := validateCoinGeckoAPIKeyEnv("op", "eth")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), shortcutEnvName) {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("global key set empty returns error", func(t *testing.T) {
		t.Setenv(coingeckoAPIKeyEnv, "")
		err := validateCoinGeckoAPIKeyEnv("op", "eth")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), coingeckoAPIKeyEnv) {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("unset keys are allowed", func(t *testing.T) {
		if err := validateCoinGeckoAPIKeyEnv("op", "eth"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("set non-empty keys are allowed", func(t *testing.T) {
		t.Setenv(testCoinGeckoScopedAPIKeyEnvName("OP"), "network-key")
		t.Setenv(testCoinGeckoScopedAPIKeyEnvName("ETH"), "shortcut-key")
		t.Setenv(coingeckoAPIKeyEnv, "global-key")
		if err := validateCoinGeckoAPIKeyEnv("op", "eth"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestCanUseBootstrapMax(t *testing.T) {
	tests := []struct {
		name        string
		cg          *Coingecko
		expectAllow bool
	}{
		{
			name:        "bootstrap url allows max",
			cg:          &Coingecko{bootstrapURL: "https://cdn.trezor.io/dynamic/coingecko/api/v3"},
			expectAllow: true,
		},
		{
			name:        "missing bootstrap url does not allow max",
			cg:          &Coingecko{},
			expectAllow: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cg.canUseBootstrapMax(); got != tt.expectAllow {
				t.Fatalf("unexpected bootstrap-max eligibility: got %v, want %v", got, tt.expectAllow)
			}
		})
	}
}

func TestNormalizeCoinGeckoPlan(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "pro", in: "pro", want: coingeckoPlanPro},
		{name: "pro uppercase", in: "PRO", want: coingeckoPlanPro},
		{name: "free", in: "free", want: coingeckoPlanFree},
		{name: "empty defaults to free", in: "", want: coingeckoPlanFree},
		{name: "unknown defaults to free", in: "demo", want: coingeckoPlanFree},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeCoinGeckoPlan(tt.in)
			if got != tt.want {
				t.Fatalf("unexpected plan normalization: got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCoingeckoPlanRequiresAPIKey(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "pro requires key", in: "pro", want: true},
		{name: "pro uppercase requires key", in: "PRO", want: true},
		{name: "free does not require key", in: "free", want: false},
		{name: "empty does not require key", in: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := coingeckoPlanRequiresAPIKey(tt.in)
			if got != tt.want {
				t.Fatalf("unexpected API-key requirement: got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveHistoricalDays(t *testing.T) {
	t.Run("nil last ticker uses max only when allowed", func(t *testing.T) {
		cg := Coingecko{}
		days, shouldRequest, rangeKind := cg.resolveHistoricalDays(nil, true)
		if !shouldRequest || days != "max" {
			t.Fatalf("unexpected max result: days=%q shouldRequest=%v", days, shouldRequest)
		}
		if rangeKind != coingeckoRangeHistorical {
			t.Fatalf("unexpected range kind: got %q, want %q", rangeKind, coingeckoRangeHistorical)
		}

		days, shouldRequest, rangeKind = cg.resolveHistoricalDays(nil, false)
		if !shouldRequest || days != "365" {
			t.Fatalf("unexpected capped result: days=%q shouldRequest=%v", days, shouldRequest)
		}
		if rangeKind != coingeckoRangeCapped {
			t.Fatalf("unexpected range kind: got %q, want %q", rangeKind, coingeckoRangeCapped)
		}
	})

	t.Run("same day ticker skips request", func(t *testing.T) {
		cg := Coingecko{}
		days, shouldRequest, rangeKind := cg.resolveHistoricalDays(&common.CurrencyRatesTicker{
			Timestamp: time.Now().Add(-1 * time.Hour),
		}, false)
		if shouldRequest || days != "" {
			t.Fatalf("unexpected same-day result: days=%q shouldRequest=%v", days, shouldRequest)
		}
		if rangeKind != "" {
			t.Fatalf("unexpected range kind: got %q, want empty", rangeKind)
		}
	})

	t.Run("older ticker is capped to 365 days", func(t *testing.T) {
		cg := Coingecko{}
		days, shouldRequest, rangeKind := cg.resolveHistoricalDays(&common.CurrencyRatesTicker{
			Timestamp: time.Now().AddDate(0, 0, -500),
		}, true)
		if !shouldRequest || days != "365" {
			t.Fatalf("unexpected capped result: days=%q shouldRequest=%v", days, shouldRequest)
		}
		if rangeKind != coingeckoRangeCapped {
			t.Fatalf("unexpected range kind: got %q, want %q", rangeKind, coingeckoRangeCapped)
		}
	})

	t.Run("recent ticker is tip query", func(t *testing.T) {
		cg := Coingecko{}
		days, shouldRequest, rangeKind := cg.resolveHistoricalDays(&common.CurrencyRatesTicker{
			Timestamp: time.Now().AddDate(0, 0, -5),
		}, false)
		if !shouldRequest || days != "5" {
			t.Fatalf("unexpected tip result: days=%q shouldRequest=%v", days, shouldRequest)
		}
		if rangeKind != coingeckoRangeTip {
			t.Fatalf("unexpected range kind: got %q, want %q", rangeKind, coingeckoRangeTip)
		}
	})
}

func TestUpdateHistoricalTickers_BootstrapStoresSuccessfulCurrenciesEvenWhenSomeFail(t *testing.T) {
	config := common.Config{
		CoinName: "fakecoin",
	}
	d, _, tmp := setupRocksDB(t, &testBitcoinParser{
		BitcoinParser: bitcoinTestnetParser(),
	}, &config)
	defer closeAndDestroyRocksDB(t, d, tmp)

	if err := d.FiatRatesSetHistoricalBootstrapComplete(false); err != nil {
		t.Fatalf("FiatRatesSetHistoricalBootstrapComplete failed: %v", err)
	}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/simple/supported_vs_currencies":
			_, _ = w.Write([]byte(`["usd","eur"]`))
		case "/coins/ethereum/market_chart":
			switch r.URL.Query().Get("vs_currency") {
			case "usd":
				_, _ = w.Write([]byte(`{"prices":[[1654732800000,1234.5]]}`))
			case "eur":
				http.Error(w, "forced-failure", http.StatusInternalServerError)
			default:
				http.Error(w, "unexpected vs_currency", http.StatusBadRequest)
			}
		default:
			http.Error(w, fmt.Sprintf("unexpected path %s", r.URL.Path), http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	cg := &Coingecko{
		coin:         "ethereum",
		bootstrapURL: mockServer.URL,
		tipURL:       mockServer.URL,
		httpClient:   mockServer.Client(),
		db:           d,
		plan:         coingeckoPlanFree,
	}

	err := cg.UpdateHistoricalTickers()
	if err == nil {
		t.Fatal("expected bootstrap incomplete error")
	}
	if !strings.Contains(err.Error(), "bootstrap incomplete") {
		t.Fatalf("unexpected error: %v", err)
	}

	usdTicker, err := d.FiatRatesFindLastTicker("usd", "")
	if err != nil {
		t.Fatalf("FiatRatesFindLastTicker usd failed: %v", err)
	}
	if usdTicker == nil {
		t.Fatal("expected usd ticker to be stored despite partial failure")
	}
	eurTicker, err := d.FiatRatesFindLastTicker("eur", "")
	if err != nil {
		t.Fatalf("FiatRatesFindLastTicker eur failed: %v", err)
	}
	if eurTicker != nil {
		t.Fatalf("expected eur ticker to be missing due to forced failure, got %+v", eurTicker)
	}
}

func TestMakeReq_ThrottleRetriesExhausted(t *testing.T) {
	originalBackoff := coingeckoThrottleRetryBackoff
	coingeckoThrottleRetryBackoff = []time.Duration{0, 0, 0, 0}
	defer func() {
		coingeckoThrottleRetryBackoff = originalBackoff
	}()

	var requests atomic.Int32
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "exceeded the rate limit", http.StatusTooManyRequests)
	}))
	defer mockServer.Close()

	cg := &Coingecko{
		httpClient: mockServer.Client(),
	}
	_, err := cg.makeReq(mockServer.URL, "market_chart", coingeckoPlanFree)
	if err == nil {
		t.Fatal("expected makeReq to fail after retries are exhausted")
	}
	wantRequests := 1 + len(coingeckoThrottleRetryBackoff)
	if got := int(requests.Load()); got != wantRequests {
		t.Fatalf("unexpected number of requests: got %d, want %d", got, wantRequests)
	}
}

func TestMakeReq_ThrottleRetriesEventuallySuccess(t *testing.T) {
	originalBackoff := coingeckoThrottleRetryBackoff
	coingeckoThrottleRetryBackoff = []time.Duration{0, 0, 0, 0}
	defer func() {
		coingeckoThrottleRetryBackoff = originalBackoff
	}()

	var requests atomic.Int32
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) <= 2 {
			http.Error(w, "throttled", http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer mockServer.Close()

	cg := &Coingecko{
		httpClient: mockServer.Client(),
	}
	resp, err := cg.makeReq(mockServer.URL, "market_chart", coingeckoPlanFree)
	if err != nil {
		t.Fatalf("makeReq unexpectedly failed: %v", err)
	}
	if string(resp) != `{"ok":true}` {
		t.Fatalf("unexpected response body: %s", string(resp))
	}
	if got := int(requests.Load()); got != 3 {
		t.Fatalf("unexpected number of requests: got %d, want %d", got, 3)
	}
}

func TestIsCoingeckoThrottleError_StatusCode(t *testing.T) {
	// A 429 must be detected even when the body has none of the legacy throttle keywords.
	if !isCoingeckoThrottleError(&coingeckoHTTPError{status: http.StatusTooManyRequests, body: `{"msg":"slow down"}`}) {
		t.Fatal("expected 429 to be detected as throttle")
	}
	if isCoingeckoThrottleError(&coingeckoHTTPError{status: http.StatusInternalServerError, body: "boom"}) {
		t.Fatal("did not expect 500 to be detected as throttle")
	}
	// Legacy keyless-endpoint signal still matches via body text.
	if !isCoingeckoThrottleError(errors.New("error code: 1015")) {
		t.Fatal("expected Cloudflare 1015 to be detected as throttle")
	}
	if isCoingeckoThrottleError(nil) {
		t.Fatal("nil error must not be a throttle error")
	}
}

func TestMakeReq_Detects429ByStatus(t *testing.T) {
	originalBackoff := coingeckoThrottleRetryBackoff
	coingeckoThrottleRetryBackoff = []time.Duration{0, 0, 0, 0}
	defer func() {
		coingeckoThrottleRetryBackoff = originalBackoff
	}()

	var requests atomic.Int32
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 429 with a body that contains none of the legacy keywords; detection must rely on status.
		if requests.Add(1) <= 2 {
			http.Error(w, `{"error":"please slow down"}`, http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer mockServer.Close()

	cg := &Coingecko{httpClient: mockServer.Client()}
	resp, err := cg.makeReq(mockServer.URL, "market_chart", coingeckoPlanFree)
	if err != nil {
		t.Fatalf("makeReq unexpectedly failed: %v", err)
	}
	if string(resp) != `{"ok":true}` {
		t.Fatalf("unexpected response body: %s", string(resp))
	}
	if got := int(requests.Load()); got != 3 {
		t.Fatalf("unexpected number of requests: got %d, want %d", got, 3)
	}
}

func TestMakeReq_PacesRequests(t *testing.T) {
	interval := 60 * time.Millisecond
	var requests atomic.Int32
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer mockServer.Close()

	cg := &Coingecko{
		httpClient:             mockServer.Client(),
		minHttpRequestInterval: interval,
	}
	start := time.Now()
	for i := 0; i < 3; i++ {
		if _, err := cg.makeReq(mockServer.URL, "simple/price", coingeckoPlanFree); err != nil {
			t.Fatalf("makeReq failed on call %d: %v", i, err)
		}
	}
	elapsed := time.Since(start)
	// 3 requests => 2 enforced gaps of at least `interval` each.
	if minElapsed := 2 * interval; elapsed < minElapsed {
		t.Fatalf("expected pacing to take at least %v across 3 requests, took %v", minElapsed, elapsed)
	}
	if got := int(requests.Load()); got != 3 {
		t.Fatalf("unexpected number of requests: got %d, want %d", got, 3)
	}
}

// In steady state (bootstrap complete) a throttled currency must not abort the whole pass:
// the loop continues with the remaining currencies, and the throttle error is still returned so
// the cycle retries the unfinished ones next run.
func TestUpdateHistoricalTickers_ContinuesPastThrottleInSteadyState(t *testing.T) {
	config := common.Config{
		CoinName: "fakecoin",
	}
	d, _, tmp := setupRocksDB(t, &testBitcoinParser{
		BitcoinParser: bitcoinTestnetParser(),
	}, &config)
	defer closeAndDestroyRocksDB(t, d, tmp)

	if err := d.FiatRatesSetHistoricalBootstrapComplete(true); err != nil {
		t.Fatalf("FiatRatesSetHistoricalBootstrapComplete failed: %v", err)
	}
	originalVsCurrencies := vsCurrencies
	originalPlatformIds := platformIds
	originalPlatformIdsToTokens := platformIdsToTokens
	defer func() {
		vsCurrencies = originalVsCurrencies
		platformIds = originalPlatformIds
		platformIdsToTokens = originalPlatformIdsToTokens
	}()

	originalBackoff := coingeckoThrottleRetryBackoff
	coingeckoThrottleRetryBackoff = []time.Duration{0, 0, 0, 0}
	defer func() {
		coingeckoThrottleRetryBackoff = originalBackoff
	}()

	var usdRequests atomic.Int32
	var eurRequests atomic.Int32

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/simple/supported_vs_currencies":
			_, _ = w.Write([]byte(`["usd","eur"]`))
		case "/coins/ethereum/market_chart":
			switch r.URL.Query().Get("vs_currency") {
			case "usd":
				usdRequests.Add(1)
				http.Error(w, "exceeded the rate limit", http.StatusTooManyRequests)
			case "eur":
				eurRequests.Add(1)
				_, _ = w.Write([]byte(`{"prices":[[1654732800000,1234.5]]}`))
			default:
				http.Error(w, "unexpected vs_currency", http.StatusBadRequest)
			}
		default:
			http.Error(w, fmt.Sprintf("unexpected path %s", r.URL.Path), http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	cg := &Coingecko{
		coin:         "ethereum",
		bootstrapURL: mockServer.URL,
		tipURL:       mockServer.URL,
		httpClient:   mockServer.Client(),
		db:           d,
		plan:         coingeckoPlanFree,
	}

	err := cg.UpdateHistoricalTickers()
	if err == nil {
		t.Fatal("expected throttle exhaustion error")
	}
	if !isCoingeckoThrottleRetriesExhaustedError(err) {
		t.Fatalf("expected throttle exhaustion error, got %v", err)
	}

	wantUSDRequests := 1 + len(coingeckoThrottleRetryBackoff)
	if got := int(usdRequests.Load()); got != wantUSDRequests {
		t.Fatalf("unexpected usd request count: got %d, want %d", got, wantUSDRequests)
	}
	// eur must still be attempted after usd was throttled (continue, not break).
	if got := int(eurRequests.Load()); got != 1 {
		t.Fatalf("expected eur to be requested once after usd throttle, got %d", got)
	}
	// the eur ticker that succeeded must be persisted despite the usd throttle.
	eurTicker, err := d.FiatRatesFindLastTicker("eur", "")
	if err != nil {
		t.Fatalf("FiatRatesFindLastTicker eur failed: %v", err)
	}
	if eurTicker == nil {
		t.Fatal("expected eur ticker to be stored after continuing past usd throttle")
	}
}

// During bootstrap the pass is strict: a throttled currency stops the pass early (break) so the
// failed attempt is counted and bootstrap retries terminate.
func TestUpdateHistoricalTickers_StopsOnThrottleExhaustionDuringBootstrap(t *testing.T) {
	config := common.Config{
		CoinName: "fakecoin",
	}
	d, _, tmp := setupRocksDB(t, &testBitcoinParser{
		BitcoinParser: bitcoinTestnetParser(),
	}, &config)
	defer closeAndDestroyRocksDB(t, d, tmp)

	if err := d.FiatRatesSetHistoricalBootstrapComplete(false); err != nil {
		t.Fatalf("FiatRatesSetHistoricalBootstrapComplete failed: %v", err)
	}
	originalVsCurrencies := vsCurrencies
	originalPlatformIds := platformIds
	originalPlatformIdsToTokens := platformIdsToTokens
	defer func() {
		vsCurrencies = originalVsCurrencies
		platformIds = originalPlatformIds
		platformIdsToTokens = originalPlatformIdsToTokens
	}()

	originalBackoff := coingeckoThrottleRetryBackoff
	coingeckoThrottleRetryBackoff = []time.Duration{0, 0, 0, 0}
	defer func() {
		coingeckoThrottleRetryBackoff = originalBackoff
	}()

	var usdRequests atomic.Int32
	var eurRequests atomic.Int32

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/simple/supported_vs_currencies":
			_, _ = w.Write([]byte(`["usd","eur"]`))
		case "/coins/ethereum/market_chart":
			switch r.URL.Query().Get("vs_currency") {
			case "usd":
				usdRequests.Add(1)
				http.Error(w, "exceeded the rate limit", http.StatusTooManyRequests)
			case "eur":
				eurRequests.Add(1)
				_, _ = w.Write([]byte(`{"prices":[[1654732800000,1234.5]]}`))
			default:
				http.Error(w, "unexpected vs_currency", http.StatusBadRequest)
			}
		default:
			http.Error(w, fmt.Sprintf("unexpected path %s", r.URL.Path), http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	cg := &Coingecko{
		coin:         "ethereum",
		bootstrapURL: mockServer.URL,
		tipURL:       mockServer.URL,
		httpClient:   mockServer.Client(),
		db:           d,
		plan:         coingeckoPlanFree,
	}

	err := cg.UpdateHistoricalTickers()
	if err == nil {
		t.Fatal("expected throttle exhaustion error")
	}
	if !isCoingeckoThrottleRetriesExhaustedError(err) {
		t.Fatalf("expected throttle exhaustion error, got %v", err)
	}

	wantUSDRequests := 1 + len(coingeckoThrottleRetryBackoff)
	if got := int(usdRequests.Load()); got != wantUSDRequests {
		t.Fatalf("unexpected usd request count: got %d, want %d", got, wantUSDRequests)
	}
	if got := int(eurRequests.Load()); got != 0 {
		t.Fatalf("expected eur request count 0 after bootstrap throttle exhaustion (break), got %d", got)
	}
}

// In steady state a throttled token must not abort the whole token pass: the loop continues with
// the remaining tokens (each gets its own bounded retries) and the throttle error is still
// returned so the unfinished tokens are retried next run.
func TestUpdateHistoricalTokenTickers_ContinuesPastThrottleInSteadyState(t *testing.T) {
	config := common.Config{
		CoinName: "fakecoin",
	}
	d, _, tmp := setupRocksDB(t, &testBitcoinParser{
		BitcoinParser: bitcoinTestnetParser(),
	}, &config)
	defer closeAndDestroyRocksDB(t, d, tmp)

	if err := d.FiatRatesSetHistoricalBootstrapComplete(true); err != nil {
		t.Fatalf("FiatRatesSetHistoricalBootstrapComplete failed: %v", err)
	}
	originalVsCurrencies := vsCurrencies
	originalPlatformIds := platformIds
	originalPlatformIdsToTokens := platformIdsToTokens
	defer func() {
		vsCurrencies = originalVsCurrencies
		platformIds = originalPlatformIds
		platformIdsToTokens = originalPlatformIdsToTokens
	}()

	originalBackoff := coingeckoThrottleRetryBackoff
	coingeckoThrottleRetryBackoff = []time.Duration{0, 0, 0, 0}
	defer func() {
		coingeckoThrottleRetryBackoff = originalBackoff
	}()

	var marketChartRequests atomic.Int32

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/coins/list":
			_, _ = w.Write([]byte(`[
				{"id":"token-a","symbol":"a","name":"A","platforms":{"ethereum":"0xa"}},
				{"id":"token-b","symbol":"b","name":"B","platforms":{"ethereum":"0xb"}}
			]`))
		case "/coins/token-a/market_chart", "/coins/token-b/market_chart":
			marketChartRequests.Add(1)
			http.Error(w, "exceeded the rate limit", http.StatusTooManyRequests)
		default:
			http.Error(w, fmt.Sprintf("unexpected path %s", r.URL.Path), http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	cg := &Coingecko{
		coin:               "ethereum",
		platformIdentifier: "ethereum",
		platformVsCurrency: "eth",
		bootstrapURL:       mockServer.URL,
		tipURL:             mockServer.URL,
		httpClient:         mockServer.Client(),
		db:                 d,
		plan:               coingeckoPlanFree,
	}

	err := cg.UpdateHistoricalTokenTickers()
	if err == nil {
		t.Fatal("expected throttle exhaustion error")
	}
	if !isCoingeckoThrottleRetriesExhaustedError(err) {
		t.Fatalf("expected throttle exhaustion error, got %v", err)
	}

	// Both tokens are throttled; continuing means each one is attempted with its own bounded retries.
	wantRequests := 2 * (1 + len(coingeckoThrottleRetryBackoff))
	if got := int(marketChartRequests.Load()); got != wantRequests {
		t.Fatalf("unexpected market_chart request count: got %d, want %d", got, wantRequests)
	}
}

func TestUpdateHistoricalTokenTickers_ReturnsInProgressError(t *testing.T) {
	cg := &Coingecko{
		updatingTokens: true,
	}
	err := cg.UpdateHistoricalTokenTickers()
	if err == nil {
		t.Fatal("expected non-nil in-progress error")
	}
	if !errors.Is(err, errCoingeckoHistoricalTokenUpdateInProgress) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetHighGranularityTickers_NotEnoughPricePoints(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// return only 1 price point
		_, _ = w.Write([]byte(`{"prices":[[1654732800000,1234.5]]}`))
	}))
	defer mockServer.Close()

	cg := &Coingecko{
		coin:       "ethereum",
		tipURL:     mockServer.URL,
		httpClient: mockServer.Client(),
		plan:       coingeckoPlanFree,
	}

	tickers, err := cg.HourlyTickers()
	if err == nil {
		t.Fatal("expected error for not enough price points")
	}
	if !strings.Contains(err.Error(), "not enough price points") {
		t.Fatalf("unexpected error message: %v", err)
	}
	if tickers != nil {
		t.Fatal("expected nil tickers")
	}
}
