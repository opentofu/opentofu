// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

//go:generate go tool golang.org/x/tools/cmd/stringer -type=DeprecationWarningLevel deprecation_level.go

import (
	"log"
)

// DeprecationWarningLevel defines different levels of deprecation warnings that can be used by the user to control
// what type of warnings it wants to see
type DeprecationWarningLevel uint8

const (
	// DeprecationWarningLevelAll shows all deprecation warnings for outputs and variables, where no filtering is applied.
	DeprecationWarningLevelAll DeprecationWarningLevel = iota
	// DeprecationWarningLevelLocal shows only the deprecation warnings for the outputs and variables that are coming from
	// modules that are referenced with a relative path (aka local module).
	DeprecationWarningLevelLocal
	// DeprecationWarningLevelNone disables any deprecation warnings, filtering out any diagnostic that was generated about
	// this.
	DeprecationWarningLevelNone
)

// ParseDeprecatedWarningLevel gets in a string and returns a DeprecationWarningLevel.
// Since these warnings are not critical to the system, this method is returning no error when the
// warn level identifier is missing a mapping. Instead, it falls back on returning the level that
// will write all the deprecation warnings.
func ParseDeprecatedWarningLevel(s string) DeprecationWarningLevel {
	switch s {
	// Adding also the empty string just to make it clear that empty string will result in DeprecationWarningLevelAll. Useful also for skipping the warn log in the default branch.
	case "all", "":
		return DeprecationWarningLevelAll
	case "local":
		return DeprecationWarningLevelLocal
	case "none":
		return DeprecationWarningLevelNone
	default:
		log.Printf(
			"[WARN] ParseDeprecatedWarningLevel: returning %s deprecation warn level since the given value is unknown: %s",
			DeprecationWarningLevelAll,
			s,
		)
		return DeprecationWarningLevelAll
	}
}
