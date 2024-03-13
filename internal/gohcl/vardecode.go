// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// This is a fork of hashicorp/hcl/gohcl/decode.go that pulls out variable dependencies in attributes

package gohcl

import (
	"fmt"
	"reflect"

	"github.com/hashicorp/hcl/v2"
)

func VariablesInBody(body hcl.Body, val interface{}) ([]hcl.Traversal, hcl.Diagnostics) {
	rv := reflect.ValueOf(val)
	if rv.Kind() != reflect.Ptr {
		panic(fmt.Sprintf("target value must be a pointer, not %s", rv.Type().String()))
	}

	return findVariablesInBody(body, rv.Elem())
}

func findVariablesInBody(body hcl.Body, val reflect.Value) ([]hcl.Traversal, hcl.Diagnostics) {
	et := val.Type()
	switch et.Kind() {
	case reflect.Struct:
		return findVariablesInBodyStruct(body, val)
	case reflect.Map:
		return findVariablesInBodyMap(body, val)
	default:
		panic(fmt.Sprintf("target value must be pointer to struct or map, not %s", et.String()))
	}
}

func findVariablesInBodyStruct(body hcl.Body, val reflect.Value) ([]hcl.Traversal, hcl.Diagnostics) {
	var variables []hcl.Traversal

	schema, partial := ImpliedBodySchema(val.Interface())

	var content *hcl.BodyContent
	var diags hcl.Diagnostics
	if partial {
		content, _, diags = body.PartialContent(schema)
	} else {
		content, diags = body.Content(schema)
	}
	if content == nil {
		return variables, diags
	}

	tags := getFieldTags(val.Type())

	for name := range tags.Attributes {
		attr := content.Attributes[name]
		if attr != nil {
			variables = append(variables, attr.Expr.Variables()...)
		}
	}

	blocksByType := content.Blocks.ByType()

	for typeName, fieldIdx := range tags.Blocks {
		blocks := blocksByType[typeName]
		field := val.Type().Field(fieldIdx)

		ty := field.Type
		if ty.Kind() == reflect.Slice {
			ty = ty.Elem()
		}
		if ty.Kind() == reflect.Ptr {
			ty = ty.Elem()
		}

		for _, block := range blocks {
			blockVars, blockDiags := findVariablesInBody(block.Body, reflect.New(ty).Elem())
			variables = append(variables, blockVars...)
			diags = append(diags, blockDiags...)
		}

	}

	return variables, diags
}

func findVariablesInBodyMap(body hcl.Body, v reflect.Value) ([]hcl.Traversal, hcl.Diagnostics) {
	var variables []hcl.Traversal

	attrs, diags := body.JustAttributes()
	if attrs == nil {
		return variables, diags
	}

	for _, attr := range attrs {
		variables = append(variables, attr.Expr.Variables()...)
	}

	return variables, diags
}
