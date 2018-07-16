// CookieJar - A contestant's algorithm toolbox
// Copyright (c) 2014 Peter Szilagyi. All rights reserved.
//
// CookieJar is dual licensed: you can redistribute it and/or modify it under
// the terms of the GNU General Public License as published by the Free Software
// Foundation, either version 3 of the License, or (at your option) any later
// version.
//
// The toolbox is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE.  See the GNU General Public License for
// more details.
//
// Alternatively, the CookieJar toolbox may be used in accordance with the terms
// and conditions contained in a signed written agreement between you and the
// author(s).

package curves

import (
	"gopkg.in/karalabe/cookiejar.v2/ai/utility"
)

// Linear curve builder. Defined as y = ax + b.
type Linear struct {
	A, B float64
}

// Creates the curve mapping function.
func (l Linear) Make() utility.Curve {
	a, b := l.A, l.B

	return func(x float64) float64 {
		return a*x + b
	}
}
