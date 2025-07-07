// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	otelAttr "go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/cloud"
	"github.com/opentofu/opentofu/internal/cloud/cloudplan"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/plans/planfile"
	"github.com/opentofu/opentofu/internal/states/statefile"
	"github.com/opentofu/opentofu/internal/states/statemgr"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tofu"
	"github.com/opentofu/opentofu/internal/tracing"
)

// Many of the methods we get data from can emit special error types if they're
// pretty sure about the file type but still can't use it. But they can't all do
// that! So, we have to do a couple ourselves if we want to preserve that data.
type errUnusableDataMisc struct {
	inner error
	kind  string
}

func errUnusable(err error, kind string) *errUnusableDataMisc {
	return &errUnusableDataMisc{inner: err, kind: kind}
}
func (e *errUnusableDataMisc) Error() string {
	return e.inner.Error()
}
func (e *errUnusableDataMisc) Unwrap() error {
	return e.inner
}

// ShowCommand is a Command implementation that reads and outputs the
// contents of a OpenTofu plan or state file.
// write about config here
type ShowCommand struct {
	Meta
	viewType arguments.ViewType
}

func (c *ShowCommand) Run(rawArgs []string) int {
	ctx := c.CommandContext()

	// Parse and apply global view arguments
	common, rawArgs := arguments.ParseView(rawArgs)
	c.View.Configure(common)

	// Parse and validate flags
	args, diags := arguments.ParseShow(rawArgs)
	if diags.HasErrors() {
		c.View.Diagnostics(diags)
		c.View.HelpPrompt("show")
		return 1
	}
	c.viewType = args.ViewType
	c.View.SetShowSensitive(args.ShowSensitive)

	//nolint:ineffassign - As this is a high-level call, we want to ensure that we are correctly using the right ctx later on when
	ctx, span := tracing.Tracer().Start(ctx, "Show",
		trace.WithAttributes(
			otelAttr.String("opentofu.show.view", args.ViewType.String()),
			otelAttr.String("opentofu.show.target", args.TargetType.String()),
			otelAttr.String("opentofu.show.target_arg", args.TargetArg),
			otelAttr.Bool("opentofu.show.show_sensitive", args.ShowSensitive),
		),
	)
	defer span.End()

	// Set up view
	view := views.NewShow(args.ViewType, c.View)

	// Check for user-supplied plugin path
	var err error
	if c.pluginPath, err = c.loadPluginPath(); err != nil {
		diags = diags.Append(fmt.Errorf("error loading plugin path: %w", err))
		view.Diagnostics(diags)
		return 1
	}

	// Inject variables from args into meta for static evaluation
	c.GatherVariables(args.Vars)

	// Load the encryption configuration
	enc, encDiags := c.Encryption(ctx)
	diags = diags.Append(encDiags)
	if encDiags.HasErrors() {
		c.showDiagnostics(diags)
		return 1
	}

	renderResult, showDiags := c.show(ctx, args.TargetType, args.TargetArg, enc)
	diags = diags.Append(showDiags)
	if showDiags.HasErrors() {
		// "tofu show" intentionally ignores warnings unless there is at
		// least one error, because view.Diagnostics produces human output
		// even in the JSON view and so would cause the JSON output to
		// be invalid if only warnings were returned.
		view.Diagnostics(diags)
		tracing.SetSpanError(span, showDiags)
		return 1
	}
	return renderResult(view)
}

func (c *ShowCommand) Help() string {
	helpText := `
Usage: tofu [global options] show [target-selection-option] [other-options]

  Reads and outputs a OpenTofu state or plan file in a human-readable
  form. If no path is specified, the current state will be shown.

Target selection options:

  Use one of the following options to specify what to show.

    -state          The latest state snapshot, if any.
    -plan=FILENAME  The plan from a saved plan file.
    -config         Show the current configuration (requires -json).

  If no target selection options are provided, -state is the default.

Other options:

  -no-color           Disable terminal escape sequences.

  -json               Show the information in a machine-readable form.

  -show-sensitive     If specified, sensitive values will be displayed.

  -var 'foo=bar'      Set a value for one of the input variables in the root
                      module of the configuration. Use this option more than
                      once to set more than one variable.

  -var-file=filename  Load variable values from the given file, in addition
                      to the default files terraform.tfvars and *.auto.tfvars.
                      Use this option more than once to include more than one
                      variables file.

`
	return strings.TrimSpace(helpText)
}

func (c *ShowCommand) Synopsis() string {
	return "Show the current state or a saved plan"
}

