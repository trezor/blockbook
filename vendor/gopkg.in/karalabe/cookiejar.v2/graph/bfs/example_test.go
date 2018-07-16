// CookieJar - A contestant's algorithm toolbox
// Copyright 2013 Peter Szilagyi. All rights reserved.
//
// CookieJar is dual licensed: use of this source code is governed by a BSD
// license that can be found in the LICENSE file. Alternatively, the CookieJar
// toolbox may be used in accordance with the terms and conditions contained
// in a signed written agreement between you and the author(s).

package bfs_test

import (
	"fmt"

	"gopkg.in/karalabe/cookiejar.v2/graph"
	"gopkg.in/karalabe/cookiejar.v2/graph/bfs"
)

// Small API demo based on a trie graph and a few disconnected vertices.
func Example_usage() {
	// Create the graph
	g := graph.New(7)
	g.Connect(0, 1)
	g.Connect(1, 2)
	g.Connect(1, 4)
	g.Connect(2, 3)
	g.Connect(4, 5)

	// Create the breadth first search algo structure for g and source node #2
	b := bfs.New(g, 0)

	// Get the path between #0 (source) and #2
	fmt.Println("Path 0->5:", b.Path(5))
	fmt.Println("Order:", b.Order())
	fmt.Println("Reachable #4 #6:", b.Reachable(4), b.Reachable(6))

	// Output:
	// Path 0->5: [0 1 4 5]
	// Order: [0 1 2 4 3 5]
	// Reachable #4 #6: true false
}
