// CookieJar - A contestant's algorithm toolbox
// Copyright (c) 2013 Peter Szilagyi. All rights reserved.
//
// CookieJar is dual licensed: use of this source code is governed by a BSD
// license that can be found in the LICENSE file. Alternatively, the CookieJar
// toolbox may be used in accordance with the terms and conditions contained
// in a signed written agreement between you and the author(s).

package stack_test

import (
	"fmt"

	"gopkg.in/karalabe/cookiejar.v2/collections/stack"
)

// Simple usage example that inserts the numbers 1, 2, 3 into a stack and then
// removes them one by one, printing them to the standard output.
func Example_usage() {
	// Create a stack and push some data in
	s := stack.New()
	for i := 0; i < 3; i++ {
		s.Push(i)
	}
	// Pop out the stack contents and display them
	for !s.Empty() {
		fmt.Println(s.Pop())
	}
	// Output:
	// 2
	// 1
	// 0
}
