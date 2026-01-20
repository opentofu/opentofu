// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"bytes"
	"crypto/fips140"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/depsfile"
	"github.com/opentofu/opentofu/internal/getproviders"
)

// VersionCommand is a Command implementation prints the version.
type VersionCommand struct {
	Meta

	Version           string
	VersionPrerelease string
	Platform          getproviders.Platform
}

type VersionOutput struct {
	Version            string            `json:"terraform_version"`
	Platform           string            `json:"platform"`
	FIPS140Enabled     bool              `json:"fips140,omitempty"`
	ProviderSelections map[string]string `json:"provider_selections"`
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

func (c *VersionCommand) Run(args []string) int {
	var versionString bytes.Buffer
	args = c.Meta.process(args)
	var jsonOutput bool
	cmdFlags := c.Meta.defaultFlagSet("version")
	cmdFlags.BoolVar(&jsonOutput, "json", false, "json")
	// Enable but ignore the global version flags. In main.go, if any of the
	// arguments are -v, -version, or --version, this command will be called
	// with the rest of the arguments, so we need to be able to cope with
	// those.
	cmdFlags.Bool("v", true, "version")
	cmdFlags.Bool("version", true, "version")
	cmdFlags.Usage = func() { c.Ui.Error(c.Help()) }
	if err := cmdFlags.Parse(args); err != nil {
		c.Ui.Error(fmt.Sprintf("Error parsing command-line flags: %s\n", err.Error()))
		return 1
	}

	fmt.Fprintf(&versionString, "OpenTofu v%s", c.Version)
	if c.VersionPrerelease != "" {
		fmt.Fprintf(&versionString, "-%s", c.VersionPrerelease)
	}

	// We'll also attempt to print out the selected plugin versions. We do
	// this based on the dependency lock file, and so the result might be
	// empty or incomplete if the user hasn't successfully run "tofu init"
	// since the most recent change to dependencies.
	//
	// Generally-speaking this is a best-effort thing that will give us a good
	// result in the usual case where the user successfully ran "tofu init"
	// and then hit a problem running _another_ command.
	var providerVersions []string
	var providerLocks map[addrs.Provider]*depsfile.ProviderLock
	if locks, err := c.lockedDependencies(); err == nil {
		providerLocks = locks.AllProviders()
		for providerAddr, lock := range providerLocks {
			version := lock.Version().String()
			if version == "0.0.0" {
				providerVersions = append(providerVersions, fmt.Sprintf("+ provider %s (unversioned)", providerAddr))
			} else {
				providerVersions = append(providerVersions, fmt.Sprintf("+ provider %s v%s", providerAddr, version))
			}
		}
	}

	if jsonOutput {
		selectionsOutput := make(map[string]string)
		for providerAddr, lock := range providerLocks {
			version := lock.Version().String()
			selectionsOutput[providerAddr.String()] = version
		}

		var versionOutput string
		if c.VersionPrerelease != "" {
			versionOutput = c.Version + "-" + c.VersionPrerelease
		} else {
			versionOutput = c.Version
		}

		output := VersionOutput{
			Version:            versionOutput,
			Platform:           c.Platform.String(),
			ProviderSelections: selectionsOutput,
			FIPS140Enabled:     fips140.Enabled(),
		}

		jsonOutput, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			c.Ui.Error(fmt.Sprintf("\nError marshalling JSON: %s", err))
			return 1
		}
		c.Ui.Output(string(jsonOutput))
		return 0
	} else {
		c.Ui.Output(versionString.String())
		c.Ui.Output(fmt.Sprintf("on %s", c.Platform))
		if fips140.Enabled() {
			c.Ui.Output("running in FIPS 140-3 mode (not yet supported)")
		}

		if len(providerVersions) != 0 {
			sort.Strings(providerVersions)
			for _, str := range providerVersions {
				c.Ui.Output(str)
			}
		}
	}

	return 0
}

func (c *VersionCommand) Synopsis() string {
	return "Show the current OpenTofu version"
}
