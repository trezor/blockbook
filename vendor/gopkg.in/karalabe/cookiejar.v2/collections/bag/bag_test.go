// CookieJar - A contestant's algorithm toolbox
// Copyright (c) 2013 Peter Szilagyi. All rights reserved.
//
// CookieJar is dual licensed: use of this source code is governed by a BSD
// license that can be found in the LICENSE file. Alternatively, the CookieJar
// toolbox may be used in accordance with the terms and conditions contained
// in a signed written agreement between you and the author(s).

package bag

import (
	"math/rand"
	"testing"
)

func TestBag(t *testing.T) {
	// Create some initial data
	size := 65536
	data := make([]int, size)
	for i := 0; i < size; i++ {
		data[i] = rand.Int()
	}
	// Insert the data into the bag, but remove every second
	bag := New()
	for i := 0; i < len(data); i++ {
		bag.Insert(data[i])
		if i%2 == 0 {
			bag.Remove(data[i])
		}
	}
	// Make sure size works
	if bag.Size() != size/2 {
		t.Errorf("size mismatch: have %v, want %v", bag.Size(), size/2)
	}
	// Calculate the sum of the elements in and out
	sumBag := int64(0)
	bag.Do(func(val interface{}) {
		sumBag += int64(val.(int))
	})
	sumDat := int64(0)
	for i := 1; i < len(data); i += 2 {
		sumDat += int64(data[i])
	}
	if sumBag != sumDat {
		t.Errorf("sum mismatch after iteration: have %v, want %v", sumBag, sumDat)
	}
	// Verify the contents of the bag
	for i := 1; i < len(data); i += 2 {
		if bag.Count(data[i]) <= 0 {
			t.Errorf("expected data, none found: %v in %v", data[i], bag)
		}
		bag.Remove(data[i])
	}
	if len(bag.data) != 0 || bag.Size() != 0 {
		t.Errorf("leftovers remained in bag: %v", bag)
	}
}

func TestReset(t *testing.T) {
	// Create some initial data
	size := 65536
	data := make([]int, size)
	for i := 0; i < size; i++ {
		data[i] = rand.Int()
	}
	// Insert the data into the bag, but remove every second
	bag := New()
	for _, val := range data {
		bag.Insert(val)
	}
	// clear the bag and verify
	bag.Reset()
	if len(bag.data) != 0 || bag.Size() != 0 {
		t.Errorf("leftovers remained in bag: %v", bag)
	}
}

func BenchmarkInsert(b *testing.B) {
	// Create some initial data
	data := make([]int, b.N)
	for i := 0; i < len(data); i++ {
		data[i] = rand.Int()
	}
	// Execute the benchmark
	b.ResetTimer()
	bag := New()
	for _, val := range data {
		bag.Insert(val)
	}
}

func BenchmarkRemove(b *testing.B) {
	// Create some initial data
	data := make([]int, b.N)
	for i := 0; i < len(data); i++ {
		data[i] = rand.Int()
	}
	// Fill the bag with it
	bag := New()
	for _, val := range data {
		bag.Insert(val)
	}
	// Execute the benchmark
	b.ResetTimer()
	for _, val := range data {
		bag.Remove(val)
	}
}

func BenchmarkDo(b *testing.B) {
	// Create some initial data
	data := make([]int, b.N)
	for i := 0; i < len(data); i++ {
		data[i] = rand.Int()
	}
	// Fill the bag with it
	bag := New()
	for _, val := range data {
		bag.Insert(val)
	}
	// Execute the benchmark
	b.ResetTimer()
	bag.Do(func(val interface{}) {
		// Do nothing
	})
}
