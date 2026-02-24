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
	"github.com/opentofu/opentofu/internal/plans/objchange"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

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
	providers ProvidersSchema
}

// ValidateProviderConfig implements evalglue.Glue.
func (v *preparationGlue) ValidateProviderConfig(ctx context.Context, provider addrs.Provider, configVal cty.Value) tfdiags.Diagnostics {
	// TODO maybe this is why we create validation glue?
	//return v.providers.ValidateProviderConfig(ctx, provider, configVal)
	return nil
}

// ResourceInstanceValue implements evaluationGlue.
func (v *preparationGlue) ResourceInstanceValue(ctx context.Context, ri *configgraph.ResourceInstance, configVal cty.Value, _ configgraph.Maybe[*configgraph.ProviderInstance], _ addrs.Set[addrs.AbsResourceInstance]) (cty.Value, tfdiags.Diagnostics) {
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

	/* TODO maybe this is why we create validation glue?
	validateDiags := v.providers.ValidateResourceConfig(ctx, ri.Provider, ri.Addr.Resource.Resource.Mode, ri.Addr.Resource.Resource.Type, configVal)
	diags = diags.Append(validateDiags)
	if diags.HasErrors() {
		// Provider indicated an invalid resource configuration
		return cty.DynamicVal, diags
	}*/

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
