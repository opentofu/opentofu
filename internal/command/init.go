// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/cloud"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/getproviders"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tofu"
	"github.com/opentofu/opentofu/internal/tofumigrate"
	"github.com/opentofu/opentofu/internal/tracing"
	"github.com/opentofu/opentofu/internal/tracing/traceattrs"
	"github.com/posener/complete"
)

// InitCommand is a Command implementation that takes a Terraform
// module and clones it to the working directory.
type InitCommand struct {
	Meta
}

func (c *InitCommand) Run(args []string) int {
	ctx := c.CommandContext()

	ctx, span := tracing.Tracer().Start(ctx, "Init")
	defer span.End()

	cfg := &initCfg{}
	flagSet := c.configureFlags(cfg, args)
	nonFlagArgs, exitCode := parseFlags(&c.Meta, flagSet, args, cfg)
	if exitCode > 0 {
		return exitCode
	}
	initActions := initActs{
		Meta:    &c.Meta,
		initCfg: cfg,
	}
	return runInit(ctx, nonFlagArgs, &initActions)
}

func runInit(ctx context.Context, nonFlagArgs []string, c *initActs) int {
	if c.migrateState && c.reconfigure {
		c.Ui.Error("The -migrate-state and -reconfigure options are mutually-exclusive")
		return 1
	}

	// Copying the state only happens during backend migration, so setting
	// -force-copy implies -migrate-state
	if c.forceInitCopy {
		c.migrateState = true
	}

	var diags tfdiags.Diagnostics

	if len(c.flagPluginPath) > 0 {
		c.pluginPath = c.flagPluginPath
	}

	// Validate the arg count and get the working directory
	path, err := modulePath(nonFlagArgs)
	if err != nil {
		c.Ui.Error(err.Error())
		return 1
	}

	if err := c.storePluginPath(c.pluginPath); err != nil {
		c.Ui.Error(fmt.Sprintf("Error saving -plugin-path values: %s", err))
		return 1
	}

	// Initialization can be aborted by interruption signals
	ctx, done := c.InterruptibleContext(ctx)
	defer done()

	// This will track whether we outputted anything so that we know whether
	// to output a newline before the success message
	var header bool

	if c.flagFromModule != "" {
		src := c.flagFromModule

		empty, err := configs.IsEmptyDir(path)
		if err != nil {
			c.Ui.Error(fmt.Sprintf("Error validating destination directory: %s", err))
			return 1
		}
		if !empty {
			c.Ui.Error(strings.TrimSpace(errInitCopyNotEmpty))
			return 1
		}

		c.Ui.Output(c.Colorize().Color(fmt.Sprintf(
			"[reset][bold]Copying configuration[reset] from %q...", src,
		)))
		header = true

		hooks := uiModuleInstallHooks{
			Ui:             c.Ui,
			ShowLocalPaths: false, // since they are in a weird location for init
		}

		ctx, span := tracing.Tracer().Start(ctx, "From module", tracing.SpanAttributes(
			traceattrs.OpenTofuModuleSource(src),
		))
		defer span.End()

		initDirFromModuleAbort, initDirFromModuleDiags := c.initDirFromModule(ctx, path, src, hooks)
		diags = diags.Append(initDirFromModuleDiags)
		if initDirFromModuleAbort || initDirFromModuleDiags.HasErrors() {
			c.showDiagnostics(diags)
			tracing.SetSpanError(span, initDirFromModuleDiags)
			span.End()
			return 1
		}

		c.Ui.Output("")
	}

	// If our directory is empty, then we're done. We can't get or set up
	// the backend with an empty directory.
	empty, err := configs.IsEmptyDir(path)
	if err != nil {
		diags = diags.Append(fmt.Errorf("Error checking configuration: %w", err))
		c.showDiagnostics(diags)
		return 1
	}
	if empty {
		c.Ui.Output(c.Colorize().Color(strings.TrimSpace(outputInitEmpty)))
		return 0
	}

	// Load just the root module to begin backend and module initialization
	rootModEarly, earlyConfDiags := c.loadSingleModuleWithTests(ctx, path, c.testsDirectory)

	// There may be parsing errors in config loading but these will be shown later _after_
	// checking for core version requirement errors. Not meeting the version requirement should
	// be the first error displayed if that is an issue, but other operations are required
	// before being able to check core version requirements.
	if rootModEarly == nil {
		c.Ui.Error(c.Colorize().Color(strings.TrimSpace(errInitConfigError)))
		diags = diags.Append(earlyConfDiags)
		c.showDiagnostics(diags)

		return 1
	}

	var enc encryption.Encryption
	// If backend flag is explicitly set to false i.e -backend=false, we disable state and plan encryption
	if c.backendFlagSet && !c.flagBackend {
		enc = encryption.Disabled()
	} else {
		// Load the encryption configuration
		var encDiags tfdiags.Diagnostics
		enc, encDiags = c.EncryptionFromModule(ctx, rootModEarly)
		diags = diags.Append(encDiags)
		if encDiags.HasErrors() {
			c.showDiagnostics(diags)
			return 1
		}
	}

	var back backend.Backend

	// There may be config errors or backend init errors but these will be shown later _after_
	// checking for core version requirement errors.
	var backDiags tfdiags.Diagnostics
	var backendOutput bool

	switch {
	case c.flagCloud && rootModEarly.CloudConfig != nil:
		back, backendOutput, backDiags = c.initCloud(ctx, rootModEarly, c.flagConfigExtra, enc)
	case c.flagBackend:
		back, backendOutput, backDiags = c.initBackend(ctx, rootModEarly, c.flagConfigExtra, enc)
	default:
		// load the previously-stored backend config
		back, backDiags = c.backendFromState(ctx, enc.State())
	}
	if backendOutput {
		header = true
	}

	var state *states.State

	// If we have a functional backend (either just initialized or initialized
	// on a previous run) we'll use the current state as a potential source
	// of provider dependencies.
	if back != nil {
		c.ignoreRemoteVersionConflict(back)
		workspace, err := c.Workspace(ctx)
		if err != nil {
			c.Ui.Error(fmt.Sprintf("Error selecting workspace: %s", err))
			return 1
		}
		sMgr, err := back.StateMgr(ctx, workspace)
		if err != nil {
			c.Ui.Error(fmt.Sprintf("Error loading state: %s", err))
			return 1
		}

		if err := sMgr.RefreshState(context.TODO()); err != nil {
			c.Ui.Error(fmt.Sprintf("Error refreshing state: %s", err))
			return 1
		}

		state = sMgr.State()
	}

	if c.flagGet {
		modsOutput, modsAbort, modsDiags := c.getModules(ctx, path, c.testsDirectory, rootModEarly, c.flagUpgrade)
		diags = diags.Append(modsDiags)
		if modsAbort || modsDiags.HasErrors() {
			c.showDiagnostics(diags)
			return 1
		}
		if modsOutput {
			header = true
		}
	}

	// With all of the modules (hopefully) installed, we can now try to load the
	// whole configuration tree.
	config, confDiags := c.loadConfigWithTests(ctx, path, c.testsDirectory)
	// configDiags will be handled after the version constraint check, since an
	// incorrect version of tofu may be producing errors for configuration
	// constructs added in later versions.

	// Before we go further, we'll check to make sure none of the modules in
	// the configuration declare that they don't support this OpenTofu
	// version, so we can produce a version-related error message rather than
	// potentially-confusing downstream errors.
	versionDiags := tofu.CheckCoreVersionRequirements(config)
	if versionDiags.HasErrors() {
		c.showDiagnostics(versionDiags)
		return 1
	}

	// We've passed the core version check, now we can show errors from the
	// configuration and backend initialization.

	// Now, we can check the diagnostics from the early configuration and the
	// backend.
	diags = diags.Append(earlyConfDiags.StrictDeduplicateMerge(backDiags))
	if earlyConfDiags.HasErrors() {
		c.Ui.Error(strings.TrimSpace(errInitConfigError))
		c.showDiagnostics(diags)
		return 1
	}

	// Now, we can show any errors from initializing the backend, but we won't
	// show the errInitConfigError preamble as we didn't detect problems with
	// the early configuration.
	if backDiags.HasErrors() {
		c.showDiagnostics(diags)
		return 1
	}

	// If everything is ok with the core version check and backend initialization,
	// show other errors from loading the full configuration tree.
	diags = diags.Append(confDiags)
	if confDiags.HasErrors() {
		c.Ui.Error(strings.TrimSpace(errInitConfigError))
		c.showDiagnostics(diags)
		return 1
	}

	if cb, ok := back.(*cloud.Cloud); ok {
		if c.RunningInAutomation {
			if err := cb.AssertImportCompatible(config); err != nil {
				diags = diags.Append(tfdiags.Sourceless(tfdiags.Error, "Compatibility error", err.Error()))
				c.showDiagnostics(diags)
				return 1
			}
		}
	}

	if state != nil {
		// Since we now have the full configuration loaded, we can use it to migrate the in-memory state view
		// prior to fetching providers.
		migratedState, migrateDiags := tofumigrate.MigrateStateProviderAddresses(config, state)
		diags = diags.Append(migrateDiags)
		if migrateDiags.HasErrors() {
			c.showDiagnostics(diags)
			return 1
		}
		state = migratedState
	}

	// Now that we have loaded all modules, check the module tree for missing providers.
	providersOutput, providersAbort, providerDiags := c.getProviders(ctx, config, state, c.flagUpgrade, c.flagPluginPath, c.flagLockfile)
	diags = diags.Append(providerDiags)
	if providersAbort || providerDiags.HasErrors() {
		c.showDiagnostics(diags)
		return 1
	}
	if providersOutput {
		header = true
	}

	// If we outputted information, then we need to output a newline
	// so that our success message is nicely spaced out from prior text.
	if header {
		c.Ui.Output("")
	}

	// If we accumulated any warnings along the way that weren't accompanied
	// by errors then we'll output them here so that the success message is
	// still the final thing shown.
	c.showDiagnostics(diags)
	_, cloud := back.(*cloud.Cloud)
	output := outputInitSuccess
	if cloud {
		output = outputInitSuccessCloud
	}

	c.Ui.Output(c.Colorize().Color(strings.TrimSpace(output)))

	if !c.RunningInAutomation {
		// If we're not running in an automation wrapper, give the user
		// some more detailed next steps that are appropriate for interactive
		// shell usage.
		output = outputInitSuccessCLI
		if cloud {
			output = outputInitSuccessCLICloud
		}
		c.Ui.Output(c.Colorize().Color(strings.TrimSpace(output)))
	}
	return 0
}

