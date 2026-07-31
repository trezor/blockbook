//go:build unittest

package eth

import (
	"context"
	"encoding/hex"
	"encoding/json"
	stdErrors "errors"
	"fmt"
	"strings"
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

// logCapStub is a bchain.EVMRPCClient fake for the eth_getLogs result-cap fallback: it can
// fail eth_getLogs with an arbitrary error and serve canned eth_getBlockReceipts JSON,
// recording per-method call counts and the arguments eth_getBlockReceipts was called with.
type logCapStub struct {
	logsJSON     string // eth_getLogs result when logsErr is nil
	logsErr      error  // when set, eth_getLogs fails with this error
	receiptsJSON string // eth_getBlockReceipts result ("null" => unknown block)
	receiptsErr  error  // when set, eth_getBlockReceipts fails with this error
	calls        map[string]int
	receiptsArgs []interface{}
}

func newLogCapStub() *logCapStub { return &logCapStub{calls: map[string]int{}} }

func (s *logCapStub) EthSubscribe(context.Context, interface{}, ...interface{}) (bchain.EVMClientSubscription, error) {
	return nil, stdErrors.New("not implemented")
}

func (s *logCapStub) Close() {}

func (s *logCapStub) CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error {
	s.calls[method]++
	switch method {
	case "eth_getLogs":
		if s.logsErr != nil {
			return s.logsErr
		}
		return json.Unmarshal([]byte(s.logsJSON), result)
	case "eth_getBlockReceipts":
		s.receiptsArgs = args
		if s.receiptsErr != nil {
			return s.receiptsErr
		}
		return json.Unmarshal([]byte(s.receiptsJSON), result)
	default:
		return stdErrors.New("unexpected method: " + method)
	}
}

// erigonLogCapErr is the verbatim error Erigon >= 3.5.0 returns once a query would exceed
// --rpc.logs.maxresults (default 20000).
var erigonLogCapErr = stdErrors.New("query returns too many logs, narrow your filter: 20000")

const (
	capBlockHash = "0x5baa205d3a15364ff905e9cbb6cfea28c634c0cb48b6a9a9be2abafc6b83e764"
	capTx1       = "0x1111111111111111111111111111111111111111111111111111111111111111"
	capTx2       = "0x2222222222222222222222222222222222222222222222222222222222222222"
	// two receipts, three logs; the last log omits transactionHash to exercise the
	// attribution fallback to the receipt's own hash
	capReceiptsJSON = `[
		{"transactionHash":"` + capTx1 + `","logs":[
			{"address":"0xaaaa000000000000000000000000000000000001","topics":["0xaa"],"data":"0x01","transactionHash":"` + capTx1 + `"},
			{"address":"0xaaaa000000000000000000000000000000000002","topics":["0xbb"],"data":"0x02","transactionHash":"` + capTx1 + `"}]},
		{"transactionHash":"` + capTx2 + `","logs":[
			{"address":"0xaaaa000000000000000000000000000000000003","topics":["0xcc"],"data":"0x03"}]}]`
)

// A block whose log count exceeds the backend cap must still be indexable: eth_getLogs
// fails with the cap error (retrying it can never succeed, so sync would stall forever),
// and the logs are recovered from eth_getBlockReceipts with per-transaction attribution
// and ordering preserved.
func TestProcessEventsForBlock_FallsBackToBlockReceiptsOnLogCap(t *testing.T) {
	stub := newLogCapStub()
	stub.logsErr, stub.receiptsJSON = erigonLogCapErr, capReceiptsJSON
	b := &EthereumRPC{RPC: stub, Timeout: time.Second}

	logs, ens, err := b.processEventsForBlock("0x1875fd0", capBlockHash, 2)
	if err != nil {
		t.Fatalf("processEventsForBlock returned error: %v", err)
	}
	if len(ens) != 0 {
		t.Errorf("ens records = %v, want none", ens)
	}
	if len(logs) != 2 {
		t.Fatalf("logs map has %d transactions, want 2: %+v", len(logs), logs)
	}
	if got := len(logs[capTx1]); got != 2 {
		t.Fatalf("tx1 has %d logs, want 2", got)
	}
	// order within a transaction must match eth_getLogs (log index order)
	if logs[capTx1][0].Data != "0x01" || logs[capTx1][1].Data != "0x02" {
		t.Errorf("tx1 logs out of order: %q, %q", logs[capTx1][0].Data, logs[capTx1][1].Data)
	}
	if got := len(logs[capTx2]); got != 1 {
		t.Fatalf("tx2 has %d logs, want 1 (log without transactionHash must fall back to the receipt hash)", got)
	}
	if logs[capTx2][0].Address != "0xaaaa000000000000000000000000000000000003" {
		t.Errorf("tx2 log = %+v, want the third log", logs[capTx2][0])
	}
	if _, ok := logs[""]; ok {
		t.Error("logs attributed to the empty transaction id")
	}
	if got := stub.calls["eth_getBlockReceipts"]; got != 1 {
		t.Errorf("eth_getBlockReceipts called %d times, want exactly 1", got)
	}
	// addressed by hash, not number: the body was already fetched by hash, so this keeps
	// the two halves of the block consistent across a reorg
	if len(stub.receiptsArgs) != 1 || stub.receiptsArgs[0] != capBlockHash {
		t.Errorf("eth_getBlockReceipts args = %v, want [%s]", stub.receiptsArgs, capBlockHash)
	}
	// the refused query must not be repeated: it is deterministic and costly
	if got := stub.calls["eth_getLogs"]; got != 1 {
		t.Errorf("eth_getLogs called %d times, want exactly 1", got)
	}
}

// Errors other than a result cap are transient (network/backend hiccups) and are retried by
// the sync loop, so they must be returned as-is without spending an extra RPC round trip.
func TestProcessEventsForBlock_NoFallbackOnOtherErrors(t *testing.T) {
	stub := newLogCapStub()
	stub.logsErr, stub.receiptsJSON = stdErrors.New("connection reset by peer"), capReceiptsJSON
	b := &EthereumRPC{RPC: stub, Timeout: time.Second}

	if _, _, err := b.processEventsForBlock("0x1875fd0", capBlockHash, 2); err == nil {
		t.Fatal("processEventsForBlock succeeded, want the eth_getLogs error")
	} else if !strings.Contains(err.Error(), "connection reset by peer") {
		t.Errorf("error = %v, want it to carry the eth_getLogs error", err)
	}
	if got := stub.calls["eth_getBlockReceipts"]; got != 0 {
		t.Errorf("eth_getBlockReceipts called %d times, want 0", got)
	}
}

// A null eth_getBlockReceipts result means the backend does not know the block. It must
// never be treated as "block has no logs": that would commit a block with >= 20000 logs as
// log-less, silently dropping every token transfer in it.
func TestProcessEventsForBlock_NullReceiptsIsError(t *testing.T) {
	stub := newLogCapStub()
	stub.logsErr, stub.receiptsJSON = erigonLogCapErr, "null"
	b := &EthereumRPC{RPC: stub, Timeout: time.Second}

	logs, _, err := b.processEventsForBlock("0x1875fd0", capBlockHash, 2)
	if err == nil {
		t.Fatalf("processEventsForBlock succeeded with logs %+v, want an error", logs)
	}
	if logs != nil {
		t.Errorf("logs = %+v, want nil on error", logs)
	}
	if got := stub.calls["eth_getBlockReceipts"]; got != 1 {
		t.Fatalf("eth_getBlockReceipts called %d times, want 1 (the fallback must have run)", got)
	}
	if !strings.Contains(err.Error(), "eth_getBlockReceipts") {
		t.Errorf("error %q does not name the failing fallback", err)
	}
}

// When the fallback itself fails (backend without eth_getBlockReceipts), the returned error
// must name both failures so the cause is diagnosable from the log line alone.
func TestProcessEventsForBlock_FallbackErrorMentionsBothCauses(t *testing.T) {
	stub := newLogCapStub()
	stub.logsErr, stub.receiptsErr = erigonLogCapErr, stdErrors.New("the method eth_getBlockReceipts does not exist")
	b := &EthereumRPC{RPC: stub, Timeout: time.Second}

	_, _, err := b.processEventsForBlock("0x1875fd0", capBlockHash, 2)
	if err == nil {
		t.Fatal("processEventsForBlock succeeded, want an error")
	}
	msg := err.Error()
	for _, want := range []string{"eth_getBlockReceipts", "does not exist", "too many logs"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not mention %q", msg, want)
		}
	}
}

