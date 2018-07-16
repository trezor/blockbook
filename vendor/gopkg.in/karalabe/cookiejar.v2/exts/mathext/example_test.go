// CookieJar - A contestant's algorithm toolbox
// Copyright (c) 2013 Peter Szilagyi. All rights reserved.
//
// CookieJar is dual licensed: use of this source code is governed by a BSD
// license that can be found in the LICENSE file. Alternatively, the CookieJar
// toolbox may be used in accordance with the terms and conditions contained
// in a signed written agreement between you and the author(s).

package mathext_test

import (
	"fmt"
	"math/big"

	"gopkg.in/karalabe/cookiejar.v2/exts/mathext"
)

func ExampleMaxBigInt() {
	// Define some sample big ints
	four := big.NewInt(4)
	five := big.NewInt(5)

	// Print the minimum and maximum of the two
	fmt.Println("Min:", mathext.MinBigInt(four, five))
	fmt.Println("Max:", mathext.MaxBigInt(four, five))

	// Output:
	// Min: 4
	// Max: 5
}
