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
	"runtime/pprof"
	"strings"
	"sync"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/internal/exec"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/refactoring"
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
	planCtx  *planContext
	oracle   *eval.PlanningOracle
	targets  addrs.Set[addrs.Targetable]
	excludes addrs.Set[addrs.Targetable]

	allResourcesDeferred bool

	// rioMetaConfigOnce is used to coalesce and memoize requests to fetch
	// the configuration-sourced subset of the metadata for a resource instance
	// object, as an implementation detail of both
	// [planGlue.resourceInstanceObjectMeta] and
	// [planGlue.resourceInstanceObjectMoveStatements] which both rely on this
	// information in slightly different ways. We don't also memoize the
	// state-based portion of the metadata because the construction of that is
	// trivial based on information already in the previous run state.
	rioMetaConfigOnce addrs.Map[addrs.AbsResourceInstanceObject, *grapheval.Once[[]eval.ModuleConfiguredResourceInstanceObjectMeta]]
	rioMetaConfigMu   sync.Mutex // Used only for fast map modifications, and not held over blocking evaluation work
}

var _ eval.PlanGlue = (*planGlue)(nil)

func (p *planGlue) isTargeting() bool {
	return len(p.targets) != 0
}
func (p *planGlue) isTargeted(addr addrs.Targetable) bool {
	return !p.isTargeting() || p.targets.HasFunc(func(targeter addrs.Targetable) bool { return targeter.TargetContains(addr) })
}

func (p *planGlue) isExcluding() bool {
	return len(p.excludes) != 0
}
func (p *planGlue) isExcluded(addr addrs.Targetable) bool {
	return p.excludes.HasFunc(func(excluder addrs.Targetable) bool { return excluder.TargetContains(addr) })
}

func (p *planGlue) CheckTargets(ctx context.Context) tfdiags.Diagnostics {
	if !p.isTargeting() {
		// Nop
		return nil
	}

	allStateResources := p.planCtx.prevRoundState.AllResourceInstanceObjectAddrs()

	// This is not concurrency safe, though each step within is safe
	for _, target := range p.targets {
		log.Printf("[TRACE] Processing target %s", target)
		// Force config compilation and evaluation of targeted resources
		p.oracle.CheckTarget(ctx, target)

		// Also locate applicable state entries and check for moves
		// This ensures that orphaned resources that are targeted and moved
		// hit the appropriate error conditions.
		for _, entry := range allStateResources {
			if p.planCtx.recordedMoves.Has(entry.Instance) {
				// Already processed
				continue
			}
			if target.TargetContains(entry.Instance) {
				// Check for applicable move
				movedToConfigAddr, _ := p.locateConfigForState(ctx, entry.Instance, true)
				if movedToConfigAddr != nil {
					log.Printf("[TRACE] Processing additional target from state %s", *movedToConfigAddr)
					p.oracle.CheckTarget(ctx, *movedToConfigAddr)
				}
			}
		}
	}
	log.Printf("[TRACE] Completed targeting")

	p.allResourcesDeferred = true

	return nil
}

// PlanDesiredResourceInstance implements eval.PlanGlue.
//
// This is called each time the evaluation system discovers a new resource
// instance in the configuration, and there are likely to be multiple calls
// active concurrently and so this function must take care to avoid races.
func (p *planGlue) PlanDesiredResourceInstance(ctx context.Context, inst *eval.DesiredResourceInstance) (cty.Value, tfdiags.Diagnostics) {
	log.Printf("[TRACE] planContext: planning desired resource instance %s", inst.Addr)

	p.planCtx.desiredMu.Lock()
	p.planCtx.desired.Add(inst.Addr)
	p.planCtx.desiredMu.Unlock()

	// The details of how we plan vary considerably depending on the resource
	// mode, so we'll dispatch each one to a separate function after we've
	// dealt with some common preparation work.
	var obj *resourceInstanceObject
	var diags tfdiags.Diagnostics
	switch mode := inst.Addr.Resource.Resource.Mode; mode {
	case addrs.ManagedResourceMode:
		obj, diags = p.planDesiredManagedResourceInstance(ctx, inst)
	case addrs.DataResourceMode:
		obj, diags = p.planDesiredDataResourceInstance(ctx, inst)
	case addrs.EphemeralResourceMode:
		// Ephemerals are not part of the resource graph
		panic("unreachable")
	default:
		// We should not get here because the cases above should always be
		// exhaustive for all of the valid resource modes.
		diags = diags.Append(fmt.Errorf("the planning engine does not support %s; this is a bug in OpenTofu", mode))
		return cty.DynamicVal, diags
	}
	rv := obj.ResultValue()
	if !isDeferredVal(rv) {
		p.planCtx.resourceInstObjs.Put(obj)
	}
	return rv, diags
}

