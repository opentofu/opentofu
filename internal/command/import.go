// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/mitchellh/cli"
	"github.com/opentofu/opentofu/internal/command/flags"
	"github.com/opentofu/opentofu/internal/tracing"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tofu"
)

// ImportCommand is a cli.Command implementation that imports resources
// into the OpenTofu state.
type ImportCommand struct {
	Meta
}

func (c *ImportCommand) Run(rawArgs []string) int {
	ctx := c.CommandContext()
	ctx, span := tracing.Tracer().Start(ctx, "Import")
	defer span.End()

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
	args, closer, diags := arguments.ParseImport(rawArgs, c.WorkingDir)
	defer closer()

	// Instantiate the view, even if there are flag errors, so that we render
	// diagnostics according to the desired view
	view := views.NewImport(args.ViewOptions, c.View)
	// ... and initialise the Meta.Ui to wrap Meta.View into a new implementation
	// that is able to print by using View abstraction and use the Meta.Ui
	// to ask for the user input.
	c.Meta.configureUiFromView(args.ViewOptions)

	if diags.HasErrors() {
		view.Diagnostics(diags)
		if args.ViewOptions.ViewType == arguments.ViewJSON {
			return 1 // in case it's json, do not print the help of the command
		}
		return cli.RunResultHelp
	}
	c.configureBackendFlags(args)
	c.GatherVariables(args.Vars)

	// Parse the provided resource address.
	traversalSrc := []byte(args.ResourceAddress)
	traversal, travDiags := hclsyntax.ParseTraversalAbs(traversalSrc, "<import-address>", hcl.Pos{Line: 1, Column: 1})
	diags = diags.Append(travDiags)
	if travDiags.HasErrors() {
		// NOTE: The call to registerSynthConfigSource works well with the view.Diagnostics too since the view is
		// configured in [Meta.initConfigLoader] with a callback to get the sources when it prints the diagnostics.
		c.registerSynthConfigSource("<import-address>", traversalSrc) // so we can include a source snippet
		view.Diagnostics(diags)
		view.InvalidAddressReference()
		return 1
	}
	addr, addrDiags := addrs.ParseAbsResourceInstance(traversal)
	diags = diags.Append(addrDiags)
	if addrDiags.HasErrors() {
		// NOTE: The call to registerSynthConfigSource works well with the view.Diagnostics too since the view is
		// configured in [Meta.initConfigLoader] with a callback to get the sources when it prints the diagnostics.
		c.registerSynthConfigSource("<import-address>", traversalSrc) // so we can include a source snippet
		view.Diagnostics(diags)
		view.InvalidAddressReference()
		return 1
	}

	if addr.Resource.Resource.Mode != addrs.ManagedResourceMode {
		var what string
		switch addr.Resource.Resource.Mode {
		case addrs.DataResourceMode:
			what = "a data resource"
		case addrs.EphemeralResourceMode:
			what = "an ephemeral resource"
		default:
			what = "a resource type"
		}
		diags = diags.Append(fmt.Errorf("A managed resource address is required. Importing into %s is not allowed.", what))
		view.Diagnostics(diags)
		return 1
	}

	if !c.dirIsConfigPath(args.ConfigPath) {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "No OpenTofu configuration files",
			Detail: fmt.Sprintf(
				"The directory %s does not contain any OpenTofu configuration files (.tf or .tf.json). To specify a different configuration directory, use the -config=\"...\" command line option.",
				args.ConfigPath,
			),
		})
		view.Diagnostics(diags)
		return 1
	}

	// Load the full config, so we can verify that the target resource is
	// already configured.
	config, configDiags := c.loadConfig(ctx, args.ConfigPath)
	diags = diags.Append(configDiags)
	if configDiags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	// Load the encryption configuration
	enc, encDiags := c.EncryptionFromPath(ctx, args.ConfigPath)
	diags = diags.Append(encDiags)
	if encDiags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	// Verify that the given address points to something that exists in config.
	// This is to reduce the risk that a typo in the resource address will
	// import something that OpenTofu will want to immediately destroy on
	// the next plan, and generally acts as a reassurance of user intent.
	targetConfig := config.DescendentForInstance(addr.Module)
	if targetConfig == nil {
		modulePath := addr.Module.String()
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Import to non-existent module",
			Detail: fmt.Sprintf(
				"%s is not defined in the configuration. Please add configuration for this module before importing into it.",
				modulePath,
			),
		})
		view.Diagnostics(diags)
		return 1
	}
	targetMod := targetConfig.Module
	rcs := targetMod.ManagedResources
	var rc *configs.Resource
	resourceRelAddr := addr.Resource.Resource
	for _, thisRc := range rcs {
		if resourceRelAddr.Type == thisRc.Type && resourceRelAddr.Name == thisRc.Name {
			rc = thisRc
			break
		}
	}
	if rc == nil {
		modulePath := addr.Module.String()
		if modulePath == "" {
			modulePath = "the root module"
		}

		view.Diagnostics(diags)
		view.MissingResourceConfiguration(addr, modulePath, resourceRelAddr.Type, resourceRelAddr.Name)
		return 1
	}

	// Check for user-supplied plugin path
	var err error
	if c.pluginPath, err = c.loadPluginPath(); err != nil {
		view.Diagnostics(tfdiags.Diagnostics{}.Append(fmt.Errorf("Error loading plugin path: %s", err)))
		return 1
	}

	// Load the backend
	b, backendDiags := c.Backend(ctx, &BackendOpts{
		Config: config.Module.Backend,
	}, enc.State())
	diags = diags.Append(backendDiags)
	if backendDiags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	// We require a backend.Local to build a context.
	// This isn't necessarily a "local.Local" backend, which provides local
	// operations, however that is the only current implementation. A
	// "local.Local" backend also doesn't necessarily provide local state, as
	// that may be delegated to a "remotestate.Backend".
	local, ok := b.(backend.Local)
	if !ok {
		view.UnsupportedLocalOp()
		return 1
	}

	// Build the operation
	opReq := c.Operation(ctx, b, args.ViewOptions, enc)
	opReq.ConfigDir = args.ConfigPath
	opReq.ConfigLoader, err = c.initConfigLoader()
	if err != nil {
		diags = diags.Append(err)
		view.Diagnostics(diags)
		return 1
	}
	opReq.Hooks = view.Hooks()
	{
		// Setup required variables/call for operation (usually done in Meta.RunOperation)
		var moreDiags, callDiags tfdiags.Diagnostics
		opReq.Variables, moreDiags = c.collectVariableValues()
		opReq.RootCall, callDiags = c.rootModuleCall(ctx, opReq.ConfigDir)
		diags = diags.Append(moreDiags).Append(callDiags)
		if moreDiags.HasErrors() {
			view.Diagnostics(diags)
			return 1
		}
	}
	opReq.View = view.Operation()

	// Check remote OpenTofu version is compatible
	remoteVersionDiags := c.remoteVersionCheck(b, opReq.Workspace)
	diags = diags.Append(remoteVersionDiags)
	view.Diagnostics(diags)
	if diags.HasErrors() {
		return 1
	}

	// Get the context
	lr, state, ctxDiags := local.LocalRun(ctx, opReq)
	diags = diags.Append(ctxDiags)
	if ctxDiags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	// Successfully creating the context can result in a lock, so ensure we release it
	defer func() {
		diags := opReq.StateLocker.Unlock()
		if diags.HasErrors() {
			view.Diagnostics(diags)
		}
	}()

	// Perform the import. Note that as you can see it is possible for this
	// API to import more than one resource at once. For now, we only allow
	// one while we stabilize this feature.
	newState, importDiags := lr.Core.Import(ctx, lr.Config, lr.InputState, &tofu.ImportOpts{
		Targets: []*tofu.ImportTarget{
			{
				CommandLineImportTarget: &tofu.CommandLineImportTarget{
					Addr: addr,
					ID:   args.ResourceID,
				},
			},
		},

		// The LocalRun idea is designed around our primary operations, so
		// the input variables end up represented as plan options even though
		// this particular operation isn't really a plan.
		SetVariables: lr.PlanOpts.SetVariables,
	})
	diags = diags.Append(importDiags)
	if diags.HasErrors() {
		view.Diagnostics(diags)
		return 1
	}

	// Get schemas, if possible, before writing state
	var schemas *tofu.Schemas
	if isCloudMode(b) {
		var schemaDiags tfdiags.Diagnostics
		schemas, schemaDiags = c.MaybeGetSchemas(ctx, newState, nil)
		diags = diags.Append(schemaDiags)
	}

	// Persist the final state
	log.Printf("[INFO] Writing state output to: %s", c.Meta.StateOutPath())
	if err := state.WriteState(newState); err != nil {
		view.Diagnostics(tfdiags.Diagnostics{}.Append(fmt.Errorf("Error writing state file: %s", err)))
		return 1
	}
	if err := state.PersistState(context.TODO(), schemas); err != nil {
		view.Diagnostics(tfdiags.Diagnostics{}.Append(fmt.Errorf("Error writing state file: %s", err)))
		return 1
	}

	view.Success()
	view.Diagnostics(diags)
	if diags.HasErrors() {
		return 1
	}

	return 0
}

