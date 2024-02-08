package encryption

import (
	"errors"
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/opentofu/opentofu/internal/encryption/keyprovider"
	"github.com/opentofu/opentofu/internal/encryption/registry"
	"github.com/opentofu/opentofu/internal/varhcl"
	"github.com/zclconf/go-cty/cty"
)

// setupKeyProviders sets up the key providers for encryption. It returns a list of diagnostics if any of the key providers
// are invalid.
func (e *encryption) setupKeyProviders() hcl.Diagnostics {
	var diags hcl.Diagnostics
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

func (e *encryption) setupKeyProvider(cfg KeyProviderConfig, stack []KeyProviderConfig) hcl.Diagnostics {
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

	// Check for circular references, this is done by inspecting the stack of key providers
	// that are currently being setup. If we find a key provider in the stack that matches
	// the current key provider, then we have a circular reference and we should return an error
	// to the user.
	for _, s := range stack {
		if s == cfg {
			return hcl.Diagnostics{&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Circular reference detected",
				// TODO add the stack trace to the detail message
				Detail: fmt.Sprintf("Can not load %q due to circular reference", KeyProviderAddr(cfg.Type, cfg.Name)),
			}}
		}
	}
	stack = append(stack, cfg)

	// Lookup the KeyProviderDescriptor from the registry
	id := keyprovider.ID(cfg.Type)
	keyProviderDescriptor, err := e.reg.GetKeyProviderDescriptor(id)
	if err != nil {
		if errors.Is(err, &registry.KeyProviderNotFoundError{}) {
			return hcl.Diagnostics{&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Unknown key_provider type",
				Detail:   fmt.Sprintf("Can not find %q", cfg.Type),
			}}
		}
		return hcl.Diagnostics{&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  fmt.Sprintf("Error fetching key_provider %q", cfg.Type),
			Detail:   err.Error(),
		}}
	}

	// Now that we know we have the correct Descriptor, we can decode the configuration
	// and build the KeyProvider
	keyProviderConfig := keyProviderDescriptor.ConfigStruct()

	// Locate all the dependencies
	deps, diags := varhcl.VariablesInBody(cfg.Body, keyProviderConfig)
	if diags.HasErrors() {
		return diags
	}

	// Required Dependencies
	for _, dep := range deps {
		// Key Provider references should be in the form key_provider.type.name
		if len(dep) != 3 {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid key_provider reference",
				Detail:   "Expected reference in form key_provider.type.name",
				Subject:  dep.SourceRange().Ptr(),
			})
			continue
		}

		// TODO this should be more defensive
		depRoot := (dep[0].(hcl.TraverseRoot)).Name
		depType := (dep[1].(hcl.TraverseAttr)).Name
		depName := (dep[2].(hcl.TraverseAttr)).Name

		if depRoot != "key_provider" {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid key_provider reference",
				Detail:   "Expected reference in form key_provider.type.name",
				Subject:  dep.SourceRange().Ptr(),
			})
			continue
		}

		for _, kpc := range e.cfg.KeyProviderConfigs {
			// Find the key provider in the config
			if kpc.Type == depType && kpc.Name == depName {
				depDiags := e.setupKeyProvider(kpc, stack)
				diags = append(diags, depDiags...)
				break
			}
		}
	}
	if diags.HasErrors() {
		// We should not continue now if we have any diagnostics that are errors
		// as we may end up in an inconsistent state.
		// The reason we collate the diags here and then show them instead of showing them as they arise
		// is to ensure that the end user does not have to play whack-a-mole with the errors one at a time.
		return diags
	}

	// Initialize the Key Provider
	decodeDiags := gohcl.DecodeBody(cfg.Body, e.ctx, keyProviderConfig)
	diags = append(diags, decodeDiags...)
	if diags.HasErrors() {
		return diags
	}

	// Build the Key Provider from the configuration
	keyProvider, err := keyProviderConfig.Build()
	if err != nil {
		return append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Unable to build encryption key data",
			Detail:   fmt.Sprintf("%s failed with error: %s", KeyProviderAddr(cfg.Type, cfg.Name), err.Error()),
		})
	}
	data, err := keyProvider.Provide()
	if err != nil {
		e.keyValues[cfg.Type][cfg.Name] = cty.UnknownVal(cty.DynamicPseudoType)
		return append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Unable to fetch encryption key data",
			Detail:   fmt.Sprintf("%s failed with error: %s", KeyProviderAddr(cfg.Type, cfg.Name), err.Error()),
		})
	}

	// Convert the data into it's cty equivalent
	ctyData := make([]cty.Value, len(data))
	for i, d := range data {
		ctyData[i] = cty.NumberIntVal(int64(d))
	}
	e.keyValues[cfg.Type][cfg.Name] = cty.ListVal(ctyData)

	return nil
}
