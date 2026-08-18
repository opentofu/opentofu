// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Show represents the command-line arguments for the show command.
type Show struct {
	// TargetType and TargetArg together describe the object that was
	// requested to be shown.
	//
	// The meaning of TargetArg varies depending on TargetType. Refer to
	// the documentation for each [ShowTargetType] constant for details.
	TargetType ShowTargetType
	TargetArg  string

	// View represents the global view options
	View *View

	Vars *Vars
}

// ShowTargetType represents the type of object that is requested to be
// shown by the "tofu show" command.
type ShowTargetType int

//go:generate go tool golang.org/x/tools/cmd/stringer -type=ShowTargetType
const (
	// ShowUnknownType is the zero value of [ShowTargetType], and represents
	// that the target type is ambiguous and so must be inferred by the
	// caller based on the [Show.TargetArg] value.
	ShowUnknownType ShowTargetType = iota

	// ShowState represents a request to show the latest state snapshot.
	//
	// This target type does not use [Show.TargetArg].
	ShowState

	// ShowPlan represents a request to show a plan loaded from a saved
	// plan file.
	//
	// For this target type, [Show.TargetArg] is the plan file to load.
	ShowPlan

	// ShowConfig represents a request to show the current configuration.
	//
	// This target type does not use [Show.TargetArg].
	ShowConfig

	// ShowModule represents a request to show just one module in isolation,
	// without requiring any of its dependencies to be installed.
	//
	// For this target type, [Show.TargetArg] is a path to the directory
	// containing the module.
	ShowModule
)

// BindShow registers CLI arguments, returning a Show value and it's corresponding hooks.
func BindShow(cli *CommandLine) *Show {
	show := Show{
		View: BindView(cli, viewFlagNoInput|viewFlagSensitive),
		Vars: BindVars(cli),
	}

	targetFlagGroup := FlagGroup{
		ID:          "target",
		Title:       "Target selection options:",
		Description: `Use one of the following options to specify what to show.`,
		Suffix:      `If no target selection options are provided, -state is the default.`,
	}
	defaultFlagGroup := FlagGroup{
		ID:    "",
		Title: "Other options:",
	}
	cli.FlagGroups = []FlagGroup{targetFlagGroup, defaultFlagGroup}

	var stateTarget bool
	var planTarget string
	var configTarget bool
	var moduleTarget string
	var args []string

	cli.BoolVar(&stateTarget, "state", false, "The latest state snapshot, if any.").SetGroup(targetFlagGroup.ID)
	cli.StringVar(&planTarget, "plan", "", "The plan from a saved plan file.").SetDisplay("=FILENAME").SetGroup(targetFlagGroup.ID)
	cli.BoolVar(&configTarget, "config", false, "Show the current configuration (requires -json).").SetGroup(targetFlagGroup.ID)
	cli.StringVar(&moduleTarget, "module", "", "Show the specified module configuration (requires -json)").SetDisplay("=DIR").SetGroup(targetFlagGroup.ID)

	cli.VariadicArg(&args, "arguments")
	cli.PreHook(func() tfdiags.Diagnostics {
		// If -config or -module=... is selected, -json is required
		if configTarget && show.View.ViewType != ViewJSON {
			return tfdiags.New(tfdiags.Sourceless(
				tfdiags.Error,
				"JSON output required for configuration",
				"The -config option requires -json to be specified.",
			))
		}
		if moduleTarget != "" && show.View.ViewType != ViewJSON {
			return tfdiags.New(tfdiags.Sourceless(
				tfdiags.Error,
				"JSON output required for module",
				"The -module=DIR option requires -json to be specified.",
			))
		}

		if planTarget == "" && moduleTarget == "" && !stateTarget && !configTarget {
			// If none of the target type options was provided then we're
			// in the legacy mode where the target type is implied by
			// the number of arguments.
			switch len(args) {
			case 0:
				show.TargetType = ShowState
				show.TargetArg = ""
			case 1:
				// This case is ambiguous: the argument could be either
				// a saved plan file or a local state snapshot such as
				// the output from "tofu state pull". The caller will need
				// to probe TargetArg to decide which it is.
				show.TargetType = ShowUnknownType
				show.TargetArg = args[0]
			default:
				return tfdiags.New(tfdiags.Sourceless(
					tfdiags.Error,
					"Too many command line arguments",
					"Expected at most one positional argument for the legacy positional argument mode.",
				))
			}
			return nil
		}

		// The following handles the modern mode where the target type is
		// chosen based on which target type option was used.
		if len(args) != 0 {
			return tfdiags.New(tfdiags.Sourceless(
				tfdiags.Error,
				"Unexpected command line arguments",
				"This command does not expect any positional arguments when using a target-selection option.",
			))
		}
		targetTypes := 0
		if stateTarget {
			targetTypes++
			show.TargetType = ShowState
			show.TargetArg = ""
		}
		if planTarget != "" {
			targetTypes++
			show.TargetType = ShowPlan
			show.TargetArg = planTarget
		}
		if configTarget {
			targetTypes++
			show.TargetType = ShowConfig
			show.TargetArg = ""
		}
		if moduleTarget != "" {
			targetTypes++
			show.TargetType = ShowModule
			show.TargetArg = moduleTarget
		}
		if targetTypes != 1 {
			return tfdiags.New(tfdiags.Sourceless(
				tfdiags.Error,
				"Conflicting object types to show",
				"The -state, -plan=FILENAME, -config, and -module=DIR options are mutually-exclusive, to specify which kind of object to show.",
			))
		}
		return nil
	})

	return &show
}

// ParseShow processes CLI arguments, returning a Show value, a closer function, and errors.
// If errors are encountered, a Show value is still returned representing
// the best effort interpretation of the arguments.
func ParseShow(args []string) (*Show, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	show := BindShow(cli)
	closer, diags := cli.parseWithHooks("show", args)
	return show, closer, diags
}
