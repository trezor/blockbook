//go:build unittest

package tron

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
)

func TestResolveTronHTTPURL_UsesExplicitURL(t *testing.T) {
	got, err := resolveTronHTTPURL("http://fullnode.example:8090", "http://backend.example:8545/jsonrpc", tronDefaultFullNodeHTTPPort)
	require.NoError(t, err)
	require.Equal(t, "http://fullnode.example:8090", got)
}

func TestResolveTronHTTPURL_DerivesFromRPCURL(t *testing.T) {
	got, err := resolveTronHTTPURL("", "https://tron-node.example:8545/jsonrpc", tronDefaultFullNodeHTTPPort)
	require.NoError(t, err)
	require.Equal(t, "https://tron-node.example:8090", got)

	got, err = resolveTronHTTPURL("", "http://tron-node.example:8545/jsonrpc", tronDefaultSolidityHTTPPort)
	require.NoError(t, err)
	require.Equal(t, "http://tron-node.example:8091", got)
}

func TestResolveTronHTTPURL_InvalidRPCURL(t *testing.T) {
	_, err := resolveTronHTTPURL("", "://missing", tronDefaultFullNodeHTTPPort)
	require.Error(t, err)
}

type tronTestMempool struct {
	txTimes map[string]uint32
}

func (m *tronTestMempool) Resync() (int, error) {
	return 0, nil
}

func (m *tronTestMempool) GetTransactions(address string) ([]bchain.Outpoint, error) {
	return nil, nil
}

func (m *tronTestMempool) GetAddrDescTransactions(addrDesc bchain.AddressDescriptor) ([]bchain.Outpoint, error) {
	return nil, nil
}

func (m *tronTestMempool) GetAllEntries() bchain.MempoolTxidEntries {
	entries := make(bchain.MempoolTxidEntries, 0, len(m.txTimes))
	for txid, firstSeen := range m.txTimes {
		entries = append(entries, bchain.MempoolTxidEntry{
			Txid: txid,
			Time: firstSeen,
		})
	}
	return entries
}

func (m *tronTestMempool) GetTransactionTime(txid string) uint32 {
	return m.txTimes[txid]
}

func (m *tronTestMempool) GetTxidFilterEntries(filterScripts string, fromTimestamp uint32) (bchain.MempoolTxidFilterEntries, error) {
	return bchain.MempoolTxidFilterEntries{}, nil
}

func TestTronRPC_EthereumTypeGetRawTransaction_Empty(t *testing.T) {
	mockHTTP := &MockTronHTTPClient{
		Resp: tronGetTransactionByIDResponse{},
	}

	tronRPC := &TronRPC{
		EthereumRPC: &eth.EthereumRPC{
			Timeout: time.Second,
		},
		fullNodeHTTP:     mockHTTP,
		solidityNodeHTTP: mockHTTP,
	}

	_, err := tronRPC.EthereumTypeGetRawTransaction("0xabc")
	require.Error(t, err)
}

func TestTronRPC_EthereumTypeGetRawTransaction_FallbackToFullNode(t *testing.T) {
	solidityHTTP := &MockTronHTTPClient{
		Resp: map[string]any{},
	}
	fullNodeHTTP := &MockTronHTTPClient{
		Resp: tronGetTransactionByIDResponse{
			RawDataHex: "deadbeef",
		},
	}

	tronRPC := &TronRPC{
		EthereumRPC: &eth.EthereumRPC{
			Timeout: time.Second,
		},
		fullNodeHTTP:     fullNodeHTTP,
		solidityNodeHTTP: solidityHTTP,
	}

	rawHex, err := tronRPC.EthereumTypeGetRawTransaction("0xabc")
	require.NoError(t, err)
	require.Equal(t, "0xdeadbeef", rawHex)
	require.Equal(t, "/walletsolidity/gettransactionbyid", solidityHTTP.LastPath)
	require.Equal(t, map[string]string{"value": "abc"}, solidityHTTP.LastBody)
	require.Equal(t, "/wallet/gettransactionbyid", fullNodeHTTP.LastPath)
	require.Equal(t, map[string]string{"value": "abc"}, fullNodeHTTP.LastBody)
}

