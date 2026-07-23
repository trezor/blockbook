//go:build unittest

package tron

import (
	"encoding/json"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"
	"github.com/trezor/blockbook/bchain"
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

func TestTronBuildExtraData_AccountCreateOperation(t *testing.T) {
	contract := tronTxContract{Type: "AccountCreateContract"}
	txByID := &tronGetTransactionByIDResponse{}
	txByID.RawData.Contract = []tronTxContract{contract}
	txInfo := &tronGetTransactionInfoByIDResponse{}

	extra := tronBuildExtraData(txByID, txInfo)
	require.Equal(t, "AccountCreateContract", extra.ContractType)
	require.Equal(t, "activateAccount", extra.Operation)
}

func TestTronBuildExtraData_Note(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{
			name: "plain hex memo",
			data: "74657374",
			want: "test",
		},
		{
			name: "prefixed hex memo",
			data: "0x48656c6c6f2054524f4e",
			want: "Hello TRON",
		},
		{
			name: "empty memo",
			data: "",
			want: "",
		},
		{
			name: "invalid hex memo",
			data: "not-hex",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			txByID := &tronGetTransactionByIDResponse{}
			txByID.RawData.Data = tt.data
			txInfo := &tronGetTransactionInfoByIDResponse{}

			extra := tronBuildExtraData(txByID, txInfo)
			require.Equal(t, tt.want, extra.Note)
		})
	}
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

	t.Run("unstake amount uses contract unfreeze balance for stake 2.0", func(t *testing.T) {
		contract := tronTxContract{Type: "UnfreezeBalanceV2Contract"}
		contract.Parameter.Value.UnfreezeBalance = int64Ptr(99000000)
		txByID := &tronGetTransactionByIDResponse{}
		txByID.RawData.Contract = []tronTxContract{contract}

		txInfo := &tronGetTransactionInfoByIDResponse{}

		extra := tronBuildExtraData(txByID, txInfo)
		require.Equal(t, "unfreeze", extra.Operation)
		require.Equal(t, "99000000", extra.UnstakeAmount)
	})

	t.Run("unstake amount uses txInfo unfreeze amount for stake 1.0", func(t *testing.T) {
		contract := tronTxContract{Type: "UnfreezeBalanceContract"}
		contract.Parameter.Value.UnfreezeBalance = int64Ptr(11111111)
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

	t.Run("withdraw balance contract uses vote reward amount", func(t *testing.T) {
		contract := tronTxContract{Type: "WithdrawBalanceContract"}
		txByID := &tronGetTransactionByIDResponse{}
		txByID.RawData.Contract = []tronTxContract{contract}
		txInfo := &tronGetTransactionInfoByIDResponse{
			WithdrawAmount: int64Ptr(6500000),
		}

		extra := tronBuildExtraData(txByID, txInfo)
		require.Equal(t, "voteRewardAmount", extra.Operation)
		require.Equal(t, "6500000", extra.ClaimedVoteReward)
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
			name:     "trc10 transfer has no trx tx value",
			contract: tronTxContract{Type: "TransferAssetContract"},
			want:     0,
		},
		{
			name:     "delegate resource has no trx tx value",
			contract: tronTxContract{Type: "DelegateResourceContract"},
			want:     0,
		},
		{
			name:     "undelegate resource has no trx tx value",
			contract: tronTxContract{Type: "UnDelegateResourceContract"},
			want:     0,
		},
	}

	tests[0].contract.Parameter.Value.Amount = int64Ptr(586000000)
	tests[1].contract.Parameter.Value.CallValue = int64Ptr(12345)
	tests[2].contract.Parameter.Value.FrozenBalance = int64Ptr(42000000)
	tests[3].contract.Parameter.Value.UnfreezeBalance = int64Ptr(77000000)
	tests[5].contract.Parameter.Value.Balance = int64Ptr(88000000)
	tests[6].contract.Parameter.Value.Balance = int64Ptr(99000000)

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

func TestTronBuildRpcTransaction_AccountCreateContractSetsToAddress(t *testing.T) {
	contract := tronTxContract{Type: "AccountCreateContract"}
	contract.Parameter.Value.OwnerAddress = "41508b7b8057fc9170398a65bbc89ff3ccfcc0f4a5"
	contract.Parameter.Value.AccountAddress = "41da79e32a568680fccedadcab18a6e1bc231c0476"

	txByID := &tronGetTransactionByIDResponse{
		TxID: "e5babca390bfb5ba2e26151f031893f5b01237536fbd700f5f563423a1dc1b7d",
	}
	txByID.RawData.Contract = []tronTxContract{contract}

	txInfo := &tronGetTransactionInfoByIDResponse{
		BlockNumber: int64Ptr(1),
	}

	tx := tronBuildRpcTransaction(txByID, txInfo)

	require.Equal(t, ToTronAddressFromAddress(contract.Parameter.Value.OwnerAddress), tx.From)
	require.Equal(t, contract.Parameter.Value.AccountAddress, tx.To)
	require.Equal(t, "0x0", tx.Value)
}

func TestTronBuildRpcTransaction_WithdrawExpireUnfreezeSetsToAndValue(t *testing.T) {
	contract := tronTxContract{Type: "WithdrawExpireUnfreezeContract"}
	contract.Parameter.Value.OwnerAddress = "41da727d310b98700af4cec797e43991899668d6f3"
	contract.Parameter.Value.ReceiverAddress = "41734c2f23ab41c52308d1206c4eb5fe8e124e6898"

	txByID := &tronGetTransactionByIDResponse{}
	txByID.RawData.Contract = []tronTxContract{contract}
	txByID.TxID = "25b18a55f86afb10e7aca38d0073d04c80397c6636069193953fdefaea0b8369"
	txInfo := &tronGetTransactionInfoByIDResponse{
		BlockNumber:          int64Ptr(1),
		WithdrawExpireAmount: int64Ptr(88000000),
	}

	tx := tronBuildRpcTransaction(txByID, txInfo)
	value, err := hexutil.DecodeBig(tx.Value)

	require.NoError(t, err)
	require.Equal(t, int64(88000000), value.Int64())
	require.Equal(t, contract.Parameter.Value.ReceiverAddress, tx.To)
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
	require.Equal(t, "0x0", receipt.GasUsed)

	txInfo.Result = "FAILED"
	receipt = tronBuildRpcReceipt(txInfo)
	require.NotNil(t, receipt)
	require.Equal(t, "0x0", receipt.Status)
	require.Equal(t, "0x0", receipt.GasUsed)
}

func TestTronBuildRpcReceipt_UsesEnergyUsageTotalAsGasUsed(t *testing.T) {
	txInfo := &tronGetTransactionInfoByIDResponse{
		ID: "tx1",
	}
	txInfo.Receipt.EnergyUsageTotal = int64Ptr(14650)

	receipt := tronBuildRpcReceipt(txInfo)
	require.NotNil(t, receipt)
	require.Equal(t, "0x393a", receipt.GasUsed)
}

func TestTronBuildRpcReceipt_NormalizesOmittedLogDataForTxCache(t *testing.T) {
	contract := tronTxContract{Type: "TriggerSmartContract"}
	contract.Parameter.Value.OwnerAddress = "41b47e4a2a3b6652af6c8e4396fc5e490b3e8fa827"
	contract.Parameter.Value.ContractAddress = "TTg3AAJBYsDNjx5Moc5EPNsgJSa4anJQ3M"

	txByID := &tronGetTransactionByIDResponse{
		TxID: "a6bf472d7fbefa1a87f63b0626a98840f37a6863f4cdadc4a4aacfceff5c1073",
	}
	txByID.RawData.Contract = []tronTxContract{contract}

	txInfo := &tronGetTransactionInfoByIDResponse{
		BlockNumber: int64Ptr(1),
		Log: []*bchain.RpcLog{
			{
				Address: "TTg3AAJBYsDNjx5Moc5EPNsgJSa4anJQ3M",
				Topics: []string{
					"6917c54e363c87122ded2db643033caa7634085108272a134387eb8e5ddee762",
					"588c52d2eba6df506d44177ddda5e60b60842d3959ecf664d2c7b756b45f4820",
				},
			},
		},
	}

	csd := tronBuildEthereumSpecificData(txByID, txInfo)
	require.NotNil(t, csd.Receipt)
	require.Equal(t, "0x", csd.Receipt.Logs[0].Data)

	parser := NewTronParser(1, false)
	tx, err := parser.EthTxToTx(csd.Tx, csd.Receipt, nil, 0, 1, true)
	require.NoError(t, err)

	_, err = parser.PackTx(tx, 1, 0)
	require.NoError(t, err)
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

func TestSynthesizeGenesisTxInfo(t *testing.T) {
	txInfo := synthesizeGenesisTxInfo("0x1fdaa5bb76e3c1a5430f7d8920fe2cebc8120a14c87b3a9cba36e0a11b68b57e", 0, 1234)
	require.NotNil(t, txInfo)
	require.Equal(t, "1fdaa5bb76e3c1a5430f7d8920fe2cebc8120a14c87b3a9cba36e0a11b68b57e", txInfo.ID)
	require.NotNil(t, txInfo.BlockNumber)
	require.Equal(t, int64(0), *txInfo.BlockNumber)
	require.NotNil(t, txInfo.BlockTimeStamp)
	require.Equal(t, int64(1234000), *txInfo.BlockTimeStamp)

	txInfo = synthesizeGenesisTxInfo("0xabc", 1, 1234)
	require.Nil(t, txInfo)
}

func TestSynthesizeGenesisTxByID(t *testing.T) {
	rpcTx := &bchain.RpcTransaction{
		Hash:      "0x1fdaa5bb76e3c1a5430f7d8920fe2cebc8120a14c87b3a9cba36e0a11b68b57e",
		From:      "0x0000000000000000000000000000000000000000",
		To:        "0x7e95e45f5a60cc45f2d0afe37ee9f77fb8ce9fff",
		Value:     "0x15fb7f9b8c38000",
		Payload:   "0x",
		GasLimit:  "0x0",
		BlockHash: "0x0000000000000000d698d4192c56cb6be724a558448e2684802de4d6cd8690dc",
	}

	txByID := synthesizeGenesisTxByID(rpcTx, 0)
	require.NotNil(t, txByID)
	require.Equal(t, "1fdaa5bb76e3c1a5430f7d8920fe2cebc8120a14c87b3a9cba36e0a11b68b57e", txByID.TxID)
	require.NotNil(t, txByID.RawData.FeeLimit)
	require.Equal(t, int64(0), *txByID.RawData.FeeLimit)
	require.Len(t, txByID.RawData.Contract, 1)
	require.Equal(t, "TransferContract", txByID.RawData.Contract[0].Type)
	require.Equal(t, "0000000000000000000000000000000000000000", txByID.RawData.Contract[0].Parameter.Value.OwnerAddress)
	require.Equal(t, "7e95e45f5a60cc45f2d0afe37ee9f77fb8ce9fff", txByID.RawData.Contract[0].Parameter.Value.ToAddress)
	require.NotNil(t, txByID.RawData.Contract[0].Parameter.Value.Amount)
	require.Equal(t, int64(99000000000000000), *txByID.RawData.Contract[0].Parameter.Value.Amount)

	txByID = synthesizeGenesisTxByID(rpcTx, 1)
	require.Nil(t, txByID)
}

func TestTronHexQuantityToInt64Ptr(t *testing.T) {
	value := tronHexQuantityToInt64Ptr("0x15fb7f9b8c38000")
	require.NotNil(t, value)
	require.Equal(t, int64(99000000000000000), *value)

	require.Nil(t, tronHexQuantityToInt64Ptr(""))
	require.Nil(t, tronHexQuantityToInt64Ptr("not-a-quantity"))
	require.Nil(t, tronHexQuantityToInt64Ptr("0x8000000000000000"))
}

func TestTronBuildTxFromHTTPData_WithSynthesizedGenesisData(t *testing.T) {
	rpcTx := &bchain.RpcTransaction{
		Hash:     "0x1fdaa5bb76e3c1a5430f7d8920fe2cebc8120a14c87b3a9cba36e0a11b68b57e",
		From:     "0x0000000000000000000000000000000000000000",
		To:       "0x7e95e45f5a60cc45f2d0afe37ee9f77fb8ce9fff",
		Value:    "0x15fb7f9b8c38000",
		Payload:  "0x",
		GasLimit: "0x0",
	}
	txByID := synthesizeGenesisTxByID(rpcTx, 0)
	txInfo := synthesizeGenesisTxInfo(txByID.TxID, 0, 0)
	tronRPC := &TronRPC{
		Parser: NewTronParser(1, false),
	}
	tx, err := tronRPC.buildTxFromHTTPData(txByID, txInfo, 0, 1, nil, true)
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, "1fdaa5bb76e3c1a5430f7d8920fe2cebc8120a14c87b3a9cba36e0a11b68b57e", tx.Txid)
	require.Len(t, tx.Vout, 1)
	require.Equal(t, ToTronAddressFromAddress("7e95e45f5a60cc45f2d0afe37ee9f77fb8ce9fff"), tx.Vout[0].ScriptPubKey.Addresses[0])

	receipt := tronBuildRpcReceipt(txInfo)
	require.NotNil(t, receipt)
	require.Equal(t, "0x1", receipt.Status)
}

func TestTronBuildTxFromHTTPData_KeepReceiptControlsTokenLogs(t *testing.T) {
	txByID := &tronGetTransactionByIDResponse{
		TxID: "b7a97862b0c719b714f0cf7e250ebc2dcdf1e5f05c54ddb21ff3a748c1aa45e4",
	}
	txByID.RawData.Contract = []tronTxContract{{
		Type: "TriggerSmartContract",
	}}
	txByID.RawData.Contract[0].Parameter.Value.OwnerAddress = "410746a05c314538e3e21faae3d702cc7939efc07a"
	txByID.RawData.Contract[0].Parameter.Value.ContractAddress = "4139dd12a54e2bab7c82aa14a1e158b34263d2d510"
	txByID.RawData.Contract[0].Parameter.Value.Data = "6f21b898"

	txInfo := &tronGetTransactionInfoByIDResponse{
		ID: txByID.TxID,
		Log: []*bchain.RpcLog{
			{
				Address: "a614f803b6fd780986a42c78ec9c7f77e6ded13c",
				Data:    "000000000000000000000000000000000000000000000000000000002d4cae00",
				Topics: []string{
					"ddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef",
					"0000000000000000000000000746a05c314538e3e21faae3d702cc7939efc07a",
					"000000000000000000000000f14cca91e8d5fff29cbcd42b26a19a7395b1aa84",
				},
			},
		},
	}
	txInfo.Receipt.Result = "SUCCESS"

	parser := NewTronParser(1, false)
	tronRPC := &TronRPC{
		Parser: parser,
	}

	pendingTx, err := tronRPC.buildTxFromHTTPData(txByID, txInfo, 0, 0, nil, false)
	require.NoError(t, err)
	pendingSpecific, ok := pendingTx.CoinSpecificData.(bchain.EthereumSpecificData)
	require.True(t, ok)
	require.Nil(t, pendingSpecific.Receipt)
	pendingTransfers, err := parser.EthereumTypeGetTokenTransfersFromTx(pendingTx)
	require.NoError(t, err)
	require.Empty(t, pendingTransfers)

	indexedTx, err := tronRPC.buildTxFromHTTPData(txByID, txInfo, 0, 1, nil, true)
	require.NoError(t, err)
	indexedSpecific, ok := indexedTx.CoinSpecificData.(bchain.EthereumSpecificData)
	require.True(t, ok)
	require.NotNil(t, indexedSpecific.Receipt)
	require.Len(t, indexedSpecific.Receipt.Logs, 1)

	indexedTransfers, err := parser.EthereumTypeGetTokenTransfersFromTx(indexedTx)
	require.NoError(t, err)
	require.Len(t, indexedTransfers, 1)
	require.Equal(t, "TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", indexedTransfers[0].Contract)
	require.Equal(t, "TAdgLcNFZjeF1AspMobkz2bbxs5yDXpwYx", indexedTransfers[0].From)
	require.Equal(t, "TXy5qqpAJNykdqsMU2dZM7B7mZbVF2izAN", indexedTransfers[0].To)
	require.Equal(t, "760000000", indexedTransfers[0].Value.String())
}
