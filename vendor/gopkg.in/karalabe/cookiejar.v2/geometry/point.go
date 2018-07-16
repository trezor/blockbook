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
)

// Two dimensional point.
type Point2 struct {
	X, Y float64
}

// Three dimensional point.
type Point3 struct {
	X, Y, Z float64
}

// Allocates and returns a new 2D point.
func NewPoint2(x, y float64) *Point2 {
	return &Point2{x, y}
}

// Allocates and returns a new 3D point.
func NewPoint3(x, y, z float64) *Point3 {
	return &Point3{x, y, z}
}

// Calculates the distance between x and y.
func (x *Point2) Dist(y *Point2) float64 {
	return math.Sqrt(x.DistSqr(y))
}

// Calculates the distance between x and y.
func (x *Point3) Dist(y *Point3) float64 {
	return math.Sqrt(x.DistSqr(y))
}

// Calculates the squared distance between x and y.
func (x *Point2) DistSqr(y *Point2) float64 {
	dx := x.X - y.X
	dy := x.Y - y.Y
	return dx*dx + dy*dy
}

// Calculates the squared distance between x and y.
func (x *Point3) DistSqr(y *Point3) float64 {
	dx := x.X - y.X
	dy := x.Y - y.Y
	dz := x.Z - y.Z
	return dx*dx + dy*dy + dz*dz
}

// Returns whether two points are equal.
func (x *Point2) Equal(y *Point2) bool {
	return math.Abs(x.X-y.X) < eps && math.Abs(x.Y-y.Y) < eps
}

// Returns whether two points are equal.
func (x *Point3) Equal(y *Point3) bool {
	return math.Abs(x.X-y.X) < eps && math.Abs(x.Y-y.Y) < eps && math.Abs(x.Z-y.Z) < eps
}
