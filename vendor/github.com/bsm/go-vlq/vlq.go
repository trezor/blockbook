package vlq

import (
	"errors"
	"io"
)

const (
	MaxLen16 = 3
	MaxLen32 = 5
	MaxLen64 = 10
)

var overflow = errors.New("vlq: number overflows a 64-bit integer")

// PutUint encodes a uint64 into buf and returns the number of bytes written.
// If the buffer is too small, PutUint will panic.
func PutUint(buf []byte, x uint64) int {
	var n uint
	for n = 9; n > 0; n-- {
		if x >= 1<<(n*7) {
			break
		}
	}

	i := int(n) + 1
	buf[n] = byte(x) & 0x7F
	for n > 0 {
		n--
		x >>= 7
		buf[n] = byte(x) | 0x80
	}
	return i
}

// Uint decodes a uint64 from buf and returns that value and the
// number of bytes read (> 0). If an error occurred, the value is 0
// and the number of bytes n is <= 0 meaning:
//
//  n == 0: buf too small
//  n  < 0: value larger than 64 bits (overflow)
//              and -n is the number of bytes read
//
func Uint(buf []byte) (uint64, int) {
	var x uint64
	for i, b := range buf {
		x = (x << 7) | uint64(b&0x7f)

		if b < 0x80 {
			return x, i + 1
		} else if i == 9 {
			return 0, -i - 1 // overflow
		}
	}
	return x, 0

}

// PutInt encodes an int64 into buf and returns the number of bytes written.
// If the buffer is too small, PutInt will panic.
func PutInt(buf []byte, x int64) int {
	ux := uint64(x) << 1
	if x < 0 {
		ux = ^ux
	}
	return PutUint(buf, ux)
}

// Int decodes an int64 from buf and returns that value and the
// number of bytes read (> 0). If an error occurred, the value is 0
// and the number of bytes n is <= 0 with the following meaning:
//
//	n == 0: buf too small
//	n  < 0: value larger than 64 bits (overflow)
//              and -n is the number of bytes read
//
func Int(buf []byte) (int64, int) {
	ux, n := Uint(buf) // ok to continue in presence of error
	x := int64(ux >> 1)
	if ux&1 != 0 {
		x = ^x
	}
	return x, n
}

// ReadUint reads an encoded unsigned integer from r and returns it as a uint64.
func ReadUint(r io.ByteReader) (uint64, error) {
	var x uint64
	for i := 0; ; i++ {
		b, err := r.ReadByte()
		if err != nil {
			return x, err
		}
		x = (x << 7) | uint64(b&0x7f)

		if b < 0x80 {
			return x, nil
		} else if i == 9 {
			return 0, overflow
		}
	}
}

// ReadInt reads an encoded signed integer from r and returns it as an int64.
func ReadInt(r io.ByteReader) (int64, error) {
	ux, err := ReadUint(r) // ok to continue in presence of error
	x := int64(ux >> 1)
	if ux&1 != 0 {
		x = ^x
	}
	return x, err
}
