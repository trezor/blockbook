// CookieJar - A contestant's algorithm toolbox
// Copyright 2013 Peter Szilagyi. All rights reserved.
//
// CookieJar is dual licensed: use of this source code is governed by a BSD
// license that can be found in the LICENSE file. Alternatively, the CookieJar
// toolbox may be used in accordance with the terms and conditions contained
// in a signed written agreement between you and the author(s).

package bfs

import (
	"testing"

	"gopkg.in/karalabe/cookiejar.v2/graph"
)

type graphTest struct {
	nodes int
	edges [][]int
}

var graphTests = []graphTest{
	// Random Erdos-Renyi graph with 64 nodes and 0.25 conenction probability.
	{
		64,
		[][]int{
			{48, 34, 9, 57, 11, 61, 16, 17, 18, 51, 21, 23, 25, 60, 29, 63},
			{32, 34, 5, 7, 8, 42, 15, 48, 17, 50, 19, 52, 57, 26, 63},
			{4, 8, 42, 51, 20, 22, 57, 36, 29},
			{34, 35, 4, 38, 7, 14, 17, 57, 54, 23, 36, 26},
			{32, 48, 47, 16, 51, 20, 21, 23, 24, 26, 31},
			{6, 9, 11, 44, 46, 19, 22, 23, 60},
			{8, 10, 11, 12, 14, 15, 16, 17, 23, 25, 30, 33, 34, 37, 39, 45, 46, 48, 54, 55, 59},
			{37, 35, 49, 8, 60, 45, 14, 17, 19, 20, 46, 56, 25, 26, 28, 29, 30, 53},
			{33, 43, 12, 48, 17, 20, 53, 22, 60},
			{32, 36, 40, 43, 12, 47, 49, 18, 20, 21, 22, 23, 24, 52, 58, 59, 30},
			{42, 46, 14, 50, 19, 20, 21, 23, 52, 58, 27, 60, 53},
			{48, 59, 39, 46, 15, 16, 19, 58, 38, 51},
			{33, 38, 42, 43, 15, 16, 40, 51, 22, 41, 24, 58, 27, 60, 29},
			{33, 37, 38, 49, 40, 42, 43, 50, 15, 16, 17, 18, 29},
			{36, 38, 40, 46, 17, 50, 20, 23, 63},
			{35, 40, 46, 51, 21, 22, 55, 24, 58, 60, 61, 53},
			{34, 38, 39, 40, 46, 51, 57, 27, 61, 30},
			{34, 54, 55, 22, 23, 57, 31},
			{32, 36, 51, 40, 42, 19, 20, 41, 52, 27, 62, 63},
			{50, 46, 20, 53, 30},
			{59, 44, 47, 21, 54, 56, 58, 27, 42},
			{33, 45, 48, 56, 53, 54, 24, 26, 47},
			{24, 25, 30, 37, 40, 41, 48, 49, 50, 52, 54, 55, 60, 62, 63},
			{32, 33, 43, 44, 45, 48, 24, 25, 61},
			{37, 38, 39, 40, 61, 48, 50, 51, 25, 26, 60, 58, 62, 63},
			{38, 41, 42, 39, 49, 50, 56, 58, 60, 29, 31},
			{33, 43, 45, 46, 47, 51, 54, 55, 58, 31},
			{33, 38, 39, 46, 49, 51, 62, 59, 28, 61, 30},
			{35, 38, 39, 59, 43, 50, 53, 54, 58, 30},
			{38, 40, 41, 42, 39, 46, 47, 51, 61},
			{59, 36, 54, 42, 43, 60, 49, 50, 52, 53, 62, 63},
			{34, 40, 44, 46, 48, 53, 61},
			{50, 33, 44, 45, 53, 54, 58, 63},
			{35, 37, 43, 46, 40, 56, 55, 60, 53},
			{49, 41, 50, 60, 62},
			{37, 42, 43, 48, 61},
			{48, 53, 55, 56, 57, 63},
			{40, 41, 39, 44, 56, 50, 52, 57, 58, 60, 62},
			{44, 53, 54, 57, 58, 62},
			{43, 44, 61, 47, 49, 58, 62},
			{50, 62},
			{52, 53, 59},
			{46, 50, 49, 51, 53, 57, 58, 52},
			{45, 44, 56, 47},
			{45, 47, 53, 54, 62},
			{59, 60, 61, 62, 53},
			{47, 49, 53, 56},
			{57, 49, 55, 52, 62},
			{56, 52, 63, 53, 55},
			{59, 53, 58, 60},
			{61, 53, 58},
			{59, 58, 53, 60},
			{53, 58, 60},
			{55, 56},
			{60, 63},
			{60, 62, 63},
			{63, 57, 59},
			{58, 63},
			{59, 60, 63},
			{61, 63},
			{63},
			{63},
			{},
			{},
		},
	},
	// Random Erdos-Renyi graph with 64 nodes and 0.1 conenction probability.
	{
		64,
		[][]int{
			{38, 40, 12, 61, 47, 17, 57, 28, 29},
			{5, 15, 22, 24, 26, 47, 28, 63},
			{33, 4, 40, 13, 15, 17, 25, 27, 62},
			{17, 4, 6, 49},
			{5, 20, 23, 27, 37},
			{32, 18, 20, 56, 57},
			{9, 55},
			{34, 63, 13, 21},
			{37, 60, 14, 47, 17, 52, 53, 28},
			{56, 45},
			{43, 58, 19, 63},
			{36, 42, 13, 14, 16, 46, 22, 56},
			{40, 30, 23},
			{40, 41, 45, 46, 56},
			{47, 50, 51, 22, 26},
			{37, 44, 45, 25, 26},
			{32, 49, 51, 22, 56},
			{36, 45, 47, 52, 56},
			{39, 43, 58, 60, 62},
			{57},
			{47, 56, 24, 61},
			{32, 49, 55},
			{34, 33, 47, 55, 39, 62, 63},
			{40, 44, 25, 60, 63},
			{38, 39, 46, 54, 57, 30},
			{35, 41, 45, 34, 52},
			{38, 39, 45, 27},
			{42},
			{33, 39, 40, 47, 51, 55, 60},
			{50, 63, 39},
			{36, 43, 57},
			{34, 35, 40, 42, 52, 53, 57},
			{34, 36, 50, 56},
			{36, 39, 44, 55},
			{38, 51},
			{47},
			{51, 55},
			{61},
			{48},
			{41, 44, 52},
			{60, 50},
			{48, 49, 50, 59},
			{46},
			{45, 47, 53, 61, 63},
			{55, 58, 61},
			{49, 48, 57},
			{58},
			{49, 55},
			{50, 51},
			{},
			{},
			{52, 53, 58},
			{57},
			{63, 59, 61},
			{56, 59},
			{61, 62},
			{},
			{},
			{62},
			{60, 63},
			{},
			{},
			{},
			{},
		},
	},
}

