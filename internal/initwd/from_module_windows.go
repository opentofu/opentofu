// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build windows
// +build windows

package initwd

import (
	"fmt"
	"syscall"
)

// expandShortPath is used to convert short paths (or 8.3 legacy paths) to
// long paths. e.g:
// C:\Users\RUNNER~1\AppData\Local becomes:
// C:\Users\runneradmin\AppData\Local
func expandShortPath(shortPath string) (string, error) {
	// Convert the Go string to a null-terminated UTF-16 wide string
	pathPtr, err := syscall.UTF16PtrFromString(shortPath)
	if err != nil {
		return "", fmt.Errorf("Error converting string: %w", err)
	}

	// First call to determine the required buffer size
	// Pass 0 for buffer and buflen to get the required size
	buflen, err := syscall.GetLongPathName(pathPtr, nil, 0)
	if err != nil {
		return "", fmt.Errorf("Error getting buffer size: %w", err)
	}

	// Allocate a buffer of the required size
	buf := make([]uint16, buflen)

	// Second call to get the long path name
	_, err = syscall.GetLongPathName(pathPtr, &buf[0], buflen)
	if err != nil {
		return "", fmt.Errorf("Error getting long path name: %w", err)

	}

	// Convert the UTF-16 wide string back to a Go string
	longPath := syscall.UTF16ToString(buf)

	return longPath, nil
}
