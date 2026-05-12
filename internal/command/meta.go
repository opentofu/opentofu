// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/go-plugin"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/opentofu/opentofu/internal/command/flags"
	"github.com/opentofu/opentofu/internal/command/system"
	"github.com/opentofu/svchost/disco"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/backend/local"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/clistate"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/command/webbrowser"
	"github.com/opentofu/opentofu/internal/command/workdir"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configload"
	"github.com/opentofu/opentofu/internal/getmodules"
	"github.com/opentofu/opentofu/internal/getproviders"
	"github.com/opentofu/opentofu/internal/plugins"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/provisioners"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tofu"
)

// Meta are the meta-options that are available on all or most commands.
type Meta struct {
	// The exported fields below should be set by anyone using a
	// command with a Meta field. These are expected to be set externally
	// (not from within the command itself).

	// WorkingDir is an object representing the "working directory" where we're
	// running commands. In the normal case this literally refers to the
	// working directory of the OpenTofu process, though this can take on
	// a more symbolic meaning when the user has overridden default behavior
	// to specify a different working directory or to override the special
	// data directory where we'll persist settings that must survive between
	// consecutive commands.
	//
	// We're currently gradually migrating the various bits of state that
	// must persist between consecutive commands in a session to be encapsulated
	// in here, but we're not there yet and so there are also some methods on
	// Meta which directly read and modify paths inside the data directory.
	WorkingDir *workdir.Dir

	// SystemCfg holds the configuration attributes that are global for all
	// the commands and are used by different parts of the system.
	//SystemCfg system.Config
	SystemCfg system.Config

	View *views.View

	// Services provides access to remote endpoint information for
	// 'tofu-native' services running at a specific user-facing hostname.
	Services *disco.Disco

	// PluginCacheMayBreakDependencyLockFile is a temporary CLI configuration-based
	// opt out for the behavior of only using the plugin cache dir if its
	// contents match checksums recorded in the dependency lock file.
	//
	// This is an accommodation for those who currently essentially ignore the
	// dependency lock file -- treating it only as transient working directory
	// state -- and therefore don't care if the plugin cache dir causes the
	// checksums inside to only be sufficient for the computer where OpenTofu
	// is currently running.
	//
	// We intend to remove this exception again (making the CLI configuration
	// setting a silent no-op) in future once we've improved the dependency
	// lock file mechanism so that it's usable for everyone and there are no
	// longer any compelling reasons for folks to not lock their dependencies.
	PluginCacheMayBreakDependencyLockFile bool

	// ProviderSource allows determining the available versions of a provider
	// and determines where a distribution package for a particular
	// provider version can be obtained.
	ProviderSource getproviders.Source

	// ModulePackageFetcher is the client to use when fetching module packages
	// from remote locations. This object effectively represents the policy
	// for how to fetch remote module packages, which is decided by the caller.
	//
	// Leaving this nil means that only local modules (using relative paths
	// in the source address) are supported, which is only reasonable for
	// unit testing.
	ModulePackageFetcher *getmodules.PackageFetcher

	// MakeRegistryHTTPClient is a function called each time a command needs
	// an HTTP client that will be used to make requests to a module or
	// provider registry.
	//
	// This is used by package main to deal with some operator-configurable
	// settings for retries and timeouts. If this isn't set then a new client
	// with reasonable defaults for tests will be used instead.
	MakeRegistryHTTPClient func() *retryablehttp.Client

	// BrowserLauncher is used by commands that need to open a URL in a
	// web browser.
	BrowserLauncher webbrowser.Launcher

	// A context.Context provided by the caller -- typically "package main" --
	// which might be carrying telemetry-related metadata and so should be
	// used when creating downstream traces, etc.
	//
	// This isn't guaranteed to be set, so use [Meta.CommandContext] to
	// safely create a context for the entire execution of a command, which
	// will be connected to this parent context if it's present.
	CallerContext context.Context

	// When this channel is closed, the command will be cancelled.
	ShutdownCh <-chan struct{}

	// ProviderDevOverrides are providers where we ignore the lock file, the
	// configured version constraints, and the local cache directory and just
	// always use exactly the path specified. This is intended to allow
	// provider developers to easily test local builds without worrying about
	// what version number they might eventually be released as, or what
	// checksums they have.
	ProviderDevOverrides map[addrs.Provider]getproviders.PackageLocalDir

	// UnmanagedProviders are a set of providers that exist as processes
	// predating OpenTofu, which OpenTofu should use but not worry about the
	// lifecycle of.
	//
	// This is essentially a more extreme version of ProviderDevOverrides where
	// OpenTofu doesn't even worry about how the provider server gets launched,
	// just trusting that someone else did it before running OpenTofu.
	UnmanagedProviders map[addrs.Provider]*plugin.ReattachConfig

	// ----------------------------------------------------------
	// Protected: commands can set these
	// ----------------------------------------------------------

	// pluginPath is a user defined set of directories to look for plugins.
	// This is set during init with the `-plugin-dir` flag, saved to a file in
	// the data directory.
	// This overrides all other search paths when discovering plugins.
	pluginPath []string

	// Override certain behavior for tests within this package
	testingOverrides *testingOverrides

	// ----------------------------------------------------------
	// Private: do not set these
	// ----------------------------------------------------------

	// configLoader is a shared configuration loader that is used by
	// LoadConfig and other commands that access configuration files.
	// It is initialized on first use.
	configLoader configload.Loader

	// backendState is the currently active backend state
	backendState *clistate.BackendState

	// Variables for the context (private)
	variableArgs []flags.RawFlag
	input        bool

	// The fields below are expected to be set by the command via
	// command line flags. See the Apply command for an example.
	//
	// parallelism is used to control the number of concurrent operations
	// allowed when walking the graph
	stateArgs   arguments.State
	backendArgs arguments.Backend
	parallelism int

	// Used to cache the root module rootModuleCallCache and known variables.
	// This helps prevent duplicate errors/warnings.
	rootModuleCallCache *configs.StaticModuleCall
	inputVariableCache  map[string]backend.UnparsedVariableValue

	// Since `tofu providers lock` and `tofu providers mirror` have their own
	// logic to create the source to fetch providers through, we had to
	// plumb this configuration through the [Meta] type to reach that part too.
	// In any other cases, this configuration is built and used directly in `realMain`
	// when the providers sources are built.
	ProviderSourceLocationConfig getproviders.LocationConfig
}

