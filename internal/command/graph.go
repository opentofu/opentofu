// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"fmt"
	"strings"

	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/dag"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/plans/planfile"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tofu"
)

// GraphCommand is a Command implementation that takes a OpenTofu
// configuration and outputs the dependency tree in graphical form.
type GraphCommand struct {
	Meta
}

func (c *GraphCommand) Run(args []string) int {
	var diags tfdiags.Diagnostics

	var drawCycles bool
	var graphTypeStr string
	var moduleDepth int
	var verbose bool
	var planPath string

	args = c.Meta.process(args)
	cmdFlags := c.Meta.defaultFlagSet("graph")
	c.Meta.varFlagSet(cmdFlags)
	cmdFlags.BoolVar(&drawCycles, "draw-cycles", false, "draw-cycles")
	cmdFlags.StringVar(&graphTypeStr, "type", "", "type")
	cmdFlags.IntVar(&moduleDepth, "module-depth", -1, "module-depth")
	cmdFlags.BoolVar(&verbose, "verbose", false, "verbose")
	cmdFlags.StringVar(&planPath, "plan", "", "plan")
	cmdFlags.Usage = func() { c.Ui.Error(c.Help()) }
	if err := cmdFlags.Parse(args); err != nil {
		c.Ui.Error(fmt.Sprintf("Error parsing command-line flags: %s\n", err.Error()))
		return 1
	}

	configPath, err := modulePath(cmdFlags.Args())
	if err != nil {
		c.Ui.Error(err.Error())
		return 1
	}

	// Check for user-supplied plugin path
	if c.pluginPath, err = c.loadPluginPath(); err != nil {
		c.Ui.Error(fmt.Sprintf("Error loading plugin path: %s", err))
		return 1
	}

	// Load the encryption configuration
	enc, encDiags := c.EncryptionFromPath(configPath)
	diags = diags.Append(encDiags)
	if encDiags.HasErrors() {
		c.showDiagnostics(diags)
		return 1
	}

	// Try to load plan if path is specified
	var planFile *planfile.WrappedPlanFile
	if planPath != "" {
		planFile, err = c.PlanFile(planPath, enc.Plan())
		if err != nil {
			c.Ui.Error(err.Error())
			return 1
		}
	}

	// Load the backend
	var b backend.Enhanced
	//nolint: nestif // This is inspired by apply:PrepareBackend
	if lp, ok := planFile.Local(); ok {
		plan, planErr := lp.ReadPlan()
		if planErr != nil {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Failed to read plan from plan file",
				fmt.Sprintf("Cannot read the plan from the given plan file: %s.", planErr),
			))
			c.showDiagnostics(diags)
			return 1
		}
		if plan.Backend.Config == nil {
			// Should never happen; always indicates a bug in the creation of the plan file
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Failed to read plan from plan file",
				"The given plan file does not have a valid backend configuration. This is a bug in the OpenTofu command that generated this plan file.",
			))
			c.showDiagnostics(diags)
			return 1
		}
		var backendDiags tfdiags.Diagnostics
		b, backendDiags = c.BackendForLocalPlan(plan.Backend, enc.State())
		diags = diags.Append(backendDiags)
		if backendDiags.HasErrors() {
			c.showDiagnostics(diags)
			return 1
		}
	} else {
		backendConfig, backendDiags := c.loadBackendConfig(configPath)
		diags = diags.Append(backendDiags)
		if diags.HasErrors() {
			c.showDiagnostics(diags)
			return 1
		}

		b, backendDiags = c.Backend(&BackendOpts{
			Config: backendConfig,
		}, enc.State())
		diags = diags.Append(backendDiags)
		if backendDiags.HasErrors() {
			c.showDiagnostics(diags)
			return 1
		}
	}

	// We require a local backend
	local, ok := b.(backend.Local)
	if !ok {
		c.showDiagnostics(diags) // in case of any warnings in here
		c.Ui.Error(ErrUnsupportedLocalOp)
		return 1
	}

	// This is a read-only command
	c.ignoreRemoteVersionConflict(b)

	// Build the operation
	opReq := c.Operation(b, arguments.ViewHuman, enc)
	opReq.ConfigDir = configPath
	opReq.ConfigLoader, err = c.initConfigLoader()
	opReq.PlanFile = planFile
	opReq.AllowUnsetVariables = true

	// Inject information required for static evaluation
	var callDiags tfdiags.Diagnostics
	opReq.RootCall, callDiags = c.rootModuleCall(opReq.ConfigDir)
	diags = diags.Append(callDiags)
	if callDiags.HasErrors() {
		c.showDiagnostics(diags)
		return 1
	}

	if err != nil {
		diags = diags.Append(err)
		c.showDiagnostics(diags)
		return 1
	}

	// Get the context
	lr, _, ctxDiags := local.LocalRun(opReq)
	diags = diags.Append(ctxDiags)
	if ctxDiags.HasErrors() {
		c.showDiagnostics(diags)
		return 1
	}

	if graphTypeStr == "" {
		switch {
		case lr.Plan != nil:
			graphTypeStr = "apply"
		default:
			graphTypeStr = "plan"
		}
	}

	var g *tofu.Graph
	var graphDiags tfdiags.Diagnostics
	switch graphTypeStr {
	case "plan":
		g, graphDiags = lr.Core.PlanGraphForUI(lr.Config, lr.InputState, plans.NormalMode)
	case "plan-refresh-only":
		g, graphDiags = lr.Core.PlanGraphForUI(lr.Config, lr.InputState, plans.RefreshOnlyMode)
	case "plan-destroy":
		g, graphDiags = lr.Core.PlanGraphForUI(lr.Config, lr.InputState, plans.DestroyMode)
	case "apply":
		plan := lr.Plan

		// Historically "tofu graph" would allow the nonsensical request to
		// render an apply graph without a plan, so we continue to support that
		// here, though perhaps one day this should be an error.
		if lr.Plan == nil {
			plan = &plans.Plan{
				Changes:      plans.NewChanges(),
				UIMode:       plans.NormalMode,
				PriorState:   lr.InputState,
				PrevRunState: lr.InputState,
			}
		}

		g, graphDiags = lr.Core.ApplyGraphForUI(plan, lr.Config)
	case "eval", "validate":
		// Terraform v0.12 through v1.0 supported both of these, but the
		// graph variants for "eval" and "validate" are purely implementation
		// details and don't reveal anything (user-model-wise) that you can't
		// see in the plan graph.
		graphDiags = graphDiags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Graph type no longer available",
			fmt.Sprintf("The graph type %q is no longer available. Use -type=plan instead to get a similar result.", graphTypeStr),
		))
	default:
		graphDiags = graphDiags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Unsupported graph type",
			`The -type=... argument must be either "plan", "plan-refresh-only", "plan-destroy", or "apply".`,
		))
	}
	diags = diags.Append(graphDiags)
	if graphDiags.HasErrors() {
		c.showDiagnostics(diags)
		return 1
	}

	graphStr, err := tofu.GraphDot(g, &dag.DotOpts{
		DrawCycles: drawCycles,
		MaxDepth:   moduleDepth,
		Verbose:    verbose,
	})
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Error converting graph: %s", err))
		return 1
	}

	if diags.HasErrors() {
		// For this command we only show diagnostics if there are errors,
		// because printing out naked warnings could upset a naive program
		// consuming our dot output.
		c.showDiagnostics(diags)
		return 1
	}

	c.Ui.Output(graphStr)

	return 0
}

