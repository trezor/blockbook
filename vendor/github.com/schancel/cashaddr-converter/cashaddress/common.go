package cashaddress

// Address is a struct containing unpacked information relevant to the new
// address specification.
type Address struct {
	Prefix  string
	Version uint8
	Payload []uint8
}

// List of valid cashaddress prefixes
const (
	RegTest = "bchreg"
	TestNet = "bchtest"
	MainNet = "bitcoincash"
)

// Valid cashaddress types
const (
	P2KH uint8 = 0
	P2SH uint8 = 1
)

// polyMod is a BCH-encoding checksum function per the CashAddr specification.
func polyMod(v []uint8) uint64 {
	var c uint64 = 1
	for _, d := range v {
		var c0 uint64 = c >> 35
		c = ((c & 0x07ffffffff) << 5) ^ uint64(d)

		if c0&0x01 != 0 {
			c ^= 0x98f2bc8e61
		}
		if c0&0x02 != 0 {
			c ^= 0x79b76d99e2
		}
		if c0&0x04 != 0 {
			c ^= 0xf33e5fb3c4
		}
		if c0&0x08 != 0 {
			c ^= 0xae2eabe2a8
		}
		if c0&0x10 != 0 {
			c ^= 0x1e4f43e470
		}
	}

	return c ^ 1
}

// expandPrefix takes a string, and returns a byte array with each element
// being the corropsonding character's right-most 5 bits.  Result additionally
// includes a null termination byte.
func expandPrefix(prefix string) []uint8 {
	out := make([]uint8, 0, len(prefix)+1)
	for _, r := range prefix {
		// Grab the right most 5 bits
		out = append(out, uint8(r)&0x1F)
	}
	out = append(out, 0)

	return out
}

// CalculateChecksum calculates a BCH checksum for a nibble-packed cashaddress
// that properly includes the network prefix.
func calculateChecksum(prefix string, packedaddr []uint8) uint64 {
	exphrp := expandPrefix(prefix)
	combined := append(exphrp, packedaddr...)
	return polyMod(combined)
}
