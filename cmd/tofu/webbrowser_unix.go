// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build unix

package main

import (
	"os"

	"github.com/opentofu/opentofu/internal/command/webbrowser"
)

func browserLauncherFromEnv() webbrowser.Launcher {
	// On Unix systems we honor the de-facto standard BROWSER environment
	// variable in its original, simpler form where it was required to refer
	// only to a single command to run with the URL to open as the first
	// and only argument.
	//
	// There's information on this convention in Debian's documentation,
	// although this is not a Debian-specific mechanism:
	//     https://wiki.debian.org/DefaultWebBrowser#BROWSER_environment_variable

	execPath := webbrowser.ParseBrowserEnv(os.Getenv("BROWSER"))
	if execPath != "" {
		return webbrowser.NewExecLauncher(execPath)
	}
	return nil
}
