package baseconv

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConvertBits(t *testing.T) {
	five, padded := ConvertBits(8, 5, []uint8{0xFF}, true)
	assert.Equal(t, true, padded, "should have been padded")
	assert.Equal(t, []uint8{0x1F, 0x1C}, five, "resulting bits incorrect")
	eight, padded := ConvertBits(5, 8, five, false)
	assert.Equal(t, false, padded, "should not have been padded")
	assert.Equal(t, eight, []uint8{0xFF}, "decoding failed")
}