func (c *ShowCommand) GatherVariables(args *arguments.Vars) {
	// FIXME the arguments package currently trivially gathers variable related
	// arguments in a heterogeneous slice, in order to minimize the number of
	// code paths gathering variables during the transition to this structure.
	// Once all commands that gather variables have been converted to this
	// structure, we could move the variable gathering code to the arguments
	// package directly, removing this shim layer.

	varArgs := args.All()
	items := make([]rawFlag, len(varArgs))
	for i := range varArgs {
		items[i].Name = varArgs[i].Name
		items[i].Value = varArgs[i].Value
	}
	c.Meta.variableArgs = rawFlags{items: &items}
}

type showRenderFunc func(view views.Show) int

func (c *ShowCommand) show(ctx context.Context, targetType arguments.ShowTargetType, targetArg string, enc encryption.Encryption) (showRenderFunc, tfdiags.Diagnostics) {
	switch targetType {
	case arguments.ShowState:
		return c.showFromLatestStateSnapshot(ctx, enc)
	case arguments.ShowPlan:
		return c.showFromSavedPlanFile(ctx, targetArg, enc)
	case arguments.ShowConfig:
		return c.showConfiguration(ctx)
	case arguments.ShowUnknownType:
		// This is a legacy case where we just have a filename and need to
		// try treating it as either a saved plan file or a local state
		// snapshot file.
		return c.legacyShowFromPath(ctx, targetArg, enc)
	default:
		// Should not get here because the above cases should cover all
		// possible values of [arguments.ShowTargetType].
		panic(fmt.Sprintf("unsupported show target type %s", targetType))
	}
}

func (c *ShowCommand) showFromLatestStateSnapshot(ctx context.Context, enc encryption.Encryption) (showRenderFunc, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	ctx, span := tracing.Tracer().Start(ctx, "Show State")
	defer span.End()

	// Load the backend
	b, backendDiags := c.Backend(ctx, nil, enc.State())
	diags = diags.Append(backendDiags)
	if backendDiags.HasErrors() {
		return nil, diags
	}
	c.ignoreRemoteVersionConflict(b)

	// Load the workspace
	workspace, err := c.Workspace(ctx)
	if err != nil {
		diags = diags.Append(fmt.Errorf("error selecting workspace: %w", err))
		return nil, diags
	}

	// Get the latest state snapshot from the backend for the current workspace
	stateFile, stateErr := getStateFromBackend(ctx, b, workspace)
	if stateErr != nil {
		diags = diags.Append(stateErr)
		return nil, diags
	}

	schemas, schemaDiags := c.maybeGetSchemas(ctx, stateFile, nil)
	diags = diags.Append(schemaDiags)
	if schemaDiags.HasErrors() {
		return nil, diags
	}
	return func(view views.Show) int {
		return view.DisplayState(ctx, stateFile, schemas)
	}, diags
}

func (c *ShowCommand) showFromSavedPlanFile(ctx context.Context, filename string, enc encryption.Encryption) (showRenderFunc, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	ctx, span := tracing.Tracer().Start(ctx, "Show Plan")
	defer span.End()

	rootCall, callDiags := c.rootModuleCall(ctx, ".")
	diags = diags.Append(callDiags)
	if diags.HasErrors() {
		return nil, diags
	}

	plan, jsonPlan, stateFile, config, err := c.getPlanFromPath(ctx, filename, enc, rootCall)
	if err != nil {
		diags = diags.Append(err)
		return nil, diags
	}

	schemas, schemaDiags := c.maybeGetSchemas(ctx, stateFile, config)
	diags = diags.Append(schemaDiags)
	if schemaDiags.HasErrors() {
		return nil, diags
	}

	return func(view views.Show) int {
		return view.DisplayPlan(ctx, plan, jsonPlan, config, stateFile, schemas)
	}, diags
}

