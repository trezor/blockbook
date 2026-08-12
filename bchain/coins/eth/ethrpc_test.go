//go:build unittest

package eth

import (
	"context"
	"encoding/hex"
	"encoding/json"
	stdErrors "errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/trezor/blockbook/bchain"
	"golang.org/x/crypto/sha3"
)

func keccak256Selector(sig string) string {
	h := sha3.NewLegacyKeccak256()
	h.Write([]byte(sig))
	return "0x" + hex.EncodeToString(h.Sum(nil)[:4])
}

// TestENSFunctionSelectors verifies that all ENS function selector constants
// match the keccak256 hash of their documented Solidity signatures. A
// mismatch means eth_call will revert against any real backend.
func TestENSFunctionSelectors(t *testing.T) {
	tests := []struct {
		constant string
		got      string
		sig      string
	}{
		{"ENSResolverFunctionSelector", ENSResolverFunctionSelector, "resolver(bytes32)"},
		{"ENSAddrFunctionSelector", ENSAddrFunctionSelector, "addr(bytes32)"},
		{"ENSExpirationFunctionSelector", ENSExpirationFunctionSelector, "nameExpires(uint256)"},
	}
	for _, tc := range tests {
		want := keccak256Selector(tc.sig)
		if tc.got != want {
			t.Errorf("%s = %q, want keccak256(%q)[:4] = %q", tc.constant, tc.got, tc.sig, want)
		}
	}
}

// recoveryStub is a method-aware bchain.EVMRPCClient fake for exercising the pruned-index
// recovery path in recoverMinedTransaction / GetTransaction. It serves canned JSON per
// JSON-RPC method, can force the positional lookup to error (to drive the block-body
// fallback), and records call counts plus arguments so tests can assert exactly which RPCs
// were made (single receipt fetch, block body only when needed).
type recoveryStub struct {
	byHashJSON   string // eth_getTransactionByHash result; "" leaves the target zero (null)
	receiptJSON  string // eth_getTransactionReceipt result ("null" => unknown tx)
	byIndexJSON  string // eth_getTransactionByBlockHashAndIndex result
	byIndexErr   error  // when set, the positional lookup errors (drives the fallback)
	blockJSON    string // eth_getBlockByHash raw result (header fields and/or transactions)
	calls        map[string]int
	byIndexArgs  []interface{}
	blockFullTxs []bool // fullTxs arg recorded per eth_getBlockByHash call
}

func newRecoveryStub() *recoveryStub { return &recoveryStub{calls: map[string]int{}} }

func (s *recoveryStub) EthSubscribe(context.Context, interface{}, ...interface{}) (bchain.EVMClientSubscription, error) {
	return nil, stdErrors.New("not implemented")
}

func (s *recoveryStub) Close() {}

func (s *recoveryStub) CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error {
	s.calls[method]++
	switch method {
	case "eth_getTransactionByHash":
		if s.byHashJSON == "" {
			return nil // null result: leaves the caller's zero-value tx untouched
		}
		return json.Unmarshal([]byte(s.byHashJSON), result)
	case "eth_getTransactionReceipt":
		return json.Unmarshal([]byte(s.receiptJSON), result)
	case "eth_getTransactionByBlockHashAndIndex":
		s.byIndexArgs = args
		if s.byIndexErr != nil {
			return s.byIndexErr
		}
		return json.Unmarshal([]byte(s.byIndexJSON), result)
	case "eth_getBlockByHash":
		if len(args) >= 2 {
			if full, ok := args[1].(bool); ok {
				s.blockFullTxs = append(s.blockFullTxs, full)
			}
		}
		if p, ok := result.(*json.RawMessage); ok {
			*p = json.RawMessage(s.blockJSON)
		}
		return nil
	default:
		return stdErrors.New("unexpected method: " + method)
	}
}

