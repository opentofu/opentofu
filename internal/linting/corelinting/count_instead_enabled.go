// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package corelinting

import (
	"context"
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/linting"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

// CountInsteadEnabled is a linting rule that returns linting diagnostics if the given `count` meta-argument
// expression could be replaced by a `lifecycle.enabled` meta-argument instead.
func CountInsteadEnabled(
	ctx context.Context,
	targetRes addrs.ConfigResource,
	targetDeclRange hcl.Range,
	countExpr hcl.Expression) tfdiags.Diagnostics {
	exec := func(ruleID linting.RuleAddr, groupIDs ...linting.RuleAddr) tfdiags.Diagnostics {
		// could be replaced with `enabled = true` or `enabled = false`
		if !canLifecycleEnabledReplaceCountExpr(countExpr, cty.NumberIntVal(1)) &&
			!canLifecycleEnabledReplaceCountExpr(countExpr, cty.NumberIntVal(0)) {
			return nil
		}
		return tfdiags.New(
			tfdiags.LintMessage(
				ruleID,
				groupIDs,
				"Could use enabled instead of count",
				fmt.Sprintf(`%q uses "count" to choose between zero or one instances using a boolean expression. Consider using "enabled" in a "lifecycle" block instead.`, targetRes.String()),
				new(tfdiags.SourceRangeFromHCL(countExpr.Range())),
				new(tfdiags.SourceRangeFromHCL(targetDeclRange)),
			),
		)
	}
	return tfdiags.ExecuteLintRule(ctx, exec, tfdiags.SourceRangeFromHCL(targetDeclRange), ruleIDcountInsteadOfEnabled, GroupIDImprovement)
}

// canLifecycleEnabledReplaceCountExpr is a function that returns true if the given `count` meta-argument expression
// could be replaced by a `lifecycle.enabled` one instead.
// The current rules which qualifies an expression as a candidate for such a case are as follows:
//   - the expression is a literal value and its value is a number equals with 1
//   - the expression is a literal value and its value is a number equals with 0
//   - the expression is a ternary operation and one of the expressions is a literal number equals with 1 and the other one
//     is a literal number equals with 0
func canLifecycleEnabledReplaceCountExpr(expr hcl.Expression, wantedVal cty.Value) bool {
	switch e := expr.(type) {
	case *hclsyntax.ConditionalExpr:
		zeroVal := cty.NumberIntVal(0)
		oneVal := cty.NumberIntVal(1)
		return (canLifecycleEnabledReplaceCountExpr(e.TrueResult, oneVal) && canLifecycleEnabledReplaceCountExpr(e.FalseResult, zeroVal)) ||
			(canLifecycleEnabledReplaceCountExpr(e.TrueResult, zeroVal) && canLifecycleEnabledReplaceCountExpr(e.FalseResult, oneVal))
	case *hclsyntax.ParenthesesExpr:
		return canLifecycleEnabledReplaceCountExpr(e.Expression, wantedVal)
	case *hclsyntax.LiteralValueExpr:
		if e.Val.RawEquals(wantedVal) {
			return true
		}
		return false
	}
	// anything else that is not covered in the switch block above cannot be part of an expression that
	// would indicate that `count` could be replaced with `lifecycle.enabled`.
	return false
}
