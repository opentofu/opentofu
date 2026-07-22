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

// BindGraph registers CLI arguments, returning a Graph value and it's corresponding hooks.
func BindGraph(flags Flags) (*Graph, Hooks) {
	var graph Graph
	var hooks Hooks

	// we only parse but do not register the views flags since this command does not need it
	hooks = append(hooks, graph.ViewOptions.ParseHook())

	graph.Vars = &Vars{}
	graph.Vars.bind(flags)

	flags.BoolVar(&graph.DrawCycles, "draw-cycles", false, "Highlight any cycles in the graph with colored edges. This helps when diagnosing cycle errors.")
	flags.StringVar(&graph.GraphType, "type", "", `Type of graph to output. Can be: plan, plan-refresh-only, plan-destroy, or apply. By default OpenTofu chooses "plan", or "apply" if you also set the -plan=... option.`)
	flags.IntVar(&graph.ModuleDepth, "module-depth", -1, "(deprecated) In prior versions of OpenTofu, specified the depth of modules to show in the output.")
	flags.BoolVar(&graph.Verbose, "verbose", false, "verbose").SetHidden(true)
	flags.StringVar(&graph.PlanPath, "plan", "", "Render graph using the specified plan file instead of the configuration in the current directory.")

	return &graph, hooks
}

// ParseGraph processes CLI arguments, returning a Graph value, a closer function, and errors.
// If errors are encountered, a Graph value is still returned representing
// the best effort interpretation of the arguments.
func ParseGraph(args []string) (*Graph, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	flags := Flags{}
	arguments, hooks := BindGraph(flags)

	cmdFlags := defaultFlagSet("graph", flags)

	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line flags",
			err.Error(),
		))
	}

	if len(cmdFlags.Args()) > 0 {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Unexpected argument",
			"Too many command line arguments. Did you mean to use -chdir?",
		))
	}

	return arguments, func() { hooks.Post() }, diags.Append(hooks.Pre())
}
