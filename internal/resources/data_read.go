// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"
	"fmt"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"

	"github.com/opentofu/opentofu/internal/lang/marks"
)

type ProviderWithEncryption interface {
	ReadDataSourceEncrypted(ctx context.Context, req providers.ReadDataSourceRequest, path addrs.AbsResourceInstance, enc encryption.Encryption) providers.ReadDataSourceResponse
}

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
func (rt *DataResourceType) Read(ctx context.Context, req *DataResourceReadRequest, dispAddr addrs.AbsResourceInstanceObject, encryption encryption.Encryption) (*DataResourceReadResponse, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	var out *DataResourceReadResponse

	schema, schemaDiags := rt.LoadSchema(ctx)
	if schemaDiags.HasErrors() {
		// Should be caught during validation, so we don't bother with a pretty error here
		diags = diags.Append(schemaDiags)
		return nil, diags
	}

	configVal, pvm := req.ConfigValue.UnmarkDeepWithPaths()

	providerReq := providers.ReadDataSourceRequest{
		TypeName:     rt.typeName,
		Config:       configVal,
		ProviderMeta: cty.NullVal(cty.DynamicPseudoType),
	}

	var providerResp providers.ReadDataSourceResponse
	if tfp, ok := rt.client.(ProviderWithEncryption); ok {
		// handling terraform_remote_state with builtin tf provider
		providerResp = tfp.ReadDataSourceEncrypted(ctx, providerReq, req.ResourceAddress, encryption)
	} else {
		providerResp = rt.client.ReadDataSource(ctx, providerReq)
	}

	// TODO Attach config to response diagnostics using InConfigBody
	// FIXME: Our "contextual diagnostics" mechanism, where the callee provides
	// an attribute path and then the caller discovers a suitable source range
	// for each diagnostic based on information in the body, can only work
	// when we have direct access to a [hcl.Body], but we intentionally
	// abstracted that away here. We'll need to find a different design for
	// contextual diagnostics that can work through the [exprs.Valuer]
	// abstraction to make a best effort to interpret attribute paths against
	// whatever the valuer was evaluating.
	diags = diags.Append(providerResp.Diagnostics)

	newVal := providerResp.State
	if newVal == cty.NilVal {
		// This can happen with incompletely-configured mocks. We'll allow it
		// and treat it as an alias for a properly-typed null value.
		newVal = cty.NullVal(schema.Block.ImpliedType())
	}

	for _, err := range newVal.Type().TestConformance(schema.Block.ImpliedType()) {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Provider produced invalid object",
			fmt.Sprintf(
				"Provider %q produced an invalid value for %s.\n\nThis is a bug in the provider, which should be reported in the provider's own issue tracker.",
				rt.providerAddr.String(), tfdiags.FormatErrorPrefixed(err, req.ResourceAddress.String()),
			),
		))
	}

	if newVal.IsNull() {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Provider produced null object",
			fmt.Sprintf(
				"Provider %q produced a null value for %s.\n\nThis is a bug in the provider, which should be reported in the provider's own issue tracker.",
				rt.providerAddr.String(), req.ResourceAddress.String(),
			),
		))
	}

	if !newVal.IsNull() && !newVal.IsWhollyKnown() {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Provider produced invalid object",
			fmt.Sprintf(
				"Provider %q produced a value for %s that is not wholly known.\n\nThis is a bug in the provider, which should be reported in the provider's own issue tracker.",
				rt.providerAddr.String(), req.ResourceAddress.String(),
			),
		))

		// We'll still save the object, but we need to eliminate any unknown
		// values first because we can't serialize them in the state file.
		// Note that this may cause set elements to be coalesced if they
		// differed only by having unknown values, but we don't worry about
		// that here because we're saving the value only for inspection
		// purposes; the error we added above will halt the graph walk.
		newVal = cty.UnknownAsNull(newVal)
	}

	if len(pvm) > 0 {
		newVal = newVal.MarkWithPaths(pvm)
	}

	out = &DataResourceReadResponse{
		Result: newVal,

		Status: states.ObjectReady,
	}

	// TODO is this sensible?
	out.SensitivePaths = make([]cty.Path, 0, len(pvm))
	for _, p := range pvm {
		for mark := range p.Marks {
			if mark != marks.Sensitive {
				continue
			}
			out.SensitivePaths = append(out.SensitivePaths, p.Path)
		}
	}

	return out, diags
}

// DataResourceReadRequest is the request type for [DataResourceType.Read].
type DataResourceReadRequest struct {
	// ResourceAddress is used mostly for diagnostics
	ResourceAddress addrs.AbsResourceInstance

	// ConfigValue is a value representing the configuration for the
	// resource instance, which is typically the result of evaluating the
	// arguments in a block in the configuration.
	ConfigValue cty.Value
}

// DataResourceReadResponse is the response type for [DataResourceType.Read].
type DataResourceReadResponse struct {
	// TODO: Include some representation of a provider's "deferred" signal
	// in here, once we've updated our provider clients to support that,
	// and then update callers to handle responses with that set.

	// Result represents the value returned by the provider, or a placeholder
	// result if DelayUntilApply is set.
	Result cty.Value

	// SensitivePaths is an array of paths to mark as sensitive when decoding.
	SensitivePaths []cty.Path

	// Status represents the "readiness" of the object as of the last time it was updated.
	Status states.ObjectStatus
}
