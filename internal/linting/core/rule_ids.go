// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package core

import "github.com/opentofu/opentofu/internal/linting"

// This block below is meant to hold all the "core" namespaced linting **rule IDs**.

var (
	variableWithNoTypeRuleID = linting.MustParseRuleAddr("no-type-variable")
)

// This block below is meant to hold all the "core" namespaced linting **group IDs**.

var (
	GroupIDConfusing = linting.MustParseRuleAddr("confusing")
)
