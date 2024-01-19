// Copyright (c) HashiCorp, Inc.
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

// Default directories to use when XDG_* is not defined
// https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html
const (
	defaultConfigDir = ".config"
	defaultDataDir   = ".local/share"
)

func configFile() (string, error) {
	dir, err := homeDir()
	if err != nil {
		return "", err
	}

	newConfigFile := filepath.Join(dir, ".tofurc")
	legacyConfigFile := filepath.Join(dir, ".terraformrc")

	if !pathExists(legacyConfigFile) && !pathExists(newConfigFile) {
		// a fresh install should not use terraform naming
		return filepath.Join(lookupEnv("XDG_CONFIG_HOME", filepath.Join(dir, defaultConfigDir)), "opentofu", "tofurc"), nil
	}

	return getNewOrLegacyPath(newConfigFile, legacyConfigFile)
}

func configDir() (string, error) {
	dir, err := homeDir()
	if err != nil {
		return "", err
	}

	configDir := filepath.Join(dir, ".terraform.d")
	if !pathExists(configDir) {
		configDir = filepath.Join(lookupEnv("XDG_CONFIG_HOME", filepath.Join(dir, defaultConfigDir)), "opentofu")
	}

	return configDir, nil
}

func dataDir() (string, error) {
	dir, err := homeDir()
	if err != nil {
		return "", err
	}

	dataDir := filepath.Join(dir, ".terraform.d")
	if !pathExists(dataDir) {
		dataDir = filepath.Join(lookupEnv("XDG_DATA_HOME", filepath.Join(dir, defaultDataDir)), "opentofu")
	}

	return dataDir, nil
}

func lookupEnv(name, defaultValue string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return defaultValue
}

func homeDir() (string, error) {
	// First prefer the HOME environmental variable
	if home := os.Getenv("HOME"); home != "" {
		// FIXME: homeDir gets called from globalPluginDirs during init, before
		// the logging is set up.  We should move meta initializtion outside of
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
