package cashaddress

import (
	"errors"
	"fmt"
	"strings"

	"github.com/schancel/cashaddr-converter/baseconv"
)

// FromCashAddr takes a CashAddr URI string, and returns and unpacked
// RawAddress, and possible an error.
func Decode(addr string, defaultPrefix string) (*Address, error) {
	prefix, address := splitAddress(addr, defaultPrefix)
	if prefix == "" {
		return nil, errors.New("no viable prefix found in address")
	}

	if strings.ToUpper(addr) != addr && strings.ToLower(addr) != addr {
		return nil, errors.New("cashaddress contains mixed upper and lowercase characters")
	}

	decoded, err := baseconv.FromBase32(address)
	if err != nil {
		return nil, err
	}

	// Ensure the checksum is zero when decoding
	chksum := calculateChecksum(prefix, decoded)
	if chksum != 0 {
		return nil, errors.New("checksum verification failed")
	}

	if len(prefix) == 0 {
		return nil, errors.New("")
	}

	// The checksum sits in the last eight bytes. We should have at least one
	// more byte than the checksum.
	if len(decoded) < 9 {
		return nil, errors.New("insufficient packed data to decode")
	}

	// Unpack the address without the checksum bits
	raw, err := unpackAddress(decoded[:len(decoded)-8], strings.ToLower(prefix))
	return raw, err
}

// splitAddress takes a cashaddr string and returns the network prefix, and
// the base32 payload separately.
func splitAddress(fulladdr string, defaultPrefix string) (string, string) {
	res := strings.SplitN(fulladdr, ":", 2)
	if len(res) == 1 {
		return defaultPrefix, res[0]
	}

	return res[0], res[1]
}

// unpackAddress takes a []uint8 in a raw (not base32 encoded) cashaddr
// format.  The checksum bits must already be removed.
func unpackAddress(packedaddr []uint8, prefix string) (*Address, error) {
	out, _ := baseconv.ConvertBits(5, 8, packedaddr, false)

	// Ensure there isn't extra non-zero padding
	extrabits := len(out) * 5 % 8
	if extrabits >= 5 {
		return nil, fmt.Errorf("non-zero padding")
	}

	version_byte := int(out[0])
	addrtype := version_byte >> 3

	decoded_size := 20 + 4*(version_byte&0x03)
	if version_byte&0x04 == 0x04 {
		decoded_size = decoded_size * 2
	}
	if decoded_size != len(out)-1 {
		return nil, fmt.Errorf("invalid size information (%v != %v)", decoded_size, len(out)-1)
	}

	return &Address{
		Prefix:  prefix,
		Version: uint8(addrtype),
		Payload: out[1:len(out)],
	}, nil
}