func (c *InitCommand) configureFlags(flags *initCfg, args []string) *flag.FlagSet {
	flags.flagConfigExtra = newRawFlags("-backend-config")

	args = c.Meta.process(args)
	cmdFlags := c.Meta.extendedFlagSet("init")
	cmdFlags.BoolVar(&flags.flagBackend, "backend", true, "")
	cmdFlags.BoolVar(&flags.flagCloud, "cloud", true, "")
	cmdFlags.Var(flags.flagConfigExtra, "backend-config", "")
	cmdFlags.StringVar(&flags.flagFromModule, "from-module", "", "copy the source of the given module into the directory before init")
	cmdFlags.BoolVar(&flags.flagGet, "get", true, "")
	cmdFlags.BoolVar(&c.forceInitCopy, "force-copy", false, "suppress prompts about copying state data")
	cmdFlags.BoolVar(&c.Meta.stateLock, "lock", true, "lock state")
	cmdFlags.DurationVar(&c.Meta.stateLockTimeout, "lock-timeout", 0, "lock timeout")
	cmdFlags.BoolVar(&c.reconfigure, "reconfigure", false, "reconfigure")
	cmdFlags.BoolVar(&c.migrateState, "migrate-state", false, "migrate state")
	cmdFlags.BoolVar(&flags.flagUpgrade, "upgrade", false, "")
	cmdFlags.Var(&flags.flagPluginPath, "plugin-dir", "plugin directory")
	cmdFlags.StringVar(&flags.flagLockfile, "lockfile", "", "Set a dependency lockfile mode")
	cmdFlags.BoolVar(&c.Meta.ignoreRemoteVersion, "ignore-remote-version", false, "continue even if remote and local OpenTofu versions are incompatible")
	cmdFlags.StringVar(&flags.testsDirectory, "test-directory", "tests", "test-directory")
	cmdFlags.BoolVar(&c.outputInJSON, "json", false, "json")
	cmdFlags.Usage = func() { c.Ui.Error(c.Help()) }
	return cmdFlags
}

