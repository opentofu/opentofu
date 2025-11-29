// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval

import (
	"context"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/configgraph"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/evalglue"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/plans/objchange"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// prepareToPlan implements an extra preparation phase we perform before
// running the main plan phase so we can deal with the "chicken or the egg"
// problem of needing to evaluate configuraton to learn the relationships
// between resources instances and provider instances but needing to already
// know those relationships in order to fully evaluate the configuration.
//
// As a compromise then, we initially perform a more conservative walk that
// just treats all resource instances as having fully-unknown values so that
// we don't need to configure any providers or open any ephemeral resource
// instances, and then ask the resulting configgraph objects to report
// the resource/provider dependencies which we can then use on a subsequent
// re-eval that includes the real provider planning actions.
//
// This approach relies on the idea that an evaluation with more unknown
// values should always produce a superset of the true dependencies that
// would be reported once there are fewer unknown values, and so the
// result of this function should capture _at least_ the dependencies
// required to successfully plan, possibly also including some harmless
// additional dependency relationships that aren't strictly needed once we can
// evaluate using real resource planning results. The planning phase will
// then be able to produce its own tighter version of this information to
// use when building the execution graph for the apply phase.
func (c *ConfigInstance) prepareToPlan(ctx context.Context) (*ResourceRelationships, tfdiags.Diagnostics) {
	// All of our work will be associated with a workgraph worker that serves
	// as the initial worker node in the work graph.
	ctx = grapheval.ContextWithNewWorker(ctx)

	rootModuleInstance, diags := c.precheckedModuleInstance(ctx)
	if diags.HasErrors() {
		return nil, diags
	}
	ret := &ResourceRelationships{
		EphemeralResourceUsers: addrs.MakeMap[addrs.AbsResourceInstance, EphemeralResourceInstanceUsers](),
		ProviderInstanceUsers:  addrs.MakeMap[addrs.AbsProviderInstanceCorrect, ProviderInstanceUsers](),
	}
	for depender := range evalglue.ResourceInstancesDeep(ctx, rootModuleInstance) {
		dependerAddr := depender.Addr
		for dependee := range depender.ResourceInstanceDependencies(ctx) {
			dependeeAddr := dependee.Addr
			if dependeeAddr.Resource.Resource.Mode == addrs.EphemeralResourceMode {
				if !ret.EphemeralResourceUsers.Has(dependeeAddr) {
					ret.EphemeralResourceUsers.Put(dependeeAddr, EphemeralResourceInstanceUsers{
						ResourceInstances: addrs.MakeSet[addrs.AbsResourceInstance](),
						ProviderInstances: addrs.MakeSet[addrs.AbsProviderInstanceCorrect](),
					})
				}
				set := ret.EphemeralResourceUsers.Get(dependeeAddr).ResourceInstances
				set.Add(dependerAddr)
			}
		}
		providerInst, _, _ := depender.ProviderInstance(ctx)
		if providerInst, known := configgraph.GetKnown(providerInst); known {
			providerInstAddr := providerInst.Addr
			if !ret.ProviderInstanceUsers.Has(providerInstAddr) {
				ret.ProviderInstanceUsers.Put(providerInstAddr, ProviderInstanceUsers{
					ResourceInstances: addrs.MakeSet[addrs.AbsResourceInstance](),
				})
			}
			set := ret.ProviderInstanceUsers.Get(providerInstAddr).ResourceInstances
			set.Add(dependerAddr)
		}
	}
	for depender := range evalglue.ProviderInstancesDeep(ctx, rootModuleInstance) {
		dependerAddr := depender.Addr
		for dependee := range depender.ResourceInstanceDependencies(ctx) {
			dependeeAddr := dependee.Addr
			if dependeeAddr.Resource.Resource.Mode == addrs.EphemeralResourceMode {
				if !ret.EphemeralResourceUsers.Has(dependeeAddr) {
					ret.EphemeralResourceUsers.Put(dependeeAddr, EphemeralResourceInstanceUsers{
						ResourceInstances: addrs.MakeSet[addrs.AbsResourceInstance](),
						ProviderInstances: addrs.MakeSet[addrs.AbsProviderInstanceCorrect](),
					})
				}
				set := ret.EphemeralResourceUsers.Get(dependeeAddr).ProviderInstances
				set.Add(dependerAddr)
			}
		}
	}
	return ret, diags
}

type ResourceRelationships struct {
	// EphemeralResourceUsers is a map from ephemeral resource instance
	// addresses to the sets of addresses of other resource instances (of
	// any mode, including other ephemeral ones) which depend on them.
	//
	// A subsequent plan phase can use this to detect when all of the
	// downstream users of an ephemeral resource instance have finished
	// their work and so it's okay to close the ephemeral resource instance.
	//
	// TODO: This should also capture the provider instances that are depending
	// on each ephemeral resource instance.
	EphemeralResourceUsers addrs.Map[addrs.AbsResourceInstance, EphemeralResourceInstanceUsers]

	// EphemeralResourceUsers is a map from provider instance addresses to the
	// sets of addresses of resource instances which a provided by them.
	//
	// A subsequent plan phase can use this to detect when all of the
	// downstream users of a provider instance have finished their work and so
	// it's okay to close the provider instance.
	ProviderInstanceUsers addrs.Map[addrs.AbsProviderInstanceCorrect, ProviderInstanceUsers]
}

type EphemeralResourceInstanceUsers struct {
	ResourceInstances addrs.Set[addrs.AbsResourceInstance]
	ProviderInstances addrs.Set[addrs.AbsProviderInstanceCorrect]
}

type ProviderInstanceUsers struct {
	ResourceInstances addrs.Set[addrs.AbsResourceInstance]
}

// precheckedModuleInstance deals with the common part of both
// [ConfigInstance.prepareToPlan] and [ConfigInstance.Validate], where we
// evaluate the configuration using unknown value placeholders for resource
// instances to discover information about the configuration even when we
// aren't able to configure any providers.
//
// This must be called with a [context.Context] that's associated with a
// [grapheval.Worker].
func (c *ConfigInstance) precheckedModuleInstance(ctx context.Context) (evalglue.CompiledModuleInstance, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	internalGlue := &preparationGlue{
		providers: c.evalContext.Providers,
	}
	rootModuleInstance, moreDiags := c.newRootModuleInstance(ctx, internalGlue)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		// If we can't even load the root module then we'll bail out early.
		return nil, diags
	}

	// For validation purposes we don't need to do anything other than the
	// full-tree check that would normally run alongside the driving of
	// some other operation.
	moreDiags = checkAll(ctx, rootModuleInstance)
	diags = diags.Append(moreDiags)
	return rootModuleInstance, diags
}

