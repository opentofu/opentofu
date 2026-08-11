// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/go-plugin"
	"github.com/mitchellh/cli"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/command/cliconfig"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/command/workdir"
	"github.com/opentofu/opentofu/internal/didyoumean"
	"github.com/opentofu/opentofu/internal/getmodules"
	"github.com/opentofu/opentofu/internal/getproviders"
	"github.com/opentofu/svchost/disco"
)

func legacyCommandMain(
	ctx context.Context,
	view *views.View,
	rv *views.Root,
	config *cliconfig.Config,
	services *disco.Disco,
	modulePkgFetcher *getmodules.PackageFetcher,
	providerDevOverrides map[addrs.Provider]getproviders.PackageLocalDir,
	unmanagedProviders map[addrs.Provider]*plugin.ReattachConfig,
) int {
	// Get the command line args.
	binName := filepath.Base(os.Args[0])
	args := os.Args[1:]

	// Create the workdir and apply the -chdir options.
	// The logic inside [NewWorkdir] handles the TF_DATA_DIR env var too.
	wd, newArgs, err := workdir.NewWorkdir(args)
	if err != nil {
		rv.Error(err.Error())
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
		rv.Error("There are some problems with the provider_installation configuration:")
		rv.Diagnostics(diags)
		if diags.HasErrors() {
			rv.Error("As a result of the above problems, OpenTofu's provider installer may not behave as intended.\n\n")
			// We continue to run anyway, because most commands don't do provider installation.
		}
	}

	// Attempt to ensure the config directory exists.
	configDir, err := cliconfig.ConfigDir()
	if err != nil {
		log.Printf("[ERROR] Failed to find the path to the config directory: %v", err)
	} else if err := mkConfigDir(configDir); err != nil {
		log.Printf("[ERROR] Failed to create the config directory at path %s: %v", configDir, err)
	}

	// In tests, Commands may already be set to provide mock commands
	if commands == nil {
		// Commands get to hold on to the original working directory here,
		// in case they need to refer back to it for any special reason, though
		// they should primarily be working with the override working directory
		// that we've now switched to above.
		meta := makeMeta(ctx, wd, view, config, services, modulePkgFetcher, providerSrc, providerDevOverrides, unmanagedProviders)
		initCommands(meta)
	}

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
		rv.Error(err.Error())
		return 1
	}

	// Prefix the args with any args from the EnvCLI targeting this command
	suffix := strings.ReplaceAll(strings.ReplaceAll(
		cliRunner.Subcommand(), "-", "_"), " ", "_")
	args, err = mergeEnvArgs(
		fmt.Sprintf("%s_%s", EnvCLI, suffix), cliRunner.Subcommand(), args)
	if err != nil {
		rv.Error(err.Error())
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
		if cmd == "completion" {
			// Conflict between legacy and new CLI, ignore
			return 0
		}
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
			rv.Error(fmt.Sprintf("OpenTofu has no command named %q.%s\n\nTo see all of OpenTofu's top-level commands, run:\n  tofu -help\n", cmd, suggestion))
			return 1
		}
	}

	exitCode, err := cliRunner.Run()
	if err != nil {
		rv.Error(fmt.Sprintf("Error executing CLI: %s", err.Error()))
		return 1
	}
	return exitCode

}
