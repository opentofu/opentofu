// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval

import (
	"context"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/lang/eval/internal/configgraph"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/evalglue"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// PlanGlue is used with [DrivePlanning] to allow the evaluation system to
// communicate with the planning engine that called it.
//
// Methods of this type can be called concurrently with themselves and with
// each other, and so implementations must use suitable synchronization to
// avoid data races between calls.
type PlanGlue interface {
	// Creates planned action(s) for the given resource instance and return
	// the planned new state that would result from those actions.
	//
	// This is called only for resource instances currently declared in the
	// configuration. The planning engine must deal with planning actions
	// for "orphaned" resource instances (those which are only present in
	// prior state) separately once [ConfigInstance.DrivePlanning] has returned.
	PlanDesiredResourceInstance(ctx context.Context, inst *DesiredResourceInstance, oracle *PlanningOracle) (cty.Value, tfdiags.Diagnostics)
}

// DrivePlanning uses this configuration instance to drive forward a planning
// process being executed by another part of the system.
//
// This function deals only with the configuration-driven portion of the
// process where the planning engine learns which resource instances are
// currently declared in the configuration. After this function returns
// the caller will need to compare that set of desired resource instances
// with the set of resource instances tracked in the prior state and then
// presumably generate additional planned actions to destroy any instances
// that are currently tracked but no longer configured.
func (c *ConfigInstance) DrivePlanning(ctx context.Context, glue PlanGlue) (*PlanningResult, tfdiags.Diagnostics) {
	// All of our work will be associated with a workgraph worker that serves
	// as the initial worker node in the work graph.
	ctx = grapheval.ContextWithNewWorker(ctx)

	relationships, diags := c.prepareToPlan(ctx)
	if diags.HasErrors() {
		return nil, diags
	}

	evalGlue := &planningEvalGlue{
		planEngineGlue: glue,
	}
	rootModuleInstance, moreDiags := c.newRootModuleInstance(ctx, evalGlue)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return nil, diags
	}
	evalGlue.oracle = &PlanningOracle{
		relationships:      relationships,
		rootModuleInstance: rootModuleInstance,
	}

	// The plan phase is driven forward by us evaluating expressions during
	// the "checkAll" process, and so we can just run that here and then
	// it'll cause various calls out to the "glue" object whenever we're
	// ready to provide configuration for a resource intance and need to
	// obtain its result for downstream use.
	moreDiags = checkAll(ctx, rootModuleInstance)
	diags = diags.Append(moreDiags)
	// (We intentionally don't return here because we'll make a best effort
	// to return a partial result even if we encountered errors, so an
	// operator can potentially use the partial result to help debug
	// the errors.)

	// Once checkAll has completed we should've either visited and evaluated
	// everything as much as we can, so we can now just collect the result
	// value and return.
	outputsVal, moreDiags := rootModuleInstance.ResultValuer(ctx).Value(ctx)
	diags = diags.Append(moreDiags)
	return &PlanningResult{
		RootModuleOutputs: configgraph.PrepareOutgoingValue(outputsVal),
	}, diags
}

// PlanningResult is the return value of [ConfigInstance.DrivePlanning],
// describing the top-level outcomes of the planning process.
type PlanningResult struct {
	// Oracle is the same [PlanningOracle] that was presented to zero or more
	// [PlanGlue.PlanDesiredResourceInstance] calls during the
	// [ConfigInstance.DrivePlanning] call, returned here so that it can
	// be used in the planning engine's followup work
	Oracle *PlanningOracle

	// RootModuleOutputs is the object representing the planned output values
	// from the root module.
	//
	// This will contain unknown value placeholders for any part of an output
	// value which depends on the result of an action that won't be taken
	// until the apply phase.
	RootModuleOutputs cty.Value
}

type planningEvalGlue struct {
	// planEngineGlue is the planning glue implementation provided by the
	// planning engine when it called [ConfigInstance.DrivePlanning].
	planEngineGlue PlanGlue

	// oracle is the PlanningOracle we'll pass to planEngineGlue when
	// we call it, so that it can request certain relevant information from
	// the configuration.
	oracle *PlanningOracle
}

var _ evalglue.Glue = (*planningEvalGlue)(nil)

// ResourceInstanceValue implements evalglue.Glue.
func (p *planningEvalGlue) ResourceInstanceValue(ctx context.Context, ri *configgraph.ResourceInstance, configVal cty.Value, providerInst configgraph.Maybe[*configgraph.ProviderInstance]) (cty.Value, tfdiags.Diagnostics) {
	desired := &DesiredResourceInstance{
		Addr:      ri.Addr,
		ConfigVal: configgraph.PrepareOutgoingValue(configVal),
		Provider:  ri.Provider,
	}
	if providerInst, ok := configgraph.GetKnown(providerInst); ok {
		desired.ProviderInstance = &providerInst.Addr
	}
	// TODO: Populate everything else in [DesiredResourceInstance], once
	// package configgraph knows how to provide those answers.

	return p.planEngineGlue.PlanDesiredResourceInstance(ctx, desired, p.oracle)
}
