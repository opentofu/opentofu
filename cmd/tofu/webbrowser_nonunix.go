// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build !unix

package main

import (
	"github.com/opentofu/opentofu/internal/command/webbrowser"
)

func browserLauncherFromEnv() webbrowser.Launcher {
	// We know of no environment variable convention for the current platform.
	return nil
}
