//go:build unittest

package db

import (
	"testing"

	"github.com/trezor/blockbook/bchain"
)

// fakeEnsRebuilder is a minimal EnsAliasRebuilder for exercising the db-side
// RebuildEnsAliases orchestration without a real EthereumRPC.
type fakeEnsRebuilder struct {
	records []bchain.AddressAliasRecord
	called  bool
}

func (f *fakeEnsRebuilder) RebuildEnsAliases(_, _ uint32, _ int, _ func() bool, store func([]bchain.AddressAliasRecord) error) error {
	f.called = true
	return store(f.records)
}

// Test_replaceAddressAliases verifies the atomic swap: listed keys are written,
// keys absent from the new set are dropped, and the in-memory cache is cleared.
func Test_replaceAddressAliases(t *testing.T) {
	d := setupRocksDB(t, &testEthereumParser{EthereumParser: ethereumTestnetParser()})
	defer closeAndDestroyRocksDB(t, d)

	const (
		keep  = "0x2C630b16Aa53ae0189880e15C23323688acb607c"
		spoof = "0x1111111111111111111111111111111111111111"
		fresh = "0x2222222222222222222222222222222222222222"
	)
	// Seed a "kept" and a "spoofed" alias, and prime the cache for both.
	if err := d.replaceAddressAliases(map[string]string{keep: "old", spoof: "ransomware"}); err != nil {
		t.Fatal(err)
	}
	if got := d.GetAddressAlias(spoof); got == "" {
		t.Fatalf("GetAddressAlias(spoof) empty before swap, expected a value")
	}

	// New set drops the spoofed key, updates keep, adds fresh.
	if err := d.replaceAddressAliases(map[string]string{keep: "new", fresh: "vitalik"}); err != nil {
		t.Fatal(err)
	}
	if got := d.GetAddressAlias(spoof); got != "" {
		t.Errorf("GetAddressAlias(spoof) = %q after swap, want empty (dropped + cache cleared)", got)
	}
	if got, want := d.GetAddressAlias(keep), d.chainParser.FormatAddressAlias(keep, "new"); got != want {
		t.Errorf("GetAddressAlias(keep) = %q after swap, want %q", got, want)
	}
	if got, want := d.GetAddressAlias(fresh), d.chainParser.FormatAddressAlias(fresh, "vitalik"); got != want {
		t.Errorf("GetAddressAlias(fresh) = %q after swap, want %q", got, want)
	}
}

// Test_RebuildEnsAliases_EmptyScanWipes documents the deliberate purge: running
// the flag when the scan yields nothing (e.g. no ens_registrars) wipes the CF.
func Test_RebuildEnsAliases_EmptyScanWipes(t *testing.T) {
	d := setupRocksDB(t, &testEthereumParser{EthereumParser: ethereumTestnetParser()})
	defer closeAndDestroyRocksDB(t, d)

	const existing = "0x2C630b16Aa53ae0189880e15C23323688acb607c"
	if err := d.replaceAddressAliases(map[string]string{existing: "legacy"}); err != nil {
		t.Fatal(err)
	}
	f := &fakeEnsRebuilder{records: nil} // scan finds nothing (trust-none coin)
	if err := d.RebuildEnsAliases(f, 0, 100, 0, nil); err != nil {
		t.Fatal(err)
	}
	if !f.called {
		t.Error("scan did not run")
	}
	if got := d.GetAddressAlias(existing); got != "" {
		t.Errorf("GetAddressAlias(existing) = %q after empty rebuild, want empty (flag wipes)", got)
	}
}

// Test_RebuildEnsAliases_Swaps verifies the full path: with registrars present,
// the scanned set replaces the CF (old, unlisted aliases are purged).
func Test_RebuildEnsAliases_Swaps(t *testing.T) {
	d := setupRocksDB(t, &testEthereumParser{EthereumParser: ethereumTestnetParser()})
	defer closeAndDestroyRocksDB(t, d)

	const (
		stale = "0x1111111111111111111111111111111111111111"
		want  = "0x2C630b16Aa53ae0189880e15C23323688acb607c"
	)
	if err := d.replaceAddressAliases(map[string]string{stale: "spoofed"}); err != nil {
		t.Fatal(err)
	}
	f := &fakeEnsRebuilder{records: []bchain.AddressAliasRecord{{Address: want, Name: "unraveled"}}}
	if err := d.RebuildEnsAliases(f, 0, 100, 0, nil); err != nil {
		t.Fatal(err)
	}
	if !f.called {
		t.Error("scan did not run despite registrars present")
	}
	if got := d.GetAddressAlias(stale); got != "" {
		t.Errorf("GetAddressAlias(stale) = %q, want empty (should be purged by swap)", got)
	}
	if got, wantAlias := d.GetAddressAlias(want), d.chainParser.FormatAddressAlias(want, "unraveled"); got != wantAlias {
		t.Errorf("GetAddressAlias(want) = %q, want %q", got, wantAlias)
	}
}
