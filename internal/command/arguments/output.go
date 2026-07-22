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
func BindOutput(flags Flags, raw *bool) (*Output, Hooks) {
	var output Output
	var hooks Hooks

	output.ViewOptions.bind(flags, false)
	hooks = append(hooks, output.ViewOptions.ParseHook())

	output.Vars = &Vars{}
	output.Vars.bind(flags)

	output.State = &State{}
	output.State.bind(flags, stateFlagStateIn)

	flags.BoolVar(raw, "raw", false, "For value types that can be automatically converted to a string, will print the raw string directly, rather than a human-oriented representation of the value.")
	flags.BoolVar(&output.ShowSensitive, "show-sensitive", false, "If specified, sensitive values will be displayed.")

	return &output, hooks
}

// ParseOutput processes CLI arguments, returning an Output value, a closer function, and errors.
// If errors are encountered, an Output value is still returned representing
// the best effort interpretation of the arguments.
func ParseOutput(args []string) (*Output, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	rawOutput := false

	flags := Flags{}
	output, hooks := BindOutput(flags, &rawOutput)

	cmdFlags := defaultFlagSet("output", flags)

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

	// TODO positional arguments
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

	return output, func() { hooks.Post() }, diags.Append(hooks.Pre())
}
