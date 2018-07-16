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

var nan = math.NaN()

type line2DTest struct {
	line  *Line2
	same  *Line2
	horiz bool
	vert  bool
	slope float64
	intX  float64
	intY  float64
	x1    float64
	y1    float64
	x2    float64
	y2    float64
}

var line2DTests = []line2DTest{
	// Variations crossing (-2, 0) and (0, 2)
	{new(Line2).SetCanon(1, -1, 2), new(Line2).SetCanon(1, -1, 2), false, false, 1, -2, 2, -1, 1, -1, 1},
	{new(Line2).SetCanon(1, -1, 2), new(Line2).SetCanon(2, -2, 4), false, false, 1, -2, 2, -1, 1, -1, 1},
	{new(Line2).SetCanon(1, -1, 2), new(Line2).SetSlope(1, 2), false, false, 1, -2, 2, -1, 1, -1, 1},
	{new(Line2).SetCanon(1, -1, 2), new(Line2).SetPoint(&Point2{-3, -1}, &Point2{1, 3}), false, false, 1, -2, 2, -1, 1, -1, 1},

	// Variations crossing (0, 2) and (2, 0)
	{new(Line2).SetCanon(1, 1, -2), new(Line2).SetCanon(1, 1, -2), false, false, -1, 2, 2, 1, 1, 1, 1},
	{new(Line2).SetCanon(1, 1, -2), new(Line2).SetCanon(2, 2, -4), false, false, -1, 2, 2, 1, 1, 1, 1},
	{new(Line2).SetCanon(1, 1, -2), new(Line2).SetSlope(-1, 2), false, false, -1, 2, 2, 1, 1, 1, 1},
	{new(Line2).SetCanon(1, 1, -2), new(Line2).SetPoint(&Point2{-1, 3}, &Point2{1, 1}), false, false, -1, 2, 2, 1, 1, 1, 1},

	// Horizontal variations
	{new(Line2).SetCanon(0, 1, 0), new(Line2).SetCanon(0, 1, 0), true, false, 0, nan, 0, 0, 0, nan, 0},
	{new(Line2).SetCanon(0, 1, 0), new(Line2).SetCanon(0, 2, 0), true, false, 0, nan, 0, 0, 0, nan, 0},
	{new(Line2).SetCanon(0, 1, 0), new(Line2).SetSlope(0, 0), true, false, 0, nan, 0, 0, 0, nan, 0},
	{new(Line2).SetCanon(0, 1, 0), new(Line2).SetPoint(&Point2{-1, 0}, &Point2{1, 0}), true, false, 0, nan, 0, 0, 0, nan, 0},

	// Vertical variations
	{new(Line2).SetCanon(1, 0, 0), new(Line2).SetCanon(1, 0, 0), false, true, nan, 0, nan, 0, nan, 0, 0},
	{new(Line2).SetCanon(1, 0, 0), new(Line2).SetCanon(2, 0, 0), false, true, nan, 0, nan, 0, nan, 0, 0},
	{new(Line2).SetCanon(1, 0, 0), new(Line2).SetPoint(&Point2{0, -1}, &Point2{0, 1}), false, true, nan, 0, nan, 0, nan, 0, 0},
}

type intersect2DTest struct {
	l1    *Line2
	l2    *Line2
	par   bool
	per   bool
	cross *Point2
}

