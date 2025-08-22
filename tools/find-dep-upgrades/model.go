package main

import (
	"fmt"
	"strings"

	"github.com/apparentlymart/go-versions/versions"
)

// ModulePath is a string containing a Go module path.
type ModulePath string

// Version is a convenience alias for [versions.Version]
type Version = versions.Version

type UpgradeCandidate struct {
	Module         ModulePath
	CurrentVersion Version
	LatestVersion  Version
}

func parseVersion(raw string) (Version, error) {
	if !strings.HasPrefix(raw, "v") {
		return versions.Unspecified, fmt.Errorf("missing 'v' prefix")
	}
	raw = raw[1:] // the "versions" library doesn't actually want the prefix
	return versions.ParseVersion(raw)
}
