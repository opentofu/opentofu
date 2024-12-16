package tf

import (
	"errors"
	"fmt"

	"github.com/opentofu/opentofu/internal/providers"
	"github.com/zclconf/go-cty/cty"
)

// "encode_tfvars"
// "decode_tfvars"
// "encode_expr"

type providerFunc interface {
	Name() string
	GetFunctionSpec() providers.FunctionSpec
	Call(args []cty.Value) (cty.Value, error)
}

func getProviderFuncs() map[string]providerFunc {
	encodeTFVars := &EncodeTFVarsFunc{}
	decodeTFVars := &DecodeTFVarsFunc{}
	encodeExpr := &EncodeExprFunc{}
	return map[string]providerFunc{
		encodeTFVars.Name(): encodeTFVars,
		decodeTFVars.Name(): decodeTFVars,
		encodeExpr.Name():   encodeExpr,
	}
}

func validateArgs(args []cty.Value, f providerFunc) error {
	declaredArgs := f.GetFunctionSpec().Parameters
	if len(args) != len(declaredArgs) {
		return fmt.Errorf("incorrect number of arguments, expected %d, got %d", len(declaredArgs), len(args))
	}
	var errs []error
	for i, arg := range args {
		var err error
		if !arg.Type().Equals(declaredArgs[i].Type) {
			err = fmt.Errorf("argument '%s' has an incorrect type, expected %s, got %s", declaredArgs[i].Name, declaredArgs[i].Type.FriendlyName(), arg.Type().FriendlyName())
		} else if !declaredArgs[i].AllowUnknownValues && !arg.IsKnown() {
			err = fmt.Errorf("argument '%s' cannot be unknown", declaredArgs[i].Name)
		} else if !declaredArgs[i].AllowNullValue && arg.IsNull() {
			err = fmt.Errorf("argument '%s' cannot be null", declaredArgs[i].Name)
		}
		if err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("invalid arguments for built-in function %q: %w", f.Name(), errors.Join(errs...))
	}
	return nil
}

type EncodeTFVarsFunc struct{}

func (f *EncodeTFVarsFunc) Name() string {
	return "encode_tfvars"
}

func (f *EncodeTFVarsFunc) GetFunctionSpec() providers.FunctionSpec {
	params := []providers.FunctionParameterSpec{}
	return providers.FunctionSpec{
		// List of parameters required to call the function
		Parameters: params,
		// Type which the function will return
		Return: cty.String,
		// Human-readable shortened documentation for the function
		Summary: "",
		// Human-readable documentation for the function
		Description: "",
		// Formatting type of the Description field
		DescriptionFormat: providers.TextFormattingPlain,
	}
}

func (f *EncodeTFVarsFunc) Call(args []cty.Value) (cty.Value, error) {
	if err := validateArgs(args, f); err != nil {
		return cty.NilVal, err
	}
	//TODO: Implement this function
	return cty.StringVal(""), nil
}

type DecodeTFVarsFunc struct{}

func (f *DecodeTFVarsFunc) Name() string {
	return "decode_tfvars"
}

func (f *DecodeTFVarsFunc) GetFunctionSpec() providers.FunctionSpec {
	params := []providers.FunctionParameterSpec{}
	return providers.FunctionSpec{
		// List of parameters required to call the function
		Parameters: params,
		// Type which the function will return
		Return: cty.String,
		// Human-readable shortened documentation for the function
		Summary: "",
		// Human-readable documentation for the function
		Description: "",
		// Formatting type of the Description field
		DescriptionFormat: providers.TextFormattingPlain,
	}
}

func (f *DecodeTFVarsFunc) Call(args []cty.Value) (cty.Value, error) {
	if err := validateArgs(args, f); err != nil {
		return cty.NilVal, err
	}
	//TODO: Implement this function
	return cty.StringVal(""), nil
}

type EncodeExprFunc struct{}

func (f *EncodeExprFunc) Name() string {
	return "encode_expr"
}

func (f *EncodeExprFunc) GetFunctionSpec() providers.FunctionSpec {
	params := []providers.FunctionParameterSpec{}
	return providers.FunctionSpec{
		// List of parameters required to call the function
		Parameters: params,
		// Type which the function will return
		Return: cty.String,
		// Human-readable shortened documentation for the function
		Summary: "",
		// Human-readable documentation for the function
		Description: "",
		// Formatting type of the Description field
		DescriptionFormat: providers.TextFormattingPlain,
	}
}

func (f *EncodeExprFunc) Call(args []cty.Value) (cty.Value, error) {
	if err := validateArgs(args, f); err != nil {
		return cty.NilVal, err
	}
	//TODO: Implement this function
	return cty.StringVal(""), nil
}
