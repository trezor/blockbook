// CookieJar - A contestant's algorithm toolbox
// Copyright (c) 2014 Peter Szilagyi. All rights reserved.
//
// CookieJar is dual licensed: use of this source code is governed by a BSD
// license that can be found in the LICENSE file. Alternatively, the CookieJar
// toolbox may be used in accordance with the terms and conditions contained
// in a signed written agreement between you and the author(s).

package utility

import (
	"fmt"

	"gopkg.in/karalabe/cookiejar.v2/collections/bag"
)

// Data-source based utility set, normalizing and transforming multiple input
// streams by an assigned curve.
type inputSetUtility struct {
	curve    Curve   // Data transformation curve
	min, max float64 // Normalization limits
	nonZero  bool    // Flag whether absolute zero output is allowed

	members map[int]*inputUtility // Members of this utility set
	deps    *bag.Bag              // Derived utilities based on the current one
}

// Creates a new data source utility and associated a transformation curve.
func newInputSetUtility(curve Curve, nonZero bool) *inputSetUtility {
	return &inputSetUtility{
		curve:   curve,
		nonZero: nonZero,
		members: make(map[int]*inputUtility),
		deps:    bag.New(),
	}
}

// Sets the data limits used during normalization.
func (u *inputSetUtility) Limit(min, max float64) {
	u.min, u.max = min, max

	// Update any already initialized members
	for _, util := range u.members {
		util.Limit(min, max)
	}
}

// Adds a new dependency to the utility hierarchy.
func (u *inputSetUtility) Dependency(util utility) {
	// Store the dependency for yet unborn members
	u.deps.Insert(util)

	// Update all existing members
	for id, member := range u.members {
		switch v := util.(type) {
		case *comboUtility:
			member.Dependency(v)
		case *comboSetUtility:
			member.Dependency(v.Member(id))
		default:
			panic(fmt.Sprintf("Unknown dependency to inject: %+v", v))
		}
	}
}

// Retrieves a member of the utility set.
func (u *inputSetUtility) Member(id int) singleUtility {
	if util, ok := u.members[id]; ok {
		return util
	} else {
		u.spawn(id)
		return u.members[id]
	}
}

// Updates the utility of a member to a new data value.
func (u *inputSetUtility) Update(id int, input float64) {
	// Create the member if not seen yet
	if _, ok := u.members[id]; !ok {
		u.spawn(id)
	}
	// Update the input of the member
	u.members[id].Update(input)
}

// Resets a member utility, requiring a reevaluation.
func (u *inputSetUtility) Reset(id int) {
	if util, ok := u.members[id]; ok {
		util.Reset()
	}
}

// Returns the utility value for member for the set data point.
func (u *inputSetUtility) Evaluate(id int) float64 {
	// Create the member if not seen yet
	if _, ok := u.members[id]; !ok {
		u.spawn(id)
	}
	// Evaluate the member and return
	return u.members[id].Evaluate()
}

// Creates a new member utility.
func (u *inputSetUtility) spawn(id int) {
	// Create the base utility
	util := newInputUtility(u.curve, u.nonZero)
	util.Limit(u.min, u.max)
	u.members[id] = util

	// Inject any pending dependencies
	u.deps.Do(func(dep interface{}) {
		switch v := dep.(type) {
		case *comboUtility:
			util.Dependency(v)
		case *comboSetUtility:
			util.Dependency(v.Member(id))
		default:
			panic(fmt.Sprintf("Unknown dependency to inject: %+v", v))
		}
	})
}