func (c *GraphCommand) Help() string {
	helpText := `
Usage: tofu [global options] graph [options]

  Produces a representation of the dependency graph between different
  objects in the current configuration and state.

  The graph is presented in the DOT language. The typical program that can
  read this format is GraphViz, but many web services are also available
  to read this format.

Options:

  -plan=tfplan     Render graph using the specified plan file instead of the
                   configuration in the current directory.

  -draw-cycles     Highlight any cycles in the graph with colored edges.
                   This helps when diagnosing cycle errors.

  -type=plan       Type of graph to output. Can be: plan, plan-refresh-only,
                   plan-destroy, or apply. By default OpenTofu chooses
				   "plan", or "apply" if you also set the -plan=... option.

  -module-depth=n  (deprecated) In prior versions of OpenTofu, specified the
				   depth of modules to show in the output.

  -var 'foo=bar'     Set a value for one of the input variables in the root
                     module of the configuration. Use this option more than
                     once to set more than one variable.

  -var-file=filename Load variable values from the given file, in addition
                     to the default files terraform.tfvars and *.auto.tfvars.
                     Use this option more than once to include more than one
                     variables file.
`
	return strings.TrimSpace(helpText)
}

func (c *GraphCommand) Synopsis() string {
	return "Generate a Graphviz graph of the steps in an operation"
}
