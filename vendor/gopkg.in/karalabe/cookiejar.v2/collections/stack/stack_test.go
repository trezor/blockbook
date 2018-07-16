// CookieJar - A contestant's algorithm toolbox
// Copyright (c) 2013 Peter Szilagyi. All rights reserved.
//
// CookieJar is dual licensed: use of this source code is governed by a BSD
// license that can be found in the LICENSE file. Alternatively, the CookieJar
// toolbox may be used in accordance with the terms and conditions contained
// in a signed written agreement between you and the author(s).

package stack

import (
	"math/rand"
	"testing"
)

func TestStack(t *testing.T) {
	// Create some initial data
	size := 16 * blockSize
	data := make([]int, size)
	for i := 0; i < size; i++ {
		data[i] = rand.Int()
	}
	stack := New()
	for rep := 0; rep < 2; rep++ {
		// Push all the data into the stack, pop out every second
		secs := []int{}
		for i := 0; i < size; i++ {
			stack.Push(data[i])
			if stack.Top() != data[i] {
				t.Errorf("push/top mismatch: have %v, want %v.", stack.Top(), data[i])
			}
			if i%2 == 0 {
				secs = append(secs, stack.Pop().(int))
			}
			if stack.Size() != (i+1)/2 {
				t.Errorf("size mismatch: have %v, want %v.", stack.Size(), (i+1)/2)
			}
		}
		rest := []int{}
		for !stack.Empty() {
			rest = append(rest, stack.Pop().(int))
		}
		// Make sure the contents of the resulting slices are ok
		for i := 0; i < size; i++ {
			if i%2 == 0 && data[i] != secs[i/2] {
				t.Errorf("push/pop mismatch: have %v, want %v.", secs[i/2], data[i])
			}
			if i%2 == 1 && data[i] != rest[len(rest)-i/2-1] {
				t.Errorf("push/pop mismatch: have %v, want %v.", rest[len(rest)-i/2-1], data[i])
			}
		}
	}
}

func TestReset(t *testing.T) {
	size := 16 * blockSize
	stack := New()
	for rep := 0; rep < 2; rep++ {
		// Push some stuff onto the stack
		for i := 0; i < size; i++ {
			stack.Push(i)
		}
		// Clear and verify
		stack.Reset()
		if !stack.Empty() {
			t.Errorf("stack not empty after reset: %v.", stack)
		}
		// Push some stuff onto the stack and verify
		for i := 0; i < size; i++ {
			stack.Push(i)
		}
		for i := size - 1; i >= 0; i-- {
			if stack.Top() != i {
				t.Errorf("corrupt state after reset: have %v, want %v.", stack.Top(), i)
			}
			stack.Pop()
		}
	}
}

func BenchmarkPush(b *testing.B) {
	stack := New()
	for i := 0; i < b.N; i++ {
		stack.Push(i)
	}
}

func BenchmarkPop(b *testing.B) {
	stack := New()
	for i := 0; i < b.N; i++ {
		stack.Push(i)
	}
	b.ResetTimer()
	for !stack.Empty() {
		stack.Pop()
	}
}
