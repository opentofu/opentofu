// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"context"
	"fmt"
	"iter"
	"log"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/collections"
	"github.com/opentofu/opentofu/internal/engine/internal/execgraph"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/plans/objchange"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// planGlue is our implementation of [eval.PlanGlue], which the evaluation
// system uses to help the planning engine drive the planning process forward
// as it learns information from the configuration.
//
// The methods of this type can all be called concurrently with themselves and
// each other, so they must use appropriate synchronization to avoid races.
type planGlue struct {
	planCtx *planContext
	oracle  *eval.PlanningOracle
}

var _ eval.PlanGlue = (*planGlue)(nil)

// I'm not sure that this belongs here
func (p *planGlue) ValidateProviderConfig(ctx context.Context, provider addrs.Provider, configVal cty.Value) tfdiags.Diagnostics {
	return p.planCtx.providers.ValidateProviderConfig(ctx, provider, configVal)
}

// PlanDesiredResourceInstance implements eval.PlanGlue.
//
// This is called each time the evaluation system discovers a new resource
// instance in the configuration, and there are likely to be multiple calls
// active concurrently and so this function must take care to avoid races.
func (p *planGlue) PlanDesiredResourceInstance(ctx context.Context, inst *eval.DesiredResourceInstance) (cty.Value, tfdiags.Diagnostics) {
	log.Printf("[TRACE] planContext: planning desired resource instance %s", inst.Addr)
	// The details of how we plan vary considerably depending on the resource
	// mode, so we'll dispatch each one to a separate function before we do
	// anything else.
	var plannedVal cty.Value
	var resultRef execgraph.ResourceInstanceResultRef
	var diags tfdiags.Diagnostics
	egb := p.planCtx.execgraphBuilder
	switch mode := inst.Addr.Resource.Resource.Mode; mode {
	case addrs.ManagedResourceMode:
		plannedVal, resultRef, diags = p.planDesiredManagedResourceInstance(ctx, inst, egb)
	case addrs.DataResourceMode:
		plannedVal, resultRef, diags = p.planDesiredDataResourceInstance(ctx, inst, egb)
	case addrs.EphemeralResourceMode:
		plannedVal, resultRef, diags = p.planDesiredEphemeralResourceInstance(ctx, inst, egb)
	default:
		// We should not get here because the cases above should always be
		// exhaustive for all of the valid resource modes.
		var diags tfdiags.Diagnostics
		diags = diags.Append(fmt.Errorf("the planning engine does not support %s; this is a bug in OpenTofu", mode))
		return cty.DynamicVal, diags
	}

	if diags.HasErrors() {
		return cty.DynamicVal, diags
	}
	log.Printf("[TRACE] planContext: final result for %s comes from %#v", inst.Addr, resultRef)
	p.planCtx.execgraphBuilder.SetResourceInstanceFinalStateResult(inst.Addr, resultRef)
	return plannedVal, diags
}

func (p *planGlue) planOrphanResourceInstance(ctx context.Context, addr addrs.AbsResourceInstance, state *states.ResourceInstanceObjectFullSrc) tfdiags.Diagnostics {
	log.Printf("[TRACE] planContext: planning orphan resource instance %s", addr)
	switch mode := addr.Resource.Resource.Mode; mode {
	case addrs.ManagedResourceMode:
		return p.planOrphanManagedResourceInstance(ctx, addr, state, p.planCtx.execgraphBuilder)
	case addrs.DataResourceMode:
		return p.planOrphanDataResourceInstance(ctx, addr, state, p.planCtx.execgraphBuilder)
	case addrs.EphemeralResourceMode:
		// It should not be possible for an ephemeral resource to be an
		// orphan because ephemeral resources should never be persisted
		// in a state snapshot.
		var diags tfdiags.Diagnostics
		diags = diags.Append(fmt.Errorf("unexpected ephemeral resource instance %s in prior state; this is a bug in OpenTofu", addr))
		return diags
	default:
		// We should not get here because the cases above should always be
		// exhaustive for all of the valid resource modes.
		var diags tfdiags.Diagnostics
		diags = diags.Append(fmt.Errorf("the planning engine does not support %s; this is a bug in OpenTofu", mode))
		return diags
	}
}

