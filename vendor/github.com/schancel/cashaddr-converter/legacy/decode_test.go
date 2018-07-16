package legacy

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDecodeP2KH(t *testing.T) {
	legacy, err := Decode("16UwLL9Risc3QfPqBUvKofHmBQ7wMtjvM")
	assert.Nil(t, err)
	assert.Equal(t, legacy.Version, uint8(0))
	expectedRipemd, _ := hex.DecodeString("010966776006953D5567439E5E39F86A0D273BEE")
	assert.Equal(t, expectedRipemd, legacy.Payload, "Error decoding address payloads")
}

func TestDecodeP2KHLegacyAndCopayLegacy(t *testing.T) {
	legacy, err := Decode("19NoN69ntmV9nKHBjArLJXXCNq3AvvMsqG")
	assert.Nil(t, err)
	copay, err := Decode("CQqgw8VrmpTggTBcQvBFt39DzxFavppafB")
	assert.Nil(t, err)
	assert.Equal(t, legacy.Version, uint8(0))
	assert.Equal(t, copay.Version, uint8(28))
	assert.Equal(t, copay.Payload, legacy.Payload, "Error decoding address payloads")
}

func TestDecodeP2SHLegacyAndCopayLegacy(t *testing.T) {
	legacy, err := Decode("3J98t1WpEZ73CNmQviecrnyiWrnqRhWNLy")
	assert.Nil(t, err)
	copay, err := Decode("HNyFLowu5sKhpYeSnQJmqBWFYWorHKAWDE")
	assert.Nil(t, err)
	assert.Equal(t, legacy.Version, uint8(5))
	assert.Equal(t, copay.Version, uint8(40))
	assert.Equal(t, copay.Payload, legacy.Payload, "Error decoding address payloads")
}
