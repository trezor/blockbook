package baseconv

// ConvertBits takes a byte array as `input`, and converts it from `frombits`
// bit representation to a `tobits` bit representation, while optionally
// padding it.  ConvertBits returns the new representation and a bool
// indicating that the output was not truncated.
func ConvertBits(frombits uint8, tobits uint8, input []uint8, pad bool) ([]uint8, bool) {
	if frombits > 8 {
		return nil, false
	}

	var acc uint64 = 0
	var bits uint64 = 0
	var out []uint8 = make([]uint8, 0, len(input)*int(frombits)/int(tobits))
	var maxv uint64 = (1 << tobits) - 1
	var max_acc uint64 = (1 << (frombits + tobits - 1)) - 1
	for _, d := range input {
		acc = ((acc << uint64(frombits)) | uint64(d)) & max_acc
		bits += uint64(frombits)
		for bits >= uint64(tobits) {
			bits -= uint64(tobits)
			v := (acc >> bits) & maxv
			out = append(out, uint8(v))
		}
	}

	// We have remaining bits to encode but do not pad.
	if !pad && bits > 0 {
		return out, false
	}

	// We have remaining bits to encode so we do pad.
	if pad && bits > 0 {
		out = append(out, uint8((acc<<(uint64(tobits)-bits))&maxv))
	}

	return out, true
}
