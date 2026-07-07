// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval

import (
	"context"
	"fmt"
	"log"

	"github.com/apparentlymart/go-workgraph/workgraph"
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/configgraph"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/evalglue"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// ApplyGlue is used with [DriveApplying] to allow the evaluation system to
// communicate with the applying engine that called it.
//
// Methods of this type can be called concurrently with each other and with
// themselves, so implementations must use appropriate synchronization
// primitives to avoid race conditions.
type ApplyGlue interface {
	// ResourceInstanceFinalState blocks until the apply phase has completed
	// enough work to decide the final state value for the resource instance
	// with the given address and then returns that value.
	//
	// If operations that would contribute to that final value fail then this
	// function returns a suitable placeholder for the final state that can
	// would allow valid dependent expressions to evaluate successfully though
	// potentially to an unknown value. Returning the "planned state" that
	// was predicted during the planning phase is acceptable, and returning
	// [cty.DynamicVal] is also acceptable as a last resort when absolutely
	// no information is available.
	//
	// Diagnostics from apply-time actions must be reported through some other
	// channel controlled by the apply engine itself.
	ResourceInstanceFinalState(ctx context.Context, addr addrs.AbsResourceInstance) cty.Value
}

// ApplyOracle creates an [ApplyOracle] object that can be used to support an
// "apply" operation that's driven by another part of the system.
//
// While in the planning phase the evaluator is the primary driver and the
// planning engine just responds to callbacks, the apply phase has an inverted
// structure where the apply engine drives execution and just calls into the
// evaluator to obtain supporting information as needed.
//
// The object returned by this function is therefore passive until asked a
// question through one of its methods, but asking a question is likely to
// cause various other evaluation work to be performed in order to gather the
// data needed to answer the question. The apply phase only evaluates parts
// of the configuration needed to perform the planned actions, because we
// assume that everything else was already evaluated and validated during the
// planning phase.
func (c *ConfigInstance) ApplyOracle(ctx context.Context, glue ApplyGlue) (*ApplyOracle, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// Our preparation work will interact with the graph-eval machinery so
	// we need a suitably-annotated context to allow it to track any promise
	// dependencies that are relevant during initialization. (The apply engine
	// will make its own grapheval context to do the main work, though.)
	ctx = grapheval.ContextWithNewWorker(ctx)

	evalGlue := &applyingEvalGlue{
		applyEngineGlue: glue,
	}
	rootModuleInstance, moreDiags := c.newRootModuleInstance(ctx, evalGlue)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return nil, diags
	}

	managedProviders := newManagedProviders(c.evalContext.Providers, func(ctx context.Context, addr addrs.AbsProviderInstanceCorrect) (cty.Value, tfdiags.Diagnostics) {
		inst := evalglue.ProviderInstance(ctx, rootModuleInstance, addr)
		if inst == nil {
			// We should not get here because the apply phase should only ask for
			// provider instances that were present during the planning phase, and
			// we should be using exactly the same configuration source code now.
			return cty.DynamicVal, tfdiags.New(fmt.Errorf("missing configuration for %s", addr))
		}
		v, diags := inst.ConfigValue(ctx)
		return configgraph.PrepareOutgoingValue(v), diags

	})

	// Inject configured providers
	evalGlue.providers = managedProviders

	return &ApplyOracle{
		root:      rootModuleInstance,
		providers: managedProviders,
	}, diags
}

// applyingEvalGlue is an adapter from the [evalglue.Glue] interface to the
// [ApplyGlue] interface, to bridge between the general-purpose evaluator code
// and the specialized API implemented by the apply engine in particular.
type applyingEvalGlue struct {
	applyEngineGlue ApplyGlue
	providers       *managedProviders
}

// ResourceInstanceValue implements [evalglue.Glue].
func (g *applyingEvalGlue) ResourceInstanceValue(ctx context.Context, ri *configgraph.ResourceInstance, cfgVal cty.Value, providerInst configgraph.Maybe[*configgraph.ProviderInstance], _ addrs.Set[addrs.AbsResourceInstance]) (cty.Value, tfdiags.Diagnostics) {
	if ri.Addr.Resource.Resource.Mode == addrs.EphemeralResourceMode {
		if providerInst, ok := configgraph.GetKnown(providerInst); ok {
			return g.providers.OpenEphemeralResourceInstance(ctx, ri.Addr, cfgVal, ri.Provider, &providerInst.Addr)
		}
		log.Printf("[WARN] Provider is not yet known for ephemeral resource %s", ri.Addr)
		return cty.UnknownVal(cty.DynamicPseudoType), nil
	}

	finalVal := g.applyEngineGlue.ResourceInstanceFinalState(ctx, ri.Addr)
	return finalVal, nil
}

// ProviderFunction implements [evalglue.Glue]
func (g *applyingEvalGlue) ProviderFunction(ctx context.Context, provider addrs.Provider, providerInst configgraph.Maybe[*configgraph.ProviderInstance], pf addrs.ProviderFunction, rng hcl.Range) (function.Function, tfdiags.Diagnostics) {
	if providerInst, ok := configgraph.GetKnown(providerInst); ok {
		return g.providers.ConfiguredFunction(ctx, providerInst.Addr, pf, rng)
	}

	return g.providers.BuildFunction(ctx, provider, pf, false, rng)
}

