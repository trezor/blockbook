package avalanche

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
	"github.com/trezor/blockbook/bchain/coins/eth"
)

// jsonrpcServer counts incoming JSON-RPC requests and returns a fixed result.
// Mirrors the helper in eth/sync_route_test.go.
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
		// Handle both single calls and batch (array) calls.
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

func TestAvalancheClientPickEC(t *testing.T) {
	httpEC := &ethclient.Client{}
	wsEC := &ethclient.Client{}
	c := &AvalancheClient{httpEC: httpEC, wsEC: wsEC}

	if got := c.pickEC(context.Background()); got != httpEC {
		t.Fatal("plain ctx must dispatch to httpEC")
	}
	if got := c.pickEC(eth.WithSyncRoute(context.Background())); got != wsEC {
		t.Fatal("sync-route ctx must dispatch to wsEC")
	}

	// Aliased clients (single underlying rpc.Client serving both) must always
	// dispatch to httpEC regardless of ctx tag.
	c2 := &AvalancheClient{httpEC: httpEC, wsEC: httpEC}
	if got := c2.pickEC(eth.WithSyncRoute(context.Background())); got != httpEC {
		t.Fatal("aliased ECs must dispatch to httpEC regardless of ctx tag")
	}
}

func TestAvalancheClientPickRPC(t *testing.T) {
	httpRPC := &AvalancheRPCClient{}
	wsRPC := &AvalancheRPCClient{}
	c := &AvalancheClient{httpRPC: httpRPC, wsRPC: wsRPC}

	if got := c.pickRPC(context.Background()); got != httpRPC {
		t.Fatal("plain ctx must dispatch to httpRPC")
	}
	if got := c.pickRPC(eth.WithSyncRoute(context.Background())); got != wsRPC {
		t.Fatal("sync-route ctx must dispatch to wsRPC")
	}
}

func TestAvalancheDualRPCClientCallContextRouting(t *testing.T) {
	httpSrv := &jsonrpcServer{}
	wsSrv := &jsonrpcServer{}
	httpHTTP := httptest.NewServer(httpSrv.handler(t))
	defer httpHTTP.Close()
	wsHTTP := httptest.NewServer(wsSrv.handler(t))
	defer wsHTTP.Close()

	// Both endpoints are HTTP for the purposes of this routing test — what
	// matters is that AvalancheDualRPCClient picks SubClient when ctx carries
	// the sync-route tag.
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

	dual := &AvalancheDualRPCClient{
		CallClient: &AvalancheRPCClient{Client: callClient},
		SubClient:  &AvalancheRPCClient{Client: subClient},
	}

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

	if err := dual.CallContext(eth.WithSyncRoute(context.Background()), &result, "eth_blockNumber"); err != nil {
		t.Fatalf("sync-route ctx call failed: %v", err)
	}
	if got := httpSrv.calls.Load(); got != 1 {
		t.Fatalf("sync-route ctx must not hit CallClient: httpCalls=%d, want 1", got)
	}
	if got, want := wsSrv.calls.Load(), int64(1); got != want {
		t.Fatalf("sync-route ctx must hit SubClient: wsCalls=%d, want %d", got, want)
	}
}

func TestAvalancheDualRPCClientBatchAlwaysHTTP(t *testing.T) {
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

	dual := &AvalancheDualRPCClient{
		CallClient: &AvalancheRPCClient{Client: callClient},
		SubClient:  &AvalancheRPCClient{Client: subClient},
	}

	// BatchCallContext must always hit CallClient regardless of ctx tag —
	// matches the eth/DualRPCClient batching contract.
	batch := []rpc.BatchElem{{Method: "eth_blockNumber", Result: new(string)}}
	if err := dual.BatchCallContext(eth.WithSyncRoute(context.Background()), batch); err != nil {
		t.Fatalf("batch call failed: %v", err)
	}
	if got, want := httpSrv.calls.Load(), int64(1); got != want {
		t.Fatalf("batch must hit CallClient: httpCalls=%d, want %d", got, want)
	}
	if got := wsSrv.calls.Load(); got != 0 {
		t.Fatalf("batch must not hit SubClient: wsCalls=%d, want 0", got)
	}
}
