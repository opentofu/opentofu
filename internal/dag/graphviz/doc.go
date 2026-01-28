// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package graphviz contains some utilities to help when using "package dag"
// to prepare graph representations in the Graphviz language.
//
// It has two main parts:
//
//   - [Node] is a [dag.Vertex] implementation which corresponds to Graphviz's
//     idea of "graph nodes", giving each a unique identifier and a set of
//     arbitrary attributes to be included in the Graphviz-language description
//     of the node.
//   - [WriteDirectedGraph] takes a [dag.Graph] (or one of its subtypes) whose
//     nodes are all [Node] instances, and generates a Graphviz-language
//     representation of it as a "digraph".
//
// Although [dag.Edge] makes it seem like edge types can also be generic, in
// practice at the time of writing the implementation in that package actually
// expects all edges to be of the unexported edge type returned by
// [dag.BasicEdge], and so this package does not currently have any way to
// represent Graphviz edge attributes on a per-edge basis, although
// [RenderGraph] does allow providing a set of general attributes that should
// apply to all edges. OpenTofu does not currently tend to generate graphs with
// labelled or otherwise-customized edges.
package graphviz

import (
	"github.com/opentofu/opentofu/internal/dag"
)

// This is here only so that this file can import "dag" for the benefit of
// the package-level documentation comment above, so godoc will be able to
// cross-link to symbols in that package.
type _ = dag.Vertex
