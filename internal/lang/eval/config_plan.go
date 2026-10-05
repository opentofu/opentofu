// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval

import (
	"context"
	"log"

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
	PreProcess(ctx context.Context, oracle *PlanningOracle, targeter func(target addrs.Targetable)) tfdiags.Diagnostics

	// Creates planned action(s) for the given resource instance and return
	// the planned new state that would result from those actions.
	//
	// This is called only for resource instances currently declared in the
	// configuration. The planning engine must deal with planning actions
	// for "orphaned" resource instances (those which are only present in
	// prior state) separately as each of the "Plan*Orphans" methods are
	// called to report what exists in the desired state.
	PlanDesiredResourceInstance(ctx context.Context, oracle *PlanningOracle, inst *DesiredResourceInstance) (cty.Value, tfdiags.Diagnostics)

	PostProcess(ctx context.Context, oracle *PlanningOracle) tfdiags.Diagnostics
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
func (c *ConfigInstance) DrivePlanning(ctx context.Context, glue PlanGlue) (*PlanningResult, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// All of our work will be associated with a workgraph worker that serves
	// as the initial worker node in the work graph.
	ctx = grapheval.ContextWithNewWorker(ctx)

	// We have a little chicken vs. egg problem here where we can't fully
	// initialize the oracle until we've built the root module instance,
	// so we initially pass an intentionally-invalid oracle to the build
	// function and then make sure it's valid before we make any use
	// of the PlanGlue object it returns.
	// TODO better handling of oracle close
	oracle := &PlanningOracle{}

	evalGlue := &planningEvalGlue{
		oracle:         oracle,
		planEngineGlue: glue,
	}
	rootModuleInstance, moreDiags := c.newRootModuleInstance(ctx, evalGlue)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return nil, diags.Append(oracle.Close(ctx))
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

	// Tell the glue that we are almost ready to walk the full configuration
	// and give it a chance to handle target/exclude logic pre-emptively.
	// We need to give it a way to pre-eval targeted resources before the main walk.
	glue.PreProcess(ctx, oracle, func(target addrs.Targetable) {
		ctx := grapheval.ContextWithNewWorker(ctx)
		ctx = grapheval.ContextWithRequestTracker(ctx, workgraphRequestTracker{rootModuleInstance})

		addTarget := func(ri *configgraph.ResourceInstance) {
			// Populate the value before the glue disables itself for the rest of processing
			log.Printf("[TRACE] %s targeting %s", target, ri.Addr)
			ri.Value(ctx)
		}

		switch target.AddrType() {
		case addrs.ConfigResourceAddrType:
			configResource := target.(addrs.ConfigResource)
			for _, modInst := range evalglue.ConfigModuleInstances(ctx, rootModuleInstance, configResource.Module) {
				for resInst := range modInst.ResourceInstancesForResource(ctx, configResource.Resource) {
					addTarget(resInst)
				}
			}
		case addrs.AbsResourceAddrType:
			absResource := target.(addrs.AbsResource)
			modInst := evalglue.ModuleInstance(ctx, rootModuleInstance, absResource.Module)
			if modInst != nil {
				for resInst := range modInst.ResourceInstancesForResource(ctx, absResource.Resource) {
					addTarget(resInst)
				}
			}
		case addrs.AbsResourceInstanceAddrType:
			absResourceInstance := target.(addrs.AbsResourceInstance)
			resInst := evalglue.ResourceInstance(ctx, rootModuleInstance, absResourceInstance)
			if resInst != nil {
				addTarget(resInst)
			}
		case addrs.ModuleAddrType:
			module := target.(addrs.Module)
			for _, modInst := range evalglue.ConfigModuleInstances(ctx, rootModuleInstance, module) {
				for resInst := range evalglue.ResourceInstancesDeep(ctx, modInst) {
					addTarget(resInst)
				}
			}
		case addrs.ModuleInstanceAddrType:
			moduleInstance := target.(addrs.ModuleInstance)
			modInst := evalglue.ModuleInstance(ctx, rootModuleInstance, moduleInstance)
			if modInst != nil {
				for resInst := range evalglue.ResourceInstancesDeep(ctx, modInst) {
					addTarget(resInst)
				}
			}
		}
	})

	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return nil, diags.Append(oracle.Close(ctx))
	}

	// The plan phase is driven forward by us evaluating expressions during
	// the "checkAll" process, and so we can just run that here and then
	// it'll cause various calls out to the "glue" object whenever we're
	// ready to provide configuration for a resource instance and need to
	// obtain its result for downstream use.
	checkDiags := checkAll(ctx, rootModuleInstance)
	diags = diags.Append(checkDiags)

	// Now that we have visited the entire configuration, we can tell the oracle
	// to PostProcess.  TODO more words here
	postDiags := glue.PostProcess(ctx, oracle)
	diags = diags.Append(postDiags)

	// (We intentionally don't return here because we'll make a best effort
	// to return a partial result even if we encountered errors, so an
	// operator can potentially use the partial result to help debug
	// the errors.)

	// Once checkAll has completed we should've either visited and evaluated
	// everything as much as we can, so we can now just collect the result
	// value and return.

	return &PlanningResult{
		RootModuleOutputs: CollectRootModuleOutputs(ctx, rootModuleInstance),
	}, diags.Append(oracle.Close(ctx))
}

// PlanningResult is the return value of [ConfigInstance.DrivePlanning],
// describing the top-level outcomes of the planning process.
type PlanningResult struct {
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
	oracle         *PlanningOracle
	providers      *managedProviders
}

var _ evalglue.Glue = (*planningEvalGlue)(nil)

// ProviderFunction implements evalglue.Glue.
func (p *planningEvalGlue) ProviderFunction(ctx context.Context, provider addrs.Provider, providerInst exprs.FromValue[*configgraph.ProviderInstance], pf addrs.ProviderFunction, rng hcl.Range) (function.Function, tfdiags.Diagnostics) {
	if providerInst, ok := providerInst.ValueOk(); ok {
		return p.providers.ConfiguredFunction(ctx, providerInst.Addr, pf, rng)
	}

	return p.providers.BuildFunction(ctx, provider, pf, false, rng)
}

// ResourceInstanceValue implements evalglue.Glue.
func (p *planningEvalGlue) ResourceInstanceValue(ctx context.Context, ri *configgraph.ResourceInstance, configVal cty.Value, providerInst exprs.FromValue[*configgraph.ProviderInstance], riDeps addrs.Set[addrs.AbsResourceInstance]) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	desired := &DesiredResourceInstance{
		Addr:                      ri.Addr,
		ConfigVal:                 configgraph.PrepareOutgoingValue(configVal),
		RequiredResourceInstances: riDeps,
		IgnoreChangesPaths:        ri.IgnoreChangesPaths,
		ReplaceTriggeredBy:        ri.ReplaceTriggeredBy,
	}

	if desired.Addr.Resource.Resource.Mode == addrs.EphemeralResourceMode {
		providerInstAddr, _ := providerInst.Derive(func(pi *configgraph.ProviderInstance) (addrs.AbsProviderInstanceCorrect, error) {
			return pi.Addr, nil
		})
		return p.providers.OpenEphemeralResourceInstance(
			ctx, desired.Addr, desired.ConfigVal,
			ri.Provider, providerInstAddr,
		)
	}

	ret, moreDiags := p.planEngineGlue.PlanDesiredResourceInstance(ctx, p.oracle, desired)
	diags = diags.Append(moreDiags)
	return ret, diags
}
