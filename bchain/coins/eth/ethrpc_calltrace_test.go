package eth

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/trezor/blockbook/bchain"
)

// These tests document and pin the rules processCallTrace uses to flatten a callTracer frame
// tree into the simplified EthereumInternalTransfer list. Only frames that represent an actual
// movement of native value (or a contract lifecycle event) become a transfer record:
//
//   - CREATE / CREATE2  -> always emitted (Type CREATE), and the new contract is recorded.
//   - SELFDESTRUCT      -> always emitted (Type SELFDESTRUCT), and the destroyed contract is recorded.
//   - DELEGATECALL      -> never emitted. Since geth v1.11 the bor/geth callTracer reports the
//     inherited msg.value on DELEGATECALL frames (https://github.com/ethereum/go-ethereum/issues/26726),
//     but a DELEGATECALL runs another contract's code in the caller's own context and moves no
//     value, so emitting it would double-count the value already accounted for by the parent CALL.
//     Its children are still traversed, so a genuine value-bearing CALL nested under a DELEGATECALL
//     is still captured.
//   - any other frame (CALL, and by fall-through CALLCODE / STATICCALL or any unrecognised type)
//     -> emitted as Type CALL only when its value field decodes cleanly AND (value > 0 ||
//     ProcessZeroInternalTransactions). STATICCALL cannot carry value and CALLCODE is effectively
//     extinct, so neither is special-cased; a value-bearing CALLCODE would, like DELEGATECALL, be
//     over-counted if one ever appeared.
//
// A transaction whose only sub-frames are DELEGATECALLs (inherited value) and/or zero-value CALLs
// legitimately produces no transfers: its single native value movement is the top-level call
// value, exposed as the regular transaction value rather than as an internal transfer.
//
// Separately, processCallTrace runs only when a block's internal data is fetched
// (getInternalDataForBlock, during sync or the internal-data refetch routine); Blockbook does not
// automatically re-trace an already-synced block that completed without an internal-data error.
// So a transaction that genuinely contains a value-bearing internal CALL but shows no internal
// transfers in the API was most likely indexed before internal-transaction data was captured for
// its block range — only a full resync repopulates it (there is no targeted historical backfill).
// TestProcessCallTrace_RealWMATICWithdraw pins that the extraction itself is correct, so such data
// is missing only from the stored index, not lost from the chain.

// newCallTraceRPC builds an EthereumRPC suitable for exercising processCallTrace directly.
func newCallTraceRPC(processZero bool) *EthereumRPC {
	return &EthereumRPC{
		ChainConfig: &Configuration{
			ProcessInternalTransactions:     true,
			ProcessZeroInternalTransactions: processZero,
		},
	}
}

// runCallTrace mirrors how getInternalDataForBlock feeds the trace: the top-level frame is
// the transaction itself and is not turned into an internal transfer, so only its child
// frames (top.Calls) are processed through processCallTrace.
func runCallTrace(b *EthereumRPC, top *rpcCallTrace, blockHeight uint32) (*bchain.EthereumInternalData, []bchain.ContractInfo) {
	d := &bchain.EthereumInternalData{}
	contracts := make([]bchain.ContractInfo, 0)
	for i := range top.Calls {
		contracts = b.processCallTrace(&top.Calls[i], d, contracts, blockHeight)
	}
	return d, contracts
}

