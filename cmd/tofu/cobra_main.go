// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"fmt"
	"log"

	"github.com/hashicorp/go-plugin"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/command"
	"github.com/opentofu/opentofu/internal/command/cliconfig"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/command/workdir"
	"github.com/opentofu/opentofu/internal/getmodules"
	"github.com/opentofu/opentofu/internal/getproviders"
	"github.com/opentofu/svchost/disco"
	"github.com/spf13/cobra"
)

func cobraMain(
	ctx context.Context,
	view *views.View,
	rv *views.Root,
	config *cliconfig.Config,
	services *disco.Disco,
	modulePkgFetcher *getmodules.PackageFetcher,
	providerDevOverrides map[addrs.Provider]getproviders.PackageLocalDir,
	unmanagedProviders map[addrs.Provider]*plugin.ReattachConfig,
) int {
	root := &cobra.Command{
		Use:  "tofu",
		Long: `The available commands for execution are listed below. The primary workflow commands are given first, followed by less common or more advanced commands.`,

		// TraverseChildren is needed to ensure that the flags from the parent command are not passed
		// as arguments in any of its subcommands.
		// Instead, by having this enabled, the flags are parsed for any parent command into their
		// dedicated pointers.
		TraverseChildren: true,
	}

	root.AddGroup(command.MainCommandGroup, command.OtherCommandGroup)

	var chdirArg string
	root.Flags().StringVar(&chdirArg, "chdir", "", "Switch to a different working directory before executing the given subcommand.")
	root.Flags().Bool("help", false, "Show this help output, or the help for a specified subcommand.")

	// In practice, this is only ever called once, though we could wrap it in a sync.Once if we want to be safer.
	meta := func() (command.Meta, error) {
		// Create the workdir and apply the -chdir options.
		// The logic inside [NewWorkdir] handles the TF_DATA_DIR env var too.
		wd, err := workdir.NewWorkdirCobra(chdirArg)
		if err != nil {
			rv.Error(err.Error())
			return command.Meta{}, command.ExitCodeError(1)
		}

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

		return makeMeta(ctx, wd, view, config, services, modulePkgFetcher, providerSrc, providerDevOverrides, unmanagedProviders), nil
	}

	root.AddCommand(command.ConsoleCommander(meta))
	root.AddCommand(command.PlanCommander(meta))

	root.SetHelpTemplate(`{{.UsageString}}`) // TODO return -1
	root.SetUsageFunc(func(cmd *cobra.Command) error {
		w := cmd.OutOrStdout()
		if cmd == root {
			return command.CommandUsage(cmd, command.UsageOptions{
				Usage: "tofu [global options] <subcommand> [args]",
				FlagGroups: []command.FlagGroup{{
					Title: "Global options (use these before the subcommand, if any)",
				}},
				FlagSingleSpace: true,
				FlagOptions: map[string]command.FlagOption{"chdir": {
					Display: "DIR",
				}},
			}, w)
		}
		// Default
		return command.CommandUsage(cmd, command.UsageOptions{}, w)
	})

	err := root.Execute()
	if err != nil {
		if exitCode, ok := err.(command.ExitCodeError); ok {
			return int(exitCode)
		}
		rv.Error(fmt.Sprintf("Error executing CLI: %s", err.Error()))
		return 1
	}
	return 0
}
