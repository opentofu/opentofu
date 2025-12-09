// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/plugins"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// This builds a provider function using an EvalContext and some additional information
// This is split out of BuiltinEvalContext for testing
func evalContextProviderFunction(ctx context.Context, providers plugins.ProviderManager, providerAddr addrs.AbsProviderInstanceCorrect, op walkOperation, pf addrs.ProviderFunction, rng tfdiags.SourceRange) (*function.Function, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// First try to look up the function from provider schema
	schema := providers.GetProviderSchema(ctx, providerAddr.Config.Config.Provider)
	if schema.Diagnostics.HasErrors() {
		return nil, schema.Diagnostics
	}
	spec, ok := schema.Functions[pf.Function]
	if !ok {
		// During the validate operation, providers are not configured and therefore won't provide
		// a comprehensive GetFunctions list
		// Validate is built around unknown values already, we can stub in a placeholder
		if op == walkValidate {
			// Configured provider functions are not available during validate
			fn := function.New(&function.Spec{
				Description: "Validate Placeholder",
				VarParam: &function.Parameter{
					Type:             cty.DynamicPseudoType,
					AllowNull:        true,
					AllowUnknown:     true,
					AllowDynamicType: true,
					AllowMarked:      false,
				},
				Type: function.StaticReturnType(cty.DynamicPseudoType),
				Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
					return cty.UnknownVal(cty.DynamicPseudoType), nil
				},
			})
			return &fn, nil
		}

		// The provider may be configured and present additional functions via GetFunctions
		specs := providers.GetFunctions(ctx, providerAddr)
		if specs.Diagnostics.HasErrors() {
			return nil, specs.Diagnostics
		}

		// If the function isn't in the custom GetFunctions list, it must be undefined
		spec, ok = specs.Functions[pf.Function]
		if !ok {
			return nil, diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Function not found in provider",
				Detail:   fmt.Sprintf("Function %q was not registered by provider", pf),
				Subject:  rng.ToHCL().Ptr(),
			})
		}
	}

	fn := providerFunction(ctx, pf.Function, spec, providers, providerAddr)

	return &fn, nil

}

// Turn a provider function spec into a cty callable function
// This will use the instance factory to get a provider to support the
// function call.
func providerFunction(ctx context.Context, name string, spec providers.FunctionSpec, pm plugins.ProviderManager, providerAddr addrs.AbsProviderInstanceCorrect) function.Function {
	params := make([]function.Parameter, len(spec.Parameters))
	for i, param := range spec.Parameters {
		params[i] = providerFunctionParameter(param)
	}

	var varParam *function.Parameter
	if spec.VariadicParameter != nil {
		value := providerFunctionParameter(*spec.VariadicParameter)
		varParam = &value
	}

	impl := func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		result, err := pm.CallFunction(ctx, providerAddr, name, args)

		if argError, ok := err.(*providers.CallFunctionArgumentError); ok {
			// Convert ArgumentError to cty error
			return result, function.NewArgError(argError.FunctionArgument, errors.New(argError.Text))
		}

		return result, err
	}

	return function.New(&function.Spec{
		Description: spec.Summary,
		Params:      params,
		VarParam:    varParam,
		Type:        function.StaticReturnType(spec.Return),
		Impl:        impl,
	})

}

// Simple mapping of function parameter spec to function parameter
func providerFunctionParameter(spec providers.FunctionParameterSpec) function.Parameter {
	return function.Parameter{
		Name:         spec.Name,
		Description:  spec.Description,
		Type:         spec.Type,
		AllowNull:    spec.AllowNullValue,
		AllowUnknown: spec.AllowUnknownValues,
		// I don't believe this is allowable for provider functions
		AllowDynamicType: false,
		// force cty to strip marks ahead of time and re-add them to the resulting object
		// GRPC: failed: value has marks, so it cannot be serialized.
		AllowMarked: false,
	}
}