func parseFlags(setHere *Meta, cmdFlags *flag.FlagSet, args []string, flags *initCfg) ([]string, int) {
	if err := cmdFlags.Parse(args); err != nil {
		return nil, 1
	}

	if setHere.outputInJSON {
		setHere.color = false
		setHere.Color = false
		setHere.oldUi = setHere.Ui
		setHere.Ui = &WrappedUi{
			cliUi:        setHere.oldUi,
			jsonView:     views.NewJSONView(setHere.View),
			outputInJSON: true,
		}
	}

	flags.backendFlagSet = arguments.FlagIsSet(cmdFlags, "backend")
	flags.cloudFlagSet = arguments.FlagIsSet(cmdFlags, "cloud")

	switch {
	case flags.backendFlagSet && flags.cloudFlagSet:
		setHere.Ui.Error("The -backend and -cloud options are aliases of one another and mutually-exclusive in their use")
		return nil, 1
	case flags.backendFlagSet:
		flags.flagCloud = flags.flagBackend
	case flags.cloudFlagSet:
		flags.flagBackend = flags.flagCloud
	}
	return cmdFlags.Args(), 0
}

// warnOnFailedImplicitProvReference returns a warn diagnostic when the downloader fails to fetch a provider that is implicitly referenced.
// In other words, if the failed to download provider is having no required_providers entry, this function is trying to give to the user
// more information on the source of the issue and gives also instructions on how to fix it.
func warnOnFailedImplicitProvReference(provider addrs.Provider, qualifs *getproviders.ProvidersQualification) tfdiags.Diagnostics {
	if _, ok := qualifs.Explicit[provider]; ok {
		return nil
	}
	refs, ok := qualifs.Implicit[provider]
	if !ok || len(refs) == 0 {
		// If there is no implicit reference for that provider, do not write the warn, let just the error to be returned.
		return nil
	}

	// NOTE: if needed, in the future we can use the rest of the "refs" to print all the culprits or at least to give
	// a hint on how many resources are causing this
	ref := refs[0]
	if ref.ProviderAttribute {
		return nil
	}
	details := fmt.Sprintf(
		implicitProviderReferenceBody,
		ref.CfgRes.String(),
		provider.Type,
		provider.ForDisplay(),
		provider.Type,
		ref.CfgRes.Resource.Type,
		provider.Type,
	)
	return tfdiags.Diagnostics{}.Append(
		&hcl.Diagnostic{
			Severity: hcl.DiagWarning,
			Subject:  ref.Ref.ToHCL().Ptr(),
			Summary:  implicitProviderReferenceHead,
			Detail:   details,
		})
}

