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
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func (p *planGlue) planDesiredDataResourceInstance(ctx context.Context, inst *eval.DesiredResourceInstance) (*resourceInstanceObject, tfdiags.Diagnostics) {
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

	validateDiags := p.planCtx.providers.ValidateResourceConfig(ctx, inst.Provider, inst.ResourceMode, inst.ResourceType, inst.ConfigVal)
	diags = diags.Append(validateDiags)
	if diags.HasErrors() {
		return ret, diags
	}

	if inst.ProviderInstance == nil {
		// TODO: Record that this was deferred because we don't yet know which
		// provider instance it belongs to.
		return ret, diags
	}

	// The equivalent of "refreshing" a data resource is just to discard it
	// completely, because we only retain the previous result in state snapshots
	// to support unusual situations like "tofu console"; it's not expected that
	// data resource instances persist between rounds and they cannot because
	// the protocol doesn't include any way to "upgrade" them if the provider
	// schema has changed since previous round.
	// FIXME: State is still using the weird old representation of provider
	// instance addresses, so we can't actually populate the provider instance
	// arguments properly here.
	p.planCtx.refreshedState.SetResourceInstanceCurrent(inst.Addr, nil, addrs.AbsProviderConfig{}, inst.ProviderInstance.Key)

	// TODO: If the config value is not wholly known, or if any resource
	// instance in inst.RequiredResourceInstances already has a planned change,
	// then plan to read this during the apply phase and set ret.PlannedChange
	// to an object using the [plans.Read] action, without writing a new
	// object into the refreshed state yet.

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

	resp := providerClient.ReadDataSource(ctx, providers.ReadDataSourceRequest{
		TypeName: inst.ResourceType,
		Config:   inst.ConfigVal,

		// TODO: ProviderMeta is a rarely-used feature that only really makes
		// sense when the module and provider are both written by the same
		// party and the module author is using the provider as a way to
		// transport module usage telemetry. We should decide whether we want
		// to keep supporting that, and if so design a way for the relevant
		// meta value to get from the evaluator into here.
		ProviderMeta: cty.NullVal(cty.DynamicPseudoType),
	})
	diags = diags.Append(resp.Diagnostics)
	if resp.Diagnostics.HasErrors() {
		return ret, diags
	}
	// TODO: Verify that the object the provider returned is a valid completion
	// of the configuration value.

	// TODO: Update the refreshed state to match what we've just read.

	// Since we've already read the data source during the planning phase,
	// we don't need a PlannedChange here and can instead just use the result
	// as the PlaceholderValue.
	ret.PlaceholderValue = resp.State

	return ret, diags
}

func (p *planGlue) planOrphanDataResourceInstance(_ context.Context, addr addrs.AbsResourceInstance, state *states.ResourceInstanceObjectFullSrc) (*resourceInstanceObject, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// An orphan data object is always just discarded completely, because
	// OpenTofu retains them only for esoteric uses like the "tofu console"
	// command: they are not actually expected to persist between rounds.
	p.planCtx.refreshedState.SetResourceInstanceObjectFull(addr, states.NotDeposed, nil)

	return &resourceInstanceObject{
		Addr:             addr.CurrentObject(),
		Dependencies:     addrs.MakeSet[addrs.AbsResourceInstanceObject](),
		Provider:         state.ProviderInstanceAddr.Config.Config.Provider,
		PlaceholderValue: cty.NullVal(cty.DynamicPseudoType),
	}, diags
}
