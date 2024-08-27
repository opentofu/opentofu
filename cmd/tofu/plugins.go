// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"fmt"
	"log"
	"path/filepath"
	"runtime"

	"github.com/terramate-io/opentofulib/internal/command/cliconfig"
)

// globalPluginDirs returns directories that should be searched for
// globally-installed plugins (not specific to the current configuration).
//
// Earlier entries in this slice get priority over later when multiple copies
// of the same plugin version are found, but newer versions always override
// older versions where both satisfy the provider version constraints.
func globalPluginDirs() []string {
	var ret []string
	// Look in ~/.terraform.d/plugins/, $XDG_DATA_HOME/opentofu/plugins, or its equivalent on non-UNIX platforms
	dirs, err := cliconfig.DataDirs()
	if err != nil {
		log.Printf("[ERROR] Error finding global plugin directories: %s", err)
	} else {
		machineDir := fmt.Sprintf("%s_%s", runtime.GOOS, runtime.GOARCH)
		for _, dir := range dirs {
			ret = append(ret, filepath.Join(dir, "plugins"))
			ret = append(ret, filepath.Join(dir, "plugins", machineDir))
		}
	}

	return ret
}
