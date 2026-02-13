// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hcltest"
	"github.com/opentofu/opentofu/internal/lang"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/zclconf/go-cty/cty"
)

func TestEvaluateImportIdExpression_SensitiveValue(t *testing.T) {
	ctx := &MockEvalContext{}
	ctx.installSimpleEval()
	ctx.EvaluationScopeScope = &lang.Scope{}

	testCases := []struct {
		name    string
		expr    hcl.Expression
		wantErr string
	}{
		{
			name:    "sensitive_value",
			expr:    hcltest.MockExprLiteral(cty.StringVal("value").Mark(marks.Sensitive)),
			wantErr: "Invalid import id argument: The import id cannot be sensitive.",
		},
		{
			name:    "ephemeral_value",
			expr:    hcltest.MockExprLiteral(cty.StringVal("value").Mark(marks.Ephemeral)),
			wantErr: "Invalid import id argument: The import id cannot be ephemeral.",
		},
		{
			name:    "expr_is_nil",
			expr:    nil,
			wantErr: "Invalid import id argument: The import id cannot be null.",
		},
		{
			name:    "evaluates_to_null",
			expr:    hcltest.MockExprLiteral(cty.NullVal(cty.String)),
			wantErr: "Invalid import id argument: The import id cannot be null.",
		},
		{
			name:    "evaluates_to_unknown",
			expr:    hcltest.MockExprLiteral(cty.UnknownVal(cty.String)),
			wantErr: "Invalid import id argument: The import block \"id\" argument depends on resource attributes that cannot be determined until apply, so OpenTofu cannot plan to import this resource.", // Adapted the message from your original code
		},
		{
			name:    "valid_value",
			expr:    hcltest.MockExprLiteral(cty.StringVal("value")),
			wantErr: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, diags := evaluateImportIdExpression(t.Context(), tc.expr, ctx, EvalDataForNoInstanceKey)

			if tc.wantErr != "" {
				if len(diags) != 1 {
					t.Errorf("expected diagnostics, got %s", spew.Sdump(diags))
				} else if diags.Err().Error() != tc.wantErr {
					t.Errorf("unexpected error diagnostic. wanted %q got %q", tc.wantErr, diags.Err().Error())
				}
			} else {
				if len(diags) != 0 {
					t.Errorf("unexpected diagnostics %s", spew.Sdump(diags))
				}
			}
		})
	}
}

// TestEvaluateImportIdentityExpression_NestedMarks verifies that sensitive and
// ephemeral marks are detected on nested attributes inside identity objects,
// not just on the top-level value.
//
// Without deep mark checking, an identity like:
//
//	import {
//	  identity = { id = var.secret_id }  // var.secret_id is sensitive
//	  to       = aws_instance.example
//	}
//
// would pass validation because the outer object is not marked -- only the
// nested "id" attribute carries the sensitive mark. This would leak the
// sensitive value into import state without any warning.
// This is mainly intended to ensure that we do not regress on this behaviour
func TestEvaluateImportIdentityExpression_NestedMarks(t *testing.T) {
	ctx := &MockEvalContext{}
	ctx.installSimpleEval()
	ctx.EvaluationScopeScope = &lang.Scope{}

	testCases := []struct {
		name    string
		expr    hcl.Expression
		wantErr string
	}{
		{
			name: "nested_sensitive_attribute",
			expr: hcltest.MockExprLiteral(cty.ObjectVal(map[string]cty.Value{
				"id": cty.StringVal("secret").Mark(marks.Sensitive),
			})),
			wantErr: "Invalid import identity argument: The import identity cannot be sensitive.",
		},
		{
			name: "nested_ephemeral_attribute",
			expr: hcltest.MockExprLiteral(cty.ObjectVal(map[string]cty.Value{
				"id": cty.StringVal("temp").Mark(marks.Ephemeral),
			})),
			wantErr: "Invalid import identity argument: The import identity cannot be ephemeral.",
		},
		{
			name: "top_level_sensitive_object",
			expr: hcltest.MockExprLiteral(cty.ObjectVal(map[string]cty.Value{
				"id": cty.StringVal("value"),
			}).Mark(marks.Sensitive)),
			wantErr: "Invalid import identity argument: The import identity cannot be sensitive.",
		},
		{
			name: "valid_identity",
			expr: hcltest.MockExprLiteral(cty.ObjectVal(map[string]cty.Value{
				"id": cty.StringVal("value"),
			})),
			wantErr: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, diags := evaluateImportIdentityExpression(t.Context(), tc.expr, ctx, EvalDataForNoInstanceKey, cty.DynamicPseudoType)

			if tc.wantErr != "" {
				if len(diags) != 1 {
					t.Errorf("expected diagnostics, got %s", spew.Sdump(diags))
				} else if diags.Err().Error() != tc.wantErr {
					t.Errorf("unexpected error diagnostic. wanted %q got %q", tc.wantErr, diags.Err().Error())
				}
			} else {
				if len(diags) != 0 {
					t.Errorf("unexpected diagnostics %s", spew.Sdump(diags))
				}
			}
		})
	}
}
