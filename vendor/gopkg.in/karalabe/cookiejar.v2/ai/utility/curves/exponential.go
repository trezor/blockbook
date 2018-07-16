// CookieJar - A contestant's algorithm toolbox
// Copyright (c) 2014 Peter Szilagyi. All rights reserved.
//
// CookieJar is dual licensed: use of this source code is governed by a BSD
// license that can be found in the LICENSE file. Alternatively, the CookieJar
// toolbox may be used in accordance with the terms and conditions contained
// in a signed written agreement between you and the author(s).

package curves

import (
	"math"

	"gopkg.in/karalabe/cookiejar.v2/ai/utility"
)

// Exponential curve builder. Defined as y = |x-infl| ^ exp.
type Exponential struct {
	Infl   float64 // Point of inflection (either peek of valley)
	Exp    float64 // Exponent defining the rate of change
	Convex bool    // Switches between upwards (true) or downwards curve
}

// Creates the curve mapping function.
func (e Exponential) Make() utility.Curve {
	infl, exp := e.Infl, e.Exp
	if e.Convex {
		return func(x float64) float64 {
			return math.Pow(math.Abs(x-infl), exp)
		}
	} else {
		return func(x float64) float64 {
			return 1 - math.Pow(math.Abs(x-infl), exp)
		}
	}
}
