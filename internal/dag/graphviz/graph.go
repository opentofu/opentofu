// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package graphviz

import (
	"bufio"
	"cmp"
	"fmt"
	"io"
	"maps"
	"slices"

	"github.com/opentofu/opentofu/internal/dag"
)

// Graph is a wrapper around a [dag.Graph] that annotates it with some
// additional information that's relevant when generating a Graphviz-language
// representation of that graph.
//
// A [dag.Graph] used with this type must only contain vertices whose dynamic
// type is [Node]. No other vertex types are allowed.
type Graph struct {
	Content *dag.Graph

	Attrs            Attributes
	DefaultNodeAttrs Attributes
	DefaultEdgeAttrs Attributes

	DefaultEdgeDirectionIn  EdgeAttachmentDirection
	DefaultEdgeDirectionOut EdgeAttachmentDirection
}

// WriteDirecteGraph generates a graphviz-language representation of the given
// graph on the given writer.
//
// If this function returns an error then an unspecified amount of partial data
// might already have been written to the writer before returning it.
func WriteDirectedGraph(g *Graph, w io.Writer) error {
	var err error

	// We'll wrap the writer in a buffered writer for more convenient
	// partial writes.
	bw := bufio.NewWriter(w)

	_, err = bw.WriteString("digraph {\n")
	if err != nil {
		return err
	}
	if len(g.Attrs) != 0 {
		names := slices.Collect(maps.Keys(g.Attrs))
		slices.Sort(names)
		for _, name := range names {
			val := g.Attrs[name]
			_, err = bw.WriteString("  ")
			if err != nil {
				return err
			}
			err = writeGraphvizAttr(name, val, bw)
			if err != nil {
				return err
			}
			_, err = bw.WriteString(";\n")
			if err != nil {
				return err
			}
		}
	}
	if len(g.DefaultNodeAttrs) != 0 {
		_, err = bw.WriteString("  node [")
		if err != nil {
			return err
		}
		err = writeGraphvizAttrList(g.DefaultNodeAttrs, bw)
		if err != nil {
			return err
		}
		_, err = bw.WriteString("];\n")
		if err != nil {
			return err
		}
	}
	if len(g.DefaultEdgeAttrs) != 0 {
		_, err = bw.WriteString("  edge [")
		if err != nil {
			return err
		}
		err = writeGraphvizAttrList(g.DefaultEdgeAttrs, bw)
		if err != nil {
			return err
		}
		_, err = bw.WriteString("];\n")
		if err != nil {
			return err
		}
	}

	// We'll write the nodes out in lexical order by their unique IDs just so
	// that our output is deterministic for easier unit testing.
	var nodes []Node
	for v := range g.Content.VerticesSeq() {
		node, ok := v.(Node)
		if !ok {
			return fmt.Errorf("graph contains vertex of unsupported type %T; only %T is allowed", v, node)
		}
		nodes = append(nodes, node)
	}
	slices.SortFunc(nodes, func(a, b Node) int {
		return cmp.Compare(a.ID, b.ID)
	})
	for _, node := range nodes {
		_, err = bw.WriteString("  ")
		if err != nil {
			return err
		}
		_, err = bw.WriteString(quoteForGraphviz(node.ID))
		if err != nil {
			return err
		}
		if len(node.Attrs) != 0 {
			_, err = bw.WriteString(" [")
			if err != nil {
				return err
			}
			err = writeGraphvizAttrList(node.Attrs, bw)
			if err != nil {
				return err
			}
			_, err = bw.WriteString("]")
			if err != nil {
				return err
			}
		}
		_, err = bw.WriteString(";\n")
		if err != nil {
			return err
		}
	}

	// We also sort the edges into lexical order first by the node they
	// emerge from and then by the node they connect to.
	var edges [][2]Node
	for e := range g.Content.EdgesSeq() {
		// These type assertions cannot fail because we checked above that
		// all verticies in the graph are [Node].
		src := e.Source().(Node)
		dst := e.Target().(Node)
		edges = append(edges, [2]Node{src, dst})
	}
	slices.SortFunc(edges, func(a, b [2]Node) int {
		srcCmp := cmp.Compare(a[0].ID, b[0].ID)
		if srcCmp != 0 {
			return srcCmp
		}
		return cmp.Compare(a[1].ID, b[1].ID)
	})
	for _, edge := range edges {
		src := edge[0]
		dst := edge[1]
		_, err = bw.WriteString("  ")
		if err != nil {
			return err
		}
		_, err = bw.WriteString(quoteForGraphviz(src.ID))
		if err != nil {
			return err
		}
		_, err = bw.WriteString(string(g.DefaultEdgeDirectionOut))
		if err != nil {
			return err
		}
		_, err = bw.WriteString(" -> ")
		if err != nil {
			return err
		}
		_, err = bw.WriteString(quoteForGraphviz(dst.ID))
		if err != nil {
			return err
		}
		_, err = bw.WriteString(string(g.DefaultEdgeDirectionIn))
		if err != nil {
			return err
		}
		_, err = bw.WriteString(";\n")
		if err != nil {
			return err
		}
	}

	_, err = bw.WriteString("}\n")
	if err != nil {
		return err
	}

	return bw.Flush()
}
