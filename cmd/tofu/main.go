// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"crypto/fips140"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
	"time"

	"github.com/hashicorp/go-plugin"
	"github.com/mattn/go-shellwords"
	"github.com/mitchellh/cli"
	"github.com/mitchellh/colorstring"
	"github.com/opentofu/opentofu/internal/command"
	"github.com/opentofu/opentofu/internal/command/workdir"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/command/cliconfig"
	"github.com/opentofu/opentofu/internal/command/format"
	"github.com/opentofu/opentofu/internal/didyoumean"
	"github.com/opentofu/opentofu/internal/logging"
	"github.com/opentofu/opentofu/internal/terminal"
	"github.com/opentofu/opentofu/internal/tracing"
	"github.com/opentofu/opentofu/version"

	backendInit "github.com/opentofu/opentofu/internal/backend/init"
)

const (
	// EnvCLI is the environment variable name to set additional CLI args.
	EnvCLI = "TF_CLI_ARGS"

	// The parent process will create a file to collect crash logs
	envTmpLogPath = "TF_TEMP_LOG_PATH"

	EnvCPUProfile = "TOFU_CPU_PROFILE"
)

func init() {
	Ui = command.NewBasicUI()
}

func main() {
	os.Exit(realMain())
}

func realMain() int {
	defer logging.PanicHandler()

	// Create a go CPU profile if requested
	// This is more intense and potentially disruptive compared to OpenTelementry tracing and does not integrate with providers
	// It does however provide a deeper window (with more noise) into performance bottlenecks that are identified by a user
	// or with OpenTelemetry tracing
	if cpuProfile := os.Getenv(EnvCPUProfile); cpuProfile != "" {
		cpuProfileOut, err := os.Create(cpuProfile)
		if err != nil {
			Ui.Error(fmt.Sprintf("Could not open cpu profile output: %s", err))
			return 1
		}
		defer func() {
			err := cpuProfileOut.Close()
			if err != nil {
				Ui.Error(fmt.Sprintf("Could not close cpu profile: %s", err))
			}
		}()
		if err := pprof.StartCPUProfile(cpuProfileOut); err != nil {
			Ui.Error(fmt.Sprintf("Could not start cpu profile: %s", err))
			return 1
		}
		defer pprof.StopCPUProfile()
	}

	ctx, err := tracing.OpenTelemetryInit(context.Background())
	if err != nil {
		// openTelemetryInit can only fail if OpenTofu was run with an
		// explicit environment variable to enable telemetry collection,
		// so in typical use we cannot get here.
		Ui.Error(fmt.Sprintf("Could not initialize telemetry: %s", err))
		Ui.Error(fmt.Sprintf("Unset environment variable %s if you don't intend to collect telemetry from OpenTofu.", tracing.OTELExporterEnvVar))

		return 1
	}
	defer tracing.ForceFlush(5 * time.Second)

	// At minimum, we emit a span covering the entire command execution.
	ctx, span := tracing.Tracer().Start(ctx, "tofu")
	defer span.End()

	tmpLogPath := os.Getenv(envTmpLogPath)
	if tmpLogPath != "" {
		f, err := os.OpenFile(tmpLogPath, os.O_RDWR|os.O_APPEND, 0666)
		if err == nil {
			defer f.Close()

			log.Printf("[DEBUG] Adding temp file log sink: %s", f.Name())
			logging.RegisterSink(f)
		} else {
			log.Printf("[ERROR] Could not open temp log file: %v", err)
		}
	}

	log.Printf("[INFO] OpenTofu version: %s %s", Version, VersionPrerelease)
	if logging.IsDebugOrHigher() {
		for _, depMod := range version.InterestingDependencies() {
			log.Printf("[DEBUG] using %s %s", depMod.Path, depMod.Version)
		}
	}
	log.Printf("[INFO] Go runtime version: %s", runtime.Version())
	if dynamicGodebug := os.Getenv("GODEBUG"); dynamicGodebug != "" {
		log.Printf("[WARN] GODEBUG environment variable is set to %q, which may activate unsupported and untested behavior", dynamicGodebug)
	}
	if fips140.Enabled() {
		log.Printf("[WARN] Go runtime FIPS 140-3 mode is enabled; OpenTofu is not supported in this configuration, which may cause undesirable behavior")
	}
	log.Printf("[INFO] CLI args: %#v", os.Args)
	if experimentsAreAllowed() {
		log.Printf("[INFO] This build of OpenTofu allows using experimental features")
	}

	streams, err := terminal.Init()
	if err != nil {
		Ui.Error(fmt.Sprintf("Failed to configure the terminal: %s", err))

		return 1
	}
	if streams.Stdout.IsTerminal() {
		log.Printf("[TRACE] Stdout is a terminal of width %d", streams.Stdout.Columns())
	} else {
		log.Printf("[TRACE] Stdout is not a terminal")
	}
	if streams.Stderr.IsTerminal() {
		log.Printf("[TRACE] Stderr is a terminal of width %d", streams.Stderr.Columns())
	} else {
		log.Printf("[TRACE] Stderr is not a terminal")
	}
	if streams.Stdin.IsTerminal() {
		log.Printf("[TRACE] Stdin is a terminal")
	} else {
		log.Printf("[TRACE] Stdin is not a terminal")
	}

	// NOTE: We're intentionally calling LoadConfig _before_ handling a possible
	// -chdir=... option on the command line, so that a possible relative
	// path in the TERRAFORM_CONFIG_FILE environment variable (though probably
	// ill-advised) will be resolved relative to the true working directory,
	// not the overridden one.
	config, diags := cliconfig.LoadConfig(ctx)

	if len(diags) > 0 {
		// Since we haven't instantiated a command.Meta yet, we need to do
		// some things manually here and use some "safe" defaults for things
		// that command.Meta could otherwise figure out in smarter ways.
		Ui.Error("There are some problems with the CLI configuration:")
		for _, diag := range diags {
			earlyColor := &colorstring.Colorize{
				Colors:  colorstring.DefaultColors,
				Disable: true, // Disable color to be conservative until we know better
				Reset:   true,
			}
			// We don't currently have access to the source code cache for
			// the parser used to load the CLI config, so we can't show
			// source code snippets in early diagnostics.
			Ui.Error(format.Diagnostic(diag, nil, earlyColor, 78))
		}
		if diags.HasErrors() {
			Ui.Error("As a result of the above problems, OpenTofu may not behave as intended.\n\n")
			// We continue to run anyway, since OpenTofu has reasonable defaults.
		}
	}

	// Get any configured credentials from the config and initialize
	// a service discovery object.
	credsSrc, err := credentialsSource(config)
	if err != nil {
		// Most commands don't actually need credentials, and most situations
		// that would get us here would already have been reported by the config
		// loading above, so we'll just log this one as an aid to debugging
		// in the unlikely event that it _does_ arise.
		log.Printf("[WARN] Cannot initialize remote host credentials manager: %s", err)
		credsSrc = nil // must be an untyped nil for newServiceDiscovery to understand "no credentials available"
	}
	services := newServiceDiscovery(ctx, config.RegistryProtocols, credsSrc)

	modulePkgFetcher := remoteModulePackageFetcher(ctx, config.OCICredentialsPolicy)

	providerDevOverrides := providerDevOverrides(config.ProviderInstallation)

	// The user can declare that certain providers are being managed on
	// OpenTofu's behalf using this environment variable. This is used
	// primarily by the SDK's acceptance testing framework.
	unmanagedProviders, err := parseReattachProviders(os.Getenv("TF_REATTACH_PROVIDERS"))
	if err != nil {
		Ui.Error(err.Error())
		return 1
	}

	// Initialize the backends.
	backendInit.Init(services)

	// Get the command line args.
	binName := filepath.Base(os.Args[0])
	args := os.Args[1:]

	// Create the workdir and apply the -chdir options.
	// The logic inside [NewWorkdir] handles the TF_DATA_DIR env var too.
	wd, newArgs, err := workdir.NewWorkdir(args)
	if err != nil {
		Ui.Error(err.Error())
		return 1
	}
	// TODO meta-refactor - this is temporary because chdir logic strips away the -chdir flag from the args.
	// Once we move to a different CLI lib, this will be handled by that, where flags defined on a parent
	// command will be excluded from the args given to child commands.
	args = newArgs

	providerSrc, diags := providerSource(ctx,
		config.ProviderInstallation,
		config.RegistryProtocols,
		services,
		config.OCICredentialsPolicy,
		wd.RootModuleDir(), // this has to be the directory that tofu has been executed from, not the one after -chdir
	)
	if len(diags) > 0 {
		Ui.Error("There are some problems with the provider_installation configuration:")
		for _, diag := range diags {
			earlyColor := &colorstring.Colorize{
				Colors:  colorstring.DefaultColors,
				Disable: true, // Disable color to be conservative until we know better
				Reset:   true,
			}
			Ui.Error(format.Diagnostic(diag, nil, earlyColor, 78))
		}
		if diags.HasErrors() {
			Ui.Error("As a result of the above problems, OpenTofu's provider installer may not behave as intended.\n\n")
			// We continue to run anyway, because most commands don't do provider installation.
		}
	}

	// In tests, Commands may already be set to provide mock commands
	if commands == nil {
		// Commands get to hold on to the original working directory here,
		// in case they need to refer back to it for any special reason, though
		// they should primarily be working with the override working directory
		// that we've now switched to above.
		initCommands(ctx, wd, streams, config, services, modulePkgFetcher, providerSrc, providerDevOverrides, unmanagedProviders)
	}

	// Attempt to ensure the config directory exists.
	configDir, err := cliconfig.ConfigDir()
	if err != nil {
		log.Printf("[ERROR] Failed to find the path to the config directory: %v", err)
	} else if err := mkConfigDir(configDir); err != nil {
		log.Printf("[ERROR] Failed to create the config directory at path %s: %v", configDir, err)
	}

	// Make sure we clean up any managed plugins at the end of this
	defer plugin.CleanupClients()

	// Build the CLI so far, we do this so we can query the subcommand.
	cliRunner := &cli.CLI{
		Args:       args,
		Commands:   commands,
		HelpFunc:   helpFunc,
		HelpWriter: os.Stdout,
	}

	// Prefix the args with any args from the EnvCLI
	args, err = mergeEnvArgs(EnvCLI, cliRunner.Subcommand(), args)
	if err != nil {
		Ui.Error(err.Error())
		return 1
	}

	// Prefix the args with any args from the EnvCLI targeting this command
	suffix := strings.ReplaceAll(strings.ReplaceAll(
		cliRunner.Subcommand(), "-", "_"), " ", "_")
	args, err = mergeEnvArgs(
		fmt.Sprintf("%s_%s", EnvCLI, suffix), cliRunner.Subcommand(), args)
	if err != nil {
		Ui.Error(err.Error())
		return 1
	}

	// We shortcut "--version" and "-v" to just show the version
	for _, arg := range args {
		if arg == "-v" || arg == "-version" || arg == "--version" {
			newArgs := make([]string, len(args)+1)
			newArgs[0] = "version"
			copy(newArgs[1:], args)
			args = newArgs
			break
		}
	}

	// Rebuild the CLI with any modified args.
	log.Printf("[INFO] CLI command args: %#v", args)
	cliRunner = &cli.CLI{
		Name:           binName,
		Args:           args,
		Commands:       commands,
		HiddenCommands: getAliasCommandKeys(),
		HelpFunc:       helpFunc,
		HelpWriter:     os.Stdout,

		Autocomplete:          true,
		AutocompleteInstall:   "install-autocomplete",
		AutocompleteUninstall: "uninstall-autocomplete",
	}

	// Before we continue we'll check whether the requested command is
	// actually known. If not, we might be able to suggest an alternative
	// if it seems like the user made a typo.
	// (This bypasses the built-in help handling in cli.CLI for the situation
	// where a command isn't found, because it's likely more helpful to
	// mention what specifically went wrong, rather than just printing out
	// a big block of usage information.)

	// Check if this is being run via shell auto-complete, which uses the
	// binary name as the first argument and won't be listed as a subcommand.
	autoComplete := os.Getenv("COMP_LINE") != ""

	if cmd := cliRunner.Subcommand(); cmd != "" && !autoComplete {
		// Due to the design of cli.CLI, this special error message only works
		// for typos of top-level commands. For a subcommand typo, like
		// "tofu state push", cmd would be "state" here and thus would
		// be considered to exist, and it would print out its own usage message.
		if _, exists := commands[cmd]; !exists {
			suggestions := make([]string, 0, len(commands))
			for name := range commands {
				suggestions = append(suggestions, name)
			}
			suggestion := didyoumean.NameSuggestion(cmd, suggestions)
			if suggestion != "" {
				suggestion = fmt.Sprintf(" Did you mean %q?", suggestion)
			}
			fmt.Fprintf(os.Stderr, "OpenTofu has no command named %q.%s\n\nTo see all of OpenTofu's top-level commands, run:\n  tofu -help\n\n", cmd, suggestion)
			return 1
		}
	}

	exitCode, err := cliRunner.Run()
	if err != nil {
		Ui.Error(fmt.Sprintf("Error executing CLI: %s", err.Error()))
		return 1
	}

	// We might generate some additional log lines if OpenTofu relied on any
	// non-default Go runtime behaviors enabled by GODEBUG settings, because
	// they might be relevant when trying to reproduce certain problems for
	// debugging or bug reporting purposes.
	logGodebugUsage()

	// if we are exiting with a non-zero code, check if it was caused by any
	// plugins crashing
	if exitCode != 0 {
		for _, panicLog := range logging.PluginPanics() {
			Ui.Error(panicLog)
		}
	}

	return exitCode
}

