package tf

import (
	"errors"
	"fmt"
	"log"

	"github.com/hashicorp/hcl/v2/hclwrite"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
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
	decodeTFVars := &DecodeTFVarsFunc{}
	encodeTFVars := &EncodeTFVarsFunc{}
	encodeExpr := &EncodeExprFunc{}
	return map[string]providerFunc{
		decodeTFVars.Name(): decodeTFVars,
		encodeTFVars.Name(): encodeTFVars,
		encodeExpr.Name():   encodeExpr,
	}
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
	//https://pkg.go.dev/github.com/hashicorp/hcl/v2/hclwrite
	toEncode := args[0]
	if !toEncode.Type().IsObjectType() {
		return cty.NullVal(cty.String), errors.New("input is not an object") //TODO errors
	}
	ef := hclwrite.NewEmptyFile()
	body := ef.Body()

	// Iterate over the elements of the input value
	it := toEncode.ElementIterator()
	for it.Next() {
		key, val := it.Element()
		log.Printf("[TRACE] key: %v, val: %v", key, val)
		// Check if the key is a string, known and not null, otherwise AsString method panics
		if !key.Type().Equals(cty.String) || !key.IsKnown() || key.IsNull() {
			return cty.NullVal(cty.String), fmt.Errorf("key is not a string: %v", key) //TODO errors
		}
		body.SetAttributeValue(key.AsString(), val)
	}
	b := ef.Bytes()
	log.Printf("[TRACE] encoded: %s", b)
	return cty.StringVal(string(b)), nil
}

type EncodeExprFunc struct{}

func (f *EncodeExprFunc) Name() string {
	return "encode_expr"
}

func (f *EncodeExprFunc) GetFunctionSpec() providers.FunctionSpec {
	// TODO detailed specs
	params := []providers.FunctionParameterSpec{
		{
			Name:              "input",
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

func (f *EncodeExprFunc) Call(args []cty.Value) (cty.Value, error) {
	//TODO add helpful logs
	toEncode := args[0]
	nf := hclwrite.NewEmptyFile()
	if !toEncode.IsWhollyKnown() {
		return cty.NullVal(cty.String), errors.New("input is not known") //TODO errors
	}
	tokens := hclwrite.TokensForValue(toEncode)
	log.Printf("[TRACE] tokens %+v", tokens)
	body := nf.Body()
	body.AppendUnstructuredTokens(tokens)
	return cty.StringVal(string(nf.Bytes())), nil
}