const (
	recTxid      = "0xd20ea523eee594f82d481b4da6a8c2f1ce1e7fee34cd6369ee0ba6093c1d19bb"
	recBlockHash = "0x4437c2e020a9c940532e2431babb3550cfdd16f498d2c7b5bb6c0f728567d69d"
	recTxIndex   = "0x21"
	recReceipt   = `{"blockHash":"` + recBlockHash + `","transactionIndex":"` + recTxIndex + `","status":"0x1","gasUsed":"0x5208","logs":[]}`
	recTxObject  = `{"blockHash":"` + recBlockHash + `","blockNumber":"0x2b83e54","from":"0xc68eff0a07180ce4b6e490ddb080b6b8f3867024","gas":"0x5208","gasPrice":"0xf4240","hash":"` + recTxid + `","input":"0x","nonce":"0x5","to":"0x22b51ee43ccab63ec03c50794a841c9189d94ed2","transactionIndex":"` + recTxIndex + `","value":"0x2386f26fc10000"}`
	// block header fields (timestamp, baseFeePerGas) plus the tx body, so one JSON serves
	// both the mined-branch header read (fullTxs=false) and the block-body fallback scan.
	recBlockJSON = `{"timestamp":"0x6819a2e0","baseFeePerGas":"0x5f5e100","transactions":[` + recTxObject + `]}`
)

// Fast path: a pruned-but-retained tx is recovered via receipt +
// eth_getTransactionByBlockHashAndIndex, the block body is never fetched, the receipt is
// fetched exactly once and returned for reuse, and the index lookup is addressed by the
// receipt's block hash and index.
func TestRecoverMinedTransaction_UsesByBlockHashAndIndex(t *testing.T) {
	stub := newRecoveryStub()
	stub.receiptJSON, stub.byIndexJSON = recReceipt, recTxObject
	b := &EthereumRPC{RPC: stub, Timeout: time.Second}

	tx, receipt := b.recoverMinedTransaction(recTxid)

	if tx == nil || tx.Hash != recTxid {
		t.Fatalf("recovered tx = %+v, want hash %s", tx, recTxid)
	}
	if tx.BlockNumber == "" {
		t.Error("recovered tx has empty BlockNumber; would be misrouted as a mempool tx")
	}
	if receipt == nil || receipt.Status != "0x1" || receipt.GasUsed != "0x5208" {
		t.Fatalf("reused receipt = %+v, want Status 0x1 / GasUsed 0x5208", receipt)
	}
	if got := stub.calls["eth_getBlockByHash"]; got != 0 {
		t.Errorf("eth_getBlockByHash called %d times, want 0 (no block-body fetch on fast path)", got)
	}
	if got := stub.calls["eth_getTransactionReceipt"]; got != 1 {
		t.Errorf("eth_getTransactionReceipt called %d times, want exactly 1", got)
	}
	if got := stub.calls["eth_getTransactionByBlockHashAndIndex"]; got != 1 {
		t.Errorf("eth_getTransactionByBlockHashAndIndex called %d times, want 1", got)
	}
	if len(stub.byIndexArgs) != 2 {
		t.Fatalf("eth_getTransactionByBlockHashAndIndex got %d args, want 2", len(stub.byIndexArgs))
	}
	if bh, ok := stub.byIndexArgs[0].(ethcommon.Hash); !ok || bh != ethcommon.HexToHash(recBlockHash) {
		t.Errorf("byIndex arg[0] = %v, want block hash %s", stub.byIndexArgs[0], recBlockHash)
	}
	if idx, ok := stub.byIndexArgs[1].(string); !ok || idx != recTxIndex {
		t.Errorf("byIndex arg[1] = %v, want index %s", stub.byIndexArgs[1], recTxIndex)
	}
}

// A genuinely unknown tx has a null receipt: recovery returns (nil, nil) so the caller
// yields ErrTxNotFound, and neither the index lookup nor the block body is fetched.
func TestRecoverMinedTransaction_UnknownReturnsNil(t *testing.T) {
	stub := newRecoveryStub()
	stub.receiptJSON = "null"
	b := &EthereumRPC{RPC: stub, Timeout: time.Second}

	tx, receipt := b.recoverMinedTransaction(recTxid)

	if tx != nil || receipt != nil {
		t.Errorf("recoverMinedTransaction(unknown) = (%v, %v), want (nil, nil)", tx, receipt)
	}
	if got := stub.calls["eth_getTransactionByBlockHashAndIndex"] + stub.calls["eth_getBlockByHash"]; got != 0 {
		t.Errorf("made %d lookup calls for unknown tx, want 0", got)
	}
}

