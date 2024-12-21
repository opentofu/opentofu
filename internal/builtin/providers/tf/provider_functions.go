// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package tf

import (
	"errors"
	"fmt"

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
	// Name returns the name of the function which is used to call it
	Name() string
	// GetFunctionSpec returns the provider function specification
	GetFunctionSpec() providers.FunctionSpec
	// Call is used to invoke the function
	Call(args []cty.Value) (cty.Value, error)
}

// getProviderFuncs returns a map of functions that are registered in the provider
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

// DecodeTFVarsFunc decodes a TFVars file content into a cty object
type DecodeTFVarsFunc struct{}

func (f *DecodeTFVarsFunc) Name() string {
	return "decode_tfvars"
}

func (f *DecodeTFVarsFunc) GetFunctionSpec() providers.FunctionSpec {
	params := []providers.FunctionParameterSpec{
		{
			Name:              "content",
			Type:              cty.String,
			Description:       "TFVars file content to decode",
			DescriptionFormat: providers.TextFormattingPlain,
		},
	}
	return providers.FunctionSpec{
		Parameters:        params,
		Return:            cty.DynamicPseudoType,
		Summary:           "Decode a TFVars file content into an object",
		Description:       "provider::terraform::decode_tfvars decodes a TFVars file content into an object",
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
	varsFileContent := args[0].AsString()
	schema, diag := hclsyntax.ParseConfig([]byte(varsFileContent), "", hcl.Pos{Line: 0, Column: 0})
	if schema == nil || diag.HasErrors() {
		return cty.NullVal(cty.DynamicPseudoType), wrapDiagErrors(FailedToDecodeError, diag)
	}
	attrs, diag := schema.Body.JustAttributes()
	// Check if there are any errors.
	// attrs == nil does not mean that there are no attributes, attrs - is still initialized as an empty map
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

// EncodeTFVarsFunc encodes an object into a string with the same format as a TFVars file
type EncodeTFVarsFunc struct{}

func (f *EncodeTFVarsFunc) Name() string {
	return "encode_tfvars"
}

func (f *EncodeTFVarsFunc) GetFunctionSpec() providers.FunctionSpec {
	params := []providers.FunctionParameterSpec{
		{
			Name: "input",
			// The input type is determined at runtime
			Type:              cty.DynamicPseudoType,
			Description:       "input to encode for TFVars file. Must be an object with key that are valid identifiers",
			DescriptionFormat: providers.TextFormattingPlain,
		},
	}
	return providers.FunctionSpec{
		Parameters:        params,
		Return:            cty.String,
		Summary:           "encode an object into a string with the same format as a TFVars file",
		Description:       "provider::terraform::encode_tfvars encodes an object into a string with the same format as a TFVars file",
		DescriptionFormat: providers.TextFormattingPlain,
	}
}

var InvalidInputError = errors.New("invalid input")

func (f *EncodeTFVarsFunc) Call(args []cty.Value) (cty.Value, error) {
	toEncode := args[0]
	// null is invalid input
	if toEncode.IsNull() {
		return cty.NullVal(cty.String), fmt.Errorf("%w: must not be null", InvalidInputError)
	}
	if !toEncode.Type().IsObjectType() {
		return cty.NullVal(cty.String), fmt.Errorf("%w: must be an object", InvalidInputError)
	}
	ef := hclwrite.NewEmptyFile()
	body := ef.Body()

	// Iterate over the elements of the input value
	it := toEncode.ElementIterator()
	for it.Next() {
		key, val := it.Element()
		// Check if the key is a string, known and not null, otherwise AsString method panics
		if !key.Type().Equals(cty.String) || !key.IsKnown() || key.IsNull() {
			return cty.NullVal(cty.String), fmt.Errorf("%w: object key must be a string: %v", InvalidInputError, key) //TODO errors
		}
		name := key.AsString()
		if valid := hclsyntax.ValidIdentifier(name); !valid {
			return cty.NullVal(cty.String), fmt.Errorf("%w: object key: %s - must be a valid identifier", InvalidInputError, name)
		}
		body.SetAttributeValue(key.AsString(), val)
	}
	b := ef.Bytes()
	return cty.StringVal(string(b)), nil
}

// EncodeExprFunc encodes an expression into a string
type EncodeExprFunc struct{}

func (f *EncodeExprFunc) Name() string {
	return "encode_expr"
}

func (f *EncodeExprFunc) GetFunctionSpec() providers.FunctionSpec {
	params := []providers.FunctionParameterSpec{
		{
			Name:              "expr",
			Type:              cty.DynamicPseudoType,
			Description:       "expression to encode",
			DescriptionFormat: providers.TextFormattingPlain,
		},
	}
	return providers.FunctionSpec{
		Parameters:        params,
		Return:            cty.String,
		Summary:           "Takes any non-null expression and returns a string representation of it in a valid OpenTofu expression format",
		Description:       "provider::terraform::encode_expr takes any non-null expression and returns a string representation of it in a valid OpenTofu expression format",
		DescriptionFormat: providers.TextFormattingPlain,
	}
}

var UnknownInputError = errors.New("input is not known")

func (f *EncodeExprFunc) Call(args []cty.Value) (cty.Value, error) {
	toEncode := args[0]
	nf := hclwrite.NewEmptyFile()
	if !toEncode.IsWhollyKnown() {
		return cty.NullVal(cty.String), UnknownInputError
	}
	tokens := hclwrite.TokensForValue(toEncode)
	body := nf.Body()
	body.AppendUnstructuredTokens(tokens)
	return cty.StringVal(string(nf.Bytes())), nil
}
