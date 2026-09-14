//go:build unittest
// +build unittest

package server

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// TestUnmarshalGetAccountInfoRequestPrivatePending verifies the optional privatePending field is
// parsed when present and simply absent otherwise, and that an unknown extra field does not break
// parsing (forward compatibility).
func TestUnmarshalGetAccountInfoRequestPrivatePending(t *testing.T) {
	t.Run("present", func(t *testing.T) {
		r, err := unmarshalGetAccountInfoRequest([]byte(`{"descriptor":"0xabc","privatePending":{"nonces":[42,43],"txids":["0xdead"]}}`))
		if err != nil {
			t.Fatalf("unmarshal error = %v", err)
		}
		if r.PrivatePending == nil {
			t.Fatal("privatePending not parsed")
		}
		if !reflect.DeepEqual(r.PrivatePending.Nonces, []uint64{42, 43}) {
			t.Errorf("nonces = %v, want [42 43]", r.PrivatePending.Nonces)
		}
		if !reflect.DeepEqual(r.PrivatePending.Txids, []string{"0xdead"}) {
			t.Errorf("txids = %v, want [0xdead]", r.PrivatePending.Txids)
		}
	})

	t.Run("absent", func(t *testing.T) {
		r, err := unmarshalGetAccountInfoRequest([]byte(`{"descriptor":"0xabc"}`))
		if err != nil || r.PrivatePending != nil {
			t.Fatalf("got (%+v, %v), want nil privatePending and no error", r.PrivatePending, err)
		}
	})

	t.Run("unknown field ignored", func(t *testing.T) {
		if _, err := unmarshalGetAccountInfoRequest([]byte(`{"descriptor":"0xabc","somethingNew":123}`)); err != nil {
			t.Fatalf("unknown field broke parsing: %v", err)
		}
	})
}

// TestPrivatePendingNonces covers the extraction helper: nil-safe, defensive copy, and the cap.
func TestPrivatePendingNonces(t *testing.T) {
	if got := privatePendingNonces(nil); got != nil {
		t.Errorf("nil input = %v, want nil", got)
	}
	if got := privatePendingNonces(&WsPrivatePending{}); got != nil {
		t.Errorf("empty nonces = %v, want nil", got)
	}

	src := &WsPrivatePending{Nonces: []uint64{7, 8, 9}}
	got := privatePendingNonces(src)
	if !reflect.DeepEqual(got, []uint64{7, 8, 9}) {
		t.Fatalf("got %v, want [7 8 9]", got)
	}
	// mutating the returned slice must not affect the request struct (defensive copy)
	got[0] = 0
	if src.Nonces[0] != 7 {
		t.Error("returned slice aliases the request's backing array")
	}

	// exactly at the cap: kept in full, no truncation
	atCap := make([]uint64, maxPrivatePendingNonces)
	if got := privatePendingNonces(&WsPrivatePending{Nonces: atCap}); len(got) != maxPrivatePendingNonces {
		t.Errorf("at-cap length = %d, want %d (no truncation at the boundary)", len(got), maxPrivatePendingNonces)
	}
	// Over the cap: keep the LOWEST nonces. The pending nonce advances from the backend's answer
	// across a contiguous run, so only the slots just above that answer can be consumed; the high
	// end would strand whatever it contained. Declared descending, so positional truncation would
	// keep exactly the wrong end and the sort is what makes the result order-independent.
	over := make([]uint64, maxPrivatePendingNonces+10)
	for i := range over {
		over[i] = uint64(len(over) - 1 - i)
	}
	overResult := privatePendingNonces(&WsPrivatePending{Nonces: over})
	if len(overResult) != maxPrivatePendingNonces {
		t.Fatalf("over-cap length = %d, want %d", len(overResult), maxPrivatePendingNonces)
	}
	for i, n := range overResult {
		if n != uint64(i) {
			t.Fatalf("over-cap result[%d] = %d, want %d (the lowest nonces, sorted ascending)", i, n, i)
		}
	}
}

// TestPrivatePendingJSONRoundTripsThroughWsReq confirms the field survives the two-stage decode
// (outer WsReq envelope, then params) the server actually uses.
func TestPrivatePendingJSONRoundTripsThroughWsReq(t *testing.T) {
	var req WsReq
	if err := json.Unmarshal([]byte(`{"id":"1","method":"getAccountInfo","params":{"descriptor":"0xabc","privatePending":{"nonces":[5]}}}`), &req); err != nil {
		t.Fatalf("outer unmarshal error = %v", err)
	}
	r, err := unmarshalGetAccountInfoRequest(req.Params)
	if err != nil {
		t.Fatalf("params unmarshal error = %v", err)
	}
	if r.PrivatePending == nil || !reflect.DeepEqual(r.PrivatePending.Nonces, []uint64{5}) {
		t.Fatalf("privatePending = %+v, want nonces [5]", r.PrivatePending)
	}
}

