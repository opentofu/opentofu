// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package testmethod

import (
	"context"
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

//go:embed data/*
var embedFS embed.FS

// Go builds a key provider as a Go binary and returns its path.
func Go(t *testing.T) []string {
	t.Helper()

	// goMod is embedded like this because the go:embed tag doesn't like having module files in embedded paths.
	var goMod = []byte(`module testmethod

go 1.22`)

	tempDir := t.TempDir()
	dir := path.Join(tempDir, "testmethod-go")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Errorf("Failed to create temporary directory (%v)", err)
	}

	if err := os.WriteFile(path.Join(dir, "go.mod"), goMod, 0600); err != nil {
		t.Errorf("%v", err)
	}
	if err := ejectFile("testmethod.go", path.Join(dir, "testmethod.go")); err != nil {
		t.Errorf("%v", err)
	}
	targetBinary := filepath.FromSlash(path.Join(dir, "testmethod"))
	if runtime.GOOS == "windows" {
		targetBinary += ".exe"
		// If we do not use raw backslashes below, the binary won't be found by HCL after parsing.
		targetBinary = strings.ReplaceAll(targetBinary, "\\", `\\`)
	}
	t.Logf("\033[32mCompiling test method binary...\033[0m")
	cmd := exec.Command("go", "build", "-o", targetBinary)
	cmd.Dir = dir
	// TODO move this to a proper test logger once available.
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Skipf("Failed to build test method binary (%v)", err)
	}
	return []string{targetBinary}
}

// Python returns the path to a Python script acting as an encryption method. The function returns all arguments
// required to run the Python script, including the Python interpreter.
func Python(t *testing.T) []string {
	t.Helper()

	tempDir := t.TempDir()
	dir := path.Join(tempDir, "testmethod-py")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Errorf("Failed to create temporary directory (%v)", err)
	}
	target := path.Join(dir, "testmethod.py")
	if runtime.GOOS == "windows" {
		// If we do not use raw backslashes below, the binary won't be found by HCL after parsing.
		target = strings.ReplaceAll(target, "\\", `\\`)
	}
	if err := ejectFile("testmethod.py", target); err != nil {
		t.Errorf("%v", err)
	}
	python := findExecutable(t, []string{"python", "python3"}, []string{"--version"})

	return []string{python, target}
}

func ejectFile(file string, target string) error {
	contents, err := embedFS.ReadFile(path.Join("data", file))
	if err != nil {
		return fmt.Errorf("failed to read %s file from embedded dataset (%w)", file, err)
	}
	if err := os.WriteFile(target, contents, 0600); err != nil {
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
