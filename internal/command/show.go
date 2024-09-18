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
	"github.com/opentofu/opentofu/internal/tofumigrate"
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
type ShowCommand struct {
	Meta
	viewType arguments.ViewType
}

func (c *ShowCommand) Run(rawArgs []string) int {
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
	enc, encDiags := c.Encryption()
	diags = diags.Append(encDiags)
	if encDiags.HasErrors() {
		c.showDiagnostics(diags)
		return 1
	}

	// Get the data we need to display
	plan, jsonPlan, stateFile, config, schemas, showDiags := c.show(args.Path, enc)
	diags = diags.Append(showDiags)
	if showDiags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	// Display the data
	return view.Display(config, plan, jsonPlan, stateFile, schemas)
}

func (c *ShowCommand) Help() string {
	helpText := `
Usage: tofu [global options] show [options] [path]

  Reads and outputs a OpenTofu state or plan file in a human-readable
  form. If no path is specified, the current state will be shown.

Options:

  -no-color           If specified, output won't contain any color.

  -json               If specified, output the OpenTofu plan or state in
                      a machine-readable form.

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

func (c *ShowCommand) show(path string, enc encryption.Encryption) (*plans.Plan, *cloudplan.RemotePlanJSON, *statefile.File, *configs.Config, *tofu.Schemas, tfdiags.Diagnostics) {
	var diags, showDiags, migrateDiags tfdiags.Diagnostics
	var plan *plans.Plan
	var jsonPlan *cloudplan.RemotePlanJSON
	var stateFile *statefile.File
	var config *configs.Config
	var schemas *tofu.Schemas

	// No plan file or state file argument provided,
	// so get the latest state snapshot
	if path == "" {
		stateFile, showDiags = c.showFromLatestStateSnapshot(enc)
		diags = diags.Append(showDiags)
		if showDiags.HasErrors() {
			return plan, jsonPlan, stateFile, config, schemas, diags
		}
	}

	// Plan file or state file argument provided,
	// so try to load the argument as a plan file first.
	// If that fails, try to load it as a statefile.
	if path != "" {
		plan, jsonPlan, stateFile, config, showDiags = c.showFromPath(path, enc)
		diags = diags.Append(showDiags)
		if showDiags.HasErrors() {
			return plan, jsonPlan, stateFile, config, schemas, diags
		}
	}

	if stateFile != nil {
		stateFile.State, migrateDiags = tofumigrate.MigrateStateProviderAddresses(config, stateFile.State)
		diags = diags.Append(migrateDiags)
		if migrateDiags.HasErrors() {
			return plan, jsonPlan, stateFile, config, schemas, diags
		}
	}

	// Get schemas, if possible
	if config != nil || stateFile != nil {
		schemas, diags = c.MaybeGetSchemas(stateFile.State, config)
		if diags.HasErrors() {
			return plan, jsonPlan, stateFile, config, schemas, diags
		}
	}

	return plan, jsonPlan, stateFile, config, schemas, diags
}
func (c *ShowCommand) showFromLatestStateSnapshot(enc encryption.Encryption) (*statefile.File, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// Load the backend
	b, backendDiags := c.Backend(nil, enc.State())
	diags = diags.Append(backendDiags)
	if backendDiags.HasErrors() {
		return nil, diags
	}
	c.ignoreRemoteVersionConflict(b)

	// Load the workspace
	workspace, err := c.Workspace()
	if err != nil {
		diags = diags.Append(fmt.Errorf("error selecting workspace: %w", err))
		return nil, diags
	}

	// Get the latest state snapshot from the backend for the current workspace
	stateFile, stateErr := getStateFromBackend(b, workspace)
	if stateErr != nil {
		diags = diags.Append(stateErr)
		return nil, diags
	}

	return stateFile, diags
}

func (c *ShowCommand) showFromPath(path string, enc encryption.Encryption) (*plans.Plan, *cloudplan.RemotePlanJSON, *statefile.File, *configs.Config, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	var planErr, stateErr error
	var plan *plans.Plan
	var jsonPlan *cloudplan.RemotePlanJSON
	var stateFile *statefile.File
	var config *configs.Config

	rootCall, callDiags := c.rootModuleCall(".")
	diags = diags.Append(callDiags)
	if diags.HasErrors() {
		return nil, nil, nil, nil, diags
	}

	// Path might be a local plan file, a bookmark to a saved cloud plan, or a
	// state file. First, try to get a plan and associated data from a local
	// plan file. If that fails, try to get a json plan from the path argument.
	// If that fails, try to get the statefile from the path argument.
	plan, jsonPlan, stateFile, config, planErr = c.getPlanFromPath(path, enc, rootCall)
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

			return nil, nil, nil, nil, diags
		}
	}
	return plan, jsonPlan, stateFile, config, diags
}

// getPlanFromPath returns a plan, json plan, statefile, and config if the
// user-supplied path points to either a local or cloud plan file. Note that
// some of the return values will be nil no matter what; local plan files do not
// yield a json plan, and cloud plans do not yield real plan/state/config
// structs. An error generally suggests that the given path is either a
// directory or a statefile.
func (c *ShowCommand) getPlanFromPath(path string, enc encryption.Encryption, rootCall configs.StaticModuleCall) (*plans.Plan, *cloudplan.RemotePlanJSON, *statefile.File, *configs.Config, error) {
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
		plan, stateFile, config, err = getDataFromPlanfileReader(lp, rootCall)
	} else if cp, ok := pf.Cloud(); ok {
		redacted := c.viewType != arguments.ViewJSON
		jsonPlan, err = c.getDataFromCloudPlan(cp, redacted, enc)
	}

	return plan, jsonPlan, stateFile, config, err
}

func (c *ShowCommand) getDataFromCloudPlan(plan *cloudplan.SavedPlanBookmark, redacted bool, enc encryption.Encryption) (*cloudplan.RemotePlanJSON, error) {
	// Set up the backend
	b, backendDiags := c.Backend(nil, enc.State())
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

// getDataFromPlanfileReader returns a plan, statefile, and config, extracted from a local plan file.
func getDataFromPlanfileReader(planReader *planfile.Reader, rootCall configs.StaticModuleCall) (*plans.Plan, *statefile.File, *configs.Config, error) {
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
	config, diags := planReader.ReadConfig(subCall)
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
func getStateFromBackend(b backend.Backend, workspace string) (*statefile.File, error) {
	// Get the state store for the given workspace
	stateStore, err := b.StateMgr(workspace)
	if err != nil {
		return nil, fmt.Errorf("Failed to load state manager: %w", err)
	}

	// Refresh the state store with the latest state snapshot from persistent storage
	if err := stateStore.RefreshState(); err != nil {
		return nil, fmt.Errorf("Failed to load state: %w", err)
	}

	// Get the latest state snapshot and return it
	stateFile := statemgr.Export(stateStore)
	return stateFile, nil
}
