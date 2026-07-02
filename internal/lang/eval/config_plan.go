// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval

import (
	"context"
	"fmt"
	"iter"
	"sync"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/configgraph"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/evalglue"
	"github.com/opentofu/opentofu/internal/lang/exprs"
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
	// prior state) separately as each of the "Plan*Orphans" methods are
	// called to report what exists in the desired state.
	PlanDesiredResourceInstance(ctx context.Context, inst *DesiredResourceInstance) (cty.Value, tfdiags.Diagnostics)

	// PlanResourceInstanceOrphans creates planned actions for any instances
	// of the given resource that existed in the prior state but whose keys
	// are NOT included in desiredInstances.
	//
	// This API assumes that the [PlanGlue] implementation has the prior
	// state represented in a tree structure that allows quickly scanning
	// all instances under a given prefix and testing whether they match
	// any of the given instance keys, after which it will presumably plan
	// a "delete" action for each of them
	//
	// If desiredInstance reports only a single instance key of type
	// [addrs.WildcardKey], or if the module instance address within
	// resourceAddr is a placeholder itself, then the set of desired instances
	// is not actually finalized and so the planning engine would need to
	// defer planning any actions for anything that matches the reported
	// wildcard.
	//
	// Different subsets of prior state resource instances can be covered
	// by different calls to the "Plan*Orphans" family of methods on
	// [PlanGlue]. An implementation of [PlanGlue] should be designed to
	// handle reports at any one of these four levels of granularity, planning
	// actions for whatever subtree of prior state resource instances happen
	// to match the calls. Typically the same objects will be described at
	// different levels of granularity and so the implementation must also
	// keep track of all of the orphan resource instances it has already
	// detected and handled to avoid generating duplicate planned actions.
	PlanResourceInstanceOrphans(ctx context.Context, resourceAddr addrs.AbsResource, desiredInstances iter.Seq[addrs.InstanceKey]) tfdiags.Diagnostics

	// PlanResourceOrphans creates planned actions for any instances of
	// resources in the given module instance that that existed in the prior
	// state but that do NOT appear in desiredResources.
	//
	// This is similar to [PlanGlue.PlanResourceInstanceOrphans] but deals
	// with entirely-removed resources instead of removed instances of a
	// resource that is still configured. The same caveat about wildcard
	// instances applies here too.
	PlanResourceOrphans(ctx context.Context, moduleInstAddr addrs.ModuleInstance, desiredResources iter.Seq[addrs.Resource]) tfdiags.Diagnostics

	// PlanModuleCallInstanceOrphans creates planned actions for any prior
	// state resource instances that belong to instances of the given module
	// call whose instance keys are NOT included in desiredInstances.
	//
	// This is similar to [PlanGlue.PlanResourceOrphans] but deals with
	// the removal of an entire module instance containing resource instances
	// instead of removal of the resources themselves. The same caveat about
	// wildcard instances applies here too.
	PlanModuleCallInstanceOrphans(ctx context.Context, moduleCallAddr addrs.AbsModuleCall, desiredInstances iter.Seq[addrs.InstanceKey]) tfdiags.Diagnostics

	// PlanModuleCallOrphans creates planned actions for any prior state
	// resource instances that belong to any module calls within
	// callerModuleInstAddr that are NOT present in desiredCalls.
	//
	// This is similar to [PlanGlue.PlanModuleCallInstanceOrphans] but deals
	// with the removal of an entire module call containing resource instances,
	// instead of removal of just one dynamic instance of a module call that's
	// still declared.
	PlanModuleCallOrphans(ctx context.Context, callerModuleInstAddr addrs.ModuleInstance, desiredCalls iter.Seq[addrs.ModuleCall]) tfdiags.Diagnostics
}

