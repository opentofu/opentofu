// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval

import (
	"context"

	"github.com/opentofu/opentofu/internal/lang/eval/internal/evalglue"
)

// A PlanningOracle provides information from the configuration that is needed
// by the planning engine to help orchestrate the planning process.
type PlanningOracle struct {
	// NOTE: Any method of PlanningOracle that interacts with methods of
	// this or anything accessible through it MUST use
	// [grapheval.ContextWithNewWorker] to make sure it's using a
	// workgraph-friendly context, since the methods of this type are
	// exported entry points for use by callers in other packages that
	// don't necessarily participate in workgraph directly.
	rootModuleInstance evalglue.CompiledModuleInstance

	evalContext *EvalContext
}

func (o *PlanningOracle) EvalContext(ctx context.Context) *EvalContext {
	return o.evalContext
}
