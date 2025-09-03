// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval

import (
	"context"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/lang/eval/internal/configgraph"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/plans/objchange"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Validate checks whether the configuration instance is valid when called
// with the previously-provided input variables and dependencies.
//
// Returns at least one error diagnostic if the configuration call is not valid.
//
// This is exposed for use by "validation-only" callers like the "tofu validate"
// command, but does NOT need to be called before other methods like
// [ConfigInstance.DrivePlanning] because equivalent checks occur within those
// operations.
func (c *ConfigInstance) Validate(ctx context.Context) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	// All of our work will be associated with a workgraph worker that serves
	// as the initial worker node in the work graph.
	ctx = grapheval.ContextWithNewWorker(ctx)

	internalGlue := &validationGlue{
		providers: c.evalContext.Providers,
	}
	rootModuleInstance, moreDiags := c.newRootModuleInstance(ctx, internalGlue)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		// If we can't even load the root module then we'll bail out early.
		return diags
	}

	// For validation purposes we don't need to do anything other than the
	// full-tree check that would normally run alongside the driving of
	// some other operation.
	moreDiags = checkAll(ctx, rootModuleInstance)
	diags = diags.Append(moreDiags)
	return diags
}

// validationGlue is the [evaluationGlue] implementation used by
// [ConfigInstance.Validate].
type validationGlue struct {
	// validationGlue uses provider schema information to prepare placeholder
	// "final state" values for resource instances because validation does
	// not use information from the state.
	providers Providers
}

// ResourceInstanceValue implements evaluationGlue.
func (v *validationGlue) ResourceInstanceValue(ctx context.Context, ri *configgraph.ResourceInstance, configVal cty.Value) (cty.Value, tfdiags.Diagnostics) {
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

	// We now have enough information to produce a placeholder "planned new
	// state" by placing unknown values in any location that the provider
	// would be allowed to choose a value.
	return objchange.ProposedNew(
		schema.Block, cty.NullVal(schema.Block.ImpliedType()), configVal,
	), diags
}