func TestProcessCallTrace_TransferRules(t *testing.T) {
	const height = uint32(123)

	tests := []struct {
		name              string
		processZero       bool
		top               *rpcCallTrace
		wantTransfers     []bchain.EthereumInternalTransfer
		wantError         string
		wantContractAddrs []string
	}{
		{
			// A genuine value-bearing CALL produces one internal transfer.
			name: "CALL with value is recorded",
			top: &rpcCallTrace{
				Type: "CALL", From: "0xowner", To: "0xcontract",
				Calls: []rpcCallTrace{
					{Type: "CALL", From: "0xcontract", To: "0xrecipient", Value: "0x64"},
				},
			},
			wantTransfers: []bchain.EthereumInternalTransfer{
				{Type: bchain.CALL, From: "0xcontract", To: "0xrecipient", Value: *big.NewInt(100)},
			},
		},
		{
			// The only nested frame is a DELEGATECALL carrying the inherited msg.value (the
			// bor/geth callTracer reports it since geth v1.11). No value actually moves, so
			// nothing is recorded and the transaction ends up with no internal transactions.
			name: "DELEGATECALL with inherited value is ignored",
			top: &rpcCallTrace{
				Type: "CALL", From: "0xowner", To: "0xproxy",
				Calls: []rpcCallTrace{
					{Type: "DELEGATECALL", From: "0xproxy", To: "0ximpl", Value: "0xde0b6b3a7640000"},
				},
			},
			wantTransfers: nil,
		},
		{
			// DELEGATECALL itself is skipped, but its children are still traversed, so a real
			// value-bearing CALL nested inside the delegated code is captured.
			name: "value CALL nested under DELEGATECALL is recorded",
			top: &rpcCallTrace{
				Type: "CALL", From: "0xowner", To: "0xproxy",
				Calls: []rpcCallTrace{
					{
						Type: "DELEGATECALL", From: "0xproxy", To: "0ximpl", Value: "0xde0b6b3a7640000",
						Calls: []rpcCallTrace{
							{Type: "CALL", From: "0xproxy", To: "0xrecipient", Value: "0x2a"},
						},
					},
				},
			},
			wantTransfers: []bchain.EthereumInternalTransfer{
				{Type: bchain.CALL, From: "0xproxy", To: "0xrecipient", Value: *big.NewInt(42)},
			},
		},
		{
			name: "zero-value CALL is skipped by default",
			top: &rpcCallTrace{
				Type: "CALL", From: "0xowner", To: "0xcontract",
				Calls: []rpcCallTrace{
					{Type: "CALL", From: "0xcontract", To: "0xother", Value: "0x0"},
				},
			},
			wantTransfers: nil,
		},
		{
			name:        "zero-value CALL is recorded when ProcessZeroInternalTransactions is set",
			processZero: true,
			top: &rpcCallTrace{
				Type: "CALL", From: "0xowner", To: "0xcontract",
				Calls: []rpcCallTrace{
					{Type: "CALL", From: "0xcontract", To: "0xother", Value: "0x0"},
				},
			},
			wantTransfers: []bchain.EthereumInternalTransfer{
				{Type: bchain.CALL, From: "0xcontract", To: "0xother", Value: *big.NewInt(0)},
			},
		},
		{
			// STATICCALL is not special-cased in processCallTrace; it falls into the same
			// catch-all branch as CALL and is skipped here purely because it carries no value.
			name: "STATICCALL (no value) is skipped",
			top: &rpcCallTrace{
				Type: "CALL", From: "0xowner", To: "0xcontract",
				Calls: []rpcCallTrace{
					{Type: "STATICCALL", From: "0xcontract", To: "0xview", Value: "0x0"},
				},
			},
			wantTransfers: nil,
		},
		{
			// CALLCODE is not special-cased: it falls through to the value-gated branch and is
			// treated exactly like CALL. Locks in that nobody adds a DELEGATECALL-style skip for
			// it by mistake. (CALLCODE is effectively extinct, so this is documentation of intent.)
			name: "CALLCODE with value is treated like CALL",
			top: &rpcCallTrace{
				Type: "CALL", From: "0xowner", To: "0xcontract",
				Calls: []rpcCallTrace{
					{Type: "CALLCODE", From: "0xcontract", To: "0xother", Value: "0x64"},
				},
			},
			wantTransfers: []bchain.EthereumInternalTransfer{
				{Type: bchain.CALL, From: "0xcontract", To: "0xother", Value: *big.NewInt(100)},
			},
		},
		{
			// DELEGATECALL siblings must be skipped without disturbing the index/order of the
			// value-bearing CALL siblings recorded around them.
			name: "DELEGATECALL sibling is skipped, surrounding CALLs keep order",
			top: &rpcCallTrace{
				Type: "CALL", From: "0xowner", To: "0xcontract",
				Calls: []rpcCallTrace{
					{Type: "CALL", From: "0xcontract", To: "0xfirst", Value: "0x1"},
					{Type: "DELEGATECALL", From: "0xcontract", To: "0ximpl", Value: "0xde0b6b3a7640000"},
					{Type: "CALL", From: "0xcontract", To: "0xsecond", Value: "0x2"},
				},
			},
			wantTransfers: []bchain.EthereumInternalTransfer{
				{Type: bchain.CALL, From: "0xcontract", To: "0xfirst", Value: *big.NewInt(1)},
				{Type: bchain.CALL, From: "0xcontract", To: "0xsecond", Value: *big.NewInt(2)},
			},
		},
		{
			name: "CREATE records a transfer and the new contract",
			top: &rpcCallTrace{
				Type: "CALL", From: "0xowner", To: "0xfactory",
				Calls: []rpcCallTrace{
					{Type: "CREATE", From: "0xfactory", To: "0xnewcontract", Value: "0x0"},
				},
			},
			wantTransfers: []bchain.EthereumInternalTransfer{
				{Type: bchain.CREATE, From: "0xfactory", To: "0xnewcontract", Value: *big.NewInt(0)},
			},
			wantContractAddrs: []string{"0xnewcontract"},
		},
		{
			name: "CREATE2 with value records a transfer and the new contract",
			top: &rpcCallTrace{
				Type: "CALL", From: "0xowner", To: "0xfactory",
				Calls: []rpcCallTrace{
					{Type: "CREATE2", From: "0xfactory", To: "0xnewcontract", Value: "0x64"},
				},
			},
			wantTransfers: []bchain.EthereumInternalTransfer{
				{Type: bchain.CREATE, From: "0xfactory", To: "0xnewcontract", Value: *big.NewInt(100)},
			},
			wantContractAddrs: []string{"0xnewcontract"},
		},
		{
			name: "SELFDESTRUCT records a transfer and the destroyed contract",
			top: &rpcCallTrace{
				Type: "CALL", From: "0xowner", To: "0xcontract",
				Calls: []rpcCallTrace{
					{Type: "SELFDESTRUCT", From: "0xdoomed", To: "0xbeneficiary", Value: "0x7"},
				},
			},
			wantTransfers: []bchain.EthereumInternalTransfer{
				{Type: bchain.SELFDESTRUCT, From: "0xdoomed", To: "0xbeneficiary", Value: *big.NewInt(7)},
			},
			wantContractAddrs: []string{"0xdoomed"},
		},
		{
			// Value can sit several frames deep, under zero-value CALLs; the recursion must reach it.
			name: "deeply nested value CALL is recorded",
			top: &rpcCallTrace{
				Type: "CALL", From: "0xowner", To: "0xa",
				Calls: []rpcCallTrace{
					{
						Type: "CALL", From: "0xa", To: "0xb", Value: "0x0",
						Calls: []rpcCallTrace{
							{
								Type: "CALL", From: "0xb", To: "0xc", Value: "0x0",
								Calls: []rpcCallTrace{
									{Type: "CALL", From: "0xc", To: "0xd", Value: "0x9"},
								},
							},
						},
					},
				},
			},
			wantTransfers: []bchain.EthereumInternalTransfer{
				{Type: bchain.CALL, From: "0xc", To: "0xd", Value: *big.NewInt(9)},
			},
		},
		{
			// An empty/omitted value string must not panic and must not produce a transfer.
			name: "empty value string is skipped without panicking",
			top: &rpcCallTrace{
				Type: "CALL", From: "0xowner", To: "0xcontract",
				Calls: []rpcCallTrace{
					{Type: "CALL", From: "0xcontract", To: "0xother", Value: ""},
				},
			},
			wantTransfers: nil,
		},
		{
			// The emit branch is gated on err == nil: an undecodable value must be dropped even
			// in zero-processing mode, rather than recorded as a bogus zero-value transfer.
			name:        "undecodable value is skipped even with ProcessZeroInternalTransactions",
			processZero: true,
			top: &rpcCallTrace{
				Type: "CALL", From: "0xowner", To: "0xcontract",
				Calls: []rpcCallTrace{
					{Type: "CALL", From: "0xcontract", To: "0xother", Value: ""},
				},
			},
			wantTransfers: nil,
		},
		{
			name: "error on a child frame propagates to internal data",
			top: &rpcCallTrace{
				Type: "CALL", From: "0xowner", To: "0xcontract",
				Calls: []rpcCallTrace{
					{Type: "CALL", From: "0xcontract", To: "0xrecipient", Value: "0x64", Error: "execution reverted"},
				},
			},
			wantTransfers: []bchain.EthereumInternalTransfer{
				{Type: bchain.CALL, From: "0xcontract", To: "0xrecipient", Value: *big.NewInt(100)},
			},
			wantError: "execution reverted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newCallTraceRPC(tt.processZero)
			d, contracts := runCallTrace(b, tt.top, height)

			if len(d.Transfers) != len(tt.wantTransfers) {
				t.Fatalf("transfers = %d (%+v), want %d (%+v)", len(d.Transfers), d.Transfers, len(tt.wantTransfers), tt.wantTransfers)
			}
			for i, want := range tt.wantTransfers {
				got := d.Transfers[i]
				if got.Type != want.Type || got.From != want.From || got.To != want.To || got.Value.Cmp(&want.Value) != 0 {
					t.Errorf("transfer[%d] = %+v, want %+v", i, got, want)
				}
			}
			if d.Error != tt.wantError {
				t.Errorf("error = %q, want %q", d.Error, tt.wantError)
			}

			gotAddrs := make([]string, len(contracts))
			for i, c := range contracts {
				gotAddrs[i] = c.Contract
			}
			if len(gotAddrs) != len(tt.wantContractAddrs) {
				t.Fatalf("contracts = %v, want %v", gotAddrs, tt.wantContractAddrs)
			}
			for i, want := range tt.wantContractAddrs {
				if gotAddrs[i] != want {
					t.Errorf("contract[%d] = %q, want %q", i, gotAddrs[i], want)
				}
			}
		})
	}
}

