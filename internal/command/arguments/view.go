// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"strings"

	"github.com/opentofu/opentofu/internal/tofu"
)

// View represents the global command-line arguments which configure the view.
type View struct {
	// NoColor is used to disable the use of terminal color codes in all
	// output.
	NoColor bool

	// CompactWarnings is used to coalesce duplicate warnings, to reduce the
	// level of noise when multiple instances of the same warning are raised
	// for a configuration.
	CompactWarnings     bool
	ConsolidateWarnings bool
	ConsolidateErrors   bool

	// Concise is used to reduce the level of noise in the output and display
	// only the important details.
	Concise bool

	// ModuleDeprecationWarnLvl is used to filter out deprecation warnings for outputs and variables as requested by the user.
	ModuleDeprecationWarnLvl tofu.DeprecationWarningLevel

	// ShowSensitive is used to display the value of variables marked as sensitive.
	ShowSensitive bool
}

// ParseView processes CLI arguments, returning a View value and a
// possibly-modified slice of arguments. If any of the supported flags are
// found, they will be removed from the slice.
func ParseView(args []string) (*View, []string) {
	common := &View{
		ConsolidateWarnings: true,
	}

	// Keep track of the length of the returned slice. When we find an
	// argument we support, "i" will not be incremented.
	i := 0
	for _, v := range args {
		if prefix := "-deprecation=module:"; strings.HasPrefix(v, prefix) {
			common.ModuleDeprecationWarnLvl = tofu.ParseDeprecatedWarningLevel(strings.ReplaceAll(v, prefix, ""))
			continue // continue to ensure that the counter is not incremented
		}
		switch v {
		case "-no-color":
			common.NoColor = true
		case "-compact-warnings":
			common.CompactWarnings = true
		case "-consolidate-warnings":
			common.ConsolidateWarnings = true
		case "-consolidate-warnings=true":
			common.ConsolidateWarnings = true
		case "-consolidate-warnings=false":
			common.ConsolidateWarnings = false
		case "-consolidate-errors":
			common.ConsolidateErrors = true
		case "-consolidate-errors=true":
			common.ConsolidateErrors = true
		case "-consolidate-errors=false":
			common.ConsolidateErrors = false
		case "-concise":
			common.Concise = true
		default:
			// Unsupported argument: move left to the current position, and
			// increment the index.
			args[i] = v
			i++
		}
	}

	// Reduce the slice to the number of unsupported arguments. Any remaining
	// to the right of "i" have already been moved left.
	args = args[:i]

	return common, args
}
