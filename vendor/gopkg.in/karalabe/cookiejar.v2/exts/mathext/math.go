// CookieJar - A contestant's algorithm toolbox
// Copyright (c) 2013 Peter Szilagyi. All rights reserved.
//
// CookieJar is dual licensed: use of this source code is governed by a BSD
// license that can be found in the LICENSE file. Alternatively, the CookieJar
// toolbox may be used in accordance with the terms and conditions contained
// in a signed written agreement between you and the author(s).

// Package mathext contains extensions to the base Go math package.
package mathext

import "math/big"

const (
	maxInt = int(^uint(0) >> 1)
	minInt = int(-maxInt - 1)
)

// AbsInt returns the absolute value of the x integer.
//
// Special cases are:
//   AbsInt(minInt) results in a panic (not representable)
func AbsInt(x int) int {
	if x >= 0 {
		return x
	}
	if x == minInt {
		panic("absolute overflows int")
	}
	return -x
}

// MaxInt returns the larger of x or y integers.
func MaxInt(x, y int) int {
	if x > y {
		return x
	}
	return y
}

// MaxBigInt returns the larger of x or y big integers.
func MaxBigInt(x, y *big.Int) *big.Int {
	if x.Cmp(y) > 0 {
		return x
	}
	return y
}

// MaxBigRat returns the larger of x or y big rationals.
func MaxBigRat(x, y *big.Rat) *big.Rat {
	if x.Cmp(y) > 0 {
		return x
	}
	return y
}

// MinInt returns the smaller of x or y integers.
func MinInt(x, y int) int {
	if x < y {
		return x
	}
	return y
}

// MinBigInt returns the smaller of x or y big integers.
func MinBigInt(x, y *big.Int) *big.Int {
	if x.Cmp(y) < 0 {
		return x
	}
	return y
}

// MinBigRat returns the smaller of x or y big rationals.
func MinBigRat(x, y *big.Rat) *big.Rat {
	if x.Cmp(y) < 0 {
		return x
	}
	return y
}

// SignInt returns the sign of the x integer.
func SignInt(x int) int {
	switch {
	case x > 0:
		return 1
	case x == 0:
		return 0
	default:
		return -1
	}
}

// SignFloat64 returns the sign of the x floating point number.
func SignFloat64(x int) int {
	switch {
	case x > 0:
		return 1
	case x == 0:
		return 0
	default:
		return -1
	}
}
