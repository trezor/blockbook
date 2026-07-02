//go:build unittest

package db

import (
	"os"
	"testing"

	"github.com/trezor/blockbook/common"
)

func TestRocksDB_RuntimeSettings(t *testing.T) {
	parser := &testBitcoinParser{
		BitcoinParser: bitcoinTestnetParser(),
	}
	d := setupRocksDB(t, parser)
	path := d.path
	dClosed := false
	defer func() {
		if !dClosed {
			if err := d.Close(); err != nil {
				t.Error(err)
			}
		}
		os.RemoveAll(path)
	}()

	getSetting := func(db *RocksDB, name string) (string, bool) {
		t.Helper()
		value, found, err := db.GetRuntimeSetting(name)
		if err != nil {
			t.Fatalf("GetRuntimeSetting(%q) unexpected error: %v", name, err)
		}
		return value, found
	}

	if value, found := getSetting(d, "MISSING"); found || value != "" {
		t.Fatalf("GetRuntimeSetting(MISSING) = %q, %v, want empty, false", value, found)
	}

	if err := d.StoreRuntimeSetting("ALLOWED_RPC_CALL_TO", "0xabcd,0xef01"); err != nil {
		t.Fatal(err)
	}
	if value, found := getSetting(d, "ALLOWED_RPC_CALL_TO"); !found || value != "0xabcd,0xef01" {
		t.Fatalf("GetRuntimeSetting after store = %q, %v, want 0xabcd,0xef01, true", value, found)
	}

	// overwrite
	if err := d.StoreRuntimeSetting("ALLOWED_RPC_CALL_TO", "0x1234"); err != nil {
		t.Fatal(err)
	}
	if value, found := getSetting(d, "ALLOWED_RPC_CALL_TO"); !found || value != "0x1234" {
		t.Fatalf("GetRuntimeSetting after overwrite = %q, %v, want 0x1234, true", value, found)
	}

	// an explicitly stored empty value must stay distinguishable from a missing row
	if err := d.StoreRuntimeSetting("ALLOWED_EVM_CALL_METHODS", ""); err != nil {
		t.Fatal(err)
	}
	if value, found := getSetting(d, "ALLOWED_EVM_CALL_METHODS"); !found || value != "" {
		t.Fatalf("GetRuntimeSetting of stored empty value = %q, %v, want empty, true", value, found)
	}

	if err := d.DeleteRuntimeSetting("ALLOWED_EVM_CALL_METHODS"); err != nil {
		t.Fatal(err)
	}
	if value, found := getSetting(d, "ALLOWED_EVM_CALL_METHODS"); found || value != "" {
		t.Fatalf("GetRuntimeSetting after delete = %q, %v, want empty, false", value, found)
	}
	// deleting a missing row is not an error
	if err := d.DeleteRuntimeSetting("ALLOWED_EVM_CALL_METHODS"); err != nil {
		t.Fatal(err)
	}

	// settings survive a database reopen
	dClosed = true
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	d2, err := NewRocksDB(path, 100000, -1, parser, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := d2.Close(); err != nil {
			t.Error(err)
		}
	}()
	is, err := d2.LoadInternalState(&common.Config{CoinName: "coin-unittest"})
	if err != nil {
		t.Fatal(err)
	}
	d2.SetInternalState(is)
	if value, found := getSetting(d2, "ALLOWED_RPC_CALL_TO"); !found || value != "0x1234" {
		t.Fatalf("GetRuntimeSetting after reopen = %q, %v, want 0x1234, true", value, found)
	}
}
