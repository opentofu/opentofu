// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"sort"

	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type Version interface {
	Diagnostics(diags tfdiags.Diagnostics)
	// PrintVersion returns true if the printing has been done successfully and false otherwise.
	PrintVersion(version string, versionPrerelease string, platform string, fipsEnabled bool, providerVersions map[string]string) bool
}

// NewVersion returns an initialized Version implementation for the given ViewType.
// This view behaves differently from the general approach since the JSON format is not meant to follow
// the general JSON format.
// Instead, the view that is returned will always print diagnostics in human format while
// [Version.PrintVersion] will return different results based on the [arguments.ViewOptions#ViewType].
func NewVersion(args arguments.ViewOptions, view *View) Version {
	return &VersionHuman{view: view, json: args.ViewType == arguments.ViewJSON}
}

type VersionHuman struct {
	view *View
	// In the case of this command, we don't use the [JSONView], but we only marshal the result and print it directly
	json bool
}

var _ Version = (*VersionHuman)(nil)

func (v *VersionHuman) Diagnostics(diags tfdiags.Diagnostics) {
	v.view.Diagnostics(diags)
}

func (v *VersionHuman) PrintVersion(version string, versionPrerelease string, platform string, fipsEnabled bool, providerVersions map[string]string) bool {
	if v.json {
		return v.printJsonVersion(version, versionPrerelease, platform, fipsEnabled, providerVersions)
	}
	return v.printHumanVersion(version, versionPrerelease, platform, fipsEnabled, providerVersions)
}

func (v *VersionHuman) printJsonVersion(version string, versionPrerelease string, platform string, fipsEnabled bool, providerVersions map[string]string) bool {
	finalVersion := version
	if versionPrerelease != "" {
		finalVersion = fmt.Sprintf("%s-%s", finalVersion, versionPrerelease)
	}

	output := versionOutput{
		Version:            finalVersion,
		Platform:           platform,
		ProviderSelections: providerVersions,
		FIPS140Enabled:     fipsEnabled,
	}
	jsonOutput, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		_, _ = v.view.streams.Eprintln(fmt.Sprintf("\nError marshalling JSON: %s", err))
		return false
	}
	_, _ = v.view.streams.Println(string(jsonOutput))
	return true
}

func (v *VersionHuman) printHumanVersion(version string, versionPrerelease string, platform string, fipsEnabled bool, providerVersions map[string]string) bool {
	formattedVersion := fmt.Sprintf("OpenTofu v%s", version)
	if versionPrerelease != "" {
		formattedVersion = fmt.Sprintf("%s-%s", formattedVersion, versionPrerelease)
	}
	_, _ = v.view.streams.Println(formattedVersion)
	_, _ = v.view.streams.Println(fmt.Sprintf("on %s", platform))
	if fipsEnabled {
		_, _ = v.view.streams.Println("running in FIPS 140-3 mode (not yet supported)")
	}

	providerAddrs := slices.Collect(maps.Keys(providerVersions))
	sort.Strings(providerAddrs)
	for _, provAddr := range providerAddrs {
		provVers, ok := providerVersions[provAddr]
		if !ok {
			continue
		}
		if provVers == "0.0.0" {
			_, _ = v.view.streams.Println(fmt.Sprintf("+ provider %s (unversioned)", provAddr))
		} else {
			_, _ = v.view.streams.Println(fmt.Sprintf("+ provider %s v%s", provAddr, provVers))
		}
	}
	return true
}

type versionOutput struct {
	Version            string            `json:"terraform_version"`
	Platform           string            `json:"platform"`
	FIPS140Enabled     bool              `json:"fips140,omitempty"`
	ProviderSelections map[string]string `json:"provider_selections"`
}
