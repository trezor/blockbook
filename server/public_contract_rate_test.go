//go:build unittest

package server

import (
	"testing"

	"github.com/trezor/blockbook/api"
	"github.com/trezor/blockbook/common"
)

func TestTemplateDataContractBaseRate(t *testing.T) {
	const (
		usdt     = "0xdAC17F958D2ee523a2206206994597C13D831ec7"
		dai      = "0x6B175474E89094C44Da98b954EedeAC495271d0F"
		midnight = int64(1758585600) // 2025-09-23 00:00:00 UTC
	)
	// other tests configure fiat rates globally
	recalculate := common.TickerRecalculateTokenRate
	common.TickerRecalculateTokenRate = false
	defer func() { common.TickerRecalculateTokenRate = recalculate }()
	var calls []int64
	lookup := func(_ *common.CurrencyRatesTicker, token string, ts int64) (float64, bool) {
		calls = append(calls, ts)
		if token == dai {
			return 0, false
		}
		return float64(ts), true
	}
	td := &TemplateData{TxTicker: &common.CurrencyRatesTicker{Rates: map[string]float32{"usd": 1}}}
	rate := func(contract string, blocktime int64) (float64, bool) {
		td.Tx = &api.Tx{Blocktime: blocktime}
		return td.contractBaseRate(contract, lookup)
	}

	// a day is (midnight-24h, midnight], matching the first stored daily row at or after the tx
	if r, found := rate(usdt, midnight-3600); !found || r != float64(midnight-3600) {
		t.Fatalf("first lookup = %v,%v, want %v,true", r, found, midnight-3600)
	}
	if r, _ := rate(usdt, midnight); r != float64(midnight-3600) {
		t.Errorf("same day not memoized: got %v", r)
	}
	if r, _ := rate(usdt, midnight+1); r != float64(midnight+1) {
		t.Errorf("next day reused previous day: got %v", r)
	}
	if _, found := rate(dai, midnight-60); found {
		t.Error("dai found, want not found")
	}
	if _, found := rate(dai, midnight-120); found {
		t.Error("memoized dai miss found")
	}
	if len(calls) != 3 {
		t.Errorf("lookups = %v, want 3 (usdt twice on different days, dai once)", calls)
	}

	// rates already in the tx ticker and a missing tx ticker never reach the lookup
	calls = nil
	td.TxTicker = &common.CurrencyRatesTicker{TokenRates: map[string]float32{usdt: 0.5}}
	if r, found := rate(usdt, midnight+7200); !found || r != 0.5 {
		t.Errorf("tx ticker rate = %v,%v, want 0.5,true", r, found)
	}
	td.TxTicker = nil
	if _, found := rate(usdt, midnight+7200); found {
		t.Error("nil tx ticker found a rate")
	}
	if len(calls) != 0 {
		t.Errorf("lookups = %v, want none", calls)
	}
}