// A successful eth_getLogs must not touch the fallback path.
func TestProcessEventsForBlock_NoFallbackOnSuccess(t *testing.T) {
	stub := newLogCapStub()
	stub.logsJSON = `[{"address":"0xaaaa000000000000000000000000000000000001","topics":["0xaa"],"data":"0x01","transactionHash":"` + capTx1 + `"}]`
	b := &EthereumRPC{RPC: stub, Timeout: time.Second}

	logs, _, err := b.processEventsForBlock("0x1875fd0", capBlockHash, 1)
	if err != nil {
		t.Fatalf("processEventsForBlock returned error: %v", err)
	}
	if len(logs[capTx1]) != 1 {
		t.Errorf("logs = %+v, want one log for tx1", logs)
	}
	if got := stub.calls["eth_getBlockReceipts"]; got != 0 {
		t.Errorf("eth_getBlockReceipts called %d times, want 0", got)
	}
}

func TestIsTooManyLogsError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"erigon cap", erigonLogCapErr, true},
		{"geth more than results", stdErrors.New("query returned more than 10000 results"), true},
		{"geth too many results", stdErrors.New("query returns too many results"), true},
		{"reth max results", stdErrors.New("query exceeds max results 20000, retry with the range 0x1-0x2"), true},
		{"provider response size", stdErrors.New("Log response size exceeded. Try with this block range"), true},
		{"connection reset", stdErrors.New("connection reset by peer"), false},
		{"block not found", stdErrors.New("block not found"), false},
		{"range limit", stdErrors.New("query block range exceeds server limit, narrow your filter"), false},
		{"context canceled", context.Canceled, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isTooManyLogsError(tc.err); got != tc.want {
				t.Errorf("isTooManyLogsError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// Every shape here is a response that contradicts the cap error that triggered the
// fallback ("this block has >= 20000 logs"), so accepting any of them would commit the
// block with no token transfers at all - silently, with no error and no retry. Erroring
// stalls that block instead, which is loud and recoverable.
func TestProcessEventsForBlock_RejectsUnderDeliveringReceipts(t *testing.T) {
	tests := []struct {
		name         string
		receiptsJSON string
		txCount      int
		wantErr      string
	}{
		{"null", "null", 2, "no receipts"},
		{"empty array", "[]", 2, "no receipts"},
		{
			name:         "fewer receipts than transactions",
			receiptsJSON: `[{"transactionHash":"` + capTx1 + `","logs":[{"address":"0xaa","topics":[],"data":"0x","transactionHash":"` + capTx1 + `"}]}]`,
			txCount:      2,
			wantErr:      "1 receipts for 2 transactions",
		},
		{
			name:         "receipts with empty logs",
			receiptsJSON: `[{"transactionHash":"` + capTx1 + `","logs":[]},{"transactionHash":"` + capTx2 + `","logs":[]}]`,
			txCount:      2,
			wantErr:      "no logs",
		},
		{
			name:         "receipts with null logs",
			receiptsJSON: `[{"transactionHash":"` + capTx1 + `","logs":null},{"transactionHash":"` + capTx2 + `"}]`,
			txCount:      2,
			wantErr:      "no logs",
		},
		{
			name:         "log with no attributable transaction hash",
			receiptsJSON: `[{"logs":[{"address":"0xaa","topics":[],"data":"0x"}]},{"transactionHash":"` + capTx2 + `","logs":[]}]`,
			txCount:      2,
			wantErr:      "no transaction hash",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stub := newLogCapStub()
			stub.logsErr, stub.receiptsJSON = erigonLogCapErr, tc.receiptsJSON
			b := &EthereumRPC{RPC: stub, Timeout: time.Second}

			logs, _, err := b.processEventsForBlock("0x1875fd0", capBlockHash, tc.txCount)
			if err == nil {
				t.Fatalf("processEventsForBlock succeeded with logs %+v, want an error", logs)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want it to mention %q", err, tc.wantErr)
			}
			if logs != nil {
				t.Errorf("logs = %+v, want nil on error", logs)
			}
		})
	}
}

// bor appends a state-sync pseudo-transaction receipt that has no counterpart in the
// transaction list, so more receipts than transactions is legitimate and must be accepted.
func TestProcessEventsForBlock_AcceptsMoreReceiptsThanTransactions(t *testing.T) {
	stub := newLogCapStub()
	stub.logsErr, stub.receiptsJSON = erigonLogCapErr, capReceiptsJSON
	b := &EthereumRPC{RPC: stub, Timeout: time.Second}

	logs, _, err := b.processEventsForBlock("0x1875fd0", capBlockHash, 1)
	if err != nil {
		t.Fatalf("processEventsForBlock returned error: %v", err)
	}
	if len(logs) != 2 {
		t.Errorf("logs map has %d transactions, want 2", len(logs))
	}
}

// Providers capitalize their messages differently, so matching must be case-insensitive.
func TestIsTooManyLogsErrorIgnoresCase(t *testing.T) {
	for _, err := range []error{
		stdErrors.New("Query Returns Too Many Logs, Narrow Your Filter: 20000"),
		stdErrors.New("Log Response Size Exceeded. Try with this block range"),
		stdErrors.New("Query Returned More Than 10000 Results"),
	} {
		if !isTooManyLogsError(err) {
			t.Errorf("isTooManyLogsError(%q) = false, want true", err)
		}
	}
}
