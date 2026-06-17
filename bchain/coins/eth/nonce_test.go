//go:build unittest

package eth

import (
	"context"
	"errors"
	"testing"
	"time"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/trezor/blockbook/bchain"
)

// nonceTagFromArgs extracts the block tag ("pending"/"latest") from eth_getTransactionCount args.
func nonceTagFromArgs(args []interface{}) string {
	if len(args) >= 2 {
		if tag, ok := args[1].(string); ok {
			return tag
		}
	}
	return ""
}

// nonceBatchStub implements bchain.EVMRPCClient AND BatchCallContext, serving
// eth_getTransactionCount per block tag. It records which tags were queried so tests can
// assert that the "latest" call is only made when the confirmed nonce is requested.
type nonceBatchStub struct {
	results  map[string]string // tag -> hex result
	errs     map[string]error  // tag -> per-call error
	batchErr error             // transport-level error from BatchCallContext
	queried  []string
}

func (s *nonceBatchStub) EthSubscribe(context.Context, interface{}, ...interface{}) (bchain.EVMClientSubscription, error) {
	return nil, errors.New("not implemented")
}

func (s *nonceBatchStub) Close() {}

func (s *nonceBatchStub) CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error {
	tag := nonceTagFromArgs(args)
	s.queried = append(s.queried, tag)
	if err := s.errs[tag]; err != nil {
		return err
	}
	p, ok := result.(*string)
	if !ok {
		return errors.New("unexpected result type")
	}
	*p = s.results[tag]
	return nil
}

func (s *nonceBatchStub) BatchCallContext(ctx context.Context, batch []rpc.BatchElem) error {
	if s.batchErr != nil {
		return s.batchErr
	}
	for i := range batch {
		tag := nonceTagFromArgs(batch[i].Args)
		s.queried = append(s.queried, tag)
		if err := s.errs[tag]; err != nil {
			batch[i].Error = err
			continue
		}
		if p, ok := batch[i].Result.(*string); ok {
			*p = s.results[tag]
		}
	}
	return nil
}

// nonceSeqStub implements bchain.EVMRPCClient WITHOUT BatchCallContext, to exercise the
// sequential fallback path of getNoncesRPC.
type nonceSeqStub struct {
	results map[string]string
	errs    map[string]error
	queried []string
}

func (s *nonceSeqStub) EthSubscribe(context.Context, interface{}, ...interface{}) (bchain.EVMClientSubscription, error) {
	return nil, errors.New("not implemented")
}

func (s *nonceSeqStub) Close() {}

func (s *nonceSeqStub) CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error {
	tag := nonceTagFromArgs(args)
	s.queried = append(s.queried, tag)
	if err := s.errs[tag]; err != nil {
		return err
	}
	p, ok := result.(*string)
	if !ok {
		return errors.New("unexpected result type")
	}
	*p = s.results[tag]
	return nil
}

var nonceTestAddr = bchain.AddressDescriptor(ethcommon.HexToAddress("0x4Bda106325C335dF99eab7fE363cAC8A0ba2a24D").Bytes())