// configureBackendFlags is a temporary shim until we move the flags for state management to a better placce
//
// TODO meta-refactor: remove this when the Meta fields configured here will be removed and replaced
// with proper arguments for the backend.
func (c *ImportCommand) configureBackendFlags(args *arguments.Import) {
	c.Meta.ignoreRemoteVersion = args.Backend.IgnoreRemoteVersion
	// TODO meta-refactor: unify these 2 args attributes with the state flags in arguments.extendedFlagSet
	//  https://github.com/opentofu/opentofu/blob/db8c872defd8666618649ef7e29fa2b809adfd5e/internal/command/arguments/extended.go#L320-L321
	c.Meta.stateLock = args.State.Lock
	c.Meta.stateLockTimeout = args.State.LockTimeout

	// TODO meta-refactor: remove this only when there is clear path of passing these from the "arguments" package to
	// the place where these needs to be used
	c.Meta.parallelism = args.Parallelism
	c.Meta.statePath = args.State.StatePath
	c.Meta.stateOutPath = args.State.StateOutPath
	c.Meta.backupPath = args.State.BackupPath

	// FIXME: the -input flag value is needed to initialize the backend and the
	// operation, but there is no clear path to pass this value down, so we
	// continue to mutate the Meta object state for now.
	c.Meta.input = args.ViewOptions.InputEnabled
}

