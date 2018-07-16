package address

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHex(t *testing.T) {
	addr := &Address{
		Payload: []uint8{0x0F, 0xF0, 0x0C},
	}

	assert.Equal(t, addr.Hex(), "0x0FF00C")
}