func TestTronRPC_GetTransactionByIDWithFallback_FallbackToFullNode(t *testing.T) {
	solidityHTTP := &MockTronHTTPClient{
		Resp: map[string]any{},
	}
	fullNodeHTTP := &MockTronHTTPClient{
		Resp: map[string]any{
			"txID": "tx1",
		},
	}

	tronRPC := &TronRPC{
		EthereumRPC: &eth.EthereumRPC{
			Timeout: time.Second,
		},
		fullNodeHTTP:     fullNodeHTTP,
		solidityNodeHTTP: solidityHTTP,
	}

	txByID, isSolidified, err := tronRPC.getTransactionByIDWithFallback("0x123")
	require.NoError(t, err)
	require.False(t, isSolidified)
	require.NotNil(t, txByID)
	require.Equal(t, "tx1", txByID.TxID)
	require.Equal(t, "/walletsolidity/gettransactionbyid", solidityHTTP.LastPath)
	require.Equal(t, "/wallet/gettransactionbyid", fullNodeHTTP.LastPath)
}

func TestTronRPC_GetTransactionInfoByIDWithFallback_FallbackToFullNode(t *testing.T) {
	solidityHTTP := &MockTronHTTPClient{
		Resp: map[string]any{},
	}
	fullNodeHTTP := &MockTronHTTPClient{
		Resp: map[string]any{
			"id": "tx1",
		},
	}

	tronRPC := &TronRPC{
		EthereumRPC: &eth.EthereumRPC{
			Timeout: time.Second,
		},
		fullNodeHTTP:     fullNodeHTTP,
		solidityNodeHTTP: solidityHTTP,
	}

	txInfo, isSolidified, err := tronRPC.getTransactionInfoByIDWithFallback("0x123")
	require.NoError(t, err)
	require.False(t, isSolidified)
	require.NotNil(t, txInfo)
	require.Equal(t, "tx1", txInfo.ID)
	require.Equal(t, "/walletsolidity/gettransactioninfobyid", solidityHTTP.LastPath)
	require.Equal(t, "/wallet/gettransactioninfobyid", fullNodeHTTP.LastPath)
}

func TestTronRPC_GetTransaction_NilMempoolDoesNotPanic(t *testing.T) {
	solidityHTTP := &MockTronHTTPClient{
		Resp: map[string]any{
			"id":   "abc",
			"txID": "abc",
		},
	}
	fullNodeHTTP := &MockTronHTTPClient{
		Resp: map[string]any{},
	}

	tronRPC := &TronRPC{
		EthereumRPC: &eth.EthereumRPC{
			Timeout: time.Second,
		},
		Parser:           NewTronParser(1, false),
		fullNodeHTTP:     fullNodeHTTP,
		solidityNodeHTTP: solidityHTTP,
	}

	tx, err := tronRPC.GetTransaction("0xabc")
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, "abc", tx.Txid)
	require.Equal(t, "/walletsolidity/gettransactionbyid", solidityHTTP.LastPath)
	require.Equal(t, "", fullNodeHTTP.LastPath)

	csd, ok := tx.CoinSpecificData.(bchain.EthereumSpecificData)
	require.True(t, ok)
	require.NotNil(t, csd.Receipt)
}

func TestTronRPC_GetTransaction_FallbackToFullNodeKeepsPendingEvenWithBlockNumber(t *testing.T) {
	solidityHTTP := &MockTronHTTPClient{
		Resp: map[string]any{},
	}
	fullNodeHTTP := &MockTronHTTPClient{
		Resp: map[string]any{
			"id":             "abc",
			"txID":           "abc",
			"blockNumber":    int64(123),
			"blockTimeStamp": int64(1700000000000),
		},
	}

	tronRPC := &TronRPC{
		EthereumRPC: &eth.EthereumRPC{
			Timeout: time.Second,
		},
		Parser:           NewTronParser(1, false),
		fullNodeHTTP:     fullNodeHTTP,
		solidityNodeHTTP: solidityHTTP,
	}

	tx, err := tronRPC.GetTransaction("0xabc")
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, "abc", tx.Txid)
	require.Equal(t, "/walletsolidity/gettransactioninfobyid", solidityHTTP.LastPath)
	require.Equal(t, "/wallet/gettransactionbyid", fullNodeHTTP.LastPath)

	csd, ok := tx.CoinSpecificData.(bchain.EthereumSpecificData)
	require.True(t, ok)
	require.Nil(t, csd.Receipt)
}

