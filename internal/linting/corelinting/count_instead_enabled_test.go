// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package corelinting

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/linting"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestCoreRule_CountInsteadEnabled(t *testing.T) {
	resSetup := func(t *testing.T, rawExpr string) (addrs.ConfigResource, hcl.Range, hcl.Expression) {
		targetRes := addrs.MustParseResourceAddr("test.resource_name")
		targetResRange := hcl.Range{Filename: "test.tf", Start: hcl.Pos{Line: 1, Byte: 4, Column: 4}, End: hcl.Pos{Line: 1, Byte: 10, Column: 10}}
		expr, exprDiags := hclsyntax.ParseExpression([]byte(rawExpr), "test.tf", hcl.Pos{Line: 1, Byte: 4, Column: 4})
		if exprDiags.HasErrors() {
			t.Fatalf("test setup failed: %s", exprDiags)
		}
		return targetRes, targetResRange, expr
	}
	cases := map[string]struct {
		setup func(t *testing.T) (addrs.ConfigResource, hcl.Range, hcl.Expression, tfdiags.Diagnostics)
	}{
		"expression is a literal number 1": {
			setup: func(t *testing.T) (addrs.ConfigResource, hcl.Range, hcl.Expression, tfdiags.Diagnostics) {
				targetRes, targetResRange, expr := resSetup(t, "1")

				wantDiags := tfdiags.New(tfdiags.LintMessage(
					ruleIDcountInsteadOfEnabled,
					[]linting.RuleAddr{GroupIDImprovement},
					"Could use enabled instead of count",
					fmt.Sprintf(`%q uses "count" to choose between zero or one instances using a boolean expression. Consider using "enabled" in a "lifecycle" block instead.`, targetRes.String()),
					new(tfdiags.SourceRangeFromHCL(expr.Range())),
					new(tfdiags.SourceRangeFromHCL(targetResRange)),
				))
				return targetRes, targetResRange, expr, wantDiags
			},
		},
		"expression is a literal number 0": {
			setup: func(t *testing.T) (addrs.ConfigResource, hcl.Range, hcl.Expression, tfdiags.Diagnostics) {
				targetRes, targetResRange, expr := resSetup(t, "0")

				wantDiags := tfdiags.New(tfdiags.LintMessage(
					ruleIDcountInsteadOfEnabled,
					[]linting.RuleAddr{GroupIDImprovement},
					"Could use enabled instead of count",
					fmt.Sprintf(`%q uses "count" to choose between zero or one instances using a boolean expression. Consider using "enabled" in a "lifecycle" block instead.`, targetRes.String()),
					new(tfdiags.SourceRangeFromHCL(expr.Range())),
					new(tfdiags.SourceRangeFromHCL(targetResRange)),
				))
				return targetRes, targetResRange, expr, wantDiags
			},
		},
		"expression is a ternary condition with the truthy expression as a literal number 1 and the falsy expression is a literal number 0": {
			setup: func(t *testing.T) (addrs.ConfigResource, hcl.Range, hcl.Expression, tfdiags.Diagnostics) {
				targetRes, targetResRange, expr := resSetup(t, "var.input ? 1 : 0")

				wantDiags := tfdiags.New(tfdiags.LintMessage(
					ruleIDcountInsteadOfEnabled,
					[]linting.RuleAddr{GroupIDImprovement},
					"Could use enabled instead of count",
					fmt.Sprintf(`%q uses "count" to choose between zero or one instances using a boolean expression. Consider using "enabled" in a "lifecycle" block instead.`, targetRes.String()),
					new(tfdiags.SourceRangeFromHCL(expr.Range())),
					new(tfdiags.SourceRangeFromHCL(targetResRange)),
				))
				return targetRes, targetResRange, expr, wantDiags
			},
		},
		"expression is a ternary condition with the truthy expression as a literal number 0 and the falsy expression is a literal number 1": {
			setup: func(t *testing.T) (addrs.ConfigResource, hcl.Range, hcl.Expression, tfdiags.Diagnostics) {
				targetRes, targetResRange, expr := resSetup(t, "var.input ? 0 : 1")

				wantDiags := tfdiags.New(tfdiags.LintMessage(
					ruleIDcountInsteadOfEnabled,
					[]linting.RuleAddr{GroupIDImprovement},
					"Could use enabled instead of count",
					fmt.Sprintf(`%q uses "count" to choose between zero or one instances using a boolean expression. Consider using "enabled" in a "lifecycle" block instead.`, targetRes.String()),
					new(tfdiags.SourceRangeFromHCL(expr.Range())),
					new(tfdiags.SourceRangeFromHCL(targetResRange)),
				))
				return targetRes, targetResRange, expr, wantDiags
			},
		},
		"expression is a ternary condition with the truthy expression as a literal number 1 and the falsy expression is a literal number 2": {
			setup: func(t *testing.T) (addrs.ConfigResource, hcl.Range, hcl.Expression, tfdiags.Diagnostics) {
				targetRes, targetResRange, expr := resSetup(t, "var.input ? 1 : 2")

				var wantDiags tfdiags.Diagnostics
				return targetRes, targetResRange, expr, wantDiags
			},
		},
		"expression is a ternary condition with the truthy expression as a literal number 2 and the falsy expression is a literal number 1": {
			setup: func(t *testing.T) (addrs.ConfigResource, hcl.Range, hcl.Expression, tfdiags.Diagnostics) {
				targetRes, targetResRange, expr := resSetup(t, "var.input ? 2 : 1")

				var wantDiags tfdiags.Diagnostics
				return targetRes, targetResRange, expr, wantDiags
			},
		},
		"expression is a multi-layered ternary condition with the truthy expression as a ternary condition whose truthy is a literal number 1 and the falsy expression is a literal number 0": {
			setup: func(t *testing.T) (addrs.ConfigResource, hcl.Range, hcl.Expression, tfdiags.Diagnostics) {
				targetRes, targetResRange, expr := resSetup(t, "var.input ? (var.input2 ? 1 : 0) : 0")

				wantDiags := tfdiags.New(tfdiags.LintMessage(
					ruleIDcountInsteadOfEnabled,
					[]linting.RuleAddr{GroupIDImprovement},
					"Could use enabled instead of count",
					fmt.Sprintf(`%q uses "count" to choose between zero or one instances using a boolean expression. Consider using "enabled" in a "lifecycle" block instead.`, targetRes.String()),
					new(tfdiags.SourceRangeFromHCL(expr.Range())),
					new(tfdiags.SourceRangeFromHCL(targetResRange)),
				))
				return targetRes, targetResRange, expr, wantDiags
			},
		},
		"expression is a multi-layered ternary condition with the falsy expression as a ternary condition whose truthy is a literal number 1 and the falsy expression is a literal number 0": {
			setup: func(t *testing.T) (addrs.ConfigResource, hcl.Range, hcl.Expression, tfdiags.Diagnostics) {
				targetRes, targetResRange, expr := resSetup(t, "(var.input ? 0 : (var.input2 ? 1 : 0))")

				wantDiags := tfdiags.New(tfdiags.LintMessage(
					ruleIDcountInsteadOfEnabled,
					[]linting.RuleAddr{GroupIDImprovement},
					"Could use enabled instead of count",
					fmt.Sprintf(`%q uses "count" to choose between zero or one instances using a boolean expression. Consider using "enabled" in a "lifecycle" block instead.`, targetRes.String()),
					new(tfdiags.SourceRangeFromHCL(expr.Range())),
					new(tfdiags.SourceRangeFromHCL(targetResRange)),
				))
				return targetRes, targetResRange, expr, wantDiags
			},
		},
		"expression is a template expression": {
			// this is not possible since such an expression will fail nonetheless but we want to check the correctness
			// of the linting rule implementation and the way it handles invalid expressions
			setup: func(t *testing.T) (addrs.ConfigResource, hcl.Range, hcl.Expression, tfdiags.Diagnostics) {
				targetRes, targetResRange, expr := resSetup(t, "\"my_value\"")

				var wantDiags tfdiags.Diagnostics
				return targetRes, targetResRange, expr, wantDiags
			},
		},
		"expression is a literal bool": {
			// this is not possible since such an expression will fail nonetheless but we want to check the correctness
			// of the linting rule implementation and the way it handles invalid expressions
			setup: func(t *testing.T) (addrs.ConfigResource, hcl.Range, hcl.Expression, tfdiags.Diagnostics) {
				targetRes, targetResRange, expr := resSetup(t, "true")

				var wantDiags tfdiags.Diagnostics
				return targetRes, targetResRange, expr, wantDiags
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			runAgainstMultipleIdentifiers(t, func(t *testing.T, ctx context.Context) {
				targetRes, targetResRange, expr, expectedDiags := tc.setup(t)
				gotDiags := CountInsteadEnabled(ctx, targetRes, targetResRange, expr)
				compareDiagnostics(t, expectedDiags, gotDiags)
			}, []linting.RuleAddr{ruleIDcountInsteadOfEnabled}, []linting.RuleAddr{GroupIDImprovement})
		})
	}
}