func (p *planGlue) Finalize(ctx context.Context) (*planContextResult, tfdiags.Diagnostics) {
	ctx = grapheval.ContextWithNewWorker(ctx)
	var diags tfdiags.Diagnostics

	p.planCtx.desiredMu.Lock()
	defer p.planCtx.desiredMu.Unlock()

	// We also need to deal with any "deposed" resource instances that were
	// in the previous round state. We do this separately afterwards because
	// these have no direct representation in the configuration at all and
	// so are not in scope for the config eval system. It's also relatively
	// rare for a previous round state to include deposed instances, since it
	// can happen only if the "delete" leg of a create-before-destroy replace
	// failed in the previous round.
	//
	// The provider instance manager should've planned ahead and arranged for
	// any providers we need for these to still be open, waiting for the
	// completion reports generated by our planning calls in this loop.
	//
	// After we complete this work, planCtx.resourceInstObjs is expanded to
	// also include any deposed resource instance objects we discovered.
	// TODO does this need a target filter???
	for objAddr, objState := range resourceInstancesObjects(p.planCtx.prevRoundState) {
		if objAddr.IsDeposed() {
			diags = diags.Append(
				p.planDeposedResourceInstanceObject(ctx, objAddr, objState),
			)
		} else if !p.planCtx.desired.HasFunc(func(desired addrs.AbsResourceInstance) bool {
			if desired.IsPlaceholder() {
				return desired.PlaceholderContains(objAddr.InstanceAddr)
			}
			return desired.Equal(objAddr.InstanceAddr)
		}) {
			diags = diags.Append(
				p.planOrphanResourceInstance(ctx, objAddr.InstanceAddr, objState),
			)
		}
	}

	// We also need to check for invalid moves
	diags = diags.Append(p.validateMoves(ctx))

	diags = diags.Append(p.validateForceReplace())

	diags = diags.Append(p.planCtx.CheckPreventDestroy(ctx, p.oracle))

	diags = diags.Append(p.oracle.Close(ctx))

	intermediate, moreDiags := p.planCtx.Close(ctx)
	diags = diags.Append(moreDiags)

	return intermediate, diags
}