func (c *InitCommand) AutocompleteArgs() complete.Predictor {
	return complete.PredictDirs("")
}

func (c *InitCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{
		"-backend":        completePredictBoolean,
		"-cloud":          completePredictBoolean,
		"-backend-config": complete.PredictFiles("*.tfvars"), // can also be key=value, but we can't "predict" that
		"-force-copy":     complete.PredictNothing,
		"-from-module":    completePredictModuleSource,
		"-get":            completePredictBoolean,
		"-input":          completePredictBoolean,
		"-lock":           completePredictBoolean,
		"-lock-timeout":   complete.PredictAnything,
		"-no-color":       complete.PredictNothing,
		"-plugin-dir":     complete.PredictDirs(""),
		"-reconfigure":    complete.PredictNothing,
		"-migrate-state":  complete.PredictNothing,
		"-upgrade":        completePredictBoolean,
	}
}

func (c *InitCommand) Help() string {
	helpText := `
Usage: tofu [global options] init [options]

  Initialize a new or existing OpenTofu working directory by creating
  initial files, loading any remote state, downloading modules, etc.

  This is the first command that should be run for any new or existing
  OpenTofu configuration per machine. This sets up all the local data
  necessary to run OpenTofu that is typically not committed to version
  control.

  This command is always safe to run multiple times. Though subsequent runs
  may give errors, this command will never delete your configuration or
  state. Even so, if you have important information, please back it up prior
  to running this command, just in case.

Options:

  -backend=false          Disable backend or cloud backend initialization
                          for this configuration and use what was previously
                          initialized instead.

                          aliases: -cloud=false

  -backend-config=path    Configuration to be merged with what is in the
                          configuration file's 'backend' block. This can be
                          either a path to an HCL file with key/value
                          assignments (same format as terraform.tfvars) or a
                          'key=value' format, and can be specified multiple
                          times. The backend type must be in the configuration
                          itself.

  -compact-warnings       If OpenTofu produces any warnings that are not
                          accompanied by errors, show them in a more compact
                          form that includes only the summary messages.

  -consolidate-warnings   If OpenTofu produces any warnings, no consolidation
                          will be performed. All locations, for all warnings
                          will be listed. Enabled by default.

  -consolidate-errors     If OpenTofu produces any errors, no consolidation
                          will be performed. All locations, for all errors
                          will be listed. Disabled by default

  -force-copy             Suppress prompts about copying state data when
                          initializing a new state backend. This is
                          equivalent to providing a "yes" to all confirmation
                          prompts.

  -from-module=SOURCE     Copy the contents of the given module into the target
                          directory before initialization.

  -get=false              Disable downloading modules for this configuration.

  -input=false            Disable interactive prompts. Note that some actions may
                          require interactive prompts and will error if input is
                          disabled.

  -lock=false             Don't hold a state lock during backend migration.
                          This is dangerous if others might concurrently run
                          commands against the same workspace.

  -lock-timeout=0s        Duration to retry a state lock.

  -no-color               If specified, output won't contain any color.

  -plugin-dir             Directory containing plugin binaries. This overrides all
                          default search paths for plugins, and prevents the
                          automatic installation of plugins. This flag can be used
                          multiple times.

  -reconfigure            Reconfigure a backend, ignoring any saved
                          configuration.

  -migrate-state          Reconfigure a backend, and attempt to migrate any
                          existing state.

  -upgrade                Install the latest module and provider versions
                          allowed within configured constraints, overriding the
                          default behavior of selecting exactly the version
                          recorded in the dependency lockfile.

  -lockfile=MODE          Set a dependency lockfile mode.
                          Currently only "readonly" is valid.

  -ignore-remote-version  A rare option used for cloud backend and the remote backend
                          only. Set this to ignore checking that the local and remote
                          OpenTofu versions use compatible state representations, making
                          an operation proceed even when there is a potential mismatch.
                          See the documentation on configuring OpenTofu with
                          cloud backend for more information.

  -test-directory=path    Set the OpenTofu test directory, defaults to "tests". When set, the
                          test command will search for test files in the current directory and
                          in the one specified by the flag.

  -json                   Produce output in a machine-readable JSON format, 
                          suitable for use in text editor integrations and other 
                          automated systems. Always disables color.

  -var 'foo=bar'          Set a value for one of the input variables in the root
                          module of the configuration. Use this option more than
                          once to set more than one variable.

  -var-file=filename      Load variable values from the given file, in addition
                          to the default files terraform.tfvars and *.auto.tfvars.
                          Use this option more than once to include more than one
                          variables file.

`
	return strings.TrimSpace(helpText)
}

