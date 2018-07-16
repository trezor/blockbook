// CookieJar - A contestant's algorithm toolbox
// Copyright 2013 Peter Szilagyi. All rights reserved.
//
// CookieJar is dual licensed: use of this source code is governed by a BSD
// license that can be found in the LICENSE file. Alternatively, the CookieJar
// toolbox may be used in accordance with the terms and conditions contained
// in a signed written agreement between you and the author(s).

// Package bfs implements the breadth-first-search algorithm for the graphs.
//
// The BFS is implemented using on demand calculations, meaning that only that
// part of the search space will be expanded as requested, iteratively expanding
// it if needed.
//
// Neighbor traversal order currently is random due to the graph implementation.
// Specific order may be added in the future.
package bfs

import (
	"gopkg.in/karalabe/cookiejar.v2/collections/queue"
	"gopkg.in/karalabe/cookiejar.v2/collections/stack"
	"gopkg.in/karalabe/cookiejar.v2/graph"
)

// Breadth-first-search algorithm data structure.
type Bfs struct {
	graph  *graph.Graph
	source int

	visited []bool
	parents []int
	order   []int
	paths   map[int][]int

	pending *queue.Queue
	builder *stack.Stack
}

// Creates a new random-order bfs structure.
func New(g *graph.Graph, src int) *Bfs {
	d := new(Bfs)

	d.graph = g
	d.source = src

	d.visited = make([]bool, g.Vertices())
	d.visited[src] = true
	d.parents = make([]int, g.Vertices())
	d.order = make([]int, 1, g.Vertices())
	d.order[0] = src
	d.paths = make(map[int][]int)

	d.pending = queue.New()
	d.pending.Push(src)
	d.builder = stack.New()

	return d
}

// Generates the path from the source node to the destination.
func (d *Bfs) Path(dst int) []int {
	// Return nil if not reachable
	if !d.Reachable(dst) {
		return nil
	}
	// If reachable, but path not yet generated, create and cache
	if cached, ok := d.paths[dst]; !ok {
		for cur := dst; cur != d.source; {
			d.builder.Push(cur)
			cur = d.parents[cur]
		}
		d.builder.Push(d.source)

		path := make([]int, d.builder.Size())
		for i := 0; i < len(path); i++ {
			path[i] = d.builder.Pop().(int)
		}
		d.paths[dst] = path
		return path
	} else {
		return cached
	}
}

// Checks whether a given vertex is reachable from the source.
func (d *Bfs) Reachable(dst int) bool {
	if !d.visited[dst] && !d.pending.Empty() {
		d.search(dst)
	}
	return d.visited[dst]
}

// Generates the full order in which nodes were traversed.
func (d *Bfs) Order() []int {
	// Force bfs termination
	if !d.pending.Empty() {
		d.search(-1)
	}
	return d.order
}

// Continues the bfs search from the last yield point, looking for dst.
func (d *Bfs) search(dst int) {
	for !d.pending.Empty() {
		// Fetch the next node, and visit if new
		src := d.pending.Pop().(int)
		d.graph.Do(src, func(peer interface{}) {
			if p := peer.(int); !d.visited[p] {
				d.visited[p] = true
				d.order = append(d.order, p)
				d.parents[p] = src
				d.pending.Push(p)
			}
		})
		// If we found the destination, yield
		if dst == src {
			return
		}
	}
}
