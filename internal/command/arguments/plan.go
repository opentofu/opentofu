// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/spf13/cobra"
)

// Plan represents the command-line arguments for the plan command.
type Plan struct {
	// State, Operation, and Vars are the common extended flags
	State     *State
	Operation *Operation
	Vars      *Vars

	// DetailedExitCode enables different exit codes for error, success with
	// changes, and success with no changes.
	DetailedExitCode bool

	// OutPath contains an optional path to store the plan file
	OutPath string

	// GenerateConfigPath tells OpenTofu that config should be generated for
	// unmatched import target paths and which path the generated file should
	// be written to.
	GenerateConfigPath string

	// ViewOptions specifies which view options to use
	ViewOptions ViewOptions

	// ShowSensitive is used to display the value of variables marked as sensitive.
	ShowSensitive bool
}

func AttachPlan(cmd *cobra.Command) *Plan {
	plan := &Plan{
		State:     &State{},
		Operation: &Operation{},
		Vars:      &Vars{},
	}

	cextendedFlagSet(cmd.Flags(), "plan", plan.Operation, plan.Vars)
	plan.State.addFlags(cmd.Flags(), stateFlagAll)
	cmd.Flags().BoolVar(&plan.DetailedExitCode, "detailed-exitcode", false,
		`Return detailed exit codes when the command exits. The detailed exit codes are:
 0 - Succeeded but no changes proposed
 1 - Planning failed with an error
 2 - Succeeded and changes are proposed`)
	cmd.Flags().StringVar(&plan.OutPath, "out", "",
		`Write a plan file to the given path. This can be used as input to the "apply" command.`)
	cmd.Flags().StringVar(&plan.GenerateConfigPath, "generate-config-out", "",
		`(Experimental) If import blocks are present in configuration, instructs OpenTofu to generate HCL for any imported resources not already present. The configuration is written to a new file at PATH, which must not already exist. 
OpenTofu may still attempt to write configuration if planning fails with an error.`)
	cmd.Flags().BoolVar(&plan.ShowSensitive, "show-sensitive", false,
		`If specified, sensitive values will not be redacted in te UI output.`)

	plan.ViewOptions.AddFlags(cmd.Flags(), true)

	AddPre(cmd, func() tfdiags.Diagnostics {
		var diags tfdiags.Diagnostics
		diags = diags.Append(plan.Operation.Parse())
		closer, moreDiags := plan.ViewOptions.Parse()
		diags = diags.Append(moreDiags)
		AddPost(cmd, func() tfdiags.Diagnostics {
			closer()
			return nil
		})
		return diags
	})

	return plan
}

// ParsePlan processes CLI arguments, returning a Plan value, a closer function, and errors.
// If errors are encountered, a Plan value is still returned representing
// the best effort interpretation of the arguments.
func ParsePlan(args []string) (*Plan, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	plan := &Plan{
		State:     &State{},
		Operation: &Operation{},
		Vars:      &Vars{},
	}

	cmdFlags := extendedFlagSet("plan", plan.Operation, plan.Vars)
	plan.State.addFlags(cmdFlags, stateFlagAll)
	cmdFlags.BoolVar(&plan.DetailedExitCode, "detailed-exitcode", false, "detailed-exitcode")
	cmdFlags.StringVar(&plan.OutPath, "out", "", "out")
	cmdFlags.StringVar(&plan.GenerateConfigPath, "generate-config-out", "", "generate-config-out")
	cmdFlags.BoolVar(&plan.ShowSensitive, "show-sensitive", false, "displays sensitive values")

	plan.ViewOptions.AddFlags(cmdFlags, true)

	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line flags",
			err.Error(),
		))
	}

	args = cmdFlags.Args()

	if len(args) > 0 {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Too many command line arguments",
			"To specify a working directory for the plan, use the global -chdir flag.",
		))
	}

	diags = diags.Append(plan.Operation.Parse())
	closer, moreDiags := plan.ViewOptions.Parse()
	diags = diags.Append(moreDiags)

	return plan, closer, diags
}