func (p *planGlue) planOrphanResourceInstance(ctx context.Context, addr addrs.AbsResourceInstance, state *states.ResourceInstanceObjectFullSrc) tfdiags.Diagnostics {
	log.Printf("[TRACE] planContext: planning orphan resource instance %s", addr)

	if p.isTargeting() {
		// NOTE: this is broken if A -> B, both are orphaned, B is targeted. A will not be targeted and have a broken dependency.
		// This *MATCHES* the existing strangeness of the original engine.
		// Given that fixing it will be non-trivial, we are deferring this until this engine is adopted and stable.
		if !p.isTargeted(addr) {
			log.Printf("[TRACE] planContext: resource instance %s not targeted", addr)

			return nil
		}
	}
	if p.isExcluding() {
		// NOTE this is broken if A -> B, both are orphaned, A is excluded. A will have a broken dependency.
		// This *MATCHES* the existing strangeness of the original engine.
		// Given that fixing it will be non-trivial, we are deferring this until this engine is adopted and stable.
		if p.isExcluded(addr) {
			log.Printf("[TRACE] planContext: resource instance %s excluded", addr)
			return nil
		}
	}

	var obj *resourceInstanceObject
	var diags tfdiags.Diagnostics
	switch mode := addr.Resource.Resource.Mode; mode {
	case addrs.ManagedResourceMode:
		obj, diags = p.planOrphanManagedResourceInstance(ctx, addr, state)
	case addrs.DataResourceMode:
		obj, diags = p.planOrphanDataResourceInstance(ctx, addr, state)
	case addrs.EphemeralResourceMode:
		// It should not be possible for an ephemeral resource to be an
		// orphan because ephemeral resources should never be persisted
		// in a state snapshot.
		diags = diags.Append(fmt.Errorf("unexpected ephemeral resource instance %s in prior state; this is a bug in OpenTofu", addr))
		return diags
	default:
		// We should not get here because the cases above should always be
		// exhaustive for all of the valid resource modes.
		diags = diags.Append(fmt.Errorf("the planning engine does not support %s; this is a bug in OpenTofu", mode))
		return diags
	}
	p.planCtx.resourceInstObjs.Put(obj)
	return diags
}

func (p *planGlue) planDeposedResourceInstanceObject(ctx context.Context, addr addrs.AbsResourceInstanceObject, state *states.ResourceInstanceObjectFullSrc) tfdiags.Diagnostics {
	log.Printf("[TRACE] planContext: planning deposed resource instance object %s", addr)
	if addr.InstanceAddr.Resource.Resource.Mode != addrs.ManagedResourceMode {
		// Should not be possible because only managed resource instances
		// support "replace" and so nothing else can have deposed objects.
		var diags tfdiags.Diagnostics
		diags = diags.Append(fmt.Errorf("deposed object for non-managed resource instance %s; this is a bug in OpenTofu", addr))
		return diags
	}
	obj, diags := p.planDeposedManagedResourceInstanceObject(ctx, addr, state)
	p.planCtx.resourceInstObjs.Put(obj)
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
	return p.oracle.ProviderInstance(ctx, addr)
}

func (p *planGlue) desiredResourceInstanceMustBeDeferred(inst *eval.DesiredResourceInstance, meta *exec.ResourceInstanceObjectMeta) bool {
	// Set during targeting after initial targets have been resolved
	if p.allResourcesDeferred {
		log.Printf("[TRACE] Deferring untargeted %s", inst.Addr)
		return true
	}

	// If the resource is excluded, handle it as such
	if p.isExcluded(inst.Addr) {
		log.Printf("[TRACE] Deferring excluded %s", inst.Addr)
		return true
	}

	// There are various reasons why we might need to defer final planning
	// of this to a later round. The following is not exhaustive but is a
	// placeholder to show where deferral might fit in.
	return inst.IsPlaceholder() || !meta.ProviderInstance.IsKnown() || derivedFromDeferredVal(inst.ConfigVal) || exprs.IsEvalError(inst.ConfigVal)
}

// resourceInstancesObjects returns a sequence of resource instances from the
// given state.
func resourceInstancesObjects(state *states.State) iter.Seq2[addrs.AbsResourceInstanceObject, *states.ResourceInstanceObjectFullSrc] {
	return func(yield func(addrs.AbsResourceInstanceObject, *states.ResourceInstanceObjectFullSrc) bool) {
		// We currently have a schism where we do all of the
		// discovery work using the traditional state model but
		// we then switch to using our new-style "full" object model
		// to act on what we've discovered. This is hopefully just
		// a temporary situation while we're operating in a mixed
		// world where most of the system doesn't know about the
		// new runtime yet.
		stateSync := state.SyncWrapper()
		yieldAddr := func(addr addrs.AbsResourceInstanceObject) bool {
			objState := stateSync.ResourceInstanceObjectFull(addr)
			if objState == nil {
				// If we get here then there's a bug in the
				// ResourceInstanceObjectFull function, because we
				// should only be here if instAddr corresponds to a
				// to an instance with a current object.
				panic(fmt.Sprintf("state has %s, but ResourceInstanceObjectFull didn't return it", addr))
			}
			return yield(addr, objState)
		}

		for _, modState := range state.Modules {
			for _, resourceState := range modState.Resources {
				for instKey, instanceState := range resourceState.Instances {
					instAddr := resourceState.Addr.Instance(instKey)
					if instanceState.HasCurrent() {
						if !yieldAddr(instAddr.CurrentObject()) {
							continue
						}
					}
					for deposedKey := range instanceState.Deposed {
						if !yieldAddr(instAddr.Object(deposedKey)) {
							continue
						}
					}
				}
			}
		}
	}
}

