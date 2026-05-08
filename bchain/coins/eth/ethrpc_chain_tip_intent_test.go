package eth

import (
	"context"
	"encoding/json"
	"math/big"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/trezor/blockbook/bchain"
)

type recordingIntentRPC struct {
	mu          sync.Mutex
	normalCalls []string
	intentCalls []string
}

func (r *recordingIntentRPC) EthSubscribe(ctx context.Context, channel interface{}, args ...interface{}) (bchain.EVMClientSubscription, error) {
	return nil, nil
}

func (r *recordingIntentRPC) CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error {
	r.mu.Lock()
	r.normalCalls = append(r.normalCalls, method)
	r.mu.Unlock()
	return setRecordingIntentRPCResult(result, method)
}

func (r *recordingIntentRPC) CallContextWithIntent(ctx context.Context, intent bchain.EVMRPCIntent, result interface{}, method string, args ...interface{}) error {
	if intent == bchain.EVMRPCIntentChainTip {
		r.mu.Lock()
		r.intentCalls = append(r.intentCalls, method)
		r.mu.Unlock()
		return setRecordingIntentRPCResult(result, method)
	}
	return r.CallContext(ctx, result, method, args...)
}

func (r *recordingIntentRPC) Close() {}

func setRecordingIntentRPCResult(result interface{}, method string) error {
	switch v := result.(type) {
	case *json.RawMessage:
		*v = json.RawMessage(`{"hash":"0x1234","parentHash":"0x0123","difficulty":"0x0","number":"0x7b","timestamp":"0x1","size":"0x2","transactions":[]}`)
	case *rpcHeader:
		*v = rpcHeader{Hash: "0xws", ParentHash: "0x01", Number: "0x7b", Difficulty: "0x0", Time: "0x1", Size: "0x2"}
	case *[]rpcLogWithTxHash:
		*v = nil
	case *[]rpcTraceResult:
		*v = nil
	default:
		return nil
	}
	return nil
}

type stubIntentEVMClient struct {
	headerCalls []string
}

func (s *stubIntentEVMClient) NetworkID(ctx context.Context) (*big.Int, error) {
	return big.NewInt(1), nil
}

func (s *stubIntentEVMClient) HeaderByNumber(ctx context.Context, number *big.Int) (bchain.EVMHeader, error) {
	if number == nil {
		s.headerCalls = append(s.headerCalls, "latest")
	} else {
		s.headerCalls = append(s.headerCalls, number.String())
	}
	return &rpcIntentHeader{
		hash:       "0xhttp",
		number:     big.NewInt(123),
		difficulty: big.NewInt(0),
	}, nil
}

func (s *stubIntentEVMClient) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	return big.NewInt(1), nil
}

func (s *stubIntentEVMClient) EstimateGas(ctx context.Context, msg interface{}) (uint64, error) {
	return 0, nil
}

func (s *stubIntentEVMClient) BalanceAt(ctx context.Context, addrDesc bchain.AddressDescriptor, blockNumber *big.Int) (*big.Int, error) {
	return big.NewInt(0), nil
}

func (s *stubIntentEVMClient) NonceAt(ctx context.Context, addrDesc bchain.AddressDescriptor, blockNumber *big.Int) (uint64, error) {
	return 0, nil
}

func newIntentTestEthereumRPC(rpc *recordingIntentRPC, client *stubIntentEVMClient) *EthereumRPC {
	return &EthereumRPC{
		Client:      client,
		RPC:         rpc,
		Timeout:     time.Second,
		ChainConfig: &Configuration{},
		bestHeader: &rpcIntentHeader{
			hash:       "0xbest",
			number:     big.NewInt(200),
			difficulty: big.NewInt(0),
		},
	}
}

