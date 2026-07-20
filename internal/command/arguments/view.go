// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
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
	ModuleDeprecationWarnLvl DeprecationWarningLevel

	// ShowSensitive is used to display the value of variables marked as sensitive.
	ShowSensitive bool
}

func BindView(flags Flags) *View {
	v := &View{}

	flags.BoolVar(&v.NoColor, "no-color", false,
		"Disable virtual terminal escape sequences.")
	flags.BoolVar(&v.CompactWarnings, "compact-warnings", false,
		"If OpenTofu produces any warnings that are not accompanied by errors, shows them in a more compact form that includes only the summary messages.")
	flags.BoolVar(&v.ConsolidateWarnings, "consolidate-warnings", true,
		"If OpenTofu produces any warnings, no consolidation will be performed. All locations, for all warnings will be listed. Enabled by default.",
	).SetDisplay("=false")
	flags.BoolVar(&v.ConsolidateErrors, "consolidate-errors", false,
		"If OpenTofu produces any errors, no consolidation will be performed. All locations, for all errors will be listed. Disabled by default.")
	flags.BoolVar(&v.Concise, "concise", false, "Disable progress-related messages.")
	flags.Func("deprecation",
		`Specify what type of warnings are shown. Accepted values for "m": all, local, none. Default: all. When "all" is selected, OpenTofu will show the deprecation warnings for all modules. When "local" is selected, the warns will be shown only for the modules that are imported with a relative path. When "none" is selected, all the deprecation warnings will be dropped.`,
		func(s string) error {
			prefix := "module:"
			if !strings.HasPrefix(s, prefix) {
				return fmt.Errorf("Expected prefix %q, got %q", prefix, s)
			}
			v.ModuleDeprecationWarnLvl = ParseDeprecatedWarningLevel(strings.ReplaceAll(s, prefix, ""))
			return nil
		}).SetDisplay("=module:m")

	return v
}

func AttachView(cmd *cobra.Command) *View {
	v := &View{}

	cmd.Flags().BoolVar(&v.NoColor, "no-color", false,
		"Disable virtual terminal escape sequences.")
	cmd.Flags().BoolVar(&v.CompactWarnings, "compact-warnings", false,
		"If OpenTofu produces any warnings that are not accompanied by errors, shows them in a more compact form that includes only the summary messages.")
	cmd.Flags().BoolVar(&v.ConsolidateWarnings, "consolidate-warnings", true,
		"If OpenTofu produces any warnings, no consolidation will be performed. All locations, for all warnings will be listed. Enabled by default.")
	cmd.Flags().BoolVar(&v.ConsolidateErrors, "consolidate-errors", false,
		"If OpenTofu produces any errors, no consolidation will be performed. All locations, for all errors will be listed. Disabled by default.")
	cmd.Flags().BoolVar(&v.Concise, "concise", false, "Disable progress-related messages.")
	cmd.Flags().Func("deprecation",
		`Specify what type of warnings are shown. Accepted values are: module:all, module:local, module:none. Default: all. When "all" is selected, OpenTofu will show the deprecation warnings for all modules. When "local" is selected, the warns will be shown only for the modules that are imported with a relative path. When "none" is selected, all the deprecation warnings will be dropped.`,
		func(s string) error {
			prefix := "module:"
			if !strings.HasPrefix(s, prefix) {
				return fmt.Errorf("Expected prefix %q, got %q", prefix, s)
			}
			v.ModuleDeprecationWarnLvl = ParseDeprecatedWarningLevel(strings.ReplaceAll(s, prefix, ""))
			return nil
		})

	return v
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
			common.ModuleDeprecationWarnLvl = ParseDeprecatedWarningLevel(strings.ReplaceAll(v, prefix, ""))
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
