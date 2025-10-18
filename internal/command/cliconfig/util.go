// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package cliconfig

import (
	"errors"
	"io/fs"
)

func getNewOrLegacyPath(fileSystem fs.FS, newPath string, legacyPath string) (string, error) {
	// If the legacy directory exists, but the new directory does not, then use the legacy directory, for backwards compatibility reasons.
	// Otherwise, use the new directory.
	if _, err := fs.Stat(fileSystem, fsRelativize(legacyPath)); err == nil {
		if _, err := fs.Stat(fileSystem, fsRelativize(newPath)); errors.Is(err, fs.ErrNotExist) {
			return legacyPath, nil
		}
	}

	return newPath, nil
}