// DrivePlanning uses this configuration instance to drive forward a planning
// process being executed by another part of the system.
//
// The caller must provide a function that builds a [PlanGlue] implementation
// that should typically somehow incorporate the given [PlanningOracle]. The
// [PlanningOracle] object is not yet valid during the buildGlue function but
// is guaranteed to be valid before any methods are called on the [PlanGlue]
// object that it returns.
//
// This function deals only with the configuration-driven portion of the
// process where the planning engine learns which resource instances are
// currently declared in the configuration. The caller will need to compare
// the set of desired resource instances with the set of resource instances
// tracked in the prior state and then presumably generate additional planned
// actions to destroy any instances that are currently tracked but no longer
// configured.
func (c *ConfigInstance) DrivePlanning(ctx context.Context, buildGlue func(*PlanningOracle) PlanGlue) (*PlanningResult, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// All of our work will be associated with a workgraph worker that serves
	// as the initial worker node in the work graph.
	ctx = grapheval.ContextWithNewWorker(ctx)

	// We have a little chicken vs. egg problem here where we can't fully
	// initialize the oracle until we've built the root module instance,
	// so we initially pass an intentionally-invalid oracle to the build
	// function and then make sure it's valid before we make any use
	// of the PlanGlue object it returns.
	oracle := &PlanningOracle{}
	glue := buildGlue(oracle)

	evalGlue := &planningEvalGlue{
		planEngineGlue: glue,
	}
	rootModuleInstance, moreDiags := c.newRootModuleInstance(ctx, evalGlue)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return nil, diags
	}

	managedProviders := newManagedProviders(c.evalContext.Providers, func(ctx context.Context, addr addrs.AbsProviderInstanceCorrect) (cty.Value, tfdiags.Diagnostics) {
		ctx = grapheval.ContextWithNewWorker(ctx)

		providerInst := evalglue.ProviderInstance(ctx, rootModuleInstance, addr)
		if providerInst == nil {
			// This suggests that the provider instance has an invalid
			// configuration. The main diagnostics for that get returned by
			// another channel but also return an error, so we just return
			// nil to prompt the caller to generate its own error saying that
			// whatever operation the provider was going to be used for cannot
			// be performed.
			//
			// FIXME: This currently doesn't handle the case where there's
			// an orphan or deposed resource instance object in the previous
			// run state referring to a provider instance whose configuration
			// was originally just implied to be empty by the existence of
			// some resource elsewhere in the configuration. Removing all
			// desired resource instances for such an implied provider when
			// there's still at least one object tracked in the state causes
			// us to return nil, here, whereas we ought to somehow attempt
			// to perform the implicit empty configuration behavior in that
			// case too.
			return cty.NilVal, nil
		}
		// We ignore diagnostics here because the CheckAll tree walk should collect
		// them when it visits the provider instance, and so they'll emerge through
		// a different path.
		configVal, _ := providerInst.ConfigValue(ctx)
		return configgraph.PrepareOutgoingValue(configVal), nil
	})

	// We can now initialize the planning oracle, before we start evaluating
	// anything that might cause calls to the evalGlue object.
	oracle.root = rootModuleInstance
	oracle.providers = managedProviders
	// Inject configured providers
	evalGlue.providers = managedProviders

	// The plan phase is driven forward by us evaluating expressions during
	// the "checkAll" process, and so we can just run that here and then
	// it'll cause various calls out to the "glue" object whenever we're
	// ready to provide configuration for a resource intance and need to
	// obtain its result for downstream use.
	//
	// We also concurrently work to call the Plan*Orphans methods on
	// PlanGlue, which does a similar tree walk but is unique only to the
	// planning phase and doesn't directly evaluate any nodes.
	var wg sync.WaitGroup
	var checkDiags tfdiags.Diagnostics
	var orphanDiags tfdiags.Diagnostics
	wg.Go(func() {
		ctx := grapheval.ContextWithNewWorker(ctx)
		checkDiags = checkAll(ctx, rootModuleInstance)
	})
	wg.Go(func() {
		ctx := grapheval.ContextWithNewWorker(ctx)
		orphanDiags = announcePlanOrphans(ctx, glue, rootModuleInstance)
	})
	wg.Wait()
	diags = diags.Append(checkDiags)
	diags = diags.Append(orphanDiags)
	// (We intentionally don't return here because we'll make a best effort
	// to return a partial result even if we encountered errors, so an
	// operator can potentially use the partial result to help debug
	// the errors.)

	// Once checkAll has completed we should've either visited and evaluated
	// everything as much as we can, so we can now just collect the result
	// value and return.

	return &PlanningResult{
		RootModuleOutputs: CollectRootModuleOutputs(ctx, rootModuleInstance),
		Glue:              glue,
		Oracle:            oracle,
	}, diags
}

