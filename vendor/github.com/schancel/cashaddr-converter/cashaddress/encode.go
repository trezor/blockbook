package cashaddress

import (
	"errors"
	"fmt"

	"github.com/schancel/cashaddr-converter/baseconv"
)

// ToCashAddr expects a RawAddress and a prefix, and returns the CashAddr URI,
// and possibly an error.
func (addr *Address) Encode() (string, error) {
	packed, err := packAddress(addr)
	if err != nil {
		return "", err
	}

	// Calculate the a checksum.  Provide 8 bytes of padding.
	poly := calculateChecksum(addr.Prefix, append(packed, 0, 0, 0, 0, 0, 0, 0, 0))
	wchk := appendChecksum(packed, poly)
	base32, err := baseconv.ToBase32(wchk)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s:%s", addr.Prefix, base32), nil
}

// String returns a string, consuming errors.  Probably shouldn't use this most places
func (addr *Address) String() string {
	str, err := addr.Encode()
	if err != nil {
		return "error"
	}

	return str
}

// packAddress takes a RawAddress and converts it's payload to a 5-bit packed
// representation.  The first byte represents the address type, and the size
// of the payload.
func packAddress(addr *Address) ([]uint8, error) {
	version := uint8(addr.Version << 3)
	size := len(addr.Payload)
	var encoded_size uint8
	switch size * 8 {
	case 160:
		encoded_size = 0
		break
	case 192:
		encoded_size = 1
		break
	case 224:
		encoded_size = 2
		break
	case 256:
		encoded_size = 3
		break
	case 320:
		encoded_size = 4
		break
	case 384:
		encoded_size = 5
		break
	case 448:
		encoded_size = 6
		break
	case 512:
		encoded_size = 7
		break
	default:
		return nil, errors.New("invalid address size")
	}

	version_byte := version | encoded_size
	input := make([]uint8, 0, len(addr.Payload)+1)
	input = append(input, version_byte)
	input = append(input, addr.Payload...)
	out, _ := baseconv.ConvertBits(8, 5, input, true)

	return out, nil
}

// appendChecksum returns packedaddr with 8 appended checksum bytes from
// PolyMod.
func appendChecksum(packedaddr []uint8, poly uint64) []uint8 {
	chkarr := make([]uint8, 0, 8)
	var i uint
	for i = 0; i < 8; i++ {
		chkarr = append(chkarr, uint8((poly>>uint(5*(7-i)))&0x1F))
	}
	return append(packedaddr, chkarr...)
}