// An ApplyOracle is returned by [ConfigInstance.ApplyOracle] to give the main
// apply engine access to various information from the configuration that it
// will need during the apply process.
//
// All methods of an [ApplyOracle] must be called with a [context.Context]
// derived from one produced by [grapheval.ContextWithWorker].
//
// Whereas the planning process is driven primarily by the dependencies
// discovered dynamically during evaluation, the apply process is instead
// driven primarily by an execution graph that was built during the planning
// process. The apply-time execution steps therefore need to be able to
// pull the information they need from the evaluation engine on request
// instead of the evaluation engine pushing the information out, and an
// object of this type provides that information.
//
// It's the responsibilty of the planning engine to construct an execution
// graph that ensures that the apply phase will request information from
// the oracle only once it has already been made available by earlier work.
type ApplyOracle struct {
	root      evalglue.CompiledModuleInstance
	providers *managedProviders
}

// DesiredResourceInstance returns the [DesiredResourceInstance] object
// associated with the given resource instance address, or nil if the given
// address does not match a desired resource instance.
//
// This API assumes that the apply phase is working from an execution graph
// built during the planning phase and is therefore relying on the plan phase
// to correctly describe a subset of the desired resource instances so that
// this should never return nil. If this _does_ return nil then that suggests
// a bug in the planning engine, which caused it to create an incorrect
// execution graph.
func (o *ApplyOracle) DesiredResourceInstance(ctx context.Context, addr addrs.AbsResourceInstance) (*DesiredResourceInstance, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	inst := evalglue.ResourceInstance(ctx, o.root, addr)
	if inst == nil {
		// We should not get here because the apply phase should only ask for
		// resource instances that were present during the planning phase, and
		// we should be using exactly the same configuration source code now.
		diags = diags.Append(fmt.Errorf("missing configuration for %s", addr))
		return nil, diags
	}
	// TODO: Factor out the logic for building a [DesiredResourceInstance]
	// into a place that all phases can share. Currently that logic is within
	// the planning codepath and so not reachable from here. For now this is
	// just a minimal stub giving just enough for the incomplete apply engine
	// to do its work.
	configVal, moreDiags := inst.ConfigValue(ctx)
	diags = diags.Append(moreDiags)
	providerInst, _, moreDiags := inst.ProviderInstance(ctx)
	diags = diags.Append(moreDiags)
	providerInstAddr, _ := configgraph.GetKnown(configgraph.MapMaybe(providerInst, func(pi *configgraph.ProviderInstance) addrs.AbsProviderInstanceCorrect {
		return pi.Addr
	}))

	riDeps := addrs.MakeSet[addrs.AbsResourceInstance]()
	for depInst := range inst.ResourceInstanceDependencies(ctx) {
		riDeps.Add(depInst.Addr)
	}

	ret := &DesiredResourceInstance{
		Addr:                      inst.Addr,
		ConfigVal:                 configVal,
		Provider:                  inst.Provider,
		ProviderInstance:          &providerInstAddr,
		ResourceMode:              addr.Resource.Resource.Mode,
		ResourceType:              addr.Resource.Resource.Type,
		RequiredResourceInstances: riDeps,
		CreateProvisioners:        inst.CreateProvisioners,
	}
	return ret, diags
}

func (o *ApplyOracle) DestroyProvisioners(ctx context.Context, addr addrs.AbsResourceInstance) []Provisioner {
	return evalglue.DestroyProvisioners(ctx, o.root, addr)
}

// ProviderInstanceConfig returns the configuration value for the given
// provider instance, or [cty.NilVal] if there is no such provider instance.
//
// This API assumes that the apply phase is working from an execution graph
// built during the planning phase and is therefore relyingo n the plan phase
// to refer only to provider instances that are present ni the configuration.
// If this _does_ return cty.NilVal then that suggests a bug in the planning
// engine, causing it to create an incorrect execution graph.
func (o *ApplyOracle) ProviderInstance(ctx context.Context, addr addrs.AbsProviderInstanceCorrect) (providers.Interface, tfdiags.Diagnostics) {
	return o.providers.ProviderInstance(ctx, addr)
}

// AnnounceAllGraphevalRequests calls the given function once for each internal
// workgraph request that has previously been started by requests to this
// oracle.
//
// This is used by the apply engine as part of its implementation of
// [grapheval.RequestTracker], so that promise-resolution-related diagnostics
// can include information about which requests were involved in the problem.
//
// This information is collected as a separate step only when needed because
// that avoids us needing to keep track of this metadata on the happy path,
// so that we only pay the cost of gathering this data when we're actually
// going to use it for something.
func (o *ApplyOracle) AnnounceAllGraphevalRequests(announce func(workgraph.RequestID, grapheval.RequestInfo)) {
	o.root.AnnounceAllGraphevalRequests(announce)
}

func (o *ApplyOracle) RootOutputs(ctx context.Context) map[string]cty.Value {
	ctx = grapheval.ContextWithNewWorker(ctx)

	return CollectRootModuleOutputs(ctx, o.root).OutputValues
}

func (o *ApplyOracle) Close(ctx context.Context) tfdiags.Diagnostics {
	return checkAll(ctx, o.root).Append(o.providers.Close(ctx))
}