type testingOverrides struct {
	Providers    map[addrs.Provider]providers.Factory
	Provisioners map[string]provisioners.Factory
}

// StateOutPath returns the true output path for the state file
func (m *Meta) StateOutPath() string {
	return m.stateArgs.StateOutPath
}

const (
	// InputModeEnvVar is the environment variable that, if set to "false" or
	// "0", causes tofu commands to behave as if the `-input=false` flag was
	// specified.
	InputModeEnvVar = "TF_INPUT"
)

// InputMode returns the type of input we should ask for in the form of
// tofu.InputMode which is passed directly to Context.Input.
func (m *Meta) InputMode() tofu.InputMode {
	if test || !m.input {
		return 0
	}

	if envVar := os.Getenv(InputModeEnvVar); envVar != "" {
		if v, err := strconv.ParseBool(envVar); err == nil {
			if !v {
				return 0
			}
		}
	}

	var mode tofu.InputMode
	mode |= tofu.InputModeProvider

	return mode
}

// UIInput returns a UIInput object to be used for asking for input.
func (m *Meta) UIInput() tofu.UIInput {
	return &UIInput{
		Colorize: m.View.Colorize(),
	}
}

// InterruptibleContext returns a context.Context that will be cancelled
// if the process is interrupted by a platform-specific interrupt signal.
//
// The typical way to use this is to pass the result of [Meta.CommandContext]
// as the base context, but that's appropriate only if the interruptible
// context is being created directly inside the "Run" method of a particular
// command, to create a context representing the entire remaining runtime of
// that command:
//
// As usual with cancelable contexts, the caller must always call the given
// cancel function once all operations are complete in order to make sure
// that the context resources will still be freed even if there is no
// interruption.
//
//	// This example is only for when using this function very early in
//	// the "Run" method of a Command implementation. If you already have
//	// an active context, pass that in as base instead.
//	ctx, done := c.InterruptibleContext(c.CommandContext())
//	defer done()
func (m *Meta) InterruptibleContext(base context.Context) (context.Context, context.CancelFunc) {
	if m.ShutdownCh == nil {
		// If we're running in a unit testing context without a shutdown
		// channel populated then we'll return an uncancelable channel.
		return base, func() {}
	}

	ctx, cancel := context.WithCancel(base)
	go func() {
		select {
		case <-m.ShutdownCh:
			cancel()
		case <-ctx.Done():
			// finished without being interrupted
		}
	}()
	return ctx, cancel
}

