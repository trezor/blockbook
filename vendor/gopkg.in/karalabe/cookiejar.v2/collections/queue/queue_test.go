// CookieJar - A contestant's algorithm toolbox
// Copyright (c) 2013 Peter Szilagyi. All rights reserved.
//
// CookieJar is dual licensed: use of this source code is governed by a BSD
// license that can be found in the LICENSE file. Alternatively, the CookieJar
// toolbox may be used in accordance with the terms and conditions contained
// in a signed written agreement between you and the author(s).

package queue

import (
	"math/rand"
	"testing"
)

func TestQueue(t *testing.T) {
	// Create some initial data
	size := 16 * blockSize
	data := make([]int, size)
	for i := 0; i < size; i++ {
		data[i] = rand.Int()
	}
	queue := New()
	for rep := 0; rep < 2; rep++ {
		// Push all the data into the queue, pop out every second, then the rest
		outs := []int{}
		for i := 0; i < size; i++ {
			queue.Push(data[i])
			if i%2 == 0 {
				outs = append(outs, queue.Pop().(int))
				if i > 0 && queue.Front() != data[len(outs)] {
					t.Errorf("pop/front mismatch: have %v, want %v.", queue.Front(), data[len(outs)])
				}
			}
			if queue.Size() != (i+1)/2 {
				t.Errorf("size mismatch: have %v, want %v.", queue.Size(), (i+1)/2)
			}
		}
		for !queue.Empty() {
			outs = append(outs, queue.Pop().(int))
		}
		// Make sure the contents of the resulting slices are ok
		for i := 0; i < size; i++ {
			if data[i] != outs[i] {
				t.Errorf("push/pop mismatch: have %v, want %v.", outs[i], data[i])
			}
		}
	}
}

func TestReset(t *testing.T) {
	size := 16 * blockSize
	queue := New()
	for rep := 0; rep < 2; rep++ {
		// Push some stuff into the queue
		for i := 0; i < size; i++ {
			queue.Push(i)
		}
		// Clear and verify
		queue.Reset()
		if !queue.Empty() {
			t.Errorf("queue not empty after reset: %v", queue)
		}
		// Push some stuff into the queue and verify
		for i := 0; i < size; i++ {
			queue.Push(i)
		}
		for i := 0; i < size; i++ {
			if queue.Front() != i {
				t.Errorf("corrupt state after reset: have %v, want %v.", queue.Front(), i)
			}
			queue.Pop()
		}
	}
}

func BenchmarkPush(b *testing.B) {
	queue := New()
	for i := 0; i < b.N; i++ {
		queue.Push(i)
	}
}

func BenchmarkPop(b *testing.B) {
	queue := New()
	for i := 0; i < b.N; i++ {
		queue.Push(i)
	}
	b.ResetTimer()
	for !queue.Empty() {
		queue.Pop()
	}
}
