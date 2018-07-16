// CookieJar - A contestant's algorithm toolbox
// Copyright (c) 2013 Peter Szilagyi. All rights reserved.
//
// CookieJar is dual licensed: use of this source code is governed by a BSD
// license that can be found in the LICENSE file. Alternatively, the CookieJar
// toolbox may be used in accordance with the terms and conditions contained
// in a signed written agreement between you and the author(s).

package geometry

import (
	"fmt"
	"math"
)

// Two dimensional line.
type Line2 struct {
	A, B, C float64
}

// Allocates and returns a new 2D line in canonical form.
func NewLine2(a, b, c float64) *Line2 {
	if a == 0 && b == 0 {
		panic(fmt.Sprintf("coefficients a and b simultaneously 0"))
	}
	return &Line2{a, b, c}
}

// Sets the canonical form coefficients on l (ax + by + c = 0) and returns l.
func (l *Line2) SetCanon(a, b, c float64) *Line2 {
	l.A, l.B, l.C = a, b, c
	return l
}

// Sets the slope form coefficients on l (y = mx + c) and returns l.
func (l *Line2) SetSlope(m, c float64) *Line2 {
	l.A, l.B, l.C = -m, 1, -c
	return l
}

// Sets the coordinate form coefficients on l ((y-y0)*(x1-x0) = (x-x0)*(y1-y0)) and returns l.
func (l *Line2) SetPoint(p0, p1 *Point2) *Line2 {
	if p0.Equal(p1) {
		panic(fmt.Sprintf("same point given twice: %v.", p0))
	}
	l.A, l.B, l.C = p0.Y-p1.Y, p1.X-p0.X, p0.X*(p1.Y-p0.Y)-p0.Y*(p1.X-p0.X)
	return l
}

// Returns whether l is horizontal.
func (l *Line2) Horizontal() bool {
	return l.A == 0
}

// Returns whether l is vertical.
func (l *Line2) Vertical() bool {
	return l.B == 0
}

// Returns the slope/gradient m from the form y = mx + c. Returns NaN if l is vertical.
func (l *Line2) Slope() float64 {
	if !l.Vertical() {
		return -l.A / l.B
	}
	return math.NaN()
}

// Returns the x-intercept of the line (y = 0). Returns NaN if l is horizontal.
func (l *Line2) InterceptX() float64 {
	if !l.Horizontal() {
		return -l.C / l.A
	}
	return math.NaN()
}

// Returns the y-intercept of the line (x = 0). Returns NaN if l is vertical.
func (l *Line2) InterceptY() float64 {
	if !l.Vertical() {
		return -l.C / l.B
	}
	return math.NaN()
}

// Calculates the value x, the image of which is y. Returns NaN if l is horizontal.
func (l *Line2) X(y float64) float64 {
	if !l.Horizontal() {
		return -(l.C + l.B*y) / l.A
	}
	return math.NaN()
}

// Calculates the image of x. Returns NaN if l is vertical.
func (l *Line2) Y(x float64) float64 {
	if !l.Vertical() {
		return -(l.C + l.A*x) / l.B
	}
	return math.NaN()
}

// Returns whether x and y define the same line.
func (x *Line2) Equal(y *Line2) bool {
	switch {
	case x.Horizontal() != y.Horizontal() || x.Vertical() != y.Vertical():
		return false
	case x.Horizontal() && y.Horizontal():
		return x.Y(0) == y.Y(0)
	case x.Vertical() && y.Vertical():
		return x.X(0) == y.X(0)
	default:
		return math.Abs(x.A/y.A-x.B/y.B) < eps && ((x.C == 0 && y.C == 0) || math.Abs(x.B/y.B-x.C/y.C) < eps)
	}
}

// Returns whether two lines are parallel.
func (x *Line2) Parallel(y *Line2) bool {
	return math.Abs(x.A*y.B-y.A*x.B) < eps
}

// Returns whether two lines are perpendicular.
func (x *Line2) Perpendicular(y *Line2) bool {
	if x.Parallel(y) {
		return false
	} else if (x.Horizontal() && y.Vertical()) || (y.Horizontal() && x.Vertical()) {
		return true
	}
	return math.Abs(x.Slope()*y.Slope()+1) < eps
}

// Calculates the intersetion point of two lines, or returns nil if they are parallel.
func (x *Line2) Intersect(y *Line2) *Point2 {
	if den := x.A*y.B - y.A*x.B; math.Abs(den) < eps {
		return nil
	} else {
		return &Point2{(x.B*y.C - y.B*x.C) / den, (y.A*x.C - x.A*y.C) / den}
	}
}

// Calculates the delta of a point form the line (i.e. signed distance).
func (l *Line2) Delta(p *Point2) float64 {
	// Get the base vector for the line
	var base *Vec2
	if l.Vertical() {
		base = NewVec2(0, 1)
	} else {
		base = new(Vec2).Sub(&Vec2{0, l.Y(0)}, &Vec2{1, l.Y(1)})
	}
	// Calculate cross product and height from that
	return base.Cross(&Vec2{p.X, p.Y}) / base.Norm()
}

// Calculates the distance of a point from the line.
func (l *Line2) Dist(p *Point2) float64 {
	return math.Abs(l.Delta(p))
}
