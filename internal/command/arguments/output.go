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
	// ShowSensitive is used to display the value of variables marked as sensitive.
	ShowSensitive bool

	// ViewOptions specifies which view options to use
	ViewOptions ViewOptions
	// Vars and State are the common extended flags
	Vars  *Vars
	State *State
}

// BindOutput registers CLI arguments, returning a Output value and it's corresponding hooks.
func BindOutput(cli *CommandLine) *Output {
	var output Output

	output.ViewOptions.bind(cli, false)

	output.Vars = &Vars{}
	output.Vars.bind(cli)

	output.State = BindState(cli, stateFlagStateIn)

	rawOutput := false
	cli.BoolVar(&rawOutput, "raw", false, `For value types that can be automatically converted to a string, will print the raw string directly, rather than a human-oriented representation of the value.

Use this with care when stdout is a terminal and when the output value might contain control characters.`)
	cli.BoolVar(&output.ShowSensitive, "show-sensitive", false, "If specified, sensitive values will be displayed.")

	cli.ArgHelp = "The output command expects exactly one argument with the name of an output variable or no arguments to show all outputs."
	cli.PositionalArg(&output.Name, "NAME", true)

	cli.Hook(Hook{Pre: func() tfdiags.Diagnostics {
		var diags tfdiags.Diagnostics
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

		if rawOutput && output.Name == "" {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Output name required",
				"You must give the name of a single output value when using the -raw option.",
			))
		}
		return diags
	}})

	return &output
}

// ParseOutput processes CLI arguments, returning an Output value, a closer function, and errors.
// If errors are encountered, an Output value is still returned representing
// the best effort interpretation of the arguments.
func ParseOutput(args []string) (*Output, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	output := BindOutput(cli)
	closer, diags := cli.Stdlib("output", args)
	return output, closer, diags
}
