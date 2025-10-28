// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build !windows && !darwin
// +build !windows,!darwin

package cliconfig

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/google/go-cmp/cmp"
)

func (tfs *testFileSystem) trim(name string) string {
	return strings.TrimLeft(name, string(os.PathSeparator))
}

var commonEnvVars = []string{
	"XDG_CONFIG_HOME",
	"XDG_DATA_HOME",
	"TF_CLI_CONFIG_FILE",
}

func TestConfigFileLocations(t *testing.T) {
	home := os.Getenv("HOME")
	xdgDir := filepath.Join(home, ".myconfig")
	tests := []locationTest{
		{
			locationTestParameters: locationTestParameters{
				name:  ".tofurc only",
				files: []string{filepath.Join(home, ".tofurc")},
			},
			expected: map[string]*ConfigHost{
				"config0.example.com": {
					Services: map[string]interface{}{
						"modules.v0": "https://config0.example.com/",
					},
				},
			},
		},
		{
			locationTestParameters: locationTestParameters{
				name:  ".terraformrc only",
				files: []string{filepath.Join(home, ".terraformrc")},
			},
			expected: map[string]*ConfigHost{
				"config0.example.com": {
					Services: map[string]interface{}{
						"modules.v0": "https://config0.example.com/",
					},
				},
			},
		},
		{
			locationTestParameters: locationTestParameters{
				name:  ".terraformrc and .tofurc",
				files: []string{filepath.Join(home, ".terraformrc"), filepath.Join(home, ".tofurc")},
			},
			expected: map[string]*ConfigHost{
				"config1.example.com": {
					Services: map[string]interface{}{
						"modules.v1": "https://config1.example.com/",
					},
				},
				"0and1.example.com": {
					Services: map[string]interface{}{
						"modules.v1": "https://0and1.example.com/",
					},
				},
			},
		},
		{
			locationTestParameters: locationTestParameters{
				name:        "xdg directory, but with .tofurc and .terraformrc present",
				files:       []string{filepath.Join(home, ".terraformrc"), filepath.Join(home, ".tofurc"), filepath.Join(xdgDir, "opentofu", "tofurc")},
				directories: []string{xdgDir},
				envVars:     map[string]string{"XDG_CONFIG_HOME": xdgDir},
			},
			expected: map[string]*ConfigHost{
				"config1.example.com": {
					Services: map[string]interface{}{
						"modules.v1": "https://config1.example.com/",
					},
				},
				"0and1.example.com": {
					Services: map[string]interface{}{
						"modules.v1": "https://0and1.example.com/",
					},
				},
				"1and2.example.com": {
					Services: map[string]interface{}{
						"modules.v1": "https://1and2.example.com/",
					},
				},
			},
		},
		{
			locationTestParameters: locationTestParameters{
				name:        "xdg directory without .tofurc and .terraformrc present",
				files:       []string{filepath.Join(xdgDir, "opentofu", "tofurc")},
				directories: []string{xdgDir},
				envVars:     map[string]string{"XDG_CONFIG_HOME": xdgDir},
			},
			expected: map[string]*ConfigHost{
				"config0.example.com": {
					Services: map[string]interface{}{
						"modules.v0": "https://config0.example.com/",
					},
				},
			},
		},
		{
			locationTestParameters: locationTestParameters{
				name:    "ignore everything else when env override is present",
				files:   []string{filepath.Join(home, "mytofufile"), filepath.Join(home, ".terraformrc"), filepath.Join(home, ".tofurc")},
				envVars: map[string]string{"TF_CLI_CONFIG_FILE": filepath.Join(home, "mytofufile")},
			},
			expected: map[string]*ConfigHost{
				"config0.example.com": {
					Services: map[string]interface{}{
						"modules.v0": "https://config0.example.com/",
					},
				},
				"0and1.example.com": {
					Services: map[string]interface{}{
						"modules.v0": "https://0and1.example.com/",
					},
				},
				"0and2.example.com": {
					Services: map[string]interface{}{
						"modules.v0": "https://0and2.example.com/",
					},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tfs := testFileSystem{
				fsys: fstest.MapFS{},
			}
			cl := ConfigLoader{
				ConfigFileSystem: &tfs,
			}
			n := len(test.files)
			for i, file := range test.files {
				b, err := getFile(i, n)
				if err != nil {
					t.Fatalf("failed to generate file %s: %v", file, err)
				}
				tfs.fsys[strings.TrimLeft(file, string(os.PathSeparator))] = &fstest.MapFile{
					Data: b,
					Mode: 0o600,
				}
			}
			for _, directory := range test.directories {
				tfs.fsys[strings.TrimLeft(directory, string(os.PathSeparator))] = &fstest.MapFile{
					Data: nil,
					Mode: fs.ModeDir | 0o755,
				}
			}
			for _, v := range commonEnvVars {
				t.Setenv(v, "")
			}
			for k, v := range test.envVars {
				t.Setenv(k, v)
			}
			actual, diags := cl.LoadConfig(t.Context())
			if diags.HasErrors() {
				t.Fatalf("no errors expected, got errors from diags")
			}
			if diff := cmp.Diff(actual.Hosts, test.expected); diff != "" {
				t.Error("unexpected result\n" + diff)
			}
		})
	}
}

func TestConfigDirLocations(t *testing.T) {
	home := os.Getenv("HOME")
	xdgDir := filepath.Join(home, ".myconfig")
	terraformD := filepath.Join(home, ".terraform.d")
	tests := []directoryLocationTest{
		{
			locationTestParameters: locationTestParameters{
				name: "default directory",
			},
			expected: []string{terraformD},
		},
		{
			locationTestParameters: locationTestParameters{
				name:        "xdg directory",
				envVars:     map[string]string{"XDG_CONFIG_HOME": xdgDir},
				directories: []string{xdgDir},
			},
			expected: []string{filepath.Join(xdgDir, "opentofu")},
		},
		{
			locationTestParameters: locationTestParameters{
				name:        "xdg directory, but with extant .terraform.d directory",
				envVars:     map[string]string{"XDG_CONFIG_HOME": xdgDir},
				directories: []string{xdgDir, terraformD},
			},
			expected: []string{terraformD},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tfs := testFileSystem{
				fsys: fstest.MapFS{},
			}
			cl := ConfigLoader{
				ConfigFileSystem: &tfs,
			}
			for _, directory := range test.directories {
				tfs.fsys[strings.TrimLeft(directory, string(os.PathSeparator))] = &fstest.MapFile{
					Data: nil,
					Mode: fs.ModeDir | 0o755,
				}
			}
			for _, v := range commonEnvVars {
				t.Setenv(v, "")
			}
			for k, v := range test.envVars {
				t.Setenv(k, v)
			}
			actual, err := cl.ConfigDir()
			if err != nil {
				t.Fatalf("no errors expected, got errors from diags")
			}
			if diff := cmp.Diff(actual, test.expected[0]); diff != "" {
				t.Error("unexpected result\n" + diff)
			}
		})
	}
}
func TestDataDirLocations(t *testing.T) {
	home := os.Getenv("HOME")
	xdgDir := filepath.Join(home, ".mydata")
	terraformD := filepath.Join(home, ".terraform.d")
	// Note that neither of these tests depend on the existence of the underlying directories.
	tests := []directoryLocationTest{
		{
			locationTestParameters: locationTestParameters{
				name: "default directory",
			},
			expected: []string{terraformD},
		},
		{
			locationTestParameters: locationTestParameters{
				name:    "xdg directory",
				envVars: map[string]string{"XDG_DATA_HOME": xdgDir},
			},
			expected: []string{terraformD, filepath.Join(xdgDir, "opentofu")},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tfs := testFileSystem{
				fsys: fstest.MapFS{},
			}
			cl := ConfigLoader{
				ConfigFileSystem: &tfs,
			}
			for _, directory := range test.directories {
				tfs.fsys[strings.TrimLeft(directory, string(os.PathSeparator))] = &fstest.MapFile{
					Data: nil,
					Mode: fs.ModeDir | 0o755,
				}
			}
			for _, v := range commonEnvVars {
				t.Setenv(v, "")
			}
			for k, v := range test.envVars {
				t.Setenv(k, v)
			}
			actual, err := cl.DataDirs()
			if err != nil {
				t.Fatalf("no errors expected, got errors from diags")
			}
			if diff := cmp.Diff(actual, test.expected); diff != "" {
				t.Error("unexpected result\n" + diff)
			}
		})
	}
}
