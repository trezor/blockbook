//go:build unittest

package eth

import (
	"context"
	"errors"
	"testing"
	"time"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
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

// newRecentSenderProvider returns an alternative provider backed by server whose recentSenders
// map holds the given addresses, i.e. useForNonces routes exactly those to the provider.
func newRecentSenderProvider(server *nonceRPCServer, senders ...ethcommon.Address) *AlternativeSendTxProvider {
	recentSenders := make(map[ethcommon.Address]recentSender)
	for _, sender := range senders {
		recentSenders[sender] = recentSender{time: time.Now(), url: server.URL}
	}
	return &AlternativeSendTxProvider{
		urls:              []string{server.URL},
		mempoolTxsTimeout: time.Hour,
		rpcTimeout:        time.Second,
		recentSenders:     recentSenders,
	}
}

// TestEthereumTypeGetNonces_AlternativeProvider_SkippedForUnknownAddress is the regression test
// for #1627: an address without a recently privately-submitted tx must be served by the primary
// RPC without any round-trip to the alternative provider.
func TestEthereumTypeGetNonces_AlternativeProvider_SkippedForUnknownAddress(t *testing.T) {
	server := newNonceRPCServer(t, map[string]string{"pending": "0x9"}, nil)
	stub := &nonceBatchStub{results: map[string]string{"pending": "0x4"}}
	b := &EthereumRPC{RPC: stub, Timeout: time.Second, alternativeSendTxProvider: newRecentSenderProvider(server)}

	pending, _, _, err := b.EthereumTypeGetNonces(nonceTestAddr, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pending != 4 {
		t.Errorf("pending = %d, want 4 from the primary RPC", pending)
	}
	if got := server.callCount("pending"); got != 0 {
		t.Errorf("alternative provider queried %d times, want 0 for an address that did not send through it", got)
	}
}

func TestEthereumTypeGetNonces_AlternativeProvider_UsedForRecentSender(t *testing.T) {
	server := newNonceRPCServer(t, map[string]string{"pending": "0x9"}, nil)
	stub := &nonceBatchStub{results: map[string]string{"pending": "0x4"}}
	sender := ethcommon.BytesToAddress(nonceTestAddr)
	b := &EthereumRPC{RPC: stub, Timeout: time.Second, alternativeSendTxProvider: newRecentSenderProvider(server, sender)}

	pending, _, _, err := b.EthereumTypeGetNonces(nonceTestAddr, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pending != 9 {
		t.Errorf("pending = %d, want 9 from the alternative provider", pending)
	}
	if len(stub.queried) != 0 {
		t.Errorf("primary RPC queried tags %v, want none for a recent private sender", stub.queried)
	}
}

func TestEthereumTypeGetNonces_AlternativeProvider_FallbackToPrimaryOnProviderError(t *testing.T) {
	// without any cached private tx there is no floor to apply, so the primary answer stands
	server := newNonceRPCServer(t, nil, map[string]bool{"pending": true})
	stub := &nonceBatchStub{results: map[string]string{"pending": "0x4"}}
	sender := ethcommon.BytesToAddress(nonceTestAddr)
	b := &EthereumRPC{RPC: stub, Timeout: time.Second, alternativeSendTxProvider: newRecentSenderProvider(server, sender)}

	pending, _, _, err := b.EthereumTypeGetNonces(nonceTestAddr, false)
	if err != nil {
		t.Fatalf("provider failure must fall back to the primary RPC, got error: %v", err)
	}
	if pending != 4 {
		t.Errorf("pending = %d, want 4 from the primary RPC fallback", pending)
	}
}

func TestEthereumTypeGetNonces_AlternativeProvider_FallbackRaisedToCacheFloor(t *testing.T) {
	// the provider lookup fails and the primary answers 4: with the cache holding the sender's tx at
	// nonce 4 the fallback must report 5, or a wallet building on the primary answer replaces that
	// in-flight tx. The gap case is the other half of #1675 - nothing fills nonce 4, so the floor must
	// NOT jump over the hole to the cached 0x7, or every later send queues behind it.
	for _, tt := range []struct {
		name        string
		cachedNonce string
		want        uint64
	}{
		{name: "contiguous with the primary answer", cachedNonce: "0x4", want: 5},
		{name: "gap below the cached tx", cachedNonce: "0x7", want: 4},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := newNonceRPCServer(t, nil, map[string]bool{"pending": true})
			stub := &nonceBatchStub{results: map[string]string{"pending": "0x4"}}
			sender := ethcommon.BytesToAddress(nonceTestAddr)
			provider := newRecentSenderProvider(server, sender)
			provider.fetchMempoolTx = true
			provider.mempoolTxs = map[string]storedTx{
				testAlternativeTxID: {
					tx:   &bchain.RpcTransaction{Hash: testAlternativeTxID, From: sender.Hex(), AccountNonce: tt.cachedNonce},
					time: uint32(time.Now().Unix()),
				},
			}
			b := &EthereumRPC{RPC: stub, Timeout: time.Second, alternativeSendTxProvider: provider}

			pending, _, _, err := b.EthereumTypeGetNonces(nonceTestAddr, false)
			if err != nil {
				t.Fatalf("provider failure must fall back to the primary RPC, got error: %v", err)
			}
			if pending != tt.want {
				t.Errorf("pending = %d, want %d", pending, tt.want)
			}
		})
	}
}

func TestEthereumTypeGetNonces_AlternativeProvider_ProviderAnswerRaisedToCacheFloor(t *testing.T) {
	// a relay stops counting a still-pending tx once past its own pending window while Blockbook
	// exposes it until the cache timeout: the provider answers 4 with the cache holding nonce 4, so
	// the answer must become 5 and never contradict Blockbook's pending view. A gap stays at 4 (#1675).
	for _, tt := range []struct {
		name        string
		cachedNonce string
		want        uint64
	}{
		{name: "contiguous with the provider answer", cachedNonce: "0x4", want: 5},
		{name: "gap below the cached tx", cachedNonce: "0x7", want: 4},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := newNonceRPCServer(t, map[string]string{"pending": "0x4"}, nil)
			stub := &nonceBatchStub{results: map[string]string{"pending": "0x2"}}
			sender := ethcommon.BytesToAddress(nonceTestAddr)
			provider := newRecentSenderProvider(server, sender)
			provider.fetchMempoolTx = true
			provider.mempoolTxs = map[string]storedTx{
				testAlternativeTxID: {
					tx:   &bchain.RpcTransaction{Hash: testAlternativeTxID, From: sender.Hex(), AccountNonce: tt.cachedNonce},
					time: uint32(time.Now().Unix()),
				},
			}
			b := &EthereumRPC{RPC: stub, Timeout: time.Second, alternativeSendTxProvider: provider}

			pending, _, _, err := b.EthereumTypeGetNonces(nonceTestAddr, false)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pending != tt.want {
				t.Errorf("pending = %d, want %d", pending, tt.want)
			}
			if len(stub.queried) != 0 {
				t.Errorf("primary RPC queried tags %v, want none for a successful provider lookup", stub.queried)
			}
		})
	}
}

