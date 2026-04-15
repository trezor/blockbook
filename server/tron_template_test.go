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
				Payload:     json.RawMessage(`{"operation":"vote","totalFee":"3076500","energyUsageTotal":"100","energyFee":"250000","bandwidthUsage":"50","bandwidthFee":"345000","stakeAmount":"125000000","unstakeAmount":"88000000","claimedVoteReward":"6500000","votes":[{"address":"TA","count":"2"}]}`),
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
		if got.ClaimedVoteRewardValue == nil || got.ClaimedVoteRewardValue.DecimalString(6) != "6.5" {
			t.Fatalf("unexpected claimedVoteReward %+v", got.ClaimedVoteRewardValue)
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
				Payload:     json.RawMessage(`{"availableStakedBandwidth":400,"totalStakedBandwidth":700,"availableFreeBandwidth":200,"totalFreeBandwidth":300,"availableEnergy":1234,"totalEnergy":9000,"stakingInfo":{"stakedBalance":"7000000","stakedBalanceEnergy":"5000000","stakedBalanceBandwidth":"2000000","unstakingBatches":[{"amount":"1112757","expireTime":1777018452}],"totalVotingPower":"10","availableVotingPower":"7","votes":[{"address":"TA","voteCount":"2"}],"unclaimedReward":"42767","delegatedBalanceEnergy":"3210000","delegatedBalanceBandwidth":"654000"}}`),
			},
		}
		got := accountChainExtra(addr)
		if got == nil {
			t.Fatal("expected extra data")
		}
		if got.AvailableStakedBandwidth != 400 || got.TotalStakedBandwidth != 700 || got.AvailableFreeBandwidth != 200 || got.TotalFreeBandwidth != 300 {
			t.Fatalf("unexpected bandwidth values %+v", got)
		}
		if got.AvailableEnergy != 1234 || got.TotalEnergy != 9000 {
			t.Fatalf("unexpected energy values %+v", got)
		}
		if got.StakingInfoData == nil {
			t.Fatal("expected staking info data")
		}
		if got.StakingInfoData.StakedBalanceValue == nil || got.StakingInfoData.StakedBalanceValue.DecimalString(6) != "7" {
			t.Fatalf("unexpected staked balance %+v", got.StakingInfoData.StakedBalanceValue)
		}
		if got.StakingInfoData.StakedBalanceEnergyValue == nil || got.StakingInfoData.StakedBalanceEnergyValue.DecimalString(6) != "5" {
			t.Fatalf("unexpected staked energy balance %+v", got.StakingInfoData.StakedBalanceEnergyValue)
		}
		if got.StakingInfoData.StakedBalanceBandwidthValue == nil || got.StakingInfoData.StakedBalanceBandwidthValue.DecimalString(6) != "2" {
			t.Fatalf("unexpected staked bandwidth balance %+v", got.StakingInfoData.StakedBalanceBandwidthValue)
		}
		if len(got.StakingInfoData.UnstakingBatchesData) != 1 {
			t.Fatalf("unexpected unstaking batches %+v", got.StakingInfoData.UnstakingBatchesData)
		}
		if got.StakingInfoData.UnstakingBatchesData[0].AmountValue == nil || got.StakingInfoData.UnstakingBatchesData[0].AmountValue.DecimalString(6) != "1.112757" {
			t.Fatalf("unexpected unstaking batch amount %+v", got.StakingInfoData.UnstakingBatchesData[0].AmountValue)
		}
		if len(got.StakingInfoData.Votes) != 1 || got.StakingInfoData.Votes[0].Address != "TA" || got.StakingInfoData.Votes[0].VoteCount != "2" {
			t.Fatalf("unexpected votes %+v", got.StakingInfoData.Votes)
		}
		if got.StakingInfoData.UnclaimedRewardValue == nil || got.StakingInfoData.UnclaimedRewardValue.DecimalString(6) != "0.042767" {
			t.Fatalf("unexpected unclaimed reward %+v", got.StakingInfoData.UnclaimedRewardValue)
		}
		if got.StakingInfoData.DelegatedBalanceEnergyValue == nil || got.StakingInfoData.DelegatedBalanceEnergyValue.DecimalString(6) != "3.21" {
			t.Fatalf("unexpected delegated energy balance %+v", got.StakingInfoData.DelegatedBalanceEnergyValue)
		}
		if got.StakingInfoData.DelegatedBalanceBandwidthValue == nil || got.StakingInfoData.DelegatedBalanceBandwidthValue.DecimalString(6) != "0.654" {
			t.Fatalf("unexpected delegated bandwidth balance %+v", got.StakingInfoData.DelegatedBalanceBandwidthValue)
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
