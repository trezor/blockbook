package zec

import (
	"crypto/sha256"
	"errors"
	"github.com/btcsuite/btcutil/base58"
)

var (
	// ErrChecksumMismatch describes an error where decoding failed due
	// to a bad checksum.
	ErrChecksumMismatch = errors.New("checksum mismatch")

	// ErrInvalidFormat describes an error where decoding failed due to invalid version
	ErrInvalidFormat = errors.New("invalid format: version and/or checksum bytes missing")
)

// checksum: first four bytes of sha256^2
func checksum(input []byte) (cksum [4]byte) {
	h := sha256.Sum256(input)
	h2 := sha256.Sum256(h[:])
	copy(cksum[:], h2[:4])
	return
}

// CheckDecode decodes a string that was encoded with CheckEncode and verifies the checksum.
func CheckDecode(input string) (result []byte, version []byte, err error) {
	decoded := base58.Decode(input)
	if len(decoded) < 5 {
		return nil, nil, ErrInvalidFormat
	}
	version = append(version, decoded[0:2]...)
	var cksum [4]byte
	copy(cksum[:], decoded[len(decoded)-4:])
	if checksum(decoded[:len(decoded)-4]) != cksum {
		return nil, nil, ErrChecksumMismatch
	}
	payload := decoded[2 : len(decoded)-4]
	result = append(result, payload...)
	return
}
