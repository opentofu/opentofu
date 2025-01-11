// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package testprovider

import (
	"context"
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strings"
	"testing"
	"time"
)

//go:embed data/*
var embedFS embed.FS

// Go builds a key provider as a Go binary and returns its path.
// This binary will always return []byte{1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16} as a hard-coded key.
// You may pass --hello-world to change it to []byte("Hello world! 123")
func Go(t *testing.T) []string {
	// goMod is embedded like this because the go:embed tag doesn't like having module files in embedded paths.
	var goMod = []byte(`module testprovider

go 1.22`)

	tempDir := t.TempDir()
	dir := path.Join(tempDir, "testprovider-go")
	if err := os.MkdirAll(dir, 0700); err != nil { //nolint:mnd // This check is stupid
		t.Errorf("Failed to create temporary directory (%v)", err)
	}

	if err := os.WriteFile(path.Join(dir, "go.mod"), goMod, 0600); err != nil { //nolint:mnd // This check is stupid
		t.Errorf("%v", err)
	}
	if err := ejectFile("testprovider.go", path.Join(dir, "testprovider.go")); err != nil {
		t.Errorf("%v", err)
	}
	targetBinary := path.Join(dir, "testprovider")
	if runtime.GOOS == "windows" {
		targetBinary += ".exe"
	}
	t.Logf("\033[32mCompiling test provider binary...\033[0m")
	cmd := exec.Command("go", "build", "-o", targetBinary)
	cmd.Dir = dir
	// TODO move this to a proper test logger once available.
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Skipf("Failed to build test provider binary (%v)", err)
	}
	return []string{targetBinary}
}

// Python returns the path to a Python script acting as a key provider. The function returns all arguments required to
// run the Python script, including the Python interpreter.
// This script will always return []byte{1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16} as a hard-coded key.
func Python(t *testing.T) []string {
	tempDir := t.TempDir()
	dir := path.Join(tempDir, "testprovider-py")
	if err := os.MkdirAll(dir, 0700); err != nil { //nolint:mnd // This check is stupid
		t.Errorf("Failed to create temporary directory (%v)", err)
	}
	target := path.Join(dir, "testprovider.py")
	if err := ejectFile("testprovider.py", target); err != nil {
		t.Errorf("%v", err)
	}
	python := findExecutable(t, []string{"python", "python3"}, []string{"--version"})
	return []string{python, target}
}

// POSIXShell returns a path to a POSIX shell script acting as a key provider.
// This script will always return []byte{1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16} as a hard-coded key.
func POSIXShell(t *testing.T) []string {
	tempDir := t.TempDir()
	dir := path.Join(tempDir, "testprovider-sh")
	if err := os.MkdirAll(dir, 0700); err != nil { //nolint:mnd // This check is stupid
		t.Errorf("Failed to create temporary directory (%v)", err)
	}
	target := path.Join(dir, "testprovider.sh")
	if err := ejectFile("testprovider.sh", target); err != nil {
		t.Errorf("%v", err)
	}
	sh := findExecutable(t, []string{"sh", "/bin/sh", "/usr/bin/sh"}, []string{"-c", "echo \"Hello world!\""})
	return []string{sh, target}
}

func ejectFile(file string, target string) error {
	contents, err := embedFS.ReadFile(path.Join("data", file))
	if err != nil {
		return fmt.Errorf("failed to read %s file from embedded dataset (%w)", file, err)
	}
	if err := os.WriteFile(target, contents, 0600); err != nil { //nolint:mnd // This check is stupid
		return fmt.Errorf("failed to create %s file at %s (%w)", file, target, err)
	}
	return nil
}

func findExecutable(t *testing.T, options []string, testArguments []string) string {
	for _, opt := range options {
		var lastError error
		func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			cmd := exec.CommandContext(ctx, opt, testArguments...)
			lastError = cmd.Run()
		}()
		if lastError == nil {
			return opt
		}
	}
	t.Skipf("No viable alternative found between %s", strings.Join(options, ", "))
	return ""
}
