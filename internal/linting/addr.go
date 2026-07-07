// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package linting

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	ruleNamespaceSeparator = ":"
	ruleCoreNamespace      = "core"
	ruleDefaultNamespace   = ruleCoreNamespace
)

// AllRulesGroupID represents the inclusion of all rule and group linting IDs.
var AllRulesGroupID = RuleAddr{
	Name: "all",
}

// RuleAddr represents the identifier for an individual lint rule or for a
// group of lint rules.
type RuleAddr struct {
	Namespace string
	Name      string
}

func (ra RuleAddr) String() string {
	if ra.Namespace == "" {
		return ra.Name
	}
	return fmt.Sprintf("%s:%s", ra.Namespace, ra.Name)
}

// addrMatchingReg is used to validate the format of the lint rule ID.
// To summarise this:
// * both, the namespace and name, must start with a lower-case letter or a digit.
// * both, the namespace and name, can contain lower-case letters, digits, dashes and underscores.
// * because the namespace could point to a provider later, it should be able to contain also forward slashes.
var addrMatchingReg = regexp.MustCompile(`^([a-z0-9]+[a-z0-9_\-/]*:)?[a-z0-9]+[a-z0-9_\-]*$`)

// ParseRuleAddr parses a string into a [RuleAddr] struct.
// This is used to validate the given identifier and if all the validation passes, it will return the parsed [RuleAddr].
// The validations applied against the given rule id are explained in details on [addrMatchingReg], but in short, here are
// some valid formats:
//   - core:depends_on
//   - 0namespace:lint_rule_id
//   - namespace/full/path:0lines
func ParseRuleAddr(raw string) (RuleAddr, error) {
	if !addrMatchingReg.Match([]byte(raw)) {
		return RuleAddr{}, fmt.Errorf("invalid lint rule id %q. It does not match the required format or character restriction", raw)
	}
	parts := strings.Split(strings.TrimSpace(raw), ruleNamespaceSeparator)
	// since the regex above is meant to catch spaces and multiple namespace separators, there is no need to do
	// additional validations here

	ns := ruleDefaultNamespace
	ruleName := parts[len(parts)-1] // name is always the last one
	if len(parts) == 2 {
		ns = parts[0]
	}
	// special case for the "all" value. That is meant to include all of the existing linting rules so that is unnamespaced.
	if ruleName == AllRulesGroupID.Name {
		return AllRulesGroupID, nil
	}

	return RuleAddr{
		Namespace: ns,
		Name:      ruleName,
	}, nil
}

// MustParseRuleAddr is the same as ParseRuleAddr but it panics if a validation error occurs.
// This function is recommended to be used in tests and at init time.
func MustParseRuleAddr(raw string) RuleAddr {
	res, err := ParseRuleAddr(raw)
	if err != nil {
		panic(err)
	}
	return res
}
