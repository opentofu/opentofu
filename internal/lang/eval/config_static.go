// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval

import (
	"context"
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/getproviders"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/configgraph"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/evalglue"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

var NewStaticPlugins = evalglue.NewStaticPlugins

// StaticCheck performs a CheckAll, but using static glue (no providers or state)
func (c *ConfigInstance) StaticCheck(ctx context.Context) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	// All of our work will be associated with a workgraph worker that serves
	// as the initial worker node in the work graph.
	ctx = grapheval.ContextWithNewWorker(ctx)

	internalGlue := &staticGlue{}
	rootModuleInstance, moreDiags := c.newRootModuleInstance(ctx, internalGlue)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		// If we can't even load the root module then we'll bail out early.
		return diags
	}

	// If the grapheval package detects a self-dependency problem during
	// evaluation then it'll use this tracker to find human-friendly names
	// for all of the requests involved in the error.
	ctx = grapheval.ContextWithRequestTracker(ctx, workgraphRequestTracker{rootModuleInstance})

	moreDiags = checkAll(ctx, rootModuleInstance)
	diags = diags.Append(moreDiags)
	return diags
}

// Fetches all of the provider requirments referred to by a given configuration tree
func (c *ConfigInstance) ProviderRequirements(ctx context.Context) (getproviders.Requirements, *getproviders.ProvidersQualification, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// All of our work will be associated with a workgraph worker that serves
	// as the initial worker node in the work graph.
	ctx = grapheval.ContextWithNewWorker(ctx)

	internalGlue := &staticGlue{}
	rootModuleInstance, moreDiags := c.newRootModuleInstance(ctx, internalGlue)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		// If we can't even load the root module then we'll bail out early.
		return nil, nil, diags
	}

	// If the grapheval package detects a self-dependency problem during
	// evaluation then it'll use this tracker to find human-friendly names
	// for all of the requests involved in the error.
	ctx = grapheval.ContextWithRequestTracker(ctx, workgraphRequestTracker{rootModuleInstance})

	reqs, quals, moreDiags := rootModuleInstance.ProviderRequirements(ctx)
	diags = diags.Append(moreDiags)

	allChildren := rootModuleInstance.ChildModuleInstances(ctx)
	for _, child := range allChildren {
		moreReqs, moreQuals, moreDiags := child.ProviderRequirements(ctx)
		diags = diags.Append(moreDiags)
		reqs = reqs.Merge(moreReqs)
		quals = quals.Merge(moreQuals)
	}

	moreDiags = checkAll(ctx, rootModuleInstance)
	diags = diags.Append(moreDiags)

	return reqs, quals, diags
}

// staticGlue is a similar construct to [configs.staticScope], but does not implement:
// * variable `const` attr checking
// * enhanced diagnostics to help track down usages of dynamic values in a static context
// * tofu.applying (not sure how/where this is set)
// It does add support for:
// * module outputs
type staticGlue struct{}

// ProviderFunction implements evalglue.Glue.
func (v *staticGlue) ProviderFunction(ctx context.Context, provider addrs.Provider, providerInst exprs.FromValue[*configgraph.ProviderInstance], subject addrs.ProviderFunction, rng hcl.Range) (function.Function, tfdiags.Diagnostics) {
	// TODO replace with a provider marked value and enhance the resource mark checking for "static" situations
	return function.Function{}, tfdiags.New(&hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  "Provider function in static context",
		Detail:   fmt.Sprintf("Unable to use %s in static context", subject.String()),
		Subject:  rng.Ptr(),
	})
}

// ResourceInstanceValue implements evaluationGlue.
func (v *staticGlue) ResourceInstanceValue(ctx context.Context, ri *configgraph.ResourceInstance, configVal cty.Value, _ exprs.FromValue[*configgraph.ProviderInstance], _ addrs.Set[addrs.AbsResourceInstance]) (cty.Value, tfdiags.Diagnostics) {
	return cty.DynamicVal, nil
}
