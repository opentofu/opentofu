// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

func TestFunctions(t *testing.T) {
	mockProvider := &MockProvider{
		GetProviderSchemaResponse: &providers.GetProviderSchemaResponse{
			Provider: providers.Schema{},
			Functions: map[string]providers.FunctionSpec{
				"echo": providers.FunctionSpec{
					Parameters: []providers.FunctionParameterSpec{providers.FunctionParameterSpec{
						Name:               "input",
						Type:               cty.String,
						AllowNullValue:     false,
						AllowUnknownValues: false,
					}},
					Return: cty.String,
				},
				"concat": providers.FunctionSpec{
					Parameters: []providers.FunctionParameterSpec{providers.FunctionParameterSpec{
						Name:               "input",
						Type:               cty.String,
						AllowNullValue:     false,
						AllowUnknownValues: false,
					}},
					VariadicParameter: &providers.FunctionParameterSpec{
						Name:           "vary",
						Type:           cty.String,
						AllowNullValue: false,
					},
					Return: cty.String,
				},
				"coalesce": providers.FunctionSpec{
					Parameters: []providers.FunctionParameterSpec{providers.FunctionParameterSpec{
						Name:               "input1",
						Type:               cty.String,
						AllowNullValue:     true,
						AllowUnknownValues: false,
					}, providers.FunctionParameterSpec{
						Name:               "input2",
						Type:               cty.String,
						AllowNullValue:     false,
						AllowUnknownValues: false,
					}},
					Return: cty.String,
				},
				"unknown_param": providers.FunctionSpec{
					Parameters: []providers.FunctionParameterSpec{providers.FunctionParameterSpec{
						Name:               "input",
						Type:               cty.String,
						AllowNullValue:     false,
						AllowUnknownValues: true,
					}},
					Return: cty.String,
				},
				"error_param": providers.FunctionSpec{
					Parameters: []providers.FunctionParameterSpec{providers.FunctionParameterSpec{
						Name:               "input",
						Type:               cty.String,
						AllowNullValue:     false,
						AllowUnknownValues: false,
					}},
					Return: cty.String,
				},
			},
		},
	}

	mockProvider.CallFunctionFn = func(req providers.CallFunctionRequest) (resp providers.CallFunctionResponse) {
		switch req.Name {
		case "echo":
			resp.Result = req.Arguments[0]
		case "concat":
			str := ""
			for _, arg := range req.Arguments {
				str += arg.AsString()
			}
			resp.Result = cty.StringVal(str)
		case "coalesce":
			resp.Result = req.Arguments[0]
			if resp.Result.IsNull() {
				resp.Result = req.Arguments[1]
			}
		case "unknown_param":
			resp.Result = cty.StringVal("knownvalue")
		case "error_param":
			resp.Error = &providers.CallFunctionArgumentError{
				Text:             "my error text",
				FunctionArgument: 0,
			}
		default:
			panic("Invalid function")
		}
		return resp
	}

	mockProvider.GetFunctionsFn = func() (resp providers.GetFunctionsResponse) {
		resp.Functions = mockProvider.GetProviderSchemaResponse.Functions
		return resp
	}

	addr := addrs.NewDefaultProvider("mock")
	rng := tfdiags.SourceRange{}
	providerFunc := func(fn string) addrs.ProviderFunction {
		pf, _ := addrs.ParseFunction(fn).AsProviderFunction()
		return pf
	}

	mockCtx := new(MockEvalContext)
	cfg := &configs.Config{
		Module: &configs.Module{
			ProviderRequirements: &configs.RequiredProviders{
				RequiredProviders: map[string]*configs.RequiredProvider{
					"mockname": &configs.RequiredProvider{
						Name: "mock",
						Type: addr,
					},
				},
			},
		},
	}

	// Provider missing
	_, diags := evalContextProviderFunction(mockCtx.Provider, cfg, walkValidate, providerFunc("provider::invalid::unknown"), rng)
	if !diags.HasErrors() {
		t.Fatal("expected unknown function provider")
	}
	if diags.Err().Error() != `Unknown function provider: Provider "invalid" does not exist within the required_providers of this module` {
		t.Fatal(diags.Err())
	}

	// Provider not initialized
	_, diags = evalContextProviderFunction(mockCtx.Provider, cfg, walkValidate, providerFunc("provider::mockname::missing"), rng)
	if !diags.HasErrors() {
		t.Fatal("expected unknown function provider")
	}
	if diags.Err().Error() != `BUG: Uninitialized function provider: Provider "provider[\"registry.opentofu.org/hashicorp/mock\"]" has not yet been initialized` {
		t.Fatal(diags.Err())
	}

	// "initialize" provider
	mockCtx.ProviderProvider = mockProvider

	// Function missing (validate)
	mockProvider.GetFunctionsCalled = false
	_, diags = evalContextProviderFunction(mockCtx.Provider, cfg, walkValidate, providerFunc("provider::mockname::missing"), rng)
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}
	if mockProvider.GetFunctionsCalled {
		t.Fatal("expected GetFunctions NOT to be called since it's not initialized")
	}

	// Function missing (Non-validate)
	mockProvider.GetFunctionsCalled = false
	_, diags = evalContextProviderFunction(mockCtx.Provider, cfg, walkPlan, providerFunc("provider::mockname::missing"), rng)
	if !diags.HasErrors() {
		t.Fatal("expected unknown function")
	}
	if diags.Err().Error() != `Function not found in provider: Function "missing" was not registered by provider "provider[\"registry.opentofu.org/hashicorp/mock\"]"` {
		t.Fatal(diags.Err())
	}
	if !mockProvider.GetFunctionsCalled {
		t.Fatal("expected GetFunctions to be called")
	}

	ctx := &hcl.EvalContext{
		Functions: map[string]function.Function{},
		Variables: map[string]cty.Value{
			"unknown_value":   cty.UnknownVal(cty.String),
			"sensitive_value": cty.StringVal("sensitive!").Mark(marks.Sensitive),
		},
	}

	// Load functions into ctx
	for _, fn := range []string{"echo", "concat", "coalesce", "unknown_param", "error_param"} {
		pf := providerFunc("provider::mockname::" + fn)
		impl, diags := evalContextProviderFunction(mockCtx.Provider, cfg, walkPlan, pf, rng)
		if diags.HasErrors() {
			t.Fatal(diags.Err())
		}
		ctx.Functions[pf.String()] = *impl
	}
	evaluate := func(exprStr string) (cty.Value, hcl.Diagnostics) {
		expr, diags := hclsyntax.ParseExpression([]byte(exprStr), "exprtest", hcl.InitialPos)
		if diags.HasErrors() {
			t.Fatal(diags)
		}
		return expr.Value(ctx)
	}

	t.Run("echo function", func(t *testing.T) {
		// These are all assumptions that the provider implementation should not have to worry about:

		t.Log("Checking not enough arguments")
		_, diags := evaluate("provider::mockname::echo()")
		if !strings.Contains(diags.Error(), `Not enough function arguments; Function "provider::mockname::echo" expects 1 argument(s). Missing value for "input"`) {
			t.Error(diags.Error())
		}

		t.Log("Checking too many arguments")
		_, diags = evaluate(`provider::mockname::echo("1", "2", "3")`)
		if !strings.Contains(diags.Error(), `Too many function arguments; Function "provider::mockname::echo" expects only 1 argument(s)`) {
			t.Error(diags.Error())
		}

		t.Log("Checking null argument")
		_, diags = evaluate(`provider::mockname::echo(null)`)
		if !strings.Contains(diags.Error(), `Invalid function argument; Invalid value for "input" parameter: argument must not be null`) {
			t.Error(diags.Error())
		}

		t.Log("Checking unknown argument")
		val, diags := evaluate(`provider::mockname::echo(unknown_value)`)
		if diags.HasErrors() {
			t.Error(diags.Error())
		}
		if !val.RawEquals(cty.UnknownVal(cty.String)) {
			t.Error(val.AsString())
		}

		// Actually test the function implementation

		t.Log("Checking valid argument")

		val, diags = evaluate(`provider::mockname::echo("hello functions!")`)
		if diags.HasErrors() {
			t.Error(diags.Error())
		}
		if !val.RawEquals(cty.StringVal("hello functions!")) {
			t.Error(val.AsString())
		}

		t.Log("Checking sensitive argument")

		val, diags = evaluate(`provider::mockname::echo(sensitive_value)`)
		if diags.HasErrors() {
			t.Error(diags.Error())
		}
		if !val.RawEquals(cty.StringVal("sensitive!").Mark(marks.Sensitive)) {
			t.Error(val.AsString())
		}
	})

	t.Run("concat function", func(t *testing.T) {
		// Make sure varargs are handled properly

		// Single
		val, diags := evaluate(`provider::mockname::concat("foo")`)
		if diags.HasErrors() {
			t.Error(diags.Error())
		}
		if !val.RawEquals(cty.StringVal("foo")) {
			t.Error(val.AsString())
		}

		// Multi
		val, diags = evaluate(`provider::mockname::concat("foo", "bar", "baz")`)
		if diags.HasErrors() {
			t.Error(diags.Error())
		}
		if !val.RawEquals(cty.StringVal("foobarbaz")) {
			t.Error(val.AsString())
		}
	})

	t.Run("coalesce function", func(t *testing.T) {
		val, diags := evaluate(`provider::mockname::coalesce("first", "second")`)
		if diags.HasErrors() {
			t.Error(diags.Error())
		}
		if !val.RawEquals(cty.StringVal("first")) {
			t.Error(val.AsString())
		}

		val, diags = evaluate(`provider::mockname::coalesce(null, "second")`)
		if diags.HasErrors() {
			t.Error(diags.Error())
		}
		if !val.RawEquals(cty.StringVal("second")) {
			t.Error(val.AsString())
		}
	})

	t.Run("unknown_param function", func(t *testing.T) {
		val, diags := evaluate(`provider::mockname::unknown_param(unknown_value)`)
		if diags.HasErrors() {
			t.Error(diags.Error())
		}
		if !val.RawEquals(cty.StringVal("knownvalue")) {
			t.Error(val.AsString())
		}
	})
	t.Run("error_param function", func(t *testing.T) {
		_, diags := evaluate(`provider::mockname::error_param("foo")`)
		if !strings.Contains(diags.Error(), `Invalid function argument; Invalid value for "input" parameter: my error text.`) {
			t.Error(diags.Error())
		}
	})
}