// PlanningResult is the return value of [ConfigInstance.DrivePlanning],
// describing the top-level outcomes of the planning process.
type PlanningResult struct {
	// Oracle is the same [PlanningOracle] that was offered when creating
	// the [PlanGlue] during the [ConfigInstance.DrivePlanning] call, returned
	// here so that it can be used in the planning engine's followup work.
	Oracle *PlanningOracle

	// Glue is the [PlanGlue] object that was constructed during the
	// [ConfigInstance.DrivePlanning] call. This is guaranteed to be exactly
	// the object that the buildPlan function returned, and so it's safe to
	// type-assert it to whatever concrete implementation type the caller
	// used.
	Glue PlanGlue

	// RootModuleOutputs is the object representing the planned output values
	// from the root module.
	//
	// This will contain unknown value placeholders for any part of an output
	// value which depends on the result of an action that won't be taken
	// until the apply phase.
	RootModuleOutputs RootModuleOutputs
}

type planningEvalGlue struct {
	// planEngineGlue is the planning glue implementation provided by the
	// planning engine when it called [ConfigInstance.DrivePlanning].
	planEngineGlue PlanGlue
	providers      *managedProviders
}

var _ evalglue.Glue = (*planningEvalGlue)(nil)

// ProviderFunction implements evalglue.Glue.
func (p *planningEvalGlue) ProviderFunction(ctx context.Context, provider addrs.Provider, providerInst configgraph.Maybe[*configgraph.ProviderInstance], pf addrs.ProviderFunction, rng hcl.Range) (function.Function, tfdiags.Diagnostics) {
	if providerInst, ok := configgraph.GetKnown(providerInst); ok {
		return p.providers.ConfiguredFunction(ctx, providerInst.Addr, pf, rng)
	}

	return p.providers.BuildFunction(ctx, provider, pf, false, rng)
}

// ResourceInstanceValue implements evalglue.Glue.
func (p *planningEvalGlue) ResourceInstanceValue(ctx context.Context, ri *configgraph.ResourceInstance, configVal cty.Value, providerInst configgraph.Maybe[*configgraph.ProviderInstance], riDeps addrs.Set[addrs.AbsResourceInstance]) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	desired := &DesiredResourceInstance{
		Addr:                      ri.Addr,
		ConfigVal:                 configgraph.PrepareOutgoingValue(configVal),
		Provider:                  ri.Provider,
		RequiredResourceInstances: riDeps,
		ResourceType:              ri.Addr.Resource.Resource.Type,
		ResourceMode:              ri.Addr.Resource.Resource.Mode,
	}
	if providerInst, ok := configgraph.GetKnown(providerInst); ok {
		desired.ProviderInstance = &providerInst.Addr
	}

	cbdVal, cbdRng, moreDiags := ri.CreateBeforeDestroy(ctx)
	diags = diags.Append(moreDiags)
	if !exprs.IsEvalError(cbdVal) {
		const errSummary = "Invalid create_before_destroy argument"
		unmarkedVal, _ := cbdVal.Unmark()
		var subjRng *hcl.Range
		if cbdRng != nil {
			subjRng = cbdRng.ToHCL().Ptr()
		}
		if !unmarkedVal.IsKnown() {
			// FIXME: Instead of being an error, this should just force deferring
			// the planning of this resource instance. That'll take some
			// refactoring to make it possible for us to defer at this layer,
			// though.
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  errSummary,
				Detail:   fmt.Sprintf("The create_before_destroy setting for %s is derived from a value that won't be decided until the apply phase, which is invalid because OpenTofu needs to decide the order of operations during the plan phase.", desired.Addr),
				Subject:  subjRng,
			})
		} else if unmarkedVal == cty.True {
			desired.CreateBeforeDestroy = true
		} else if unmarkedVal == cty.False {
			// We reject "false" here for now because in future it might come
			// to represent forcing the destroy-then-create order for situations
			// where it's simply impossible for two objects to exist for the
			// resource instance at the same time. If we do that then we'll
			// need to change [DesiredResourceInstance.CreateBeforeDestroy] to
			// be a more general "replace order" field that can accept all
			// three possibilities of "create-then-destroy", "destroy-then-create",
			// or "don't care".
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  errSummary,
				Detail:   fmt.Sprintf("The create_before_destroy setting for %s may not be set to false.\n\nIf this argument is present then it must be set to true. It is not currently possible to force destroy-then-create ordering.", desired.Addr),
				Subject:  subjRng,
			})
		} else if !unmarkedVal.IsNull() {
			// No other result should be possible if [CreateBeforeDestroy]
			// was implemented correctly.
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				errSummary,
				fmt.Sprintf("Internal processing of create_before_destroy for %s returned unexpected value %#v. This is a bug in OpenTofu.", desired.Addr, cbdVal),
			))
		}
	}

	// TODO: Populate everything else in [DesiredResourceInstance], once
	// package configgraph knows how to provide those answers.

	if desired.Addr.Resource.Resource.Mode == addrs.EphemeralResourceMode {
		return p.providers.OpenEphemeralResourceInstance(ctx, desired.Addr, desired.ConfigVal, desired.Provider, desired.ProviderInstance)
	}

	ret, moreDiags := p.planEngineGlue.PlanDesiredResourceInstance(ctx, desired)
	diags = diags.Append(moreDiags)
	return ret, diags
}