// Fallback: when eth_getTransactionByBlockHashAndIndex is unavailable (errors), recovery
// falls back to scanning the block body, still recovering the tx and returning the receipt.
func TestRecoverMinedTransaction_FallsBackToBlockBody(t *testing.T) {
	stub := newRecoveryStub()
	stub.receiptJSON = recReceipt
	stub.byIndexErr = stdErrors.New("the method eth_getTransactionByBlockHashAndIndex does not exist")
	stub.blockJSON = recBlockJSON
	b := &EthereumRPC{RPC: stub, Timeout: time.Second}

	tx, receipt := b.recoverMinedTransaction(recTxid)

	if tx == nil || tx.Hash != recTxid {
		t.Fatalf("fallback recovered tx = %+v, want hash %s", tx, recTxid)
	}
	if receipt == nil || receipt.Status != "0x1" {
		t.Fatalf("fallback receipt = %+v, want Status 0x1", receipt)
	}
	if len(stub.blockFullTxs) != 1 || !stub.blockFullTxs[0] {
		t.Errorf("block-body fallback should fetch eth_getBlockByHash once with fullTxs=true, got %v", stub.blockFullTxs)
	}
}

// If both the positional lookup fails and the block body lacks the tx, recovery returns
// (nil, nil).
func TestRecoverMinedTransaction_FallbackMissReturnsNil(t *testing.T) {
	stub := newRecoveryStub()
	stub.receiptJSON = recReceipt
	stub.byIndexErr = stdErrors.New("unsupported")
	stub.blockJSON = `{"timestamp":"0x1","transactions":[]}`
	b := &EthereumRPC{RPC: stub, Timeout: time.Second}

	tx, receipt := b.recoverMinedTransaction(recTxid)

	if tx != nil || receipt != nil {
		t.Errorf("recoverMinedTransaction(fallback miss) = (%v, %v), want (nil, nil)", tx, receipt)
	}
}

// End-to-end GetTransaction on a pruned tx: eth_getTransactionByHash returns null, the tx is
// recovered, and the recovered receipt is reused for EthTxToTx instead of being fetched a
// second time - so eth_getTransactionReceipt is called exactly once and no block body is read.
func TestGetTransaction_RecoveryReusesReceipt(t *testing.T) {
	stub := newRecoveryStub()
	stub.receiptJSON, stub.byIndexJSON, stub.blockJSON = recReceipt, recTxObject, recBlockJSON
	b := &EthereumRPC{
		RPC:        stub,
		Timeout:    time.Second,
		Parser:     NewEthereumParser(1, false),
		bestHeader: stubHeader{n: 45629030}, // > tx block 0x2b83e54, for computeConfirmations
	}

	got, err := b.GetTransaction(recTxid)
	if err != nil {
		t.Fatalf("GetTransaction returned error: %v", err)
	}
	if got == nil || got.Txid != recTxid {
		t.Fatalf("GetTransaction = %+v, want Txid %s", got, recTxid)
	}
	if c := stub.calls["eth_getTransactionReceipt"]; c != 1 {
		t.Errorf("eth_getTransactionReceipt called %d times, want exactly 1 (receipt must be reused, not re-fetched)", c)
	}
	if c := stub.calls["eth_getTransactionByHash"]; c != 1 {
		t.Errorf("eth_getTransactionByHash called %d times, want 1", c)
	}
	// Only the mined-branch header read (fullTxs=false); no block-body fetch.
	for _, full := range stub.blockFullTxs {
		if full {
			t.Errorf("GetTransaction fetched a full block body during recovery, want header-only reads: %v", stub.blockFullTxs)
		}
	}
	if got.Confirmations <= 0 {
		t.Errorf("Confirmations = %d, want > 0", got.Confirmations)
	}
}

