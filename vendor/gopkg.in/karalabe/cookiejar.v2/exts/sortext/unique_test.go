// CookieJar - A contestant's algorithm toolbox
// Copyright (c) 2013 Peter Szilagyi. All rights reserved.
//
// CookieJar is dual licensed: use of this source code is governed by a BSD
// license that can be found in the LICENSE file. Alternatively, the CookieJar
// toolbox may be used in accordance with the terms and conditions contained
// in a signed written agreement between you and the author(s).

package sortext

import (
	"sort"
	"testing"
)

type uniqueTest struct {
	data []int
	num  int
}

var uniqueTests = []uniqueTest{
	{[]int{}, 0},
	{[]int{1}, 1},
	{
		[]int{
			1,
			2, 2,
			3, 3, 3,
			4, 4, 4, 4,
			5, 5, 5, 5, 5,
			6, 6, 6, 6, 6, 6,
		},
		6,
	},
}

func TestUnique(t *testing.T) {
	for i, tt := range uniqueTests {
		n := Unique(sort.IntSlice(tt.data))
		if n != tt.num {
			t.Errorf("test %d: unique count mismatch: have %v, want %v.", i, n, tt.num)
		}
		for j := 0; j < n; j++ {
			for k := j + 1; k < n; k++ {
				if tt.data[j] >= tt.data[k] {
					t.Errorf("test %d: uniqueness violation: (%d, %d) in %v.", i, j, k, tt.data[:n])
				}
			}
		}
	}
}
