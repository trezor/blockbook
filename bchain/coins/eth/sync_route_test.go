package eth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/trezor/blockbook/bchain"
)

func TestSyncRouteCtxRoundTrip(t *testing.T) {
	if isSyncRoute(context.Background()) {
		t.Fatal("plain ctx must not carry the sync-route tag")
	}
	if !isSyncRoute(WithSyncRoute(context.Background())) {
		t.Fatal("WithSyncRoute(ctx) must mark ctx as sync-route")
	}
	if isSyncRoute(nil) {
		t.Fatal("nil ctx must not be reported as sync-route")
	}
	// Wrapping with another value-bearing ctx must preserve the sync-route tag.
	parent := WithSyncRoute(context.Background())
	child := context.WithValue(parent, struct{}{}, "x")
	if !isSyncRoute(child) {
		t.Fatal("sync-route tag must propagate through wrapping ctx")
	}
}

func TestEthereumClientPick(t *testing.T) {
	// httpClient and wsClient must be distinct pointers so identity-based
	// dispatch is observable. They are not used for any actual call here.
	httpEC := &ethclient.Client{}
	wsEC := &ethclient.Client{}
	c := &EthereumClient{httpClient: httpEC, wsClient: wsEC}

	if got := c.pick(context.Background()); got != httpEC {
		t.Fatal("plain ctx must dispatch to httpClient")
	}
	if got := c.pick(WithSyncRoute(context.Background())); got != wsEC {
		t.Fatal("sync-route ctx must dispatch to wsClient")
	}

	// When the WS client is aliased to the HTTP client (single underlying
	// rpc.Client), both ctx flavours dispatch to the same client.
	c2 := &EthereumClient{httpClient: httpEC, wsClient: httpEC}
	if got := c2.pick(WithSyncRoute(context.Background())); got != httpEC {
		t.Fatal("aliased clients must dispatch to httpClient regardless of ctx tag")
	}
}

// jsonrpcServer counts incoming JSON-RPC requests and returns a fixed result.
type jsonrpcServer struct {
	calls atomic.Int64
}

func (s *jsonrpcServer) handler(t *testing.T) http.HandlerFunc {
	type rpcReq struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		s.calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		// Server is a stub: handle both single calls and batch (array) calls,
		// returning a uniform "0x1" result for each.
		if len(body) > 0 && body[0] == '[' {
			var reqs []rpcReq
			if err := json.Unmarshal(body, &reqs); err != nil {
				t.Fatalf("parse batch body: %v", err)
			}
			out := "["
			for i, req := range reqs {
				if i > 0 {
					out += ","
				}
				out += `{"jsonrpc":"2.0","id":` + string(req.ID) + `,"result":"0x1"}`
			}
			out += "]"
			_, _ = w.Write([]byte(out))
			return
		}
		var req rpcReq
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("parse body: %v", err)
		}
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":` + string(req.ID) + `,"result":"0x1"}`))
	}
}

func TestDualRPCClientCallContextRouting(t *testing.T) {
	httpSrv := &jsonrpcServer{}
	wsSrv := &jsonrpcServer{}
	httpHTTP := httptest.NewServer(httpSrv.handler(t))
	defer httpHTTP.Close()
	wsHTTP := httptest.NewServer(wsSrv.handler(t))
	defer wsHTTP.Close()

	// Both endpoints are HTTP for the purposes of this routing test — the
	// transport is incidental; what matters is that DualRPCClient picks
	// SubClient when ctx carries the sync-route tag.
	callClient, err := rpc.Dial(httpHTTP.URL)
	if err != nil {
		t.Fatalf("dial http: %v", err)
	}
	defer callClient.Close()
	subClient, err := rpc.Dial(wsHTTP.URL)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	defer subClient.Close()

	dual := &DualRPCClient{CallClient: callClient, SubClient: subClient}

	var result string
	if err := dual.CallContext(context.Background(), &result, "eth_blockNumber"); err != nil {
		t.Fatalf("plain ctx call failed: %v", err)
	}
	if got, want := httpSrv.calls.Load(), int64(1); got != want {
		t.Fatalf("plain ctx must hit CallClient: httpCalls=%d, want %d", got, want)
	}
	if got := wsSrv.calls.Load(); got != 0 {
		t.Fatalf("plain ctx must not hit SubClient: wsCalls=%d, want 0", got)
	}

	if err := dual.CallContext(WithSyncRoute(context.Background()), &result, "eth_blockNumber"); err != nil {
		t.Fatalf("sync-route ctx call failed: %v", err)
	}
	if got := httpSrv.calls.Load(); got != 1 {
		t.Fatalf("sync-route ctx must not hit CallClient: httpCalls=%d, want 1", got)
	}
	if got, want := wsSrv.calls.Load(), int64(1); got != want {
		t.Fatalf("sync-route ctx must hit SubClient: wsCalls=%d, want %d", got, want)
	}
}

