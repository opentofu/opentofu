// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package core

import (
	"context"
	"fmt"

	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/linting"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

// RootVariableWithNoType is a linting rule that checks any root "variable" block for a defined
// "type". If none is present, then it issues a new linting diagnostic.
func RootVariableWithNoType(ctx context.Context, vc *configs.Variable) tfdiags.Diagnostic {
	if !tfdiags.LintRuleEnabledOnSource(ctx, tfdiags.SourceRangeFromHCL(vc.DeclRange), variableWithNoTypeRuleID, GroupIDConfusing) {
		return nil
	}
	if vc.Type == cty.DynamicPseudoType {
		return tfdiags.LintMessage(
			variableWithNoTypeRuleID,
			[]linting.RuleAddr{GroupIDConfusing},
			"Variable with no type",
			fmt.Sprintf("Variable %q has no type specified.", vc.Name),
			new(tfdiags.SourceRangeFromHCL(vc.DeclRange)),
			nil,
		)
	}
	return nil
}
