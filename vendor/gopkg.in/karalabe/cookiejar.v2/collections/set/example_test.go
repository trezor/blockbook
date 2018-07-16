// CookieJar - A contestant's algorithm toolbox
// Copyright (c) 2013 Peter Szilagyi. All rights reserved.
//
// CookieJar is dual licensed: use of this source code is governed by a BSD
// license that can be found in the LICENSE file. Alternatively, the CookieJar
// toolbox may be used in accordance with the terms and conditions contained
// in a signed written agreement between you and the author(s).

package set_test

import (
	"fmt"

	"gopkg.in/karalabe/cookiejar.v2/collections/set"
)

// Insert some numbers into a set, remove one and sum the remainder.
func Example_usage() {
	// Create a new set and insert some data
	s := set.New()
	s.Insert(3.14)
	s.Insert(1.41)
	s.Insert(2.71)
	s.Insert(10) // Isn't this one just ugly?

	// Remove unneeded data and verify that it's gone
	s.Remove(10)
	if !s.Exists(10) {
		fmt.Println("Yay, ugly 10 is no more!")
	} else {
		fmt.Println("Welcome To Facebook")
	}
	// Sum the remainder and output
	sum := 0.0
	s.Do(func(val interface{}) {
		sum += val.(float64)
	})
	fmt.Println("Sum:", sum)

	// Output:
	// Yay, ugly 10 is no more!
	// Sum: 7.26
}
