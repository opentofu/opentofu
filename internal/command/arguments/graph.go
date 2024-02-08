// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import "github.com/opentofu/opentofu/internal/tfdiags"

// Graph represents the command-line arguments for the graph command.
type Graph struct {
	// State, Operation, and Vars are the common extended flags
	State     *State
	Operation *Operation
	Vars      *Vars

	DrawCycles bool

	GraphTypeStr string

	ModuleDepth int

	// PlanPath contains an optional path to a stored plan file
	PlanPath string

	Verbose bool

	// ViewType specifies which output format to use
	ViewType ViewType
}

// ParseGraph processes CLI arguments, returning a Graph value and errors.
// If errors are encountered, a Graph value is still returned representing
// the best effort interpretation of the arguments.
func ParseGraph(args []string) (*Graph, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	graph := &Graph{
		State:     &State{},
		Operation: &Operation{},
		Vars:      &Vars{},
		ViewType:  ViewHuman,
	}

	cmdFlags := extendedFlagSet("graph", graph.State, graph.Operation, graph.Vars)
	cmdFlags.BoolVar(&graph.DrawCycles, "draw-cycles", false, "draw-cycles")
	cmdFlags.StringVar(&graph.GraphTypeStr, "type", "", "type")
	cmdFlags.IntVar(&graph.ModuleDepth, "module-depth", -1, "module-depth")
	cmdFlags.StringVar(&graph.PlanPath, "plan", "", "plan")
	cmdFlags.BoolVar(&graph.Verbose, "verbose", false, "verbose")

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
			"Expected no positional argument.",
		))
	}

	diags = diags.Append(graph.Operation.Parse())

	return graph, diags
}
