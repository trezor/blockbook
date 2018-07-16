package cashaddress

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var validSizes = [8]int{20, 24, 28, 32, 40, 48, 56, 64}

func RandomAddress(size int, addrtype uint8, network string) *Address {
	rand.Seed(time.Now().UTC().UnixNano())

	payload := make([]uint8, 0, size)
	for i := 0; i < size; i++ {
		payload = append(payload, uint8(rand.Uint32()))
	}

	return &Address{
		Prefix:  network,
		Version: addrtype,
		Payload: payload,
	}
}

func TestNetworkChange(t *testing.T) {
	for i := 0; i < 10; i++ {
		addr := RandomAddress(20, P2KH, MainNet)
		cashaddr, err := addr.Encode()
		assert.Nil(t, err)

		_, data := splitAddress(cashaddr, MainNet)
		_, err = Decode(fmt.Sprintf("notbitcoincash:%s", data), "")
		assert.NotNil(t, err, fmt.Sprintf("should not have decoded"))
	}
}

func TestExpandPrefix(t *testing.T) {
	expanded := expandPrefix("bitcoincash")
	expected := []uint8{
		'b' & 0x1F, 'i' & 0x1F, 't' & 0x1F, 'c' & 0x1F, 'o' & 0x1F, 'i' & 0x1F, 'n' & 0x1F,
		'c' & 0x1F, 'a' & 0x1F, 's' & 0x1F, 'h' & 0x1F, 0}
	assert.Equal(t, expected, expanded, "expansion invalid")
}

func TestPolyMod(t *testing.T) {
	assert.Equal(t, polyMod([]uint8{}), uint64(0))

	var c uint64
	for ; c < 32; c++ {
		assert.Equal(t, polyMod([]uint8{uint8(c)}), uint64(0x21^c))
		assert.Equal(t, polyMod([]uint8{uint8(c), 0}), uint64(0x401^(c<<5)))
		assert.Equal(t, polyMod([]uint8{uint8(c), 0, 0}), uint64(0x8001^(c<<10)))
		assert.Equal(t, polyMod([]uint8{uint8(c), 0, 0, 0}), uint64(0x100001^(c<<15)))
		assert.Equal(t, polyMod([]uint8{uint8(c), 0, 0, 0, 0}), uint64(0x2000001^(c<<20)))
		assert.Equal(t, polyMod([]uint8{uint8(c), 0, 0, 0, 0, 0}), uint64(0x40000001^(c<<25)))
		assert.Equal(t, polyMod([]uint8{uint8(c), 0, 0, 0, 0, 0, 0}), uint64(0x800000001^(uint64(c)<<30)))
		assert.Equal(t, polyMod([]uint8{uint8(c), 0, 0, 0, 0, 0, 0, 0}), uint64(0x98f2bc8e60^(uint64(c)<<35)))

	}

	assert.Equal(t, polyMod([]uint8{
		0, 0, 0, 0, 0, 0, 0, 0,
		0, 1, 2, 3, 4, 5, 6, 7,
		8, 9, 10, 11, 12, 13, 14, 15,
		16, 17, 18, 19, 20, 21, 22, 23,
		24, 25, 26, 27, 28, 29, 30, 31,
		0, 0, 0, 0, 0, 0, 0, 0,
	}), uint64(0x724afe7af2))
}

func TestAddressEncodeDecode(t *testing.T) {
	payloads := [][]uint8{
		{118, 160, 64, 83, 189, 160, 168, 139, 218, 81, 119, 184, 106, 21, 195, 178, 159, 85, 152, 115},
		{203, 72, 18, 50, 41, 156, 213, 116, 49, 81, 172, 75, 45, 99, 174, 25, 142, 123, 176, 169},
		{1, 31, 40, 228, 115, 201, 95, 64, 19, 215, 213, 62, 197, 251, 195, 180, 45, 248, 237, 16},
	}
	strings := map[uint8][]string{
		P2KH: {
			"bitcoincash:qpm2qsznhks23z7629mms6s4cwef74vcwvy22gdx6a",
			"bitcoincash:qr95sy3j9xwd2ap32xkykttr4cvcu7as4y0qverfuy",
			"bitcoincash:qqq3728yw0y47sqn6l2na30mcw6zm78dzqre909m2r",
		},
		P2SH: {
			"bitcoincash:ppm2qsznhks23z7629mms6s4cwef74vcwvn0h829pq",
			"bitcoincash:pr95sy3j9xwd2ap32xkykttr4cvcu7as4yc93ky28e",
			"bitcoincash:pqq3728yw0y47sqn6l2na30mcw6zm78dzq5ucqzc37",
		},
	}
	for i, payload := range payloads {
		for _, addrtype := range []uint8{P2KH, P2SH} {
			addr := &Address{
				Prefix:  MainNet,
				Version: addrtype,
				Payload: payload,
			}
			addrstr, err := addr.Encode()

			assert.Nil(t, err)
			assert.Equal(t, addrstr, strings[addrtype][i])
			decodedAddr, err := Decode(addrstr, MainNet)
			assert.Nil(t, err)
			assert.Equal(t, payload, decodedAddr.Payload)
			assert.Equal(t, addrtype, decodedAddr.Version)
		}
	}
}
