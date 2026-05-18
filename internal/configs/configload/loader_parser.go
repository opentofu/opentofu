// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// This file contains the configs.Parser wrapper methods.
// A Loader implementation should not expose its inner components but provide
// logic around it.

package configload

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/configs"
)

// LoadConfigDirUneval implements Loader
func (l *loader) LoadConfigDirUneval(path string, load configs.SelectiveLoader) (*configs.Module, hcl.Diagnostics) {
	return l.parser.LoadConfigDirUneval(path, load)
}

// LoadConfigDir implements Loader
func (l *loader) LoadConfigDir(path string, call configs.StaticModuleCall) (*configs.Module, hcl.Diagnostics) {
	return l.parser.LoadConfigDir(path, call)
}

// LoadHCLFile implements Loader
func (l *loader) LoadHCLFile(path string) (hcl.Body, hcl.Diagnostics) {
	return l.parser.LoadHCLFile(path)
}

// LoadConfigDirSelective implements Loader
func (l *loader) LoadConfigDirSelective(path string, call configs.StaticModuleCall, load configs.SelectiveLoader) (*configs.Module, hcl.Diagnostics) {
	return l.parser.LoadConfigDirSelective(path, call, load)
}

// LoadConfigDirWithTests implements Loader
func (l *loader) LoadConfigDirWithTests(path string, testDirectory string, call configs.StaticModuleCall) (*configs.Module, hcl.Diagnostics) {
	return l.parser.LoadConfigDirWithTests(path, testDirectory, call)
}

// ForceFileSource implements Loader
func (l *loader) ForceFileSource(filename string, src []byte) {
	l.parser.ForceFileSource(filename, src)
}
