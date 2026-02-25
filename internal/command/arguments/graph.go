// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Graph represents the command-line arguments for the graph command.
type Graph struct {
	// DrawCycles highlights any cycles in the graph with colored edges.
	DrawCycles bool
	// GraphType specifies the type of graph to output (plan, plan-refresh-only, plan-destroy, or apply).
	GraphType string
	// ModuleDepth specifies the depth of modules to show in the output.
	ModuleDepth int
	// Verbose enables verbose output.
	Verbose bool
	// PlanPath specifies the path to a plan file to render the graph from.
	PlanPath string

	// ViewOptions specifies which view options to use
	ViewOptions ViewOptions

	// Vars holds and provides information for the flags related to variables that a user can give into the process
	Vars *Vars
}

// ParseGraph processes CLI arguments, returning a Graph value, a closer function, and errors.
// If errors are encountered, a Graph value is still returned representing
// the best effort interpretation of the arguments.
func ParseGraph(args []string) (*Graph, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	arguments := &Graph{
		Vars: &Vars{},
	}

	cmdFlags := extendedFlagSet("graph", nil, nil, arguments.Vars)
	cmdFlags.BoolVar(&arguments.DrawCycles, "draw-cycles", false, "draw-cycles")
	cmdFlags.StringVar(&arguments.GraphType, "type", "", "type")
	cmdFlags.IntVar(&arguments.ModuleDepth, "module-depth", -1, "module-depth")
	cmdFlags.BoolVar(&arguments.Verbose, "verbose", false, "verbose")
	cmdFlags.StringVar(&arguments.PlanPath, "plan", "", "plan")

	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line flags",
			err.Error(),
		))
	}

	// we only parse but do not register the views flags since this command does not need it
	closer, moreDiags := arguments.ViewOptions.Parse()
	diags = diags.Append(moreDiags)
	if diags.HasErrors() {
		return arguments, closer, diags
	}

	// The graph command accepts an optional path argument
	if len(cmdFlags.Args()) > 0 {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Unexpected argument",
			"Too many command line arguments. Did you mean to use -chdir?",
		))
	}

	return arguments, closer, diags
}
