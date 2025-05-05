// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

//go:generate go run golang.org/x/tools/cmd/stringer -type=DeprecationWarningLevel deprecation_level.go

import (
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"log"

	"github.com/opentofu/opentofu/internal/addrs"
)

// DeprecationWarningLevel defines the levels that can be given by the user to control the level of what deprecation
// warnings should be shown.
type DeprecationWarningLevel uint8

const (
	DeprecationWarningLevelAll DeprecationWarningLevel = iota
	DeprecationWarningLevelLocal
	DeprecationWarningLevelNone
)

func variableDeprecationWarnAllowed(lvl DeprecationWarningLevel, source addrs.ModuleSource) bool {
	switch lvl {
	case DeprecationWarningLevelNone:
		return false
	case DeprecationWarningLevelLocal:
		_, ok := source.(addrs.ModuleSourceLocal)
		return ok
	default:
		return true
	}
}

func DeprecatedOutputDiagAllowed(lvl DeprecationWarningLevel, diagnostic tfdiags.Diagnostic) bool {
	switch lvl {
	case DeprecationWarningLevelAll:
		return true
	case DeprecationWarningLevelLocal:
		cause, ok := marks.DiagnosticDeprecationCause(diagnostic)
		if !ok {
			// If it's not a deprecation warning diagnostic, always allow it to not filter out diagnostics unrelated with deprecation
			return true
		}
		if cause.IsFromRemoteModule { // do not allow deprecation warnings for outputs from remote module calls
			return false
		}
		return true
	case DeprecationWarningLevelNone:
		_, ok := marks.DiagnosticDeprecationCause(diagnostic)
		// Always ignore the deprecation warnings if it's having a deprecation cause
		return !ok
	default:
		return true
	}
}

// ParseDeprecatedWarningLevel gets in a string and returns a DeprecationWarningLevel.
// Since these warnings are not critical to the system, this method is returning no error when the
// warn level identifier is missing a mapping. Instead, it falls back on returning the level that
// will write all the deprecation warnings.
//
// This function is purposely not handling DeprecationWarningLevelNone as we don't want the users to be able to disable the warnings entirely.
// In case it's decided later to allow users to disable this type of warnings entirely, just adding DeprecationWarningLevelNone here
// should be enough.
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
