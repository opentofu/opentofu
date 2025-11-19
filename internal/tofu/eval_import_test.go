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
			wantErr: "Invalid import id argument: The import ID cannot be sensitive.",
		},
		{
			name:    "ephemeral_value",
			expr:    hcltest.MockExprLiteral(cty.StringVal("value").Mark(marks.Ephemeral)),
			wantErr: "Invalid import id argument: The import ID cannot be ephemeral.",
		},
		{
			name:    "expr_is_nil",
			expr:    nil,
			wantErr: "Invalid import id argument: The import ID cannot be null.",
		},
		{
			name:    "evaluates_to_null",
			expr:    hcltest.MockExprLiteral(cty.NullVal(cty.String)),
			wantErr: "Invalid import id argument: The import ID cannot be null.",
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
			_, diags := evaluateImportIdExpression(tc.expr, ctx, EvalDataForNoInstanceKey)

			if tc.wantErr != "" {
				if len(diags) != 1 {
					t.Errorf("expected diagnostics, got %s", spew.Sdump(diags))
				} else if diags.Err().Error() != tc.wantErr {
					t.Errorf("unexpected error diagnostic %s", diags.Err().Error())
				}
			} else {
				if len(diags) != 0 {
					t.Errorf("unexpected diagnostics %s", spew.Sdump(diags))
				}
			}
		})
	}
}