func TestBFS(t *testing.T) {
	for i, tt := range graphTests {
		// Assemble the graph
		g := graph.New(tt.nodes)
		for v, peers := range tt.edges {
			for _, peer := range peers {
				g.Connect(v, peer)
			}
		}
		// Create a bfs structure and verify it
		for src := 0; src < tt.nodes; src++ {
			b := New(g, src)

			// Ensure that paths are indeed connected links
			for dst := 0; dst < tt.nodes; dst++ {
				if b.Reachable(dst) {
					// If reachable, generate the path and verify each link
					if path := b.Path(dst); path == nil {
						t.Errorf("test %d: reachable nil path %v->%v.", i, src, dst)
					} else {
						for p := 1; p < len(path); p++ {
							a := path[p-1]
							b := path[p]
							if a > b {
								a, b = b, a
							}
							found := false
							for _, v := range tt.edges[a] {
								if v == b {
									found = true
									break
								}
							}
							if !found {
								t.Errorf("test %d: path link %v-%v not found.", i, a, b)
							}
						}
					}
				} else {
					// If not reachable, make sure path is also nil
					if path := b.Path(dst); path != nil {
						t.Errorf("test %d: non reachable path %v->%v: have %v, want %v.", i, src, dst, path, nil)
					}
				}
			}
			// Ensure that the order is consistent with the paths returned
			ord := b.Order()
			for dst := 0; dst < tt.nodes; dst++ {
				if b.Reachable(dst) {
					path := b.Path(dst)
					for oi, pi := 0, 0; pi < len(path); pi++ {
						for ord[oi] != path[pi] {
							if oi >= len(ord) {
								t.Errorf("test %d: order/path mismatch: o=%v, p=%v.", i, ord, path)
							}
							oi++
						}
					}
				}
			}
		}
	}
}

func TestBreadth(t *testing.T) {
	// Create a simple graph and check breadth visit order
	g := graph.New(6)
	g.Connect(0, 1)
	g.Connect(1, 2)
	g.Connect(1, 4)
	g.Connect(2, 3)
	g.Connect(4, 5)
	g.Connect(3, 5)

	// Check the bfs paths
	b := New(g, 0)
	if p := b.Path(5); len(p) != 4 || p[0] != 0 || p[1] != 1 || p[2] != 4 || p[3] != 5 {
		t.Errorf("path mismatch: have %v, want%v.", p, []int{0, 1, 4, 5})
	}
}
