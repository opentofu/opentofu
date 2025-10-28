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

func (cl *ConfigLoader) configFile() (string, error) {
	dir, err := cl.homeDir()
	if err != nil {
		return "", err
	}

	newConfigFile := filepath.Join(dir, ".tofurc")
	legacyConfigFile := filepath.Join(dir, ".terraformrc")

	if xdgDir := os.Getenv("XDG_CONFIG_HOME"); xdgDir != "" && !cl.pathExists(legacyConfigFile) && !cl.pathExists(newConfigFile) {
		// a fresh install should not use terraform naming
		return filepath.Join(xdgDir, "opentofu", "tofurc"), nil
	}

	return getNewOrLegacyPath(cl, newConfigFile, legacyConfigFile)
}

func (cl *ConfigLoader) configDir() (string, error) {
	dir, err := cl.homeDir()
	if err != nil {
		return "", err
	}

	configDir := filepath.Join(dir, ".terraform.d")
	if xdgDir := os.Getenv("XDG_CONFIG_HOME"); !cl.pathExists(configDir) && xdgDir != "" {
		configDir = filepath.Join(xdgDir, "opentofu")
	}

	return configDir, nil
}

func (cl *ConfigLoader) dataDirs() ([]string, error) {
	dir, err := cl.homeDir()
	if err != nil {
		return nil, err
	}

	dirs := []string{filepath.Join(dir, ".terraform.d")}
	if xdgDir := os.Getenv("XDG_DATA_HOME"); xdgDir != "" {
		dirs = append(dirs, filepath.Join(xdgDir, "opentofu"))
	}

	return dirs, nil
}

func (cl *ConfigLoader) homeDir() (string, error) {
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

func (cl *ConfigLoader) pathExists(path string) bool {
	_, err := cl.Stat(path)
	return err == nil
}
