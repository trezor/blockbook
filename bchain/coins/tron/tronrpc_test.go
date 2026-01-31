//go:build unittest

package tron

import (
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
	require.Equal(t, map[string]string{"value": "abc"}, mockHTTP.LastBody)
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
