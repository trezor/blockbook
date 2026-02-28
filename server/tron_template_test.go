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
				Payload:     json.RawMessage(`{"operation":"vote","totalFee":"3076500","energyUsageTotal":"100","energyFee":"250000","bandwidthUsage":"50","bandwidthFee":"345000","votes":[{"address":"TA","count":"2"}]}`),
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
		if got.TotalFeeAmount == nil || got.TotalFeeAmount.DecimalString(6) != "3.0765" {
			t.Fatalf("unexpected totalFee %+v", got.TotalFeeAmount)
		}
		if got.EnergyFeeAmount == nil || got.EnergyFeeAmount.DecimalString(6) != "0.25" {
			t.Fatalf("unexpected energyFee %+v", got.EnergyFeeAmount)
		}
		if got.BandwidthFeeAmount == nil || got.BandwidthFeeAmount.DecimalString(6) != "0.345" {
			t.Fatalf("unexpected bandwidthFee %+v", got.BandwidthFeeAmount)
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

	t.Run("invalid fee amount", func(t *testing.T) {
		tx := &api.Tx{
			ChainExtraData: &api.ChainExtraData{
				PayloadType: "tron",
				Payload:     json.RawMessage(`{"operation":"vote","totalFee":"x","energyFee":"x","bandwidthFee":"345000"}`),
			},
		}
		got := chainExtra(tx)
		if got == nil {
			t.Fatal("expected extra data")
		}
		if got.EnergyFeeAmount != nil {
			t.Fatalf("expected nil energyFeeAmount, got %+v", got.EnergyFeeAmount)
		}
		if got.TotalFeeAmount != nil {
			t.Fatalf("expected nil totalFeeAmount, got %+v", got.TotalFeeAmount)
		}
		if got.BandwidthFeeAmount == nil || got.BandwidthFeeAmount.DecimalString(6) != "0.345" {
			t.Fatalf("unexpected bandwidthFeeAmount %+v", got.BandwidthFeeAmount)
		}
	})
}
