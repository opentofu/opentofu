// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

//go:generate go run golang.org/x/tools/cmd/stringer -type=DeprecationWarningLevel deprecation_level.go

import (
	"log"

	"github.com/opentofu/opentofu/internal/addrs"
)

// DeprecationWarningLevel defines the levels that can be given by the user to control the level of what deprecation
// warnings should be shown.
type DeprecationWarningLevel uint8

const (
	DeprecationWarningLevelAll DeprecationWarningLevel = iota
	DeprecationWarningLevelLocal
)

func variableDeprecationWarnAllowed(lvl DeprecationWarningLevel, source addrs.ModuleSource) bool {
	switch lvl {
	case DeprecationWarningLevelLocal:
		_, ok := source.(addrs.ModuleSourceLocal)
		return ok
	default:
		return true
	}
}

// ParseDeprecatedWarningLevel gets in a string and returns a DeprecationWarningLevel.
// Since these warnings are not critical to the system, this method is returning no error when the
// warn level identifier is missing a mapping. Instead, it falls back on returning the level that
// will write all the deprecation warnings.
func ParseDeprecatedWarningLevel(s string) DeprecationWarningLevel {
	switch s {
	case "local":
		return DeprecationWarningLevelLocal
	case "all":
		return DeprecationWarningLevelAll
	default:
		log.Printf(
			"[WARN] ParseDeprecatedWarningLevel: returning %s deprecation warn level since the given value is unknown: %s", // TODO stringer generator
			DeprecationWarningLevelAll,
			s,
		)
		return DeprecationWarningLevelAll
	}
}
