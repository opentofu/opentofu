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

// Read encapsulates the logic for reading data for a data resource instance.
//
// The caller must ensure that all of the provided values conform to the schema
// of the named resource type in the given provider, or the results are
// unspecified. [DataResourceType.LoadSchema] returns the expected schema.
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
func (rt *DataResourceType) Read(ctx context.Context, req *DataResourceReadRequest, dispAddr addrs.AbsResourceInstanceObject) (*DataResourceReadResponse, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	schema, moreDiags := rt.LoadSchema(ctx)
	diags = diags.Append(moreDiags)
	if diags.HasErrors() {
		return nil, diags
	}
	ty := schema.Block.ImpliedType().WithoutOptionalAttributesDeep()

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
		// cannot be updated-in-place, but unfortunately existing providers
		// do generate spurious "requires replace" signals for non-update
		// plans and so we need to just ignore them.
		log.Printf("[WARN] Ignoring nonsensical RequiresReplace values from provider %s while planning a non-update change for %s", rt.providerAddr, dispAddr)
		// We'll discard the meaningless extra info here just so that the
		// rest of the system can assume that this is populated only when it
		// actually needs to be acted on.
		resp.RequiresReplace = nil
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

// DataResourceReadRequest is the request type for [DataResourceType.Read].
type DataResourceReadRequest struct {
	// ConfigValue is a value representing the configuration for the
	// resource instance, which is typically the result of evaluating the
	// arguments in a block in the configuration.
	ConfigValue cty.Value

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

// DataResourceReadResponse is the response type for [DataResourceType.Read].
type DataResourceReadResponse struct {
	// TODO: Include some representation of a provider's "deferred" signal
	// in here, once we've updated our provider clients to support that,
	// and then update callers to handle responses with that set.

	// ConfigValue echoes back the value  given in the corresponding request
	// field, possibly with some normalization such as transforming an absent
	// value into null.
	ConfigValue cty.Value

	// Result represents the value returned by the provider, or a placeholder
	// result if DelayUntilApply is set.
	Result cty.Value

	// DelayedUntilApply is true if some other changes must be applied before
	// the requested resource instance can be read.
	//
	// When this is true, Result contains a placeholder value which has unknown
	// values in place of the results that the provider will populate once
	// the request is actually made.
	DelayedUntilApply bool

	// RequiredUpstreamChanges may be set when DelayedUntilApply is true, in
	// which case it describes a set of specific resource instance addresses
	// whose changes must be applied before we can make a real call to read this
	// data.
	//
	// Note that this can be empty even when DelayedUntilApply is set, because
	// not all "delays" are caused by resource instance changes. For example,
	// if the configuration includes a call to an impure function like
	// "timestamp" then the read would _always_ be delayed until the apply
	// phase, since that's when the timestamp would be decided.
	RequiredUpstreamChanges addrs.Set[addrs.AbsResourceInstance]
}
