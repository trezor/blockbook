package legacy

import (
	"crypto/sha256"

	"github.com/mr-tron/base58/base58"
)

// Encode returns a base58 encoded string conforming to the Bitcoin legacy
// address format.
func (addr *Address) Encode() (string, error) {
	versionAndRipemd := append([]uint8{addr.Version}, addr.Payload...)
	sha := sha256.Sum256(versionAndRipemd)
	dblsha := sha256.Sum256(sha[:])
	newAddressDecoded := append(versionAndRipemd, dblsha[:4]...)
	return base58.Encode(newAddressDecoded), nil
}
