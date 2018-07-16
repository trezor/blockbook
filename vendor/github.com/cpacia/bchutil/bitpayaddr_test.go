package bchutil

import (
	"testing"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcutil"
	"bytes"
)

func TestDecodeBitpay(t *testing.T) {
	addr, err := DecodeBitpay("CLtewbVQHRvs2J9JzRo58aVCZAVF8VbkXU", &chaincfg.MainNetParams)
	if err != nil {
		t.Error(err)
	}
	if addr.String() != "CLtewbVQHRvs2J9JzRo58aVCZAVF8VbkXU" {
		t.Error("Decoding failed")
	}

	addr2, err := btcutil.DecodeAddress("15RmNZ9LQNxL8AEtJgU9Z4sAw3GqJK99vf", &chaincfg.MainNetParams)
	if err != nil {
		t.Error(err)
	}

	if !bytes.Equal(addr.ScriptAddress(), addr2.ScriptAddress()) {
		t.Error("Script doesn't match original btc address")
	}

	addr, err = DecodeBitpay("HDLrcK5m6yUx3ogjGyjwnevdbfYg26RVUa", &chaincfg.MainNetParams)
	if err != nil {
		t.Error(err)
	}
	if addr.String() != "HDLrcK5m6yUx3ogjGyjwnevdbfYg26RVUa" {
		t.Error("Decoding failed")
	}

	addr2, err = btcutil.DecodeAddress("38Wk9WegFfGHRdohRJ5npGQ6a1Xf9guvs6", &chaincfg.MainNetParams)
	if err != nil {
		t.Error(err)
	}

	if !bytes.Equal(addr.ScriptAddress(), addr2.ScriptAddress()) {
		t.Error("Script doesn't match original btc address")
	}
}
