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

func (p *planContext) planDesiredDataResourceInstance(ctx context.Context, inst *eval.DesiredResourceInstance, oracle *eval.PlanningOracle) (cty.Value, tfdiags.Diagnostics) {
	// Regardless of outcome we'll always report that we completed planning.
	defer p.reportResourceInstancePlanCompletion(inst.Addr)
	var diags tfdiags.Diagnostics

	if inst.ProviderInstance == nil {
		// TODO: Record that this was deferred because we don't yet know which
		// provider instance it belongs to.
		return deferredVal(cty.DynamicVal), diags
	}

	// TODO: There are various other reasons why we might need to defer planning
	// this until a future plan/apply round.

	// The equivalent of "refreshing" a data resource is just to discard it
	// completely, because we only retain the previous result in state snapshots
	// to support unusual situations like "tofu console"; it's not expected that
	// data resource instances persist between rounds and they cannot because
	// the protocol doesn't include any way to "upgrade" them if the provider
	// schema has changed since previous round.
	// FIXME: State is still using the weird old representation of provider
	// instance addresses, so we can't actually populate the provider instance
	// arguments properly here.
	p.refreshedState.SetResourceInstanceCurrent(inst.Addr, nil, addrs.AbsProviderConfig{}, inst.ProviderInstance.Key)

	// TODO: If the config value is not wholly known, or if any resource
	// instance in inst.RequiredResourceInstances already has a planned change,
	// then plan to read this during the apply phase and return a "proposed new
	// value" for use during the planning phase.

	providerClient, moreDiags := p.providerInstances.ProviderClient(ctx, *inst.ProviderInstance, oracle, p)
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
		return cty.DynamicVal, diags
	}

	resp := providerClient.ReadDataSource(ctx, providers.ReadDataSourceRequest{
		TypeName: inst.Addr.Resource.Resource.Type,
		Config:   inst.ConfigVal,
		// TODO: Add ProviderMeta information to eval.DesiredResourceInstance
		// and then pass it on to the provider here.
	})
	diags = diags.Append(resp.Diagnostics)
	if resp.Diagnostics.HasErrors() {
		return cty.DynamicVal, diags
	}

	// TODO: Implement
	panic("unimplemented")
}

func (p *planContext) planOrphanDataResourceInstance(_ context.Context, addr addrs.AbsResourceInstance, state *states.ResourceInstance, _ *eval.PlanningOracle) tfdiags.Diagnostics {
	// Regardless of outcome we'll always report that we completed planning.
	defer p.reportResourceInstancePlanCompletion(addr)
	var diags tfdiags.Diagnostics

	// An orphan data resource is always just discarded completely, because
	// OpenTofu retains them only for esoteric uses like the "tofu console"
	// command: they are not actually expected to persist between rounds.
	//
	// FIXME: We can't actually populate the provider instance address here
	// because in our current model it's split awkwardly across *states.Resource
	// and *states.ResourceInstance, and we only have the latter here.
	p.refreshedState.SetResourceInstanceCurrent(addr, nil, addrs.AbsProviderConfig{}, state.ProviderKey)

	return diags
}
