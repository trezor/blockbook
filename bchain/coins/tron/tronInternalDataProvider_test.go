//go:build unittest

package tron

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/trezor/blockbook/bchain"
)

type MockTronHTTPClient struct {
	Resp       interface{}
	RespByPath map[string]interface{}
	ErrByPath  map[string]error
	Err        error

	mu sync.RWMutex

	LastPath string
	LastBody interface{}
	Paths    []string
	Bodies   []interface{}
}

func (m *MockTronHTTPClient) Request(ctx context.Context, path string, reqBody interface{}, respBody interface{}) error {
	m.mu.Lock()
	m.LastPath = path
	m.LastBody = reqBody
	m.Paths = append(m.Paths, path)
	m.Bodies = append(m.Bodies, reqBody)
	m.mu.Unlock()

	if m.ErrByPath != nil {
		if err, ok := m.ErrByPath[path]; ok {
			return err
		}
	}
	if m.Err != nil {
		return m.Err
	}
	resp := m.Resp
	if m.RespByPath != nil {
		if v, ok := m.RespByPath[path]; ok {
			resp = v
		}
	}
	b, _ := json.Marshal(resp)
	return json.Unmarshal(b, respBody)
}

func (m *MockTronHTTPClient) SnapshotLastRequest() (string, interface{}) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.LastPath, m.LastBody
}

func (m *MockTronHTTPClient) SnapshotRequests() ([]string, []interface{}) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	paths := append([]string(nil), m.Paths...)
	bodies := append([]interface{}(nil), m.Bodies...)
	return paths, bodies
}

