// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package cliconfig

import "os"

type ConfigFileSystem interface {
	// ReadFile can read files
	ReadFile(name string) ([]byte, error)
	// Stat can stat files
	Stat(name string) (os.FileInfo, error)
	// ReadDir can read files in a directory
	ReadDir(name string) ([]os.DirEntry, error)
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

type ConfigLoader struct {
	ConfigFileSystem
}

func standardConfigLoader() *ConfigLoader {
	return &ConfigLoader{ConfigFileSystem: &standardFileSystem{}}
}