func TestTronRPC_ReconcileTronMempoolWithPendingList_RemovesMissingTxs(t *testing.T) {
	m := &tronTestMempool{
		txTimes: map[string]uint32{
			"a1": 1,
			"b2": 2,
			"c3": 3,
		},
	}

	removedTxs := make(map[string]struct{})
	removed := reconcileTronMempoolWithPendingList(m, []string{"0xa1", "c3"}, func(txid string) {
		removedTxs[txid] = struct{}{}
		delete(m.txTimes, txid)
	})

	require.Equal(t, 1, removed)
	_, removedB2 := removedTxs["b2"]
	require.True(t, removedB2)
	require.Equal(t, map[string]uint32{
		"a1": 1,
		"c3": 3,
	}, m.txTimes)
}

func TestTronRPC_GetTransactionByID_EmptyObjectMeansNotFound(t *testing.T) {
	mockHTTP := &MockTronHTTPClient{
		Resp: map[string]any{},
	}

	tronRPC := &TronRPC{
		EthereumRPC: &eth.EthereumRPC{
			Timeout: time.Second,
		},
		fullNodeHTTP:     mockHTTP,
		solidityNodeHTTP: mockHTTP,
	}

	tx, err := tronRPC.getTransactionByID("0x788b4d0ca432b3d07f895dffe80429bf58398d0e86222460b07f9db38e238803", true)
	require.Error(t, err)
	require.Nil(t, tx)
	require.Equal(t, "/walletsolidity/gettransactionbyid", mockHTTP.LastPath)
	require.Equal(t, map[string]string{"value": "788b4d0ca432b3d07f895dffe80429bf58398d0e86222460b07f9db38e238803"}, mockHTTP.LastBody)
}

func TestTronRPC_GetTransactionInfoByID_EmptyObjectMeansNoData(t *testing.T) {
	mockHTTP := &MockTronHTTPClient{
		Resp: map[string]any{},
	}

	tronRPC := &TronRPC{
		EthereumRPC: &eth.EthereumRPC{
			Timeout: time.Second,
		},
		fullNodeHTTP:     mockHTTP,
		solidityNodeHTTP: mockHTTP,
	}

	txInfo, err := tronRPC.getTransactionInfoByID("0x788b4d0ca432b3d07f895dffe80429bf58398d0e86222460b07f9db38e238803", true)
	require.Error(t, err)
	require.Nil(t, txInfo)
	require.Equal(t, "/walletsolidity/gettransactioninfobyid", mockHTTP.LastPath)
	require.Equal(t, map[string]string{"value": "788b4d0ca432b3d07f895dffe80429bf58398d0e86222460b07f9db38e238803"}, mockHTTP.LastBody)
}

func TestTronRPC_GetTransactionInfoByID_NonEmptyObjectReturned(t *testing.T) {
	mockHTTP := &MockTronHTTPClient{
		Resp: map[string]any{
			"id": "tx1",
		},
	}

	tronRPC := &TronRPC{
		EthereumRPC: &eth.EthereumRPC{
			Timeout: time.Second,
		},
		fullNodeHTTP:     mockHTTP,
		solidityNodeHTTP: mockHTTP,
	}

	txInfo, err := tronRPC.getTransactionInfoByID("0x123", true)
	require.NoError(t, err)
	require.NotNil(t, txInfo)
	require.Equal(t, "tx1", txInfo.ID)
	require.Equal(t, "/walletsolidity/gettransactioninfobyid", mockHTTP.LastPath)
}