func (p *planGlue) planDeposedResourceInstanceObject(ctx context.Context, addr addrs.AbsResourceInstance, deposedKey states.DeposedKey, state *states.ResourceInstanceObjectFullSrc) tfdiags.Diagnostics {
	log.Printf("[TRACE] planContext: planning deposed resource instance object %s %s", addr, deposedKey)
	if addr.Resource.Resource.Mode != addrs.ManagedResourceMode {
		// Should not be possible because only managed resource instances
		// support "replace" and so nothing else can have deposed objects.
		var diags tfdiags.Diagnostics
		diags = diags.Append(fmt.Errorf("deposed object for non-managed resource instance %s; this is a bug in OpenTofu", addr))
		return diags
	}
	return p.planDeposedManagedResourceInstanceObject(ctx, addr, deposedKey, state, p.planCtx.execgraphBuilder)
}

// PlanModuleCallInstanceOrphans implements eval.PlanGlue.
func (p *planGlue) PlanModuleCallInstanceOrphans(ctx context.Context, moduleCallAddr addrs.AbsModuleCall, desiredInstances iter.Seq[addrs.InstanceKey]) tfdiags.Diagnostics {
	if moduleCallAddr.Module.IsPlaceholder() {
		// can't predict anything about what might be desired or orphaned
		// under this module instance.
		// FIXME: _Something_ still needs to make sure we call
		// p.planCtx.reportResourceInstancePlanCompletion for any
		// potentially-matching instances in the previous round state, because
		// nothing in the desired state is going to match them and so they
		// won't actually get planned.
		return nil
	}
	desiredSet := collections.CollectSet(desiredInstances)
	for key := range desiredSet {
		if _, ok := key.(addrs.WildcardKey); ok {
			// can't predict what instances are desired for this module call
			return nil
		}
	}

	orphaned := resourceInstancesFilter(p.planCtx.prevRoundState, func(instAddr addrs.AbsResourceInstance) bool {
		// This should return true for any resource instance in the given
		// module instance that belongs to a module call not included in
		// desiredCalls, and false otherwise.
		if instAddr.Module.IsRoot() {
			// A resource in the root module cannot possibly belong to a
			// module call.
			return false
		}
		instCallerModuleInstAddr, instModuleCallInstance := instAddr.Module.CallInstance()
		if !instCallerModuleInstAddr.Equal(moduleCallAddr.Module) {
			return false // not in the relevant calling module instance
		}
		if !instModuleCallInstance.Call.Equal(moduleCallAddr.Call) {
			return false // not in the relevant module call
		}
		if desiredSet.Has(instModuleCallInstance.Key) {
			return false
		}
		return true
	})
	var diags tfdiags.Diagnostics
	for addr, state := range orphaned {
		diags = diags.Append(
			p.planOrphanResourceInstance(ctx, addr, state),
		)
	}
	return diags
}

// PlanModuleCallOrphans implements eval.PlanGlue.
func (p *planGlue) PlanModuleCallOrphans(ctx context.Context, callerModuleInstAddr addrs.ModuleInstance, desiredCalls iter.Seq[addrs.ModuleCall]) tfdiags.Diagnostics {
	if callerModuleInstAddr.IsPlaceholder() {
		// can't predict anything about what might be desired or orphaned
		// under this module instance.
		// FIXME: _Something_ still needs to make sure we call
		// p.planCtx.reportResourceInstancePlanCompletion for any
		// potentially-matching instances in the previous round state, because
		// nothing in the desired state is going to match them and so they
		// won't actually get planned.
		return nil
	}
	desiredSet := addrs.CollectSet(desiredCalls)

	orphaned := resourceInstancesFilter(p.planCtx.prevRoundState, func(instAddr addrs.AbsResourceInstance) bool {
		// This should return true for any resource instance in the given
		// module instance that belongs to a module call not included in
		// desiredCalls, and false otherwise.
		if instAddr.Module.IsRoot() {
			// A resource in the root module cannot possibly belong to a
			// module call.
			return false
		}
		instCallerModuleInstAddr, instModuleCall := instAddr.Module.Call()
		if !instCallerModuleInstAddr.Equal(callerModuleInstAddr) {
			return false // not in the relevant module instance
		}
		if desiredSet.Has(instModuleCall) {
			return false
		}
		return true
	})
	var diags tfdiags.Diagnostics
	for addr, state := range orphaned {
		diags = diags.Append(
			p.planOrphanResourceInstance(ctx, addr, state),
		)
	}
	return diags
}

