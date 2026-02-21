//go:build unittest

package tron

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func int64Ptr(v int64) *int64 {
	return &v
}

func TestTronBuildExtraData_VoteWitness(t *testing.T) {
	contract := tronTxContract{Type: "VoteWitnessContract"}
	contract.Parameter.Value.Votes = []tronTxVote{
		{
			VoteAddress: "41734c2f23ab41c52308d1206c4eb5fe8e124e6898",
			VoteCount:   int64(17),
		},
		{
			VoteAddress: "41da727d310b98700af4cec797e43991899668d6f3",
			VoteCount:   int64(3),
		},
	}

	txByID := &tronGetTransactionByIDResponse{}
	txByID.RawData.Contract = []tronTxContract{contract}

	extra := tronBuildExtraData(txByID, nil)
	require.Equal(t, "VoteWitnessContract", extra.ContractType)
	require.Equal(t, "vote", extra.Operation)
	require.Len(t, extra.Votes, 2)
	require.Equal(t, ToTronAddressFromAddress(contract.Parameter.Value.Votes[0].VoteAddress), extra.Votes[0].Address)
	require.Equal(t, "17", extra.Votes[0].Count)
	require.Equal(t, ToTronAddressFromAddress(contract.Parameter.Value.Votes[1].VoteAddress), extra.Votes[1].Address)
	require.Equal(t, "3", extra.Votes[1].Count)
}

func TestTronBuildExtraData_StakeAndDelegateDetails(t *testing.T) {
	t.Run("stake amount", func(t *testing.T) {
		contract := tronTxContract{Type: "FreezeBalanceV2Contract"}
		contract.Parameter.Value.FrozenBalance = int64(125000000)
		contract.Parameter.Value.Resource = "ENERGY"

		txByID := &tronGetTransactionByIDResponse{}
		txByID.RawData.Contract = []tronTxContract{contract}

		extra := tronBuildExtraData(txByID, nil)
		require.Equal(t, "freeze", extra.Operation)
		require.Equal(t, "125000000", extra.StakeAmount)
		require.Equal(t, "energy", extra.Resource)
	})

	t.Run("unstake amount fallback from txInfo", func(t *testing.T) {
		contract := tronTxContract{Type: "UnfreezeBalanceContract"}
		txByID := &tronGetTransactionByIDResponse{}
		txByID.RawData.Contract = []tronTxContract{contract}

		txInfo := &tronGetTransactionInfoByIDResponse{
			UnfreezeAmount: int64Ptr(88000000),
		}

		extra := tronBuildExtraData(txByID, txInfo)
		require.Equal(t, "unfreeze", extra.Operation)
		require.Equal(t, "88000000", extra.UnstakeAmount)
	})

	t.Run("delegate amount and receiver", func(t *testing.T) {
		contract := tronTxContract{Type: "DelegateResourceContract"}
		contract.Parameter.Value.Balance = int64(42000000)
		contract.Parameter.Value.ReceiverAddress = "41da727d310b98700af4cec797e43991899668d6f3"
		contract.Parameter.Value.Resource = "BANDWIDTH"

		txByID := &tronGetTransactionByIDResponse{}
		txByID.RawData.Contract = []tronTxContract{contract}

		extra := tronBuildExtraData(txByID, nil)
		require.Equal(t, "delegate", extra.Operation)
		require.Equal(t, "42000000", extra.DelegateAmount)
		require.Equal(t, ToTronAddressFromAddress(contract.Parameter.Value.ReceiverAddress), extra.DelegateTo)
		require.Equal(t, "bandwidth", extra.Resource)
	})
}

func TestTronBuildExtraData_AssetIssueID(t *testing.T) {
	contract := tronTxContract{Type: "TransferAssetContract"}
	txByID := &tronGetTransactionByIDResponse{}
	txByID.RawData.Contract = []tronTxContract{contract}

	txInfo := &tronGetTransactionInfoByIDResponse{
		AssetIssueID: "1000047",
	}

	extra := tronBuildExtraData(txByID, txInfo)
	require.Equal(t, "trc10Transfer", extra.Operation)
	require.Equal(t, "1000047", extra.AssetIssueID)
}
