// CookieJar - A contestant's algorithm toolbox
// Copyright (c) 2013 Peter Szilagyi. All rights reserved.
//
// CookieJar is dual licensed: use of this source code is governed by a BSD
// license that can be found in the LICENSE file. Alternatively, the CookieJar
// toolbox may be used in accordance with the terms and conditions contained
// in a signed written agreement between you and the author(s).

package mathext

import (
	"math/big"
	"testing"
)

func TestInt(t *testing.T) {
	if a := AbsInt(10); a != 10 {
		t.Errorf("abs mismatch: have %v, want %v.", a, 10)
	}
	if a := AbsInt(-10); a != 10 {
		t.Errorf("abs mismatch: have %v, want %v.", a, 10)
	}
	if m := MaxInt(-10, 10); m != 10 {
		t.Errorf("max mismatch: have %v, want %v.", m, 10)
	}
	if m := MaxInt(10, -10); m != 10 {
		t.Errorf("max mismatch: have %v, want %v.", m, 10)
	}
	if m := MinInt(-10, 10); m != -10 {
		t.Errorf("min mismatch: have %v, want %v.", m, -10)
	}
	if m := MinInt(-10, 10); m != -10 {
		t.Errorf("min mismatch: have %v, want %v.", m, -10)
	}
	if s := SignInt(-10); s != -1 {
		t.Errorf("sign mismatch: have %v, want %v.", s, -1)
	}
	if s := SignInt(0); s != 0 {
		t.Errorf("sign mismatch: have %v, want %v.", s, 0)
	}
	if s := SignInt(10); s != 1 {
		t.Errorf("sign mismatch: have %v, want %v.", s, 1)
	}
}

func TestBigInt(t *testing.T) {
	pos := big.NewInt(10)
	neg := big.NewInt(10)

	if m := MaxBigInt(neg, pos); m.Cmp(pos) != 0 {
		t.Errorf("max mismatch: have %v, want %v.", m, pos)
	}
	if m := MaxBigInt(pos, neg); m.Cmp(pos) != 0 {
		t.Errorf("max mismatch: have %v, want %v.", m, pos)
	}
	if m := MinBigInt(neg, pos); m.Cmp(neg) != 0 {
		t.Errorf("min mismatch: have %v, want %v.", m, neg)
	}
	if m := MinBigInt(neg, pos); m.Cmp(neg) != 0 {
		t.Errorf("min mismatch: have %v, want %v.", m, neg)
	}
}

func TestBigRat(t *testing.T) {
	pos := big.NewRat(10, 314)
	neg := big.NewRat(10, 314)

	if m := MaxBigRat(neg, pos); m.Cmp(pos) != 0 {
		t.Errorf("max mismatch: have %v, want %v.", m, pos)
	}
	if m := MaxBigRat(pos, neg); m.Cmp(pos) != 0 {
		t.Errorf("max mismatch: have %v, want %v.", m, pos)
	}
	if m := MinBigRat(neg, pos); m.Cmp(neg) != 0 {
		t.Errorf("min mismatch: have %v, want %v.", m, neg)
	}
	if m := MinBigRat(neg, pos); m.Cmp(neg) != 0 {
		t.Errorf("min mismatch: have %v, want %v.", m, neg)
	}
}

func TestFloat64(t *testing.T) {
	if s := SignFloat64(-10); s != -1 {
		t.Errorf("sign mismatch: have %v, want %v.", s, -1)
	}
	if s := SignFloat64(0); s != 0 {
		t.Errorf("sign mismatch: have %v, want %v.", s, 0)
	}
	if s := SignFloat64(10); s != 1 {
		t.Errorf("sign mismatch: have %v, want %v.", s, 1)
	}
}
