// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"testing"

	"github.com/opentofu/opentofu/internal/dag"
)

// testGraphNotContains is an assertion helper that tests that a node is
// NOT contained in the graph.
func testGraphNotContains(t *testing.T, g *Graph, name string) {
	for _, v := range g.Vertices() {
		if dag.VertexName(v) == name {
			t.Fatalf(
				"Expected %q to NOT be in:\n\n%s",
				name, g.String())
		}
	}
}

// testGraphHappensBefore is an assertion helper that tests that node
// A (dag.VertexName value) happens before node B.
func testGraphHappensBefore(t *testing.T, g *Graph, A, B string) {
	t.Helper()
	// Find the B vertex
	var vertexB dag.Vertex
	for _, v := range g.Vertices() {
		if dag.VertexName(v) == B {
			vertexB = v
			break
		}
	}
	if vertexB == nil {
		t.Fatalf(
			"Expected %q before %q. Couldn't find %q in:\n\n%s",
			A, B, B, g.String())
	}

	// Look at ancestors
	deps, err := g.Ancestors(vertexB)
	if err != nil {
		t.Fatalf("Error: %s in graph:\n\n%s", err, g.String())
	}

	// Make sure B is in there
	for _, v := range deps.List() {
		if dag.VertexName(v) == A {
			// Success
			return
		}
	}

	t.Fatalf(
		"Expected %q before %q in:\n\n%s",
		A, B, g.String())
}
