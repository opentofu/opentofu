// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build !windows
// +build !windows

package cliconfig

import (
	"errors"
	"io/fs"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

func rootFileSystem() fs.FS {
	return os.DirFS(string(os.PathSeparator))
}

func configFile(fileSystem fs.FS) (string, error) {
	dir, err := homeDir()
	if err != nil {
		return "", err
	}

	newConfigFile := filepath.Join(dir, ".tofurc")
	legacyConfigFile := filepath.Join(dir, ".terraformrc")

	if xdgDir := os.Getenv("XDG_CONFIG_HOME"); xdgDir != "" && !pathExists(fileSystem, legacyConfigFile) && !pathExists(fileSystem, newConfigFile) {
		// a fresh install should not use terraform naming
		return filepath.Join(xdgDir, "opentofu", "tofurc"), nil
	}

	return getNewOrLegacyPath(fileSystem, newConfigFile, legacyConfigFile)
}

func configDir(fileSystem fs.FS) (string, error) {
	dir, err := homeDir()
	if err != nil {
		return "", err
	}

	configDir := filepath.Join(dir, ".terraform.d")
	if xdgDir := os.Getenv("XDG_CONFIG_HOME"); !pathExists(fileSystem, configDir) && xdgDir != "" {
		configDir = filepath.Join(xdgDir, "opentofu")
	}

	return configDir, nil
}

func dataDirs(_ fs.FS) ([]string, error) {
	dir, err := homeDir()
	if err != nil {
		return nil, err
	}

	dirs := []string{filepath.Join(dir, ".terraform.d")}
	if xdgDir := os.Getenv("XDG_DATA_HOME"); xdgDir != "" {
		dirs = append(dirs, filepath.Join(xdgDir, "opentofu"))
	}

	return dirs, nil
}

func homeDir() (string, error) {
	// First prefer the HOME environmental variable
	if home := os.Getenv("HOME"); home != "" {
		// FIXME: homeDir gets called from globalPluginDirs during init, before
		// the logging is set up.  We should move meta initialization outside of
		// init, but in the meantime we just need to silence this output.
		// log.Printf("[DEBUG] Detected home directory from env var: %s", home)

		return home, nil
	}

	// If that fails, try build-in module
	user, err := user.Current()
	if err != nil {
		return "", err
	}

	if user.HomeDir == "" {
		return "", errors.New("blank output")
	}

	return user.HomeDir, nil
}

// fsRelativize removes the leading and trailing slash from an absolute file path. The fs.FS filesystem type only works with
// "relative directories". So, a DirFS based at "/" will take a file path like "home/username/.tofurc" and
// look in the operating system file system at "/home/username/.tofurc".
// More details in this documentation: https://pkg.go.dev/io/fs#ValidPath
func fsRelativize(dir string) string {
	if dir == "" {
		return ""
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		log.Printf("[WARNING] Attempted to form absolute representation of relative path \"%s\", but ran into an error: %v", dir, err)
	}
	return filepath.ToSlash(strings.Trim(absDir, string(os.PathSeparator)))
}

func pathExists(fileSystem fs.FS, path string) bool {
	_, err := fs.Stat(fileSystem, fsRelativize(path))
	return err == nil
}
