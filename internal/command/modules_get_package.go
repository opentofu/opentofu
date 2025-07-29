// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"fmt"

	version "github.com/hashicorp/go-version"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/initwd"
)

// MetadataFunctionsCommand is a Command implementation that prints out information
// about the available functions in OpenTofu.
type ModulesGetPackageCommand struct {
	Meta
}

// Run implements cli.Command.
func (c *ModulesGetPackageCommand) Run(args []string) int {
	ctx := c.CommandContext()

	// If we were to build something like this "for real", rather than just
	// prototyping, we should probably use the arguments/views approach for
	// consistency with other modern commands, and consider having options
	// for choosing between "-raw" and "-json" modes where the latter could
	// potentially return more detailed information, describe gradual progress,
	// and return machine-readable error messages. It should also use proper
	// diagnostics rather than the legacy c.Ui.Error API.
	//
	// For now this is just simple to show what it might look like to _just_
	// install a single module package, without doing all of the other stuff
	// that "tofu init" or "tofu get" would normally do.

	var versionRaw string
	args = c.Meta.process(args)
	cmdFlags := c.Meta.extendedFlagSet("modules get-package")
	cmdFlags.StringVar(&versionRaw, "version", "", "Version constraint for module registry address")
	cmdFlags.Usage = func() { c.Ui.Error(c.Help()) }
	if err := cmdFlags.Parse(args); err != nil {
		c.Ui.Error(fmt.Sprintf("Error parsing command-line options: %s\n", err.Error()))
		return 1
	}
	args = cmdFlags.Args()
	if len(args) != 2 {
		c.Ui.Error(c.Help())
		return 1
	}

	sourceAddrRaw := args[0]
	targetDir := args[1]

	sourceAddr, err := addrs.ParseModuleSource(sourceAddrRaw)
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Invalid module source address: %s\n", err.Error()))
		return 1
	}
	if _, isLocal := sourceAddr.(addrs.ModuleSourceLocal); isLocal {
		c.Ui.Error(fmt.Sprintf("Invalid module source address: must be a remote or module registry source address\n"))
		return 1
	}

	// This API was originally designed for installing full dependency trees
	// rather than individual packages, and the inner functions we use for
	// fetching packages are not exported any other way, so for now we're
	// passing nil for the config loader and assuming that the methods
	// we're using on this type will not use the config loader.
	// FIXME: Rework the module installer design a little so that the
	// individual-package-level operations are exposed separately from the
	// recursive dependency chasing.
	inst := initwd.NewModuleInstaller(c.modulesDir(), nil, c.registryClient(ctx), c.ModulePackageFetcher)

	// If we were given a registry address then we need to first translate it
	// into its underlying remote source address.
	if registrySourceAddr, isRegistry := sourceAddr.(addrs.ModuleSourceRegistry); isRegistry {
		versionConstraints, err := version.NewConstraint(versionRaw)
		if err != nil {
			c.Ui.Error(fmt.Sprintf("Invalid version constraint: %s\n", err.Error()))
			return 1
		}
		remoteSourceAddr, _, diags := inst.ResolveRegistryModule(ctx, registrySourceAddr, versionConstraints)
		if diags.HasErrors() {
			c.showDiagnostics(diags)
			return 1
		}
		sourceAddr = remoteSourceAddr
	} else {
		if versionRaw != "" {
			c.Ui.Error(fmt.Sprintf("The -version option is valid only for module registry source addresses\n"))
			return 1
		}
	}

	// By the time we get here sourceAddr should definitely be a remote source
	// address, but we'll check to make sure.
	remoteSourceAddr, ok := sourceAddr.(addrs.ModuleSourceRemote)
	if !ok {
		c.Ui.Error(fmt.Sprintf("Invalid module source address: unsupported address type\n"))
		return 1
	}
}

// Help implements cli.Command.
func (m *ModulesGetPackageCommand) Help() string {
	return `
Usage: tofu [global options] modules get-package -version=<version-constraint> <source-addr> <target-dir>

  Fetches a single module package from the given source address and extracts
  it into the given target directory.

  If successful, returns with exit code zero after printing the local filepath
  of the selected module to stdout. The returned filepath will not exactly match
  the given target directory if the source address includes a "//subdir"
  portion to select a module in a subdirectory of the package.

  This is a plumbing command intended only for use by external tools that
  need to use files from OpenTofu module packages. 
`
}

// Synopsis implements cli.Command.
func (m *ModulesGetPackageCommand) Synopsis() string {
	return "Plumbing command for fetching a single module package"
}
