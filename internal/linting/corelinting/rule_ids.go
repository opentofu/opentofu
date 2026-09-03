// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package corelinting

import "github.com/opentofu/opentofu/internal/linting"

// This block below is meant to hold all the "core" namespaced linting **rule IDs**.

var (
	ruleIDUntypedVariable       = linting.MustParseRuleAddr("core:no-type-variable")
	ruleIDCountInsteadOfEnabled = linting.MustParseRuleAddr("core:count-instead-enabled")
	ruleIDVariableNotUsed       = linting.MustParseRuleAddr("core:unused-variable")
	ruleIDLocalNotUsed          = linting.MustParseRuleAddr("core:unused-local")
	ruleIDImpureFuncUsed        = linting.MustParseRuleAddr("core:impurefunc")
)

// This block below is meant to hold all the "core" namespaced linting **group IDs**.

var (
	// GroupIDAll is the rule that should be assigned to any core linting rule
	GroupIDAll         = linting.MustParseRuleAddr("core:all")
	GroupIDConfusing   = linting.MustParseRuleAddr("core:confusing")
	GroupIDImprovement = linting.MustParseRuleAddr("core:improvement")
	GroupIDNoConverge  = linting.MustParseRuleAddr("core:noconverge")
)
