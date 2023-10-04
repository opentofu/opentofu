// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build !windows
// +build !windows

package cliconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigFileConfigDir(t *testing.T) {
	baseDir := t.TempDir()
	homeDir := filepath.Join(baseDir, "home")

	tests := []struct {
		name     string
		xdgHome  string
		files    []string
		testFunc func() (string, error)
		expect   string
	}{
		{
			name:     "configFile: use home tofurc",
			testFunc: configFile,
			files:    []string{filepath.Join(homeDir, ".tofurc")},
			expect:   filepath.Join(homeDir, ".tofurc"),
		},
		{
			name:     "configFile: use home terraformrc",
			testFunc: configFile,
			files:    []string{filepath.Join(homeDir, ".terraformrc")},
			expect:   filepath.Join(homeDir, ".terraformrc"),
		},
		{
			name:     "configFile: use home tofurc fallback",
			testFunc: configFile,
			expect:   filepath.Join(homeDir, ".tofurc"),
		},
		{
			name:     "configFile: use xdg tofurc",
			testFunc: configFile,
			xdgHome:  filepath.Join(baseDir, "xdg"),
			expect:   filepath.Join(baseDir, "xdg", "opentofu", "tofurc"),
		},
		{
			name:     "configFile: prefer home tofurc",
			testFunc: configFile,
			xdgHome:  filepath.Join(baseDir, "xdg"),
			files:    []string{filepath.Join(homeDir, ".tofurc")},
			expect:   filepath.Join(homeDir, ".tofurc"),
		},
		{
			name:     "configFile: prefer home terraformrc",
			testFunc: configFile,
			xdgHome:  filepath.Join(baseDir, "xdg"),
			files:    []string{filepath.Join(homeDir, ".terraformrc")},
			expect:   filepath.Join(homeDir, ".terraformrc"),
		},
		{
			name:     "configDir: no xdg",
			testFunc: configDir,
			expect:   filepath.Join(homeDir, ".terraform.d"),
		},
		{
			name:     "configDir: xdg but path exists",
			testFunc: configDir,
			xdgHome:  filepath.Join(baseDir, "xdg"),
			files:    []string{filepath.Join(homeDir, ".terraform.d", "placeholder")},
			expect:   filepath.Join(homeDir, ".terraform.d"),
		},
		{
			name:     "configDir: use xdg",
			testFunc: configDir,
			xdgHome:  filepath.Join(baseDir, "xdg"),
			expect:   filepath.Join(baseDir, "xdg", "opentofu"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("HOME", homeDir)
			t.Setenv("XDG_CONFIG_HOME", test.xdgHome)
			for _, f := range test.files {
				createFile(t, f)
			}

			file, err := test.testFunc()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if test.expect != file {
				t.Fatalf("expected %q, but got %q", test.expect, file)
			}
		})
	}
}

func createFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(path)) })
}
