// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"log"
	"strings"

	"github.com/opentofu/opentofu/internal/collections"
	"github.com/opentofu/opentofu/internal/linting"
)

// ParseLintingRules is a utility function that coordinates the parsing of a raw string into structured linting rules.
//
// The lintingRulesRaw string argument is expected to contain a string separated list of linting rules
// to be executed or skipped.
//
//	Format: rule1,rule2,!rule3,!rule4
//
// which will translate to "include rule1 and rule2" and "exclude rule3 and rule4".
//
// Any parsing failure will be only logged. Due to the nature of the linting feature, we want to be
// permissive in this layer since migrating from one OpenTofu version to another some rules might be
// changed or removed which should not break the existing operators configuration.
//
// This function allows for the same rule to be included and excluded but will warn about it. Due to the
// filtering rules in the internal/tfdiags/lint.go, in situations where the same rule is included and excluded,
// the inclusion will take precedence.
func ParseLintingRules(lintingRulesRaw string) (collections.Set[linting.RuleAddr], collections.Set[linting.RuleAddr]) {
	include, exclude := collections.NewSet[linting.RuleAddr](), collections.NewSet[linting.RuleAddr]()
	rules := strings.Split(strings.TrimSpace(lintingRulesRaw), ",")
	for _, rawRule := range rules {
		rawRule = strings.TrimSpace(rawRule)
		if rawRule == "" {
			continue
		}
		t := include
		rule, found := strings.CutPrefix(rawRule, "!")
		if found {
			t = exclude
		}
		la, err := linting.ParseRuleAddr(rule)
		if err != nil {
			log.Printf("[WARN] Linting rule %q ignored since it's parsing failed: %s", rawRule, err)
			continue
		}
		t[la] = struct{}{}
	}
	for _, r := range include.Intersection(exclude) {
		log.Printf("[WARN] Linting rule %q included and excluded in the same time. This might create unwanted behavior", r)
	}

	return include, exclude
}
