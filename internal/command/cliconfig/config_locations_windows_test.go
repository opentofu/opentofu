// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build windows
// +build windows

package cliconfig

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/davecgh/go-spew/spew"
)

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
			fileSystem := fstest.MapFS{}
			n := len(test.files)
			for i, file := range test.files {
				b, err := getFile(i, n)
				if err != nil {
					t.Fatalf("failed to generate file %s: %v", file, err)
				}
				// TODO trim correctly
				s, _ := strings.CutPrefix(file, "C:\\")
				fileSystem[filepath.ToSlash(s)] = &fstest.MapFile{
					Data: b,
					Mode: 0o600,
				}
			}
			for _, directory := range test.directories {
				s, _ := strings.CutPrefix(directory, "C:\\")
				fileSystem[filepath.ToSlash(s)] = &fstest.MapFile{
					Data: nil,
					Mode: fs.ModeDir | 0o755,
				}
			}
			for k, v := range test.envVars {
				t.Setenv(k, v)
			}
			actual, diags := LoadConfig(t.Context(), fileSystem)
			if diags.HasErrors() {
				t.Fatalf("no errors expected, got errors from diags")
			}
			if !reflect.DeepEqual(actual.Hosts, test.expected) {
				t.Errorf("wrong result\ngot:  %s\nwant: %s", spew.Sdump(actual.Hosts), spew.Sdump(test.expected))
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
			fileSystem := fstest.MapFS{}
			for _, directory := range test.directories {
				s, _ := strings.CutPrefix(directory, "C:\\")
				fileSystem[filepath.ToSlash(s)] = &fstest.MapFile{
					Data: nil,
					Mode: fs.ModeDir | 0o755,
				}
			}
			for k, v := range test.envVars {
				t.Setenv(k, v)
			}
			actual, err := ConfigDir(fileSystem)
			if err != nil {
				t.Fatalf("no errors expected, got errors from diags")
			}
			if actual != test.expected[0] {
				t.Errorf("wrong result\ngot:  %s\nwant: %s", actual, test.expected[0])
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
			fileSystem := fstest.MapFS{}
			for _, directory := range test.directories {
				s, _ := strings.CutPrefix(directory, "C:\\")
				fileSystem[filepath.ToSlash(s)] = &fstest.MapFile{
					Data: nil,
					Mode: fs.ModeDir | 0o755,
				}
			}
			for k, v := range test.envVars {
				t.Setenv(k, v)
			}
			actual, err := DataDirs(fileSystem)
			if err != nil {
				t.Fatalf("no errors expected, got errors from diags")
			}
			if !reflect.DeepEqual(actual, test.expected) {
				t.Errorf("wrong result\ngot:  %s\nwant: %s", spew.Sdump(actual), spew.Sdump(test.expected))
			}
		})
	}
}
