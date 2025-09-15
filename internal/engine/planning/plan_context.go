package planning

import (
	"context"
	"fmt"
	"iter"
	"log"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/collections"
	"github.com/opentofu/opentofu/internal/engine/lifecycle"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// planContext is our shared state for the various parts of a single call
// to [PlanChanges], and also serves as our [eval.PlanGlue] implementation
// through which the evaluator calls us to ask for planning results.
type planContext struct {
	evalCtx        *eval.EvalContext
	plannedChanges *plans.ChangesSync

	// TODO: The following should probably track a reason why each resource
	// instance was deferred, but since deferral is not the focus of this
	// current experiment we'll just keep this boolean for now.
	deferred addrs.Map[addrs.AbsResourceInstance, struct{}]

	// prevRoundState MUST be treated as immutable
	prevRoundState *states.State

	// refreshedState is where we record the results of refreshing
	// resource instances as we visit them. This starts as a deep copy
	// of prevRoundState.
	refreshedState *states.SyncState

	completion *completionTracker

	providerInstances *providerInstances

	// TODO: something to track which ephemeral resource instances are currently
	// open? (Do we actually need that, or can we just rely on a background
	// goroutine to babysit those based on the completion tracker?)
}

var _ eval.PlanGlue = (*planContext)(nil)

func newPlanContext(evalCtx *eval.EvalContext, prevRoundState *states.State) *planContext {
	if prevRoundState == nil {
		prevRoundState = states.NewState()
	}
	changes := plans.NewChanges()
	refreshedState := prevRoundState.DeepCopy()

	completion := lifecycle.NewCompletionTracker[completionEvent]()

	return &planContext{
		evalCtx:           evalCtx,
		plannedChanges:    changes.SyncWrapper(),
		prevRoundState:    prevRoundState,
		refreshedState:    refreshedState.SyncWrapper(),
		completion:        completion,
		providerInstances: newProviderInstances(completion),
	}
}

// PlanDesiredResourceInstance implements eval.PlanGlue.
//
// This is called each time the evaluation system discovers a new resource
// instance in the configuration, and there are likely to be multiple calls
// active concurrently and so this function must take care to avoid races.
func (p *planContext) PlanDesiredResourceInstance(ctx context.Context, inst *eval.DesiredResourceInstance, oracle *eval.PlanningOracle) (cty.Value, tfdiags.Diagnostics) {
	log.Printf("[TRACE] planContext: planning desired resource instance %s", inst.Addr)
	// The details of how we plan vary considerably depending on the resource
	// mode, so we'll dispatch each one to a separate function before we do
	// anything else.
	switch mode := inst.Addr.Resource.Resource.Mode; mode {
	case addrs.ManagedResourceMode:
		return p.planDesiredManagedResourceInstance(ctx, inst, oracle)
	case addrs.DataResourceMode:
		return p.planDesiredDataResourceInstance(ctx, inst, oracle)
	case addrs.EphemeralResourceMode:
		return p.planDesiredEphemeralResourceInstance(ctx, inst, oracle)
	default:
		// We should not get here because the cases above should always be
		// exhaustive for all of the valid resource modes.
		var diags tfdiags.Diagnostics
		diags = diags.Append(fmt.Errorf("the planning engine does not support %s; this is a bug in OpenTofu", mode))
		return cty.DynamicVal, diags
	}
}

// PlanModuleCallInstanceOrphans implements eval.PlanGlue.
func (p *planContext) PlanModuleCallInstanceOrphans(ctx context.Context, moduleCallAddr addrs.AbsModuleCall, desiredInstances iter.Seq[addrs.InstanceKey], oracle *eval.PlanningOracle) tfdiags.Diagnostics {
	if moduleCallAddr.Module.IsPlaceholder() {
		// can't predict anything about what might be desired or orphaned
		// under this module instance.
		return nil
	}
	desiredSet := collections.CollectSet(desiredInstances)
	for key := range desiredSet {
		if _, ok := key.(addrs.WildcardKey); ok {
			// can't predict what instances are desired for this module call
			return nil
		}
	}

	orphaned := resourceInstancesFilter(p.prevRoundState, func(instAddr addrs.AbsResourceInstance) bool {
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
			p.planOrphanResourceInstance(ctx, addr, state, oracle),
		)
	}
	return diags
}

// PlanModuleCallOrphans implements eval.PlanGlue.
func (p *planContext) PlanModuleCallOrphans(ctx context.Context, callerModuleInstAddr addrs.ModuleInstance, desiredCalls iter.Seq[addrs.ModuleCall], oracle *eval.PlanningOracle) tfdiags.Diagnostics {
	if callerModuleInstAddr.IsPlaceholder() {
		// can't predict anything about what might be desired or orphaned
		// under this module instance.
		return nil
	}
	desiredSet := addrs.CollectSet(desiredCalls)

	orphaned := resourceInstancesFilter(p.prevRoundState, func(instAddr addrs.AbsResourceInstance) bool {
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
			p.planOrphanResourceInstance(ctx, addr, state, oracle),
		)
	}
	return diags
}

// PlanResourceInstanceOrphans implements eval.PlanGlue.
func (p *planContext) PlanResourceInstanceOrphans(ctx context.Context, resourceAddr addrs.AbsResource, desiredInstances iter.Seq[addrs.InstanceKey], oracle *eval.PlanningOracle) tfdiags.Diagnostics {
	if resourceAddr.IsPlaceholder() {
		// can't predict anything about what might be desired or orphaned
		// under this resource.
		return nil
	}
	desiredSet := collections.CollectSet(desiredInstances)
	for key := range desiredSet {
		if _, ok := key.(addrs.WildcardKey); ok {
			// can't predict what instances are desired for this resource
			return nil
		}
	}

	orphaned := resourceInstancesFilter(p.prevRoundState, func(instAddr addrs.AbsResourceInstance) bool {
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
			p.planOrphanResourceInstance(ctx, addr, state, oracle),
		)
	}
	return diags
}

// PlanResourceOrphans implements eval.PlanGlue.
func (p *planContext) PlanResourceOrphans(ctx context.Context, moduleInstAddr addrs.ModuleInstance, desiredResources iter.Seq[addrs.Resource], oracle *eval.PlanningOracle) tfdiags.Diagnostics {
	if moduleInstAddr.IsPlaceholder() {
		// can't predict anything about what might be desired or orphaned
		// under this resource instance.
		return nil
	}
	desiredSet := addrs.CollectSet(desiredResources)

	orphaned := resourceInstancesFilter(p.prevRoundState, func(addr addrs.AbsResourceInstance) bool {
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
			p.planOrphanResourceInstance(ctx, addr, state, oracle),
		)
	}
	return diags
}

func (p *planContext) planOrphanResourceInstance(ctx context.Context, addr addrs.AbsResourceInstance, state *states.ResourceInstance, oracle *eval.PlanningOracle) tfdiags.Diagnostics {
	log.Printf("[TRACE] planContext: planning orphan resource instance %s", addr)
	switch mode := addr.Resource.Resource.Mode; mode {
	case addrs.ManagedResourceMode:
		return p.planOrphanManagedResourceInstance(ctx, addr, state, oracle)
	case addrs.DataResourceMode:
		return p.planOrphanDataResourceInstance(ctx, addr, state, oracle)
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

// Close marks the end of the use of the [planContext] object, returning a
// [plans.Plan] representation of the plan that was created.
//
// After calling this function the [planContext] object is invalid and must
// not be used anymore.
func (p *planContext) Close() *plans.Plan {
	// Before we return we'll make sure our completion tracker isn't waiting
	// for anything else to complete, so that we can unblock closing of
	// any provider instances or ephemeral resource instances that might've
	// got left behind by panics/etc. We should not be relying on this in the
	// happy path.
	for event := range p.completion.PendingItems() {
		log.Printf("[TRACE] planContext: synthetic completion of %#v", event)
		p.completion.ReportCompletion(event)
	}

	return &plans.Plan{
		UIMode:       plans.NormalMode, // TODO: This PlanChanges function needs something analogous to [tofu.PlanOpts] for planning mode/options
		Changes:      p.plannedChanges.Close(),
		PrevRunState: p.prevRoundState,
		PriorState:   p.refreshedState.Close(),
		// TODO: various other fields that we need to actually make use
		// of this plan result. But this is intentionally just a partial
		// result for now because it's not clear that we'd even be using
		// plans.Plan in a final version of this new approach.
	}
}

// resourceInstancesFilter returns a sequence of resource instances from the
// given state whose addresses caused the "want" function to return true.
//
// This is an inefficient way to implement detection of "orphans" with our
// current state model. If we decide to adopt a design like this then we
// should adopt a different representation of state which uses a tree structure
// where we can efficiently scan over subtrees that match a particular prefix,
// rather than always scanning over everything.
func resourceInstancesFilter(state *states.State, want func(addrs.AbsResourceInstance) bool) iter.Seq2[addrs.AbsResourceInstance, *states.ResourceInstance] {
	return func(yield func(addrs.AbsResourceInstance, *states.ResourceInstance) bool) {
		for _, modState := range state.Modules {
			for _, resourceState := range modState.Resources {
				for instKey, instanceState := range resourceState.Instances {
					instAddr := resourceState.Addr.Instance(instKey)
					if !want(instAddr) {
						continue
					}
					if !yield(instAddr, instanceState) {
						return
					}
				}
			}
		}
	}
}
