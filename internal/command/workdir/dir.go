// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package workdir

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	workingDirEnvVarKey = "TF_DATA_DIR"

	// DefaultDataDir is the default directory for storing local data.
	DefaultDataDir = ".terraform"

	// modulesDir is the name of the directory inside [Dir.DataDir] that is used to store the configuration
	// used modules.
	modulesDir = "modules"
)

// Dir represents a single OpenTofu working directory.
//
// "Working directory" is unfortunately a slight misnomer, because non-default
// options can potentially stretch the definition such that multiple working
// directories end up appearing to share a data directory, or other similar
// anomalies, but we continue to use this terminology both for historical
// reasons and because it reflects the common case without any special
// overrides.
//
// The naming convention for methods on this type is that methods whose names
// begin with "Override" affect only characteristics of the particular object
// they're called on, changing where it looks for data, while methods whose
// names begin with "Set" will write settings to disk such that other instances
// referring to the same directories will also see them. Given that, the
// "Override" methods should be used only during the initialization steps
// for a Dir object, typically only inside "package main", so that all
// subsequent work elsewhere will access consistent locations on disk.
//
// We're gradually transitioning to using this type to manage working directory
// settings, and so not everything in the working directory "data dir" is
// encapsulated here yet, but hopefully we'll gradually migrate all of those
// settings here over time. The working directory state not yet managed in here
// is typically managed directly in the "command" package, either directly
// inside commands or in methods of the giant command.Meta type.
type Dir struct {
	// mainDir is the path to the directory that we present as the
	// "working directory" in the user model, which is typically the
	// current working directory when running OpenTofu CLI, or the
	// directory explicitly chosen by the user using the -chdir=...
	// global option.
	mainDir string

	// originalDir is the path to the working directory that was
	// selected when creating the OpenTofu CLI process, regardless of
	// -chdir=... being set. This is only for very limited purposes
	// related to backward compatibility; most functionality should
	// use mainDir instead.
	originalDir string

	// dataDir is the path to the directory where we will store our
	// working directory settings and artifacts. This is typically a
	// directory named ".terraform" within mainDir, but users may
	// override it.
	dataDir string
}

// NewWorkdir returns a [*Dir] instance configured with the following:
//   - mainDir that is the current working directory or the directory indicated by -chdir flag.
//   - originalDir that is the directory that the program was executed from. If no -chdir value provided, this equals mainDir.
//   - dataDir is the directory where tofu stores it's per-run-configuration, generally .terraform. This can be configured
//     by using the TF_DATA_DIR env var.
//
// It gets the args that the program has been executed with, extracts the -chdir flag from it and applies it if
// specified, returning back the args without that -chdir flag.
// TODO meta-refactor: the args should be removed from here once the CLI library has been replaced.
func NewWorkdir(args []string) (*Dir, []string, error) {
	originalWd, err := os.Getwd()
	if err != nil {
		return nil, nil, fmt.Errorf("Failed to determine current working directory: %s", err)
	}

	args, err = runChdir(args)
	if err != nil {
		return nil, nil, err
	}

	ret := NewDir(".") // caller should already have used os.Chdir in "-chdir=..." mode
	ret.OverrideOriginalWorkingDir(originalWd)
	if overrideWd := os.Getenv(workingDirEnvVarKey); overrideWd != "" {
		ret.OverrideDataDir(overrideWd)
	}
	return ret, args, nil
}

// NewDir constructs a new working directory, anchored at the given path.
//
// In normal use, mainPath should be "." to reflect the current working
// directory, with "package main" having switched the process's current
// working directory if necessary prior to calling this function. However,
// unusual situations in tests may set mainPath to a temporary directory, or
// similar.
//
// WARNING: Although the logic in this package is intended to work regardless
// of whether mainPath is actually the current working directory, we're
// currently in a transitional state where this package shares responsibility
// for the working directory with various command.Meta methods, and those
// often assume that the main path of the working directory will always be
// ".". If you're writing test code that spans across both areas of
// responsibility then you must ensure that the test temporarily changes the
// test process's working directory to the directory returned by RootModuleDir
// before using the result inside a command.Meta.
func NewDir(mainPath string) *Dir {
	mainPath = filepath.Clean(mainPath)
	return &Dir{
		mainDir:     mainPath,
		originalDir: mainPath,
		dataDir:     filepath.Join(mainPath, DefaultDataDir),
	}
}

// OverrideOriginalWorkingDir records a different path as the
// "original working directory" for the receiver.
//
// Use this only to record the original working directory when OpenTofu is run
// with the -chdir=... global option. In that case, the directory given in
// -chdir=... is the "main path" to pass in to NewDir, while the original
// working directory should be sent to this method.
func (d *Dir) OverrideOriginalWorkingDir(originalPath string) {
	d.originalDir = filepath.Clean(originalPath)
}

// OverrideDataDir chooses a specific alternative directory to read and write
// the persistent working directory settings.
//
// "package main" can call this if it detects that the user has overridden
// the default location by setting the relevant environment variable. Don't
// call this when that environment variable isn't set, in order to preserve
// the default setting of a dot-prefixed directory directly inside the main
// working directory.
func (d *Dir) OverrideDataDir(dataDir string) {
	d.dataDir = filepath.Clean(dataDir)
}

// RootModuleDir returns the directory where we expect to find the root module
// configuration for this working directory.
func (d *Dir) RootModuleDir() string {
	// The root module configuration is just directly inside the main directory.
	return d.mainDir
}

// OriginalWorkingDir returns the true, operating-system-originated working
// directory that the current OpenTofu process was launched from.
//
// This is usually the same as the main working directory, but differs in the
// special case where the user ran OpenTofu with the global -chdir=...
// option. This is here only for a few backward compatibility affordances
// from before we had the -chdir=... option, so should typically not be used
// for anything new.
func (d *Dir) OriginalWorkingDir() string {
	return d.originalDir
}

// DataDir returns the base path where the receiver keeps all of the settings
// and artifacts that must persist between consecutive commands in a single
// session.
//
// This is exported only to allow the legacy behaviors in command.Meta to
// continue accessing this directory directly. Over time we should replace
// all of those direct accesses with methods on this type, and then remove
// this method. Avoid using this method for new use-cases.
func (d *Dir) DataDir() string {
	return d.dataDir
}

// ModulesDir returns the directory in the [Dir.DataDir] that is used to store
// the cached modules.
func (d *Dir) ModulesDir() string {
	return filepath.Join(d.DataDir(), modulesDir)
}

// ensureDataDir creates the data directory and all of the necessary parent
// directories that lead to it, if they don't already exist.
//
// For directories that already exist ensureDataDir will preserve their
// permissions, while it'll create any new directories to be owned by the user
// running OpenTofu, readable and writable by that user, and readable by
// all other users, or some approximation of that on non-Unix platforms which
// have a different permissions model.
func (d *Dir) ensureDataDir() error {
	err := os.MkdirAll(d.dataDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to prepare working directory: %w", err)
	}
	return nil
}
