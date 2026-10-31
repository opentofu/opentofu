// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package symlib

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

func TestTableTypeContext_missingLibrary(t *testing.T) {
	table := EmptyTable
	typeCtx := table.TypeContext()

	expr, diags := hclsyntax.ParseExpression([]byte("symbols::missing::my_type()"), "test.hcl", hcl.InitialPos)
	if diags.HasErrors() {
		t.Fatalf("failed to parse expression: %s", diags.Error())
	}

	ty, defaults, typeDiags := typeCtx.TypeConstraintWithDefaults(expr)
	if !typeDiags.HasErrors() {
		t.Fatalf("expected errors for missing library, got none")
	}
	if ty != cty.DynamicPseudoType {
		t.Errorf("expected DynamicPseudoType fallback, got %#v", ty)
	}
	if defaults != nil {
		t.Errorf("expected nil defaults, got %#v", defaults)
	}

	foundSummary := false
	for _, diag := range typeDiags {
		if diag.Summary == "Missing symbol library" {
			foundSummary = true
			break
		}
	}
	if !foundSummary {
		t.Errorf("expected 'Missing symbol library' diagnostic, got: %s", typeDiags.Error())
	}
}

func TestTableTypeContext_missingTypeInLibrary(t *testing.T) {
	table := Table{
		"mylib": {
			name:   "mylib",
			types:  map[string]typeWithDefault{},
			values: map[string]cty.Value{},
		},
	}
	typeCtx := table.TypeContext()

	expr, diags := hclsyntax.ParseExpression([]byte("symbols::mylib::nonexistent()"), "test.hcl", hcl.InitialPos)
	if diags.HasErrors() {
		t.Fatalf("failed to parse expression: %s", diags.Error())
	}

	ty, defaults, typeDiags := typeCtx.TypeConstraintWithDefaults(expr)
	if !typeDiags.HasErrors() {
		t.Fatalf("expected errors for missing type, got none")
	}
	if ty != cty.DynamicPseudoType {
		t.Errorf("expected DynamicPseudoType fallback, got %#v", ty)
	}
	if defaults != nil {
		t.Errorf("expected nil defaults, got %#v", defaults)
	}

	foundSummary := false
	for _, diag := range typeDiags {
		if diag.Summary == "Missing type" {
			foundSummary = true
			break
		}
	}
	if !foundSummary {
		t.Errorf("expected 'Missing type' diagnostic, got: %s", typeDiags.Error())
	}
}

func TestTableTypeContext_invalidCallStructure(t *testing.T) {
	table := EmptyTable
	typeCtx := table.TypeContext()

	tests := []struct {
		name string
		expr string
	}{
		{"two_segments", "symbols::invalid()"},
		{"four_segments", "symbols::a::b::c()"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, diags := hclsyntax.ParseExpression([]byte(tt.expr), "test.hcl", hcl.InitialPos)
			if diags.HasErrors() {
				t.Fatalf("failed to parse expression: %s", diags.Error())
			}

			ty, _, typeDiags := typeCtx.TypeConstraintWithDefaults(expr)
			if !typeDiags.HasErrors() {
				t.Fatalf("expected errors for invalid call structure, got none")
			}
			if ty != cty.DynamicPseudoType {
				t.Errorf("expected DynamicPseudoType fallback, got %#v", ty)
			}

			foundSummary := false
			for _, diag := range typeDiags {
				if diag.Summary == "Invalid type call structure" {
					foundSummary = true
					break
				}
			}
			if !foundSummary {
				t.Errorf("expected 'Invalid type call structure' diagnostic, got: %s", typeDiags.Error())
			}
		})
	}
}

func TestTableTypeContext_optionalAttributeWithDefault(t *testing.T) {
	// Regression test for issue #4733:
	// When an uninitialized or deleted symbol library is referenced inside an optional attribute
	// with a default value, typeexpr must not panic with:
	// "WithoutOptionalAttributesDeep does not support the given type"
	table := EmptyTable
	typeCtx := table.TypeContext()

	expr, diags := hclsyntax.ParseExpression([]byte(`object({
		attr = optional(symbols::missing::custom(), "default_val")
	})`), "test.hcl", hcl.InitialPos)
	if diags.HasErrors() {
		t.Fatalf("failed to parse expression: %s", diags.Error())
	}

	ty, defaults, typeDiags := typeCtx.TypeConstraintWithDefaults(expr)
	if !typeDiags.HasErrors() {
		t.Fatalf("expected errors for missing symbol library, got none")
	}
	if !ty.IsObjectType() {
		t.Fatalf("expected object type, got %#v", ty)
	}

	// Verify WithoutOptionalAttributesDeep can be called safely without panicking.
	unwrapped := ty.WithoutOptionalAttributesDeep()
	if !unwrapped.IsObjectType() {
		t.Fatalf("expected object type after WithoutOptionalAttributesDeep, got %#v", unwrapped)
	}

	attrTy := unwrapped.AttributeType("attr")
	if attrTy != cty.DynamicPseudoType {
		t.Errorf("expected DynamicPseudoType attribute type, got %#v", attrTy)
	}

	_ = defaults
}

func TestTableTypeContext_nilTable(t *testing.T) {
	var table Table
	typeCtx := table.TypeContext()

	expr, diags := hclsyntax.ParseExpression([]byte("symbols::missing::my_type()"), "test.hcl", hcl.InitialPos)
	if diags.HasErrors() {
		t.Fatalf("failed to parse expression: %s", diags.Error())
	}

	ty, _, typeDiags := typeCtx.TypeConstraintWithDefaults(expr)
	if !typeDiags.HasErrors() {
		t.Fatalf("expected errors for nil table, got none")
	}
	if ty != cty.DynamicPseudoType {
		t.Errorf("expected DynamicPseudoType fallback, got %#v", ty)
	}
}
