// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/mitchellh/cli"
)

// verify that we output valid autocomplete results
func TestMain_autoComplete(t *testing.T) {
	// Restore original CLI args
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	// Set up test command and restore that
	commands = make(map[string]cli.CommandFactory)
	defer func() {
		commands = nil
	}()

	// Set up test command and restore that
	commands["foo"] = func() (cli.Command, error) {
		return &testCommandCLI{}, nil
	}

	t.Setenv("COMP_LINE", "tofu versio")

	// Run it!
	os.Args = []string{"tofu", "tofu", "versio"}
	exit := realMain()
	if exit != 0 {
		t.Fatalf("unexpected exit status %d; want 0", exit)
	}
}

type testCommandCLI struct {
	Args []string
}

func (c *testCommandCLI) Run(args []string) int {
	c.Args = args
	return 0
}

func (c *testCommandCLI) Synopsis() string { return "" }
func (c *testCommandCLI) Help() string     { return "" }

func TestWarnOutput(t *testing.T) {
	mock := cli.NewMockUi()
	wrapped := &ui{mock}
	wrapped.Warn("WARNING")

	stderr := mock.ErrorWriter.String()
	stdout := mock.OutputWriter.String()

	if stderr != "" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}

	if stdout != "WARNING\n" {
		t.Fatalf("unexpected stdout: %q\n", stdout)
	}
}

func TestMkConfigDir_new(t *testing.T) {
	tmpConfigDir := filepath.Join(t.TempDir(), ".terraform.d")

	err := mkConfigDir(tmpConfigDir)
	if err != nil {
		t.Fatalf("Failed to create the new config directory: %v", err)
	}

	info, err := os.Stat(tmpConfigDir)
	if err != nil {
		t.Fatalf("Directory does not exist after creation: %v", err)
	}

	if !info.IsDir() {
		t.Fatalf("%s should be a directory but it's not", tmpConfigDir)
	}

	mode := int(info.Mode().Perm())
	expectedMode := 0755
	// Unix permissions bits are not applicable on Windows. Perm() returns
	// 0777 regardless of whether readonly or hidden flags are set.
	if runtime.GOOS == "windows" {
		expectedMode = 0777
	}
	if mode != expectedMode {
		t.Fatalf("Expected mode: %04o, but got: %04o", expectedMode, mode)
	}
}

func TestMkConfigDir_exists(t *testing.T) {
	tmpConfigDir := filepath.Join(t.TempDir(), ".terraform.d")
	os.Mkdir(tmpConfigDir, os.ModePerm)

	err := mkConfigDir(tmpConfigDir)
	if err != nil {
		t.Fatalf("Failed to create the new config directory: %v", err)
	}

	_, err = os.Stat(tmpConfigDir)
	if err != nil {
		t.Fatalf("Directory does not exist after creation: %v", err)
	}
}

func TestMkConfigDir_noparent(t *testing.T) {
	tmpConfigDir := filepath.Join(t.TempDir(), "nonexistenthomedir", ".terraform.d")

	err := mkConfigDir(tmpConfigDir)
	if err == nil {
		t.Fatal("Expected an error, but got none")
	}

	// We wouldn't dare creating the home dir. If the parent of our config dir
	// is missing, it's likely an issue with the system.
	expectedError := fmt.Sprintf("mkdir %s: no such file or directory", tmpConfigDir)
	if runtime.GOOS == "windows" {
		expectedError = fmt.Sprintf("mkdir %s: The system cannot find the path specified.", tmpConfigDir)
	}
	if err.Error() != expectedError {
		t.Fatalf("Expected error: %s, but got: %v", expectedError, err)
	}
}
