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

	// View represents the global view options
	View *View

	// Vars holds and provides information for the flags related to variables that a user can give into the process
	Vars *Vars
}

// BindGraph registers CLI arguments, returning a Graph value and it's corresponding hooks.
func BindGraph(cli *CommandLine) *Graph {
	graph := Graph{
		View: BindView(cli, viewFlagNone),
		Vars: BindVars(cli),
	}

	cli.BoolVar(&graph.DrawCycles, "draw-cycles", false, "Highlight any cycles in the graph with colored edges. This helps when diagnosing cycle errors.")
	cli.StringVar(&graph.GraphType, "type", "", `Type of graph to output. Can be: plan, plan-refresh-only, plan-destroy, or apply. By default OpenTofu chooses "plan", or "apply" if you also set the -plan=... option.`).SetDisplay("=plan")
	cli.IntVar(&graph.ModuleDepth, "module-depth", -1, "(deprecated) In prior versions of OpenTofu, specified the depth of modules to show in the output.")
	cli.BoolVar(&graph.Verbose, "verbose", false, "verbose").SetHidden(true)
	cli.StringVar(&graph.PlanPath, "plan", "", "Render graph using the specified plan file instead of the configuration in the current directory.").SetDisplay("=tfplan")

	return &graph
}

// ParseGraph processes CLI arguments, returning a Graph value, a closer function, and errors.
// If errors are encountered, a Graph value is still returned representing
// the best effort interpretation of the arguments.
func ParseGraph(args []string) (*Graph, func(), tfdiags.Diagnostics) {
	cli := new(CommandLine)
	arguments := BindGraph(cli)
	closer, diags := cli.parseWithHooks("graph", args)
	return arguments, closer, diags
}
