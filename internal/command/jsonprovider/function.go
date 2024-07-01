// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package jsonprovider

import (
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/zclconf/go-cty/cty"
)

const (
	mapTypeName   = "map"
	listTypeName  = "list"
	setTypeName   = "set"
	tupleTypeName = "tuple"
)

// Function is the top-level object returned when exporting function schemas
type Function struct {
	Description       string           `json:"description"`
	Summary           string           `json:"summary"`
	ReturnType        any              `json:"return_type"`
	Parameters        []*FunctionParam `json:"parameters,omitempty"`
	VariadicParameter *FunctionParam   `json:"variadic_parameter,omitempty"`
}

// FunctionParam is the object for wrapping the functions parameters and return types
type FunctionParam struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        any    `json:"type"`
	IsNullable  *bool  `json:"is_nullable,omitempty"`
}

func marshalReturnType(returnType cty.Type) any {
	switch {
	case returnType.IsObjectType():
		return []any{
			returnType.FriendlyName(),
			returnType.AttributeTypes(),
		}
	case returnType.IsListType():
		return []any{
			listTypeName,
			returnType.ListElementType(),
		}
	case returnType.IsMapType():
		return []any{
			mapTypeName,
			returnType.MapElementType(),
		}
	case returnType.IsSetType():
		return []any{
			setTypeName,
			returnType.SetElementType(),
		}
	case returnType.IsTupleType():
		return []any{
			tupleTypeName,
			returnType.TupleElementTypes(),
		}
	default:
		return returnType.FriendlyName()
	}
}

func marshalParameter(parameter providers.FunctionParameterSpec) *FunctionParam {
	var output FunctionParam
	output.Description = parameter.Description
	output.Name = parameter.Name
	output.Type = marshalReturnType(parameter.Type)

	if parameter.AllowNullValue {
		isNullable := true
		output.IsNullable = &isNullable
	}

	return &output
}

func marshalParameters(parameters []providers.FunctionParameterSpec) []*FunctionParam {
	output := make([]*FunctionParam, 0, len(parameters))
	for _, parameter := range parameters {
		output = append(output, marshalParameter(parameter))
	}

	return output
}

func marshalFunction(function providers.FunctionSpec) *Function {
	var output Function
	output.Description = function.Description
	output.Summary = function.Summary
	output.ReturnType = marshalReturnType(function.Return)
	output.Parameters = marshalParameters(function.Parameters)
	if function.VariadicParameter != nil {
		output.VariadicParameter = marshalParameter(*function.VariadicParameter)
	}

	return &output
}

func marshalFunctions(functions map[string]providers.FunctionSpec) map[string]*Function {
	if functions == nil {
		return map[string]*Function{}
	}
	output := make(map[string]*Function, len(functions))
	for k, v := range functions {
		output[k] = marshalFunction(v)
	}
	return output
}