func announcePlanOrphans(ctx context.Context, glue PlanGlue, rootModuleInstance evalglue.CompiledModuleInstance) tfdiags.Diagnostics {
	var diags collectedDiagnostics
	announcePlanOrphansRecursive(ctx, glue, &diags, addrs.RootModuleInstance, rootModuleInstance)
	return diags.diags
}

func announcePlanOrphansRecursive(ctx context.Context, glue PlanGlue, diags *collectedDiagnostics, currentModuleInstAddr addrs.ModuleInstance, currentModuleInstance evalglue.CompiledModuleInstance) {
	var wg sync.WaitGroup
	// Announce the module calls themselves
	diags.Append(
		glue.PlanModuleCallOrphans(ctx, currentModuleInstAddr, currentModuleInstance.ChildModuleCalls(ctx)),
	)
	// Announce the instances of each module call and recurse into each one
	// to deal with the declarations within it.
	wg.Go(func() {
		ctx := grapheval.ContextWithNewWorker(ctx)
		for callAddr := range currentModuleInstance.ChildModuleCalls(ctx) {
			diags.Append(
				glue.PlanModuleCallInstanceOrphans(ctx, callAddr.Absolute(currentModuleInstAddr), func(yield func(addrs.InstanceKey) bool) {
					ctx := grapheval.ContextWithNewWorker(ctx)
					for callInstAddr := range currentModuleInstance.ChildModuleInstancesForCall(ctx, callAddr) {
						if !yield(callInstAddr.Key) {
							return
						}
					}
				}),
			)
			for callInstAddr, childInst := range currentModuleInstance.ChildModuleInstancesForCall(ctx, callAddr) {
				childInstAddr := currentModuleInstAddr.Child(callInstAddr.Call.Name, callInstAddr.Key)
				announcePlanOrphansRecursive(ctx, glue, diags, childInstAddr, childInst)
			}
		}
	})
	// Announce the resource declarations themselves
	diags.Append(
		glue.PlanResourceOrphans(ctx, currentModuleInstAddr, currentModuleInstance.Resources(ctx)),
	)
	// Announce the instances of each resource
	wg.Go(func() {
		ctx := grapheval.ContextWithNewWorker(ctx)
		for resourceAddr := range currentModuleInstance.Resources(ctx) {
			diags.Append(
				glue.PlanResourceInstanceOrphans(ctx, resourceAddr.Absolute(currentModuleInstAddr), func(yield func(addrs.InstanceKey) bool) {
					ctx := grapheval.ContextWithNewWorker(ctx)
					for resourceInst := range currentModuleInstance.ResourceInstancesForResource(ctx, resourceAddr) {
						if !yield(resourceInst.Addr.Resource.Key) {
							return
						}
					}
				}),
			)
		}
	})
	wg.Wait()
}

type collectedDiagnostics struct {
	diags tfdiags.Diagnostics
	mu    sync.Mutex
}

func (d *collectedDiagnostics) Append(items ...any) {
	d.mu.Lock()
	d.diags = d.diags.Append(items...)
	d.mu.Unlock()
}
