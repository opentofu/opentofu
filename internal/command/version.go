// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"crypto/fips140"
	"strings"

	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/getproviders"
)

func VersionCommander(version string, versionPrerelease string, platform getproviders.Platform) Command {
	cmd := Command{
		Name:  "version",
		Short: "Show the current OpenTofu version",
		Long:  `Displays the version of OpenTofu and all installed plugins`,

		DiagsWithNewline: true,
	}

	args := arguments.BindVersion(&cmd.CommandLine)
	cmd.Run = func(meta Meta) int {
		return VersionCommand{
			Meta:              meta,
			Version:           version,
			VersionPrerelease: versionPrerelease,
			Platform:          platform,
		}.Execute(views.NewVersion(args.View, meta.View))
	}

	return cmd
}

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
	return RunCommand(VersionCommander(c.Version, c.VersionPrerelease, c.Platform), c.Meta, rawArgs)
}
func (c VersionCommand) Execute(view views.Version) int {
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
