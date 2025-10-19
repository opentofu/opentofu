// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"

	"github.com/apparentlymart/go-shquot/shquot"
)

func findUpgradeCandidates() (map[ModulePath]UpgradeCandidate, error) {
	cmd := exec.Command("go", "list", "-json=Path,Version,Update,Indirect", "-m", "-u", "all")
	raw, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("running %s: %w", cmdlineForErrorMessage(cmd), err)
	}

	type Entry struct {
		Path     string `json:"Path"`
		Version  string `json:"Version"`
		Indirect bool   `json:"Indirect"`
		Update   *Entry `json:"Update"`
	}

	candidates := make(map[ModulePath]UpgradeCandidate)
	dec := json.NewDecoder(bytes.NewReader(raw))
	for {
		var entry Entry
		err := dec.Decode(&entry)
		if err == io.EOF {
			break // we've reached the end of the results
		}
		if err != nil {
			return nil, fmt.Errorf("invalid JSON object in result: %w", err)
		}

		// First we'll make sure this object is of a sensible shape that
		// matches our expectations. These situations can arise legitimately for
		// modules where we're using "replace" directives, or other such
		// oddities, so we'll just skip them and assume we'll be managing their
		// upgrades in some other way.
		if entry.Update == nil || entry.Update.Path != entry.Path {
			continue
		}

		modulePath := ModulePath(entry.Path)
		currentVersion, err := parseVersion(entry.Version)
		if err != nil {
			return nil, fmt.Errorf("entry for %q has invalid current version %q: %w", entry.Path, entry.Version, err)
		}
		latestVersion, err := parseVersion(entry.Update.Version)
		if err != nil {
			return nil, fmt.Errorf("entry for %q has invalid latest version %q: %w", entry.Path, entry.Update.Version, err)
		}

		if entry.Indirect {
			// Only our direct dependencies are upgrade canididates; upgrading
			// those will automatically ratchet the indirect dependencies
			// as needed.
			continue
		}
		if latestVersion.Same(currentVersion) {
			continue
		}
		candidates[modulePath] = UpgradeCandidate{
			Module:         modulePath,
			CurrentVersion: currentVersion,
			LatestVersion:  latestVersion,
		}
	}

	return candidates, nil
}

func findModuleDependencies(modulePath ModulePath, version Version) (map[ModulePath]Version, error) {
	// This ensures that we have a copy of this module version's go.mod in
	// the local Go module cache and then returns the path to that cached
	// copy of the file.
	goModPath, err := findGoModPath(modulePath, version)
	if err != nil {
		return nil, fmt.Errorf("fetching go.mod file for %s@%s: %w", modulePath, version, err)
	}

	ret, err := findGoModDependencies(goModPath)
	if err != nil {
		return nil, fmt.Errorf("reading dependency information from %s: %w", goModPath, err)
	}
	return ret, nil
}

func findGoModPath(modulePath ModulePath, version Version) (string, error) {
	cmd := exec.Command("go", "list", "-json=GoMod", "-m", string(modulePath)+"@v"+version.String())
	raw, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("running %s: %w", cmdlineForErrorMessage(cmd), err)
	}

	type Entry struct {
		GoMod string `json:"GoMod"`
	}
	var entry Entry
	err = json.Unmarshal(raw, &entry)
	if err != nil {
		return "", fmt.Errorf("invalid JSON object in result: %w", err)
	}
	if entry.GoMod == "" {
		// Should not happen because everything we depend on is a modern
		// module, but this could in principle catch a dependency on a legacy
		// codebase that predates Go Modules.
		return "", fmt.Errorf("no go.mod file available")
	}
	return entry.GoMod, nil
}

func findGoModDependencies(goModPath string) (map[ModulePath]Version, error) {
	// Despite the command name, the following is not actually making any
	// edits to the file: this just parses the go.mod file and returns a
	// JSON description of its contents.
	cmd := exec.Command("go", "mod", "edit", "-json", goModPath)
	raw, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("running %s: %w", cmdlineForErrorMessage(cmd), err)
	}

	type Requirement struct {
		Path    string `json:"Path"`
		Version string `json:"Version"`

		// NOTE: We intentionally ignore whether a dependency is indirect
		// or not here, because we want to detect whether upgrading to this
		// version of the module would upgrade any of our _own_ direct
		// dependencies regardless of whether they are direct or indirect
		// from the perspective of this other module.
	}
	type Manifest struct {
		Require []Requirement `json:"Require"`
	}
	var manifest Manifest
	err = json.Unmarshal(raw, &manifest)
	if err != nil {
		return nil, fmt.Errorf("invalid JSON object in result: %w", err)
	}

	ret := make(map[ModulePath]Version, len(manifest.Require))
	for _, req := range manifest.Require {
		modulePath := ModulePath(req.Path)
		version, err := parseVersion(req.Version)
		if err != nil {
			return nil, fmt.Errorf("dependency %q has invalid version %q: %w", req.Path, req.Version, err)
		}
		ret[modulePath] = version
	}
	return ret, nil
}

func cmdlineForErrorMessage(cmd *exec.Cmd) string {
	return shquot.POSIXShell(cmd.Args)
}
