//go:build unittest

package api

import (
	"fmt"
	"testing"

	"github.com/trezor/blockbook/bchain"
)

func TestDefaultXpubConfigDerivedFields(t *testing.T) {
	cfg := DefaultXpubConfig()
	if cfg.MaxAddressDerivations != (cfg.MaxAddressesGap+1)*2 {
		t.Fatalf("DefaultXpubConfig().MaxAddressDerivations = %d, want %d", cfg.MaxAddressDerivations, (cfg.MaxAddressesGap+1)*2)
	}
	if cfg.MaxAddressDerivations == 0 {
		t.Fatal("DefaultXpubConfig().MaxAddressDerivations must be non-zero, otherwise every xpub scan is rejected")
	}
	// The default config must accept a normal scan at the default gap; this is
	// the value NewWorker hands to validateXpubScanLimits when a coin has no
	// xpubConfig override.
	if err := validateXpubScanLimits(&bchain.XpubDescriptor{ChangeIndexes: []uint32{0, 1}}, cfg.DefaultAddressesGap+1, cfg.MaxAddressDerivations); err != nil {
		t.Fatalf("default config rejected a default-gap scan: %v", err)
	}
}

func TestApplyXpubConfigClampsAndDerives(t *testing.T) {
	// A default gap larger than the max gap must be clamped down so it never
	// exceeds the accepted maximum.
	cfg := ApplyXpubConfig(&bchain.XpubConfig{DefaultAddressesGap: 50, MaxAddressesGap: 20})
	if cfg.DefaultAddressesGap > cfg.MaxAddressesGap {
		t.Fatalf("DefaultAddressesGap = %d not clamped to MaxAddressesGap = %d", cfg.DefaultAddressesGap, cfg.MaxAddressesGap)
	}
	if cfg.MaxAddressDerivations != (cfg.MaxAddressesGap+1)*2 {
		t.Fatalf("MaxAddressDerivations = %d, want %d", cfg.MaxAddressDerivations, (cfg.MaxAddressesGap+1)*2)
	}

	// A nil override yields the finalized defaults.
	if got, want := ApplyXpubConfig(nil), DefaultXpubConfig(); got != want {
		t.Fatalf("ApplyXpubConfig(nil) = %+v, want %+v", got, want)
	}
}

func TestValidateXpubScanLimits(t *testing.T) {
	const testMaxDerivations = 20002
	if err := validateXpubScanLimits(&bchain.XpubDescriptor{ChangeIndexes: []uint32{0, 1}}, 10001, testMaxDerivations); err != nil {
		t.Fatalf("expected default change indexes at max gap to pass, got %v", err)
	}

	changes := make([]uint32, bchain.MaxXpubChangeIndexes+1)
	if err := validateXpubScanLimits(&bchain.XpubDescriptor{ChangeIndexes: changes}, 21, testMaxDerivations); err == nil {
		t.Fatal("expected change index count above limit to fail")
	}

	changes = make([]uint32, 3)
	if err := validateXpubScanLimits(&bchain.XpubDescriptor{ChangeIndexes: changes}, 10001, testMaxDerivations); err == nil {
		t.Fatal("expected scan size above limit to fail")
	}
}

func TestTrimXpubCacheItemsLocked(t *testing.T) {
	const testMaxEntries = 128
	cachedXpubsMux.Lock()
	defer cachedXpubsMux.Unlock()

	originalCache := cachedXpubs
	defer func() {
		cachedXpubs = originalCache
	}()

	cachedXpubs = make(map[string]xpubData, testMaxEntries+2)
	for i := 0; i < testMaxEntries+2; i++ {
		cachedXpubs[fmt.Sprintf("xpub-%03d", i)] = xpubData{accessed: int64(i)}
	}

	if got := trimXpubCacheItemsLocked(testMaxEntries); got != 2 {
		t.Fatalf("trimXpubCacheItemsLocked() evicted %d entries, want 2", got)
	}
	if got := len(cachedXpubs); got != testMaxEntries {
		t.Fatalf("cachedXpubs length = %d, want %d", got, testMaxEntries)
	}
	if _, ok := cachedXpubs["xpub-000"]; ok {
		t.Fatal("oldest cache entry was not evicted")
	}
	if _, ok := cachedXpubs["xpub-001"]; ok {
		t.Fatal("second oldest cache entry was not evicted")
	}
}

func TestMergeXpubTxidsDeduplicatesAndSorts(t *testing.T) {
	data := &xpubData{
		txCountEstimate: 4,
		addresses: [][]xpubAddress{
			{
				{txids: xpubTxids{
					{txid: "duplicate", height: 5, inputOutput: txOutput},
					{txid: "newest", height: 7, inputOutput: txOutput},
				}},
			},
			{
				{txids: xpubTxids{
					{txid: "duplicate", height: 5, inputOutput: txInput},
					{txid: "same-height-input", height: 5, inputOutput: txInput},
				}},
			},
		},
	}

	txids := mergeXpubTxids(data)
	got := make([]string, len(txids))
	for i := range txids {
		got[i] = txids[i].txid
	}
	want := []string{"newest", "same-height-input", "duplicate"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("mergeXpubTxids order = %v, want %v", got, want)
	}
	if txids[2].inputOutput != txOutput {
		t.Fatal("mergeXpubTxids did not preserve the first duplicate occurrence")
	}
}

func TestIsUnfilteredXpubTxidFilter(t *testing.T) {
	if !isUnfilteredXpubTxidFilter(&AddressFilter{Vout: AddressFilterVoutOff}) {
		t.Fatal("default xpub txid filter should be unfiltered")
	}
	if isUnfilteredXpubTxidFilter(&AddressFilter{Vout: AddressFilterVoutInputs}) {
		t.Fatal("input filter should not be treated as unfiltered")
	}
	if isUnfilteredXpubTxidFilter(&AddressFilter{Vout: AddressFilterVoutOff, FromHeight: 1}) {
		t.Fatal("height filter should not be treated as unfiltered")
	}
}