var intersect2DTests = []intersect2DTest{
	// Parallel lines (horizontal, vertical, diagonals)
	{new(Line2).SetCanon(0, 1, 0), new(Line2).SetCanon(0, 1, 0), true, false, nil},
	{new(Line2).SetCanon(0, 1, 0), new(Line2).SetCanon(0, 2, 0), true, false, nil},
	{new(Line2).SetCanon(1, 0, 0), new(Line2).SetCanon(1, 0, 0), true, false, nil},
	{new(Line2).SetCanon(1, 0, 0), new(Line2).SetCanon(2, 0, 0), true, false, nil},
	{new(Line2).SetSlope(1, 0), new(Line2).SetSlope(1, 0), true, false, nil},
	{new(Line2).SetSlope(1, 0), new(Line2).SetSlope(1, 1), true, false, nil},
	{new(Line2).SetSlope(-1, 0), new(Line2).SetSlope(-1, 0), true, false, nil},
	{new(Line2).SetSlope(-1, 0), new(Line2).SetSlope(-1, 1), true, false, nil},

	// Perpendicular lines
	{new(Line2).SetPoint(&Point2{-1, 0}, &Point2{1, 0}), new(Line2).SetPoint(&Point2{0, -1}, &Point2{0, 1}), false, true, &Point2{0, 0}},
	{new(Line2).SetPoint(&Point2{1, -1}, &Point2{1, 1}), new(Line2).SetPoint(&Point2{-1, 1}, &Point2{1, 1}), false, true, &Point2{1, 1}},
	{new(Line2).SetPoint(&Point2{1, 0}, &Point2{5, 4}), new(Line2).SetPoint(&Point2{1, 4}, &Point2{5, 0}), false, true, &Point2{3, 2}},

	// Simple lines
	{new(Line2).SetPoint(&Point2{0, 0}, &Point2{1, 1}), new(Line2).SetPoint(&Point2{0, 2}, &Point2{1, -1}), false, false, &Point2{0.5, 0.5}},
}

// Slightly modified comparer to allow NaN-NaN comaprisons
func equal(a, b float64) bool {
	if math.IsNaN(a) && math.IsNaN(b) {
		return true
	}
	if !math.IsNaN(a) && !math.IsNaN(b) {
		return math.Abs(a-b) < eps
	}
	return false
}

func TestLine2D(t *testing.T) {
	for i, tt := range line2DTests {
		if !tt.line.Equal(tt.same) {
			t.Errorf("test %d: equality mismatch: %v and %v.", i, tt.line, tt.same)
		}
		if res := tt.line.Horizontal(); res != tt.horiz {
			t.Errorf("test %d: failed horizontality check: have %v, want %v.", i, res, tt.horiz)
		}
		if res := tt.line.Vertical(); res != tt.vert {
			t.Errorf("test %d: failed verticality check: have %v, want %v.", i, res, tt.horiz)
		}
		if res := tt.line.Slope(); !equal(res, tt.slope) {
			t.Errorf("test %d: slope mismatch: have %v, want %v.", i, res, tt.slope)
		}
		if res := tt.line.InterceptX(); !equal(res, tt.intX) {
			t.Errorf("test %d: x intercept mismatch: have %v, want %v.", i, res, tt.intX)
		}
		if res := tt.line.InterceptY(); !equal(res, tt.intY) {
			t.Errorf("test %d: y intercept mismatch: have %v, want %v.", i, res, tt.intY)
		}
		if res := tt.line.Y(tt.x1); !equal(res, tt.y1) {
			t.Errorf("test %d: image mismatch: have %v, want %v.", i, res, tt.y1)
		}
		if res := tt.line.X(tt.y2); !equal(res, tt.x2) {
			t.Errorf("test %d: point mismatch: have %v, want %v.", i, res, tt.x2)
		}
	}
}

func TestIntersect2D(t *testing.T) {
	for i, tt := range intersect2DTests {
		if res := tt.l1.Parallel(tt.l2); res != tt.par {
			t.Errorf("test %d: parallelism mismatch: have %v, want %v.", i, res, tt.par)
		}
		if res := tt.l1.Perpendicular(tt.l2); res != tt.per {
			t.Errorf("test %d: perpendicularity mismatch: have %v, want %v.", i, res, tt.per)
		}
		cross := tt.l1.Intersect(tt.l2)
		switch {
		case cross == nil && tt.cross != nil:
			t.Errorf("test %d: intersection not found: have %v, want %v.", i, cross, tt.cross)
		case cross != nil && tt.cross == nil:
			t.Errorf("test %d: non-exostent intersection: have %v, want %v.", i, cross, tt.cross)
		case cross != nil && tt.cross != nil && !cross.Equal(tt.cross):
			t.Errorf("test %d: intersection mismatch: have %v, want %v.", i, cross, tt.cross)
		}
	}
}
