// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package memory

import (
	"context"
	"fmt"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
	ctyjson "github.com/zclconf/go-cty/cty/json"

	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// This provider has only a single resource type, and so the implementation
// of that resource type is directly inside the provider's managed-resource-type
// related methods.

func resourceTypeSchema() providers.Schema {
	return providers.Schema{
		Block: &configschema.Block{
			Attributes: map[string]*configschema.Attribute{
				"value": {
					// "value" is the currently-stored value. This is unknown
					// during planning whenever a change is pending, and then
					// becomes known again during the apply phase once the
					// change is committed. If no change is pending then this
					// just retains the most recently-written value.
					Type:     cty.DynamicPseudoType,
					Computed: true,
				},

				"initial_value": {
					// "initial_value" is used to populate "value" during
					// initial creation if either there is no update requested
					// at the same time or if the requested update is derived
					// from a previously-stored value.
					//
					// For example, if using "add_value" to apply a numeric
					// offset to the previously-stored value then this should
					// be set to a number that the adjustment would be applied
					// to whenever there is no previous value to use.
					Type:     cty.DynamicPseudoType,
					Required: true,
					// This is write-only so that it's possible to set it from
					// an ephemeral resource, if needed, just like with
					// new_value.
					WriteOnly: true,
				},

				// Zero or one of the following attributes can be set to
				// request a change to the stored value. These arguments
				// are mutually-exclusive so that it's always unambiguous
				// whether a change is requested and what kind of change
				// is being requested.
				//
				// The "nullness" of these values must remain consistent
				// between plan and apply in order to actually cause a change
				// to happen, but the actual value of the one that is non-null
				// is allowed to change.
				//
				// These are both intentionally write-only so that they can be
				// set from an ephemeral resource when needed. It also means
				// OpenTofu will never show a diff relative to a previously-set
				// value, which would probably just confuse people.
				"new_value": {
					// "new_value" represents the most straightforward change:
					// just overwriting any existing value with
					Type:      cty.DynamicPseudoType,
					Optional:  true,
					WriteOnly: true,
				},
				"add_to_value": {
					// "add_to_value" represents adding the given number to
					// whatever number was previously stored. If the previous
					// value was not something that can convert to a number
					// then it's invalid to set this argument.
					//
					// Setting this to zero is treated as a write even though
					// the new value would not change, because the given
					// value may be ephemeral and so might be nonzero during
					// apply even if it was zero during planning.
					Type:      cty.Number,
					Optional:  true,
					WriteOnly: true,
				},
			},
		},
	}
}

// ValidateResourceConfig implements providers.Interface.
func (p *Provider) ValidateResourceConfig(_ context.Context, req providers.ValidateResourceConfigRequest) providers.ValidateResourceConfigResponse {
	var resp providers.ValidateResourceConfigResponse

	newValue := req.Config.GetAttr("new_value")
	addToValue := req.Config.GetAttr("add_to_value")
	if !newValue.IsNull() && !addToValue.IsNull() {
		resp.Diagnostics = resp.Diagnostics.Append(tfdiags.AttributeValue(
			tfdiags.Error,
			"Conflicting write arguments for memory",
			"Cannot set both \"new_value\" and \"add_to_value\" at the same time.",
			// We arbitrarily "blame" the add_to_value argument because that's
			// the least general of the two.
			cty.GetAttrPath("add_to_value"),
		))
	}

	return resp
}

// PlanResourceChange implements providers.Interface.
func (p *Provider) PlanResourceChange(_ context.Context, req providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse {
	var resp providers.PlanResourceChangeResponse
	// The following assumes that the caller previously called
	// ValidateResourceConfig and so we don't need to repeat any checks already
	// made by that function.

	if req.Config == cty.NilVal || req.Config.IsNull() {
		// We're planning to destroy the memory object then, and so we'll just
		// confirm to the caller that it's okay to do that.
		return resp
	}

	plannedNewValue := cty.DynamicVal
	if req.PriorState != cty.NilVal && !req.PriorState.IsNull() {
		plannedNewValue = req.PriorState.GetAttr("value")
	}

	// Due to ValidateResourceConfig's checks we can assume that at most one
	// of the following is non-null.
	newValue := req.Config.GetAttr("new_value")
	addToValue := req.Config.GetAttr("add_to_value")
	if !newValue.IsNull() || !addToValue.IsNull() {
		// The value will be finalized during the apply step. The unknown-ness
		// of this value signals to the apply step that a change was expected.
		plannedNewValue = cty.DynamicVal
	}

	resp.PlannedState = memoryInstanceObject(plannedNewValue)
	return resp
}

// ApplyResourceChange implements providers.Interface.
func (p *Provider) ApplyResourceChange(_ context.Context, req providers.ApplyResourceChangeRequest) providers.ApplyResourceChangeResponse {
	var resp providers.ApplyResourceChangeResponse
	// The following assumes that the caller previously called both
	// ValidateResourceConfig and PlanResourceChange and so we don't need to
	// repeat any checks previously made and the planned new state matches
	// what PlanResourceChange would've produced.

	if req.PlannedState == cty.NilVal || req.PlannedState.IsNull() {
		// We're applying a "destroy" plan, so we've nothing to do except
		// signal that it was successful.
		// We'll create a stub response value just to avoid duplicating
		// the type information here to return a correctly-typed null result.
		stubVal := memoryInstanceObject(cty.NullVal(cty.DynamicPseudoType))
		resp.NewState = cty.NullVal(stubVal.Type())
		return resp
	}

	var oldValue cty.Value
	if req.PriorState != cty.NilVal && !req.PriorState.IsNull() {
		oldValue = req.PriorState.GetAttr("value")
	} else {
		// If we're creating a "memory" for the first time then we use
		// initial_value as a placeholder for the old value, so that the
		// rest of the logic doesn't need to distinguish between
		// creating and updating.
		oldValue = req.Config.GetAttr("initial_value")
	}

	if plannedValue := req.PlannedState.GetAttr("value"); plannedValue.IsKnown() {
		// If we knew the new value during planning then that means we aren't
		// making a change at all, and so we'll just echo back whatever
		// we were given.
		resp.NewState = memoryInstanceObject(plannedValue)
		return resp
	}
	if !oldValue.IsKnown() {
		// This should not be possible: neither "value" nor "initial_value"
		// can be unknown during the apply phase because prior state is
		// always known and during the apply phase config is also wholly
		// known.
		resp.Diagnostics = resp.Diagnostics.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Inconsistent values in \"memory\" object",
			"Previous value for memory is unknown during apply. This is a bug in OpenTofu.",
		))
		return resp
	}

	// Due to ValidateResourceConfig's checks we can assume that at most one
	// of the following is non-null.
	wantNewValue := req.Config.GetAttr("new_value")
	wantAddToValue := req.Config.GetAttr("add_to_value")
	if !wantNewValue.IsNull() {
		resp.NewState = memoryInstanceObject(wantNewValue)
	} else if !wantAddToValue.IsNull() {
		// In this case the old value must be something we can convert to
		// a number that we'll add the argument value to.
		oldNum, err := convert.Convert(oldValue, cty.Number)
		if err != nil {
			resp.Diagnostics = resp.Diagnostics.Append(tfdiags.AttributeValue(
				tfdiags.Error,
				"Cannot add to non-numeric memory value",
				fmt.Sprintf("The \"add_to_value\" argument requires that the previous value be a number, so that the argument value can be added to it. The current memory value is %s.", oldValue.Type().FriendlyName()),
				cty.GetAttrPath("add_to_value"),
			))
			return resp
		}
		resp.NewState = memoryInstanceObject(oldNum.Add(wantAddToValue))
	} else {
		// No change at all, so the new value is equal to the old value.
		resp.NewState = memoryInstanceObject(oldValue)
	}
	return resp
}

