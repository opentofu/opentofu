// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/plans/objchange"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// PlanChanges encapsulates the logic for deciding what changes, if any, to make
// to a managed resource instance object by comparing its current and desired
// states.
//
// The caller must ensure that all of the provided values conform to the schema
// of the named resource type in the given provider, or the results are
// unspecified. [ManagedResourceType.LoadSchema] returns the expected schema.
//
// The dispAddr argument is used only to name the corresponding resource
// instance object when generating diagnostics. If no diagnostics are returned
// then that argument is completely ignored. Some of the returned diagnostics
// can be config-contextual diagnostics expecting to be elaborated by calling
// [tfdiags.Diagnostics.InConfigBody] with the configuration body that the
// desired value was built from, if any.
//
// If the returned diagnostics contains errors then the response object might
// either be nil or be a partial description of the invalid plan, depending on
// the nature of the failure. Callers should use defensive programming
// techniques if interacting with a partial response associated with an error.
func (rt *ManagedResourceType) PlanChanges(ctx context.Context, req *ManagedResourcePlanRequest, dispAddr addrs.AbsResourceInstanceObject) (*ManagedResourcePlanResponse, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	schema, moreDiags := rt.LoadSchema(ctx)
	diags = diags.Append(moreDiags)
	if diags.HasErrors() {
		return nil, diags
	}
	ty := schema.Block.ImpliedType().WithoutOptionalAttributesDeep()

	var currentVal, desiredVal cty.Value
	var currentPrivate []byte
	if req.Current.Value != cty.NilVal {
		currentVal = req.Current.Value
		currentPrivate = req.Current.Private
	} else {
		currentVal = cty.NullVal(ty)
	}
	if req.DesiredValue != cty.NilVal {
		desiredVal = req.DesiredValue
	} else {
		desiredVal = cty.NullVal(ty)
	}
	var providerMetaVal cty.Value
	if req.ProviderMetaValue != cty.NilVal {
		providerMetaVal = req.ProviderMetaValue
	} else {
		// Leaving the ProviderMeta field unpopulated in the provider
		// request makes some provider clients crash, so we'll substitute an
		// untyped null just to avoid that.
		providerMetaVal = cty.NullVal(cty.DynamicPseudoType)
	}

	// proposedVal is essentially a default answer for how to merge currentVal
	// and desiredVal, which providers are allowed to use as a shortcut in
	// their planning logic for simple cases where no special planning behavior
	// is needed. Providers are allowed to ignore this value completely and
	// implement their own merging logic though, as long as the result conforms
	// to the rules that [objchange.AssertPlanValid] enforces.
	var proposedVal cty.Value
	if !desiredVal.IsNull() {
		proposedVal = objchange.ProposedNew(schema.Block, currentVal, desiredVal)
	} else {
		proposedVal = cty.NullVal(ty)
	}

	currentValUnmarked, currentMarks := currentVal.UnmarkDeepWithPaths()
	desiredValUnmarked, desiredMarks := desiredVal.UnmarkDeepWithPaths()
	proposedValUnmarked, _ := proposedVal.UnmarkDeep()
	providerMetaValUnmarked, _ := providerMetaVal.UnmarkDeep()

	var resp providers.PlanResourceChangeResponse
	if !desiredValUnmarked.IsNull() || rt.providerCanPlanDestroy(ctx) {
		resp = rt.client.PlanResourceChange(ctx, providers.PlanResourceChangeRequest{
			TypeName:         rt.typeName,
			PriorState:       currentValUnmarked,
			PriorPrivate:     currentPrivate,
			Config:           desiredValUnmarked,
			ProposedNewState: proposedValUnmarked,
			ProviderMeta:     providerMetaValUnmarked,
		})
		diags = diags.Append(resp.Diagnostics)
		if resp.Diagnostics.HasErrors() {
			return nil, diags
		}
	} else {
		// For older providers that are not capable of generating destroy plans
		// themselves, we generate a synthetic destroy plan.
		resp = rt.fakeDestroyPlan(ty)
	}

	plannedValUnmarked := resp.PlannedState
	plannedPrivate := resp.PlannedPrivate
	if errs := objchange.AssertPlanValid(schema.Block, currentValUnmarked, desiredValUnmarked, plannedValUnmarked); len(errs) > 0 {
		if resp.LegacyTypeSystem {
			// The shimming of the old type system in the legacy SDK is not precise
			// enough to pass this consistency check, so we'll give it a pass here,
			// but we will generate a warning about it so that we are more likely
			// to notice in the logs if an inconsistency beyond the type system
			// leads to a downstream provider failure.
			var buf strings.Builder
			fmt.Fprintf(&buf,
				"[WARN] Provider %q produced an invalid plan for %s, but we are tolerating it because it is using the legacy plugin SDK.\n    The following problems may be the cause of any confusing errors from downstream operations:",
				rt.providerAddr, dispAddr,
			)
			for _, err := range errs {
				fmt.Fprintf(&buf, "\n      - %s", tfdiags.FormatError(err))
			}
			log.Print(buf.String())
		} else {
			for _, err := range errs {
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					"Provider produced invalid plan",
					fmt.Sprintf(
						"Provider %q planned an invalid value for %s.\n\nThis is a bug in the provider, which should be reported in the provider's own issue tracker.",
						rt.providerAddr, tfdiags.FormatErrorPrefixed(err, dispAddr.String()),
					),
				))
			}
			return nil, diags
		}
	}
	if len(resp.RequiresReplace) != 0 && (currentVal.IsNull() || desiredVal.IsNull()) {
		// RequiresReplace is only applicable when the plan request had both
		// a current and a desired value, because it specifies attributes that
		// cannot be updated-in-place.
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Provider produced invalid plan",
			fmt.Sprintf(
				"Provider %s reported that a create or delete plan for %s has changes that require replacement, but replacement is only valid as a modification of update-in-place.\n\nThis is a bug in the provider, which should be reported in the provider's own issue tracker.",
				rt.providerAddr, dispAddr,
			),
		))
	}

	// FIXME: plannedVal also needs sensitive marks added to it based on the
	// static attribute flags in the resource type schema.
	plannedVal := plannedValUnmarked.MarkWithPaths(currentMarks).MarkWithPaths(desiredMarks)

	return &ManagedResourcePlanResponse{
		Current: ValueWithPrivate{
			Value:   currentVal,
			Private: currentPrivate,
		},
		DesiredValue: desiredVal,
		Planned: ValueWithPrivate{
			Value:   plannedVal,
			Private: plannedPrivate,
		},
		RequiresReplace: resp.RequiresReplace,
	}, diags
}

