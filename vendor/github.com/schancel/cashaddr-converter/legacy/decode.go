package legacy

import (
	"bytes"
	"crypto/sha256"
	"errors"

	"github.com/mr-tron/base58/base58"
)

// Decode decode a base58 encoded legacy bitcoin address and returns an
// `Address` structure.
func Decode(originalAddress string) (*Address, error) {
	decoded, err := base58.Decode(originalAddress)
	if err != nil {
		return nil, err
	}
	if len(decoded) < 5 {
		return nil, errors.New("insufficient decoded data")
	}

	checksum := decoded[len(decoded)-4:]
	versionAndRipemd := decoded[:len(decoded)-4]
	sha := sha256.Sum256(versionAndRipemd)
	dblsha := sha256.Sum256(sha[:])
	verifyChecksum := dblsha[:4]
	version := uint8(versionAndRipemd[0])
	ripemd := versionAndRipemd[1:]

	if !bytes.Equal(checksum, verifyChecksum) {
		return nil, errors.New("checksum verification failed")
	}

	return &Address{
		Version: version,
		Payload: ripemd,
	}, nil
}
