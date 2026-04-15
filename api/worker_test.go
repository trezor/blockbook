//go:build unittest

package api

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/fiat"
)

func TestGetSecondaryTicker_SkipsLookupWithoutSecondaryCurrency(t *testing.T) {
	w := &Worker{
		fiatRates: &fiat.FiatRates{Enabled: true},
	}
	originalGetter := getCurrentTicker
	defer func() {
		getCurrentTicker = originalGetter
	}()

	calls := 0
	getCurrentTicker = func(_ *fiat.FiatRates, _, _ string) *common.CurrencyRatesTicker {
		calls++
		return &common.CurrencyRatesTicker{}
	}

	ticker := w.getSecondaryTicker("")
	if ticker != nil {
		t.Fatalf("expected nil ticker when secondary currency is not requested, got %+v", ticker)
	}
	if calls != 0 {
		t.Fatalf("expected no ticker lookup call, got %d", calls)
	}
}

func TestGetSecondaryTicker_PerformsLookupWithSecondaryCurrency(t *testing.T) {
	w := &Worker{
		fiatRates: &fiat.FiatRates{Enabled: true},
	}
	originalGetter := getCurrentTicker
	defer func() {
		getCurrentTicker = originalGetter
	}()

	calls := 0
	expected := &common.CurrencyRatesTicker{Rates: map[string]float32{"usd": 1}}
	getCurrentTicker = func(_ *fiat.FiatRates, _, _ string) *common.CurrencyRatesTicker {
		calls++
		return expected
	}

	ticker := w.getSecondaryTicker("usd")
	if ticker != expected {
		t.Fatalf("unexpected ticker returned: got %+v, want %+v", ticker, expected)
	}
	if calls != 1 {
		t.Fatalf("expected one ticker lookup call, got %d", calls)
	}
}

func TestTronBalanceHistoryOverrides(t *testing.T) {
	tests := []struct {
		name              string
		payload           string
		fallbackAmount    string
		hasFallbackAmount bool
		wantOverride      bool
		wantDirection     tronBalanceHistoryDirection
		wantAmount        string
	}{
		{
			name:              "freeze uses stake amount",
			payload:           `{"operation":"freeze","stakeAmount":"42000000"}`,
			fallbackAmount:    "1",
			hasFallbackAmount: true,
			wantOverride:      true,
			wantDirection:     tronBalanceHistoryDirectionOutgoing,
			wantAmount:        "42000000",
		},
		{
			name:              "withdraw uses unstake amount",
			payload:           `{"operation":"withdraw","unstakeAmount":"77000000"}`,
			fallbackAmount:    "1",
			hasFallbackAmount: true,
			wantOverride:      true,
			wantDirection:     tronBalanceHistoryDirectionIncoming,
			wantAmount:        "77000000",
		},
		{
			name:              "withdraw falls back to tx value",
			payload:           `{"operation":"withdraw"}`,
			fallbackAmount:    "123",
			hasFallbackAmount: true,
			wantOverride:      true,
			wantDirection:     tronBalanceHistoryDirectionIncoming,
			wantAmount:        "123",
		},
		{
			name:              "vote reward amount uses claimed vote reward",
			payload:           `{"operation":"voteRewardAmount","claimedVoteReward":"6500000"}`,
			fallbackAmount:    "1",
			hasFallbackAmount: true,
			wantOverride:      true,
			wantDirection:     tronBalanceHistoryDirectionIncoming,
			wantAmount:        "6500000",
		},
		{
			name:              "vote reward amount falls back to tx value",
			payload:           `{"operation":"voteRewardAmount"}`,
			fallbackAmount:    "321",
			hasFallbackAmount: true,
			wantOverride:      true,
			wantDirection:     tronBalanceHistoryDirectionIncoming,
			wantAmount:        "321",
		},
		{
			name:              "freeze invalid amount falls back to tx value",
			payload:           `{"operation":"freeze","stakeAmount":"not-a-number"}`,
			fallbackAmount:    "999",
			hasFallbackAmount: true,
			wantOverride:      true,
			wantDirection:     tronBalanceHistoryDirectionOutgoing,
			wantAmount:        "999",
		},
		{
			name:          "unfreeze has explicit no-move override",
			payload:       `{"operation":"unfreeze","unstakeAmount":"77000000"}`,
			wantOverride:  true,
			wantDirection: tronBalanceHistoryDirectionNone,
			wantAmount:    "0",
		},
		{
			name:         "non-freeze operation has no override",
			payload:      `{"operation":"transfer","stakeAmount":"42000000"}`,
			wantOverride: false,
		},
		{
			name:         "invalid json has no override",
			payload:      `{`,
			wantOverride: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var fallback *big.Int
			if tt.hasFallbackAmount {
				var ok bool
				fallback, ok = new(big.Int).SetString(tt.fallbackAmount, 10)
				if !ok {
					t.Fatalf("invalid fallback amount in test: %q", tt.fallbackAmount)
				}
			}

			override, hasOverride := tronBalanceHistoryOverrideFromExtraData(json.RawMessage(tt.payload), fallback)
			if hasOverride != tt.wantOverride {
				t.Fatalf("override mismatch: got %v want %v", hasOverride, tt.wantOverride)
			}
			if !tt.wantOverride {
				return
			}
			if override.direction != tt.wantDirection {
				t.Fatalf("direction mismatch: got %v want %v", override.direction, tt.wantDirection)
			}
			if got := override.amount.String(); got != tt.wantAmount {
				t.Fatalf("amount mismatch: got %s want %s", got, tt.wantAmount)
			}
		})
	}
}
