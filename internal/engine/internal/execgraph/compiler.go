// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package execgraph

import (
	"context"

	"github.com/apparentlymart/go-workgraph/workgraph"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
)

func (g *Graph) Compile() *CompiledGraph {
	ret := &CompiledGraph{
		ops:                    make([]anyCompiledOperation, len(g.ops)),
		resourceInstanceValues: addrs.MakeMap[addrs.AbsResourceInstance, func(ctx context.Context) cty.Value](),
		cleanupWorker:          workgraph.NewWorker(),
	}
	c := &compiler{
		sourceGraph:   g,
		compiledGraph: ret,
	}
	return c.Compile()
}

// compiler is a temporary object we use during compilation to coordinate
// between all of the different parts of the compilation process.
//
// After compilation is complete, only the object from the compiledGraph
// field remains as the result.
type compiler struct {
	sourceGraph   *Graph
	compiledGraph *CompiledGraph
}

func (c *compiler) Compile() *CompiledGraph {
	// TODO: Implement
	return c.compiledGraph
}
