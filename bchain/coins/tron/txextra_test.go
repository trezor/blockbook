//go:build unittest

package tron

import (
	"encoding/json"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"
)

func int64Ptr(v int64) *int64 {
	return &v
}

func resourceCodePtr(v tronResourceCode) *tronResourceCode {
	return &v
}

func TestTronBuildExtraData_VoteWitness(t *testing.T) {
	contract := tronTxContract{Type: "VoteWitnessContract"}
	contract.Parameter.Value.Votes = []tronTxVote{
		{
			VoteAddress: "41734c2f23ab41c52308d1206c4eb5fe8e124e6898",
			VoteCount:   int64Ptr(17),
		},
		{
			VoteAddress: "41da727d310b98700af4cec797e43991899668d6f3",
			VoteCount:   int64Ptr(3),
		},
	}

	txByID := &tronGetTransactionByIDResponse{}
	txByID.RawData.Contract = []tronTxContract{contract}
	txInfo := &tronGetTransactionInfoByIDResponse{}

	extra := tronBuildExtraData(txByID, txInfo)
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
		contract.Parameter.Value.FrozenBalance = int64Ptr(125000000)
		contract.Parameter.Value.Resource = resourceCodePtr(tronResourceEnergy)

		txByID := &tronGetTransactionByIDResponse{}
		txByID.RawData.Contract = []tronTxContract{contract}
		txInfo := &tronGetTransactionInfoByIDResponse{}

		extra := tronBuildExtraData(txByID, txInfo)
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
		contract.Parameter.Value.Balance = int64Ptr(42000000)
		contract.Parameter.Value.ReceiverAddress = "41da727d310b98700af4cec797e43991899668d6f3"
		contract.Parameter.Value.Resource = resourceCodePtr(tronResourceBandwidth)

		txByID := &tronGetTransactionByIDResponse{}
		txByID.RawData.Contract = []tronTxContract{contract}
		txInfo := &tronGetTransactionInfoByIDResponse{}

		extra := tronBuildExtraData(txByID, txInfo)
		require.Equal(t, "delegate", extra.Operation)
		require.Equal(t, "42000000", extra.DelegateAmount)
		require.Equal(t, ToTronAddressFromAddress(contract.Parameter.Value.ReceiverAddress), extra.DelegateTo)
		require.Equal(t, "bandwidth", extra.Resource)
	})

	t.Run("vote power resource", func(t *testing.T) {
		contract := tronTxContract{Type: "UnfreezeBalanceV2Contract"}
		contract.Parameter.Value.Resource = resourceCodePtr(tronResourceVotePower)

		txByID := &tronGetTransactionByIDResponse{}
		txByID.RawData.Contract = []tronTxContract{contract}
		txInfo := &tronGetTransactionInfoByIDResponse{}

		extra := tronBuildExtraData(txByID, txInfo)
		require.Equal(t, "votePower", extra.Resource)
	})
}

func TestTronBuildExtraData_AssetIssueID(t *testing.T) {
	contract := tronTxContract{Type: "TransferAssetContract"}
	txByID := &tronGetTransactionByIDResponse{}
	txByID.RawData.Contract = []tronTxContract{contract}
	txByID.RawData.FeeLimit = int64Ptr(999000000)

	txInfo := &tronGetTransactionInfoByIDResponse{
		AssetIssueID: "1000047",
	}

	extra := tronBuildExtraData(txByID, txInfo)
	require.Equal(t, "trc10Transfer", extra.Operation)
	require.Equal(t, "1000047", extra.AssetIssueID)
	require.Equal(t, "999000000", extra.FeeLimit)
}

