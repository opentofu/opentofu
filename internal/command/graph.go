// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/dag"
	applyEngine "github.com/opentofu/opentofu/internal/engine/apply"
	"github.com/opentofu/opentofu/internal/engine/plugins"
	"github.com/opentofu/opentofu/internal/experiments"
	"github.com/opentofu/opentofu/internal/lang/eval"
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

	ctx := c.CommandContext()

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
	enc, encDiags := c.EncryptionFromPath(ctx, configPath)
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
		b, backendDiags = c.BackendForLocalPlan(ctx, plan.Backend, enc.State())
		diags = diags.Append(backendDiags)
		if backendDiags.HasErrors() {
			c.showDiagnostics(diags)
			return 1
		}
	} else {
		backendConfig, backendDiags := c.loadBackendConfig(ctx, configPath)
		diags = diags.Append(backendDiags)
		if diags.HasErrors() {
			c.showDiagnostics(diags)
			return 1
		}

		b, backendDiags = c.Backend(ctx, &BackendOpts{
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
		c.showDiagnostics(diags)
		return 1
	}

	if err != nil {
		diags = diags.Append(err)
		c.showDiagnostics(diags)
		return 1
	}

	// Get the context
	lr, _, ctxDiags := local.LocalRun(ctx, opReq)
	diags = diags.Append(ctxDiags)
	if ctxDiags.HasErrors() {
		c.showDiagnostics(diags)
		return 1
	}

	if experiments.ExperimentalRuntimeEnabled() {
		// As temporary scaffolding while we work on a new language runtime,
		// we have an opt-in alternative implementation of this command which
		// presents information from that new runtime.
		// This conditional branch should be completely removed once the
		// experiment is concluded. For more information refer to the file
		// containing [experiments.ExperimentalRuntimeEnabled].
		return c.runForExperimentalRuntime(ctx, lr)
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

func (c *GraphCommand) runForExperimentalRuntime(ctx context.Context, lr *backend.LocalRun) int {
	// This experimental implementation currently supports only the
	// "-plan=FILENAME" option, which causes it to render an execution graph
	// instead of a resource instance graph. This is primarily here to help
	// developers working on the new language runtime, by visualizing the
	// new graph structures it works with instead of showing those used by the
	// traditional language runtime.
	//
	// This variant of the "tofu graph" command still generates
	// graphviz-compatible syntax, but does not attempt to closely mimic
	// details such as exact node shapes and label names, and is instead
	// closer to how the new language runtime "thinks about" the graphs.

	if plan := lr.Plan; plan == nil {
		// Visualization of the "resource graph" that we'd generate as part
		// of the planning process, without actually producing a plan.

		// FIXME: Factor all of this new-style config preparation code out into
		// a common place that we can share across everything interacting with
		// the new language runtime. But for now it's just inlined here as a
		// temporary stopgap while we're trying to keep the new-runtime
		// codepaths segregated from traditional codepaths and avoid doing any
		// risky refactoring before this new code is more settled.
		provisionerFactories := c.provisionerFactories()
		providerFactories, err := c.providerFactories()
		if err != nil {
			c.Ui.Error(fmt.Sprintf("Failed to prepare provider factories: %s", err))
			return 1
		}
		plugins := plugins.NewRuntimePlugins(providerFactories, provisionerFactories)
		evalCtx := &eval.EvalContext{
			RootModuleDir:      c.WorkingDir.RootModuleDir(),
			OriginalWorkingDir: c.WorkingDir.OriginalWorkingDir(),
			// FIXME: Populate "Modules" properly so that we can work with
			// configurations that have nested modules. Currently this
			// implementation only works with root modules that contain no
			// module call blocks.
			Modules: eval.ModulesForTesting(map[addrs.ModuleSourceLocal]*configs.Module{
				addrs.ModuleSourceLocal("./."): lr.Config.Module,
			}),
			Providers:    plugins,
			Provisioners: plugins,
		}
		defer func() {
			// We'll call close with a cancel-free context because we do still
			// want to shut the providers down even if we're dealing with
			// graceful shutdown after cancellation.
			err := plugins.Close(context.WithoutCancel(ctx))
			// If a provider fails to close there isn't really much we can do
			// about that... this shouldn't really be possible unless the
			// plugin process already exited for some other reason anyway.
			if err != nil {
				log.Printf("[ERROR] plugin shutdown failed: %s", err)
			}
		}()

		// We have some redundancy here for now because we actually already
		// loaded the configuration the old way but we now need to reload it in
		// the way that the new evaluator expects.
		configDir := c.WorkingDir.RootModuleDir()
		if !filepath.IsAbs(configDir) {
			configDir = "." + string(filepath.Separator) + configDir
		}
		rootModuleSource, err := addrs.ParseModuleSource(configDir)
		if err != nil {
			var diags tfdiags.Diagnostics
			diags = diags.Append(fmt.Errorf("invalid root module source address: %w", err))
			c.showDiagnostics(diags)
			return 1
		}
		configCall := &eval.ConfigCall{
			RootModuleSource: rootModuleSource,
			// TODO: InputValues
			AllowImpureFunctions: false,
			EvalContext:          evalCtx,
		}
		configInst, diags := eval.NewConfigInstance(ctx, configCall)
		if diags.HasErrors() {
			c.showDiagnostics(diags)
			return 1
		}

		diags = configInst.WriteGraphvizGraphForDebugging(ctx, c.Streams.Stdout.File)
		if diags.HasErrors() {
			c.showDiagnostics(diags)
			return 1
		}
		return 0
	} else {
		// Visualization of the "execution graph" that is saved as part of
		// the plan, which we'd use during an apply phase.

		if len(plan.ExecutionGraph) == 0 {
			// This field is only populated for plans created using the new
			// runtime's planning engine, so this not being set suggests
			// that we've been given a traditional-style plan file.
			c.Ui.Error("The given plan file does not contain an execution graph. Was it generated by the traditional OpenTofu runtime, instead of the experimental new one?")
			return 1
		}

		err := applyEngine.WriteExecutionGraphForGraphviz(plan.ExecutionGraph, c.Streams.Stdout.File)
		if err != nil {
			c.Ui.Error(fmt.Sprintf("Failed to render execution graph: %s", err))
			return 1
		}

		return 0
	}
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
