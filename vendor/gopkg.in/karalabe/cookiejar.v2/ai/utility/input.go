// CookieJar - A contestant's algorithm toolbox
// Copyright (c) 2014 Peter Szilagyi. All rights reserved.
//
// CookieJar is dual licensed: use of this source code is governed by a BSD
// license that can be found in the LICENSE file. Alternatively, the CookieJar
// toolbox may be used in accordance with the terms and conditions contained
// in a signed written agreement between you and the author(s).

package utility

import (
	"math"

	"gopkg.in/karalabe/cookiejar.v2/collections/bag"
)

// Data-source based utility, normalizing and transforming an input stream by an
// assigned curve.
type inputUtility struct {
	curve    Curve   // Data transformation curve
	min, max float64 // Normalization limits
	nonZero  bool    // Flag whether absolute zero output is allowed

	children *bag.Bag // Derived utilities based on the current one

	reset  bool    // Flag whether the output is not yet calculated
	input  float64 // Input value which to o map to the curve
	output float64 // Cached output utility value
}

// Creates a new data source utility and associated a transformation curve.
func newInputUtility(curve Curve, nonZero bool) *inputUtility {
	return &inputUtility{
		curve:    curve,
		nonZero:  nonZero,
		children: bag.New(),
		reset:    true,
	}
}

// Sets the data limits used during normalization.
func (u *inputUtility) Limit(min, max float64) {
	u.min, u.max = min, max
	u.Reset()
}

// Updates the utility to a new data value.
func (u *inputUtility) Update(input float64) {
	u.input = input
	u.Reset()
}

// Resets the utility, requiring a reevaluation.
func (u *inputUtility) Reset() {
	if !u.reset {
		u.reset = true
		u.children.Do(func(util interface{}) {
			util.(*comboUtility).Reset()
		})
	}
}

// Adds a new dependency to the utility hierarchy.
func (u *inputUtility) Dependency(util utility) {
	u.children.Insert(util)
}

// Returns the utility value for the set data point.
func (u *inputUtility) Evaluate() float64 {
	// Recalculate the output value if not cached
	if u.reset {
		// Normalize the input and calculate the output
		if diff := u.max - u.min; diff != 0 {
			u.input = (u.input - u.min) / diff
		}
		u.output = math.Min(1, math.Max(0, u.curve(u.input)))

		// If requested, prevent a result of absolute zero
		if u.nonZero && u.output == 0 {
			u.output = float64(1e-18)
		}
		u.reset = false
	}
	return u.output
}
