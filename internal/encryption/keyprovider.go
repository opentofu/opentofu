// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package encryption

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/encryption/config"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
	"github.com/opentofu/opentofu/internal/encryption/registry"
	"github.com/opentofu/opentofu/internal/lang"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/zclconf/go-cty/cty"
)

// valueMap is a helper type for building hcl.EvalContexts for key_providers.
type valueMap map[string]map[string]cty.Value

func (v valueMap) set(first string, second string, value cty.Value) {
	if _, ok := v[first]; !ok {
		v[first] = make(map[string]cty.Value)
	}
	v[first][second] = value
}

func (v valueMap) has(first string, second string) bool {
	s, ok := v[first]
	if !ok {
		return false
	}
	_, ok = s[second]
	return ok
}

func (v valueMap) hclEvalContext(root string) *hcl.EvalContext {
	mMap := make(map[string]cty.Value)
	for name, ms := range v {
		mMap[name] = cty.ObjectVal(ms)
	}

	return &hcl.EvalContext{
		Variables: map[string]cty.Value{root: cty.ObjectVal(mMap)},
	}
}

// Given a set of hcl.Traversals, determine the required key provider configs and non-key_provider references
func filterKeyProviderReferences(cfg *config.EncryptionConfig, deps []hcl.Traversal) ([]config.KeyProviderConfig, []*addrs.Reference, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	var keyProviderDeps []config.KeyProviderConfig
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

		depRoot := dep.RootName()
		if depRoot != "key_provider" {
			nonKeyProviderDeps = append(nonKeyProviderDeps, dep)
			continue
		}
		depTypeAttr, typeOk := dep[1].(hcl.TraverseAttr)
		depNameAttr, nameOk := dep[2].(hcl.TraverseAttr)

		if !typeOk || !nameOk {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid Key Provider expression format",
				Detail:   "The key_provider symbol must be followed by two more attribute names specifying the type and name of the selected key provider.",
				Subject:  dep.SourceRange().Ptr(),
			})
			continue
		}

		depType := depTypeAttr.Name
		depName := depNameAttr.Name

		kpc, ok := cfg.GetKeyProvider(depType, depName)
		if !ok {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Reference to undeclared key provider",
				Detail:   fmt.Sprintf("There is no key_provider %q %q block declared in the encryption block.", depType, depName),
				Subject:  dep.SourceRange().Ptr(),
			})
			continue
		}

		keyProviderDeps = append(keyProviderDeps, kpc)
	}

	refs, refDiags := lang.References(addrs.ParseRef, nonKeyProviderDeps)
	diags = diags.Extend(refDiags.ToHCL())

	return keyProviderDeps, refs, diags
}

// setupKeyProviders sets up the key providers for encryption. It returns a list of diagnostics if any of the key providers
// are invalid.
func setupKeyProviders(enc *config.EncryptionConfig, cfgs []config.KeyProviderConfig, meta keyProviderMetadata, reg registry.Registry, staticEval *configs.StaticEvaluator) (*hcl.EvalContext, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	kpData := make(valueMap)

	for _, keyProviderConfig := range cfgs {
		diags = diags.Extend(setupKeyProvider(enc, keyProviderConfig, kpData, nil, meta, reg, staticEval))
	}

	return kpData.hclEvalContext("key_provider"), diags
}

