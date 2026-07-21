package eth

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
)

// TestEthereumTypeEstimateGasRoutesOnPrivatePendingHint verifies the declared privatePending hint
// short-circuits routing: a sender that is NOT a recent private sender (so useForNonces is false and
// the estimate would otherwise go to the primary backend) is routed to the alternative provider
// purely because the request declared an in-flight private nonce.
func TestEthereumTypeEstimateGasRoutesOnPrivatePendingHint(t *testing.T) {
	primary, primaryHits := countingEstimateGasServer(t, "0x5208")
	provider, providerHits := countingEstimateGasServer(t, "0x9999")
	b := newEstimateGasTestRPC(t, primary.URL, provider.URL)

	gas, err := b.EthereumTypeEstimateGas(map[string]interface{}{
		"from":           "0x2222222222222222222222222222222222222222",
		"to":             "0x3333333333333333333333333333333333333333",
		"privatePending": map[string]interface{}{"nonces": []interface{}{float64(42)}},
	})
	if err != nil {
		t.Fatalf("EthereumTypeEstimateGas() error = %v", err)
	}
	if gas != 0x9999 {
		t.Fatalf("gas = %#x, want 0x9999 (provider value)", gas)
	}
	if got := atomic.LoadInt32(providerHits); got != 1 {
		t.Errorf("provider hits = %d, want 1 (hint must route despite no recent send)", got)
	}
	if got := atomic.LoadInt32(primaryHits); got != 0 {
		t.Errorf("primary hits = %d, want 0", got)
	}
}

// TestEthereumTypeEstimateGasHintStripsPrivatePendingFromRelayCall confirms the wallet's
// privatePending bookkeeping is not forwarded as part of the eth_estimateGas call object sent to the
// relay - only the real tx-call fields (from/to/…) are.
func TestEthereumTypeEstimateGasHintStripsPrivatePendingFromRelayCall(t *testing.T) {
	primary, _ := countingEstimateGasServer(t, "0x5208")

	var gotParams map[string]interface{}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Params []map[string]interface{} `json:"params"`
		}
		_ = json.Unmarshal(body, &req)
		if len(req.Params) > 0 {
			gotParams = req.Params[0]
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x9999"}`))
	}))
	t.Cleanup(provider.Close)
	b := newEstimateGasTestRPC(t, primary.URL, provider.URL)

	if _, err := b.EthereumTypeEstimateGas(map[string]interface{}{
		"from":           "0x2222222222222222222222222222222222222222",
		"privatePending": map[string]interface{}{"nonces": []interface{}{float64(42)}, "txids": []interface{}{"0xdead"}},
	}); err != nil {
		t.Fatalf("EthereumTypeEstimateGas() error = %v", err)
	}
	if gotParams == nil {
		t.Fatal("provider was not called")
	}
	if _, present := gotParams["privatePending"]; present {
		t.Errorf("privatePending was forwarded to eth_estimateGas: %v", gotParams)
	}
	if gotParams["from"] != "0x2222222222222222222222222222222222222222" {
		t.Errorf("from not forwarded to the relay call: %v", gotParams["from"])
	}
}

func TestEstimatePrivatePendingDeclared(t *testing.T) {
	cases := []struct {
		name   string
		params map[string]interface{}
		want   bool
	}{
		{"absent", map[string]interface{}{"from": "0x1"}, false},
		{"empty object", map[string]interface{}{"privatePending": map[string]interface{}{}}, false},
		{"empty nonces", map[string]interface{}{"privatePending": map[string]interface{}{"nonces": []interface{}{}}}, false},
		{"declared", map[string]interface{}{"privatePending": map[string]interface{}{"nonces": []interface{}{float64(42)}}}, true},
		{"wrong type", map[string]interface{}{"privatePending": "nope"}, false},
		{"nonces wrong type", map[string]interface{}{"privatePending": map[string]interface{}{"nonces": "nope"}}, false},
	}
	for _, c := range cases {
		if got := estimatePrivatePendingDeclared(c.params); got != c.want {
			t.Errorf("%s: estimatePrivatePendingDeclared = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestEstimateParamsWithoutPrivatePending(t *testing.T) {
	// absent: returns the same map, no copy
	noHint := map[string]interface{}{"from": "0x1", "to": "0x2"}
	if got := estimateParamsWithoutPrivatePending(noHint); !reflect.DeepEqual(got, noHint) {
		t.Errorf("no-hint result = %v, want unchanged %v", got, noHint)
	}

	// present: privatePending removed, other fields kept, input not mutated
	withHint := map[string]interface{}{
		"from":           "0x1",
		"privatePending": map[string]interface{}{"nonces": []interface{}{float64(1)}},
	}
	got := estimateParamsWithoutPrivatePending(withHint)
	if _, present := got["privatePending"]; present {
		t.Error("privatePending not removed from returned params")
	}
	if got["from"] != "0x1" {
		t.Errorf("from = %v, want 0x1 (other fields must be kept)", got["from"])
	}
	if _, present := withHint["privatePending"]; !present {
		t.Error("input map was mutated (privatePending removed from caller's map)")
	}
}