// TestProcessCallTrace_ContractLifecycleBlockHeight verifies that the created/destroyed
// block height is stamped on the contract records emitted for CREATE and SELFDESTRUCT frames.
func TestProcessCallTrace_ContractLifecycleBlockHeight(t *testing.T) {
	const height = uint32(987654)
	b := newCallTraceRPC(false)
	top := &rpcCallTrace{
		Type: "CALL", From: "0xowner", To: "0xfactory",
		Calls: []rpcCallTrace{
			{Type: "CREATE", From: "0xfactory", To: "0xcreated", Value: "0x0"},
			{Type: "SELFDESTRUCT", From: "0xdestroyed", To: "0xbeneficiary", Value: "0x0"},
		},
	}
	_, contracts := runCallTrace(b, top, height)
	if len(contracts) != 2 {
		t.Fatalf("contracts = %d, want 2", len(contracts))
	}
	// A CREATE record is stamped with CreatedInBlock and the Unhandled token standard/type...
	if contracts[0].Contract != "0xcreated" || contracts[0].CreatedInBlock != height || contracts[0].DestructedInBlock != 0 {
		t.Errorf("created contract = %+v, want {Contract:0xcreated CreatedInBlock:%d}", contracts[0], height)
	}
	if contracts[0].Standard != bchain.UnhandledTokenStandard || contracts[0].Type != bchain.UnhandledTokenStandard {
		t.Errorf("created contract standard/type = %q/%q, want %q/%q", contracts[0].Standard, contracts[0].Type, bchain.UnhandledTokenStandard, bchain.UnhandledTokenStandard)
	}
	// ...whereas a SELFDESTRUCT record carries only DestructedInBlock, leaving standard/type empty.
	if contracts[1].Contract != "0xdestroyed" || contracts[1].DestructedInBlock != height || contracts[1].CreatedInBlock != 0 {
		t.Errorf("destroyed contract = %+v, want {Contract:0xdestroyed DestructedInBlock:%d}", contracts[1], height)
	}
	if contracts[1].Standard != "" || contracts[1].Type != "" {
		t.Errorf("destroyed contract standard/type = %q/%q, want empty", contracts[1].Standard, contracts[1].Type)
	}
}

