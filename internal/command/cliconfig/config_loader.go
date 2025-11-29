// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package cliconfig

import (
	"io/fs"
	"os"
)

// ConfigFileSystem is an abstraction layer for file system operations
// involved in loading the CLI configuration.
//
// The logic of choosing CLI configuration files is highly dependent on
// the location of those files in the filesystem. Since it's not feasible
// to test that in a real filesystem, we separate the filesystem operations
// from the logic of how the files are chosen. The real CLI config loader
// uses standard os package functions, and the test config loader uses
// fstest-based functions.
type ConfigFileSystem interface {
	// Open opens the named file for reading.
	Open(name string) (fs.File, error)
	// ReadDir reads the named directory, returning all its directory entries sorted by filename.
	ReadDir(name string) ([]os.DirEntry, error)
	// ReadFile reads the named file and returns the contents.
	ReadFile(name string) ([]byte, error)
	// Stat returns a [FileInfo] describing the named file. If there is an error, it will be of type [*PathError].
	Stat(name string) (os.FileInfo, error)
}

type standardFileSystem struct{}

func (sfs *standardFileSystem) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(name)
}

func (sfs *standardFileSystem) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}

func (sfs *standardFileSystem) ReadDir(name string) ([]os.DirEntry, error) {
	return os.ReadDir(name)
}

func (tfs *standardFileSystem) Open(name string) (fs.File, error) {
	return os.Open(name)
}

type ConfigLoader struct {
	ConfigFileSystem
}

func standardConfigLoader() *ConfigLoader {
	return &ConfigLoader{ConfigFileSystem: &standardFileSystem{}}
}
