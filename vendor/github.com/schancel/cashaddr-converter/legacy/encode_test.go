package legacy

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEncodeP2KH(t *testing.T) {
	payload, _ := hex.DecodeString("010966776006953D5567439E5E39F86A0D273BEE")
	legacy := &Address{
		Version: P2KH,
		Payload: payload,
	}
	address, err := legacy.Encode()
	assert.Nil(t, err)
	assert.Equal(t, "16UwLL9Risc3QfPqBUvKofHmBQ7wMtjvM", address, "Conversion failed")
}

func TestEncodeLegacyToCopayLegacyP2KH(t *testing.T) {
	legacy, err := Decode("19NoN69ntmV9nKHBjArLJXXCNq3AvvMsqG")
	assert.Nil(t, err)
	legacy.Version = P2KHCopay
	copay, err := legacy.Encode()
	assert.Nil(t, err)
	assert.Equal(t, copay, "CQqgw8VrmpTggTBcQvBFt39DzxFavppafB", "Conversion failed")

}

func TestEncodeLegacyToCopayLegacyP2SH(t *testing.T) {
	legacy, err := Decode("3J98t1WpEZ73CNmQviecrnyiWrnqRhWNLy")
	assert.Nil(t, err)
	legacy.Version = P2SHCopay
	copay, err := legacy.Encode()
	assert.Nil(t, err)
	assert.Equal(t, copay, "HNyFLowu5sKhpYeSnQJmqBWFYWorHKAWDE", "Conversion failed")
}
