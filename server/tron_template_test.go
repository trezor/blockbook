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
			ChainExtraData: &api.TxChainExtraData{
				PayloadType: "tron",
				Payload:     json.RawMessage(`{"operation":"vote","totalFee":"3076500","energyUsageTotal":"100","energyFee":"250000","bandwidthUsage":"50","bandwidthFee":"345000","stakeAmount":"125000000","unstakeAmount":"88000000","votes":[{"address":"TA","count":"2"}]}`),
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
		if got.StakeAmountValue == nil || got.StakeAmountValue.DecimalString(6) != "125" {
			t.Fatalf("unexpected stakeAmount %+v", got.StakeAmountValue)
		}
		if got.UnstakeAmountValue == nil || got.UnstakeAmountValue.DecimalString(6) != "88" {
			t.Fatalf("unexpected unstakeAmount %+v", got.UnstakeAmountValue)
		}
		if len(got.Votes) != 1 || got.Votes[0].Address != "TA" || got.Votes[0].Count != "2" {
			t.Fatalf("unexpected votes %+v", got.Votes)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		tx := &api.Tx{ChainExtraData: &api.TxChainExtraData{PayloadType: "tron", Payload: json.RawMessage("{")}}
		if got := chainExtra(tx); got != nil {
			t.Fatalf("expected nil for invalid json, got %+v", got)
		}
	})

	t.Run("empty object", func(t *testing.T) {
		tx := &api.Tx{ChainExtraData: &api.TxChainExtraData{PayloadType: "tron", Payload: json.RawMessage(`{}`)}}
		if got := chainExtra(tx); got == nil {
			t.Fatal("expected non-nil for valid empty extra")
		}
	})

	t.Run("only feeLimit", func(t *testing.T) {
		tx := &api.Tx{
			ChainExtraData: &api.TxChainExtraData{
				PayloadType: "tron",
				Payload:     json.RawMessage(`{"feeLimit":"5000000"}`),
			},
		}
		got := chainExtra(tx)
		if got == nil {
			t.Fatal("expected extra data")
		}
		if got.FeeLimit != "5000000" {
			t.Fatalf("unexpected feeLimit %q", got.FeeLimit)
		}
	})
}

func TestAccountChainExtra(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		addr := &api.Address{
			ChainExtraData: &api.AccountChainExtraData{
				PayloadType: "tron",
				Payload:     json.RawMessage(`{"availableBandwidth":600,"totalBandwidth":1000,"availableEnergy":1234,"totalEnergy":9000}`),
			},
		}
		got := accountChainExtra(addr)
		if got == nil {
			t.Fatal("expected extra data")
		}
		if got.AvailableBandwidth != 600 || got.TotalBandwidth != 1000 {
			t.Fatalf("unexpected bandwidth values %+v", got)
		}
		if got.AvailableEnergy != 1234 || got.TotalEnergy != 9000 {
			t.Fatalf("unexpected energy values %+v", got)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		addr := &api.Address{ChainExtraData: &api.AccountChainExtraData{PayloadType: "tron", Payload: json.RawMessage("{")}}
		if got := accountChainExtra(addr); got != nil {
			t.Fatalf("expected nil for invalid json, got %+v", got)
		}
	})

	t.Run("empty object", func(t *testing.T) {
		addr := &api.Address{ChainExtraData: &api.AccountChainExtraData{PayloadType: "tron", Payload: json.RawMessage(`{}`)}}
		if got := accountChainExtra(addr); got == nil {
			t.Fatal("expected non-nil for valid empty extra")
		}
	})
}