// CommandContext returns the "root context" to use in the main Run function
// of a command.
//
// This method is just a substitute for passing a context directly to the
// "Run" method of a command, which we can't do because that API is owned by
// mitchellh/cli rather than by OpenTofu. Use this only in situations
// comparable to the context having been passed in as an argument to Run.
//
// If the caller (e.g. "package main") provided a context when it instantiated
// the Meta then the returned context will inherit all of its values, deadlines,
// etc. If the caller did not provide a context then the result is an inert
// background context ready to be passed to other functions.
func (m *Meta) CommandContext() context.Context {
	if m.CallerContext == nil {
		return context.Background()
	}
	// We just return the caller context directly for now, since we don't
	// have anything to add to it.
	return m.CallerContext
}

// RunOperation executes the given operation on the given backend, blocking
// until that operation completes or is interrupted, and then returns
// the RunningOperation object representing the completed or
// aborted operation that is, despite the name, no longer running.
//
// An error is returned if the operation either fails to start or is cancelled.
// If the operation runs to completion then no error is returned even if the
// operation itself is unsuccessful. Use the "Result" field of the
// returned operation object to recognize operation-level failure.
func (m *Meta) RunOperation(ctx context.Context, b backend.Enhanced, opReq *backend.Operation) (*backend.RunningOperation, tfdiags.Diagnostics) {
	if opReq.View == nil {
		panic("RunOperation called with nil View")
	}
	if opReq.ConfigDir != "" {
		opReq.ConfigDir = m.WorkingDir.NormalizePath(opReq.ConfigDir)
	}

	// Inject variables and root module call
	var diags, callDiags tfdiags.Diagnostics
	opReq.Variables, diags = m.collectVariableValues()
	opReq.RootCall, callDiags = m.rootModuleCall(ctx, opReq.ConfigDir)
	diags = diags.Append(callDiags)
	if diags.HasErrors() {
		return nil, diags
	}

	op, err := b.Operation(ctx, opReq)
	if err != nil {
		return nil, diags.Append(fmt.Errorf("error starting operation: %w", err))
	}

	// Wait for the operation to complete or an interrupt to occur
	select {
	case <-m.ShutdownCh:
		// gracefully stop the operation
		op.Stop()

		// Notify the user
		opReq.View.Interrupted()

		// Still get the result, since there is still one
		select {
		case <-m.ShutdownCh:
			opReq.View.FatalInterrupt()

			// cancel the operation completely
			op.Cancel()

			// the operation should return asap
			// but timeout just in case
			select {
			case <-op.Done():
			case <-time.After(5 * time.Second):
			}

			return nil, diags.Append(errors.New("operation canceled"))

		case <-op.Done():
			// operation completed after Stop
		}
	case <-op.Done():
		// operation completed normally
	}

	return op, diags
}

// contextOpts returns the options to use to initialize a OpenTofu
// context with the settings from this Meta.
func (m *Meta) contextOpts(ctx context.Context) (*tofu.ContextOpts, error) {
	workspace, err := m.Workspace(ctx)
	if err != nil {
		return nil, err
	}

	var opts tofu.ContextOpts

	opts.UIInput = m.UIInput()
	opts.Parallelism = m.parallelism

	// If testingOverrides are set, we'll skip the plugin discovery process
	// and just work with what we've been given, thus allowing the tests
	// to provide mock providers and provisioners.
	if m.testingOverrides != nil {
		opts.Plugins = plugins.NewLibrary(
			m.testingOverrides.Providers,
			m.testingOverrides.Provisioners,
		)
	} else {
		var providerFactories map[addrs.Provider]providers.Factory
		providerFactories, err = m.providerFactories()
		opts.Plugins = plugins.NewLibrary(
			providerFactories,
			m.provisionerFactories(),
		)
	}

	opts.Meta = &tofu.ContextMeta{
		Env:                workspace,
		OriginalWorkingDir: m.WorkingDir.OriginalWorkingDir(),
	}

	return &opts, err
}

// confirm asks a yes/no confirmation.
func (m *Meta) confirm(opts *tofu.InputOpts) (bool, error) {
	if !m.Input() {
		return false, errors.New("input is disabled")
	}

	for range 2 {
		v, err := m.UIInput().Input(context.Background(), opts)
		if err != nil {
			return false, fmt.Errorf(
				"Error asking for confirmation: %w", err)
		}

		switch strings.ToLower(v) {
		case "no":
			return false, nil
		case "yes":
			return true, nil
		}
	}
	return false, nil
}

