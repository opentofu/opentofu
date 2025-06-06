// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package depsrccfgs

import (
	"fmt"
	"strings"

	svchost "github.com/hashicorp/terraform-svchost"

	"github.com/opentofu/opentofu/internal/addrs"
)

// Wildcard is a special placeholder value used in both [ProviderAddrPattern]
// and [SourceAddrPattern] to represent address segments that are wildcarded,
// so that addresses under a particular prefix can all be systematically
// mapped to physical source locations using a single rule.
const Wildcard = "*"

// ProviderAddrPattern is a variant of [addrs.Provider] which extends the
// valid values to allow [Wildcard] to appear in place of specific literal
// values in each of the fields.
//
// Wildcarded elements must be contiguous at the start of the pattern.
// For example, "example.com/*/*" is valid but "*/foo/*" is not.
type ProviderAddrPattern addrs.Provider

func ParseProviderAddrPattern(src string) (ProviderAddrPattern, error) {
	ret := ProviderAddrPattern{}

	// This is largely the same as the addrs.Provider parser but additionally
	// allows "*" to appear in all positions, as long as all of the wildcard
	// parts are consecutive at the end of the address.
	parts := strings.Split(src, "/")
	if len(parts) != 3 {
		if len(parts) == 2 {
			// We don't support the shorthand that omits the hostname here,
			// to keep things as explicit as possible.
			return ret, fmt.Errorf("not enough address parts; if you intend to match providers on registry.opentofu.org then specify that prefix explicitly")
		}
		return ret, fmt.Errorf("provider address pattern must have three parts")
	}

	if parts[0] == "*" {
		ret.Hostname = svchost.Hostname(Wildcard)
	} else {
		hostname, err := svchost.ForComparison(parts[0])
		if err != nil {
			return ret, fmt.Errorf("invalid hostname: %w", err)
		}
		ret.Hostname = hostname
	}
	if parts[1] == "*" {
		ret.Namespace = Wildcard
	} else {
		namespace, err := addrs.ParseProviderPart(parts[1])
		if err != nil {
			return ret, fmt.Errorf("invalid namespace %q: %w", parts[1], err)
		}
		ret.Namespace = namespace
	}
	if parts[2] == "*" {
		ret.Type = Wildcard
	} else {
		typeName, err := addrs.ParseProviderPart(parts[2])
		if err != nil {
			return ret, fmt.Errorf("invalid type name %q: %w", parts[2], err)
		}
		ret.Namespace = typeName
	}

	// TODO: Verify that all of the wildcard segments are collected at the
	// suffix of the path, and return an error if not. Currently we'll
	// just accept invalid patterns with the rest of the system exhibiting
	// unspecified behavior if they are present.

	return ret, nil
}

func (p ProviderAddrPattern) Matches(provider addrs.Provider) bool {
	switch {
	case p.Hostname == svchost.Hostname(Wildcard):
		return true
	case p.Hostname != provider.Hostname:
		return false
	case p.Namespace == Wildcard:
		return true
	case p.Namespace != provider.Namespace:
		return false
	case p.Type == Wildcard:
		return true
	default:
		return p.Type == provider.Type
	}
}

func (p ProviderAddrPattern) Specificity() PatternSpecificity {
	if p.Hostname == svchost.Hostname(Wildcard) {
		return 0
	}
	if p.Namespace == Wildcard {
		return 1
	}
	if p.Type == Wildcard {
		return 2
	}
	return 3
}

// SourceAddrPattern is a variant of [addrs.ModuleRegistryPackage] which
// extends the valid values to allow [Wildcard] to appear in place of
// specific literal values in each of the fields.
//
// Wildcarded elements must be contiguous at the start of the pattern.
// For example, "example.com/*/*/*" is valid but "*/foo/*/*" is not.
type SourceAddrPattern addrs.ModuleRegistryPackage

func (p SourceAddrPattern) Specificity() PatternSpecificity {
	if p.Host == svchost.Hostname(Wildcard) {
		return 0
	}
	if p.Namespace == Wildcard {
		return 1
	}
	if p.Name == Wildcard {
		return 2
	}
	if p.TargetSystem == Wildcard {
		return 3
	}
	return 4
}

// PatternSpecificity specifies how many of the leading elements of a
// [ProviderAddrPattern] or [SourceAddrPattern] are literal rather than
// wildcarded.
//
// Rules with higher specificity take precedence over rules with lower
// specificity.
//
// The maximum value for [ProviderAddrPattern] is three, representing a
// fully-qualified provider address.
//
// The maximum value for [SourceAddrPattern] is four, representing a
// fully-qualified source address.
type PatternSpecificity int
