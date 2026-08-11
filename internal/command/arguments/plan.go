// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
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

	// View represents the global view options
	View *View
}

// BindPlan registers CLI arguments, returning a Plan value and it's corresponding hooks.
func BindPlan(cli *CommandLine) *Plan {
	plan := Plan{
		View:      BindView(cli, viewFlagAll),
		Operation: BindOperation(cli),
		Vars:      BindVars(cli),
		State:     BindState(cli, stateFlagAll),
	}

	cli.BoolVar(&plan.DetailedExitCode, "detailed-exitcode", false,
		`Return detailed exit codes when the command exits. The detailed exit codes are:
  0 - Succeeded but no changes proposed
  1 - Planning failed with an error
  2 - Succeeded and changes are proposed`)
	cli.StringVar(&plan.OutPath, "out", "",
		`Write a plan file to the given path. This can be used as input to the "apply" command.`,
	).SetDisplay("=path")
	cli.StringVar(&plan.GenerateConfigPath, "generate-config-out", "",
		`(Experimental) If import blocks are present in configuration, instructs OpenTofu to generate HCL for any imported resources not already present. The configuration is written to a new file at PATH, which must not already exist.
OpenTofu may still attempt to write configuration if planning fails with an error.`,
	).SetDisplay("=path")

	// Special handling for flag groups!
	for _, name := range []string{"destroy", "refresh-only", "refresh", "replace", "target", "target-file", "exclude", "exclude-file", "var", "var-file"} {
		cli.Flags[name].SetGroup("plan")
	}
	cli.FlagGroups = []FlagGroup{{
		ID:          "plan",
		Title:       "Plan Customization Options:",
		Description: `The following options customize how OpenTofu will produce its plan. You can also use these options when you run "tofu apply" without passing it a saved plan, in order to plan and apply in a single command.`,
	}, {
		Title: "Other Options:",
	}}

	return &plan

}

// ParsePlan processes CLI arguments, returning a Plan value, a closer function, and errors.
// If errors are encountered, a Plan value is still returned representing
// the best effort interpretation of the arguments.
func ParsePlan(args []string) (*Plan, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	plan := BindPlan(cli)
	closer, diags := cli.parseWithHooks("plan", args)
	return plan, closer, diags
}