// WorkspaceNameEnvVar is the name of the environment variable that can be used
// to set the name of the OpenTofu workspace, overriding the workspace chosen
// by `tofu workspace select`.
//
// Note that this environment variable is ignored by `tofu workspace new`
// and `tofu workspace delete`.
const WorkspaceNameEnvVar = "TF_WORKSPACE"

var errInvalidWorkspaceNameEnvVar = fmt.Errorf("Invalid workspace name set using %s", WorkspaceNameEnvVar)

// Workspace returns the name of the currently configured workspace, corresponding
// to the desired named state.
func (m *Meta) Workspace(ctx context.Context) (string, error) {
	current, overridden := m.WorkspaceOverridden(ctx)
	if overridden && !validWorkspaceName(current) {
		return "", errInvalidWorkspaceNameEnvVar
	}
	return current, nil
}

// WorkspaceOverridden returns the name of the currently configured workspace,
// corresponding to the desired named state, as well as a bool saying whether
// this was set via the TF_WORKSPACE environment variable.
func (m *Meta) WorkspaceOverridden(_ context.Context) (string, bool) {
	if envVar := os.Getenv(WorkspaceNameEnvVar); envVar != "" {
		return envVar, true
	}

	envData, err := os.ReadFile(filepath.Join(m.WorkingDir.DataDir(), local.DefaultWorkspaceFile))
	current := string(bytes.TrimSpace(envData))
	if current == "" {
		current = backend.DefaultStateName
	}

	if err != nil && !os.IsNotExist(err) {
		// always return the default if we can't get a workspace name
		log.Printf("[ERROR] failed to read current workspace: %s", err)
	}

	return current, false
}

// SetWorkspace saves the given name as the current workspace in the local
// filesystem.
func (m *Meta) SetWorkspace(name string) error {
	err := os.MkdirAll(m.WorkingDir.DataDir(), 0755)
	if err != nil {
		return err
	}

	err = os.WriteFile(filepath.Join(m.WorkingDir.DataDir(), local.DefaultWorkspaceFile), []byte(name), 0644)
	if err != nil {
		return err
	}
	return nil
}

// isAutoVarFile determines if the file ends with .auto.tfvars or .auto.tfvars.json
func isAutoVarFile(path string) bool {
	return strings.HasSuffix(path, ".auto.tfvars") ||
		strings.HasSuffix(path, ".auto.tfvars.json")
}

// checkRequiredVersion loads the config and check if the
// core version requirements are satisfied.
func (m *Meta) checkRequiredVersion(ctx context.Context) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	loader, err := m.initConfigLoader()
	if err != nil {
		diags = diags.Append(err)
		return diags
	}

	pwd, err := os.Getwd()
	if err != nil {
		diags = diags.Append(fmt.Errorf("Error getting pwd: %w", err))
		return diags
	}

	call, callDiags := m.rootModuleCall(ctx, pwd)
	if callDiags.HasErrors() {
		diags = diags.Append(callDiags)
		return diags
	}

	_, configDiags := loader.LoadConfig(ctx, pwd, call)
	if configDiags.HasErrors() {
		diags = diags.Append(configDiags)
		return diags
	}

	// If there were any OpenTofu-version-related errors then they would've
	// already been detected by loader.LoadConfig above.
	return nil
}

// MaybeGetSchemas attempts to load and return the schemas
// If there is not enough information to return the schemas,
// it could potentially return nil without errors. It is the
// responsibility of the caller to handle the lack of schema
// information accordingly
func (c *Meta) MaybeGetSchemas(ctx context.Context, state *states.State, config *configs.Config) (*tofu.Schemas, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	path, err := os.Getwd()
	if err != nil {
		diags.Append(tfdiags.SimpleWarning(failedToLoadSchemasMessage))
		return nil, diags
	}

	if config == nil {
		config, diags = c.loadConfig(ctx, path)
		if diags.HasErrors() {
			diags.Append(tfdiags.SimpleWarning(failedToLoadSchemasMessage))
			return nil, diags
		}
	}

	if config != nil || state != nil {
		opts, err := c.contextOpts(ctx)
		if err != nil {
			diags = diags.Append(err)
			return nil, diags
		}
		tfCtx, ctxDiags := tofu.NewContext(opts)
		diags = diags.Append(ctxDiags)
		if ctxDiags.HasErrors() {
			return nil, diags
		}
		var schemaDiags tfdiags.Diagnostics
		schemas, schemaDiags := tfCtx.Schemas(ctx, config, state)
		diags = diags.Append(schemaDiags)
		if schemaDiags.HasErrors() {
			return nil, diags
		}
		return schemas, diags

	}
	return nil, diags
}
