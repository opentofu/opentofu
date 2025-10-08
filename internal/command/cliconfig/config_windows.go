// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build windows
// +build windows

package cliconfig

import (
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var (
	shell         = syscall.MustLoadDLL("Shell32.dll")
	getFolderPath = shell.MustFindProc("SHGetFolderPathW")
)

const CSIDL_APPDATA = 26
const SYSTEM_DRIVE = "SystemDrive"

func rootFileSystem() fs.FS {
	// https://learn.microsoft.com/en-us/windows/deployment/usmt/usmt-recognized-environment-variables
	// Note that this will resolve to "C:", not "C:\"
	drive := os.Getenv(SYSTEM_DRIVE)
	return os.DirFS(drive + string(os.PathSeparator))
}

func configFile(fileSystem fs.FS) (string, error) {
	dir, err := homeDir()
	if err != nil {
		return "", err
	}

	newConfigFile := filepath.Join(dir, "tofu.rc")
	legacyConfigFile := filepath.Join(dir, "terraform.rc")

	return getNewOrLegacyPath(fileSystem, newConfigFile, legacyConfigFile)
}

func configDir(_ fs.FS) (string, error) {
	dir, err := homeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "terraform.d"), nil
}

func dataDirs(fileSystem fs.FS) ([]string, error) {
	dir, err := configDir(fileSystem)
	if err != nil {
		return nil, err
	}
	return []string{dir}, nil
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

// fsRelativize removes the leading drive letter and trailing backslash from the file path. The fs.FS filesystem type only works with
// "relative directories". So, a DirFS based at "C:\" will take a file path like "Users/username/tofu.rc" and
// look in the operating system file system at "C:\Users\username\tofu.rc".
// Note the slashes: fs.FS does not accept a backslash as an acceptable path separator.
// More details in this documentation: https://pkg.go.dev/io/fs#ValidPath
func fsRelativize(dir string) string {
	if dir == "" {
		return ""
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		log.Printf("[WARNING] Attempted to form absolute representation of relative path \"%s\", but ran into an error: %v", dir, err)
	}
	systemDrive := strings.ToUpper(os.Getenv(SYSTEM_DRIVE))
	dirVolume := strings.ToUpper(filepath.VolumeName(absDir))
	if systemDrive != dirVolume {
		log.Printf("[WARNING] System Drive %s and file location %s are in different drives, and will not look up correctly on a relativized file system", systemDrive, dirVolume)
	}
	// https://learn.microsoft.com/en-us/windows/deployment/usmt/usmt-recognized-environment-variables
	// Note that this will resolve to "C:", not "C:\"
	// We trim both the upper- and lower-case drive, just in case the absolute filepath provides one or the other (drive letters are case-insensitive)
	driveRemovedPath := strings.TrimPrefix(strings.TrimPrefix(absDir, systemDrive), strings.ToLower(systemDrive))
	return filepath.ToSlash(strings.Trim(driveRemovedPath, string(os.PathSeparator)))
}
