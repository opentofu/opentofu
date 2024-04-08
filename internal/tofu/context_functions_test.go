package tofu

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/zclconf/go-cty/cty"
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

	// Initial call to getSchema
	expectProviderInit := true

	mockFactory := func() (providers.Interface, error) {
		if !expectProviderInit {
			return nil, fmt.Errorf("Unexpected call to provider init!")
		}
		expectProviderInit = false
		return mockProvider, nil
	}

	addr := addrs.NewDefaultProvider("mock")
	plugins := newContextPluginsForTest(map[addrs.Provider]providers.Factory{
		addr: mockFactory,
	}, t)

	t.Run("empty names map", func(t *testing.T) {
		res := plugins.Functions(map[string]addrs.Provider{})
		if len(res.ProviderNames) != 0 {
			t.Error("did not expect any names")
		}
		if len(res.Functions) != 0 {
			t.Error("did not expect any functions")
		}
	})

	t.Run("broken names map", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("Expected panic due to broken configuration")
			}
		}()

		res := plugins.Functions(map[string]addrs.Provider{
			"borky": addrs.NewDefaultProvider("my_borky"),
		})
		if len(res.ProviderNames) != 0 {
			t.Error("did not expect any names")
		}
		if len(res.Functions) != 0 {
			t.Error("did not expect any functions")
		}
	})

	res := plugins.Functions(map[string]addrs.Provider{
		"mockname": addr,
	})
	if res.ProviderNames["mockname"] != addr {
		t.Errorf("expected names %q, got %q", addr, res.ProviderNames["mockname"])
	}

	ctx := &hcl.EvalContext{
		Functions: res.Functions,
		Variables: map[string]cty.Value{
			"unknown_value":   cty.UnknownVal(cty.String),
			"sensitive_value": cty.StringVal("sensitive!").Mark(marks.Sensitive),
		},
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

		// Do this a few times but only expect a single init()
		expectProviderInit = true
		for i := 0; i < 5; i++ {
			t.Log("Checking valid argument")

			val, diags = evaluate(`provider::mockname::echo("hello functions!")`)
			if diags.HasErrors() {
				t.Error(diags.Error())
			}
			if !val.RawEquals(cty.StringVal("hello functions!")) {
				t.Error(val.AsString())
			}

			if expectProviderInit {
				t.Error("Expected provider init to have been called")
			}
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
