// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package encryption

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/encryption/config"
	"github.com/opentofu/opentofu/internal/lang"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
	"github.com/opentofu/opentofu/internal/encryption/registry"
	"github.com/zclconf/go-cty/cty"
)

// setupKeyProviders sets up the key providers for encryption. It returns a list of diagnostics if any of the key providers
// are invalid.
func (e *targetBuilder) setupKeyProviders() hcl.Diagnostics {
	var diags hcl.Diagnostics

	e.keyValues = make(map[string]map[string]cty.Value)

	for _, keyProviderConfig := range e.cfg.KeyProviderConfigs {
		diags = append(diags, e.setupKeyProvider(keyProviderConfig, nil)...)
	}

	// Regenerate the context now that the key provider is loaded
	kpMap := make(map[string]cty.Value)
	for name, kps := range e.keyValues {
		kpMap[name] = cty.ObjectVal(kps)
	}
	e.ctx.Variables["key_provider"] = cty.ObjectVal(kpMap)

	return diags
}

func (e *targetBuilder) setupKeyProvider(cfg config.KeyProviderConfig, stack []config.KeyProviderConfig) hcl.Diagnostics {
	// Ensure cfg.Type is in keyValues, if it isn't then add it in preparation for the next step
	if _, ok := e.keyValues[cfg.Type]; !ok {
		e.keyValues[cfg.Type] = make(map[string]cty.Value)
	}

	// Check if we have already setup this Descriptor (due to dependency loading)
	// if we've already setup this key provider, then we don't need to do it again
	// and we can return early
	if _, ok := e.keyValues[cfg.Type][cfg.Name]; ok {
		return nil
	}

	// Mark this key provider as partially handled.  This value will be replaced below once it is actually known.
	// The goal is to allow an early return via the above if statement to prevent duplicate errors if errors are encountered in the key loading stack.
	e.keyValues[cfg.Type][cfg.Name] = cty.UnknownVal(cty.DynamicPseudoType)

	// Check for circular references, this is done by inspecting the stack of key providers
	// that are currently being setup. If we find a key provider in the stack that matches
	// the current key provider, then we have a circular reference and we should return an error
	// to the user.
	for _, s := range stack {
		if s == cfg {
			addr, diags := keyprovider.NewAddr(cfg.Type, cfg.Name)
			diags = diags.Append(
				&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Circular reference detected",
					// TODO add the stack trace to the detail message
					Detail: fmt.Sprintf("Can not load %q due to circular reference", addr),
				},
			)
			return diags
		}
	}
	stack = append(stack, cfg)

	// Pull the meta key out for error messages and meta storage
	metakey, diags := cfg.Addr()
	if diags.HasErrors() {
		return diags
	}

	// Lookup the KeyProviderDescriptor from the registry
	id := keyprovider.ID(cfg.Type)
	keyProviderDescriptor, err := e.reg.GetKeyProviderDescriptor(id)
	if err != nil {
		if errors.Is(err, &registry.KeyProviderNotFoundError{}) {
			return diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Unknown key_provider type",
				Detail:   fmt.Sprintf("Can not find %q", cfg.Type),
			})
		}
		return diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  fmt.Sprintf("Error fetching key_provider %q", cfg.Type),
			Detail:   err.Error(),
		})
	}

	// Now that we know we have the correct Descriptor, we can decode the configuration
	// and build the KeyProvider
	keyProviderConfig := keyProviderDescriptor.ConfigStruct()

	// Locate all the dependencies
	deps, varDiags := gohcl.VariablesInBody(cfg.Body, keyProviderConfig)
	diags = append(diags, varDiags...)
	if diags.HasErrors() {
		return diags
	}

	// lang.References is going to fail parsing key_provider deps
	// so we filter them out in nonKeyProviderDeps.
	var nonKeyProviderDeps []hcl.Traversal

	// Setting up key providers from deps.
	for _, dep := range deps {
		// Key Provider references should be in the form key_provider.type.name
		if len(dep) != 3 {
			nonKeyProviderDeps = append(nonKeyProviderDeps, dep)
			continue
		}

		// This will always be a TraverseRoot, panic is OK if that's not the case
		depRoot := (dep[0].(hcl.TraverseRoot)).Name
		if depRoot != "key_provider" {
			nonKeyProviderDeps = append(nonKeyProviderDeps, dep)
			continue
		}
		depTypeAttr, typeOk := dep[1].(hcl.TraverseAttr)
		depNameAttr, nameOk := dep[2].(hcl.TraverseAttr)

		if !typeOk || !nameOk {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid Key Provider expression format",
				Detail:   "Expected key_provider.<type>.<name>",
				Subject:  dep.SourceRange().Ptr(),
			})
			continue
		}

		depType := depTypeAttr.Name
		depName := depNameAttr.Name

		kpc, ok := e.cfg.GetKeyProvider(depType, depName)
		if !ok {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Undefined Key Provider",
				Detail:   fmt.Sprintf("Key provider %s.%s is missing from the encryption configuration.", depType, depName),
				Subject:  dep.SourceRange().Ptr(),
			})
			continue
		}

		depDiags := e.setupKeyProvider(kpc, stack)
		diags = append(diags, depDiags...)
	}
	if diags.HasErrors() {
		// We should not continue now if we have any diagnostics that are errors
		// as we may end up in an inconsistent state.
		// The reason we collate the diags here and then show them instead of showing them as they arise
		// is to ensure that the end user does not have to play whack-a-mole with the errors one at a time.
		return diags
	}

	refs, refDiags := lang.References(addrs.ParseRef, nonKeyProviderDeps)
	diags = append(diags, refDiags.ToHCL()...)
	if diags.HasErrors() {
		return diags
	}

	evalCtx, evalDiags := e.staticEval.EvalContextWithParent(e.ctx, configs.StaticIdentifier{
		Module:    addrs.RootModule,
		Subject:   fmt.Sprintf("encryption.key_provider.%s.%s", cfg.Type, cfg.Name),
		DeclRange: e.cfg.DeclRange,
	}, refs)
	diags = append(diags, evalDiags...)
	if diags.HasErrors() {
		return diags
	}

	// Initialize the Key Provider
	decodeDiags := gohcl.DecodeBody(cfg.Body, evalCtx, keyProviderConfig)
	diags = append(diags, decodeDiags...)
	if diags.HasErrors() {
		return diags
	}

	// Build the Key Provider from the configuration
	keyProvider, keyMetaIn, err := keyProviderConfig.Build()
	if err != nil {
		return append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Unable to build encryption key data",
			Detail:   fmt.Sprintf("%s failed with error: %s", metakey, err.Error()),
		})
	}

	// Add the metadata
	if meta, ok := e.keyProviderMetadata[metakey]; ok {
		err := json.Unmarshal(meta, keyMetaIn)
		if err != nil {
			return append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Unable to decode encrypted metadata (did you change your encryption config?)",
				Detail:   fmt.Sprintf("metadata decoder for %s failed with error: %s", metakey, err.Error()),
			})
		}
	}

	output, keyMetaOut, err := keyProvider.Provide(keyMetaIn)
	if err != nil {
		return append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Unable to fetch encryption key data",
			Detail:   fmt.Sprintf("%s failed with error: %s", metakey, err.Error()),
		})
	}

	if keyMetaOut != nil {
		e.keyProviderMetadata[metakey], err = json.Marshal(keyMetaOut)

		if err != nil {
			return append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Unable to encode encrypted metadata",
				Detail:   fmt.Sprintf("metadata encoder for %s failed with error: %s", metakey, err.Error()),
			})
		}
	}

	e.keyValues[cfg.Type][cfg.Name] = output.Cty()

	return nil

}
