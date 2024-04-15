package tofu

import (
	"errors"

	"github.com/opentofu/opentofu/internal/providers"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

// TODO move this into a better named file

// Turn a provider function spec into a cty callable function
// This will use the instance factory to get a provider to support the
// function call.
func providerFunction(name string, spec providers.FunctionSpec, provider providers.Interface) function.Function {
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
		resp := provider.CallFunction(providers.CallFunctionRequest{
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
