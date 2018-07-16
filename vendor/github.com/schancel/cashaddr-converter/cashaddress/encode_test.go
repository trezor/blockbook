package cashaddress

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPackAddress(t *testing.T) {
	for i, size := range validSizes {
		addr := RandomAddress(size, P2SH, MainNet)
		packed, err := packAddress(addr)
		assert.Nil(t, err)

		// Size is in the second byte, shifted left 2
		assert.Equal(t, uint8(i), packed[1]>>2)
		assert.Equal(t, true, true)
	}
}

func TestAppendChecksum(t *testing.T) {
	addr := RandomAddress(20, P2SH, MainNet)
	packed, err := packAddress(addr)
	assert.Nil(t, err)
	chksum := calculateChecksum(MainNet, append(packed, 0, 0, 0, 0, 0, 0, 0, 0))
	wchk := appendChecksum(packed, chksum)
	assert.Equal(t, calculateChecksum(MainNet, wchk), uint64(0), "failed to append checksum correctly")
}

func TestEncodeSizes(t *testing.T) {
	for _, size := range validSizes {
		addr := RandomAddress(size, P2SH, MainNet)
		cashaddr, err := addr.Encode()
		assert.Nil(t, err)
		assert.NotEqual(t, "", cashaddr)

		addr = RandomAddress(size-1, P2SH, MainNet)
		cashaddr, err = addr.Encode()
		assert.NotNil(t, err)
		assert.Equal(t, "", cashaddr)

	}
}
