// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"log"

	"github.com/opentofu/opentofu/internal/logging"
)

// GraphTransformer is the interface that transformers implement. This
// interface is only for transforms that need entire graph visibility.
type GraphTransformer interface {
	Transform(context.Context, *Graph) error
}

type graphTransformerMulti struct {
	Transforms []GraphTransformer
}

func (t *graphTransformerMulti) Transform(ctx context.Context, g *Graph) error {
	var lastStepStr string
	for _, t := range t.Transforms {
		log.Printf("[TRACE] (graphTransformerMulti) Executing graph transform %T", t)
		if err := t.Transform(ctx, g); err != nil {
			return err
		}

		if logging.IsDebugOrHigher() {
			if thisStepStr := g.StringWithNodeTypes(); thisStepStr != lastStepStr {
				log.Printf("[TRACE] (graphTransformerMulti) Completed graph transform %T with new graph:\n%s  ------", t, logging.Indent(thisStepStr))
				lastStepStr = thisStepStr
			} else {
				log.Printf("[TRACE] (graphTransformerMulti) Completed graph transform %T (no changes)", t)
			}
		}
	}

	return nil
}

// GraphTransformMulti combines multiple graph transformers into a single
// GraphTransformer that runs all the individual graph transformers.
func GraphTransformMulti(ts ...GraphTransformer) GraphTransformer {
	return &graphTransformerMulti{Transforms: ts}
}
