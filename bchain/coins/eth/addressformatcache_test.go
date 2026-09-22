//go:build unittest

package eth

import (
	"crypto/rand"
	"fmt"
	"sync"
	"testing"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/trezor/blockbook/bchain"
	"golang.org/x/crypto/sha3"
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

// the Sum fallback must produce the same checksum as the squeeze path
func TestEIP55Checksum_SumFallback(t *testing.T) {
	k := &eip55Scratch{keccak: sha3.NewLegacyKeccak256()}
	for i := 0; i < 500; i++ {
		var a ethcommon.Address
		rand.Read(a[:])
		if got := k.checksum(a[:]); got != a.Hex() {
			t.Fatalf("checksum(%x) = %s, want %s", a, got, a.Hex())
		}
	}
}

func TestAddressFormatCache_Replacement(t *testing.T) {
	c := NewAddressFormatCache(2)
	calls := 0
	format := func(d bchain.AddressDescriptor) string {
		calls++
		return fmt.Sprintf("%x", []byte(d))
	}
	// the slot is the lowest bit of the first of the last eight bytes (little endian):
	// a and b collide, other does not
	a := bchain.AddressDescriptor{1, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	b := bchain.AddressDescriptor{2, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	other := bchain.AddressDescriptor{3, 0, 1, 0, 0, 0, 0, 0, 0, 0}
	if got := c.Format(a, format); got != "01000000000000000000" || calls != 1 {
		t.Fatalf("first format: %s, calls %d", got, calls)
	}
	if c.Format(a, format); calls != 1 {
		t.Fatalf("hit recomputed, calls = %d", calls)
	}
	c.Format(other, format)
	if c.Format(a, format); calls != 2 {
		t.Fatalf("other slot evicted a, calls = %d", calls)
	}
	c.Format(b, format)
	if got := c.Format(a, format); got != "01000000000000000000" || calls != 4 {
		t.Fatalf("colliding key must evict: %s, calls = %d", got, calls)
	}
	if c.Format(other, format); calls != 4 {
		t.Fatalf("other slot disturbed, calls = %d", calls)
	}
	// short and over-long descriptors bypass the table
	for _, d := range []bchain.AddressDescriptor{{1}, make([]byte, addressFormatKeyMax+1)} {
		before := calls
		c.Format(d, format)
		c.Format(d, format)
		if calls != before+2 {
			t.Fatalf("descriptor of %d bytes was cached", len(d))
		}
	}
	if c.slots.Load() == nil {
		t.Fatal("slots not allocated after use")
	}
	if NewAddressFormatCache(1<<17).slots.Load() != nil {
		t.Fatal("slots allocated before first use")
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
