// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"log"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/logging"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// GraphBuilder is an interface that can be implemented and used with
// OpenTofu to build the graph that OpenTofu walks.
type GraphBuilder interface {
	// Build builds the graph for the given module path. It is up to
	// the interface implementation whether this build should expand
	// the graph or not.
	Build(context.Context, addrs.ModuleInstance) (*Graph, tfdiags.Diagnostics)
}

// BasicGraphBuilder is a GraphBuilder that builds a graph out of a
// series of transforms and (optionally) validates the graph is a valid
// structure.
type BasicGraphBuilder struct {
	Steps []GraphTransformer
	// Optional name to add to the graph debug log
	Name string
}

func (b *BasicGraphBuilder) Build(ctx context.Context, path addrs.ModuleInstance) (*Graph, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	g := &Graph{Path: path}

	var lastStepStr string
	for _, step := range b.Steps {
		if step == nil {
			continue
		}
		log.Printf("[TRACE] Executing graph transform %T", step)

		err := step.Transform(ctx, g)

		if logging.IsDebugOrHigher() {
			if thisStepStr := g.StringWithNodeTypes(); thisStepStr != lastStepStr {
				log.Printf("[TRACE] Completed graph transform %T with new graph:\n%s  ------", step, logging.Indent(thisStepStr))
				lastStepStr = thisStepStr
			} else {
				log.Printf("[TRACE] Completed graph transform %T (no changes)", step)
			}
		}

		if err != nil {
			if nf, isNF := err.(tfdiags.NonFatalError); isNF {
				diags = diags.Append(nf.Diagnostics)
			} else {
				diags = diags.Append(err)
				return g, diags
			}
		}
	}

	if err := g.Validate(); err != nil {
		log.Printf("[ERROR] Graph validation failed. Graph:\n\n%s", g.String())
		diags = diags.Append(err)
		return nil, diags
	}

	return g, diags
}
