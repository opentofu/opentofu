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

	// View represents the global view options
	View *View
	// Vars and State are the common extended flags
	Vars  *Vars
	State *State
}

// BindOutput registers CLI arguments, returning a Output value and it's corresponding hooks.
func BindOutput(cli *CommandLine) *Output {
	output := Output{
		View:  BindView(cli, viewFlagNoInput|viewFlagSensitive),
		Vars:  BindVars(cli),
		State: BindState(cli, stateFlagStateIn),
	}

	rawOutput := false
	cli.BoolVar(&rawOutput, "raw", false, `For value types that can be automatically converted to a string, will print the raw string directly, rather than a human-oriented representation of the value.

Use this with care when stdout is a terminal and when the output value might contain control characters.`)

	cli.ArgHelp = "The output command expects exactly one argument with the name of an output variable or no arguments to show all outputs."
	cli.PositionalArg(&output.Name, "NAME", true)

	cli.PreHook(func() tfdiags.Diagnostics {
		var diags tfdiags.Diagnostics
		if rawOutput {
			jsonSet := output.View.ViewType == ViewJSON
			output.View.ViewType = ViewRaw
			if jsonSet {
				diags = diags.Append(tfdiags.Sourceless(
					tfdiags.Error,
					"Invalid output format",
					"The -raw and -json options are mutually-exclusive.",
				))

				// Since the desired output format is unknowable, fall back to default
				output.View.ViewType = ViewHuman
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
	})

	return &output
}

// ParseOutput processes CLI arguments, returning an Output value, a closer function, and errors.
// If errors are encountered, an Output value is still returned representing
// the best effort interpretation of the arguments.
func ParseOutput(args []string) (*Output, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	output := BindOutput(cli)
	closer, diags := cli.parseWithHooks("output", args)
	return output, closer, diags
}
