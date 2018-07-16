// CookieJar - A contestant's algorithm toolbox
// Copyright 2013 Peter Szilagyi. All rights reserved.
//
// CookieJar is dual licensed: you can redistribute it and/or modify it under
// the terms of the GNU General Public License as published by the Free Software
// Foundation, either version 3 of the License, or (at your option) any later
// version.
//
// The toolbox is distributed in the hope that it will be useful, but WITHOUT
// ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
// FITNESS FOR A PARTICULAR PURPOSE.  See the GNU General Public License for
// more details.
//
// Alternatively, the CookieJar toolbox may be used in accordance with the terms
// and conditions contained in a signed written agreement between you and the
// author(s).

// Package dfs implements the depth-first-search algorithm for the graphs.
//
// The DFS is implemented using on demand calculations, meaning that only that
// part of the search space will be expanded as requested, iteratively expanding
// it if needed.
//
// Neighbor traversal order currently is random due to the graph implementation.
// Specific order may be added in the future.
package dfs

import (
	"gopkg.in/karalabe/cookiejar.v2/collections/stack"
	"gopkg.in/karalabe/cookiejar.v2/graph"
)

// Depth-first-search algorithm data structure.
type Dfs struct {
	graph  *graph.Graph
	source int

	visited []bool
	parents []int
	order   []int

	pending *stack.Stack
	builder *stack.Stack
}

// Creates a new random-order dfs structure.
func New(g *graph.Graph, src int) *Dfs {
	d := new(Dfs)

	d.graph = g
	d.source = src

	d.visited = make([]bool, g.Vertices())
	d.parents = make([]int, g.Vertices())
	d.order = make([]int, 0, g.Vertices())

	d.pending = stack.New()
	d.pending.Push(src)
	d.builder = stack.New()

	return d
}

// Generates the path from the source node to the destination.
func (d *Dfs) Path(dst int) []int {
	// If not found yet, but processing's not done, search
	if !d.visited[dst] && !d.pending.Empty() {
		d.search(dst)
	}
	// If done but still not found return a nil slice
	if !d.visited[dst] {
		return nil
	}
	// Generate the path and return
	for dst != d.source {
		d.builder.Push(dst)
		dst = d.parents[dst]
	}
	d.builder.Push(dst)

	path := make([]int, d.builder.Size())
	for i := 0; i < len(path); i++ {
		path[i] = d.builder.Pop().(int)
	}
	return path
}

// Checks whether a given vertex is reachable from the source.
func (d *Dfs) Reachable(dst int) bool {
	if !d.visited[dst] && !d.pending.Empty() {
		d.search(dst)
	}
	return d.visited[dst]
}

// Generates the full order in which nodes were traversed.
func (d *Dfs) Order() []int {
	// Force dfs termination
	if !d.pending.Empty() {
		d.search(-1)
	}
	return d.order
}

// Continues the DFS search from the last yield point, looking for dst.
func (d *Dfs) search(dst int) {
	for !d.pending.Empty() {
		// Fetch the next node, and visit if new
		src := d.pending.Pop().(int)
		if !d.visited[src] {
			d.visited[src] = true
			d.order = append(d.order, src)

			d.graph.Do(src, func(peer interface{}) {
				if p := peer.(int); !d.visited[p] {
					d.parents[p] = src
					d.pending.Push(p)
				}
			})
		}
		// If we found the destination, yield
		if dst == src {
			return
		}
	}
}