func (c *ShowCommand) legacyShowFromPath(ctx context.Context, path string, enc encryption.Encryption) (showRenderFunc, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	var planErr, stateErr error
	var plan *plans.Plan
	var jsonPlan *cloudplan.RemotePlanJSON
	var stateFile *statefile.File
	var config *configs.Config

	ctx, span := tracing.Tracer().Start(ctx, "Show")
	defer span.End()

	rootCall, callDiags := c.rootModuleCall(ctx, ".")
	diags = diags.Append(callDiags)
	if diags.HasErrors() {
		return nil, diags
	}

	// Path might be a local plan file, a bookmark to a saved cloud plan, or a
	// state file. First, try to get a plan and associated data from a local
	// plan file. If that fails, try to get a json plan from the path argument.
	// If that fails, try to get the statefile from the path argument.
	plan, jsonPlan, stateFile, config, planErr = c.getPlanFromPath(ctx, path, enc, rootCall)
	if planErr != nil {
		stateFile, stateErr = getStateFromPath(path, enc)
		if stateErr != nil {
			// To avoid spamming the user with irrelevant errors, first check to
			// see if one of our errors happens to know for a fact what file
			// type we were dealing with. If so, then we can ignore the other
			// ones (which are likely to be something unhelpful like "not a
			// valid zip file"). If not, we can fall back to dumping whatever
			// we've got.
			var unLocal *planfile.ErrUnusableLocalPlan
			var unState *statefile.ErrUnusableState
			var unMisc *errUnusableDataMisc
			if errors.As(planErr, &unLocal) {
				diags = diags.Append(
					tfdiags.Sourceless(
						tfdiags.Error,
						"Couldn't show local plan",
						fmt.Sprintf("Plan read error: %s", unLocal),
					),
				)
			} else if errors.As(planErr, &unMisc) {
				diags = diags.Append(
					tfdiags.Sourceless(
						tfdiags.Error,
						fmt.Sprintf("Couldn't show %s", unMisc.kind),
						fmt.Sprintf("Plan read error: %s", unMisc),
					),
				)
			} else if errors.As(stateErr, &unState) {
				diags = diags.Append(
					tfdiags.Sourceless(
						tfdiags.Error,
						"Couldn't show state file",
						fmt.Sprintf("Plan read error: %s", unState),
					),
				)
			} else if errors.As(stateErr, &unMisc) {
				diags = diags.Append(
					tfdiags.Sourceless(
						tfdiags.Error,
						fmt.Sprintf("Couldn't show %s", unMisc.kind),
						fmt.Sprintf("Plan read error: %s", unMisc),
					),
				)
			} else {
				// Ok, give up and show the really big error
				diags = diags.Append(
					tfdiags.Sourceless(
						tfdiags.Error,
						"Failed to read the given file as a state or plan file",
						fmt.Sprintf("State read error: %s\n\nPlan read error: %s", stateErr, planErr),
					),
				)
			}
			tracing.SetSpanError(span, diags)
			return nil, diags
		}
	}

	schemas, schemaDiags := c.maybeGetSchemas(ctx, stateFile, config)
	diags = diags.Append(schemaDiags)
	if schemaDiags.HasErrors() {
		tracing.SetSpanError(span, diags)
		return nil, diags
	}

	// If we successfully loaded some things then the show mode we
	// choose depends on what we loaded.
	switch {
	case plan != nil || jsonPlan != nil:
		return func(view views.Show) int {
			return view.DisplayPlan(ctx, plan, jsonPlan, config, stateFile, schemas)
		}, diags
	default:
		// We treat all other cases as a state, and DisplayState
		// tolerates stateFile being nil.
		return func(view views.Show) int {
			return view.DisplayState(ctx, stateFile, schemas)
		}, diags
	}
}

// getPlanFromPath returns a plan, json plan, statefile, and config if the
// user-supplied path points to either a local or cloud plan file. Note that
// some of the return values will be nil no matter what; local plan files do not
// yield a json plan, and cloud plans do not yield real plan/state/config
// structs. An error generally suggests that the given path is either a
// directory or a statefile.
func (c *ShowCommand) getPlanFromPath(ctx context.Context, path string, enc encryption.Encryption, rootCall configs.StaticModuleCall) (*plans.Plan, *cloudplan.RemotePlanJSON, *statefile.File, *configs.Config, error) {
	var err error
	var plan *plans.Plan
	var jsonPlan *cloudplan.RemotePlanJSON
	var stateFile *statefile.File
	var config *configs.Config

	pf, err := planfile.OpenWrapped(path, enc.Plan())
	if err != nil {
		return nil, nil, nil, nil, err
	}

	if lp, ok := pf.Local(); ok {
		plan, stateFile, config, err = getDataFromPlanfileReader(ctx, lp, rootCall)
	} else if cp, ok := pf.Cloud(); ok {
		redacted := c.viewType != arguments.ViewJSON
		jsonPlan, err = c.getDataFromCloudPlan(ctx, cp, redacted, enc)
	}

	return plan, jsonPlan, stateFile, config, err
}

func (c *ShowCommand) getDataFromCloudPlan(ctx context.Context, plan *cloudplan.SavedPlanBookmark, redacted bool, enc encryption.Encryption) (*cloudplan.RemotePlanJSON, error) {
	// Set up the backend
	b, backendDiags := c.Backend(ctx, nil, enc.State())
	if backendDiags.HasErrors() {
		return nil, errUnusable(backendDiags.Err(), "cloud plan")
	}
	// Cloud plans only work if we're cloud.
	cl, ok := b.(*cloud.Cloud)
	if !ok {
		return nil, errUnusable(fmt.Errorf("can't show a saved cloud plan unless the current root module is connected to Terraform Cloud"), "cloud plan")
	}

	result, err := cl.ShowPlanForRun(context.Background(), plan.RunID, plan.Hostname, redacted)
	if err != nil {
		err = errUnusable(err, "cloud plan")
	}
	return result, err
}