func TestDefaultEthereumBlockReadsStayOnHTTP(t *testing.T) {
	rpc := &recordingIntentRPC{}
	client := &stubIntentEVMClient{}
	b := newIntentTestEthereumRPC(rpc, client)

	if got, err := b.GetBlockHash(123); err != nil {
		t.Fatalf("GetBlockHash() error = %v", err)
	} else if got != "0xhttp" {
		t.Fatalf("GetBlockHash() = %q, want 0xhttp", got)
	}
	if _, err := b.getBlockRaw("0x1234", 0, true); err != nil {
		t.Fatalf("getBlockRaw() error = %v", err)
	}
	if _, _, err := b.processEventsForBlock("0x7b"); err != nil {
		t.Fatalf("processEventsForBlock() error = %v", err)
	}

	if len(rpc.intentCalls) != 0 {
		t.Fatalf("intentCalls = %v, want none", rpc.intentCalls)
	}
	if want := []string{"eth_getBlockByHash", "eth_getLogs"}; !reflect.DeepEqual(rpc.normalCalls, want) {
		t.Fatalf("normalCalls = %v, want %v", rpc.normalCalls, want)
	}
	if want := []string{"123"}; !reflect.DeepEqual(client.headerCalls, want) {
		t.Fatalf("headerCalls = %v, want %v", client.headerCalls, want)
	}
}

func TestChainTipSyncViewRoutesTargetReadsToIntentClient(t *testing.T) {
	rpc := &recordingIntentRPC{}
	b := newIntentTestEthereumRPC(rpc, &stubIntentEVMClient{})
	b.bestHeader = nil
	view := b.BlockChainForSyncIntent(bchain.SyncIntentChainTip)

	if got, err := view.GetBestBlockHash(); err != nil {
		t.Fatalf("GetBestBlockHash() error = %v", err)
	} else if got != "0xws" {
		t.Fatalf("GetBestBlockHash() = %q, want 0xws", got)
	}
	if got, err := view.GetBlockHash(123); err != nil {
		t.Fatalf("GetBlockHash() error = %v", err)
	} else if got != "0xws" {
		t.Fatalf("GetBlockHash() = %q, want 0xws", got)
	}
	if _, err := view.GetBlock("0x1234", 0); err != nil {
		t.Fatalf("GetBlock() error = %v", err)
	}

	want := []string{"eth_getBlockByNumber", "eth_getBlockByNumber", "eth_getBlockByHash", "eth_getLogs"}
	if !reflect.DeepEqual(rpc.intentCalls, want) {
		t.Fatalf("intentCalls = %v, want %v", rpc.intentCalls, want)
	}
	if len(rpc.normalCalls) != 0 {
		t.Fatalf("normalCalls = %v, want none", rpc.normalCalls)
	}
}

func TestChainTipSyncIntentKeepsPendingBlockOnHTTP(t *testing.T) {
	rpc := &recordingIntentRPC{}
	b := newIntentTestEthereumRPC(rpc, &stubIntentEVMClient{})

	if _, err := b.getBlockRawWithSyncIntent(bchain.SyncIntentChainTip, "pending", 0, false); err != nil {
		t.Fatalf("getBlockRawWithSyncIntent(pending) error = %v", err)
	}
	if len(rpc.intentCalls) != 0 {
		t.Fatalf("intentCalls = %v, want none", rpc.intentCalls)
	}
	if want := []string{"eth_getBlockByNumber"}; !reflect.DeepEqual(rpc.normalCalls, want) {
		t.Fatalf("normalCalls = %v, want %v", rpc.normalCalls, want)
	}
}

func TestChainTipSyncIntentKeepsInternalTraceOnHTTP(t *testing.T) {
	rpc := &recordingIntentRPC{}
	b := newIntentTestEthereumRPC(rpc, &stubIntentEVMClient{})
	view := b.BlockChainForSyncIntent(bchain.SyncIntentChainTip)

	processInternalTransactions := bchain.ProcessInternalTransactions
	bchain.ProcessInternalTransactions = true
	defer func() {
		bchain.ProcessInternalTransactions = processInternalTransactions
	}()

	if _, err := view.GetBlock("0x1234", 0); err != nil {
		t.Fatalf("GetBlock() error = %v", err)
	}
	if want := []string{"eth_getBlockByHash", "eth_getLogs"}; !reflect.DeepEqual(rpc.intentCalls, want) {
		t.Fatalf("intentCalls = %v, want %v", rpc.intentCalls, want)
	}
	if want := []string{"debug_traceBlockByHash"}; !reflect.DeepEqual(rpc.normalCalls, want) {
		t.Fatalf("normalCalls = %v, want %v", rpc.normalCalls, want)
	}
}
