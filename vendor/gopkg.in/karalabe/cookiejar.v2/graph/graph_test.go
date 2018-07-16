// CookieJar - A contestant's algorithm toolbox
// Copyright 2013 Peter Szilagyi. All rights reserved.
//
// CookieJar is dual licensed: use of this source code is governed by a BSD
// license that can be found in the LICENSE file. Alternatively, the CookieJar
// toolbox may be used in accordance with the terms and conditions contained
// in a signed written agreement between you and the author(s).

package graph

import (
	"sort"
	"testing"
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
}

func TestGraph(t *testing.T) {
	for i, tt := range graphTests {
		// Assemble the graph
		graph := New(tt.nodes)
		for v, peers := range tt.edges {
			for _, peer := range peers {
				graph.Connect(v, peer)
			}
		}
		// Check basic properties
		if graph.Vertices() != tt.nodes {
			t.Errorf("test %d: vertex count mismatch: have %v, want %v.", i, graph.Vertices(), tt.nodes)
		}
		for v := 0; v < graph.Vertices(); v++ {
			// Collect the bigger neighbors
			peers := []int{}
			graph.Do(v, func(vertex interface{}) {
				if v < vertex.(int) {
					peers = append(peers, vertex.(int))
				}
			})
			// Sort due to undefined ordering
			sort.Sort(sort.IntSlice(tt.edges[v]))
			sort.Sort(sort.IntSlice(peers))

			// Ensure results are the same as input
			if len(peers) != len(tt.edges[v]) {
				t.Errorf("test %d: neighbor set size mismatch: have %v, want %v.", i, len(peers), len(tt.edges[v]))
			} else {
				for j := 0; j < len(peers); j++ {
					if peers[j] != tt.edges[v][j] {
						t.Errorf("test %d: neighbor mismatch: have %v, want %v.", i, peers[j], tt.edges[v][j])
					}
				}
			}
		}
	}
}
