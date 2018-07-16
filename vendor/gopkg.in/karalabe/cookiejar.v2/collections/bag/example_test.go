// CookieJar - A contestant's algorithm toolbox
// Copyright (c) 2013 Peter Szilagyi. All rights reserved.
//
// CookieJar is dual licensed: use of this source code is governed by a BSD
// license that can be found in the LICENSE file. Alternatively, the CookieJar
// toolbox may be used in accordance with the terms and conditions contained
// in a signed written agreement between you and the author(s).

package bag_test

import (
	"fmt"

	"gopkg.in/karalabe/cookiejar.v2/collections/bag"
)

// Small demo of the common functions in the bag package.
func Example_usage() {
	// Create a new bag with some integers in it
	b := bag.New()
	for i := 0; i < 10; i++ {
		b.Insert(i)
	}
	b.Insert(8)
	// Remove every odd integer
	for i := 1; i < 10; i += 2 {
		b.Remove(i)
	}
	// Print the element count of all numbers
	for i := 0; i < 10; i++ {
		fmt.Printf("#%d: %d\n", i, b.Count(i))
	}
	// Calculate the sum with a Do iteration
	sum := 0
	b.Do(func(val interface{}) {
		sum += val.(int)
	})
	fmt.Println("Sum:", sum)
	// Output:
	// #0: 1
	// #1: 0
	// #2: 1
	// #3: 0
	// #4: 1
	// #5: 0
	// #6: 1
	// #7: 0
	// #8: 2
	// #9: 0
	// Sum: 28
}
