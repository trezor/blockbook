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
type comboSetUtility struct {
	combinator Combinator // Curve transformation combinator
	srcA, srcB utility    // Base utilities from which to derive this one

	members map[int]*comboUtility // Members of this utility set
	deps    *bag.Bag              // Derived utilities based on the current one
}

// Creates a new derived utility set based on two existing ones.
func newComboSetUtility(combinator Combinator, srcA, srcB utility) *comboSetUtility {
	return &comboSetUtility{
		combinator: combinator,
		srcA:       srcA,
		srcB:       srcB,
		members:    make(map[int]*comboUtility),
		deps:       bag.New(),
	}
}

// Adds a new dependency to the utility hierarchy.
func (u *comboSetUtility) Dependency(util utility) {
	// Store the dependency for yet unborn members
	u.deps.Insert(util)

	// Update all existing members
	for _, member := range u.members {
		switch v := util.(type) {
		case *comboUtility:
			member.Dependency(v)
		}
	}
}

// Retrieves a member of the utility set.
func (u *comboSetUtility) Member(id int) singleUtility {
	if util, ok := u.members[id]; ok {
		return util
	} else {
		u.spawn(id)
		return u.members[id]
	}
}

// Resets a member utility, requiring a reevaluation.
func (u *comboSetUtility) Reset(id int) {
	if util, ok := u.members[id]; ok {
		util.Reset()
	}
}

// Returns the utility value for member for the set data point.
func (u *comboSetUtility) Evaluate(id int) float64 {
	// Create the member if not seen yet
	if _, ok := u.members[id]; !ok {
		u.spawn(id)
	}
	// Evaluate the member and return
	return u.members[id].Evaluate()
}

// Creates a new member utility.
func (u *comboSetUtility) spawn(id int) {
	util := newComboUtility(u.combinator)
	u.members[id] = util

	// Extract the singleton utility from source A
	var srcA singleUtility
	switch v := u.srcA.(type) {
	case singleUtility:
		srcA = v
	case multiUtility:
		srcA = v.Member(id)
	default:
		panic(fmt.Sprintf("Unknown utility type during combo spawn: %+v", v))
	}
	// Extract the singleton utility from source B
	var srcB singleUtility
	switch v := u.srcB.(type) {
	case singleUtility:
		srcB = v
	case multiUtility:
		srcB = v.Member(id)
	default:
		panic(fmt.Sprintf("Unknown utility type during combo spawn: %+v", v))
	}
	// Finish initiating the utility
	util.Init(srcA, srcB)

	// Inherit any pending dependencies
	u.deps.Do(func(dep interface{}) {
		util.Dependency(dep.(*comboUtility))
	})
}