// ensCallStub is a minimal bchain.EVMRPCClient fake that serves a single canned
// eth_call result (or forces an error), for exercising CheckENSExpiration's
// result-parsing path without a real backend.
type ensCallStub struct {
	result string // eth_call result string, e.g. an ABI-encoded uint256 word
	err    error  // when set, CallContext returns it (simulates a revert)
}

func (s *ensCallStub) EthSubscribe(context.Context, interface{}, ...interface{}) (bchain.EVMClientSubscription, error) {
	return nil, stdErrors.New("not implemented")
}

func (s *ensCallStub) Close() {}

func (s *ensCallStub) CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error {
	if method != "eth_call" {
		return stdErrors.New("unexpected method: " + method)
	}
	if s.err != nil {
		return s.err
	}
	p, ok := result.(*json.RawMessage)
	if !ok {
		return stdErrors.New("unexpected result type")
	}
	encoded, err := json.Marshal(s.result)
	if err != nil {
		return err
	}
	*p = json.RawMessage(encoded)
	return nil
}

// abiWord left-pads an unsigned integer into a full 32-byte ABI-encoded word,
// mirroring how nameExpires(uint256) is returned on the wire.
func abiWord(v int64) string { return "0x" + fmt.Sprintf("%064x", v) }

// TestCheckENSExpiration_ParsesPaddedResult locks in the result-parsing path of
// CheckENSExpiration — the exact place the historical bug lived. nameExpires
// returns an ABI-encoded uint256 (a 32-byte word left-padded with zeros);
// hexutil.DecodeBig rejects those leading zeros, so the parse must treat the
// word as raw bytes. Without that fix every real lookup silently skips the
// expiration check.
func TestCheckENSExpiration_ParsesPaddedResult(t *testing.T) {
	past := time.Now().Add(-24 * time.Hour).Unix()
	future := time.Now().Add(24 * time.Hour).Unix()

	tests := []struct {
		name        string
		result      string
		callErr     error
		wantExpired bool
	}{
		{"future expiration (registered, valid)", abiWord(future), nil, false},
		{"past expiration (registered, expired)", abiWord(past), nil, true},
		{"zero word (unregistered label)", abiWord(0), nil, false},
		{"empty result", "0x", nil, false},
		{"rpc revert", "", stdErrors.New("execution reverted"), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := &EthereumRPC{
				BaseChain:      &bchain.BaseChain{}, // Testnet defaults to false, so ensContracts resolves
				RPC:            &ensCallStub{result: tc.result, err: tc.callErr},
				Timeout:        time.Second,
				MainNetChainID: MainNet,
			}
			expired, err := b.CheckENSExpiration("vitalik.eth")
			if err != nil {
				t.Fatalf("CheckENSExpiration returned error: %v", err)
			}
			if expired != tc.wantExpired {
				t.Errorf("CheckENSExpiration(%q) expired = %v, want %v", tc.result, expired, tc.wantExpired)
			}
		})
	}
}

// TestNewEthereumRPC_PropagatesEnsReverseOptIn pins the config->parser wiring that both
// gates read: processEventsForBlock (write path) and api.NewWorker (read path) both call
// UseEnsReverseAliases on this parser, so ENS reverse aliasing must stay off unless a chain
// sets enable_ens_reverse_aliases alongside address_aliases.
func TestNewEthereumRPC_PropagatesEnsReverseOptIn(t *testing.T) {
	tests := []struct {
		name   string
		params string
		want   bool
	}{
		{name: "aliases on, opt-in absent", params: `"address_aliases": true`},
		{name: "aliases on, opt-in false", params: `"address_aliases": true, "enable_ens_reverse_aliases": false`},
		{name: "aliases on, opt-in true", params: `"address_aliases": true, "enable_ens_reverse_aliases": true`, want: true},
		{name: "opt-in without address_aliases", params: `"enable_ens_reverse_aliases": true`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// NewEthereumRPC writes this process-wide; keep the test order-independent.
			orig := bchain.ProcessInternalTransactions
			defer func() { bchain.ProcessInternalTransactions = orig }()

			config := json.RawMessage(`{
				"coin_name": "Ethereum",
				"rpc_url": "http://127.0.0.1:8545",
				"rpc_timeout": 25,
				"block_addresses_to_keep": 300,
				` + tt.params + `,
				"averageBlockTimeMs": 12000,
				"mempoolTxTimeoutHours": 48
			}`)

			chain, err := NewEthereumRPC(config, func(bchain.NotificationType) {})
			if err != nil {
				t.Fatalf("NewEthereumRPC: %v", err)
			}
			if got := chain.(*EthereumRPC).Parser.UseEnsReverseAliases(); got != tt.want {
				t.Errorf("UseEnsReverseAliases() = %v, want %v", got, tt.want)
			}
		})
	}
}

