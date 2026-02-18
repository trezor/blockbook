//go:build unittest

package fiat

import (
	"fmt"
	"testing"
	"time"

	"github.com/trezor/blockbook/common"
)

func TestResolveHistoricalDays_FreeAPIWithoutLastTickerUses365(t *testing.T) {
	cg := &Coingecko{
		url: "https://api.coingecko.com/api/v3",
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
		url:  "https://pro-api.coingecko.com/api/v3",
		plan: "pro",
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
		url: "https://api.coingecko.com/api/v3",
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
		url: "https://api.coingecko.com/api/v3",
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

func TestIsHistoricalRangeLimitError(t *testing.T) {
	rangeErr := fmt.Errorf(`{"error":{"status":{"error_code":10012,"error_message":"Your request exceeds the allowed time range. Public API users are limited to querying historical data within the past 365 days."}}}`)
	if !isHistoricalRangeLimitError(rangeErr) {
		t.Fatal("expected range-limit error to be detected")
	}

	if isHistoricalRangeLimitError(fmt.Errorf("generic network error")) {
		t.Fatal("expected generic error not to be treated as range-limit")
	}
}
