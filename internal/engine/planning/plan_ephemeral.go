// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"context"
	"fmt"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/shared"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func (p *planGlue) planDesiredEphemeralResourceInstance(ctx context.Context, inst *eval.DesiredResourceInstance) (*resourceInstanceObject, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	ret := &resourceInstanceObject{
		Addr:         inst.Addr.CurrentObject(),
		Dependencies: addrs.MakeSet[addrs.AbsResourceInstanceObject](),
		Provider:     inst.Provider,

		// We'll start off with a completely-unknown placeholder value, but
		// we might refine this to be more specific as we learn more below.
		PlaceholderValue: cty.DynamicVal,

		// NOTE: PlannedChange remains nil until we actually produce a plan,
		// so early returns with errors are not guaranteed to have a valid
		// change object. Evaluation falls back on using PlaceholderValue
		// when no planned change is present.
	}
	for dep := range inst.RequiredResourceInstances.All() {
		ret.Dependencies.Add(dep.CurrentObject())
	}

	schema, _ := p.planCtx.providers.ResourceTypeSchema(ctx, inst.Provider, inst.Addr.Resource.Resource.Mode, inst.Addr.Resource.Resource.Type)
	if schema == nil || schema.Block == nil {
		// Should be caught during validation, so we don't bother with a pretty error here
		diags = diags.Append(fmt.Errorf("provider %q does not support ephemeral resource %q", inst.ProviderInstance, inst.Addr.Resource.Resource.Type))
		return ret, diags
	}

	if inst.ProviderInstance == nil {
		// If we don't even know which provider instance we're supposed to be
		// talking to then we'll just return a placeholder value, because
		// we don't have any way to generate a speculative plan.
		return ret, diags
	}

	providerClient, moreDiags := p.providerClient(ctx, *inst.ProviderInstance)
	if providerClient == nil {
		moreDiags = moreDiags.Append(tfdiags.AttributeValue(
			tfdiags.Error,
			"Provider instance not available",
			fmt.Sprintf("Cannot plan %s because its associated provider instance %s cannot initialize.", inst.Addr, *inst.ProviderInstance),
			nil,
		))
	}
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return ret, diags
	}

	newVal, closeFunc, openDiags := shared.OpenEphemeralResourceInstance(
		ctx,
		inst.Addr,
		schema.Block,
		*inst.ProviderInstance,
		providerClient,
		inst.ConfigVal,
		shared.EphemeralResourceHooks{},
	)
	diags = diags.Append(openDiags)
	if openDiags.HasErrors() {
		return ret, diags
	}

	p.planCtx.closeStackMu.Lock()
	p.planCtx.closeStack = append(p.planCtx.closeStack, closeFunc)
	p.planCtx.closeStackMu.Unlock()

	// FIXME: Unlike other resource modes, ephemeral resources can have a
	// different set of instances in the plan phase vs. the apply phase whenever
	// their count/for_each/enabled is derived from an ephemeral value, such
	// as `terraform.applying` which _intentionally_ varies between phases.
	// Therefore this current structure is subtly incorrect: it assumes that
	// the apply phase will have exactly the same ephemeral resource instances
	// as we have during the plan phase.
	//
	// To deal with that, instead of proactively execgraph-ing all of the
	// instances we know at plan time we could instead insert them only once
	// at least one non-ephemeral resource depends on them. We can assume that
	// only that subset will actually be needed during the apply phase.
	// That's similar to what we're doing with provider instances where we only
	// add their open/close nodes to the execgraph once at least one resource
	// instance refers to them, and any instances that aren't referred to from
	// anything non-ephemeral won't be opened during the apply phase regardless
	// of what the evaluator discovers.
	//
	// With that in mind, this planDesiredEphemeralResourceInstance function
	// should probably just focus on calling
	// [shared.OpenEphemeralResourceInstance] and not put anything in the
	// execution graph, and then we can arrange for
	// [execGraphBuilder.resourceInstanceFinalStateResult] to implicitly
	// call EphemeralResourceInstanceSubgraph on the first request for each
	// distinct ephemeral resource instance address so that the set of instances
	// we put in the execution graph is completely independent of the set of
	// instances we open during the planning phase. (At that point it's not
	// really honest for this method to be named "plan" anymore, and should
	// probably be called [planGlue.openDesiredEphemeralResourceInstance]
	// instead and to no longer return an [execgraph.ResultRef] at all.
	ret.PlaceholderValue = newVal
	return ret, diags
}