// getLogsStub serves one canned eth_getLogs response.
type getLogsStub struct{ logsJSON string }

func (s *getLogsStub) EthSubscribe(context.Context, interface{}, ...interface{}) (bchain.EVMClientSubscription, error) {
	return nil, stdErrors.New("not implemented")
}

func (s *getLogsStub) Close() {}

func (s *getLogsStub) CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error {
	if method != "eth_getLogs" {
		return stdErrors.New("unexpected method: " + method)
	}
	return json.Unmarshal([]byte(s.logsJSON), result)
}

// TestProcessEventsForBlock_EnsReverseGate covers the write-path half of the opt-out: a
// NameRegistered log must yield no AddressAliasRecord (so nothing reaches cfAddressAliases)
// unless the chain opted in, while the tx logs themselves are returned either way.
func TestProcessEventsForBlock_EnsReverseGate(t *testing.T) {
	const txHash = "0x1a2b3c4d5e6f70000000000000000000000000000000000000000000000000ab"
	// The "unraveled" NameRegistered fixture from Test_getEnsRecord.
	logsJSON := `[{
		"transactionHash": "` + txHash + `",
		"address": "0x283Af0B28c62C092C9727F1Ee09c02CA627EB7F5",
		"topics": [
			"0xca6abbe9d7f11422cb6ca7629fbf6fe9efb1c621f71ce8f02b9f2a230097404f",
			"0x40ce2aa8cd9ee9fef4bf3a68abab7fbcceb6bac89370518caf6a602cefe836bd",
			"0x0000000000000000000000002c630b16aa53ae0189880e15c23323688acb607c"
		],
		"data": "0x00000000000000000000000000000000000000000000000000000000000000600000000000000000000000000000000000000000000000000017629245f5a86f0000000000000000000000000000000000000000000000000000000069dbb21d0000000000000000000000000000000000000000000000000000000000000009756e726176656c65640000000000000000000000000000000000000000000000"
	}]`

	tests := []struct {
		name      string
		enableEns bool
		wantEns   []bchain.AddressAliasRecord
	}{
		{name: "gate closed records no ENS label"},
		{
			name:      "gate open records the ENS label",
			enableEns: true,
			wantEns:   []bchain.AddressAliasRecord{{Address: "0x2C630b16Aa53ae0189880e15C23323688acb607c", Name: "unraveled"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewEthereumParser(1, true)
			parser.EnableEnsReverseAliases = tt.enableEns
			b := &EthereumRPC{
				BaseChain: &bchain.BaseChain{},
				Parser:    parser,
				RPC:       &getLogsStub{logsJSON: logsJSON},
				Timeout:   time.Second,
			}

			logs, ens, err := b.processEventsForBlock("0x1")
			if err != nil {
				t.Fatalf("processEventsForBlock: %v", err)
			}
			// The gate must not affect the tx logs themselves.
			if len(logs[txHash]) != 1 {
				t.Errorf("logs[%s] = %d entries, want 1", txHash, len(logs[txHash]))
			}
			if !reflect.DeepEqual(ens, tt.wantEns) {
				t.Errorf("ensRecords = %+v, want %+v", ens, tt.wantEns)
			}
		})
	}
}
