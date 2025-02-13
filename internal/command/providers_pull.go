// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"context"
	"errors"
	"fmt"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/depsfile"
	"github.com/opentofu/opentofu/internal/getproviders"
	"github.com/opentofu/opentofu/internal/providercache"
	"os"
)

// ProvidersPullCommand pulls a specified provider into a specified directory for the current platform. This is mainly
// a debugging tool to verify that the provider can be pulled correctly, but can also be used in workflows that require
// the provider to be present (e.g. to generate an SBOM file).
type ProvidersPullCommand struct {
	Meta
}

func (c *ProvidersPullCommand) Help() string {
	return providersPullCommandHelp
}

func (c *ProvidersPullCommand) Synopsis() string {
	return "Pulls a provider binary into a directory"
}

func (c *ProvidersPullCommand) Run(args []string) int {
	ctx := c.CommandContext()

	args = c.Meta.process(args)
	cmdFlags := c.Meta.defaultFlagSet("providers pull")
	c.Meta.varFlagSet(cmdFlags)
	var addr string
	var targetDir string
	cmdFlags.StringVar(&addr, "addr", "", "provider address in the hostname/namespace/type format")
	cmdFlags.StringVar(&targetDir, "target-dir", "", "target directory to unpack the provider into")

	cmdFlags.Usage = func() { c.Ui.Error(c.Help()) }
	if err := cmdFlags.Parse(args); err != nil {
		c.Ui.Error(fmt.Sprintf("Error parsing command-line flags: %s\n", err.Error()))
		return 1
	}

	if addr == "" {
		c.Ui.Error("The -addr option is required.")
		return 1
	}
	if targetDir == "" {
		c.Ui.Error("The -target-dir option is required.")
		return 1
	}

	if err := os.MkdirAll(targetDir, 0755); err != nil { //nolint:mnd //Nope, we are not adding a constant for this.
		c.Ui.Error(fmt.Sprintf("Cannot create target directory %s (%s)\n", targetDir, err.Error()))
		return 1
	}

	address, diags := addrs.ParseProviderSourceString(addr)
	if diags.HasErrors() {
		c.Ui.Error(fmt.Sprintf("Invalid provider address: %s (%s)\n", addr, diags.Err().Error()))
		return 1
	}

	inst := providercache.NewInstaller(providercache.NewDir(targetDir), c.providerInstallSource())

	_, err := inst.EnsureProviderVersions(ctx, depsfile.NewLocks(), getproviders.Requirements{
		address: nil,
	}, providercache.InstallNewProvidersOnly)
	if errors.Is(ctx.Err(), context.Canceled) {
		c.Ui.Error("Provider installation was canceled by an interrupt signal.")
		return 1
	}
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Provider installation failed: %s", err.Error()))
		return 1
	}

	c.Ui.Info(fmt.Sprintf("Provider binary for %s pulled into %s.", addr, targetDir))

	return 0
}

const providersPullCommandHelp = `
Usage: tofu [global options] providers pull [options]

  Pulls the specified providers for the current platform into the target
  directory. This command can be used without a project present.

Options:

  -addr address          Provider address in the hostname/namespace/type
                         format. (Required)

  -target-dir targetDir  Directory to unpack the provider into. OpenTofu must
                         have read permission on all files in this directory.
                         Do not use /tmp for this purpose. (Required)
`
