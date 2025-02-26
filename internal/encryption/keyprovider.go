// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package encryption

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/encryption/config"
	"github.com/opentofu/opentofu/internal/encryption/method"
	"github.com/opentofu/opentofu/internal/encryption/method/unencrypted"
	"github.com/opentofu/opentofu/internal/lang"
	"github.com/opentofu/opentofu/internal/lang/marks"

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

	kpMap := make(map[string]cty.Value)
	for _, keyProviderConfig := range e.cfg.KeyProviderConfigs {
		diags = append(diags, e.setupKeyProvider(keyProviderConfig, nil)...)
		if diags.HasErrors() {
			return diags
		}
		for name, kps := range e.keyValues {
			kpMap[name] = cty.ObjectVal(kps)
		}
		e.ctx.Variables["key_provider"] = cty.ObjectVal(kpMap)
	}

	// Make sure that the key_provider variable is set even if no key providers are configured. This will ultimately
	// result in an error, but we want to avoid unpredictable behavior.
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

	diags := ensureNoCircularKeyProviderRef(cfg, stack)
	if diags.HasErrors() {
		return diags
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

		//nolint:errcheck // This will always be a TraverseRoot, panic is OK if that's not the case
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

	// gohcl does not handle marks, we need to remove the sensitive marks from any input variables
	// We assume that the entire configuration in the encryption block should be treated as sensitive
	for key, sv := range evalCtx.Variables {
		if marks.Contains(sv, marks.Sensitive) {
			evalCtx.Variables[key], _ = sv.UnmarkDeep()
		}
	}

	// TODO - bug here
	// Considering that a provider A can reference a provider B (in `xor` provider case), if the provider B is not parsed
	// individually in setupKeyProviders before processing provider A, DecodeBody here can fail.
	// This is due to the fact that the DecodeBody below relies on having all the referenced resources in the EvalCtx.
	// When this method is called recursively, the attributes of the provider B are not added to the EvalCtx before calling this DecodeBody for the provider A.
	// NOTE: When this bug is fixed, please adjust the comment inside requiredProviders and also remove that slices.Reverse call to test this out.

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
			Detail:   fmt.Sprintf("%s failed with error: %s", metaKey, err.Error()),
		})
	}

	// Add the metadata
	if meta, ok := e.inputKeyProviderMetadata[metaKey]; ok {
		err := json.Unmarshal(meta, keyMetaIn)
		if err != nil {
			return append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Unable to decode encrypted metadata (did you change your encryption config?)",
				Detail:   fmt.Sprintf("metadata decoder for %s failed with error: %s", metaKey, err.Error()),
			})
		}
	}

	output, keyMetaOut, err := keyProvider.Provide(keyMetaIn)
	if err != nil {
		return append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Unable to fetch encryption key data",
			Detail:   fmt.Sprintf("%s failed with error: %s", metaKey, err.Error()),
		})
	}

	if keyMetaOut != nil {
		if _, ok := e.outputKeyProviderMetadata[metaKey]; ok {
			return append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Duplicate metadata key",
				Detail:   fmt.Sprintf("The metadata key %s is duplicated across multiple key providers for the same method; use the encrypted_metadata_alias option to specify unique metadata keys for each key provider in an encryption method", metaKey),
			})
		}
		e.outputKeyProviderMetadata[metaKey], err = json.Marshal(keyMetaOut)

		if err != nil {
			return append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Unable to encode encrypted metadata",
				Detail:   fmt.Sprintf("The metadata encoder for %s failed with error: %s", metaKey, err.Error()),
			})
		}
	}

	e.keyValues[cfg.Type][cfg.Name] = output.Cty()

	return nil

}

func (base *baseEncryption) requiredProviders(cfgs []config.MethodConfig) ([]config.KeyProviderConfig, hcl.Diagnostics) {
	var (
		diags hcl.Diagnostics
		out   []config.KeyProviderConfig
	)
	// method used only for diagnostics
	keyProviderConfig := func(addr string, method config.MethodConfig) (*config.KeyProviderConfig, hcl.Diagnostics) {
		for _, kpCfg := range base.enc.cfg.KeyProviderConfigs {
			kpAddr, diag := kpCfg.Addr()
			if diag.HasErrors() {
				return nil, diag
			}
			if string(kpAddr) == addr {
				return &kpCfg, nil
			}
		}
		var subj *hcl.Range
		if c, ok := method.Body.(*hclsyntax.Body); ok {
			subj = &c.SrcRange
		}
		return nil, hcl.Diagnostics{&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Undefined key_provider",
			Detail:   fmt.Sprintf("Can not find key_provider %q referenced by method %q", addr, method.Name),
			Subject:  subj,
		}}
	}
	for _, m := range cfgs {
		if unencrypted.IsID(method.ID(m.Type)) {
			continue
		}
		var keys struct {
			Keys cty.Value `hcl:"keys"`
		}
		innerKpRefs, diag := gohcl.VariablesInBody(m.Body, &keys)
		diags = append(diags, diag...)
		if diags.HasErrors() {
			return nil, diags
		}
		for _, dep := range innerKpRefs {
			// extract the key_provider address as defined inside the current config.MethodConfig
			addr, diag := traversalToKeyProviderAddr(dep, m.Name)
			diags = append(diags, diag...)
			if diags.HasErrors() {
				return nil, diags
			}
			// search for the config.KeyProviderConfig that is referenced by the address from the current method
			kp, diag := keyProviderConfig(string(addr), m)
			diags = append(diags, diag...)
			if diags.HasErrors() {
				return nil, diags
			}
			// collect all the (possibly) referenced additional key_providers
			kpCfgs, cfgsDiags := base.extractInnerKeyProviders(*kp, nil)
			diags = append(diags, cfgsDiags...)
			if diags.HasErrors() {
				return nil, diags
			}
			out = append(out, kpCfgs...)
		}
	}
	// Due to an issue found in setupKeyProvider, the order of the configs in the returned slice matters, as that method needs to process the innermost key provider(s) first.
	slices.Reverse(out)
	return out, diags
}