func TestTronRPC_SendRawTransaction(t *testing.T) {
	txID := "7c2d4206c03a883dd9066d620335dc1be272a8dc733cfa3f6d10308faa37facc"
	txHex := "0xdeadbeef"

	mockHTTP := &MockTronHTTPClient{
		Resp: tronBroadcastHexResponse{
			Result: true,
			TxID:   txID,
		},
	}

	tronRPC := &TronRPC{
		EthereumRPC: &eth.EthereumRPC{
			Timeout: time.Second,
		},
		fullNodeHTTP:     mockHTTP,
		solidityNodeHTTP: mockHTTP,
	}

	gotTxID, err := tronRPC.SendRawTransaction(txHex, false)
	require.NoError(t, err)
	require.Equal(t, txID, gotTxID)
	require.Equal(t, "/wallet/broadcasthex", mockHTTP.LastPath)
	require.Equal(t, map[string]string{"transaction": "deadbeef"}, mockHTTP.LastBody)
}

func TestTronRPC_SendRawTransaction_StripsPrefixFromResponse(t *testing.T) {
	txHex := "deadbeef"

	mockHTTP := &MockTronHTTPClient{
		Resp: tronBroadcastHexResponse{
			Result: true,
			TxID:   "0x7c2d4206c03a883dd9066d620335dc1be272a8dc733cfa3f6d10308faa37facc",
		},
	}

	tronRPC := &TronRPC{
		EthereumRPC: &eth.EthereumRPC{
			Timeout: time.Second,
		},
		fullNodeHTTP:     mockHTTP,
		solidityNodeHTTP: mockHTTP,
	}

	gotTxID, err := tronRPC.SendRawTransaction(txHex, false)
	require.NoError(t, err)
	require.Equal(t, "7c2d4206c03a883dd9066d620335dc1be272a8dc733cfa3f6d10308faa37facc", gotTxID)
}

func TestTronRPC_SendRawTransaction_Failed(t *testing.T) {
	mockHTTP := &MockTronHTTPClient{
		Resp: tronBroadcastHexResponse{
			Result:  false,
			Code:    "SIGERROR",
			Message: "error",
		},
	}

	tronRPC := &TronRPC{
		EthereumRPC: &eth.EthereumRPC{
			Timeout: time.Second,
		},
		fullNodeHTTP:     mockHTTP,
		solidityNodeHTTP: mockHTTP,
	}

	_, err := tronRPC.SendRawTransaction("deadbeef", false)
	require.Error(t, err)
}

func TestTronRPC_GetMempoolTransactions(t *testing.T) {
	mockHTTP := &MockTronHTTPClient{
		Resp: tronGetTransactionListFromPendingResponse{
			TxID: []string{
				"a431984fef1d014620504d02f821f872221cf44c250a81a31e81fa4855b2b302",
				"b431984fef1d014620504d02f821f872221cf44c250a81a31e81fa4855b2b303",
			},
		},
	}

	tronRPC := &TronRPC{
		EthereumRPC: &eth.EthereumRPC{
			Timeout: time.Second,
		},
		fullNodeHTTP:     mockHTTP,
		solidityNodeHTTP: mockHTTP,
	}

	txs, err := tronRPC.GetMempoolTransactions()
	require.NoError(t, err)
	require.Equal(t, []string{
		"a431984fef1d014620504d02f821f872221cf44c250a81a31e81fa4855b2b302",
		"b431984fef1d014620504d02f821f872221cf44c250a81a31e81fa4855b2b303",
	}, txs)
	require.Equal(t, "/wallet/gettransactionlistfrompending", mockHTTP.LastPath)
	require.Equal(t, map[string]any{}, mockHTTP.LastBody)
}

func TestTronRPC_GetMempoolTransactions_Error(t *testing.T) {
	mockHTTP := &MockTronHTTPClient{
		Err: errors.New("backend error"),
	}

	tronRPC := &TronRPC{
		EthereumRPC: &eth.EthereumRPC{
			Timeout: time.Second,
		},
		fullNodeHTTP:     mockHTTP,
		solidityNodeHTTP: mockHTTP,
	}

	_, err := tronRPC.GetMempoolTransactions()
	require.Error(t, err)
}