// ValidateFinalPlan compares two planned values returned by calls to
// [ManagedResourceType.PlanChanges] -- typically comparing the initial plan
// found during the planning phase with the final plan decided during the apply
// phase -- and returns diagnostics if the two differ in any way that is not
// allowed by the resource instance object lifecycle rules.
//
// dispAddr is used only as part of any returned diagnostic messages, to explain
// which object had an invalid final plan.
func (rt *ManagedResourceType) ValidateFinalPlan(ctx context.Context, initialPlannedValue, finalPlannedValue cty.Value, dispAddr addrs.AbsResourceInstanceObject) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	schema, moreDiags := rt.LoadSchema(ctx)
	diags = diags.Append(moreDiags)
	if diags.HasErrors() {
		return diags
	}

	initialValueUnmarked, _ := initialPlannedValue.UnmarkDeep()
	finalValueUnmarked, _ := finalPlannedValue.UnmarkDeep()
	for _, err := range objchange.AssertObjectCompatible(schema.Block, initialValueUnmarked, finalValueUnmarked) {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Provider produced inconsistent final plan",
			fmt.Sprintf(
				"When expanding the plan for %s to include new values learned so far during apply, provider %q produced an invalid new value for %s.\n\nThis is a bug in the provider, which should be reported in the provider's own issue tracker.",
				dispAddr, rt.providerAddr, tfdiags.FormatError(err),
			),
		))
	}
	return diags
}

// fakeDestroyPlan is used instead of [providers.Interface.PlanResourceChange]
// if planning to destroy an existing object and the provider has not announced
// that it is capable of producing such a plan itself.
//
// This situation exists because the original provider protocol only expected
// providers to participate in planning "create" or "update" changes, with
// "delete" ones always generated synthetically inside the runtime. That was
// later generalized, but existing providers would crash if asked to plan with
// a null desired state and so providers are expected to opt-in using the
// capabilities system.
func (rt *ManagedResourceType) fakeDestroyPlan(ty cty.Type) providers.PlanResourceChangeResponse {
	return providers.PlanResourceChangeResponse{
		PlannedState: cty.NullVal(ty),
	}
}

