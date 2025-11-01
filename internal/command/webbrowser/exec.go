// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package webbrowser

import (
	"fmt"
	"os"
	"os/exec"
)

// NewExecLauncher creates and returns a Launcher that just attempts to run
// the executable at the given path, with the given URL as its first and
// only argument.
//
// The given path must be ready to use, without reference to the PATH
// environment variable. The caller can use [exec.LookPath] to prepare
// a suitable path if searching PATH is appropriate.
//
// This is intended to allow overriding which browser to use using the
// BROWSER environment variable on Unix-like systems, but the rules for
// that are in "package main". [ParseBrowserEnv] implements parsing of the
// value of that environment variable when the main package decides it's
// appropriate to do so.
func NewExecLauncher(execPath string) Launcher {
	return execLauncher{
		execPath: execPath,
	}
}

type execLauncher struct {
	execPath string
}

func (l execLauncher) OpenURL(url string) error {
	cmd := &exec.Cmd{
		Path: l.execPath,
		Args: []string{l.execPath, url},
		Env:  os.Environ(),
	}
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("%s: %w", l.execPath, err)
	}
	return nil
}

// ParseBrowserEnv takes the raw value of a BROWSER environment variable and
// attempts to parse it as a reference to an executable, whose absolute
// path is returned if successful. Returns an empty string if the value cannot
// be interpreted as an executable to run.
//
// This implements the simple form of this environment variable commonly used
// by software on Unix-like systems, where the value must be literally just
// a command to run whose first and only argument would be the URL to open.
//
// It does NOT support the more complex interpretation of that environment
// variable that was proposed at http://www.catb.org/~esr/BROWSER/ , because
// that form has not been widely implemented and the implementations that
// exist do not have consistent behavior due to the proposal being
// ambiguous.
//
// Callers that use this should typically pass a successful result to
// [NewExecLauncher] to use the resolved command as a browser launcher. The
// caller is responsible for deciding the policy for whether to consider a
// BROWSER environment variable and for accessing the environment table to
// obtain its value.
func ParseBrowserEnv(raw string) string {
	if raw == "" {
		return "" // empty is treated the same as unset
	}

	execPath, err := exec.LookPath(raw)
	if err != nil {
		// We silently ignore variable values we cannot use, because this
		// environment variable is not OpenTofu-specific and so it may have
		// been set for the benefit of software other than OpenTofu which
		// interprets it differently.
		return ""
	}
	return execPath
}