// TODO meta-refactor: move this to arguments once all commands are using the same shim logic
func (c *ImportCommand) GatherVariables(args *arguments.Vars) {
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

func (c *ImportCommand) Help() string {
	helpText := `
Usage: tofu [global options] import [options] ADDR ID

  Import existing infrastructure into your OpenTofu state.

  This will find and import the specified resource into your OpenTofu
  state, allowing existing infrastructure to come under OpenTofu
  management without having to be initially created by OpenTofu.

  The ADDR specified is the address to import the resource to. Please
  see the documentation online for resource addresses. The ID is a
  resource-specific ID to identify that resource being imported. Please
  reference the documentation for the resource type you're importing to
  determine the ID syntax to use. It typically matches directly to the ID
  that the provider uses.

  This command will not modify your infrastructure, but it will make
  network requests to inspect parts of your infrastructure relevant to
  the resource being imported.

Options:

  -compact-warnings       If OpenTofu produces any warnings that are not
                          accompanied by errors, show them in a more compact
                          form that includes only the summary messages.

  -consolidate-warnings   If OpenTofu produces any warnings, no consolidation
                          will be performed. All locations, for all warnings
                          will be listed. Enabled by default.

  -consolidate-errors     If OpenTofu produces any errors, no consolidation
                          will be performed. All locations, for all errors
                          will be listed. Disabled by default

  -config=path            Path to a directory of OpenTofu configuration files
                          to use to configure the provider. Defaults to pwd.
                          If no config files are present, they must be provided
                          via the input prompts or env vars.

  -input=false            Disable interactive input prompts.

  -lock=false             Don't hold a state lock during the operation. This is
                          dangerous if others might concurrently run commands
                          against the same workspace.

  -lock-timeout=0s        Duration to retry a state lock.

  -no-color               If specified, output won't contain any color.

  -var 'foo=bar'          Set a variable in the OpenTofu configuration. This
                          flag can be set multiple times. This is only useful
                          with the "-config" flag.

  -var-file=foo           Set variables in the OpenTofu configuration from
                          a file. If "terraform.tfvars" or any ".auto.tfvars"
                          files are present, they will be automatically loaded.

  -ignore-remote-version  A rare option used for the remote backend only. See
                          the remote backend documentation for more information.

    
  -json                   The output of the command is printed in json format.

  -json-into=out.json     Produce the same output as -json, but sent directly
                          to the given file. This allows automation to preserve
                          the original human-readable output streams, while
                          capturing more detailed logs for machine analysis.

  -state, state-out, and -backup are legacy options supported for the local
  backend only. For more information, see the local backend's documentation.

`
	return strings.TrimSpace(helpText)
}

func (c *ImportCommand) Synopsis() string {
	return "Associate existing infrastructure with a OpenTofu resource"
}