// TestEthereumTypeGetNonces_AlternativeProvider_ObservesNonceRequests asserts the
// eth_alternative_nonce_requests_total counter is incremented only for lookups actually routed to the
// alternative provider (labeled success/error), and never for a gated-out address served by the primary
// RPC - so the metric measures the small gated subset, not the hot eth_getTransactionCount endpoint.
func TestEthereumTypeGetNonces_AlternativeProvider_ObservesNonceRequests(t *testing.T) {
	newMetrics := func() *common.Metrics {
		return &common.Metrics{
			EthAlternativeNonceRequests: prometheus.NewCounterVec(
				prometheus.CounterOpts{Name: "test_alt_nonce_requests_total"}, []string{"result"}),
		}
	}
	sender := ethcommon.BytesToAddress(nonceTestAddr)

	t.Run("routed provider success increments result=success", func(t *testing.T) {
		server := newNonceRPCServer(t, map[string]string{"pending": "0x9"}, nil)
		stub := &nonceBatchStub{results: map[string]string{"pending": "0x4"}}
		m := newMetrics()
		b := &EthereumRPC{RPC: stub, Timeout: time.Second, metrics: m, alternativeSendTxProvider: newRecentSenderProvider(server, sender)}

		if _, _, _, err := b.EthereumTypeGetNonces(nonceTestAddr, false); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := counterVecValue(t, m.EthAlternativeNonceRequests, "result", "success"); got != 1 {
			t.Errorf("result=success = %v, want 1", got)
		}
		if got := counterVecValue(t, m.EthAlternativeNonceRequests, "result", "error"); got != 0 {
			t.Errorf("result=error = %v, want 0", got)
		}
	})

	t.Run("routed provider error increments result=error", func(t *testing.T) {
		server := newNonceRPCServer(t, nil, map[string]bool{"pending": true})
		stub := &nonceBatchStub{results: map[string]string{"pending": "0x4"}}
		m := newMetrics()
		b := &EthereumRPC{RPC: stub, Timeout: time.Second, metrics: m, alternativeSendTxProvider: newRecentSenderProvider(server, sender)}

		if _, _, _, err := b.EthereumTypeGetNonces(nonceTestAddr, false); err != nil {
			t.Fatalf("provider failure must fall back to the primary RPC, got error: %v", err)
		}
		if got := counterVecValue(t, m.EthAlternativeNonceRequests, "result", "error"); got != 1 {
			t.Errorf("result=error = %v, want 1", got)
		}
		if got := counterVecValue(t, m.EthAlternativeNonceRequests, "result", "success"); got != 0 {
			t.Errorf("result=success = %v, want 0", got)
		}
	})

	t.Run("gated-out address records nothing", func(t *testing.T) {
		server := newNonceRPCServer(t, map[string]string{"pending": "0x9"}, nil)
		stub := &nonceBatchStub{results: map[string]string{"pending": "0x4"}}
		m := newMetrics()
		// no recent senders: the address is served by the primary RPC and must not be counted
		b := &EthereumRPC{RPC: stub, Timeout: time.Second, metrics: m, alternativeSendTxProvider: newRecentSenderProvider(server)}

		if _, _, _, err := b.EthereumTypeGetNonces(nonceTestAddr, false); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		total := counterVecValue(t, m.EthAlternativeNonceRequests, "result", "success") +
			counterVecValue(t, m.EthAlternativeNonceRequests, "result", "error")
		if total != 0 {
			t.Errorf("nonce-request counter = %v, want 0 for a gated-out address", total)
		}
	})
}

