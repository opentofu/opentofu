// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Output represents the command-line arguments for the output command.
type Output struct {
	// Name identifies which root module output to show.  If empty, show all
	// outputs.
	Name string

	// StatePath is an optional path to a state file, from which outputs will
	// be loaded.
	StatePath string

	// ViewOptions specifies which view options to use
	ViewOptions ViewOptions

	Vars *Vars

	// ShowSensitive is used to display the value of variables marked as sensitive.
	ShowSensitive bool
}

// ParseOutput processes CLI arguments, returning an Output value, a closer function, and errors.
// If errors are encountered, an Output value is still returned representing
// the best effort interpretation of the arguments.
func ParseOutput(args []string) (*Output, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	output := &Output{
		Vars: &Vars{},
	}

	var rawOutput bool
	var statePath string
	cmdFlags := extendedFlagSet("output", nil, nil, output.Vars)
	cmdFlags.BoolVar(&rawOutput, "raw", false, "raw")
	cmdFlags.StringVar(&statePath, "state", "", "path")
	cmdFlags.BoolVar(&output.ShowSensitive, "show-sensitive", false, "displays sensitive values")

	output.ViewOptions.AddFlags(cmdFlags, false)

	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line flags",
			err.Error(),
		))
	}

	args = cmdFlags.Args()
	if len(args) > 1 {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Unexpected argument",
			"The output command expects exactly one argument with the name of an output variable or no arguments to show all outputs.",
		))
	}

	closer, moreDiags := output.ViewOptions.Parse()
	diags = diags.Append(moreDiags)
	if rawOutput {
		output.ViewOptions.ViewType = ViewRaw
		if output.ViewOptions.jsonFlag {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Invalid output format",
				"The -raw and -json options are mutually-exclusive.",
			))

			// Since the desired output format is unknowable, fall back to default
			output.ViewOptions.ViewType = ViewHuman
			rawOutput = false
		}
	}

	output.StatePath = statePath

	if len(args) > 0 {
		output.Name = args[0]
	}

	if rawOutput && output.Name == "" {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Output name required",
			"You must give the name of a single output value when using the -raw option.",
		))
	}

	return output, closer, diags
}
