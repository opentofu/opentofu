// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build windows

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

func (cl *ConfigLoader) configFile() (string, error) {
	dir, err := cl.homeDir()
	if err != nil {
		return "", err
	}

	newConfigFile := filepath.Join(dir, "tofu.rc")
	legacyConfigFile := filepath.Join(dir, "terraform.rc")

	return getNewOrLegacyPath(cl, newConfigFile, legacyConfigFile)
}

func (cl *ConfigLoader) configDir() (string, error) {
	dir, err := cl.homeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "terraform.d"), nil
}

func (cl *ConfigLoader) dataDirs() ([]string, error) {
	dir, err := cl.configDir()
	if err != nil {
		return nil, err
	}
	return []string{dir}, nil
}

func (cl *ConfigLoader) homeDir() (string, error) {
	b := make([]uint16, syscall.MAX_PATH)

	// See: http://msdn.microsoft.com/en-us/library/windows/desktop/bb762181(v=vs.85).aspx
	r, _, err := getFolderPath.Call(0, CSIDL_APPDATA, 0, 0, uintptr(unsafe.Pointer(&b[0])))
	if uint32(r) != 0 {
		return "", err
	}

	return syscall.UTF16ToString(b), nil
}