// TestEstimateFeePrivatePendingDecodesIntoSpecific confirms the estimate-side wire contract: a real
// estimateFee envelope carrying specific.privatePending decodes so that r.Specific["privatePending"]
// is the nested object EthereumTypeEstimateGas reads (nonces arriving as JSON float64). Unlike the
// typed getAccountInfo field, the estimate path is an untyped map, so this is the only place the
// nesting is pinned end-to-end.
func TestEstimateFeePrivatePendingDecodesIntoSpecific(t *testing.T) {
	var req WsReq
	body := `{"id":"1","method":"estimateFee","params":{"blocks":[1],"specific":{"from":"0xabc","privatePending":{"nonces":[42],"txids":["0xdead"]}}}}`
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("outer unmarshal error = %v", err)
	}
	var r WsEstimateFeeReq
	if err := json.Unmarshal(req.Params, &r); err != nil {
		t.Fatalf("params unmarshal error = %v", err)
	}
	pp, ok := r.Specific["privatePending"].(map[string]interface{})
	if !ok {
		t.Fatalf("specific.privatePending = %#v, want a nested object", r.Specific["privatePending"])
	}
	nonces, ok := pp["nonces"].([]interface{})
	if !ok || len(nonces) != 1 || nonces[0].(float64) != 42 {
		t.Fatalf("specific.privatePending.nonces = %#v, want [42]", pp["nonces"])
	}
}

// TestPrivatePendingTxids covers the extraction helper: nil-safe, normalization, validation, dedupe
// and the cap. Every declared hash costs a backend lookup when unknown, so junk must never reach it.
func TestPrivatePendingTxids(t *testing.T) {
	const valid = "0x00000000000000000000000000000000000000000000000000000000000000aa"
	const other = "0x00000000000000000000000000000000000000000000000000000000000000bb"

	if got := privatePendingTxids(nil); got != nil {
		t.Errorf("nil input = %v, want nil", got)
	}
	if got := privatePendingTxids(&WsPrivatePending{}); got != nil {
		t.Errorf("empty txids = %v, want nil", got)
	}

	t.Run("normalizes and deduplicates", func(t *testing.T) {
		src := &WsPrivatePending{Txids: []string{strings.ToUpper(valid[2:]) /* no prefix */, "0x" + strings.ToUpper(valid[2:]), valid, other}}
		got := privatePendingTxids(src)
		if !reflect.DeepEqual(got, []string{valid, other}) {
			t.Fatalf("got %v, want [%s %s]", got, valid, other)
		}
	})

	t.Run("drops malformed hashes", func(t *testing.T) {
		src := &WsPrivatePending{Txids: []string{"0xdead", valid[:65], valid + "aa", "0x" + strings.Repeat("z", 64), ""}}
		if got := privatePendingTxids(src); got != nil {
			t.Fatalf("got %v, want nil - none of these can match a mempool entry", got)
		}
	})

	t.Run("caps the list", func(t *testing.T) {
		atCap := make([]string, maxPrivatePendingTxids)
		for i := range atCap {
			atCap[i] = fmt.Sprintf("0x%064x", i)
		}
		if got := privatePendingTxids(&WsPrivatePending{Txids: atCap}); len(got) != maxPrivatePendingTxids {
			t.Fatalf("at cap: got %d txids, want %d", len(got), maxPrivatePendingTxids)
		}
		overCap := append(append([]string{}, atCap...), fmt.Sprintf("0x%064x", maxPrivatePendingTxids))
		got := privatePendingTxids(&WsPrivatePending{Txids: overCap})
		if len(got) != maxPrivatePendingTxids || got[0] != atCap[0] {
			t.Fatalf("over cap: got %d txids starting %s, want %d starting %s", len(got), got[0], maxPrivatePendingTxids, atCap[0])
		}
	})

	t.Run("defensive copy", func(t *testing.T) {
		src := &WsPrivatePending{Txids: []string{valid}}
		got := privatePendingTxids(src)
		got[0] = other
		if src.Txids[0] != valid {
			t.Fatalf("request txids mutated to %v, want the helper to return its own slice", src.Txids)
		}
	})
}
