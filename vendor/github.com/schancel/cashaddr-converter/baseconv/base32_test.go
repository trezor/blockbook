package baseconv

import (
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func RandomBytes(size int) []byte {
	rand.Seed(time.Now().UTC().UnixNano())

	out := make([]uint8, 0, size)
	for i := 0; i < size; i++ {
		out = append(out, uint8(rand.Uint32()))
	}

	return out
}

func TestBase32(t *testing.T) {
	bytes := RandomBytes(20)
	inbase, _ := ConvertBits(8, 5, bytes, true)
	strrep, err := ToBase32(inbase)
	assert.Nil(t, err)
	frombase, err := FromBase32(strrep)
	assert.Nil(t, err)
	recovered, _ := ConvertBits(5, 8, frombase, false)
	assert.Equal(t, recovered, bytes, "value mutated during base32 conversion")
}