// PlanResourceInstanceOrphans implements eval.PlanGlue.
func (p *planGlue) PlanResourceInstanceOrphans(ctx context.Context, resourceAddr addrs.AbsResource, desiredInstances iter.Seq[addrs.InstanceKey]) tfdiags.Diagnostics {
	if resourceAddr.IsPlaceholder() {
		// can't predict anything about what might be desired or orphaned
		// under this resource.
		// FIXME: _Something_ still needs to make sure we call
		// p.planCtx.reportResourceInstancePlanCompletion for any
		// potentially-matching instances in the previous round state, because
		// nothing in the desired state is going to match them and so they
		// won't actually get planned.
		return nil
	}
	desiredSet := collections.CollectSet(desiredInstances)
	for key := range desiredSet {
		if _, ok := key.(addrs.WildcardKey); ok {
			// can't predict what instances are desired for this resource
			return nil
		}
	}

	orphaned := resourceInstancesFilter(p.planCtx.prevRoundState, func(instAddr addrs.AbsResourceInstance) bool {
		// This should return true for any resource instance in the given
		// module instance that belongs to a resource not included in
		// desiredResources, and false otherwise.
		if !instAddr.Module.Equal(resourceAddr.Module) {
			return false // not in the relevant module instance
		}
		if !instAddr.Resource.Resource.Equal(resourceAddr.Resource) {
			return false // not in the relevant resource
		}
		if desiredSet.Has(instAddr.Resource.Key) {
			return false
		}
		return true
	})
	var diags tfdiags.Diagnostics
	for addr, state := range orphaned {
		diags = diags.Append(
			p.planOrphanResourceInstance(ctx, addr, state),
		)
	}
	return diags
}

// PlanResourceOrphans implements eval.PlanGlue.
func (p *planGlue) PlanResourceOrphans(ctx context.Context, moduleInstAddr addrs.ModuleInstance, desiredResources iter.Seq[addrs.Resource]) tfdiags.Diagnostics {
	if moduleInstAddr.IsPlaceholder() {
		// can't predict anything about what might be desired or orphaned
		// under this resource instance.
		// FIXME: _Something_ still needs to make sure we call
		// p.planCtx.reportResourceInstancePlanCompletion for any
		// potentially-matching instances in the previous round state, because
		// nothing in the desired state is going to match them and so they
		// won't actually get planned.
		return nil
	}
	desiredSet := addrs.CollectSet(desiredResources)

	orphaned := resourceInstancesFilter(p.planCtx.prevRoundState, func(addr addrs.AbsResourceInstance) bool {
		// This should return true for any resource instance in the given
		// module instance that belongs to a resource not included in
		// desiredResources, and false otherwise.
		if !addr.Module.Equal(moduleInstAddr) {
			return false // not in the relevant module instance
		}
		if desiredSet.Has(addr.Resource.Resource) {
			return false
		}
		return true
	})
	var diags tfdiags.Diagnostics
	for addr, state := range orphaned {
		diags = diags.Append(
			p.planOrphanResourceInstance(ctx, addr, state),
		)
	}
	return diags
}

// ProviderClient returns a client for the requested provider instance, launching
// and configuring the provider first if no caller has previously requested a
// client for this instance.
//
// Returns nil if the configuration for the requested provider instance is too
// invalid to actually configure it. The diagnostics for such a problem would
// be reported by our main [ConfigInstance.DrivePlanning] call but the caller
// of this function will probably want to return a more specialized error saying
// that the corresponding resource cannot be planned because its associated
// provider has an invalid configuration.
func (p *planGlue) providerClient(ctx context.Context, addr addrs.AbsProviderInstanceCorrect) (providers.Configured, tfdiags.Diagnostics) {
	return p.planCtx.providerInstances.ProviderClient(ctx, addr, p)
}

