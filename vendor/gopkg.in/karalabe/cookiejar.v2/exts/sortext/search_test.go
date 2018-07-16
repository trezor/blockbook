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
	"testing"
)

// Smoke tests for convenience wrappers - not comprehensive.
var idata = []*big.Int{big.NewInt(-5), big.NewInt(0), big.NewInt(11), big.NewInt(100)}
var rdata = []*big.Rat{big.NewRat(-314, 100), big.NewRat(0, 1), big.NewRat(1, 1), big.NewRat(2, 1), big.NewRat(10007, 10)}

var wrappertests = []struct {
	name   string
	result int
	i      int
}{
	{"SearchBigInts", SearchBigInts(idata, big.NewInt(11)), 2},
	{"SearchBigRats", SearchBigRats(rdata, big.NewRat(21, 10)), 4},
	{"BigIntSlice.Search", BigIntSlice(idata).Search(big.NewInt(0)), 1},
	{"BigRatSlice.Search", BigRatSlice(rdata).Search(big.NewRat(20, 10)), 3},
}

func TestSearchWrappers(t *testing.T) {
	for _, e := range wrappertests {
		if e.result != e.i {
			t.Errorf("%s: expected index %d; got %d", e.name, e.i, e.result)
		}
	}
}