func (p *planGlue) evaluateReplaceTriggeredBy(ref eval.ResourceInstanceAttributePath) (*addrs.AbsResourceInstance, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	var changes []*plans.ResourceInstanceChange

	instance, ok := p.planCtx.resourceInstObjs.Get(ref.ResourceInstance.CurrentObject())
	if !ok {
		// This should not happen!
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  `Reference to undeclared resource`,
			Detail:   fmt.Sprintf(`A resource %s has not been declared`, ref.ResourceInstance),
		})
		return nil, diags
	}
	currentChange := instance.PlannedChange
	if currentChange != nil {
		changes = append(changes, currentChange)
	}

	if len(changes) == 0 {
		return nil, diags
	}

	// If we don't have a traversal beyond the resource, then we can just look
	// for any change.
	if len(ref.Path) == 0 {
		for _, c := range changes {
			if c.Action.CanTriggerDownstreamReplace() {
				return &ref.ResourceInstance, diags
			}
		}

		// no change triggered
		return nil, diags
	}

	// This must be an instances to have a remaining traversal, which means a
	// single change.
	change := changes[0]

	// Make sure the change is actionable. A Delete action will have a change
	// in value, but is not valid for our purposes here.
	if !change.Action.CanTriggerDownstreamReplace() {
		return nil, diags
	}

	attrBefore, _ := ref.Path.Apply(change.Before)
	attrAfter, _ := ref.Path.Apply(change.After)

	replace := false
	if attrBefore == cty.NilVal || attrAfter == cty.NilVal {
		replace = attrBefore != attrAfter
	} else {
		replace = !attrBefore.RawEquals(attrAfter)
	}

	if replace {
		return &ref.ResourceInstance, diags
	}
	return nil, diags
}

func (p *planGlue) validateForceReplace() tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	// TODO consider extending this logic to match key types.
	// This only handles one very specific class of errors and should probably be improved at some point.
	desiredResources := addrs.MakeMap[addrs.AbsResource, []addrs.AbsResourceInstance]()
	for _, desired := range p.planCtx.desired {
		key := desired.ContainingResource()
		desiredResources.Put(key, append(desiredResources.Get(key), desired))
	}

	forcedResources := addrs.MakeSet[addrs.AbsResource]()
	for _, forced := range p.planCtx.forceReplace {
		// Matches without instance key
		if forced.Resource.Key == addrs.NoKey {
			forcedResources.Add(forced.ContainingResource())
		}
	}

	for _, resourceAddr := range desiredResources.Keys() {
		if !forcedResources.Has(resourceAddr) {
			continue
		}

		instanceAddrs := desiredResources.Get(resourceAddr)

		replaceNeedsKey := false
		isPlaceholder := false
		for _, addr := range instanceAddrs {
			if addr.Resource.IsPlaceholder() {
				// can't predict what instances are desired for this resource
				isPlaceholder = true
				break
			}
			if addr.Resource.Key != addrs.NoKey {
				replaceNeedsKey = true
				break
			}
		}

		if !replaceNeedsKey || isPlaceholder {
			continue
		}

		// Copied from nodeExpandPlannableResource.expandResourceInstances
		switch {
		case len(instanceAddrs) == 0:
			// In this case there _are_ no instances to replace, so
			// there isn't any alternative address for us to suggest.
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Warning,
				"Incompletely-matched force-replace resource instance",
				fmt.Sprintf(
					"Your force-replace request for %s doesn't match any resource instances because this resource doesn't have any instances.",
					resourceAddr,
				),
			))
		case len(instanceAddrs) == 1:
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Warning,
				"Incompletely-matched force-replace resource instance",
				fmt.Sprintf(
					"Your force-replace request for %s doesn't match any resource instances because it lacks an instance key.\n\nTo force replacement of the single declared instance, use the following option instead:\n  -replace=%q",
					resourceAddr, instanceAddrs[0],
				),
			))
		default:
			var possibleValidOptions strings.Builder
			for _, addr := range instanceAddrs {
				fmt.Fprintf(&possibleValidOptions, "\n  -replace=%q", addr)
			}

			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Warning,
				"Incompletely-matched force-replace resource instance",
				fmt.Sprintf(
					"Your force-replace request for %s doesn't match any resource instances because it lacks an instance key.\n\nTo force replacement of particular instances, use one or more of the following options instead:%s",
					resourceAddr, possibleValidOptions.String(),
				),
			))
		}
	}

	return diags
}

