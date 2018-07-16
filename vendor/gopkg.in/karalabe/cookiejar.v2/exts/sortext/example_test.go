// CookieJar - A contestant's algorithm toolbox
// Copyright (c) 2013 Peter Szilagyi. All rights reserved.
//
// CookieJar is dual licensed: use of this source code is governed by a BSD
// license that can be found in the LICENSE file. Alternatively, the CookieJar
// toolbox may be used in accordance with the terms and conditions contained
// in a signed written agreement between you and the author(s).

package sortext_test

import (
	"fmt"
	"math/big"
	"sort"

	"gopkg.in/karalabe/cookiejar.v2/exts/sortext"
)

func ExampleBigInts() {
	// Define some sample big ints
	one := big.NewInt(1)
	two := big.NewInt(2)
	three := big.NewInt(3)
	four := big.NewInt(4)
	five := big.NewInt(5)
	six := big.NewInt(6)

	// Sort and print a random slice
	s := []*big.Int{five, two, six, three, one, four}
	sortext.BigInts(s)
	fmt.Println(s)

	// Output:
	// [1 2 3 4 5 6]
}

func ExampleUnique() {
	// Create some array of data
	data := []int{1, 5, 4, 3, 1, 3, 2, 5, 4, 3, 3, 0, 0}

	// Sort it
	sort.Ints(data)

	// Get unique elements and siplay them
	n := sortext.Unique(sort.IntSlice(data))
	fmt.Println("Uniques:", data[:n])

	// Output:
	// Uniques: [0 1 2 3 4 5]
}
