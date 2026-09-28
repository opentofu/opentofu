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
	"github.com/opentofu/opentofu/internal/tfdiags"
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
	// TODO I can do whatever I want here!
	// plan_data is the only file using this method

	// Things the OG had that I need, and why:
	// - encryption.Encryption (for ReadDataSourceEncrypted)
	// That's it!
	// Looks like that's obtained during EvalContext() thru ContextGraphWalker,
	// which in turn gets set in graphWalker for *Context
	// Which is set by the NewContext function in the tofu package; one of the options is encryption

	var diags tfdiags.Diagnostics
	var out *DataResourceReadResponse

	schema, schemaDiags := rt.LoadSchema(ctx)
	if schemaDiags.HasErrors() {
		// Should be caught during validation, so we don't bother with a pretty error here
		diags = diags.Append(schemaDiags)
		return nil, diags
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

	configVal, pvm := req.ConfigValue.UnmarkDeepWithPaths()

	providerReq := providers.ReadDataSourceRequest{
		TypeName:     rt.typeName,
		Config:       configVal,
		ProviderMeta: providerMetaVal,
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

	// TODO this data resource response doesn't look right, only Result is actually set??? Where do the other values come from?
	out = &DataResourceReadResponse{
		ConfigValue:             cty.Value{},
		Result:                  newVal,
		DelayedUntilApply:       false,
		RequiredUpstreamChanges: addrs.Set[addrs.AbsResourceInstance]{},
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
