// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
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

// "decode_tfvars"
// "encode_tfvars"
// "encode_expr"

// decodeTFVarsFunc decodes a TFVars file content into a cty object
type decodeTFVarsFunc struct{}

func (f *decodeTFVarsFunc) Name() string {
	return "decode_tfvars"
}

func (f *decodeTFVarsFunc) GetFunctionSpec() providers.FunctionSpec {
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

var errFailedToDecode = errors.New("failed to decode tfvars content")

func wrapDiagErrors(m error, diag hcl.Diagnostics) error {
	//Prepend the main error
	errs := append([]error{m}, diag.Errs()...)
	return errors.Join(errs...)
}

func (f *decodeTFVarsFunc) Call(args []cty.Value) (cty.Value, error) {
	varsFileContent := args[0].AsString()
	schema, diag := hclsyntax.ParseConfig([]byte(varsFileContent), "", hcl.Pos{Line: 0, Column: 0})
	if schema == nil || diag.HasErrors() {
		return cty.NullVal(cty.DynamicPseudoType), wrapDiagErrors(errFailedToDecode, diag)
	}
	attrs, diag := schema.Body.JustAttributes()
	// Check if there are any errors.
	// attrs == nil does not mean that there are no attributes, attrs - is still initialized as an empty map
	if attrs == nil || diag.HasErrors() {
		return cty.NullVal(cty.DynamicPseudoType), wrapDiagErrors(errFailedToDecode, diag)
	}
	vals := make(map[string]cty.Value)
	for name, attr := range attrs {
		val, diag := attr.Expr.Value(nil)
		if diag.HasErrors() {
			return cty.NullVal(cty.DynamicPseudoType), wrapDiagErrors(errFailedToDecode, diag)
		}
		vals[name] = val
	}
	return cty.ObjectVal(vals), nil
}

// encodeTFVarsFunc encodes an object into a string with the same format as a TFVars file
type encodeTFVarsFunc struct{}

func (f *encodeTFVarsFunc) Name() string {
	return "encode_tfvars"
}

func (f *encodeTFVarsFunc) GetFunctionSpec() providers.FunctionSpec {
	params := []providers.FunctionParameterSpec{
		{
			Name: "input",
			// The input type is determined at runtime
			Type:              cty.DynamicPseudoType,
			Description:       "Input to encode for TFVars file. Must be an object with key that are valid identifiers",
			DescriptionFormat: providers.TextFormattingPlain,
		},
	}
	return providers.FunctionSpec{
		Parameters:        params,
		Return:            cty.String,
		Summary:           "Encode an object into a string with the same format as a TFVars file",
		Description:       "provider::terraform::encode_tfvars encodes an object into a string with the same format as a TFVars file",
		DescriptionFormat: providers.TextFormattingPlain,
	}
}

var errInvalidInput = errors.New("invalid input")

func (f *encodeTFVarsFunc) Call(args []cty.Value) (cty.Value, error) {
	toEncode := args[0]
	// null is invalid input
	if toEncode.IsNull() {
		return cty.NullVal(cty.String), fmt.Errorf("%w: must not be null", errInvalidInput)
	}
	if !toEncode.Type().IsObjectType() {
		return cty.NullVal(cty.String), fmt.Errorf("%w: must be an object", errInvalidInput)
	}
	ef := hclwrite.NewEmptyFile()
	body := ef.Body()

	// Iterate over the elements of the input value
	it := toEncode.ElementIterator()
	for it.Next() {
		key, val := it.Element()
		// Check if the key is a string, known and not null, otherwise AsString method panics
		if !key.Type().Equals(cty.String) || !key.IsKnown() || key.IsNull() {
			return cty.NullVal(cty.String), fmt.Errorf("%w: object key must be a string: %v", errInvalidInput, key)
		}
		name := key.AsString()
		if valid := hclsyntax.ValidIdentifier(name); !valid {
			return cty.NullVal(cty.String), fmt.Errorf("%w: object key: %s - must be a valid identifier", errInvalidInput, name)
		}
		body.SetAttributeValue(key.AsString(), val)
	}
	b := ef.Bytes()
	return cty.StringVal(string(b)), nil
}

// encodeExprFunc encodes an expression into a string
type encodeExprFunc struct{}

func (f *encodeExprFunc) Name() string {
	return "encode_expr"
}

func (f *encodeExprFunc) GetFunctionSpec() providers.FunctionSpec {
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
		Summary:           "Takes an arbitrary expression and converts it into a string with valid OpenTofu syntax",
		Description:       "provider::terraform::encode_expr takes an arbitrary expression and converts it into a string with valid OpenTofu syntax",
		DescriptionFormat: providers.TextFormattingPlain,
	}
}

var errUnknownInput = errors.New("input is not wholly known")

func (f *encodeExprFunc) Call(args []cty.Value) (cty.Value, error) {
	toEncode := args[0]
	nf := hclwrite.NewEmptyFile()
	if !toEncode.IsWhollyKnown() {
		return cty.NullVal(cty.String), errUnknownInput
	}
	tokens := hclwrite.TokensForValue(toEncode)
	body := nf.Body()
	body.AppendUnstructuredTokens(tokens)
	return cty.StringVal(string(nf.Bytes())), nil
}
