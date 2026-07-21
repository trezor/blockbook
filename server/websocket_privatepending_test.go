//go:build unittest
// +build unittest

package server

import (
	"encoding/json"
	"reflect"
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
	// over the cap: collapses to the single highest nonce, so the pending floor is still correct
	// even when the maximum is not at a positional index the old first-N truncation would keep.
	over := make([]uint64, maxPrivatePendingNonces+10)
	for i := range over {
		over[i] = uint64(i) // ascending: the true max sits at the last index, beyond the cap
	}
	overResult := privatePendingNonces(&WsPrivatePending{Nonces: over})
	if len(overResult) != 1 || overResult[0] != uint64(len(over)-1) {
		t.Errorf("over-cap result = %v, want [%d] (highest nonce preserved)", overResult, len(over)-1)
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
