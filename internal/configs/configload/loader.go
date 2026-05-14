// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configload

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/hashicorp/hcl/v2"
	"github.com/spf13/afero"

	"github.com/opentofu/opentofu/internal/configs"
)

type Loader interface {
	ImportSources(sources map[string][]byte)
	ImportSourcesFromSnapshot(snap *Snapshot)
	IsConfigDir(path string) bool
	ModulesDir() string
	RefreshModules() error
	Sources() map[string]*hcl.File
	LoadConfig(ctx context.Context, rootDir string, call configs.StaticModuleCall) (*configs.Config, hcl.Diagnostics)
	LoadConfigWithTests(ctx context.Context, rootDir string, testDir string, call configs.StaticModuleCall) (*configs.Config, hcl.Diagnostics)
	LoadConfigWithSnapshot(ctx context.Context, rootDir string, call configs.StaticModuleCall) (*configs.Config, *Snapshot, hcl.Diagnostics)

	LoadConfigDirUneval(path string, load configs.SelectiveLoader) (*configs.Module, hcl.Diagnostics)
	LoadConfigDir(path string, call configs.StaticModuleCall) (*configs.Module, hcl.Diagnostics)
	LoadHCLFile(path string) (hcl.Body, hcl.Diagnostics)
	LoadConfigDirSelective(path string, call configs.StaticModuleCall, load configs.SelectiveLoader) (*configs.Module, hcl.Diagnostics)
	LoadConfigDirWithTests(path string, testDirectory string, call configs.StaticModuleCall) (*configs.Module, hcl.Diagnostics)
	ForceFileSource(filename string, src []byte)
}

// A loader instance is the main entry-point for loading configurations via
// this package.
//
// It extends the general config-loading functionality in the parent package
// "configs" to support installation of modules from remote sources and
// loading full configurations using modules that were previously installed.
type loader struct {
	// parser is used to read configuration
	parser *configs.Parser

	// modules is used to install and locate descendent modules that are
	// referenced (directly or indirectly) from the root module.
	modules moduleMgr
}

// Config is used with NewLoader to specify configuration arguments for the
// loader.
type Config struct {
	// ModulesDir is a path to a directory where descendent modules are
	// (or should be) installed. (This is usually the
	// .terraform/modules directory, in the common case where this package
	// is being loaded from the main OpenTofu CLI package.)
	ModulesDir string

	// AllowLanguageExperiments specifies whether subsequent Loader.LoadConfig (and
	// similar) calls will allow opting in to experimental language features.
	//
	// If this is not configured as true, any language experiments will be disallowed.
	//
	// Main code should set this only for alpha or development builds. Test code
	// is responsible for deciding for itself whether and how to call this
	// method.
	//
	// We don't currently have any support for language experiments. We'll
	// add support here later if we decide to make use of language experiments
	// in future versions of OpenTofu.
	// This is the reason why this attribute is not used.
	AllowLanguageExperiments bool
}

// NewLoader creates and returns a loader that reads configuration from the
// real OS filesystem.
//
// The loader has some internal state about the modules that are currently
// installed, which is read from disk as part of this function. If that
// manifest cannot be read then an error will be returned.
func NewLoader(config *Config) (Loader, error) {
	fs := afero.NewOsFs()
	parser := configs.NewParser(fs)

	ret := &loader{
		parser: parser,
		modules: moduleMgr{
			FS:         afero.Afero{Fs: fs},
			CanInstall: true,
			Dir:        config.ModulesDir,
		},
	}

	err := ret.modules.readModuleManifestSnapshot()
	if err != nil {
		return nil, fmt.Errorf("failed to read module manifest: %w", err)
	}

	return ret, nil
}

// ModulesDir returns the path to the directory where the loader will look for
// the local cache of remote module packages.
func (l *loader) ModulesDir() string {
	return l.modules.Dir
}

// RefreshModules updates the in-memory cache of the module manifest from the
// module manifest file on disk. This is not necessary in normal use because
// module installation and configuration loading are separate steps, but it
// can be useful in tests where module installation is done as a part of
// configuration loading by a helper function.
//
// Call this function after any module installation where an existing loader
// is already alive and may be used again later.
//
// An error is returned if the manifest file cannot be read.
func (l *loader) RefreshModules() error {
	if l == nil {
		// Nothing to do, then.
		return nil
	}
	return l.modules.readModuleManifestSnapshot()
}

// Sources returns the source code cache for the underlying parser of this
// loader. This is a shorthand for l.Parser().Sources().
func (l *loader) Sources() map[string]*hcl.File {
	return l.parser.Sources()
}

// IsConfigDir returns true if and only if the given directory contains at
// least one OpenTofu configuration file. This is a wrapper around calling
// the same method name on the loader's parser.
func (l *loader) IsConfigDir(path string) bool {
	return l.parser.IsConfigDir(path)
}

// ImportSources writes into the receiver's source code map the given source
// code buffers.
//
// This is useful in the situation where an ancillary loader is created for
// some reason (e.g. loading config from a plan file) but the cached source
// code from that loader must be imported into the "main" loader in order
// to return source code snapshots in diagnostic messages.
//
//	loader.ImportSources(otherLoader.Sources())
func (l *loader) ImportSources(sources map[string][]byte) {
	for name, src := range sources {
		l.parser.ForceFileSource(name, src)
	}
}

// ImportSourcesFromSnapshot writes into the receiver's source code the
// source files from the given snapshot.
//
// This is similar to ImportSources but knows how to unpack and flatten a
// snapshot data structure to get the corresponding flat source file map.
func (l *loader) ImportSourcesFromSnapshot(snap *Snapshot) {
	for _, m := range snap.Modules {
		baseDir := m.Dir
		for fn, src := range m.Files {
			fullPath := filepath.Join(baseDir, fn)
			l.parser.ForceFileSource(fullPath, src)
		}
	}
}