// rpcRecorder satisfies bchain.EVMRPCClient and records the ctx forwarded by
// EthereumRPC helpers. It returns a JSON null so getBlockRaw short-circuits
// with ErrBlockNotFound — the test only inspects the ctx, not the result.
type rpcRecorder struct {
	lastCtx context.Context
}

func (r *rpcRecorder) CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error {
	r.lastCtx = ctx
	if raw, ok := result.(*json.RawMessage); ok {
		*raw = json.RawMessage("null")
	}
	return nil
}

func (r *rpcRecorder) EthSubscribe(ctx context.Context, channel interface{}, args ...interface{}) (bchain.EVMClientSubscription, error) {
	return nil, nil
}

func (r *rpcRecorder) Close() {}

func TestSyncFacadeTagsCtx(t *testing.T) {
	rec := &rpcRecorder{}
	b := &EthereumRPC{RPC: rec, Timeout: 0}

	// Public path — must NOT carry the sync-route tag.
	_, _ = b.GetBlock("0xabc", 100)
	if rec.lastCtx == nil {
		t.Fatal("expected CallContext to be invoked on public GetBlock")
	}
	if isSyncRoute(rec.lastCtx) {
		t.Fatal("public GetBlock must not carry sync-route tag")
	}

	// Sync facade path — must carry the sync-route tag.
	rec.lastCtx = nil
	view := &ethSyncView{EthereumRPC: b}
	_, _ = view.GetBlock("0xabc", 100)
	if rec.lastCtx == nil {
		t.Fatal("expected CallContext to be invoked on sync facade GetBlock")
	}
	if !isSyncRoute(rec.lastCtx) {
		t.Fatal("sync facade GetBlock must carry sync-route tag")
	}
}

func TestDualRPCClientBatchAlwaysHTTP(t *testing.T) {
	httpSrv := &jsonrpcServer{}
	wsSrv := &jsonrpcServer{}
	httpHTTP := httptest.NewServer(httpSrv.handler(t))
	defer httpHTTP.Close()
	wsHTTP := httptest.NewServer(wsSrv.handler(t))
	defer wsHTTP.Close()

	callClient, err := rpc.Dial(httpHTTP.URL)
	if err != nil {
		t.Fatalf("dial http: %v", err)
	}
	defer callClient.Close()
	subClient, err := rpc.Dial(wsHTTP.URL)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	defer subClient.Close()

	dual := &DualRPCClient{CallClient: callClient, SubClient: subClient}

	// BatchCallContext must always hit CallClient regardless of ctx tag —
	// batching is HTTP-shaped in this codebase (contract.go ERC-20 fan-out).
	batch := []rpc.BatchElem{{Method: "eth_blockNumber", Result: new(string)}}
	if err := dual.BatchCallContext(WithSyncRoute(context.Background()), batch); err != nil {
		t.Fatalf("batch call failed: %v", err)
	}
	if got, want := httpSrv.calls.Load(), int64(1); got != want {
		t.Fatalf("batch must hit CallClient: httpCalls=%d, want %d", got, want)
	}
	if got := wsSrv.calls.Load(); got != 0 {
		t.Fatalf("batch must not hit SubClient: wsCalls=%d, want 0", got)
	}
}
