//go:build unittest

package db

import (
	"testing"

	"github.com/trezor/blockbook/bchain"
)

// Test_deleteAllAddressAliases verifies that wiping the alias column family also
// clears the in-memory cache, so a stale alias is not served after the wipe.
func Test_deleteAllAddressAliases(t *testing.T) {
	d := setupRocksDB(t, &testEthereumParser{EthereumParser: ethereumTestnetParser()})
	defer closeAndDestroyRocksDB(t, d)

	records := []bchain.AddressAliasRecord{
		{Address: "0x2C630b16Aa53ae0189880e15C23323688acb607c", Name: "unraveled"},
		{Address: "0x1111111111111111111111111111111111111111", Name: "foo"},
	}
	if err := d.storeAddressAliasRecordsBatch(records); err != nil {
		t.Fatal(err)
	}
	// Reading through GetAddressAlias also seeds the LRU cache, so the wipe has to
	// clear both the column family and the cache to return empty afterwards.
	for _, r := range records {
		if got := d.GetAddressAlias(r.Address); got == "" {
			t.Fatalf("GetAddressAlias(%s) empty before wipe, expected a value", r.Address)
		}
	}
	if err := d.deleteAllAddressAliases(); err != nil {
		t.Fatal(err)
	}
	for _, r := range records {
		if got := d.GetAddressAlias(r.Address); got != "" {
			t.Errorf("GetAddressAlias(%s) = %q after wipe, want empty (CF + cache cleared)", r.Address, got)
		}
	}
}

// Test_storeAddressAliasRecordsBatch_Empty verifies the batch writer is a no-op
// for an empty slice (no write, no panic).
func Test_storeAddressAliasRecordsBatch_Empty(t *testing.T) {
	d := setupRocksDB(t, &testEthereumParser{EthereumParser: ethereumTestnetParser()})
	defer closeAndDestroyRocksDB(t, d)
	if err := d.storeAddressAliasRecordsBatch(nil); err != nil {
		t.Fatal(err)
	}
}
