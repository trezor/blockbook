// CookieJar - A contestant's algorithm toolbox
// Copyright (c) 2013 Peter Szilagyi. All rights reserved.
//
// CookieJar is dual licensed: use of this source code is governed by a BSD
// license that can be found in the LICENSE file. Alternatively, the CookieJar
// toolbox may be used in accordance with the terms and conditions contained
// in a signed written agreement between you and the author(s).

package geometry

import (
	"math"
	"testing"
)

var origin2 = &Point2{0, 0}
var unitX2 = &Point2{1, 0}
var unitY2 = &Point2{0, 1}

var origin3 = &Point3{0, 0, 0}
var unitX3 = &Point3{1, 0, 0}
var unitY3 = &Point3{0, 1, 0}
var unitZ3 = &Point3{0, 0, 1}
var diag3 = &Point3{1, 1, 1}

func TestDist2D(t *testing.T) {
	if d := origin2.Dist(unitX2); math.Abs(d-1) > eps {
		t.Errorf("distance mismatch: have %v, want %v.", d, 1)
	}
	if d := origin2.Dist(unitY2); math.Abs(d-1) > eps {
		t.Errorf("distance mismatch: have %v, want %v.", d, 1)
	}
	if d := unitX2.Dist(unitY2); math.Abs(d-math.Sqrt2) > eps {
		t.Errorf("distance mismatch: have %v, want %v.", d, math.Sqrt2)
	}
}

func TestDistSqr2D(t *testing.T) {
	if d := origin2.DistSqr(unitX2); math.Abs(d-1) > eps {
		t.Errorf("squared distance mismatch: have %v, want %v.", d, 1)
	}
	if d := origin2.DistSqr(unitY2); math.Abs(d-1) > eps {
		t.Errorf("squared distance mismatch: have %v, want %v.", d, 1)
	}
	if d := unitX2.DistSqr(unitY2); math.Abs(d-2) > eps {
		t.Errorf("squared distance mismatch: have %v, want %v.", d, 2)
	}
}

func TestEqual2D(t *testing.T) {
	// Check X coordinate
	a, b, c := &Point2{0, 0}, &Point2{0.999999 * eps, 0}, &Point2{eps, 0}
	if !a.Equal(b) {
		t.Errorf("equality should hold: %v == %v (given eps)", a, b)
	}
	if a.Equal(c) {
		t.Errorf("equality should not hold: %v == %v (given eps)", a, c)
	}
	// Check Y coordinate
	a, b, c = &Point2{0, 0}, &Point2{0, 0.999999 * eps}, &Point2{0, eps}
	if !a.Equal(b) {
		t.Errorf("equality should hold: %v == %v (given eps)", a, b)
	}
	if a.Equal(c) {
		t.Errorf("equality should not hold: %v == %v (given eps)", a, c)
	}
}

func TestDist3D(t *testing.T) {
	if d := origin3.Dist(unitX3); math.Abs(d-1) > eps {
		t.Errorf("distance mismatch: have %v, want %v.", d, 1)
	}
	if d := origin3.Dist(unitY3); math.Abs(d-1) > eps {
		t.Errorf("distance mismatch: have %v, want %v.", d, 1)
	}
	if d := origin3.Dist(unitZ3); math.Abs(d-1) > eps {
		t.Errorf("distance mismatch: have %v, want %v.", d, 1)
	}
	if d := origin3.Dist(diag3); math.Abs(d-math.Sqrt(3)) > eps {
		t.Errorf("distance mismatch: have %v, want %v.", d, math.Sqrt(3))
	}
}

func TestDistSqr3D(t *testing.T) {
	if d := origin3.DistSqr(unitX3); math.Abs(d-1) > eps {
		t.Errorf("squared distance mismatch: have %v, want %v.", d, 1)
	}
	if d := origin3.DistSqr(unitY3); math.Abs(d-1) > eps {
		t.Errorf("squared distance mismatch: have %v, want %v.", d, 1)
	}
	if d := origin3.DistSqr(unitZ3); math.Abs(d-1) > eps {
		t.Errorf("squared distance mismatch: have %v, want %v.", d, 1)
	}
	if d := origin3.DistSqr(diag3); math.Abs(d-3) > eps {
		t.Errorf("squared distance mismatch: have %v, want %v.", d, 3)
	}
}

func TestEqual3D(t *testing.T) {
	// Check X coordinate
	a, b, c := &Point3{0, 0, 0}, &Point3{0.999999 * eps, 0, 0}, &Point3{eps, 0, 0}
	if !a.Equal(b) {
		t.Errorf("equality should hold: %v == %v (given eps)", a, b)
	}
	if a.Equal(c) {
		t.Errorf("equality should not hold: %v == %v (given eps)", a, c)
	}
	// Check Y coordinate
	a, b, c = &Point3{0, 0, 0}, &Point3{0, 0.999999 * eps, 0}, &Point3{0, eps, 0}
	if !a.Equal(b) {
		t.Errorf("equality should hold: %v == %v (given eps)", a, b)
	}
	if a.Equal(c) {
		t.Errorf("equality should not hold: %v == %v (given eps)", a, c)
	}
	// Check Z coordinate
	a, b, c = &Point3{0, 0, 0}, &Point3{0, 0, 0.999999 * eps}, &Point3{0, 0, eps}
	if !a.Equal(b) {
		t.Errorf("equality should hold: %v == %v (given eps)", a, b)
	}
	if a.Equal(c) {
		t.Errorf("equality should not hold: %v == %v (given eps)", a, c)
	}
}
