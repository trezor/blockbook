//go:build unittest

package fiat

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/trezor/blockbook/common"
)

func testCoinGeckoScopedAPIKeyEnvName(prefix string) string {
	return strings.ToUpper(strings.TrimSpace(prefix)) + coingeckoAPIKeyEnvSuffix
}

func TestResolveCoinGeckoPlan(t *testing.T) {
	tests := []struct {
		name      string
		plan      string
		url       string
		hasAPIKey bool
		want      string
	}{
		{
			name:      "explicit free overrides pro url and api key",
			plan:      "free",
			url:       coingeckoProAPIURL,
			hasAPIKey: true,
			want:      coingeckoPlanFree,
		},
		{
			name:      "explicit pro",
			plan:      "pro",
			url:       "",
			hasAPIKey: false,
			want:      coingeckoPlanPro,
		},
		{
			name:      "infer pro from pro url",
			plan:      "",
			url:       coingeckoProAPIURL,
			hasAPIKey: false,
			want:      coingeckoPlanPro,
		},
		{
			name:      "infer pro from pro url with trailing slash and uppercase",
			plan:      "",
			url:       "HTTPS://PRO-API.COINGECKO.COM/API/V3/",
			hasAPIKey: false,
			want:      coingeckoPlanPro,
		},
		{
			name:      "infer free from public url",
			plan:      "",
			url:       coingeckoFreeAPIURL,
			hasAPIKey: true,
			want:      coingeckoPlanFree,
		},
		{
			name:      "empty plan with api key stays backward compatible and defaults to pro",
			plan:      "",
			url:       "",
			hasAPIKey: true,
			want:      coingeckoPlanPro,
		},
		{
			name:      "empty plan without api key defaults to free",
			plan:      "",
			url:       "",
			hasAPIKey: false,
			want:      coingeckoPlanFree,
		},
		{
			name:      "unknown plan falls back to api key default",
			plan:      "enterprise",
			url:       "",
			hasAPIKey: true,
			want:      coingeckoPlanPro,
		},
		{
			name:      "unknown plan skips url inference and falls back to api key default",
			plan:      "enterprise",
			url:       coingeckoFreeAPIURL,
			hasAPIKey: true,
			want:      coingeckoPlanPro,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveCoinGeckoPlan(tt.plan, tt.url, tt.hasAPIKey)
			if got != tt.want {
				t.Fatalf("unexpected plan: got %q, want %q", got, tt.want)
			}
		})
	}
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

func TestResolveHistoricalDays_FreeAPIWithoutLastTickerUses365(t *testing.T) {
	cg := &Coingecko{
		plan: coingeckoPlanFree,
	}

	days, shouldRequest := cg.resolveHistoricalDays(nil)
	if !shouldRequest {
		t.Fatal("expected request to be required")
	}
	if days != "365" {
		t.Fatalf("unexpected days value: got %q, want %q", days, "365")
	}
}

func TestResolveHistoricalDays_ProAPIWithoutLastTickerUsesMax(t *testing.T) {
	cg := &Coingecko{
		plan: coingeckoPlanPro,
	}

	days, shouldRequest := cg.resolveHistoricalDays(nil)
	if !shouldRequest {
		t.Fatal("expected request to be required")
	}
	if days != "max" {
		t.Fatalf("unexpected days value: got %q, want %q", days, "max")
	}
}

func TestResolveHistoricalDays_FreeAPICapsLongLookbackTo365(t *testing.T) {
	cg := &Coingecko{
		plan: coingeckoPlanFree,
	}

	days, shouldRequest := cg.resolveHistoricalDays(&common.CurrencyRatesTicker{
		Timestamp: time.Now().AddDate(0, 0, -500),
	})
	if !shouldRequest {
		t.Fatal("expected request to be required")
	}
	if days != "365" {
		t.Fatalf("unexpected days value: got %q, want %q", days, "365")
	}
}

func TestResolveHistoricalDays_SkipsWhenSameDayTickerExists(t *testing.T) {
	cg := &Coingecko{
		plan: coingeckoPlanFree,
	}

	days, shouldRequest := cg.resolveHistoricalDays(&common.CurrencyRatesTicker{
		Timestamp: time.Now().Add(-10 * time.Hour),
	})
	if shouldRequest {
		t.Fatal("expected request to be skipped")
	}
	if days != "" {
		t.Fatalf("unexpected days value: got %q, want empty", days)
	}
}

func TestHistoricalRangeDaysLimit_DependsOnPlan(t *testing.T) {
	free := (&Coingecko{plan: coingeckoPlanFree}).historicalRangeDaysLimit()
	if free != coingeckoFreeHistoryDaysLimit {
		t.Fatalf("unexpected free limit: got %d, want %d", free, coingeckoFreeHistoryDaysLimit)
	}

	pro := (&Coingecko{plan: coingeckoPlanPro}).historicalRangeDaysLimit()
	if pro != 0 {
		t.Fatalf("unexpected pro limit: got %d, want %d", pro, 0)
	}
}

func TestIsHistoricalRangeLimitError(t *testing.T) {
	rangeErr := fmt.Errorf(`{"error":{"status":{"error_code":10012,"error_message":"Your request exceeds the allowed time range. Public API users are limited to querying historical data within the past 365 days."}}}`)
	if !isHistoricalRangeLimitError(rangeErr) {
		t.Fatal("expected range-limit error to be detected")
	}

	otherCoingeckoErr := fmt.Errorf(`{"error":{"status":{"error_code":10013,"error_message":"some other coingecko error"}}}`)
	if isHistoricalRangeLimitError(otherCoingeckoErr) {
		t.Fatal("expected non-10012 coingecko error not to be treated as range-limit")
	}

	textOnlyErr := fmt.Errorf("Your request exceeds the allowed time range within the past 365 days")
	if isHistoricalRangeLimitError(textOnlyErr) {
		t.Fatal("expected text-only error not to be treated as range-limit without error_code")
	}

	if isHistoricalRangeLimitError(fmt.Errorf("generic network error")) {
		t.Fatal("expected generic error not to be treated as range-limit")
	}
}