func TestEthereumTypeGetNonces_GatedOff_FetchesPendingOnly(t *testing.T) {
	stub := &nonceBatchStub{results: map[string]string{"pending": "0x4", "latest": "0x2"}}
	b := &EthereumRPC{RPC: stub, Timeout: time.Second}

	pending, confirmed, confirmedOK, err := b.EthereumTypeGetNonces(nonceTestAddr, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pending != 4 {
		t.Errorf("pending = %d, want 4", pending)
	}
	if confirmedOK {
		t.Errorf("confirmedOK = true, want false when not requested")
	}
	if confirmed != 0 {
		t.Errorf("confirmed = %d, want 0 when not requested", confirmed)
	}
	// the latest tag must not be queried when confirmed nonce is not requested
	if len(stub.queried) != 1 || stub.queried[0] != "pending" {
		t.Errorf("queried tags = %v, want exactly [pending]", stub.queried)
	}
}

func TestEthereumTypeGetNonces_GatedOn_Batched(t *testing.T) {
	stub := &nonceBatchStub{results: map[string]string{"pending": "0x4", "latest": "0x2"}}
	b := &EthereumRPC{RPC: stub, Timeout: time.Second}

	pending, confirmed, confirmedOK, err := b.EthereumTypeGetNonces(nonceTestAddr, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pending != 4 || confirmed != 2 || !confirmedOK {
		t.Errorf("got (pending=%d confirmed=%d ok=%v), want (4 2 true)", pending, confirmed, confirmedOK)
	}
}

func TestEthereumTypeGetNonces_GatedOn_ConfirmedFailureIsBestEffort(t *testing.T) {
	// the latest sub-call fails but pending succeeds: pending must still be returned,
	// confirmed omitted, and NO error surfaced (so the whole address response survives)
	stub := &nonceBatchStub{
		results: map[string]string{"pending": "0x4"},
		errs:    map[string]error{"latest": errors.New("boom")},
	}
	b := &EthereumRPC{RPC: stub, Timeout: time.Second}

	pending, confirmed, confirmedOK, err := b.EthereumTypeGetNonces(nonceTestAddr, true)
	if err != nil {
		t.Fatalf("confirmed-nonce failure must not be fatal, got error: %v", err)
	}
	if pending != 4 {
		t.Errorf("pending = %d, want 4", pending)
	}
	if confirmedOK || confirmed != 0 {
		t.Errorf("got (confirmed=%d ok=%v), want (0 false) on best-effort failure", confirmed, confirmedOK)
	}
}

func TestEthereumTypeGetNonces_GatedOn_ConfirmedDecodeFailureIsBestEffort(t *testing.T) {
	// the latest sub-call SUCCEEDS but returns an unparsable hex value (a backend answering 200
	// with garbage). This is a distinct best-effort failure mode from a transport/lookup error and
	// exercises the decode branch of decodeConfirmedNonce: pending must still be returned, confirmed
	// omitted, and NO error surfaced.
	stub := &nonceBatchStub{results: map[string]string{"pending": "0x4", "latest": "0xZZ"}}
	b := &EthereumRPC{RPC: stub, Timeout: time.Second}

	pending, confirmed, confirmedOK, err := b.EthereumTypeGetNonces(nonceTestAddr, true)
	if err != nil {
		t.Fatalf("unparsable confirmed nonce must not be fatal, got error: %v", err)
	}
	if pending != 4 {
		t.Errorf("pending = %d, want 4", pending)
	}
	if confirmedOK || confirmed != 0 {
		t.Errorf("got (confirmed=%d ok=%v), want (0 false) on unparsable confirmed nonce", confirmed, confirmedOK)
	}
}

func TestEthereumTypeGetNonces_GatedOn_PendingFailureIsFatal(t *testing.T) {
	stub := &nonceBatchStub{
		results: map[string]string{"latest": "0x2"},
		errs:    map[string]error{"pending": errors.New("boom")},
	}
	b := &EthereumRPC{RPC: stub, Timeout: time.Second}

	if _, _, _, err := b.EthereumTypeGetNonces(nonceTestAddr, true); err == nil {
		t.Fatal("expected fatal error when the required pending nonce cannot be obtained")
	}
}

func TestEthereumTypeGetNonces_GatedOn_BatchTransportFailureIsFatal(t *testing.T) {
	stub := &nonceBatchStub{batchErr: errors.New("transport down")}
	b := &EthereumRPC{RPC: stub, Timeout: time.Second}

	if _, _, _, err := b.EthereumTypeGetNonces(nonceTestAddr, true); err == nil {
		t.Fatal("expected fatal error on batch transport failure")
	}
}

func TestEthereumTypeGetNonces_SequentialFallback(t *testing.T) {
	// a client without BatchCallContext must fall back to two sequential calls
	stub := &nonceSeqStub{results: map[string]string{"pending": "0x4", "latest": "0x2"}}
	b := &EthereumRPC{RPC: stub, Timeout: time.Second}

	pending, confirmed, confirmedOK, err := b.EthereumTypeGetNonces(nonceTestAddr, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pending != 4 || confirmed != 2 || !confirmedOK {
		t.Errorf("got (pending=%d confirmed=%d ok=%v), want (4 2 true)", pending, confirmed, confirmedOK)
	}
	if len(stub.queried) != 2 {
		t.Errorf("queried %d times, want 2 sequential calls (%v)", len(stub.queried), stub.queried)
	}
}

func TestEthereumTypeGetNonces_SequentialFallback_ConfirmedFailureIsBestEffort(t *testing.T) {
	stub := &nonceSeqStub{
		results: map[string]string{"pending": "0x4"},
		errs:    map[string]error{"latest": errors.New("boom")},
	}
	b := &EthereumRPC{RPC: stub, Timeout: time.Second}

	pending, _, confirmedOK, err := b.EthereumTypeGetNonces(nonceTestAddr, true)
	if err != nil {
		t.Fatalf("confirmed-nonce failure must not be fatal, got error: %v", err)
	}
	if pending != 4 || confirmedOK {
		t.Errorf("got (pending=%d ok=%v), want (4 false)", pending, confirmedOK)
	}
}

func TestEthereumTypeGetNonces_SequentialFallback_ConfirmedDecodeFailureIsBestEffort(t *testing.T) {
	// sequential-fallback counterpart of the batched decode-failure case: an unparsable latest
	// result must be best-effort (pending returned, confirmedOK=false, no error)
	stub := &nonceSeqStub{results: map[string]string{"pending": "0x4", "latest": "0xZZ"}}
	b := &EthereumRPC{RPC: stub, Timeout: time.Second}

	pending, _, confirmedOK, err := b.EthereumTypeGetNonces(nonceTestAddr, true)
	if err != nil {
		t.Fatalf("unparsable confirmed nonce must not be fatal, got error: %v", err)
	}
	if pending != 4 || confirmedOK {
		t.Errorf("got (pending=%d ok=%v), want (4 false)", pending, confirmedOK)
	}
}