// ResourceInstanceObjectMeta gathers and assembles the metadata for a resource
// instance object based on both its previous run and current desired addresses,
// using a mixture of configuration-based and previous-run-state-based
// information.
//
// This function is appropriate to use only any "moves" have been resolved,
// so the caller can specify a different previous run address and desired
// address describing that move. In the common case where an object is not
// moving between addresses, callers should pass the same address in both
// arguments. For codepaths that are collecting data to discover which "moves"
// are actually required, use [planGlue.ResourceInstanceMovedStatements]
// instead.
//
// Use this method only for objects that are either desired, have previous round
// state, or both. It will panic if both prevRunAddr does not exist in the
// previous round state and desiredAddr is not declared as desired in the
// current configuration.
//
// Fetching resource instance metadata is likely to cause evaluation of
// expressions in the configuration. In particular, if the requested resource
// instance address is in a non-root module then this process forces deciding
// which instances are declared for each module call along the path, and then
// various expressions related to the requested resource will be evalauted
// in each of the relevant module instances.
func (p *planGlue) ResourceInstanceObjectMeta(ctx context.Context, prevRunAddr, desiredAddr addrs.AbsResourceInstanceObject) *exec.ResourceInstanceObjectMeta {
	// We currently have a schism where we do all of the
	// discovery work using the traditional state model but
	// we then switch to using our new-style "full" object model
	// to act on what we've discovered. This is hopefully just
	// a temporary situation while we're operating in a mixed
	// world where most of the system doesn't know about the
	// new runtime yet.
	stateSync := p.planCtx.prevRoundState.SyncWrapper()

	configMeta := p.configuredResourceInstanceObjectMeta(ctx, desiredAddr)
	prevRoundState := stateSync.ResourceInstanceObjectFull(prevRunAddr)
	return exec.BuildResourceInstanceObjectMeta(desiredAddr, configMeta, prevRoundState)
}

