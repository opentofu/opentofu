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
	"github.com/opentofu/opentofu/internal/plans/objchange"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/resources"
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

	requiredChanges := addrs.CollectSet(objchange.PrereqChangesForValue(inst.ConfigVal))
	if len(requiredChanges) != 0 || !inst.ConfigVal.IsWhollyKnown() {
		// The configuration for this data resource instance is relying on
		// values that won't be finalized until the apply phase, so we'll need
		// to delay reading this until the apply phase.
		// Note that this is not "deferral" in the sense of "deferred actions":
		// that terminology refers to skipping any actions for a particular
		// resource instance _even in the apply phase_ of this round, whereas
		// "delaying" here just means that it gets read in the apply phase
		// instead of during the plan phase.
		//
		// TODO: We should also used [derivedFromDeferredVal] somewhere in this
		// function to handle when this is derived from something that _is_
		// being completely deferred in this round, in which case we must also
		// defer reading this data resource instance to a future round.
		ret, moreDiags := p.planDelayedDataResourceInstance(ctx, inst, ret)
		diags = diags.Append(moreDiags)
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

// planDelayedDataResourceInstance deals with the situation where a data
// resource instance has a configuration that includes values that won't be
// finalized and known until the apply phase.
//
// In that case we produce a planned action to read the resource instance during
// the apply phase, and then use a marked placeholder for ongoing evaluation.
//
// This is called by [planDesiredDataResourceInstance] after it has already
// partially-constructed the [resourceInstanceObject] to return, so that's
// passed in as "ret" and then modified in-place before returning it. The
// caller is expected to then just return that result verbatim.
func (p *planGlue) planDelayedDataResourceInstance(ctx context.Context, inst *eval.DesiredResourceInstance, ret *resourceInstanceObject) (*resourceInstanceObject, tfdiags.Diagnostics) {

	resourceType := resources.NewDataResourceType(inst.Provider, inst.Addr.Resource.Resource.Type, providerClient)
	schema, schemaDiags := resourceType.LoadSchema(ctx)
	if schemaDiags.HasErrors() {
		// We don't return the schema-loading diagnostics directly here because
		// they should have already been returned by earlier code, but we do
		// return a more specific error to make it clear that this specific
		// resource instance was unplannable because of the problem.
		diags = diags.Append(tfdiags.AttributeValue(
			tfdiags.Error,
			"Resource type schema unavailable",
			fmt.Sprintf(
				"Cannot plan %s because provider %s failed to return the schema for its resource type %q.",
				inst.Addr, inst.Provider, inst.Addr.Resource.Resource.Type,
			),
			nil, // this error belongs to the whole resource config
		))
		return ret, diags
	}

	// TODO: plan to read this during the apply phase and set ret.PlannedChange
	// to an object using the [plans.Read] action, without writing a new
	// object into the refreshed state yet.
	//
	// In this case the placeholder value for any computed attribute in
	// the object we return should also be annotated with
	// [objchange.ValuePendingChange] using this data resource instance's
	// address, so that any downstream data resource instance that derives
	// from the results of this one will also get delayed to the apply
	// phase.
	panic("TODO: delaying of data resource instances to the apply phase not implemented yet")

	// TODO: It would be nice to also report the requiredChanges set in a way
	// that would allow us to enumerate in the UI exactly which managed
	// resource instances are blocking the reading of this data resource
	// instance, but that's less important than making sure the generated
	// execution graph respects those dependencies.
	//
	// When we do this note that there can be unknown values in the config
	// even when there aren't any required changes, such as if for some
	// reason the data resource configuration includes a call to an
	// impure function like "timestamp", so we should make sure the UI still
	// does something sensible when requiredChanges is empty.

}

func (p *planGlue) planOrphanDataResourceInstance(_ context.Context, addr addrs.AbsResourceInstance, state *states.ResourceInstanceObjectFullSrc) (*resourceInstanceObject, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// An orphan data object is always just discarded completely, because
	// OpenTofu retains them only for esoteric uses like the "tofu console"
	// command: they are not actually expected to persist between rounds.
	p.planCtx.refreshedState.RemoveResourceInstanceObjectFull(addr.CurrentObject(), state.ProviderInstanceAddr)

	return &resourceInstanceObject{
		Addr:             addr.CurrentObject(),
		Dependencies:     addrs.MakeSet[addrs.AbsResourceInstanceObject](),
		Provider:         state.ProviderInstanceAddr.Config.Config.Provider,
		PlaceholderValue: cty.NullVal(cty.DynamicPseudoType),
	}, diags
}
