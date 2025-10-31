// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build windows

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

var commonEnvVars = []string{
	"XDG_CONFIG_HOME",
	"XDG_DATA_HOME",
	"TF_CLI_CONFIG_FILE",
}

var DRIVE = os.Getenv("SystemDrive")

func (tfs *testFileSystem) trim(name string) string {
	subname := strings.TrimPrefix(name, DRIVE)
	return filepath.ToSlash(strings.TrimLeft(subname, string(os.PathSeparator)))
}

func TestConfigFileLocations(t *testing.T) {
	configDir := os.Getenv("APPDATA")
	tests := []locationTest{
		{
			locationTestParameters: locationTestParameters{
				name:  "tofu.rc only",
				files: []string{filepath.Join(configDir, "tofu.rc")},
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
				name:  "terraform.rc only",
				files: []string{filepath.Join(configDir, "terraform.rc")},
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
				name:  "tofu.rc and terraform.rc",
				files: []string{filepath.Join(configDir, "terraform.rc"), filepath.Join(configDir, "tofu.rc")},
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
				tfs.fsys[filepath.ToSlash(file[3:])] = &fstest.MapFile{
					Data: b,
					Mode: 0o600,
				}
			}
			for _, directory := range test.directories {
				tfs.fsys[filepath.ToSlash(directory[3:])] = &fstest.MapFile{
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
	configDir := os.Getenv("APPDATA")
	terraformD := filepath.Join(configDir, "terraform.d")
	tests := []directoryLocationTest{
		{
			locationTestParameters: locationTestParameters{
				name: "default directory",
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
				tfs.fsys[filepath.ToSlash(directory[3:])] = &fstest.MapFile{
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
	configDir := os.Getenv("APPDATA")
	terraformD := filepath.Join(configDir, "terraform.d")
	// Note that neither of these tests depend on the existence of the underlying directories.
	tests := []directoryLocationTest{
		{
			locationTestParameters: locationTestParameters{
				name: "default directory",
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
				tfs.fsys[filepath.ToSlash(directory[3:])] = &fstest.MapFile{
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
