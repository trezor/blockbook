//go:build unittest

package api

import (
	"fmt"
	"testing"

	"github.com/trezor/blockbook/bchain"
)

func TestValidateXpubScanLimits(t *testing.T) {
	if err := validateXpubScanLimits(&bchain.XpubDescriptor{ChangeIndexes: []uint32{0, 1}}, maxAddressesGap+1); err != nil {
		t.Fatalf("expected default change indexes at max gap to pass, got %v", err)
	}

	changes := make([]uint32, bchain.MaxXpubChangeIndexes+1)
	if err := validateXpubScanLimits(&bchain.XpubDescriptor{ChangeIndexes: changes}, defaultAddressesGap+1); err == nil {
		t.Fatal("expected change index count above limit to fail")
	}

	changes = make([]uint32, 3)
	if err := validateXpubScanLimits(&bchain.XpubDescriptor{ChangeIndexes: changes}, maxAddressesGap+1); err == nil {
		t.Fatal("expected scan size above limit to fail")
	}
}

func TestTrimXpubCacheItemsLocked(t *testing.T) {
	cachedXpubsMux.Lock()
	defer cachedXpubsMux.Unlock()

	originalCache := cachedXpubs
	defer func() {
		cachedXpubs = originalCache
	}()

	cachedXpubs = make(map[string]xpubData, xpubCacheMaxEntries+2)
	for i := 0; i < xpubCacheMaxEntries+2; i++ {
		cachedXpubs[fmt.Sprintf("xpub-%03d", i)] = xpubData{accessed: int64(i)}
	}

	if got := trimXpubCacheItemsLocked(); got != 2 {
		t.Fatalf("trimXpubCacheItemsLocked() evicted %d entries, want 2", got)
	}
	if got := len(cachedXpubs); got != xpubCacheMaxEntries {
		t.Fatalf("cachedXpubs length = %d, want %d", got, xpubCacheMaxEntries)
	}
	if _, ok := cachedXpubs["xpub-000"]; ok {
		t.Fatal("oldest cache entry was not evicted")
	}
	if _, ok := cachedXpubs["xpub-001"]; ok {
		t.Fatal("second oldest cache entry was not evicted")
	}
}
