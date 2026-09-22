//go:build unittest

package eth

import (
	"crypto/rand"
	"fmt"
	"sync"
	"testing"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/trezor/blockbook/bchain"
)

// the cached checksum must agree with go-ethereum's reference implementation
func TestEIP55Address_MatchesReference(t *testing.T) {
	for i := 0; i < 2000; i++ {
		var a ethcommon.Address
		rand.Read(a[:])
		want := a.Hex()
		for pass := 0; pass < 2; pass++ {
			if got := EIP55Address(a[:]); got != want {
				t.Fatalf("pass %d: EIP55Address(%x) = %s, want %s", pass, a, got, want)
			}
		}
		if got := EIP55AddressFromAddress(a.Hex()); got != want {
			t.Fatalf("EIP55AddressFromAddress(%s) = %s", want, got)
		}
	}
	for _, desc := range []bchain.AddressDescriptor{nil, {}, {1, 2, 3}, make([]byte, 21)} {
		if got, want := EIP55Address(desc), fmt.Sprintf("0x%x", []byte(desc)); got != want {
			t.Errorf("EIP55Address(%x) = %s, want %s", desc, got, want)
		}
	}
}

func TestAddressFormatCache_AgesOut(t *testing.T) {
	c := NewAddressFormatCache(addressFormatShards * 2)
	calls := 0
	format := func(d bchain.AddressDescriptor) string {
		calls++
		return fmt.Sprintf("%x", []byte(d))
	}
	// all keys land in one shard: same last byte
	key := func(i int) bchain.AddressDescriptor { return bchain.AddressDescriptor{byte(i), byte(i >> 8), 7} }
	for i := 0; i < 2; i++ {
		c.Format(key(i), format)
	}
	if c.Format(key(0), format); calls != 2 {
		t.Fatalf("hit recomputed, calls = %d", calls)
	}
	// filling the current generation pushes the first keys to the previous one
	c.Format(key(2), format)
	c.Format(key(3), format)
	if c.Format(key(0), format); calls != 4 {
		t.Fatalf("previous generation not consulted, calls = %d", calls)
	}
	// a second rollover drops them: the next lookup recomputes and re-inserts
	c.Format(key(4), format)
	c.Format(key(5), format)
	if c.Format(key(0), format); calls != 7 {
		t.Fatalf("aged-out key served, calls = %d", calls)
	}
	if got := c.Format(key(0), format); got != "000007" || calls != 7 {
		t.Fatalf("re-inserted key: %s, calls = %d", got, calls)
	}
}

func TestAddressFormatCache_Concurrent(t *testing.T) {
	c := NewAddressFormatCache(256)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 5000; i++ {
				d := bchain.AddressDescriptor{byte(i), byte(i >> 8), byte(g)}
				want := fmt.Sprintf("%x", []byte(d))
				if got := c.Format(d, func(d bchain.AddressDescriptor) string { return fmt.Sprintf("%x", []byte(d)) }); got != want {
					t.Errorf("got %s, want %s", got, want)
					return
				}
			}
		}(g)
	}
	wg.Wait()
}
