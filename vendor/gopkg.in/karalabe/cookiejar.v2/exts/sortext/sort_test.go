// CookieJar - A contestant's algorithm toolbox
// Copyright (c) 2013 Peter Szilagyi. All rights reserved.
//
// CookieJar is dual licensed: use of this source code is governed by a BSD
// license that can be found in the LICENSE file. Alternatively, the CookieJar
// toolbox may be used in accordance with the terms and conditions contained
// in a signed written agreement between you and the author(s).

package sortext

import (
	"math/big"
	"sort"
	"testing"
)

var bigints = []*big.Int{
	big.NewInt(74),
	big.NewInt(59),
	big.NewInt(238),
	big.NewInt(-784),
	big.NewInt(9845),
	big.NewInt(959),
	big.NewInt(905),
	big.NewInt(0),
	big.NewInt(0),
	big.NewInt(42),
	big.NewInt(7586),
	big.NewInt(-5467984),
	big.NewInt(7586),
}

var bigrats = []*big.Rat{
	big.NewRat(74, 314),
	big.NewRat(59, 314),
	big.NewRat(238, 314),
	big.NewRat(-784, 314),
	big.NewRat(9845, 314),
	big.NewRat(959, 314),
	big.NewRat(905, 314),
	big.NewRat(0, 314),
	big.NewRat(0, 314),
	big.NewRat(42, 314),
	big.NewRat(7586, 314),
	big.NewRat(-5467984, 314),
	big.NewRat(7586, 314),
}

func TestSortBigIntSlice(t *testing.T) {
	data := bigints
	a := BigIntSlice(data[0:])
	sort.Sort(a)
	if !sort.IsSorted(a) {
		t.Errorf("sorted %v", bigints)
		t.Errorf("   got %v", data)
	}
}

func TestSortBigRatSlice(t *testing.T) {
	data := bigrats
	a := BigRatSlice(data[0:])
	sort.Sort(a)
	if !sort.IsSorted(a) {
		t.Errorf("sorted %v", bigrats)
		t.Errorf("   got %v", data)
	}
}

func TestBigInts(t *testing.T) {
	data := bigints
	BigInts(data[0:])
	if !BigIntsAreSorted(data[0:]) {
		t.Errorf("sorted %v", bigints)
		t.Errorf("   got %v", data)
	}
}

func TestBigRats(t *testing.T) {
	data := bigrats
	BigRats(data[0:])
	if !BigRatsAreSorted(data[0:]) {
		t.Errorf("sorted %v", bigrats)
		t.Errorf("   got %v", data)
	}
}
