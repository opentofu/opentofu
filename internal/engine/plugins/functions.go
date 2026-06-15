// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package plugins

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

// BuildFunction implements evalglue.Providers.
func (n *newRuntimePlugins) BuildFunction(ctx context.Context, provider addrs.Provider, pf addrs.ProviderFunction, stubMissing bool, rng hcl.Range) (function.Function, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	inst, moreDiags := n.unconfiguredProviderInst(ctx, provider)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return function.Function{}, diags
	}

	fn, moreDiags := BuildProviderFunction(ctx, inst.(providers.Interface), pf, stubMissing, rng)
	diags = diags.Append(moreDiags)
	return fn, diags

}

// This is all copied and modified from internal/tofu/context_functions.go

// BuildProviderFunction constructs a cty function given a provider and a function address.
//
// The main oddity of this function is the stubMissing parameter. During validation, we do not
// have configured providers available. This prevents the full list of functions that a configured
// provider exposes from being known. If we ever deprecate functions on configured providers, this
// argument should be removed.
func BuildProviderFunction(ctx context.Context, provider providers.Interface, pf addrs.ProviderFunction, stubMissing bool, rng hcl.Range) (function.Function, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// First try to look up the function from provider schema
	schema := provider.GetProviderSchema(ctx) // TODO this should use the new providers cache...
	if schema.Diagnostics.HasErrors() {
		return function.Function{}, schema.Diagnostics
	}
	spec, ok := schema.Functions[pf.Function]
	if !ok {
		// During the validate operation, providers are not configured and therefore won't provide
		// a comprehensive GetFunctions list
		// Validate is built around unknown values already, we can stub in a placeholder
		if stubMissing {
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
			return fn, nil
		}

		// The provider may be configured and present additional functions via GetFunctions
		specs := provider.GetFunctions(ctx)
		if specs.Diagnostics.HasErrors() {
			return function.Function{}, specs.Diagnostics
		}

		// If the function isn't in the custom GetFunctions list, it must be undefined
		spec, ok = specs.Functions[pf.Function]
		if !ok {
			return function.Function{}, diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Function not found in provider",
				Detail:   fmt.Sprintf("Function %q was not registered by provider", pf),
				Subject:  rng.Ptr(),
			})
		}
	}

	return providerFunction(ctx, pf.Function, spec, provider), diags
}

// Turn a provider function spec into a cty callable function
// This will use the instance factory to get a provider to support the
// function call.
func providerFunction(ctx context.Context, name string, spec providers.FunctionSpec, provider providers.Interface) function.Function {
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
		resp := provider.CallFunction(ctx, providers.CallFunctionRequest{
			Name:      name,
			Arguments: args,
		})

		if argError, ok := resp.Error.(*providers.CallFunctionArgumentError); ok {
			// Convert ArgumentError to cty error
			return resp.Result, function.NewArgError(argError.FunctionArgument, errors.New(argError.Text))
		}

		return resp.Result, resp.Error
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