// maybeGetSchemas is a thin wrapper around [Meta.MaybeGetSchemas] that
// takes a [*statefile.File] instead of a [*states.State] and tolerates
// the state file being nil, since that's more convenient for the
// "tofu show" methods that may or may not have a state file to use.
func (c *ShowCommand) maybeGetSchemas(ctx context.Context, stateFile *statefile.File, config *configs.Config) (*tofu.Schemas, tfdiags.Diagnostics) {
	ctx, span := tracing.Tracer().Start(ctx, "Get Schemas")
	defer span.End()

	if stateFile == nil {
		return nil, nil
	}

	schemas, diags := c.MaybeGetSchemas(ctx, stateFile.State, config)
	if diags.HasErrors() {
		tracing.SetSpanError(span, diags.Err())
		return nil, diags
	}

	return schemas, nil

}

// getDataFromPlanfileReader returns a plan, statefile, and config, extracted from a local plan file.
func getDataFromPlanfileReader(ctx context.Context, planReader *planfile.Reader, rootCall configs.StaticModuleCall) (*plans.Plan, *statefile.File, *configs.Config, error) {
	// Get plan
	plan, err := planReader.ReadPlan()
	if err != nil {
		return nil, nil, nil, err
	}

	// Get statefile
	stateFile, err := planReader.ReadStateFile()
	if err != nil {
		return nil, nil, nil, err
	}

	subCall := rootCall.WithVariables(func(variable *configs.Variable) (cty.Value, hcl.Diagnostics) {
		var diags hcl.Diagnostics

		name := variable.Name
		v, ok := plan.VariableValues[name]
		if !ok {
			if variable.Required() {
				// This should not happen...
				return cty.DynamicVal, diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Missing plan variable " + variable.Name,
				})
			}
			return variable.Default, nil
		}

		parsed, parsedErr := v.Decode(cty.DynamicPseudoType)
		if parsedErr != nil {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  parsedErr.Error(),
			})
		}
		return parsed, diags
	})

	// Get config
	config, diags := planReader.ReadConfig(ctx, subCall)
	if diags.HasErrors() {
		return nil, nil, nil, errUnusable(diags.Err(), "local plan")
	}

	return plan, stateFile, config, err
}

// getStateFromPath returns a statefile if the user-supplied path points to a statefile.
func getStateFromPath(path string, enc encryption.Encryption) (*statefile.File, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("Error loading statefile: %w", err)
	}
	defer file.Close()

	var stateFile *statefile.File
	stateFile, err = statefile.Read(file, enc.State())
	if err != nil {
		return nil, fmt.Errorf("Error reading %s as a statefile: %w", path, err)
	}
	return stateFile, nil
}

// getStateFromBackend returns the State for the current workspace, if available.
func getStateFromBackend(ctx context.Context, b backend.Backend, workspace string) (*statefile.File, error) {
	ctx, span := tracing.Tracer().Start(ctx, "Get State from Backend")
	defer span.End()
	// Get the state store for the given workspace
	stateStore, err := b.StateMgr(ctx, workspace)
	if err != nil {
		tracing.SetSpanError(span, err)
		return nil, fmt.Errorf("failed to load state manager: %w", err)
	}

	// Refresh the state store with the latest state snapshot from persistent storage
	if err := stateStore.RefreshState(context.TODO()); err != nil {
		tracing.SetSpanError(span, err)
		return nil, fmt.Errorf("failed to load state: %w", err)
	}

	// Get the latest state snapshot and return it
	stateFile := statemgr.Export(stateStore)
	return stateFile, nil
}

// showConfiguration returns a function that will display the current configuration
// in JSON format. This is a new feature that requires -json to be specified.
func (c *ShowCommand) showConfiguration(ctx context.Context) (showRenderFunc, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// Check if the directory is empty
	empty, err := configs.IsEmptyDir(".")
	if err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Error validating configuration directory",
			fmt.Sprintf("OpenTofu encountered an unexpected error while verifying that the given configuration directory is valid: %s.", err),
		))
		return nil, diags
	}
	if empty {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"No configuration files",
			"This directory contains no OpenTofu configuration files.",
		))
		return nil, diags
	}

	// Load the configuration
	config, configDiags := c.loadConfig(ctx, ".")
	diags = diags.Append(configDiags)
	if configDiags.HasErrors() {
		return nil, diags
	}

	// Load provider schemas (without state)
	schemas, schemaDiags := c.MaybeGetSchemas(ctx, nil, config)
	diags = diags.Append(schemaDiags)
	if schemaDiags.HasErrors() {
		return nil, diags
	}
	if schemas == nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to load provider schemas",
			"The configuration cannot be shown without provider schema information.",
		))
		return nil, diags
	}

	// Return a function that will render the configuration as JSON
	return func(view views.Show) int {
		// Display the configuration using the view
		return view.DisplayConfig(config, schemas)
	}, diags
}
