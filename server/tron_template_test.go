//go:build unittest

package server

import (
	"encoding/json"
	"testing"

	"github.com/trezor/blockbook/api"
)

func TestChainExtra(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		tx := &api.Tx{
			ChainExtraData: &api.ChainExtraData{
				PayloadType: "tron",
				Payload:     json.RawMessage(`{"operation":"vote","energyUsageTotal":"100","bandwidthUsage":"50","votes":[{"address":"TA","count":"2"}]}`),
			},
		}
		got := chainExtra(tx)
		if got == nil {
			t.Fatal("expected extra data")
		}
		if got.Operation != "vote" {
			t.Fatalf("unexpected operation %q", got.Operation)
		}
		if got.EnergyUsageTotal != "100" {
			t.Fatalf("unexpected energyUsageTotal %q", got.EnergyUsageTotal)
		}
		if len(got.Votes) != 1 || got.Votes[0].Address != "TA" || got.Votes[0].Count != "2" {
			t.Fatalf("unexpected votes %+v", got.Votes)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		tx := &api.Tx{ChainExtraData: &api.ChainExtraData{PayloadType: "tron", Payload: json.RawMessage("{")}}
		if got := chainExtra(tx); got != nil {
			t.Fatalf("expected nil for invalid json, got %+v", got)
		}
	})

	t.Run("empty object", func(t *testing.T) {
		tx := &api.Tx{ChainExtraData: &api.ChainExtraData{PayloadType: "tron", Payload: json.RawMessage(`{}`)}}
		if got := chainExtra(tx); got != nil {
			t.Fatalf("expected nil for empty extra, got %+v", got)
		}
	})

	t.Run("wrong type", func(t *testing.T) {
		tx := &api.Tx{ChainExtraData: &api.ChainExtraData{PayloadType: "ethereum", Payload: json.RawMessage(`{"operation":"vote"}`)}}
		if got := chainExtra(tx); got != nil {
			t.Fatalf("expected nil for non-tron extra, got %+v", got)
		}
	})
}
