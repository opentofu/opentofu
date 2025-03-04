// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"fmt"
	"log"

	"github.com/opentofu/opentofu/internal/addrs"
)

const (
	deprecatedWarningLvlLocal = "local"
	deprecatedWarningLvlAll   = "all"
)

// DeprecatedWarningLevel defines the contract of a deprecation warning level.
// When modules used by a user are having variables/outputs that are marked as deprecated, and
// the module user is referencing those, the user is able to control
// what kind of warns can be shown. This is what the deprecation warning levels are for.
type DeprecatedWarningLevel interface {
	fmt.Stringer
	VariableDeprecationAllowed(source addrs.ModuleSource) bool
}

// localDeprecatedWarningLevel is allowing showing deprecation warnings only for variables/outputs
// that are referenced from local module calls.
// This will filter out any warning that could be generated for a remote module call.
type localDeprecatedWarningLevel struct{}

func (l localDeprecatedWarningLevel) VariableDeprecationAllowed(source addrs.ModuleSource) bool {
	_, ok := source.(addrs.ModuleSourceLocal)
	return ok
}

func (l localDeprecatedWarningLevel) String() string {
	return deprecatedWarningLvlLocal
}

// allDeprecatedWarningLevel is the level that shows all the deprecation warnings.
type allDeprecatedWarningLevel struct{}

func (r allDeprecatedWarningLevel) VariableDeprecationAllowed(_ addrs.ModuleSource) bool {
	return true
}

func (r allDeprecatedWarningLevel) String() string {
	return deprecatedWarningLvlAll
}

// ParseDeprecatedWarningLevel gets in a string and returns a DeprecatedWarningLevel instance.
// Since these warnings are not critical to the system, this method is returning no error when the
// warn level identifier is missing a mapping. Instead, it falls back on returning the level that
// will write all the deprecation warns.
func ParseDeprecatedWarningLevel(s string) DeprecatedWarningLevel {
	switch s {
	case deprecatedWarningLvlLocal:
		return &localDeprecatedWarningLevel{}
	case deprecatedWarningLvlAll:
		return &allDeprecatedWarningLevel{}
	default:
		log.Printf(
			"[WARN] ParseDeprecatedWarningLevel: returning %s deprecation warn level since the given value is unknown: %s",
			deprecatedWarningLvlAll,
			s,
		)
		return &allDeprecatedWarningLevel{}
	}
}