// ManagedResourcePlanRequest is the request type for [ManagedResourceType.PlanChanges].
type ManagedResourcePlanRequest struct {
	// Current is a value representing the current state of the object, bundled
	// with an arbitrary byte array that was associated with that value by
	// the provider that previously generated it.
	//
	// Providers sometimes use the "private" blob to track additional metadata
	// that is not exposed as part of the value but is still needed to track
	// the object between plan/apply rounds.
	//
	// This field is typically set to the result of "refreshing" the object
	// that was saved at the end of the previous apply phase, in which case
	// the Private field must also match the blob returned from that refresh
	// operation.
	//
	// When planning to create a new object, this should be set to the zero
	// value of [ValueWithPrivate].
	Current ValueWithPrivate

	// DesiredValue is a value representing the desired state for the
	// object, which is typically the result of evaluating the arguments
	// in a block in the configuration.
	//
	// There is no "private" counterpart to this one because it is evaluated
	// fresh from the configuration each time, rather than being generated
	// by a provider.
	//
	// This field is typically set to a value obtained by evaluating a resource
	// block in the configuration. When planning to destroy an existing object,
	// this should be set to the zero value of [cty.Value], which is
	// [cty.NilVal].
	DesiredValue cty.Value

	// ProviderMetaValue is an optional value declared in the same module
	// where the associated resource was declared, which should be sent
	// to the provider as part of any planning request.
	//
	// This is a rarely-used feature that only really makes sense when a
	// module is written by the same entity that owns a provider it uses,
	// in which case the module author might want to use the provider as
	// a covert channel for collecting usage statistics about the module.
	//
	// When no metadata was provided for this provider in the current module,
	// this should be set to the zero value of [cty.Value], which is
	// [cty.NilVal].
	ProviderMetaValue cty.Value
}

// ManagedResourcePlanResponse is the response type for [ManagedResourceType.PlanChanges].
type ManagedResourcePlanResponse struct {
	// TODO: Include some representation of a provider's "deferred" signal
	// in here, once we've updated our provider clients to support that,
	// and then update callers to handle responses with that set.

	// Current echoes back the value given in the corresponding request field,
	// possibly with some normalization such as transforming an absent value
	// into null.
	Current ValueWithPrivate

	// DesiredValue echoes back the value  given in the corresponding request
	// field, possibly with some normalization such as transforming an absent
	// value into null.
	DesiredValue cty.Value

	// Planned has a prediction for what value will be associated with
	// this resource instance object after applying the planned change, along
	// with an optional opaque byte array that must be sent back to the
	// provider verbatim if this planned change is applied.
	//
	// The value typically includes unknown values as placeholders for specific
	// values that the provider cannot predict, such as opaque unique
	// identifiers selected by the remote system only once an object has
	// been created.
	//
	// Any part of the value that is not unknown is required to be identical
	// in the final object returned after applying the planned change, and so
	// it's reasonable to use this value when evaluating downstream expressions
	// that refer to a symbol representing this resource instance object.
	//
	// If the plan is to destroy the object, this is set to the zero value of
	// [ValueWithPrivate]. Otherwise, the caller must compare the value with
	// the request's "Current" value to determine whether any changes are
	// actually needed, taking no action at all if this value equals the
	// current value.
	Planned ValueWithPrivate

	// RequiresReplace describes paths within the planned value whose changes
	// require this change to be handled as a "replace" rather than as an
	// in-place update.
	//
	// If this collection is not empty then this change must be applied across
	// two separate [ApplyManagedResourceChange] calls, where one destroys the
	// prior object and the other creates a new object using the value from the
	// Planned field.
	//
	// If this collection is zero-length then this change should instead be
	// applied with only a single call to [ApplyManagedResourceChange].
	RequiresReplace []cty.Path
}

func (rt *ManagedResourceType) providerCanPlanDestroy(ctx context.Context) bool {
	// FIXME: Can we capture this somewhere else so that we don't need to
	// pull the whole schema again here? It's not a huge deal in practice
	// because the main implementations of [providers.Interface] do caching
	// of the schema result anyway, but the current factoring of this code
	// makes it hard to encapsulate this behavior nicely.

	resp := rt.client.GetProviderSchema(ctx)
	if resp.Diagnostics.HasErrors() {
		// If the provider can't return schema at all then something else is
		// going to go wrong soon enough anyway, and so we'll just return a
		// conservative default.
		return false
	}
	return resp.ServerCapabilities.PlanDestroy
}
