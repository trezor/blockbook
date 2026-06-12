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

func TestMakeReq_ThrottleRetriesWithoutCap(t *testing.T) {
	originalBackoff := coingeckoThrottleBackoff
	coingeckoThrottleBackoff = 0
	defer func() {
		coingeckoThrottleBackoff = originalBackoff
	}()

	// 429 more times than the old retry cap (4) to prove makeReq keeps retrying past it instead
	// of giving up, then succeeds once the provider stops throttling.
	throttleHits := 9
	var requests atomic.Int32
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if int(requests.Add(1)) <= throttleHits {
			http.Error(w, "exceeded the rate limit", http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer mockServer.Close()

	cg := &Coingecko{
		httpClient: mockServer.Client(),
	}
	resp, err := cg.makeReq(mockServer.URL, "market_chart", coingeckoPlanFree, priorityHigh)
	if err != nil {
		t.Fatalf("makeReq unexpectedly failed: %v", err)
	}
	if string(resp) != `{"ok":true}` {
		t.Fatalf("unexpected response body: %s", string(resp))
	}
	if got, want := int(requests.Load()), throttleHits+1; got != want {
		t.Fatalf("unexpected number of requests: got %d, want %d", got, want)
	}
}

func TestMakeReq_ThrottleRetriesEventuallySuccess(t *testing.T) {
	originalBackoff := coingeckoThrottleBackoff
	coingeckoThrottleBackoff = 0
	defer func() {
		coingeckoThrottleBackoff = originalBackoff
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
	resp, err := cg.makeReq(mockServer.URL, "market_chart", coingeckoPlanFree, priorityHigh)
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

func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  time.Duration
	}{
		{"seconds", "5", 5 * time.Second},
		{"trimmed seconds", "  10 ", 10 * time.Second},
		{"zero", "0", 0},
		{"negative", "-3", 0},
		{"empty", "", 0},
		{"garbage", "soon", 0},
		{"http-date unsupported", "Wed, 10 Jun 2026 11:34:33 GMT", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseRetryAfter(tt.value); got != tt.want {
				t.Fatalf("parseRetryAfter(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestRetryAfterFromError(t *testing.T) {
	if got := retryAfterFromError(&coingeckoHTTPError{status: http.StatusTooManyRequests, retryAfter: 7 * time.Second}); got != 7*time.Second {
		t.Fatalf("retryAfterFromError direct = %v, want 7s", got)
	}
	// errors.As must unwrap wrapped errors to reach the underlying Retry-After.
	wrapped := fmt.Errorf("wrapped: %w", &coingeckoHTTPError{status: http.StatusTooManyRequests, retryAfter: 7 * time.Second})
	if got := retryAfterFromError(wrapped); got != 7*time.Second {
		t.Fatalf("retryAfterFromError wrapped = %v, want 7s", got)
	}
	if got := retryAfterFromError(errors.New("boom")); got != 0 {
		t.Fatalf("retryAfterFromError non-http = %v, want 0", got)
	}
}

func TestThrottleBackoffDelay(t *testing.T) {
	originalBackoff := coingeckoThrottleBackoff
	coingeckoThrottleBackoff = 10 * time.Second
	defer func() { coingeckoThrottleBackoff = originalBackoff }()

	within := func(t *testing.T, got, base time.Duration) {
		t.Helper()
		upper := base + base/5 // base + up to ~20% jitter
		if got < base || got > upper {
			t.Fatalf("delay %v out of expected [%v, %v]", got, base, upper)
		}
	}

	noRetryAfter := &coingeckoHTTPError{status: http.StatusTooManyRequests}
	// The fixed backoff is used only when there is no Retry-After hint.
	within(t, throttleBackoffDelay(noRetryAfter), 10*time.Second)
	// A Retry-After hint takes priority over the fixed backoff, whether it is longer...
	within(t, throttleBackoffDelay(&coingeckoHTTPError{status: http.StatusTooManyRequests, retryAfter: 30 * time.Second}), 30*time.Second)
	// ...or shorter than the fixed backoff.
	within(t, throttleBackoffDelay(&coingeckoHTTPError{status: http.StatusTooManyRequests, retryAfter: 5 * time.Second}), 5*time.Second)

	// A zeroed backoff with no Retry-After stays instant (keeps the throttle tests fast).
	coingeckoThrottleBackoff = 0
	if got := throttleBackoffDelay(noRetryAfter); got != 0 {
		t.Fatalf("zeroed backoff delay = %v, want 0", got)
	}
}

func TestDoReq_ParsesRetryAfter(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header()["retry-after"] = []string{"5"}
		http.Error(w, "exceeded the rate limit", http.StatusTooManyRequests)
	}))
	defer mockServer.Close()

	req, err := http.NewRequest("GET", mockServer.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest failed: %v", err)
	}
	_, err = doReq(req, mockServer.Client())
	var httpErr *coingeckoHTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected coingeckoHTTPError, got %v", err)
	}
	if httpErr.status != http.StatusTooManyRequests {
		t.Fatalf("unexpected status: %d", httpErr.status)
	}
	if httpErr.retryAfter != 5*time.Second {
		t.Fatalf("unexpected retryAfter: %v, want 5s", httpErr.retryAfter)
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
	// Cloudflare actually returns the 1015 code inside a full HTML page, so the
	// substring must be detected even when it is not the entire error string.
	cloudflareBody := "<!DOCTYPE html><html><body>The owner of this website has banned you temporarily. error code: 1015</body></html>"
	if !isCoingeckoThrottleError(&coingeckoHTTPError{status: http.StatusForbidden, body: cloudflareBody}) {
		t.Fatal("expected Cloudflare 1015 HTML page to be detected as throttle")
	}
	if isCoingeckoThrottleError(nil) {
		t.Fatal("nil error must not be a throttle error")
	}
}

func TestMakeReq_Detects429ByStatus(t *testing.T) {
	originalBackoff := coingeckoThrottleBackoff
	coingeckoThrottleBackoff = 0
	defer func() {
		coingeckoThrottleBackoff = originalBackoff
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
	resp, err := cg.makeReq(mockServer.URL, "market_chart", coingeckoPlanFree, priorityHigh)
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
		if _, err := cg.makeReq(mockServer.URL, "simple/price", coingeckoPlanFree, priorityHigh); err != nil {
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

// The bootstrap CDN has no CoinGecko rate limit, so requests to it must use the light
// bootstrap spacing instead of the (potentially multi-second) plan pacing. Otherwise the
// free-plan 6s spacing would stretch the initial bootstrap from minutes to hours.
func TestMakeReq_BootstrapCDNBypassesPlanPacing(t *testing.T) {
	var requests atomic.Int32
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer mockServer.Close()

	cg := &Coingecko{
		httpClient:               mockServer.Client(),
		minHttpRequestInterval:   10 * time.Second, // plan pacing, must NOT apply to the CDN
		bootstrapRequestInterval: 0,
		bootstrapURL:             mockServer.URL,
	}
	start := time.Now()
	for i := 0; i < 3; i++ {
		if _, err := cg.makeReq(mockServer.URL+"/coins/list", "coins/list", coingeckoPlanFree, priorityLow); err != nil {
			t.Fatalf("makeReq failed on call %d: %v", i, err)
		}
	}
	elapsed := time.Since(start)
	// With plan pacing applied this would take >= 20s; the CDN path must be far faster.
	if elapsed > time.Second {
		t.Fatalf("bootstrap CDN requests were paced at the plan interval: 3 requests took %v", elapsed)
	}
	if got := int(requests.Load()); got != 3 {
		t.Fatalf("unexpected number of requests: got %d, want %d", got, 3)
	}
}

// A configured bootstrap URL must not relax pacing for the rate-limited CoinGecko API
// hosts: only requests whose URL actually targets the CDN get the light spacing.
func TestMakeReq_NonBootstrapURLStillPacedWhenBootstrapConfigured(t *testing.T) {
	interval := 60 * time.Millisecond
	var requests atomic.Int32
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer mockServer.Close()

	cg := &Coingecko{
		httpClient:               mockServer.Client(),
		minHttpRequestInterval:   interval,
		bootstrapRequestInterval: 0,
		bootstrapURL:             "https://cdn.trezor.io/dynamic/coingecko/api/v3", // different host than mockServer
	}
	start := time.Now()
	for i := 0; i < 3; i++ {
		if _, err := cg.makeReq(mockServer.URL, "simple/price", coingeckoPlanFree, priorityHigh); err != nil {
			t.Fatalf("makeReq failed on call %d: %v", i, err)
		}
	}
	elapsed := time.Since(start)
	if minElapsed := 2 * interval; elapsed < minElapsed {
		t.Fatalf("expected pacing to take at least %v across 3 requests, took %v", minElapsed, elapsed)
	}
	if got := int(requests.Load()); got != 3 {
		t.Fatalf("unexpected number of requests: got %d, want %d", got, 3)
	}
}

func TestMarkThrottled_ExtendsNeverShortens(t *testing.T) {
	cg := &Coingecko{}
	cg.markThrottled(time.Minute)
	first := cg.throttledUntil
	if first.IsZero() {
		t.Fatal("markThrottled did not open the throttle window")
	}
	// a shorter delay must not shorten the already open window
	cg.markThrottled(time.Second)
	if !cg.throttledUntil.Equal(first) {
		t.Fatalf("shorter delay moved the window: %v -> %v", first, cg.throttledUntil)
	}
	// a longer delay extends it
	cg.markThrottled(2 * time.Minute)
	if !cg.throttledUntil.After(first) {
		t.Fatalf("longer delay did not extend the window past %v: %v", first, cg.throttledUntil)
	}
}

func TestWaitForRequestSlot_HighPriorityWaitsOutWindowButIsNotParked(t *testing.T) {
	cg := &Coingecko{
		throttledUntil:       time.Now().Add(100 * time.Millisecond),
		highPriorityInFlight: 1, // a concurrent high-priority request must not park another one
	}
	until := cg.throttledUntil
	done := make(chan time.Time, 1)
	go func() {
		cg.waitForRequestSlot(priorityHigh, cg.minHttpRequestInterval)
		done <- time.Now()
	}()
	select {
	case returnedAt := <-done:
		if returnedAt.Before(until) {
			t.Fatalf("high priority request fired at %v, before the throttle window cleared at %v", returnedAt, until)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("high priority request was parked; it must only wait out the throttle window")
	}
}

func TestWaitForRequestSlot_LowPriorityParksUntilHighPriorityCompletes(t *testing.T) {
	originalPoll := coingeckoLowPriorityPollInterval
	coingeckoLowPriorityPollInterval = time.Millisecond
	defer func() { coingeckoLowPriorityPollInterval = originalPoll }()

	cg := &Coingecko{
		throttledUntil:       time.Now().Add(10 * time.Second),
		highPriorityInFlight: 1,
	}
	done := make(chan struct{})
	go func() {
		cg.waitForRequestSlot(priorityLow, cg.minHttpRequestInterval)
		close(done)
	}()
	// while throttled with a high-priority request in flight, low priority must stay parked
	time.Sleep(20 * time.Millisecond)
	select {
	case <-done:
		t.Fatal("low priority request fired while throttled with a high priority request in flight")
	default:
	}
	// once the high-priority request completes and the window clears, low priority proceeds
	cg.reqMu.Lock()
	cg.highPriorityInFlight = 0
	cg.throttledUntil = time.Now()
	cg.reqMu.Unlock()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("low priority request stayed parked after the gate cleared")
	}
}

func TestWaitForRequestSlot_RechecksWindowExtendedDuringSleep(t *testing.T) {
	cg := &Coingecko{}
	cg.markThrottled(200 * time.Millisecond)
	done := make(chan time.Time, 1)
	go func() {
		cg.waitForRequestSlot(priorityHigh, cg.minHttpRequestInterval)
		done <- time.Now()
	}()
	// wait until the goroutine reserved its slot inside the first window
	for {
		cg.reqMu.Lock()
		reserved := !cg.lastRequestAt.IsZero()
		cg.reqMu.Unlock()
		if reserved {
			break
		}
		time.Sleep(time.Millisecond)
	}
	// extend the window while the goroutine sleeps on its already-reserved slot
	cg.markThrottled(500 * time.Millisecond)
	cg.reqMu.Lock()
	extendedUntil := cg.throttledUntil
	cg.reqMu.Unlock()
	returnedAt := <-done
	if returnedAt.Before(extendedUntil) {
		t.Fatalf("request fired at %v, inside the extended throttle window ending %v", returnedAt, extendedUntil)
	}
}

// A 429 received by one request opens a throttle window that a second, unrelated request
// waits out as well.
func TestMakeReq_SharedThrottleWindow(t *testing.T) {
	originalBackoff := coingeckoThrottleBackoff
	coingeckoThrottleBackoff = 100 * time.Millisecond
	defer func() { coingeckoThrottleBackoff = originalBackoff }()

	var requests atomic.Int32
	var windowUntil atomic.Int64 // unix nanos of the shared window once opened
	var violations atomic.Int32
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if until := windowUntil.Load(); until != 0 && time.Now().UnixNano() < until {
			violations.Add(1)
		}
		if requests.Add(1) == 1 {
			http.Error(w, "exceeded the rate limit", http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer mockServer.Close()

	cg := &Coingecko{httpClient: mockServer.Client()}
	firstDone := make(chan error, 1)
	go func() {
		_, err := cg.makeReq(mockServer.URL, "market_chart", coingeckoPlanFree, priorityLow)
		firstDone <- err
	}()
	// wait until the first request's 429 opened the shared window
	for {
		cg.reqMu.Lock()
		until := cg.throttledUntil
		cg.reqMu.Unlock()
		if !until.IsZero() {
			windowUntil.Store(until.UnixNano())
			break
		}
		time.Sleep(time.Millisecond)
	}
	// a second request on another endpoint must wait out the shared window too
	if _, err := cg.makeReq(mockServer.URL, "simple/price", coingeckoPlanFree, priorityHigh); err != nil {
		t.Fatalf("second makeReq failed: %v", err)
	}
	if err := <-firstDone; err != nil {
		t.Fatalf("first makeReq failed: %v", err)
	}
	if violations.Load() != 0 {
		t.Fatal("a request reached the server inside the shared throttle window")
	}
}

// While a high-priority request keeps retrying through a 429 storm, a concurrent
// low-priority request stays parked and reaches the server only after the high-priority
// request succeeded.
func TestMakeReq_LowPriorityYieldsToHighDuringThrottle(t *testing.T) {
	originalBackoff := coingeckoThrottleBackoff
	originalPoll := coingeckoLowPriorityPollInterval
	coingeckoThrottleBackoff = 60 * time.Millisecond
	coingeckoLowPriorityPollInterval = time.Millisecond
	defer func() {
		coingeckoThrottleBackoff = originalBackoff
		coingeckoLowPriorityPollInterval = originalPoll
	}()

	var highThrottleHits atomic.Int32
	var highSucceededAt atomic.Int64
	var lowArrivedAt atomic.Int64
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/high":
			if highThrottleHits.Add(1) <= 2 {
				http.Error(w, "exceeded the rate limit", http.StatusTooManyRequests)
				return
			}
			highSucceededAt.Store(time.Now().UnixNano())
		case "/low":
			lowArrivedAt.Store(time.Now().UnixNano())
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer mockServer.Close()

	cg := &Coingecko{
		httpClient: mockServer.Client(),
		// spacing larger than the loopback round trip keeps an unparked low-priority
		// request from firing in the instant between a window expiring and the retrying
		// high-priority request extending it
		minHttpRequestInterval: 20 * time.Millisecond,
	}
	highDone := make(chan error, 1)
	go func() {
		_, err := cg.makeReq(mockServer.URL+"/high", "market_chart", coingeckoPlanFree, priorityHigh)
		highDone <- err
	}()
	// wait until the high-priority request opened the window (it stays in flight while retrying)
	for {
		cg.reqMu.Lock()
		opened := !cg.throttledUntil.IsZero()
		cg.reqMu.Unlock()
		if opened {
			break
		}
		time.Sleep(time.Millisecond)
	}
	lowDone := make(chan error, 1)
	go func() {
		_, err := cg.makeReq(mockServer.URL+"/low", "simple/price", coingeckoPlanFree, priorityLow)
		lowDone <- err
	}()
	if err := <-highDone; err != nil {
		t.Fatalf("high priority makeReq failed: %v", err)
	}
	if err := <-lowDone; err != nil {
		t.Fatalf("low priority makeReq failed: %v", err)
	}
	high, low := highSucceededAt.Load(), lowArrivedAt.Load()
	if high == 0 || low == 0 {
		t.Fatal("expected both endpoints to be hit")
	}
	if low < high {
		t.Fatalf("low priority request reached the server %v before the high priority request succeeded", time.Duration(high-low))
	}
}

// Throttled requests are retried without an attempt cap, so a provider-wide 429 no longer ends
// the pass: makeReq keeps backing off until the request succeeds, then the pass proceeds through
// the remaining currencies. Every currency is eventually fetched and stored.
func TestUpdateHistoricalTickers_RetriesThrottleUntilSuccess(t *testing.T) {
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
	originalBackoff := coingeckoThrottleBackoff
	coingeckoThrottleBackoff = 0
	defer func() {
		coingeckoThrottleBackoff = originalBackoff
	}()

	// Throttle usd more times than the old retry cap (4) to prove the attempt cap is gone.
	throttleHits := 7
	var usdRequests atomic.Int32
	var eurRequests atomic.Int32

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/simple/supported_vs_currencies":
			_, _ = w.Write([]byte(`["usd","eur"]`))
		case "/coins/ethereum/market_chart":
			switch r.URL.Query().Get("vs_currency") {
			case "usd":
				if int(usdRequests.Add(1)) <= throttleHits {
					http.Error(w, "exceeded the rate limit", http.StatusTooManyRequests)
					return
				}
				_, _ = w.Write([]byte(`{"prices":[[1654732800000,1111.1]]}`))
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

	if err := cg.UpdateHistoricalTickers(); err != nil {
		t.Fatalf("UpdateHistoricalTickers returned error: %v", err)
	}

	// usd is retried past the old attempt cap until it finally succeeds.
	if got, want := int(usdRequests.Load()), throttleHits+1; got != want {
		t.Fatalf("unexpected usd request count: got %d, want %d", got, want)
	}
	// once usd succeeds the pass continues to eur (no break on throttle anymore).
	if got := int(eurRequests.Load()); got != 1 {
		t.Fatalf("unexpected eur request count: got %d, want 1", got)
	}
	for _, cur := range []string{"usd", "eur"} {
		ticker, err := d.FiatRatesFindLastTicker(cur, "")
		if err != nil {
			t.Fatalf("FiatRatesFindLastTicker %s failed: %v", cur, err)
		}
		if ticker == nil {
			t.Fatalf("expected %s ticker to be stored after retry-until-success", cur)
		}
	}
}

// Even during bootstrap, throttled requests are retried without an attempt cap: the pass waits
// out the provider-wide 429 instead of failing the bootstrap cycle. Once the requests succeed,
// every currency is fetched and the bootstrap pass completes without error.
func TestUpdateHistoricalTickers_RetriesThrottleUntilSuccessDuringBootstrap(t *testing.T) {
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
	originalBackoff := coingeckoThrottleBackoff
	coingeckoThrottleBackoff = 0
	defer func() {
		coingeckoThrottleBackoff = originalBackoff
	}()

	// Throttle usd more times than the old retry cap (4) to prove the attempt cap is gone.
	throttleHits := 7
	var usdRequests atomic.Int32
	var eurRequests atomic.Int32

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/simple/supported_vs_currencies":
			_, _ = w.Write([]byte(`["usd","eur"]`))
		case "/coins/ethereum/market_chart":
			switch r.URL.Query().Get("vs_currency") {
			case "usd":
				if int(usdRequests.Add(1)) <= throttleHits {
					http.Error(w, "exceeded the rate limit", http.StatusTooManyRequests)
					return
				}
				_, _ = w.Write([]byte(`{"prices":[[1654732800000,1111.1]]}`))
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

	if err := cg.UpdateHistoricalTickers(); err != nil {
		t.Fatalf("UpdateHistoricalTickers returned error during bootstrap: %v", err)
	}

	// usd is retried past the old attempt cap until it finally succeeds.
	if got, want := int(usdRequests.Load()), throttleHits+1; got != want {
		t.Fatalf("unexpected usd request count: got %d, want %d", got, want)
	}
	// once usd succeeds the bootstrap pass continues to eur (no break on throttle anymore).
	if got := int(eurRequests.Load()); got != 1 {
		t.Fatalf("unexpected eur request count: got %d, want 1", got)
	}
	for _, cur := range []string{"usd", "eur"} {
		ticker, err := d.FiatRatesFindLastTicker(cur, "")
		if err != nil {
			t.Fatalf("FiatRatesFindLastTicker %s failed: %v", cur, err)
		}
		if ticker == nil {
			t.Fatalf("expected %s ticker to be stored after retry-until-success", cur)
		}
	}
}

// Throttled requests are retried without an attempt cap, so a provider-wide 429 no longer stops
// the token pass after the first throttled token: makeReq waits out the 429 until the request
// succeeds, and the pass then continues through the remaining tokens.
func TestUpdateHistoricalTokenTickers_RetriesThrottleUntilSuccess(t *testing.T) {
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
	originalBackoff := coingeckoThrottleBackoff
	coingeckoThrottleBackoff = 0
	defer func() {
		coingeckoThrottleBackoff = originalBackoff
	}()

	// Throttle the token requests more times than the old retry cap (4) to prove the cap is gone.
	throttleHits := 7
	var marketChartRequests atomic.Int32

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/coins/list":
			_, _ = w.Write([]byte(`[
				{"id":"token-a","symbol":"a","name":"A","platforms":{"ethereum":"0xa"}},
				{"id":"token-b","symbol":"b","name":"B","platforms":{"ethereum":"0xb"}}
			]`))
		case "/coins/token-a/market_chart", "/coins/token-b/market_chart":
			if int(marketChartRequests.Add(1)) <= throttleHits {
				http.Error(w, "exceeded the rate limit", http.StatusTooManyRequests)
				return
			}
			_, _ = w.Write([]byte(`{"prices":[[1654732800000,1.5]]}`))
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

	if err := cg.UpdateHistoricalTokenTickers(); err != nil {
		t.Fatalf("UpdateHistoricalTokenTickers returned error: %v", err)
	}

	// The first token absorbs all the throttles (retried past the old cap until it succeeds); the
	// remaining token then succeeds on its first request. No token is dropped on a 429.
	if got, want := int(marketChartRequests.Load()), throttleHits+2; got != want {
		t.Fatalf("unexpected market_chart request count: got %d, want %d", got, want)
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
