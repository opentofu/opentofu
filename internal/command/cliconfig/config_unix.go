// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build !windows
// +build !windows

package cliconfig

import (
	"errors"
	"os"
	"os/user"
	"path/filepath"
)

func configFile() (string, error) {
	dir, err := homeDir()
	if err != nil {
		return "", err
	}

	newConfigFile := filepath.Join(dir, ".tofurc")
	legacyConfigFile := filepath.Join(dir, ".terraformrc")

	if xdgDir := os.Getenv("XDG_CONFIG_HOME"); xdgDir != "" && !pathExists(legacyConfigFile) && !pathExists(newConfigFile) {
		// a fresh install should not use terraform naming
		return filepath.Join(xdgDir, "opentofu", "tofurc"), nil
	}

	return getNewOrLegacyPath(newConfigFile, legacyConfigFile)
}

func configDir() (string, error) {
	dir, err := homeDir()
	if err != nil {
		return "", err
	}

	configDir := filepath.Join(dir, ".terraform.d")
	if xdgDir := os.Getenv("XDG_CONFIG_HOME"); !pathExists(configDir) && xdgDir != "" {
		configDir = filepath.Join(xdgDir, "opentofu")
	}

	return configDir, nil
}

func dataDirs() ([]string, error) {
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

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
