//go:build unittest

package tron

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/trezor/blockbook/bchain"
)

type MockTronHTTPClient struct {
	Resp interface{}
	Err  error

	LastPath string
	LastBody interface{}
}

func (m *MockTronHTTPClient) Request(ctx context.Context, path string, reqBody interface{}, respBody interface{}) error {
	m.LastPath = path
	m.LastBody = reqBody

	if m.Err != nil {
		return m.Err
	}
	b, _ := json.Marshal(m.Resp)
	return json.Unmarshal(b, respBody)
}

func TestTronInternalDataProvider_GetInternalDataForBlock_Simple(t *testing.T) {
	bchain.ProcessInternalTransactions = true

	// fake transaction info returned from the Tron HTTP API
	fake := []tronTxInfo{
		{
			ID: "abcd",
			InternalTransactions: []tronInternalTransaction{
				{
					CallerAddress:     "41734c2f23ab41c52308d1206c4eb5fe8e124e6898",
					TransferToAddress: "41da727d310b98700af4cec797e43991899668d6f3",
					Note:              "63616c6c", // "call"
					CallValueInfo: []tronCallValueInfo{
						{CallValue: 123456},
					},
				},
			},
			Receipt: tronReceipt{Result: "SUCCESS"},
		},
	}

	mockHTTP := &MockTronHTTPClient{
		Resp: fake,
	}

	provider := NewTronInternalDataProvider(mockHTTP, time.Second)

	txs := []bchain.RpcTransaction{
		{Hash: "0xabcd"},
	}

	data, contracts, err := provider.GetInternalDataForBlock("", 99, txs)

	require.NoError(t, err)

	// verify HTTP call
	require.Equal(t, "/walletsolidity/gettransactioninfobyblocknum", mockHTTP.LastPath)
	require.Equal(t, map[string]any{"num": uint32(99)}, mockHTTP.LastBody)

	// verify parsed internal data
	require.Len(t, data, 1)
	require.Len(t, contracts, 0)

	d := data[0]
	require.Equal(t, bchain.CALL, d.Type)
	require.Len(t, d.Transfers, 1)
	require.Equal(t, int64(123456), d.Transfers[0].Value.Int64())

	require.Equal(t, "TLUqyV9rGYXZ2E8kXe6J3P1rvYV1Au1Goe", d.Transfers[0].From)
	require.Equal(t, "TVtFTiSQmeMkdpusjefUcPcEeTPtqnhz3D", d.Transfers[0].To)
}

func TestBuildInternalDataFromTronInfos(t *testing.T) {

	tests := []struct {
		name              string
		infos             []tronTxInfo
		txs               []bchain.RpcTransaction
		wantType          bchain.EthereumInternalTransactionType
		wantTransfers     int
		wantContracts     int
		wantErrContains   string // error return from function
		wantDataErrSubstr string // d.Error (EthereumInternalData.Error)
		wantContract      string
		wantFrom          string
		wantTo            string
		wantValue         int64
	}{
		{
			name: "CALL with TRX transfer",
			infos: []tronTxInfo{
				{
					ID: "abcd1234",
					InternalTransactions: []tronInternalTransaction{
						{
							CallerAddress:     "41734c2f23ab41c52308d1206c4eb5fe8e124e6898",
							TransferToAddress: "41da727d310b98700af4cec797e43991899668d6f3",
							Note:              "63616c6c", // "call"
							CallValueInfo: []tronCallValueInfo{
								{CallValue: 700000},
							},
						},
					},
					Receipt: tronReceipt{Result: "SUCCESS"},
				},
			},
			txs: []bchain.RpcTransaction{{Hash: "0xabcd1234"}},

			wantType:      bchain.CALL,
			wantTransfers: 1,

			wantFrom:  "TLUqyV9rGYXZ2E8kXe6J3P1rvYV1Au1Goe",
			wantTo:    "TVtFTiSQmeMkdpusjefUcPcEeTPtqnhz3D",
			wantValue: 700000,
		},

		{
			name: "CREATE detected by internal note",
			infos: []tronTxInfo{
				{
					ID:              "0544ab15ada7051af68b57ca29d69c753b64e6701cfebe5cdbe53a2a9127a88d",
					ContractAddress: "4139dd12a54e2bab7c82aa14a1e158b34263d2d510",
					InternalTransactions: []tronInternalTransaction{
						{
							CallerAddress:     "4139dd12a54e2bab7c82aa14a1e158b34263d2d510",
							TransferToAddress: "41ed56e617db5eab11b61a9eaefc98c77a6798d257",
							Note:              "637265617465", // create
						},
					},
				},
			},
			txs:           []bchain.RpcTransaction{{Hash: "0x0544ab15ada7051af68b57ca29d69c753b64e6701cfebe5cdbe53a2a9127a88d"}},
			wantType:      bchain.CREATE,
			wantContracts: 1,
			wantContract:  "TXc9FMgWcKK7zGApKj9rArxDb49QkJZWXn",
		},

		{
			name: "SELFDESTRUCT detected",
			infos: []tronTxInfo{
				{
					ID: "deadbeef",
					InternalTransactions: []tronInternalTransaction{
						{Note: "73756963696465", CallerAddress: "4139dd12a54e2bab7c82aa14a1e158b34263d2d510"}, // suicide
					},
				},
			},
			txs:      []bchain.RpcTransaction{{Hash: "0xdeadbeef"}},
			wantType: bchain.SELFDESTRUCT,
		},

		{
			name: "Rejected internal call",
			infos: []tronTxInfo{
				{
					ID: "fail01",
					InternalTransactions: []tronInternalTransaction{
						{
							Note:     "63616c6c",
							Rejected: true,
						},
					},
					Receipt: tronReceipt{Result: "SUCCESS"},
				},
			},
			txs:               []bchain.RpcTransaction{{Hash: "0xfail01"}},
			wantType:          bchain.CALL,
			wantDataErrSubstr: "rejected",
		},

		{
			name: "Invalid hex in note",
			infos: []tronTxInfo{
				{
					ID: "bad1",
					InternalTransactions: []tronInternalTransaction{
						{Note: "this-is-not-hex"},
					},
				},
			},
			txs:             []bchain.RpcTransaction{{Hash: "0xbad1"}},
			wantErrContains: "invalid",
		},

		{
			name: "No internal transactions",
			infos: []tronTxInfo{
				{ID: "nointernal"},
			},
			txs:      []bchain.RpcTransaction{{Hash: "0xnointernal"}},
			wantType: bchain.CALL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			data, contracts, err := buildInternalDataFromTronInfos(tt.infos, tt.txs, 12345)

			if tt.wantErrContains != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErrContains)
				return
			}

			require.NoError(t, err)
			require.Len(t, data, 1)

			d := data[0]

			if tt.wantType != 0 {
				require.Equal(t, tt.wantType, d.Type)
			}

			require.Len(t, d.Transfers, tt.wantTransfers)

			if tt.wantTransfers > 0 {
				tr := d.Transfers[0]

				require.Equal(t, tt.wantValue, tr.Value.Int64())

				if tt.wantFrom != "" {
					require.Equal(t, tt.wantFrom, tr.From)
				}
				if tt.wantTo != "" {
					require.Equal(t, tt.wantTo, tr.To)
				}
			}

			if tt.wantContracts > 0 {
				require.Len(t, contracts, tt.wantContracts)
				if tt.wantContract != "" {
					require.Equal(t, tt.wantContract, d.Contract)
				}
			}

			if tt.wantDataErrSubstr != "" {
				require.Contains(t, d.Error, tt.wantDataErrSubstr)
			}
		})
	}
}