func mergeEnvArgs(envName string, cmd string, args []string) ([]string, error) {
	v := os.Getenv(envName)
	if v == "" {
		return args, nil
	}

	log.Printf("[INFO] %s value: %q", envName, v)
	extra, err := shellwords.Parse(v)
	if err != nil {
		return nil, fmt.Errorf(
			"Error parsing extra CLI args from %s: %s",
			envName, err)
	}

	// Find the command to look for in the args. If there is a space,
	// we need to find the last part.
	search := cmd
	if idx := strings.LastIndex(search, " "); idx >= 0 {
		search = cmd[idx+1:]
	}

	// Find the index to place the flags. We put them exactly
	// after the first non-flag arg.
	idx := -1
	for i, v := range args {
		if v == search {
			idx = i
			break
		}
	}

	// idx points to the exact arg that isn't a flag. We increment
	// by one so that all the copying below expects idx to be the
	// insertion point.
	idx++

	// Copy the args
	newArgs := make([]string, len(args)+len(extra))
	copy(newArgs, args[:idx])
	copy(newArgs[idx:], extra)
	copy(newArgs[len(extra)+idx:], args[idx:])
	return newArgs, nil
}

// parse information on reattaching to unmanaged providers out of a
// JSON-encoded environment variable.
func parseReattachProviders(in string) (map[addrs.Provider]*plugin.ReattachConfig, error) {
	unmanagedProviders := map[addrs.Provider]*plugin.ReattachConfig{}
	if in != "" {
		type reattachConfig struct {
			Protocol        string
			ProtocolVersion int
			Addr            struct {
				Network string
				String  string
			}
			Pid  int
			Test bool
		}
		var m map[string]reattachConfig
		err := json.Unmarshal([]byte(in), &m)
		if err != nil {
			return unmanagedProviders, fmt.Errorf("Invalid format for TF_REATTACH_PROVIDERS: %w", err)
		}
		for p, c := range m {
			a, diags := addrs.ParseProviderSourceString(p)
			if diags.HasErrors() {
				return unmanagedProviders, fmt.Errorf("Error parsing %q as a provider address: %w", a, diags.Err())
			}
			var addr net.Addr
			switch c.Addr.Network {
			case "unix":
				addr, err = net.ResolveUnixAddr("unix", c.Addr.String)
				if err != nil {
					return unmanagedProviders, fmt.Errorf("Invalid unix socket path %q for %q: %w", c.Addr.String, p, err)
				}
			case "tcp":
				addr, err = net.ResolveTCPAddr("tcp", c.Addr.String)
				if err != nil {
					return unmanagedProviders, fmt.Errorf("Invalid TCP address %q for %q: %w", c.Addr.String, p, err)
				}
			default:
				return unmanagedProviders, fmt.Errorf("Unknown address type %q for %q", c.Addr.Network, p)
			}
			unmanagedProviders[a] = &plugin.ReattachConfig{
				Protocol:        plugin.Protocol(c.Protocol),
				ProtocolVersion: c.ProtocolVersion,
				Pid:             c.Pid,
				Test:            c.Test,
				Addr:            addr,
			}
		}
	}
	return unmanagedProviders, nil
}

// Creates the configuration directory.
// `configDir` should refer to `~/.terraform.d`, `$XDG_CONFIG_HOME/opentofu` or its equivalent
// on non-UNIX platforms.
func mkConfigDir(configDir string) error {
	err := os.Mkdir(configDir, os.ModePerm)

	if err == nil {
		log.Printf("[DEBUG] Created the config directory: %s", configDir)
		return nil
	}

	if os.IsExist(err) {
		log.Printf("[DEBUG] Found the config directory: %s", configDir)
		return nil
	}

	return err
}