// preparationGlue is the [evaluationGlue] implementation used by
// [ConfigInstance.precheckedModuleInstance].
type preparationGlue struct {
	// preparationGlue uses provider schema information to prepare placeholder
	// "final state" values for resource instances because validation does
	// not use information from the state.
	providers Providers
}

// ResourceInstanceValue implements evaluationGlue.
func (v *preparationGlue) ResourceInstanceValue(ctx context.Context, ri *configgraph.ResourceInstance, configVal cty.Value, _ configgraph.Maybe[*configgraph.ProviderInstance]) (cty.Value, tfdiags.Diagnostics) {
	schema, diags := v.providers.ResourceTypeSchema(ctx,
		ri.Provider,
		ri.Addr.Resource.Resource.Mode,
		ri.Addr.Resource.Resource.Type,
	)
	if diags.HasErrors() {
		// If we can't get schema then we'll return a fully-unknown value
		// as a placeholder because we don't even know what type we need.
		return cty.DynamicVal, diags
	}

	// FIXME: If we have a managed or data resource instance, as opposed to
	// an ephemeral resource instance, then we should check to make sure
	// that ephemeral-marked values only appear in parts of the configVal
	// that correspond to WriteOnly attributes in the schema.

	// We now have enough information to produce a placeholder "planned new
	// state" by placing unknown values in any location that the provider
	// would be allowed to choose a value.
	// NOTE: With the implementation of this function as of commit
	// dd5257d58e27b1af3b8dde97c80daec97f6ca55e this shows as a pretty
	// hot path in CPU profiling, which is not a huge surprise -- we've
	// known it as relatively expensive from its use in "package tofu"
	// already -- but it stands out more in this new implementation because
	// it's not competing with other expensive work like performing transitive
	// reduction on a dag, etc. The main problem seems to be that it allocates
	// a _lot_ of temporary objects, and so there's lots of GC pressure.
	return objchange.ProposedNew(
		schema.Block, cty.NullVal(schema.Block.ImpliedType()), configVal,
	), diags
}
