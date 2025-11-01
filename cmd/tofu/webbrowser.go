// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"github.com/opentofu/opentofu/internal/command/webbrowser"
)

// browserLauncher implements the policy for deciding how to launch a web
// browser in the current execution environment.
func browserLauncher() webbrowser.Launcher {
	if envLauncher := browserLauncherFromEnv(); envLauncher != nil {
		return envLauncher
	}
	return webbrowser.NewNativeLauncher()
}
