// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval

import (
	"context"
	"fmt"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
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

// A PlanningOracle provides information from the configuration that is needed
// by the planning engine to help orchestrate the planning process.
type PlanningOracle struct {
	relationships *ResourceRelationships

	// NOTE: Any method of PlanningOracle that interacts with methods of
	// this or anything accessible through it MUST use
	// [grapheval.ContextWithNewWorker] to make sure it's using a
	// workgraph-friendly context, since the methods of this type are
	// exported entry points for use by callers in other packages that
	// don't necessarily participate in workgraph directly.
	rootModuleInstance evalglue.CompiledModuleInstance
}

// ProviderInstanceConfig returns a value representing the configuration to
// use when configuring the provider instance with the given address.
//
// The result might contain unknown values, but those should still typically
// be sent to the provider so that it can decide how to deal with them. Some
// providers just immediately fail in that case, but others are able to work
// in a partially-configured mode where some resource types are plannable while
// others need to be deferred to a later plan/apply round.
//
// If the requested provider instance does not exist in the configuration at
// all then this will return [cty.NilVal]. That should not occur for any
// provider instance address reported by this package as part of the same
// planning phase, but could occur in subsequent work done by the planning
// phase to deal with resource instances that are in prior state but no longer
// in desired state, if their provider instances have also been removed from
// the desired state at the same time.
func (o *PlanningOracle) ProviderInstanceConfig(ctx context.Context, addr addrs.AbsProviderInstanceCorrect) cty.Value {
	// TODO: Implement this by asking o.rootModuleInstance to provide it.
	// (There isn't currently any API for that.)
	return cty.NilVal
}

// ProviderInstanceUsers returns an object representing which resource instances
// are associated with the provider instance that has the given address.
//
// The planning phase must keep the provider open at least long enough for
// all of the reported resource instances to be planned.
//
// Note that the planning engine will need to plan destruction of any resource
// instances that aren't in the desired state once
// [ConfigInstance.DrivePlanning] returns, and provider instances involved in
// those followup steps will need to remain open until that other work is
// done. This package is not concerned with those details; that's the planning
// engine's responsibility.
func (o *PlanningOracle) ProviderInstanceUsers(ctx context.Context, addr addrs.AbsProviderInstanceCorrect) ProviderInstanceUsers {
	return o.relationships.ProviderInstanceUsers.Get(addr)
}

// EphemeralResourceInstanceUsers returns an object describing which other
// resource instances and providers rely on the result value of the ephemeral
// resource with the given address.
//
// The planning phase must keep the ephemeral resource instance open at least
// long enough for all of the reported resource instances to be planned and
// for all of the reported provider instances to be closed.
//
// The given address must be for an ephemeral resource instance or this function
// will panic.
//
// Note that the planning engine will need to plan destruction of any resource
// instances that aren't in the desired state once
// [ConfigInstance.DrivePlanning] returns, and provider instances involved in
// those followup steps might be included in a result from this method, in
// which case the planning phase must hold the provider instance open long
// enough to complete those followup steps.
func (o *PlanningOracle) EphemeralResourceInstanceUsers(ctx context.Context, addr addrs.AbsResourceInstance) EphemeralResourceInstanceUsers {
	if addr.Resource.Resource.Mode != addrs.EphemeralResourceMode {
		panic(fmt.Sprintf("EphemeralResourceInstanceUsers with non-ephemeral %s", addr))
	}
	return o.relationships.EphemeralResourceUsers.Get(addr)
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
