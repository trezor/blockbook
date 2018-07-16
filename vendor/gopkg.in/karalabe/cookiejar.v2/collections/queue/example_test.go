// CookieJar - A contestant's algorithm toolbox
// Copyright (c) 2013 Peter Szilagyi. All rights reserved.
//
// CookieJar is dual licensed: use of this source code is governed by a BSD
// license that can be found in the LICENSE file. Alternatively, the CookieJar
// toolbox may be used in accordance with the terms and conditions contained
// in a signed written agreement between you and the author(s).

package queue_test

import (
	"fmt"

	"gopkg.in/karalabe/cookiejar.v2/collections/queue"
)

// Simple usage example that inserts the numbers 0, 1, 2 into a queue and then
// removes them one by one, printing them to the standard output.
func Example_usage() {
	// Create a queue an push some data in
	q := queue.New()
	for i := 0; i < 3; i++ {
		q.Push(i)
	}
	// Pop out the queue contents and display them
	for !q.Empty() {
		fmt.Println(q.Pop())
	}
	// Output:
	// 0
	// 1
	// 2
}
