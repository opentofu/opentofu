// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build windows
// +build windows

package cliconfig

import (
	"os"
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

	newConfigDir := filepath.Join(dir, "opentf.d")
	legacyConfigDir := filepath.Join(dir, "terraform.d")

	return getNewOrLegacyPath(newConfigDir, legacyConfigDir)
}

func homeDir() (string, error) {
	// For unit-testing purposes.
	if home := os.Getenv("OPENTF_TEST_HOME"); home != "" {
		return home, nil
	}

	b := make([]uint16, syscall.MAX_PATH)

	// See: http://msdn.microsoft.com/en-us/library/windows/desktop/bb762181(v=vs.85).aspx
	r, _, err := getFolderPath.Call(0, CSIDL_APPDATA, 0, 0, uintptr(unsafe.Pointer(&b[0])))
	if uint32(r) != 0 {
		return "", err
	}

	return syscall.UTF16ToString(b), nil
}