func TestTronBuildRpcTransaction_ValueIsEthereumHexQuantity(t *testing.T) {
	tests := []struct {
		name     string
		contract tronTxContract
		want     int64
	}{
		{
			name:     "transfer amount",
			contract: tronTxContract{Type: "TransferContract"},
			want:     586000000,
		},
		{
			name:     "trigger smart contract call value",
			contract: tronTxContract{Type: "TriggerSmartContract"},
			want:     12345,
		},
		{
			name:     "freeze balance integer",
			contract: tronTxContract{Type: "FreezeBalanceContract"},
			want:     42000000,
		},
		{
			name:     "unfreeze balance v2",
			contract: tronTxContract{Type: "UnfreezeBalanceV2Contract"},
			want:     77000000,
		},
		{
			name:     "unfreeze balance v1 has no tx value",
			contract: tronTxContract{Type: "UnfreezeBalanceContract"},
			want:     0,
		},
		{
			name:     "delegate balance integer",
			contract: tronTxContract{Type: "DelegateResourceContract"},
			want:     88000000,
		},
	}

	tests[0].contract.Parameter.Value.Amount = int64Ptr(586000000)
	tests[1].contract.Parameter.Value.CallValue = int64Ptr(12345)
	tests[2].contract.Parameter.Value.FrozenBalance = int64Ptr(42000000)
	tests[3].contract.Parameter.Value.UnfreezeBalance = int64Ptr(77000000)
	tests[5].contract.Parameter.Value.Balance = int64Ptr(88000000)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			txByID := &tronGetTransactionByIDResponse{}
			txByID.RawData.Contract = []tronTxContract{tt.contract}
			txByID.TxID = "25b18a55f86afb10e7aca38d0073d04c80397c6636069193953fdefaea0b8369"
			txInfo := &tronGetTransactionInfoByIDResponse{
				BlockNumber: int64Ptr(1),
			}

			tx := tronBuildRpcTransaction(txByID, txInfo)
			value, err := hexutil.DecodeBig(tx.Value)

			require.NoError(t, err)
			require.Equal(t, tt.want, value.Int64())
		})
	}
}

func TestTronGetTransactionInfoByIDResponse_IgnoresCancelUnfreezeV2AmountShape(t *testing.T) {
	raw := []byte(`[
		{
			"id":"tx1",
			"fee":123,
			"cancel_unfreezeV2_amount":[]
		},
		{
			"id":"tx2",
			"fee":456,
			"cancel_unfreezeV2_amount":{"ENERGY":100}
		}
	]`)

	var resp []tronGetTransactionInfoByIDResponse
	err := json.Unmarshal(raw, &resp)
	require.NoError(t, err)
	require.Len(t, resp, 2)
	require.Equal(t, "tx1", resp[0].ID)
	require.Equal(t, int64(123), *resp[0].Fee)
	require.Equal(t, "tx2", resp[1].ID)
	require.Equal(t, int64(456), *resp[1].Fee)
}

func TestTronBuildRpcReceipt_UsesTopLevelResultOmittedAsSuccess(t *testing.T) {
	txInfo := &tronGetTransactionInfoByIDResponse{
		ID: "tx1",
	}
	receipt := tronBuildRpcReceipt(txInfo)
	require.NotNil(t, receipt)
	require.Equal(t, "0x1", receipt.Status)

	txInfo.Result = "FAILED"
	receipt = tronBuildRpcReceipt(txInfo)
	require.NotNil(t, receipt)
	require.Equal(t, "0x0", receipt.Status)
}

func TestTronBuildExtraData_ResultRequiresTransactionInfo(t *testing.T) {
	txByID := &tronGetTransactionByIDResponse{}
	txInfo := &tronGetTransactionInfoByIDResponse{}

	extra := tronBuildExtraData(txByID, txInfo)
	require.Equal(t, "", extra.Result)

	txInfo.Receipt.Result = "SUCCESS"
	extra = tronBuildExtraData(txByID, txInfo)
	require.Equal(t, "SUCCESS", extra.Result)
}

func TestTronBuildExtraData_BandwidthUsageDefaultsToZero(t *testing.T) {
	txByID := &tronGetTransactionByIDResponse{}
	txInfo := &tronGetTransactionInfoByIDResponse{}

	extra := tronBuildExtraData(txByID, txInfo)
	require.Equal(t, "0", extra.BandwidthUsage)

	txInfo.Receipt.NetUsage = int64Ptr(42)
	extra = tronBuildExtraData(txByID, txInfo)
	require.Equal(t, "42", extra.BandwidthUsage)
}

func TestTronTxMeta_GetCorrectTxMeta(t *testing.T) {
	txInfo := &tronGetTransactionInfoByIDResponse{
		BlockNumber:    int64Ptr(12345),
		BlockTimeStamp: int64Ptr(1700000000000),
	}
	blockTime, blockNumber, hasBlockNumber := tronTxMeta(txInfo)
	require.True(t, hasBlockNumber)
	require.Equal(t, uint64(12345), blockNumber)
	require.Equal(t, int64(1700000000), blockTime)
}
