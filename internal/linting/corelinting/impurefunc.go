// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package corelinting

import (
	"context"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/opentofu/opentofu/internal/linting"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// ImpureFuncs is a linting rule that generates linting warnings when usage of impure functions is detected inside
// resource arguments.
// This relies on marks.LintingInfo to provide a list of impure functions usage linting information that was
// extracted from the final configuration of a resource.
//
// For information on which functions are impure, see internal/lang/functions.go#impureFunctions.
func ImpureFuncs(ctx context.Context, targetDeclRange hcl.Range, funcsUsed marks.LintingInfo) tfdiags.Diagnostics {
	exec := func(ruleID linting.RuleAddr, groupIDs ...linting.RuleAddr) tfdiags.Diagnostics {
		input := funcsUsed.ImpureFuncUsage()
		// when there is no information for this particular linting rule, skip marking the rule executed
		// for this particular resource since the reason for this could be that the values in the resource
		// body were in an unknown state so those might miss the required marks.
		// If we would allow this rule to be executed, it will be skipped later when the actual values are generated
		// and the marks could be determined reliably.
		if len(input) == 0 {
			return tfdiags.New(tfdiags.SkipLintingExecution())
		}
		var diags tfdiags.Diagnostics
		for _, i := range input {
			diags = diags.Append(tfdiags.LintMessage(
				ruleID,
				groupIDs,
				i.Summary(),
				i.Detail(),
				new(tfdiags.SourceRangeFromHCL(targetDeclRange)),
				nil,
			))
		}

		return diags
	}
	return tfdiags.ExecuteLintRule(ctx, exec, tfdiags.SourceRangeFromHCL(targetDeclRange), ruleIDImpureFuncUsed, GroupIDNoConverge)
}