func TestTronRPC_GetAddressChainExtraData(t *testing.T) {
	mockHTTP := &MockTronHTTPClient{
		Resp: tronGetAccountResourceResponse{
			FreeNetLimit: 600,
			FreeNetUsed:  100,
			NetLimit:     400,
			NetUsed:      250,
			EnergyLimit:  9000,
			EnergyUsed:   1234,
		},
	}
	parser := NewTronParser(1, false)
	addrDesc, err := parser.GetAddrDescFromAddress("TLUqyV9rGYXZ2E8kXe6J3P1rvYV1Au1Goe")
	require.NoError(t, err)

	tronRPC := &TronRPC{
		EthereumRPC: &eth.EthereumRPC{
			Timeout: time.Second,
		},
		fullNodeHTTP:     mockHTTP,
		solidityNodeHTTP: mockHTTP,
	}

	payload, err := tronRPC.GetAddressChainExtraData(addrDesc)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"availableStakedBandwidth":150,
		"totalStakedBandwidth":400,
		"availableFreeBandwidth":500,
		"totalFreeBandwidth":600,
		"availableEnergy":7766,
		"totalEnergy":9000
	}`, string(payload))
	require.Equal(t, "/wallet/getaccountresource", mockHTTP.LastPath)
	require.Equal(t, map[string]any{
		"address": "TLUqyV9rGYXZ2E8kXe6J3P1rvYV1Au1Goe",
		"visible": true,
	}, mockHTTP.LastBody)
}

func TestTronRPC_GetAddressChainExtraData_MissingFieldsClampToZero(t *testing.T) {
	mockHTTP := &MockTronHTTPClient{
		Resp: map[string]any{
			"freeNetLimit": int64(100),
			"freeNetUsed":  int64(150),
			"NetLimit":     int64(50),
			"NetUsed":      int64(10),
			"EnergyUsed":   int64(20),
		},
	}

	tronRPC := &TronRPC{
		EthereumRPC: &eth.EthereumRPC{
			Timeout: time.Second,
		},
		fullNodeHTTP:     mockHTTP,
		solidityNodeHTTP: mockHTTP,
	}

	payload, err := tronRPC.GetAddressChainExtraData(bchain.AddressDescriptor{
		0x73, 0x4c, 0x2f, 0x23, 0xab, 0x41, 0xc5, 0x23, 0x08, 0xd1,
		0x20, 0x6c, 0x4e, 0xb5, 0xfe, 0x8e, 0x12, 0x4e, 0x68, 0x98,
	})
	require.NoError(t, err)

	var extra bchain.TronAccountExtraData
	require.NoError(t, json.Unmarshal(payload, &extra))
	require.Equal(t, bchain.TronAccountExtraData{
		AvailableStakedBandwidth: 40,
		TotalStakedBandwidth:     50,
		AvailableFreeBandwidth:   0,
		TotalFreeBandwidth:       100,
		AvailableEnergy:          0,
		TotalEnergy:              0,
	}, extra)
}

func TestTronRPC_RequestLatestSolidifiedBlockHeight(t *testing.T) {
	mockHTTP := &MockTronHTTPClient{
		Resp: map[string]any{
			"block_header": map[string]any{
				"raw_data": map[string]any{
					"number": uint64(123456),
				},
			},
		},
	}

	tronRPC := &TronRPC{
		EthereumRPC: &eth.EthereumRPC{
			Timeout: time.Second,
		},
		fullNodeHTTP:     mockHTTP,
		solidityNodeHTTP: mockHTTP,
	}

	height, err := tronRPC.requestLatestSolidifiedBlockHeight(context.Background())
	require.NoError(t, err)
	require.Equal(t, uint64(123456), height)
	require.Equal(t, "/walletsolidity/getblock", mockHTTP.LastPath)
	require.Equal(t, map[string]any{"detail": false}, mockHTTP.LastBody)
}

func TestTronRPC_RequestLatestSolidifiedBlockHeight_MissingNumber(t *testing.T) {
	mockHTTP := &MockTronHTTPClient{
		Resp: map[string]any{
			"block_header": map[string]any{
				"raw_data": map[string]any{},
			},
		},
	}

	tronRPC := &TronRPC{
		EthereumRPC: &eth.EthereumRPC{
			Timeout: time.Second,
		},
		fullNodeHTTP:     mockHTTP,
		solidityNodeHTTP: mockHTTP,
	}

	_, err := tronRPC.requestLatestSolidifiedBlockHeight(context.Background())
	require.Error(t, err)
}