// providerInstanceCompletionEvents returns all of the [completionEvent] values
// that need to have been reported to the completion tracker before an
// instance of the given provider can be closed.
func (p *planGlue) providerInstanceCompletionEvents(ctx context.Context, addr addrs.AbsProviderInstanceCorrect) iter.Seq[completionEvent] {
	return func(yield func(completionEvent) bool) {
		configUsers := p.oracle.ProviderInstanceUsers(ctx, addr)
		for _, resourceInstAddr := range configUsers.ResourceInstances {
			event := resourceInstancePlanningComplete{resourceInstAddr.UniqueKey()}
			if !yield(event) {
				return
			}
		}
		// We also need to wait for the completion of anything we can find
		// in the state, just in case any resource instances are "orphaned"
		// and in case there are any deposed objects we need to deal with.
		for _, modState := range p.planCtx.prevRoundState.Modules {
			for _, resourceState := range modState.Resources {
				for instKey, instanceState := range resourceState.Instances {
					resourceInstAddr := resourceState.Addr.Instance(instKey)
					// FIXME: State is still using the not-quite-right address
					// types for provider instances, so we'll shim here.
					providerInstAddr := resourceState.ProviderConfig.InstanceCorrect(instanceState.ProviderKey)
					if !addr.Equal(providerInstAddr) {
						continue // not for this provider instance
					}
					if instanceState.Current != nil {
						event := resourceInstancePlanningComplete{resourceInstAddr.UniqueKey()}
						if !yield(event) {
							return
						}
					}
					for dk := range instanceState.Deposed {
						event := resourceInstanceDeposedPlanningComplete{resourceInstAddr.UniqueKey(), dk}
						if !yield(event) {
							return
						}
					}
				}
			}
		}
	}
}

func (p *planGlue) desiredResourceInstanceMustBeDeferred(inst *eval.DesiredResourceInstance) bool {
	// There are various reasons why we might need to defer final planning
	// of this to a later round. The following is not exhaustive but is a
	// placeholder to show where deferral might fit in.
	return inst.IsPlaceholder() || inst.ProviderInstance == nil || derivedFromDeferredVal(inst.ConfigVal)
}

func (p *planGlue) resourceInstancePlaceholderValue(ctx context.Context, providerAddr addrs.Provider, resourceMode addrs.ResourceMode, resourceType string, priorVal, configVal cty.Value) cty.Value {
	evalCtx := p.oracle.EvalContext(ctx)
	schema, diags := evalCtx.Providers.ResourceTypeSchema(ctx, providerAddr, resourceMode, resourceType)
	if diags.HasErrors() {
		// If we can't get any schema information then we'll just return
		// a completely-unknown object as our placeholder. We should get here
		// only if the eval system already failed to use the provider to decode
		// or validate the configuration, and so it should already have reported
		// a related error upstream.
		return cty.DynamicVal
	}

	if configVal.IsNull() {
		return cty.NullVal(schema.Block.ImpliedType().WithoutOptionalAttributesDeep())
	}
	if !configVal.IsKnown() {
		return cty.UnknownVal(schema.Block.ImpliedType().WithoutOptionalAttributesDeep())
	}
	return objchange.ProposedNew(
		schema.Block,
		priorVal,
		configVal,
	)
}

// resourceInstancesFilter returns a sequence of resource instances from the
// given state whose addresses caused the "want" function to return true.
//
// This is an inefficient way to implement detection of "orphans" with our
// current state model. If we decide to adopt a design like this then we
// should adopt a different representation of state which uses a tree structure
// where we can efficiently scan over subtrees that match a particular prefix,
// rather than always scanning over everything.
func resourceInstancesFilter(state *states.State, want func(addrs.AbsResourceInstance) bool) iter.Seq2[addrs.AbsResourceInstance, *states.ResourceInstanceObjectFullSrc] {
	return func(yield func(addrs.AbsResourceInstance, *states.ResourceInstanceObjectFullSrc) bool) {
		for _, modState := range state.Modules {
			for _, resourceState := range modState.Resources {
				for instKey, instanceState := range resourceState.Instances {
					if instanceState.Current == nil {
						// Only the current object for a resource instance
						// can be an "orphan". (Deposed objects are handled
						// elsewhere.)
						continue
					}
					instAddr := resourceState.Addr.Instance(instKey)
					if !want(instAddr) {
						continue
					}
					// We currently have a schism where we do all of the
					// discovery work using the traditional state model but
					// we then switch to using our new-style "full" object model
					// to act on what we've discovered. This is hopefully just
					// a temporary situation while we're operating in a mixed
					// world where most of the system doesn't know about the
					// new runtime yet.
					objState := state.SyncWrapper().ResourceInstanceObjectFull(instAddr, states.NotDeposed)
					if objState == nil {
						// If we get here then there's a bug in the
						// ResourceInstanceObjectFull function, because we
						// should only be here if instAddr corresponds to a
						// to an instance with a current object.
						panic(fmt.Sprintf("state has %s, but ResourceInstanceObjectFull didn't return it", instAddr))
					}
					if !yield(instAddr, objState) {
						return
					}
				}
			}
		}
	}
}