func memoryInstanceObject(value cty.Value) cty.Value {
	return cty.ObjectVal(map[string]cty.Value{
		"value": value,
		// The write-only attributes must all be set to null, per the usual
		// rules for attributes of that kind.
		"initial_value": cty.NullVal(cty.DynamicPseudoType),
		"new_value":     cty.NullVal(cty.DynamicPseudoType),
		"add_to_value":  cty.NullVal(cty.Number),
	})
}

// ReadResource implements providers.Interface.
func (p *Provider) ReadResource(_ context.Context, req providers.ReadResourceRequest) providers.ReadResourceResponse {
	// There is no remote data associated with this resource type, so we just
	// echo back whatever we were given.
	return providers.ReadResourceResponse{
		NewState: req.PriorState,
		Private:  req.Private,
	}
}

// ImportResourceState implements providers.Interface.
func (p *Provider) ImportResourceState(context.Context, providers.ImportResourceStateRequest) providers.ImportResourceStateResponse {
	// Perhaps once OpenTofu supports import-by-resource-identity we can allow
	// importing with a value specified in there, but for now we just disallow
	// importing altogether.
	var resp providers.ImportResourceStateResponse
	resp.Diagnostics = resp.Diagnostics.Append(tfdiags.Sourceless(
		tfdiags.Error,
		"Cannot import \"memory\" object",
		"Importing into a memory object is not supported.",
	))
	return resp
}

// MoveResourceState implements providers.Interface.
func (p *Provider) MoveResourceState(_ context.Context, req providers.MoveResourceStateRequest) providers.MoveResourceStateResponse {
	// No other resource type can be converted to a "memory".
	var resp providers.MoveResourceStateResponse
	resp.Diagnostics = resp.Diagnostics.Append(tfdiags.Sourceless(
		tfdiags.Error,
		"Cannot convert to \"memory\" object",
		fmt.Sprintf("A resource instance of type %q cannot be converted into a \"memory\" object.", req.SourceTypeName),
	))
	return resp
}

// UpgradeResourceState implements providers.Interface.
func (p *Provider) UpgradeResourceState(_ context.Context, req providers.UpgradeResourceStateRequest) providers.UpgradeResourceStateResponse {
	var resp providers.UpgradeResourceStateResponse
	// This is awkward because we get given the prior state in JSON format
	// and need to convert it back to the cty representation here just to
	// satisfy the provider API. This doesn't achieve anything particularly
	// useful, though this _is_ where we might catch some problems if someone
	// did manual state surgery and made the JSON representation invalid.
	schema := resourceTypeSchema()
	v, err := ctyjson.Unmarshal(req.RawStateJSON, schema.Block.ImpliedType())
	if err != nil {
		resp.Diagnostics = resp.Diagnostics.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Invalid prior state for \"memory\" instance",
			fmt.Sprintf("Failed to decode prior state for \"memory\" object: %s.", tfdiags.FormatError(err)),
		))
		return resp
	}
	resp.UpgradedState = v
	return resp
}
