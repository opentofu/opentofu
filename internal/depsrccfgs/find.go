// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package depsrccfgs

import (
	"os"
	"path/filepath"
)

// FindImplicitConfigFiles searches the given directory and all of its
// ancestor directories for implicitly-selected dependency source mapping files.
//
// The current default behavior is to search for ".terraform.deps.hcl" and
// ".opentofu.deps.override.hcl" files in ancestor directories, returning
// the override version first if both are found, with the intention that
// ".opentofu.deps.hcl" could be placed into version control while
// the override file can be used locally in a specific dev environment in case
// a developer temporarily needs some different settings for development
// purposes.
func FindImplicitConfigFiles(startDir string) []string {
	const mainFilename = ".opentofu.deps.hcl"
	const overrideFilename = ".opentofu.deps.override.hcl"

	var mainFilePath, overrideFilePath string

	currentDir, err := filepath.Abs(startDir)
	if err != nil {
		// No implicit paths are available if we can't absolutize the
		// given path. (This should be highly unlikely.)
		return nil
	}
	for {
		if mainFilePath == "" {
			candidate := filepath.Join(currentDir, mainFilename)
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				mainFilePath = candidate
			}
		}
		if overrideFilePath == "" {
			candidate := filepath.Join(currentDir, overrideFilename)
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				overrideFilePath = candidate
			}
		}

		nextDir := filepath.Dir(currentDir)
		if nextDir == currentDir {
			// If the directory containing our current directory is itself
			// then we have reached the root of the filesystem, so we're done.
			break
		}
		currentDir = nextDir
	}

	if mainFilePath == "" && overrideFilePath == "" {
		return nil
	}
	ret := make([]string, 0, 2)
	if overrideFilePath != "" {
		ret = append(ret, overrideFilePath)
	}
	if mainFilePath != "" {
		ret = append(ret, mainFilePath)
	}
	return ret
}
