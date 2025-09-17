// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build !windows
// +build !windows

package initwd

// expandShortPath is a no-op on Unix.
func expandShortPath(shortPath string) (string, error) {
	return shortPath, nil
}
