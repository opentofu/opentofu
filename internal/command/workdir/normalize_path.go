// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package workdir

import (
	"path/filepath"
)

// NormalizePath attempts to transform the given path so that it's relative
// to the working directory, which is our preferred way to present and store
// paths to files and directories within a configuration so that they can
// be portable to operations in other working directories.
//
// It isn't always possible to produce a relative path. For example, on Windows
// the given path might be on a different volume (e.g. drive letter or network
// share) than the working directory.
//
// Note that the result will be relative to the main directory of the receiver,
// which should always be the actual process working directory in normal code,
// but might be some other temporary working directory when in test code.
// If you need to access the file or directory that the result refers to with
// functions that aren't aware of our base directory, you can use something
// like the following, which again should be needed only in test code which
// might need to inspect the filesystem in order to make assertions:
//
//	filepath.Join(d.RootModuleDir(), normalizePathResult)
//
// The above is suitable only for situations where the given path is known
// to be beneath the working directory, which is the typical situation for
// temporary working directories created for automated tests.
func (d *Dir) NormalizePath(given string) string {
	// We need an absolute version of d.mainDir in order for our "Rel"
	// result to be reliable.
	absMain, err := filepath.Abs(d.mainDir)
	if err != nil {
		// Weird, but okay...
		return filepath.Clean(given)
	}

	// filepath.Abs relies on os.Getwd, which on Unix prefers the $PWD
	// environment variable when it names the current directory. If OpenTofu
	// was launched from a directory reached through a symlink, $PWD keeps the
	// symlinked path while the kernel still resolves relative paths against
	// the real directory. Computing a relative path against the symlinked
	// path then yields a result that no longer resolves to the intended file
	// (e.g. "tofu fmt /abs/path" failing with "No file or directory"). We
	// canonicalize the main directory so the relative result stays in sync
	// with how the filesystem actually resolves it.
	// See https://github.com/opentofu/opentofu/issues/3879.
	if resolved, err := filepath.EvalSymlinks(absMain); err == nil {
		absMain = resolved
	}

	if !filepath.IsAbs(given) {
		given = filepath.Join(absMain, given)
	}

	ret, err := filepath.Rel(absMain, given)
	if err != nil {
		// It's not always possible to find a relative path. For example,
		// the given path might be on an entirely separate volume
		// (e.g. drive letter or network share) on a Windows system, which
		// always requires an absolute path.
		return filepath.Clean(given)
	}

	return ret
}