// extractInnerKeyProviders receives a config.KeyProviderConfig and tries to get any other config.KeyProvider object(s) that might be referenced internally.
// This is handled accordingly with the initial implementatin of the state encryption, where the key provider is processed on its type definition fetched from registry.Registry.
// This method is not returning only the inner referenced config.KeyProviderConfig but also returns the given config.KeyProviderConfig.
func (base *baseEncryption) extractInnerKeyProviders(cfg config.KeyProviderConfig, stack []config.KeyProviderConfig) ([]config.KeyProviderConfig, hcl.Diagnostics) {
	diags := ensureNoCircularKeyProviderRef(cfg, stack)
	if diags.HasErrors() {
		return nil, diags
	}
	stack = append(stack, cfg)

	out := []config.KeyProviderConfig{
		cfg,
	}

	// Lookup the KeyProviderDescriptor from the registry
	id := keyprovider.ID(cfg.Type)
	keyProviderDescriptor, err := base.enc.reg.GetKeyProviderDescriptor(id)
	if err != nil {
		if errors.Is(err, &registry.KeyProviderNotFoundError{}) {
			return nil, diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Unknown key_provider type",
				Detail:   fmt.Sprintf("Can not find key_provider %q", cfg.Type),
			})
		}
		return nil, diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  fmt.Sprintf("Error fetching key_provider %q", cfg.Type),
			Detail:   err.Error(),
		})
	}

	// Now that we know we have the correct Descriptor, we can decode the configuration
	// and build the KeyProvider
	keyProviderConfig := keyProviderDescriptor.ConfigStruct()

	// Locate all the dependencies - this is looking for any possible definitions of key_providers inside the current provider body content.
	deps, varDiags := gohcl.VariablesInBody(cfg.Body, keyProviderConfig)
	diags = append(diags, varDiags...)
	if diags.HasErrors() {
		return nil, diags
	}

	// Setting up key providers from deps.
	for _, dep := range deps {
		if !isKeyProviderTraversal(dep) {
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

		kpc, ok := base.enc.cfg.GetKeyProvider(depType, depName)
		if !ok {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Undefined Key Provider",
				Detail:   fmt.Sprintf("Key provider %s.%s is missing from the encryption configuration.", depType, depName),
				Subject:  dep.SourceRange().Ptr(),
			})
			continue
		}

		addrs, depDiags := base.extractInnerKeyProviders(kpc, stack)
		diags = append(diags, depDiags...)
		out = append(out, addrs...)
	}
	if diags.HasErrors() {
		// We should not continue now if we have any diagnostics that are errors
		// as we may end up in an inconsistent state.
		// The reason we collate the diags here and then show them instead of showing them as they arise
		// is to ensure that the end user does not have to play whack-a-mole with the errors one at a time.
		return nil, diags
	}

	return out, diags
}

func traversalToKeyProviderAddr(trav hcl.Traversal, methodName string) (keyprovider.Addr, hcl.Diagnostics) {
	if !isKeyProviderTraversal(trav) {
		return "", hcl.Diagnostics{&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  fmt.Sprintf("Method %q `keys` attribute is not referencing a key_provider", methodName),
			Detail:   "Expected key_provider.<type>.<name>",
			Subject:  trav.SourceRange().Ptr(),
		}}
	}
	depTypeAttr, typeOk := trav[1].(hcl.TraverseAttr)
	depNameAttr, nameOk := trav[2].(hcl.TraverseAttr)

	if !typeOk || !nameOk {
		return "", hcl.Diagnostics{&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid Key Provider expression format",
			Detail:   "Expected key_provider.<type>.<name>",
			Subject:  trav.SourceRange().Ptr(),
		}}
	}

	depType := depTypeAttr.Name
	depName := depNameAttr.Name

	return keyprovider.NewAddr(depType, depName)
}

// Check for circular references, this is done by inspecting the stack of key providers
// that are currently being setup. If we find a key provider in the stack that matches
// the current key provider, then we have a circular reference and we should return an error
// to the user.
func ensureNoCircularKeyProviderRef(currentCfg config.KeyProviderConfig, stack []config.KeyProviderConfig) hcl.Diagnostics {
	for _, s := range stack {
		if s == currentCfg {
			addr, diags := keyprovider.NewAddr(currentCfg.Type, currentCfg.Name)
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
	return nil
}

func isKeyProviderTraversal(trav hcl.Traversal) bool {
	if len(trav) != 3 {
		return false
	}
	root, ok := trav[0].(hcl.TraverseRoot)
	if !ok {
		return false
	}
	if root.Name != "key_provider" {
		return false
	}
	return true
}
