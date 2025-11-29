// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package cliconfig

import "os"

func getNewOrLegacyPath(cl *ConfigLoader, newPath string, legacyPath string) (string, error) {
	// If the legacy directory exists, but the new directory does not, then use the legacy directory, for backwards compatibility reasons.
	// Otherwise, use the new directory.
	if _, err := cl.Stat(legacyPath); err == nil {
		if _, err := cl.Stat(newPath); os.IsNotExist(err) {
			return legacyPath, nil
		}
	}

	return newPath, nil
}
