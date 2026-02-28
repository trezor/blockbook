//go:build unittest

package tron

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/trezor/blockbook/bchain/coins/eth"
)

func TestTronRPC_EthereumTypeGetRawTransaction(t *testing.T) {
	rawDataHex := "0a02b6632208fb1feb948ee9fff240e0d4f1dbf7305a67080112630a2d747970652e676f6f676c65617069732e636f6d2f70726f746f636f6c2e5472616e73666572436f6e747261637412320a1541816cf60987aa124eed29db9a057e476861b8d8dc1215413516435fb1e706c51efff614c7e14ce2625f28e51880897a70f494e0caf7309001a0c21e"
	mockHTTP := &MockTronHTTPClient{
		Resp: tronGetTransactionByIDResponse{
			RawDataHex: rawDataHex,
		},
	}

	tronRPC := &TronRPC{
		EthereumRPC: &eth.EthereumRPC{
			Timeout: time.Second,
		},
		http: mockHTTP,
	}

	rawHex, err := tronRPC.EthereumTypeGetRawTransaction("0x7c2d4206c03a883dd9066d620335dc1be272a8dc733cfa3f6d10308faa37facc")
	require.NoError(t, err)
	require.Equal(t, "0x"+rawDataHex, rawHex)
	require.Equal(t, "/wallet/gettransactionbyid", mockHTTP.LastPath)
	require.Equal(t, map[string]string{"value": "7c2d4206c03a883dd9066d620335dc1be272a8dc733cfa3f6d10308faa37facc"}, mockHTTP.LastBody)
}

func TestTronRPC_EthereumTypeGetRawTransaction_Empty(t *testing.T) {
	mockHTTP := &MockTronHTTPClient{
		Resp: tronGetTransactionByIDResponse{},
	}

	tronRPC := &TronRPC{
		EthereumRPC: &eth.EthereumRPC{
			Timeout: time.Second,
		},
		http: mockHTTP,
	}

	_, err := tronRPC.EthereumTypeGetRawTransaction("0xabc")
	require.Error(t, err)
}

func TestTronRPC_GetTransactionByID_EmptyObjectMeansNotFound(t *testing.T) {
	mockHTTP := &MockTronHTTPClient{
		Resp: map[string]any{},
	}

	tronRPC := &TronRPC{
		EthereumRPC: &eth.EthereumRPC{
			Timeout: time.Second,
		},
		http: mockHTTP,
	}

	tx, err := tronRPC.getTransactionByID("0x788b4d0ca432b3d07f895dffe80429bf58398d0e86222460b07f9db38e238803")
	require.NoError(t, err)
	require.Nil(t, tx)
	require.Equal(t, "/wallet/gettransactionbyid", mockHTTP.LastPath)
	require.Equal(t, map[string]string{"value": "788b4d0ca432b3d07f895dffe80429bf58398d0e86222460b07f9db38e238803"}, mockHTTP.LastBody)
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
		http: mockHTTP,
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
		http: mockHTTP,
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
		http: mockHTTP,
	}

	_, err := tronRPC.SendRawTransaction("deadbeef", false)
	require.Error(t, err)
}

func TestTronRPC_GetTransactionByIDMapForBlock_ByHeight(t *testing.T) {
	txid := "7c2d4206c03a883dd9066d620335dc1be272a8dc733cfa3f6d10308faa37facc"
	mockHTTP := &MockTronHTTPClient{
		Resp: tronGetBlockResponse{
			Transactions: []tronGetTransactionByIDResponse{
				{
					TxID:       txid,
					RawDataHex: "01",
				},
			},
		},
	}

	tronRPC := &TronRPC{
		EthereumRPC: &eth.EthereumRPC{
			Timeout: time.Second,
		},
		http: mockHTTP,
	}

	txByID, err := tronRPC.getTransactionByIDMapForBlock("", 25)
	require.NoError(t, err)
	require.Equal(t, "/wallet/getblockbynum", mockHTTP.LastPath)
	require.Equal(t, map[string]any{"num": uint32(25)}, mockHTTP.LastBody)
	require.NotNil(t, txByID[txid])
}

func TestTronRPC_GetTransactionByIDMapForBlock_ByHash(t *testing.T) {
	txid := "7c2d4206c03a883dd9066d620335dc1be272a8dc733cfa3f6d10308faa37facc"
	mockHTTP := &MockTronHTTPClient{
		Resp: tronGetBlockResponse{
			Transactions: []tronGetTransactionByIDResponse{
				{
					TxID:       txid,
					RawDataHex: "01",
				},
			},
		},
	}

	tronRPC := &TronRPC{
		EthereumRPC: &eth.EthereumRPC{
			Timeout: time.Second,
		},
		http: mockHTTP,
	}

	txByID, err := tronRPC.getTransactionByIDMapForBlock("0xabc123", 0)
	require.NoError(t, err)
	require.Equal(t, "/wallet/getblockbyid", mockHTTP.LastPath)
	require.Equal(t, map[string]string{"value": "abc123"}, mockHTTP.LastBody)
	require.NotNil(t, txByID[txid])
}

func TestTronRPC_GetMempoolTransactions(t *testing.T) {
	mockHTTP := &MockTronHTTPClient{
		Resp: tronGetTransactionListFromPendingResponse{
			TxID: []string{
				"a431984fef1d014620504d02f821f872221cf44c250a81a31e81fa4855b2b302",
				"0xb431984fef1d014620504d02f821f872221cf44c250a81a31e81fa4855b2b303",
			},
		},
	}

	tronRPC := &TronRPC{
		EthereumRPC: &eth.EthereumRPC{
			Timeout: time.Second,
		},
		http: mockHTTP,
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
		http: mockHTTP,
	}

	_, err := tronRPC.GetMempoolTransactions()
	require.Error(t, err)
}