func TestEthereumTypeGetNonces_AlternativeProvider_FloorAppliedWithoutRouting(t *testing.T) {
	// the routing entry can expire before the cached tx stops being exposed as pending
	// (recentSender.time is stamped at send time, storedTx.time at the later fetch-back time,
	// and the reconcile ticker adds up to a minute of granularity) - the floor must hold even
	// when the gate no longer routes the sender to the provider
	stub := &nonceBatchStub{results: map[string]string{"pending": "0x4"}}
	sender := ethcommon.BytesToAddress(nonceTestAddr)
	provider := &AlternativeSendTxProvider{
		urls:              []string{"http://127.0.0.1:1"},
		fetchMempoolTx:    true,
		mempoolTxsTimeout: time.Hour,
		rpcTimeout:        time.Second,
		mempoolTxs: map[string]storedTx{
			testAlternativeTxID: {
				tx:   &bchain.RpcTransaction{Hash: testAlternativeTxID, From: sender.Hex(), AccountNonce: "0x4"},
				time: uint32(time.Now().Unix()),
			},
		},
	}
	b := &EthereumRPC{RPC: stub, Timeout: time.Second, alternativeSendTxProvider: provider}

	pending, _, _, err := b.EthereumTypeGetNonces(nonceTestAddr, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pending != 5 {
		t.Errorf("pending = %d, want 5 (floor applied although the sender is not routed)", pending)
	}
}

// TestEthereumTypeGetNonces_AlternativeProvider_ObservesFloorMetrics pins which floor counter a request
// lands on: raised when the cache fills the slots above the backend's answer, stranded when it holds one
// above a slot nothing fills. A gap is BOTH not-raised and stranded, the answer staying the backend's -
// the point of the #1675 clamp.
func TestEthereumTypeGetNonces_AlternativeProvider_ObservesFloorMetrics(t *testing.T) {
	for _, tt := range []struct {
		name         string
		cachedNonce  string
		wantPending  uint64
		wantRaised   float64
		wantStranded float64
	}{
		{name: "contiguous raises the floor", cachedNonce: "0x4", wantPending: 5, wantRaised: 1},
		{name: "gap strands the cached tx", cachedNonce: "0x7", wantPending: 4, wantStranded: 1},
		{name: "already consumed nonce does neither", cachedNonce: "0x2", wantPending: 4},
	} {
		t.Run(tt.name, func(t *testing.T) {
			sender := ethcommon.BytesToAddress(nonceTestAddr)
			stub := &nonceBatchStub{results: map[string]string{"pending": "0x4"}}
			m := &common.Metrics{
				EthAlternativePendingFloorRaised: prometheus.NewCounterVec(
					prometheus.CounterOpts{Name: "test_alt_floor_raised_total"}, []string{"source"}),
				EthAlternativePendingFloorStranded: prometheus.NewCounterVec(
					prometheus.CounterOpts{Name: "test_alt_floor_stranded_total"}, []string{"source"}),
			}
			provider := &AlternativeSendTxProvider{
				urls:              []string{"http://127.0.0.1:1"},
				fetchMempoolTx:    true,
				mempoolTxsTimeout: time.Hour,
				rpcTimeout:        time.Second,
				mempoolTxs: map[string]storedTx{
					testAlternativeTxID: {
						tx:   &bchain.RpcTransaction{Hash: testAlternativeTxID, From: sender.Hex(), AccountNonce: tt.cachedNonce},
						time: uint32(time.Now().Unix()),
					},
				},
			}
			b := &EthereumRPC{RPC: stub, Timeout: time.Second, metrics: m, alternativeSendTxProvider: provider}

			pending, _, _, err := b.EthereumTypeGetNonces(nonceTestAddr, false)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pending != tt.wantPending {
				t.Errorf("pending = %d, want %d", pending, tt.wantPending)
			}
			if got := counterVecValue(t, m.EthAlternativePendingFloorRaised, "source", "primary"); got != tt.wantRaised {
				t.Errorf("floor_raised{primary} = %v, want %v", got, tt.wantRaised)
			}
			if got := counterVecValue(t, m.EthAlternativePendingFloorStranded, "source", "primary"); got != tt.wantStranded {
				t.Errorf("floor_stranded{primary} = %v, want %v", got, tt.wantStranded)
			}
		})
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
