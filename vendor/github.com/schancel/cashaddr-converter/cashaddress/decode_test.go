package cashaddress

import (
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDecode(t *testing.T) {
	addr := RandomAddress(20, P2SH, MainNet)
	cashaddr, err := addr.Encode()
	assert.Nil(t, err)
	daddr, err := Decode(cashaddr, MainNet)
	assert.Nil(t, err)
	assert.Equal(t, addr.Version, daddr.Version, "decoding type failed")
	assert.Equal(t, addr.Payload, daddr.Payload, "decoding payload failed")
}

func TestChecksumFails(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())

	addr := RandomAddress(20, P2KH, MainNet)
	packed, err := packAddress(addr)
	assert.Nil(t, err)
	chksum := calculateChecksum("bitcoincash", append(packed, 0, 0, 0, 0, 0, 0, 0, 0))
	wchk := appendChecksum(packed, chksum)
	errors := int(rand.Int31n(10) + 1)
	for r := 0; r < errors; r++ {
		position := rand.Int31n(int32(len(wchk)))
		wchk[position] ^= uint8(rand.Int31n(32))
	}
	assert.NotEqual(t, uint64(0), calculateChecksum("bitcoincash", wchk), "checksum validation should have failed")
}

func TestChecksum(t *testing.T) {
	// Checksums are valid, but payload is not
	values := []string{
		"prefix:x64nx6hz",
		"p:gpf8m4h7",
		"bitcoincash:qpzry9x8gf2tvdw0s3jn54khce6mua7lcw20ayyn",
		"bchtest:testnetaddress4d6njnut",
		"bchreg:555555555555555555555555555555555555555555555udxmlmrz",
	}
	for _, addr := range values {
		_, err := Decode(addr, MainNet)
		assert.NotNil(t, err)
		// These fail for a variety of reasons, but checksum should not be one
		// of them.
		assert.NotEqual(t, err.Error(), "checksum verification failed")
	}
}

func TestUpperLower(t *testing.T) {
	// Checksums are valid, but payload is not
	values := []string{
		"bitcoincash:qpm2qsznhKs23z7629mms6s4cwef74vcwvy22gdx6a",
	}
	for _, addr := range values {
		_, err := Decode(addr, MainNet)
		assert.NotNil(t, err)
		assert.Equal(t, err.Error(), "cashaddress contains mixed upper and lowercase characters")
	}
}
