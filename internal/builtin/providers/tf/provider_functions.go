package tf

import (
	"errors"
	"log"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/zclconf/go-cty/cty"
)

// "encode_tfvars"
// "decode_tfvars"

type providerFunc interface {
	Name() string
	GetFunctionSpec() providers.FunctionSpec
	Call(args []cty.Value) (cty.Value, error)
}

func getProviderFuncs() map[string]providerFunc {
	encodeTFVars := &EncodeTFVarsFunc{}
	decodeTFVars := &DecodeTFVarsFunc{}
	return map[string]providerFunc{
		encodeTFVars.Name(): encodeTFVars,
		decodeTFVars.Name(): decodeTFVars,
	}
}

type EncodeTFVarsFunc struct{}

func (f *EncodeTFVarsFunc) Name() string {
	return "encode_tfvars"
}

func (f *EncodeTFVarsFunc) GetFunctionSpec() providers.FunctionSpec {
	// TODO detailed specs
	params := []providers.FunctionParameterSpec{
		{
			Name: "input",
			// The input type is determined at runtime
			Type:              cty.DynamicPseudoType,
			Description:       "input to encode",
			DescriptionFormat: providers.TextFormattingPlain,
		},
	}
	return providers.FunctionSpec{
		Parameters:        params,
		Return:            cty.String,
		Summary:           "print string",
		Description:       "",
		DescriptionFormat: providers.TextFormattingPlain,
	}
}

func (f *EncodeTFVarsFunc) Call(args []cty.Value) (cty.Value, error) {
	log.Printf("[TRACE] args1: %v", args)
	return cty.StringVal(args[0].AsString()), nil
}

type DecodeTFVarsFunc struct{}

func (f *DecodeTFVarsFunc) Name() string {
	return "decode_tfvars"
}

func (f *DecodeTFVarsFunc) GetFunctionSpec() providers.FunctionSpec {
	// TODO detailed specs
	params := []providers.FunctionParameterSpec{
		{
			Name:              "input",
			Type:              cty.String,
			Description:       "input to decode",
			DescriptionFormat: providers.TextFormattingPlain,
		},
	}
	return providers.FunctionSpec{
		Parameters:        params,
		Return:            cty.DynamicPseudoType,
		Summary:           "",
		Description:       "",
		DescriptionFormat: providers.TextFormattingPlain,
	}
}

var FailedToDecodeError = errors.New("failed to decode tfvars content")

func wrapDiagErrors(m error, diag hcl.Diagnostics) error {
	//Prepend the main error
	errs := append([]error{m}, diag.Errs()...)
	return errors.Join(errs...)
}
func (f *DecodeTFVarsFunc) Call(args []cty.Value) (cty.Value, error) {
	//TODO Add logs
	varsFileContent := args[0].AsString()
	schema, diag := hclsyntax.ParseConfig([]byte(varsFileContent), "", hcl.Pos{Line: 0, Column: 0})
	if schema == nil || diag.HasErrors() {
		return cty.NullVal(cty.DynamicPseudoType), wrapDiagErrors(FailedToDecodeError, diag)
	}
	attrs, diag := schema.Body.JustAttributes()
	if attrs == nil || diag.HasErrors() {
		return cty.NullVal(cty.DynamicPseudoType), wrapDiagErrors(FailedToDecodeError, diag)
	}
	vals := make(map[string]cty.Value)
	for name, attr := range attrs {
		val, diag := attr.Expr.Value(nil)
		if diag.HasErrors() {
			return cty.NullVal(cty.DynamicPseudoType), wrapDiagErrors(FailedToDecodeError, diag)
		}
		vals[name] = val
	}
	return cty.ObjectVal(vals), nil
}