// ResourceInstanceMoveStatements is a specialized variant of
// [planGlue.ResourceInstanceObjectMeta] intended only for the work of
// resolving which objects need to move between addresses in the current
// round.
//
// This only returns moved statements that have the given resource instance
// address directly as one of their endpoints. It's a caller's responsibility
// to chase those references and make further calls to this function for
// the resource instance at the opposing endpoint of each statement.
//
// Move statements identify their endpoints relative to the module instance
// where the statement was declared, and so each result is returned with the
// address of the module instance that declared it so the caller can determine
// the corresponding absolute address using
// [addrs.MoveEndpointInModule.InModuleInstance].
//
// Calling this has similar expression evaluation consequences as documented for
// ResourceInstanceObjectMeta, but any work done for a current resource instance
// by one of these two functions is coalesced with that same work in the other
// so each is resolved at most once regardless of which function is called
// first.
func (p *planGlue) ResourceInstanceObjectMoveStatements(ctx context.Context, addr addrs.AbsResourceInstance) iter.Seq2[addrs.ModuleInstance, *refactoring.MoveStatement] {
	// Unlike all of the other uses of metadata, we rely exclusively on the
	// configuration to discover the effective graph of moved statements. This
	// is because we need to resolve where an object is moving from before
	// we can know which address it would've had in the previous run state.

	// We do the potentially-blocking work here _before_ returning and use
	// the iter.Seq only to do the trivial work to flatten the sequence of
	// statements, so callers don't need to consider the workgraph
	// rules when working with this function's result.
	configMeta := p.configuredResourceInstanceObjectMeta(ctx, addr.CurrentObject())
	return func(yield func(addrs.ModuleInstance, *refactoring.MoveStatement) bool) {
		for _, moduleInstMeta := range configMeta {
			for _, stmt := range moduleInstMeta.MoveStatements {
				if !yield(moduleInstMeta.ModuleInstance, stmt) {
					return
				}
			}
		}
	}
}

// configuredResourceInstanceObjectMeta is the internal config-loading
// implementation shared by both [planGlue.ResourceInstanceObjectMeta] and
// [planGlue.ResourceInstanceMoveStatements]. Use one of those two functions
// instead of calling this directly.
func (p *planGlue) configuredResourceInstanceObjectMeta(ctx context.Context, addr addrs.AbsResourceInstanceObject) []eval.ModuleConfiguredResourceInstanceObjectMeta {
	ctx = pprof.WithLabels(ctx, pprof.Labels("RIOMeta", addr.String()))

	// We hold p.rioMetaConfigMu _only_ during the fast map manipulation here,
	// and then rely on grapheval.Once to manage the main blocking work so
	// that the main work for two distinct objects can run concurrently.
	p.rioMetaConfigMu.Lock()
	if p.rioMetaConfigOnce.Len() == 0 {
		p.rioMetaConfigOnce = addrs.MakeMap[addrs.AbsResourceInstanceObject, *grapheval.Once[[]eval.ModuleConfiguredResourceInstanceObjectMeta]]()
	}
	once, ok := p.rioMetaConfigOnce.GetOk(addr)
	if !ok {
		once = &grapheval.Once[[]eval.ModuleConfiguredResourceInstanceObjectMeta]{}
		p.rioMetaConfigOnce.Put(addr, once)
	}
	p.rioMetaConfigMu.Unlock()

	// FIXME: The workgraph requests embedded in the grapheval.Once objects
	// we're creating here aren't currently visible to the
	// [grapheval.RequestTracker] that's active during planning, because that
	// lives inside the eval package instead of here in the planning engine.
	// This situation already arose for the applying engine which uses
	// workgraph directly to manage execution graph processing, and so it
	// seems like the appropriate design is for each engine to be in charge
	// of its own [grapheval.RequestTracker] implementation and delegate to
	// helpers in eval as necessary, so that they can both report their own
	// requests _and_ the evaluator's requests together when needed.
	//
	// Once that's done we should have the planning engine's request tracker
	// report a name like "metadata for <object-address>" for each element
	// of p.rioMetaConfigOnce and then that'll show up in the chain of
	// requests when we report a self-dependency problem to the user.

	// We intentionally discard the diagnostics here because we assume that
	// any problems this could return would also be reported through other parts
	// of the "CheckAll" process that drives the planning engine forward.
	oracle := p.oracle
	ret, _ := once.Do(ctx, func(ctx context.Context) ([]eval.ModuleConfiguredResourceInstanceObjectMeta, tfdiags.Diagnostics) {
		return oracle.ResourceInstanceObjectMeta(ctx, addr), nil
	})
	return ret
}