// TestProcessCallTrace_RealWMATICWithdraw feeds the verbatim callTracer frame that a Polygon
// (bor) archive node returns for tx
// 0x23d7acf0d6a0de63b3cdd6e4bc53c60d4a48f19b797d57f00c267a3e0a6f25cc (a WMATIC withdraw, block
// 15526560), which was reported as "missing" its internal transaction, and asserts the inner
// value-bearing CALL is extracted as one internal transfer. See the file header for why the API
// nonetheless returns null for it (pre-capture indexed data, not a flattening bug).
func TestProcessCallTrace_RealWMATICWithdraw(t *testing.T) {
	const trace = `{
		"from": "0x749da3b1ca18f020f1454a9c813f62b5aa823264",
		"to": "0x0d500b1d8e8ef31e21c99d1db9a6444d3adf1270",
		"type": "CALL",
		"value": "0x0",
		"input": "0x2e1a7d4d00000000000000000000000000000000000000000000000004d15bb279a6b725",
		"calls": [
			{
				"from": "0x0d500b1d8e8ef31e21c99d1db9a6444d3adf1270",
				"to": "0x749da3b1ca18f020f1454a9c813f62b5aa823264",
				"type": "CALL",
				"input": "0x",
				"value": "0x4d15bb279a6b725"
			}
		]
	}`
	var top rpcCallTrace
	if err := json.Unmarshal([]byte(trace), &top); err != nil {
		t.Fatalf("unmarshal trace: %v", err)
	}
	b := newCallTraceRPC(false)
	d, _ := runCallTrace(b, &top, 15526560)

	want := big.NewInt(0)
	want.SetString("347159468387514149", 10) // 0x4d15bb279a6b725
	if len(d.Transfers) != 1 {
		t.Fatalf("transfers = %d (%+v), want 1", len(d.Transfers), d.Transfers)
	}
	got := d.Transfers[0]
	if got.Type != bchain.CALL ||
		got.From != "0x0d500b1d8e8ef31e21c99d1db9a6444d3adf1270" ||
		got.To != "0x749da3b1ca18f020f1454a9c813f62b5aa823264" ||
		got.Value.Cmp(want) != 0 {
		t.Errorf("transfer = %+v, want CALL 0x0d50…→0x749d… value %s", got, want)
	}
}
