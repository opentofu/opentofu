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

	// ViewType specifies which output format to use: human, JSON, or "raw".
	ViewType ViewType

	Vars *Vars

	// ShowSensitive is used to display the value of variables marked as sensitive.
	ShowSensitive bool
}

// ShowTargetType represents the type of object that is requested to be
// shown by the "tofu show" command.
type ShowTargetType int

//go:generate go run golang.org/x/tools/cmd/stringer -type=ShowTargetType
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
)

// ParseShow processes CLI arguments, returning a Show value and errors.
// If errors are encountered, a Show value is still returned representing
// the best effort interpretation of the arguments.
func ParseShow(args []string) (*Show, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	show := &Show{
		Vars: &Vars{},
	}

	var jsonOutput bool
	var stateTarget bool
	var planTarget string
	cmdFlags := extendedFlagSet("show", nil, nil, show.Vars)
	cmdFlags.BoolVar(&jsonOutput, "json", false, "json")
	cmdFlags.BoolVar(&show.ShowSensitive, "show-sensitive", false, "displays sensitive values")
	cmdFlags.BoolVar(&stateTarget, "state", false, "show the latest state snapshot")
	cmdFlags.StringVar(&planTarget, "plan", "", "show the plan from a saved plan file")

	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line options",
			err.Error(),
		))
	}

	switch {
	case jsonOutput:
		show.ViewType = ViewJSON
	default:
		show.ViewType = ViewHuman
	}

	if planTarget == "" && !stateTarget {
		// If neither of the target type options was provided then we're
		// in the legacy mode where the target type is implied by
		// the number of arguments.
		args = cmdFlags.Args()
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
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Too many command line arguments",
				"Expected at most one positional argument for the legacy positional argument mode.",
			))
		}
		return show, diags
	}

	// The following handles the modern mode where the target type is
	// chosen based on which target type option was used.
	if len(cmdFlags.Args()) != 0 {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Unexpected command line arguments",
			"This command does not expect any positional arguments when using a target-selection option.",
		))
		return show, diags
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
	if targetTypes != 1 {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Conflicting object types to show",
			"The -state and -plan=FILENAME options are mutually-exclusive, to specify which kind of object to show.",
		))
	}
	return show, diags
}
