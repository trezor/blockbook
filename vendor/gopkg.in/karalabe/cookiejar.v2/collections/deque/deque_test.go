// CookieJar - A contestant's algorithm toolbox
// Copyright (c) 2013 Peter Szilagyi. All rights reserved.
//
// CookieJar is dual licensed: use of this source code is governed by a BSD
// license that can be found in the LICENSE file. Alternatively, the CookieJar
// toolbox may be used in accordance with the terms and conditions contained
// in a signed written agreement between you and the author(s).

package deque

import (
	"math/rand"
	"testing"
)

func TestDeque(t *testing.T) {
	// Create some initial data
	size := 16 * blockSize
	data := make([]int, size)
	for i := 0; i < size; i++ {
		data[i] = rand.Int()
	}
	deque := New()
	for rep := 0; rep < 2; rep++ {
		// Push all the data into the deque, each one on a different side
		outs := []int{}
		for i := 0; i < size; i++ {
			if i%2 == 0 {
				deque.PushLeft(data[i])
				if deque.Left() != data[i] {
					t.Errorf("push/left mismatch: have %v, want %v.", deque.Left(), data[i])
				}
			} else {
				deque.PushRight(data[i])
				if deque.Right() != data[i] {
					t.Errorf("push/right mismatch: have %v, want %v.", deque.Right(), data[i])
				}
			}
			// Pop out every third and fourth (inversely than inserted)
			if i%4 == 2 {
				outs = append(outs, deque.PopRight().(int))
			} else if i%4 == 3 {
				outs = append(outs, deque.PopLeft().(int))
			}
			// Make sure size is consistent
			if deque.Size() != i/2+i%2+1-(i%4)/3 {
				t.Errorf("size mismatch: have %v, want %v.", deque.Size(), i/2+i%2+1-(i%4)/3)
			}
		}
		rest := []int{}
		for !deque.Empty() {
			if len(rest)%2 == 0 {
				rest = append(rest, deque.PopRight().(int))
			} else {
				rest = append(rest, deque.PopLeft().(int))
			}
		}
		// Make sure the contents of the resulting slices are ok
		for i := 1; i < size; i += 4 {
			if data[i] != outs[i/2] {
				t.Errorf("push/pop mismatch: have %v, want %v.", outs[i/2], data[i])
			}
			if data[i+1] != outs[i/2+1] {
				t.Errorf("push/pop mismatch: have %v, want %v.", outs[i/2+1], data[i+1])
			}
		}
		for i := 0; i < size; i += 4 {
			if data[i] != rest[len(rest)-1-i/2] {
				t.Errorf("push/pop mismatch: have %v, want %v.", rest[len(rest)-1-i/2], data[i])
			}
			if data[i+3] != rest[len(rest)-1-i/2-1] {
				t.Errorf("push/pop mismatch: have %v, want %v.", rest[len(rest)-1-i/2-1], data[i+1])
			}
		}
	}
}

func TestQueue(t *testing.T) {
	// Create some initial data
	size := 16 * blockSize
	data := make([]int, size)
	for i := 0; i < size; i++ {
		data[i] = rand.Int()
	}
	// Simulate a queue in both directions
	deque := New()
	for rep := 0; rep < 2; rep++ {
		for _, val := range data {
			deque.PushLeft(val)
		}
		outs := []int{}
		for !deque.Empty() {
			outs = append(outs, deque.PopRight().(int))
		}
		for i := 0; i < len(data); i++ {
			if data[i] != outs[i] {
				t.Errorf("push/pop mismatch: have %v, want %v.", outs[i], data[i])
			}
		}
		for _, val := range data {
			deque.PushRight(val)
		}
		outs = []int{}
		for !deque.Empty() {
			outs = append(outs, deque.PopLeft().(int))
		}
		for i := 0; i < len(data); i++ {
			if data[i] != outs[i] {
				t.Errorf("push/pop mismatch: have %v, want %v.", outs[i], data[i])
			}
		}
	}
}

func TestStack(t *testing.T) {
	// Create some initial data
	size := 16 * blockSize
	data := make([]int, size)
	for i := 0; i < size; i++ {
		data[i] = rand.Int()
	}
	// Simulate a stack in both directions
	deque := New()
	for rep := 0; rep < 2; rep++ {
		for _, val := range data {
			deque.PushLeft(val)
		}
		outs := []int{}
		for !deque.Empty() {
			outs = append(outs, deque.PopLeft().(int))
		}
		for i := 0; i < len(data); i++ {
			if data[i] != outs[len(outs)-i-1] {
				t.Errorf("push/pop mismatch: have %v, want %v.", outs[len(outs)-i-1], data[i])
			}
		}
		for _, val := range data {
			deque.PushRight(val)
		}
		outs = []int{}
		for !deque.Empty() {
			outs = append(outs, deque.PopRight().(int))
		}
		for i := 0; i < len(data); i++ {
			if data[i] != outs[len(outs)-i-1] {
				t.Errorf("push/pop mismatch: have %v, want %v.", outs[len(outs)-i-1], data[i])
			}
		}
	}
}

func TestReset(t *testing.T) {
	size := 16 * blockSize
	deque := New()
	for rep := 0; rep < 2; rep++ {
		// Push some stuff into the deque
		for i := 0; i < size; i++ {
			deque.PushLeft(i)
		}
		// Clear and verify
		deque.Reset()
		if !deque.Empty() {
			t.Errorf("deque not empty after reset: %v", deque)
		}
		// Push again and verify
		for i := 0; i < size; i++ {
			deque.PushLeft(i)
			if deque.Right() != i {
				t.Errorf("corrupt state after reset: have %v, want %v.", deque.Right(), i)
			}
			deque.PopRight()
		}
	}
}

func BenchmarkPush(b *testing.B) {
	deque := New()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			deque.PushLeft(i)
		} else {
			deque.PushRight(i)
		}
	}
}

func BenchmarkPop(b *testing.B) {
	deque := New()
	for i := 0; i < b.N; i++ {
		deque.PushLeft(i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			deque.PopLeft()
		} else {
			deque.PopRight()
		}
	}
}
