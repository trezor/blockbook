//go:build unittest

package eth

import (
	"context"
	"encoding/json"
	stdErrors "errors"
	"testing"
	"time"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/trezor/blockbook/bchain"
)

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
