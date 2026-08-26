// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package corelinting

import (
	"context"
	"fmt"
	"iter"

	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/linting"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// UnusedVariables generates a linting diagnostic for a not used variable.
// This gets an iter.Seq that can load data lazily only when the linting rule is actually enabled.
func UnusedVariables(ctx context.Context, vars iter.Seq[*configs.Variable]) tfdiags.Diagnostics {
	if !tfdiags.LintRuleEnabled(ctx, ruleIDVariableNotUsed, GroupIDImprovement) {
		return nil
	}
	var diags tfdiags.Diagnostics
	for vc := range vars {
		exec := func(ruleID linting.RuleAddr, groupIDs ...linting.RuleAddr) tfdiags.Diagnostics {
			return tfdiags.New(tfdiags.LintMessage(
				ruleID,
				groupIDs,
				"Input variable not used",
				fmt.Sprintf("Found no usage of the variable %q", vc.Name),
				new(tfdiags.SourceRangeFromHCL(vc.DeclRange)),
				nil,
			))
		}
		diags = diags.Append(tfdiags.ExecuteLintRule(ctx, exec, tfdiags.SourceRangeFromHCL(vc.DeclRange), ruleIDVariableNotUsed, GroupIDImprovement))
	}

	return diags
}

// UnusedLocal generates a linting diagnostic for a not used local.
// This gets an iter.Seq that can load data lazily only when the linting rule is actually enabled.
func UnusedLocal(ctx context.Context, locals iter.Seq[*configs.Local]) tfdiags.Diagnostics {
	if !tfdiags.LintRuleEnabled(ctx, ruleIDLocalNotUsed, GroupIDImprovement) {
		return nil
	}
	var diags tfdiags.Diagnostics
	for lc := range locals {
		exec := func(ruleID linting.RuleAddr, groupIDs ...linting.RuleAddr) tfdiags.Diagnostics {
			return tfdiags.New(tfdiags.LintMessage(
				ruleID,
				groupIDs,
				"Local value not used",
				fmt.Sprintf("Found no usage of the local value %q", lc.Name),
				new(tfdiags.SourceRangeFromHCL(lc.DeclRange)),
				nil,
			))
		}
		diags = diags.Append(tfdiags.ExecuteLintRule(ctx, exec, tfdiags.SourceRangeFromHCL(lc.DeclRange), ruleIDLocalNotUsed, GroupIDImprovement))
	}

	return diags
}
