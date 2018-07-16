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

// Two dimensional vector.
type Vec2 struct {
	X, Y float64
}

// Three dimensional vector.
type Vec3 struct {
	X, Y, Z float64
}

// Allocates and returns a new 2D vector.
func NewVec2(x, y float64) *Vec2 {
	return &Vec2{x, y}
}

// Allocates and returns a new 3D vector.
func NewVec3(x, y, z float64) *Vec3 {
	return &Vec3{x, y, z}
}

// Returns the length of x.
func (x *Vec2) Norm() float64 {
	return math.Sqrt(x.X*x.X + x.Y*x.Y)
}

// Returns the length of x.
func (x *Vec3) Norm() float64 {
	return math.Sqrt(x.X*x.X + x.Y*x.Y + x.Z*x.Z)
}

// Sets z to the sum x+y and returns z.
func (z *Vec2) Add(x, y *Vec2) *Vec2 {
	z.X, z.Y = x.X+y.X, x.Y+y.Y
	return z
}

// Sets z to the sum x+y and returns z.
func (z *Vec3) Add(x, y *Vec3) *Vec3 {
	z.X, z.Y, z.Z = x.X+y.X, x.Y+y.Y, x.Z+y.Z
	return z
}

// Sets z to the difference x+y and returns z.
func (z *Vec2) Sub(x, y *Vec2) *Vec2 {
	z.X, z.Y = x.X-y.X, x.Y-y.Y
	return z
}

// Sets z to the difference x+y and returns z.
func (z *Vec3) Sub(x, y *Vec3) *Vec3 {
	z.X, z.Y, z.Z = x.X-y.X, x.Y-y.Y, x.Z-y.Z
	return z
}

// Sets y to x scaled by s and returns y.
func (y *Vec2) Mul(x *Vec2, s float64) *Vec2 {
	y.X, y.Y = s*x.X, s*x.Y
	return y
}

// Sets y to x scaled by s and returns y.
func (y *Vec3) Mul(x *Vec3, s float64) *Vec3 {
	y.X, y.Y, y.Z = s*x.X, s*x.Y, s*x.Z
	return y
}

// Returns the dot product of x and y.
func (x *Vec2) Dot(y *Vec2) float64 {
	return x.X*y.X + x.Y*y.Y
}

// Returns the dot product of x and y.
func (x *Vec3) Dot(y *Vec3) float64 {
	return x.X*y.X + x.Y*y.Y + x.Z*y.Z
}

// Returns the cross product of x and y.
func (x *Vec2) Cross(y *Vec2) float64 {
	return x.X*y.Y - x.Y*y.X
}
