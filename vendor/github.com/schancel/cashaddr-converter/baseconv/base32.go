package baseconv

import (
	"bytes"
	"errors"
)

// Charset for converting from base32
var charset_rev = []int8{
	-1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1,
	-1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1,
	-1, -1, -1, -1, -1, -1, -1, -1, -1, -1, 15, -1, 10, 17, 21, 20, 26, 30, 7,
	5, -1, -1, -1, -1, -1, -1, -1, 29, -1, 24, 13, 25, 9, 8, 23, -1, 18, 22,
	31, 27, 19, -1, 1, 0, 3, 16, 11, 28, 12, 14, 6, 4, 2, -1, -1, -1, -1,
	-1, -1, 29, -1, 24, 13, 25, 9, 8, 23, -1, 18, 22, 31, 27, 19, -1, 1, 0,
	3, 16, 11, 28, 12, 14, 6, 4, 2, -1, -1, -1, -1, -1}

// Charset for converting to base32
var charset = []rune{
	'q', 'p', 'z', 'r', 'y', '9', 'x', '8', 'g', 'f',
	'2', 't', 'v', 'd', 'w', '0', 's', '3', 'j', 'n',
	'5', '4', 'k', 'h', 'c', 'e', '6', 'm', 'u', 'a',
	'7', 'l',
}

// ToBase32 converts an input byte array into a base32 string.  It expects the
// byte array to be 5-bit packed.
func ToBase32(input []uint8) (string, error) {
	var buf bytes.Buffer

	for _, i := range input {
		if i < 0 || int(i) >= len(charset) {
			return "", errors.New("invalid byte in input array")
		}
		buf.WriteRune(charset[i])
	}
	return buf.String(), nil
}

// FromBase32 takes a string in base32 format and returns a byte array that is
// 5-bit packed.
func FromBase32(input string) ([]uint8, error) {
	out := make([]uint8, 0, len(input))

	for _, c := range input {
		val := charset_rev[c]
		if val == -1 {
			return nil, errors.New("invalid base32 input string")
		}
		out = append(out, uint8(val))
	}
	return out, nil
}
