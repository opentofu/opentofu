// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package corelinting

import "github.com/opentofu/opentofu/internal/linting"

// This block below is meant to hold all the "core" namespaced linting **rule IDs**.

var (
	variableWithNoTypeRuleID    = linting.MustParseRuleAddr("core:no-type-variable")
	ruleIDcountInsteadOfEnabled = linting.MustParseRuleAddr("core:count-instead-enabled")
)

// This block below is meant to hold all the "core" namespaced linting **group IDs**.

var (
	GroupIDConfusing   = linting.MustParseRuleAddr("core:confusing")
	GroupIDImprovement = linting.MustParseRuleAddr("core:improvement")
)