func (c *InitCommand) Synopsis() string {
	return "Prepare your working directory for other commands"
}

const errInitConfigError = `
[reset]OpenTofu encountered problems during initialization, including problems
with the configuration, described below.

The OpenTofu configuration must be valid before initialization so that
OpenTofu can determine which modules and providers need to be installed.
`

const errInitCopyNotEmpty = `
The working directory already contains files. The -from-module option requires
an empty directory into which a copy of the referenced module will be placed.

To initialize the configuration already in this working directory, omit the
-from-module option.
`

const outputInitEmpty = `
[reset][bold]OpenTofu initialized in an empty directory![reset]

The directory has no OpenTofu configuration files. You may begin working
with OpenTofu immediately by creating OpenTofu configuration files.
`

const outputInitSuccess = `
[reset][bold][green]OpenTofu has been successfully initialized![reset][green]
`

const outputInitSuccessCloud = `
[reset][bold][green]Cloud backend has been successfully initialized![reset][green]
`

const outputInitSuccessCLI = `[reset][green]
You may now begin working with OpenTofu. Try running "tofu plan" to see
any changes that are required for your infrastructure. All OpenTofu commands
should now work.

If you ever set or change modules or backend configuration for OpenTofu,
rerun this command to reinitialize your working directory. If you forget, other
commands will detect it and remind you to do so if necessary.
`

