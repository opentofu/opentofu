// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval

import (
	"context"

	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

// PlanGlue is used with [DrivePlanning] to allow the evaluation system to
// communicate with the planning engine that called it.
type PlanGlue interface {
	// Creates planned action(s) for the given resource instance and return
	// the planned new state that would result from those actions.
	//
	// This is called only for resource instances currently declared in the
	// configuration. The planning engine must deal with planning actions
	// for "orphaned" resource instances (those which are only present in
	// prior state) separately once [ConfigInstance.DrivePlanning] has returned.
	//
	// TODO: Figure out what information we'll pass to this method so it
	// knows what it's planning and how to plan it (what other resource
	// instances it depends on, what provider instance to use to plan it,
	// etc.)
	PlanDesiredResourceInstance(ctx context.Context /* ... */) (cty.Value, tfdiags.Diagnostics)
}

// DrivePlanning uses this configuration instance to drive forward a planning
// process being executed by another part of the system.
//
// This function deals only with the configuration-driven portion of the
// process where the planning engine learns which resource instances are
// currently declared in the configuration. After this function returns
// the caller will need to compare that set of configured resource instances
// with the set of resource instances tracked in the prior state and then
// presumably generate additional planned actions to destroy any instances
// that are currently tracked but no longer configured.
func (c *ConfigInstance) DrivePlanning(ctx context.Context, glue PlanGlue) tfdiags.Diagnostics {
	// All of our work will be associated with a workgraph worker that serves
	// as the initial worker node in the work graph.
	ctx = grapheval.ContextWithNewWorker(ctx)

	panic("unimplemented")
}
