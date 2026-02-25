// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"fmt"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/flags"
	"github.com/opentofu/opentofu/internal/command/views"
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

func (c *GraphCommand) Run(rawArgs []string) int {
	ctx := c.CommandContext()

	common, rawArgs := arguments.ParseView(rawArgs)
	c.View.Configure(common)
	// Because the legacy UI was using println to show diagnostics and the new view is using, by default, print,
	// in order to keep functional parity, we setup the view to add a new line after each diagnostic.
	c.View.DiagsWithNewline()

	// Propagate -no-color for legacy use of Ui. The remote backend and
	// cloud package use this; it should be removed when/if they are
	// migrated to views.
	c.Meta.color = !common.NoColor
	c.Meta.Color = c.Meta.color

	// Parse and validate flags
	args, closer, diags := arguments.ParseGraph(rawArgs)
	defer closer()

	// Instantiate the view, even if there are flag errors, so that we render
	// diagnostics according to the desired view
	view := views.NewGraph(c.View)
	// ... and initialise the Meta.Ui to wrap Meta.View into a new implementation
	// that is able to print by using View abstraction and use the Meta.Ui
	// to ask for the user input.
	c.Meta.configureUiFromView(args.ViewOptions)
	if diags.HasErrors() {
		view.Diagnostics(diags)
		return cli.RunResultHelp
	}
	c.GatherVariables(args.Vars)

	// This gets the current directory as full path.
	configPath := c.WorkingDir.NormalizePath(c.WorkingDir.RootModuleDir())

	var err error
	// Check for user-supplied plugin path
	if c.pluginPath, err = c.loadPluginPath(); err != nil {
		view.Diagnostics(diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Error loading plugin path",
			fmt.Sprintf("Encountered an error while loading the plugin path: %s", err),
		)))
		return 1
	}

	// Load the encryption configuration
	enc, encDiags := c.EncryptionFromPath(ctx, configPath)
	diags = diags.Append(encDiags)
	if encDiags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	// Try to load plan if path is specified
	var planFile *planfile.WrappedPlanFile
	if args.PlanPath != "" {
		planFile, err = c.PlanFile(args.PlanPath, enc.Plan())
		if err != nil {
			view.Diagnostics(diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Failed loading the plan file",
				fmt.Sprintf("Encountered an error while opening the plan file %q: %s", args.PlanPath, err),
			)))
			return 1
		}
	}

	// Load the backend
	var b backend.Enhanced
	if lp, ok := planFile.Local(); ok {
		plan, planErr := lp.ReadPlan()
		if planErr != nil {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Failed to read plan from plan file",
				fmt.Sprintf("Cannot read the plan from the given plan file: %s", planErr),
			))
			view.Diagnostics(diags)
			return 1
		}
		if plan.Backend.Config == nil {
			// Should never happen; always indicates a bug in the creation of the plan file
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Failed to read plan from plan file",
				"The given plan file does not have a valid backend configuration. This is a bug in the OpenTofu command that generated this plan file.",
			))
			view.Diagnostics(diags)
			return 1
		}
		var backendDiags tfdiags.Diagnostics
		b, backendDiags = c.BackendForLocalPlan(ctx, plan.Backend, enc.State())
		diags = diags.Append(backendDiags)
		if backendDiags.HasErrors() {
			view.Diagnostics(diags)
			return 1
		}
	} else {
		backendConfig, backendDiags := c.loadBackendConfig(ctx, configPath)
		diags = diags.Append(backendDiags)
		if diags.HasErrors() {
			view.Diagnostics(diags)
			return 1
		}

		b, backendDiags = c.Backend(ctx, &BackendOpts{
			Config: backendConfig,
		}, enc.State())
		diags = diags.Append(backendDiags)
		if backendDiags.HasErrors() {
			view.Diagnostics(diags)
			return 1
		}
	}

	// We require a local backend
	local, ok := b.(backend.Local)
	if !ok {
		view.Diagnostics(diags) // in case of any warnings in here
		view.ErrorUnsupportedLocalOp()
		return 1
	}

	// This is a read-only command
	c.ignoreRemoteVersionConflict(b)

	// Build the operation
	opReq := c.Operation(ctx, b, arguments.ViewOptions{ViewType: arguments.ViewHuman}, enc)
	opReq.ConfigDir = configPath
	opReq.ConfigLoader, err = c.initConfigLoader()
	opReq.PlanFile = planFile
	opReq.AllowUnsetVariables = true

	// Inject information required for static evaluation
	var callDiags tfdiags.Diagnostics
	opReq.RootCall, callDiags = c.rootModuleCall(ctx, opReq.ConfigDir)
	diags = diags.Append(callDiags)
	if callDiags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	if err != nil {
		diags = diags.Append(err)
		view.Diagnostics(diags)
		return 1
	}

	// Get the context
	lr, _, ctxDiags := local.LocalRun(ctx, opReq)
	diags = diags.Append(ctxDiags)
	if ctxDiags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	if args.GraphType == "" {
		switch {
		case lr.Plan != nil:
			args.GraphType = "apply"
		default:
			args.GraphType = "plan"
		}
	}

	var g *tofu.Graph
	var graphDiags tfdiags.Diagnostics
	switch args.GraphType {
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
			fmt.Sprintf("The graph type %q is no longer available. Use -type=plan instead to get a similar result.", args.GraphType),
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
		view.Diagnostics(diags)
		return 1
	}

	graphStr, err := tofu.GraphDot(g, &dag.DotOpts{
		DrawCycles: args.DrawCycles,
		MaxDepth:   args.ModuleDepth,
		Verbose:    args.Verbose,
	})
	if err != nil {
		view.Diagnostics(diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed generating the graph representation",
			fmt.Sprintf("Error encountered while generating the graph representation: %s.", err),
		)))
		return 1
	}

	if diags.HasErrors() {
		// For this command we only show diagnostics if there are errors,
		// because printing out naked warnings could upset a naive program
		// consuming our dot output.
		view.Diagnostics(diags)
		return 1
	}

	view.Output(graphStr)

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

// TODO meta-refactor: move this to arguments once all commands are using the same shim logic
func (c *GraphCommand) GatherVariables(args *arguments.Vars) {
	// FIXME the arguments package currently trivially gathers variable related
	// arguments in a heterogeneous slice, in order to minimize the number of
	// code paths gathering variables during the transition to this structure.
	// Once all commands that gather variables have been converted to this
	// structure, we could move the variable gathering code to the arguments
	// package directly, removing this shim layer.

	varArgs := args.All()
	items := make([]flags.RawFlag, len(varArgs))
	for i := range varArgs {
		items[i].Name = varArgs[i].Name
		items[i].Value = varArgs[i].Value
	}
	c.Meta.variableArgs = flags.RawFlags{Items: &items}
}
