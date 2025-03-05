// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"fmt"
	"log"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/hcl2shim"
	"github.com/opentofu/opentofu/internal/lang"
	"github.com/opentofu/opentofu/internal/lang/evalchecks"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func buildProviderConfig(ctx EvalContext, addr addrs.AbsProviderConfig, config *configs.Provider) hcl.Body {
	var configBody hcl.Body
	if config != nil {
		configBody = config.Config
	}

	var inputBody hcl.Body
	inputConfig := ctx.ProviderInput(addr)
	if len(inputConfig) > 0 {
		inputBody = configs.SynthBody("<input-prompt>", inputConfig)
	}

	switch {
	case configBody != nil && inputBody != nil:
		log.Printf("[TRACE] buildProviderConfig for %s: merging explicit config and input", addr)
		return hcl.MergeBodies([]hcl.Body{inputBody, configBody})
	case configBody != nil:
		log.Printf("[TRACE] buildProviderConfig for %s: using explicit config only", addr)
		return configBody
	case inputBody != nil:
		log.Printf("[TRACE] buildProviderConfig for %s: using input only", addr)
		return inputBody
	default:
		log.Printf("[TRACE] buildProviderConfig for %s: no configuration at all", addr)
		addr := fmt.Sprintf("%s with no configuration", addr)
		return hcl2shim.SynthBody(addr, make(map[string]cty.Value))
	}
}

func resolveProviderResourceInstance(ctx EvalContext, keyExpr hcl.Expression, resourcePath addrs.AbsResourceInstance) (addrs.InstanceKey, tfdiags.Diagnostics) {
	keyData := ctx.InstanceExpander().GetResourceInstanceRepetitionData(resourcePath)
	keyScope := ctx.EvaluationScope(nil, nil, keyData)
	return resolveProviderInstance(keyExpr, keyScope, resourcePath.String())
}

func resolveProviderModuleInstance(ctx EvalContext, keyExpr hcl.Expression, modulePath addrs.ModuleInstance, source string) (addrs.InstanceKey, tfdiags.Diagnostics) {
	keyData := ctx.InstanceExpander().GetModuleInstanceRepetitionData(modulePath)
	// module providers block is evaluated in the parent module scope, similar to GraphNodeReferenceOutside
	evalPath := modulePath.Parent()
	keyScope := ctx.WithPath(evalPath).EvaluationScope(nil, nil, keyData)
	return resolveProviderInstance(keyExpr, keyScope, source)
}

func resolveProviderInstance(keyExpr hcl.Expression, keyScope *lang.Scope, source string) (addrs.InstanceKey, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	keyVal, keyDiags := keyScope.EvalExpr(keyExpr, cty.DynamicPseudoType)
	diags = diags.Append(keyDiags)
	if keyDiags.HasErrors() {
		return nil, diags
	}

	if keyVal.HasMark(marks.Sensitive) {
		return nil, diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid provider instance key",
			Detail:   "A provider instance key must not be derived from a sensitive value.",
			Subject:  keyExpr.Range().Ptr(),
			Extra:    evalchecks.DiagnosticCausedBySensitive(true),
		})
	}
	if keyVal.IsNull() {
		return nil, diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid provider instance key",
			Detail:   "A provider instance key must not be null.",
			Subject:  keyExpr.Range().Ptr(),
		})
	}
	if !keyVal.IsKnown() {
		return nil, diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid provider instance key",
			Detail:   fmt.Sprintf("The provider instance key for %s depends on values that cannot be determined until apply, and so OpenTofu cannot select a provider instance to create a plan for this resource instance.", source),
			Subject:  keyExpr.Range().Ptr(),
			Extra:    evalchecks.DiagnosticCausedByUnknown(true),
		})
	}

	// bool and number type are converted to string
	keyVal, convertErr := convert.Convert(keyVal, cty.String)
	if convertErr != nil {
		return nil, diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid provider instance key",
			Detail:   fmt.Sprintf("The given instance key is unsuitable: %s.", tfdiags.FormatError(convertErr)),
			Subject:  keyExpr.Range().Ptr(),
		})
	}

	// Because of the string conversion before the call of the ParseInstanceKey function,
	// no errors will be raised. Because keyVal is guaranteed to be a string.
	// We can keep the error handling in case the implementation of ParseInstanceKey change in the future
	parsedKey, parsedErr := addrs.ParseInstanceKey(keyVal)
	if parsedErr != nil {
		return nil, diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid provider instance key",
			Detail:   fmt.Sprintf("The given instance key is unsuitable: %s.", tfdiags.FormatError(parsedErr)),
			Subject:  keyExpr.Range().Ptr(),
		})
	}
	return parsedKey, diags
}

// getProvider returns the providers.Interface and schema for a given provider.
func getProvider(ctx EvalContext, addr addrs.AbsProviderConfig, providerKey addrs.InstanceKey) (providers.Interface, providers.ProviderSchema, error) {
	if addr.Provider.Type == "" {
		// Should never happen
		panic("GetProvider used with uninitialized provider configuration address")
	}
	provider := ctx.Provider(addr, providerKey)
	if provider == nil {
		return nil, providers.ProviderSchema{}, fmt.Errorf("provider %s not initialized", addr.InstanceString(providerKey))
	}
	// Not all callers require a schema, so we will leave checking for a nil
	// schema to the callers.
	schema, err := ctx.ProviderSchema(addr)
	if err != nil {
		return nil, providers.ProviderSchema{}, fmt.Errorf("failed to read schema for provider %s: %w", addr, err)
	}
	return provider, schema, nil
}
