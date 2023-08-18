// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build windows
// +build windows

package cliconfig

import (
	"path/filepath"
	"syscall"
	"unsafe"
)

var (
	shell         = syscall.MustLoadDLL("Shell32.dll")
	getFolderPath = shell.MustFindProc("SHGetFolderPathW")
)

const CSIDL_APPDATA = 26

func configFile() (string, error) {
	dir, err := homeDir()
	if err != nil {
		return "", err
	}

	newConfigFile := filepath.Join(dir, "terraform.rc")
	oldConfigFile := filepath.Join(dir, "opentf.rc")

	return getNewOrLegacyPath(newConfigFile, oldConfigFile)
}

func configDir() (string, error) {
	dir, err := homeDir()
	if err != nil {
		return "", err
	}

	newConfigDir := filepath.Join(dir, ".opentf.d")
	legacyConfigDir := filepath.Join(dir, ".terraform.d")

	// If the legacy directory exists, but the new directory does not, then use the legacy directory, for backwards compatibility reasons.
	// Otherwise, use the new directory.
	if _, err := os.Stat(legacyConfigDir); err == nil {
		if _, err := os.Stat(newConfigDir); os.IsNotExist(err) {
			return legacyConfigDir, nil
		}
	}

	return newConfigDir, nil
}

func homeDir() (string, error) {
	b := make([]uint16, syscall.MAX_PATH)

	// See: http://msdn.microsoft.com/en-us/library/windows/desktop/bb762181(v=vs.85).aspx
	r, _, err := getFolderPath.Call(0, CSIDL_APPDATA, 0, 0, uintptr(unsafe.Pointer(&b[0])))
	if uint32(r) != 0 {
		return "", err
	}

	return syscall.UTF16ToString(b), nil
}
