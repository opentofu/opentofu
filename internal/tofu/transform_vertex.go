// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"fmt"

	"github.com/opentofu/opentofu/internal/dag"
)

// VertexTransformer is a GraphTransformer that transforms vertices
// using the GraphVertexTransformers. The Transforms are run in sequential
// order. If a transform replaces a vertex then the next transform will see
// the new vertex.
type VertexTransformer struct {
	Transforms []GraphVertexTransformer
}

func (t *VertexTransformer) Transform(ctx context.Context, g *Graph) error {
	for _, v := range g.Vertices() {
		for _, vt := range t.Transforms {
			newV, err := vt.Transform(v)
			if err != nil {
				return err
			}

			// If the vertex didn't change, then don't do anything more
			if newV == v {
				continue
			}

			// Vertex changed, replace it within the graph
			if ok := g.Replace(v, newV); !ok {
				// This should never happen, big problem
				return fmt.Errorf(
					"failed to replace %s with %s!\n\nSource: %#v\n\nTarget: %#v",
					dag.VertexName(v), dag.VertexName(newV), v, newV)
			}

			// Replace v so that future transforms use the proper vertex
			v = newV
		}
	}

	return nil
}
