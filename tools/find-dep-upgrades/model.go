// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"fmt"
	"strings"

	"github.com/apparentlymart/go-versions/versions"
)

// ModulePath is a string containing a Go module path.
type ModulePath string

// ModulePaths represents an unordered set of [ModulePath] values.
type ModulePaths map[ModulePath]struct{}

// Version is a convenience alias for [versions.Version]
type Version = versions.Version

type UpgradeCandidate struct {
	Module         ModulePath
	CurrentVersion Version
	LatestVersion  Version
}

type PendingUpgrade struct {
	Module         ModulePath
	CurrentVersion Version
	LatestVersion  Version
	Prereqs        map[ModulePath]Version
}

// PendingUpgradeCluster is one or more [PendingUpgrade] which need to happen
// as a single unit.
//
// Clusters with more than one upgrade result from dependency cycles between
// the modules, making it impossible to upgrade one without also upgrading
// the others.
type PendingUpgradeCluster []PendingUpgrade

func parseVersion(raw string) (Version, error) {
	if !strings.HasPrefix(raw, "v") {
		return versions.Unspecified, fmt.Errorf("missing 'v' prefix")
	}
	raw = raw[1:] // the "versions" library doesn't actually want the prefix
	return versions.ParseVersion(raw)
}
