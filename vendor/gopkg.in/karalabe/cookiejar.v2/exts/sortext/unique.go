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
)

// Unique gathers the first occurance of each element to the front, returning
// their number. Data must be sorted in ascending order. The order of the rest
// is ruined.
func Unique(data sort.Interface) int {
	n, u, i := data.Len(), 0, 1
	if n < 2 {
		return n
	}
	for i < n {
		if data.Less(u, i) {
			u++
			data.Swap(u, i)
		}
		i++
	}
	return u + 1
}