func TestTronInternalDataProvider_GetInternalDataForBlock_Simple(t *testing.T) {
	bchain.ProcessInternalTransactions = true
	t.Cleanup(func() { bchain.ProcessInternalTransactions = false })

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
	lastPath, lastBody := mockHTTP.SnapshotLastRequest()
	require.Equal(t, "/walletsolidity/gettransactioninfobyblocknum", lastPath)
	require.Equal(t, map[string]any{"num": uint32(99)}, lastBody)

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

// contractEvent describes one expected contract registry entry: a creation
// (destroyed=false) or a destruction (destroyed=true), in emission order
type contractEvent struct {
	addr      string
	destroyed bool
}

// transferEvent describes one expected internal transfer, in emission order
type transferEvent struct {
	typ   bchain.EthereumInternalTransactionType
	from  string
	to    string
	value int64
}

func TestBuildInternalDataFromTronInfos(t *testing.T) {

	tests := []struct {
		name             string
		infos            []tronTxInfo
		txs              []bchain.RpcTransaction
		wantType         bchain.EthereumInternalTransactionType
		wantTransfers    int
		wantErrContains  string // error return from function
		wantDataErr      string // exact d.Error (EthereumInternalData.Error)
		wantContract     string
		wantContracts    []contractEvent // exact contract registry entries in order
		wantTransferList []transferEvent // exact transfers in order; overrides the wantTransfers/wantFrom/wantTo/wantValue shorthand
		wantFrom         string
		wantTo           string
		wantValue        int64
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
			txs: []bchain.RpcTransaction{{Hash: "0xabcd1234", To: "0x39dd12a54e2bab7c82aa14a1e158b34263d2d510"}},

			wantType:      bchain.CALL,
			wantTransfers: 1,

			wantFrom:  "TLUqyV9rGYXZ2E8kXe6J3P1rvYV1Au1Goe",
			wantTo:    "TVtFTiSQmeMkdpusjefUcPcEeTPtqnhz3D",
			wantValue: 700000,
		},

		{
			// regression for #1660: java-tron fills contract_address for ordinary
			// TriggerSmartContract calls; they must not be treated as deployments
			name: "TriggerSmartContract with contract_address stays CALL",
			infos: []tronTxInfo{
				{
					ID:              "call1660",
					ContractAddress: "4139dd12a54e2bab7c82aa14a1e158b34263d2d510",
					InternalTransactions: []tronInternalTransaction{
						{
							CallerAddress:     "41734c2f23ab41c52308d1206c4eb5fe8e124e6898",
							TransferToAddress: "41da727d310b98700af4cec797e43991899668d6f3",
							Note:              "63616c6c", // "call"
							CallValueInfo: []tronCallValueInfo{
								{CallValue: 13245012561},
							},
						},
					},
					Receipt: tronReceipt{Result: "SUCCESS"},
				},
			},
			txs: []bchain.RpcTransaction{{Hash: "0xcall1660", To: "0x39dd12a54e2bab7c82aa14a1e158b34263d2d510"}},

			wantType:      bchain.CALL,
			wantTransfers: 1,
			wantValue:     13245012561,
		},

		{
			name: "Deployment without internal transactions",
			infos: []tronTxInfo{
				{
					ID:              "deploy1",
					ContractAddress: "4139dd12a54e2bab7c82aa14a1e158b34263d2d510",
					Receipt:         tronReceipt{Result: "SUCCESS"},
				},
			},
			txs:           []bchain.RpcTransaction{{Hash: "0xdeploy1"}},
			wantType:      bchain.CREATE,
			wantContract:  "TFFAMQLZybALaLb4uxHA9RBE7pxhUAjF3U",
			wantContracts: []contractEvent{{addr: "TFFAMQLZybALaLb4uxHA9RBE7pxhUAjF3U"}},
		},

		{
			// java-tron precomputes contract_address even when the deployment
			// failed - nothing was created, nothing may be registered
			name: "Failed deployment registers no contract",
			infos: []tronTxInfo{
				{
					ID:              "deployfail",
					ContractAddress: "4139dd12a54e2bab7c82aa14a1e158b34263d2d510",
					Receipt:         tronReceipt{Result: "OUT_OF_ENERGY"},
				},
			},
			txs:         []bchain.RpcTransaction{{Hash: "0xdeployfail"}},
			wantType:    bchain.CALL,
			wantDataErr: "OUT_OF_ENERGY",
		},

		{
			name: "Deployment with constructor-created child contract",
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
			txs:          []bchain.RpcTransaction{{Hash: "0x0544ab15ada7051af68b57ca29d69c753b64e6701cfebe5cdbe53a2a9127a88d"}},
			wantType:     bchain.CREATE,
			wantContract: "TFFAMQLZybALaLb4uxHA9RBE7pxhUAjF3U",
			wantContracts: []contractEvent{
				{addr: "TFFAMQLZybALaLb4uxHA9RBE7pxhUAjF3U"}, // deployed contract
				{addr: "TXc9FMgWcKK7zGApKj9rArxDb49QkJZWXn"}, // constructor-created child
			},
			// the zero-value create emits a transfer so that the child's
			// address history contains the deploying transaction
			wantTransferList: []transferEvent{
				{typ: bchain.CREATE, from: "TFFAMQLZybALaLb4uxHA9RBE7pxhUAjF3U", to: "TXc9FMgWcKK7zGApKj9rArxDb49QkJZWXn"},
			},
		},

		{
			// a call into a factory that internally deploys a contract stays a CALL,
			// only the child is registered
			name: "Factory call with nested create stays CALL",
			infos: []tronTxInfo{
				{
					ID:              "factory1",
					ContractAddress: "4139dd12a54e2bab7c82aa14a1e158b34263d2d510",
					InternalTransactions: []tronInternalTransaction{
						{
							CallerAddress:     "4139dd12a54e2bab7c82aa14a1e158b34263d2d510",
							TransferToAddress: "41ed56e617db5eab11b61a9eaefc98c77a6798d257",
							Note:              "637265617465", // create
						},
					},
					Receipt: tronReceipt{Result: "SUCCESS"},
				},
			},
			txs:           []bchain.RpcTransaction{{Hash: "0xfactory1", To: "0x39dd12a54e2bab7c82aa14a1e158b34263d2d510"}},
			wantType:      bchain.CALL,
			wantContracts: []contractEvent{{addr: "TXc9FMgWcKK7zGApKj9rArxDb49QkJZWXn"}},
			wantTransferList: []transferEvent{
				{typ: bchain.CREATE, from: "TFFAMQLZybALaLb4uxHA9RBE7pxhUAjF3U", to: "TXc9FMgWcKK7zGApKj9rArxDb49QkJZWXn"},
			},
		},

		{
			// the top-level type stays CALL: only its low bit survives packing,
			// so a top-level SELFDESTRUCT could never round-trip through the DB
			name: "Suicide frame registers destruction, top-level type stays CALL",
			infos: []tronTxInfo{
				{
					ID: "deadbeef",
					InternalTransactions: []tronInternalTransaction{
						{Note: "73756963696465", CallerAddress: "4139dd12a54e2bab7c82aa14a1e158b34263d2d510"}, // suicide
					},
				},
			},
			txs:           []bchain.RpcTransaction{{Hash: "0xdeadbeef"}},
			wantType:      bchain.CALL,
			wantContracts: []contractEvent{{addr: "TFFAMQLZybALaLb4uxHA9RBE7pxhUAjF3U", destroyed: true}},
			// a missing beneficiary is not a shape java-tron emits, but the
			// destroyed contract must stay indexed even then
			wantTransferList: []transferEvent{
				{typ: bchain.SELFDESTRUCT, from: "TFFAMQLZybALaLb4uxHA9RBE7pxhUAjF3U", to: ""},
			},
		},

		{
			// an ephemeral (MEV-style) contract created, used and selfdestructed
			// within one call must produce both registry events, creation first -
			// storeContractInfo merges the destruction into the same-batch creation
			name: "Ephemeral contract created and destroyed in one call",
			infos: []tronTxInfo{
				{
					ID:              "ephemeral1",
					ContractAddress: "41734c2f23ab41c52308d1206c4eb5fe8e124e6898",
					InternalTransactions: []tronInternalTransaction{
						{
							CallerAddress:     "41734c2f23ab41c52308d1206c4eb5fe8e124e6898",
							TransferToAddress: "41ed56e617db5eab11b61a9eaefc98c77a6798d257",
							Note:              "637265617465", // create
						},
						{
							CallerAddress:     "41ed56e617db5eab11b61a9eaefc98c77a6798d257",
							TransferToAddress: "41734c2f23ab41c52308d1206c4eb5fe8e124e6898",
							Note:              "73756963696465", // suicide - sweeps remaining balance
							CallValueInfo: []tronCallValueInfo{
								{CallValue: 5},
							},
						},
					},
					Receipt: tronReceipt{Result: "SUCCESS"},
				},
			},
			txs:      []bchain.RpcTransaction{{Hash: "0xephemeral1", To: "0x734c2f23ab41c52308d1206c4eb5fe8e124e6898"}},
			wantType: bchain.CALL,
			wantContracts: []contractEvent{
				{addr: "TXc9FMgWcKK7zGApKj9rArxDb49QkJZWXn"},
				{addr: "TXc9FMgWcKK7zGApKj9rArxDb49QkJZWXn", destroyed: true},
			},
			wantTransferList: []transferEvent{
				{typ: bchain.CREATE, from: "TLUqyV9rGYXZ2E8kXe6J3P1rvYV1Au1Goe", to: "TXc9FMgWcKK7zGApKj9rArxDb49QkJZWXn"},
				{typ: bchain.SELFDESTRUCT, from: "TXc9FMgWcKK7zGApKj9rArxDb49QkJZWXn", to: "TLUqyV9rGYXZ2E8kXe6J3P1rvYV1Au1Goe", value: 5},
			},
		},

		{
			// every destroyed contract must be registered, not just the first one
			name: "Multiple ephemeral contracts register all creations and destructions",
			infos: []tronTxInfo{
				{
					ID:              "ephemeral2",
					ContractAddress: "41734c2f23ab41c52308d1206c4eb5fe8e124e6898",
					InternalTransactions: []tronInternalTransaction{
						{
							CallerAddress:     "41734c2f23ab41c52308d1206c4eb5fe8e124e6898",
							TransferToAddress: "41ed56e617db5eab11b61a9eaefc98c77a6798d257",
							Note:              "637265617465", // create
						},
						{
							CallerAddress: "41ed56e617db5eab11b61a9eaefc98c77a6798d257",
							Note:          "73756963696465", // suicide
						},
						{
							CallerAddress:     "41734c2f23ab41c52308d1206c4eb5fe8e124e6898",
							TransferToAddress: "41da727d310b98700af4cec797e43991899668d6f3",
							Note:              "637265617465", // create
						},
						{
							CallerAddress: "41da727d310b98700af4cec797e43991899668d6f3",
							Note:          "73756963696465", // suicide
						},
					},
					Receipt: tronReceipt{Result: "SUCCESS"},
				},
			},
			txs:      []bchain.RpcTransaction{{Hash: "0xephemeral2", To: "0x734c2f23ab41c52308d1206c4eb5fe8e124e6898"}},
			wantType: bchain.CALL,
			wantContracts: []contractEvent{
				{addr: "TXc9FMgWcKK7zGApKj9rArxDb49QkJZWXn"},
				{addr: "TXc9FMgWcKK7zGApKj9rArxDb49QkJZWXn", destroyed: true},
				{addr: "TVtFTiSQmeMkdpusjefUcPcEeTPtqnhz3D"},
				{addr: "TVtFTiSQmeMkdpusjefUcPcEeTPtqnhz3D", destroyed: true},
			},
			wantTransferList: []transferEvent{
				{typ: bchain.CREATE, from: "TLUqyV9rGYXZ2E8kXe6J3P1rvYV1Au1Goe", to: "TXc9FMgWcKK7zGApKj9rArxDb49QkJZWXn"},
				{typ: bchain.SELFDESTRUCT, from: "TXc9FMgWcKK7zGApKj9rArxDb49QkJZWXn", to: ""},
				{typ: bchain.CREATE, from: "TLUqyV9rGYXZ2E8kXe6J3P1rvYV1Au1Goe", to: "TVtFTiSQmeMkdpusjefUcPcEeTPtqnhz3D"},
				{typ: bchain.SELFDESTRUCT, from: "TVtFTiSQmeMkdpusjefUcPcEeTPtqnhz3D", to: ""},
			},
		},

		{
			// java-tron rejects single frames inside transactions that succeed
			// (a nested call reverted); the tx must not carry an error
			name: "Rejected internal call in a successful tx sets no error",
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
			txs:         []bchain.RpcTransaction{{Hash: "0xfail01"}},
			wantType:    bchain.CALL,
			wantDataErr: "",
		},

		{
			// a failed transaction rejects its internal transactions; their call
			// values did not move and must not be booked as transfers
			name: "Rejected internal transfers are not booked",
			infos: []tronTxInfo{
				{
					ID:              "fail02",
					ContractAddress: "4139dd12a54e2bab7c82aa14a1e158b34263d2d510",
					InternalTransactions: []tronInternalTransaction{
						{
							CallerAddress:     "41734c2f23ab41c52308d1206c4eb5fe8e124e6898",
							TransferToAddress: "41da727d310b98700af4cec797e43991899668d6f3",
							Note:              "63616c6c", // call
							Rejected:          true,
							CallValueInfo: []tronCallValueInfo{
								{CallValue: 457584},
							},
						},
						{
							CallerAddress:     "41da727d310b98700af4cec797e43991899668d6f3",
							TransferToAddress: "41734c2f23ab41c52308d1206c4eb5fe8e124e6898",
							Note:              "63616c6c", // call
							Rejected:          true,
							CallValueInfo: []tronCallValueInfo{
								{CallValue: 457584},
							},
						},
					},
					Receipt: tronReceipt{Result: "OUT_OF_ENERGY"},
				},
			},
			txs:           []bchain.RpcTransaction{{Hash: "0xfail02", To: "0x39dd12a54e2bab7c82aa14a1e158b34263d2d510"}},
			wantType:      bchain.CALL,
			wantTransfers: 0,
			wantDataErr:   "OUT_OF_ENERGY",
		},

		{
			// rejected frames deployed and destroyed nothing - no registry entries
			name: "Rejected create and suicide register nothing",
			infos: []tronTxInfo{
				{
					ID: "fail03",
					InternalTransactions: []tronInternalTransaction{
						{
							CallerAddress:     "41734c2f23ab41c52308d1206c4eb5fe8e124e6898",
							TransferToAddress: "41ed56e617db5eab11b61a9eaefc98c77a6798d257",
							Note:              "637265617465", // create
							Rejected:          true,
						},
						{
							CallerAddress: "41ed56e617db5eab11b61a9eaefc98c77a6798d257",
							Note:          "73756963696465", // suicide
							Rejected:      true,
						},
					},
					Receipt: tronReceipt{Result: "REVERT"},
				},
			},
			txs:         []bchain.RpcTransaction{{Hash: "0xfail03", To: "0x734c2f23ab41c52308d1206c4eb5fe8e124e6898"}},
			wantType:    bchain.CALL,
			wantDataErr: "REVERT",
		},

		{
			// featured internal txs (node ran with vm.saveFeaturedInternalTx) put
			// the staked/delegated amount into callValue although no TRX moves -
			// they must not be booked as transfers
			name: "Featured internal transaction is ignored",
			infos: []tronTxInfo{
				{
					ID: "featured1",
					InternalTransactions: []tronInternalTransaction{
						{
							CallerAddress:     "41c64e69acde1c7b16c2a3efcdbbdaa96c3644c2b3",
							TransferToAddress: "41c9b586fec130cea232371d7ceecde6ad0d3c2991",
							Note:              "64656c65676174655265736f757263654f66456e65726779", // delegateResourceOfEnergy
							CallValueInfo: []tronCallValueInfo{
								{CallValue: 1255110000000},
							},
						},
					},
					Receipt: tronReceipt{Result: "SUCCESS"},
				},
			},
			txs:      []bchain.RpcTransaction{{Hash: "0xfeatured1", To: "0x39dd12a54e2bab7c82aa14a1e158b34263d2d510"}},
			wantType: bchain.CALL,
		},

		{
			// callValueInfo with a tokenId is a TRC-10 transfer, not TRX
			name: "TRC-10 call frame books no transfer",
			infos: []tronTxInfo{
				{
					ID: "trc10call",
					InternalTransactions: []tronInternalTransaction{
						{
							CallerAddress:     "41734c2f23ab41c52308d1206c4eb5fe8e124e6898",
							TransferToAddress: "41da727d310b98700af4cec797e43991899668d6f3",
							Note:              "63616c6c", // call
							CallValueInfo: []tronCallValueInfo{
								{},
								{CallValue: 57753367, TokenID: "1002000"},
							},
						},
					},
					Receipt: tronReceipt{Result: "SUCCESS"},
				},
			},
			txs:      []bchain.RpcTransaction{{Hash: "0xtrc10call", To: "0x39dd12a54e2bab7c82aa14a1e158b34263d2d510"}},
			wantType: bchain.CALL,
		},

		{
			// a create frame carrying only TRC-10 value still emits the
			// zero-value transfer that links the child's address history
			name: "TRC-10-only create frame emits zero-value CREATE transfer",
			infos: []tronTxInfo{
				{
					ID: "trc10create",
					InternalTransactions: []tronInternalTransaction{
						{
							CallerAddress:     "41734c2f23ab41c52308d1206c4eb5fe8e124e6898",
							TransferToAddress: "41ed56e617db5eab11b61a9eaefc98c77a6798d257",
							Note:              "637265617465", // create
							CallValueInfo: []tronCallValueInfo{
								{CallValue: 57753367, TokenID: "1002000"},
							},
						},
					},
					Receipt: tronReceipt{Result: "SUCCESS"},
				},
			},
			txs:           []bchain.RpcTransaction{{Hash: "0xtrc10create", To: "0x734c2f23ab41c52308d1206c4eb5fe8e124e6898"}},
			wantType:      bchain.CALL,
			wantContracts: []contractEvent{{addr: "TXc9FMgWcKK7zGApKj9rArxDb49QkJZWXn"}},
			wantTransferList: []transferEvent{
				{typ: bchain.CREATE, from: "TLUqyV9rGYXZ2E8kXe6J3P1rvYV1Au1Goe", to: "TXc9FMgWcKK7zGApKj9rArxDb49QkJZWXn"},
			},
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

			require.Equal(t, tt.wantType, d.Type)
			require.Equal(t, tt.wantContract, d.Contract)

			if tt.wantTransferList != nil {
				require.Len(t, d.Transfers, len(tt.wantTransferList))
				for i, want := range tt.wantTransferList {
					tr := d.Transfers[i]
					require.Equal(t, want.typ, tr.Type)
					require.Equal(t, want.from, tr.From)
					require.Equal(t, want.to, tr.To)
					require.Equal(t, want.value, tr.Value.Int64())
				}
			} else {
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
			}

			require.Len(t, contracts, len(tt.wantContracts))
			for i, want := range tt.wantContracts {
				require.Equal(t, want.addr, contracts[i].Contract)
				if want.destroyed {
					require.Equal(t, uint32(12345), contracts[i].DestructedInBlock)
					require.Equal(t, uint32(0), contracts[i].CreatedInBlock)
				} else {
					require.Equal(t, uint32(12345), contracts[i].CreatedInBlock)
					require.Equal(t, uint32(0), contracts[i].DestructedInBlock)
					require.Equal(t, bchain.UnhandledTokenStandard, contracts[i].Standard)
				}
			}

			require.Equal(t, tt.wantDataErr, d.Error)
		})
	}
}
