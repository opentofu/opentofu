package tofu

import (
	"errors"
	"fmt"
	"log"
	"sync"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

// Lazily creates a single instance of a provider for repeated use.
// Concurrency safe
func lazyProviderInstance(addr addrs.Provider, factory providers.Factory) providers.Factory {
	var provider providers.Interface
	var providerLock sync.Mutex
	var err error

	return func() (providers.Interface, error) {
		providerLock.Lock()
		defer providerLock.Unlock()

		if provider == nil {
			log.Printf("[TRACE] tofu.contextFunctions: Initializing function provider %q", addr)
			provider, err = factory()
		}
		return provider, err
	}
}

// Loop through all functions specified and build a map of name -> function.
// All functions will use the same lazily initialized provider instance.
// This instance will run until the application is terminated.
func providerFunctions(addr addrs.Provider, funcSpecs map[string]providers.FunctionSpec, factory providers.Factory) map[string]function.Function {
	lazy := lazyProviderInstance(addr, factory)

	functions := make(map[string]function.Function)
	for name, spec := range funcSpecs {
		log.Printf("[TRACE] tofu.contextFunctions: Registering function %q in provider type %q", name, addr)
		if _, ok := functions[name]; ok {
			panic(fmt.Sprintf("broken provider %q: multiple functions registered under name %q", addr, name))
		}
		functions[name] = providerFunction(name, spec, lazy)
	}
	return functions
}

// Turn a provider function spec into a cty callable function
// This will use the instance factory to get a provider to support the
// function call.
func providerFunction(name string, spec providers.FunctionSpec, instance providers.Factory) function.Function {
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
		provider, err := instance()
		if err != nil {
			// Incredibly unlikely
			return cty.UnknownVal(retType), err
		}
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