func setupKeyProvider(enc *config.EncryptionConfig, cfg config.KeyProviderConfig, kpData valueMap, stack []config.KeyProviderConfig, meta keyProviderMetadata, reg registry.Registry, staticEval *configs.StaticEvaluator) hcl.Diagnostics {
	// Check if we have already setup this Descriptor (due to dependency loading)
	// if we've already setup this key provider, then we don't need to do it again
	// and we can return early
	if kpData.has(cfg.Type, cfg.Name) {
		return nil
	}

	// Mark this key provider as partially handled.  This value will be replaced below once it is actually known.
	// The goal is to allow an early return via the above if statement to prevent duplicate errors if errors are encountered in the key loading stack.
	kpData.set(cfg.Type, cfg.Name, cty.UnknownVal(cty.DynamicPseudoType))

	// Check for circular references, this is done by inspecting the stack of key providers
	// that are currently being setup. If we find a key provider in the stack that matches
	// the current key provider, then we have a circular reference and we should return an error
	// to the user.
	for _, s := range stack {
		if s == cfg {
			addr, diags := keyprovider.NewAddr(cfg.Type, cfg.Name)
			stackAddrs, diag := keyProvidersStack(append(stack, cfg))
			diags = diags.Extend(diag)
			return diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Circular reference detected",
				Detail:   fmt.Sprintf("Cannot load %s due to circular reference between key providers. Stack trace %s", addr, strings.Join(stackAddrs, " -> ")),
			})
		}
	}
	stack = append(stack, cfg)

	// Pull the meta key out for error messages and meta storage
	tmpMetaKey, diags := cfg.Addr()
	if diags.HasErrors() {
		return diags
	}
	metaKey := keyprovider.MetaStorageKey(tmpMetaKey)
	if cfg.EncryptedMetadataAlias != "" {
		metaKey = keyprovider.MetaStorageKey(cfg.EncryptedMetadataAlias)
	}

	// Lookup the KeyProviderDescriptor from the registry
	id := keyprovider.ID(cfg.Type)
	keyProviderDescriptor, err := reg.GetKeyProviderDescriptor(id)
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
	diags = diags.Extend(varDiags)
	if diags.HasErrors() {
		return diags
	}

	// Filter between dependent key providers and static references
	kpConfigs, refs, filterDiags := filterKeyProviderReferences(enc, deps)
	diags = diags.Extend(filterDiags)
	if diags.HasErrors() {
		return diags
	}

	// Ensure all key provider dependencies have been initialized
	for _, kp := range kpConfigs {
		diags = diags.Extend(setupKeyProvider(enc, kp, kpData, stack, meta, reg, staticEval))
	}
	if diags.HasErrors() {
		return diags
	}

	evalCtx, evalDiags := staticEval.EvalContextWithParent(context.TODO(), kpData.hclEvalContext("key_provider"), configs.StaticIdentifier{
		Module:    addrs.RootModule,
		Subject:   fmt.Sprintf("encryption.key_provider.%s.%s", cfg.Type, cfg.Name),
		DeclRange: enc.DeclRange,
	}, refs)
	diags = diags.Extend(evalDiags)
	if diags.HasErrors() {
		return diags
	}

	// gohcl does not handle marks, we need to remove the sensitive marks from any input variables
	// We assume that the entire configuration in the encryption block should be treated as sensitive
	for key, sv := range evalCtx.Variables {
		if marks.Contains(sv, marks.Sensitive) {
			evalCtx.Variables[key], _ = sv.UnmarkDeep()
		}
	}

	// Initialize the Key Provider
	decodeDiags := gohcl.DecodeBody(cfg.Body, evalCtx, keyProviderConfig)
	diags = diags.Extend(decodeDiags)
	if diags.HasErrors() {
		return diags
	}

	// Build the Key Provider from the configuration
	keyProvider, keyMetaIn, err := keyProviderConfig.Build()
	if err != nil {
		return diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Unable to build encryption key data",
			Detail:   fmt.Sprintf("%s failed with error: %s", metaKey, err.Error()),
		})
	}

	// Add the metadata
	if meta, ok := meta.input[metaKey]; ok {
		err := json.Unmarshal(meta, keyMetaIn)
		if err != nil {
			return diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Unable to decode encrypted metadata (did you change your encryption config?)",
				Detail:   fmt.Sprintf("metadata decoder for %s failed with error: %s", metaKey, err.Error()),
			})
		}
	}

	output, keyMetaOut, err := keyProvider.Provide(keyMetaIn)
	if err != nil {
		return diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Unable to fetch encryption key data",
			Detail:   fmt.Sprintf("%s failed with error: %s", metaKey, err.Error()),
		})
	}

	if keyMetaOut != nil {
		if _, ok := meta.output[metaKey]; ok {
			return diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Duplicate metadata key",
				Detail:   fmt.Sprintf("The metadata key %s is duplicated across multiple key providers for the same method; use the encrypted_metadata_alias option to specify unique metadata keys for each key provider in an encryption method", metaKey),
			})
		}
		meta.output[metaKey], err = json.Marshal(keyMetaOut)

		if err != nil {
			return diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Unable to encode encrypted metadata",
				Detail:   fmt.Sprintf("The metadata encoder for %s failed with error: %s", metaKey, err.Error()),
			})
		}
	}

	kpData.set(cfg.Type, cfg.Name, output.Cty())

	return nil

}

func keyProvidersStack(stack []config.KeyProviderConfig) ([]string, hcl.Diagnostics) {
	res := make([]string, len(stack))
	var diags hcl.Diagnostics
	for i, cfg := range stack {
		addr, diag := cfg.Addr()
		diags = diags.Extend(diag)
		if diag.HasErrors() {
			res[i] = "<unknown>"
			continue
		}
		res[i] = string(addr)
	}
	return res, diags
}
