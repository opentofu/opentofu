// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"crypto/fips140"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/getproviders"
)

// VersionCommand is a Command implementation prints the version.
type VersionCommand struct {
	Meta

	Version           string
	VersionPrerelease string
	Platform          getproviders.Platform
}

func (c *VersionCommand) Help() string {
	helpText := `
Usage: tofu [global options] version [options]

  Displays the version of OpenTofu and all installed plugins

Options:

  -json       Output the version information as a JSON object.
`
	return strings.TrimSpace(helpText)
}

func (c *VersionCommand) Run(rawArgs []string) int {
	// new view
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
	args, closer, diags := arguments.ParseVersion(rawArgs)
	defer closer()

	// Instantiate the view, even if there are flag errors, so that we render
	// diagnostics according to the desired view
	view := views.NewVersion(args.ViewOptions, c.View)

	if diags.HasErrors() {
		view.Diagnostics(diags)
		return cli.RunResultHelp
	}

	// We'll also attempt to print out the selected plugin versions. We do
	// this based on the dependency lock file, and so the result might be
	// empty or incomplete if the user hasn't successfully run "tofu init"
	// since the most recent change to dependencies.
	//
	// Generally-speaking this is a best-effort thing that will give us a good
	// result in the usual case where the user successfully ran "tofu init"
	// and then hit a problem running _another_ command.
	providerVersions := map[string]string{}
	if locks, err := c.lockedDependencies(); err == nil {
		for providerAddr, lock := range locks.AllProviders() {
			providerVersions[providerAddr.String()] = lock.Version().String()
		}
	}
	if !view.PrintVersion(c.Version, c.VersionPrerelease, c.Platform.String(), fips140.Enabled(), providerVersions) {
		return 1
	}
	return 0
}

func (c *VersionCommand) Synopsis() string {
	return "Show the current OpenTofu version"
}