const outputInitSuccessCLICloud = `[reset][green]
You may now begin working with cloud backend. Try running "tofu plan" to
see any changes that are required for your infrastructure.

If you ever set or change modules or OpenTofu Settings, run "tofu init"
again to reinitialize your working directory.
`

// providerProtocolTooOld is a message sent to the CLI UI if the provider's
// supported protocol versions are too old for the user's version of tofu,
// but a newer version of the provider is compatible.
const providerProtocolTooOld = `Provider %q v%s is not compatible with OpenTofu %s.
Provider version %s is the latest compatible version. Select it with the following version constraint:
	version = %q

OpenTofu checked all of the plugin versions matching the given constraint:
	%s

Consult the documentation for this provider for more information on compatibility between provider and OpenTofu versions.
`

// providerProtocolTooNew is a message sent to the CLI UI if the provider's
// supported protocol versions are too new for the user's version of tofu,
// and the user could either upgrade tofu or choose an older version of the
// provider.
const providerProtocolTooNew = `Provider %q v%s is not compatible with OpenTofu %s.
You need to downgrade to v%s or earlier. Select it with the following constraint:
	version = %q

OpenTofu checked all of the plugin versions matching the given constraint:
	%s

Consult the documentation for this provider for more information on compatibility between provider and OpenTofu versions.
Alternatively, upgrade to the latest version of OpenTofu for compatibility with newer provider releases.
`

// No version of the provider is compatible.
const errProviderVersionIncompatible = `No compatible versions of provider %s were found.`

// incompleteLockFileInformationHeader is the summary displayed to users when
// the lock file has only recorded local hashes.
const incompleteLockFileInformationHeader = `Incomplete lock file information for providers`

// incompleteLockFileInformationBody is the body of text displayed to users when
// the lock file has only recorded local hashes.
const incompleteLockFileInformationBody = `Due to your customized provider installation methods, OpenTofu was forced to calculate lock file checksums locally for the following providers:
  - %s

The current .terraform.lock.hcl file only includes checksums for %s, so OpenTofu running on another platform will fail to install these providers.

To calculate additional checksums for another platform, run:
  tofu providers lock -platform=linux_amd64
(where linux_amd64 is the platform to generate)`

const implicitProviderReferenceHead = `Automatically-inferred provider dependency`

const implicitProviderReferenceBody = `Due to the prefix of the resource type name OpenTofu guessed that you intended to associate %s with a provider whose local name is "%s", but that name is not declared in this module's required_providers block. OpenTofu therefore guessed that you intended to use %s, but that provider does not exist.

Make at least one of the following changes to tell OpenTofu which provider to use:

- Add a declaration for local name "%s" to this module's required_providers block, specifying the full source address for the provider you intended to use.
- Verify that "%s" is the correct resource type name to use. Did you omit a prefix which would imply the correct provider?
- Use a "provider" argument within this resource block to override OpenTofu's automatic selection of the local name "%s".
`
