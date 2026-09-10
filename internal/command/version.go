// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"cmp"
	"crypto/fips140"
	"maps"
	"slices"
	"strings"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/getproviders"
	"github.com/opentofu/opentofu/internal/modsdir"
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

	var moduleVersions map[string]string
	if mani, err := modsdir.ReadManifestSnapshotForDir(c.WorkingDir.ModulesDir()); err == nil {
		vals := slices.Collect(maps.Values(mani))
		moduleVersions = readModuleVersions(vals)
	}

	if !view.PrintVersion(c.Version, c.VersionPrerelease, c.Platform.String(), fips140.Enabled(), providerVersions, moduleVersions) {
		return 1
	}
	return 0
}

func readModuleVersions(records []modsdir.Record) map[string]string {
	moduleVersions := map[string]string{}
	slices.SortFunc(records, func(a, b modsdir.Record) int {
		return cmp.Compare(strings.Count(a.Key, "."), strings.Count(b.Key, "."))
	})
	resolved := map[string]addrs.ModuleSource{}
	for _, m := range records {
		rawSrc, err := addrs.ParseModuleSource(m.SourceAddr)
		if err != nil {
			continue
		}
		if m.Key == "" {
			resolved[m.Key] = rawSrc
			continue
		}
		parentKey := parentModuleKey(m.Key)
		parentMS, hasParent := resolved[parentKey]

		var outSrc addrs.ModuleSource
		if hasParent && parentMS.String() != "" {
			outSrc, _ = addrs.ResolveRelativeModuleSource(parentMS, rawSrc)
		} else {
			outSrc = rawSrc
		}

		resolved[m.Key] = outSrc

		if m.Version != nil {
			moduleVersions[outSrc.String()] = m.Version.String()
		} else {
			moduleVersions[outSrc.String()] = "0.0.0"
		}
	}
	return moduleVersions
}

func parentModuleKey(key string) string {
	if i := strings.LastIndex(key, "."); i >= 0 {
		return key[:i]
	}
	return ""
}
