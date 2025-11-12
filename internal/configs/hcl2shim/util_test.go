// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package hcl2shim

import (
	"reflect"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	hclJSON "github.com/hashicorp/hcl/v2/json"
)

func TestConvertJSONExpressionToHCL(t *testing.T) {
	tests := []struct {
		Input string
	}{
		{
			Input: "hello",
		},
		{
			Input: "resource.test_resource[0]",
		},
	}

	for _, test := range tests {
		JSONExpr, diags := hclJSON.ParseExpression([]byte(`"`+test.Input+`"`), "")
		if diags.HasErrors() {
			t.Errorf("got %d diagnostics; want 0", len(diags))
			for _, d := range diags {
				t.Logf("  - %s", d.Error())
			}
		}

		want, diags := hclsyntax.ParseExpression([]byte(test.Input), "", hcl.Pos{Line: 1, Column: 1})
		if diags.HasErrors() {
			t.Errorf("got %d diagnostics; want 0", len(diags))
			for _, d := range diags {
				t.Logf("  - %s", d.Error())
			}
		}

		t.Run(test.Input, func(t *testing.T) {
			resultExpr, diags := ConvertJSONExpressionToHCL(JSONExpr)
			if diags.HasErrors() {
				t.Errorf("got %d diagnostics; want 0", len(diags))
				for _, d := range diags {
					t.Logf("  - %s", d.Error())
				}
			}

			if !reflect.DeepEqual(resultExpr, want) {
				t.Errorf("got %s, but want %s", resultExpr, want)
			}
		})
	}
}

func TestConvertJSONExpressionToHCL_Invalid(t *testing.T) {
	tests := []struct {
		Input    string
		WantDiag string
	}{
		{
			Input:    "123",
			WantDiag: "This value must be a string, but got number.",
		},
		{
			Input:    "null",
			WantDiag: "This value must be a string, but got dynamic.",
		},
	}

	for _, test := range tests {
		JSONExpr, diags := hclJSON.ParseExpression([]byte(test.Input), "")
		if diags.HasErrors() {
			t.Errorf("got %d diagnostics; want 0", len(diags))
			for _, d := range diags {
				t.Logf("  - %s", d.Error())
			}
		}

		t.Run(test.Input, func(t *testing.T) {
			resultExpr, diags := ConvertJSONExpressionToHCL(JSONExpr)
			if resultExpr != nil {
				t.Errorf("expected nil result but got %v", resultExpr)
			}
			if !diags.HasErrors() {
				t.Errorf("got none, but want error")
			}
			if len(diags) != 1 {
				t.Errorf("got %d diagnostics; want 1", len(diags))
			}
			gotDetail := diags[0].Detail
			if gotDetail != test.WantDiag {
				t.Errorf("wrong diagnostic detail\ngot:  %s\nwant: %s", gotDetail, test.WantDiag)
			}
		})
	}
}
