// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestMain_mergeEnvArgs(t *testing.T) {
	testCommandName := "test-command-name"

	cases := []struct {
		Name     string
		Command  string
		EnvVar   string
		Args     []string
		Value    string
		Expected []string
		Err      bool
	}{
		{
			Name:     "no env",
			Command:  testCommandName,
			Args:     []string{testCommandName, "foo", "bar"},
			Value:    "",
			Expected: []string{testCommandName, "foo", "bar"},
		},

		{
			Name:     "both env var and CLI",
			Command:  testCommandName,
			Args:     []string{testCommandName, "foo", "bar"},
			Value:    "-foo baz",
			Expected: []string{testCommandName, "-foo", "baz", "foo", "bar"},
		},

		{
			Name:     "only env var",
			Command:  testCommandName,
			Args:     []string{testCommandName},
			Value:    "-foo bar",
			Expected: []string{testCommandName, "-foo", "bar"},
		},

		{
			Name:     "cli string has blank values",
			Command:  testCommandName,
			Args:     []string{testCommandName, "bar", "", "baz"},
			Value:    "-foo bar",
			Expected: []string{testCommandName, "-foo", "bar", "bar", "", "baz"},
		},

		{
			Name:     "cli string has blank values before the command",
			Command:  testCommandName,
			Args:     []string{"", testCommandName, "bar"},
			Value:    "-foo bar",
			Expected: []string{"", testCommandName, "-foo", "bar", "bar"},
		},

		{
			// this should fail gracefully, this is just testing
			// that we don't panic with our slice arithmetic
			Name:     "no command",
			Command:  "",
			Args:     []string{},
			Value:    "-foo bar",
			Expected: []string{"-foo", "bar"},
			Err:      true,
		},

		{
			Name:     "single quoted strings",
			Command:  testCommandName,
			Args:     []string{testCommandName, "foo"},
			Value:    "-foo 'bar baz'",
			Expected: []string{testCommandName, "-foo", "bar baz", "foo"},
		},

		{
			Name:     "double quoted strings",
			Command:  testCommandName,
			Args:     []string{testCommandName, "foo"},
			Value:    `-foo "bar baz"`,
			Expected: []string{testCommandName, "-foo", "bar baz", "foo"},
		},

		{
			Name:     "double quoted single quoted strings",
			Command:  testCommandName,
			Args:     []string{testCommandName, "foo"},
			Value:    `-foo "'bar baz'"`,
			Expected: []string{testCommandName, "-foo", "'bar baz'", "foo"},
		},
		{
			Name:     "targeted to another command",
			Command:  "command",
			EnvVar:   EnvCLI + "_foo",
			Args:     []string{"command", "foo", "bar"},
			Value:    "-flag",
			Expected: []string{"command", "foo", "bar"},
		},

		{
			Name:     "targeted to this command",
			Command:  "command",
			EnvVar:   EnvCLI + "_command",
			Args:     []string{"command", "foo", "bar"},
			Value:    "-flag",
			Expected: []string{"command", "-flag", "foo", "bar"},
		},

		{
			Name:     "targeted to a command with a hyphen",
			Command:  "command-name",
			EnvVar:   EnvCLI + "_command_name",
			Args:     []string{"command-name", "foo", "bar"},
			Value:    "-flag",
			Expected: []string{"command-name", "-flag", "foo", "bar"},
		},

		{
			Name:     "targeted to a command with a space",
			Command:  "command name",
			EnvVar:   EnvCLI + "_command_name",
			Args:     []string{"command", "name", "foo", "bar"},
			Value:    "-flag",
			Expected: []string{"command", "name", "-flag", "foo", "bar"},
		},
	}

	for i, tc := range cases {
		t.Run(fmt.Sprintf("%d-%s", i, tc.Name), func(t *testing.T) {
			// Set the env var value
			if tc.Value != "" {
				if tc.EnvVar == "" {
					t.Setenv(EnvCLI, tc.Value)
				} else {
					t.Setenv(tc.EnvVar, tc.Value)
				}
			}

			// Set up the args
			args := make([]string, len(tc.Args)+1)
			args[0] = "tofu" // process name
			copy(args[1:], tc.Args)

			// Run it!
			{
				subcommand := tc.Command
				if subcommand == "" {
					subcommand = args[0]
				}
				var err error
				args, err = mergeEnvArgs(EnvCLI, subcommand, args)
				if err != nil {
					t.Fatal(err.Error())
				}

				// Prefix the args with any args from the EnvCLI targeting this command
				suffix := strings.ReplaceAll(strings.ReplaceAll(
					subcommand, "-", "_"), " ", "_")
				args, err = mergeEnvArgs(
					fmt.Sprintf("%s_%s", EnvCLI, suffix), subcommand, args)
				if err != nil {
					t.Fatal(err.Error())
				}
			}

			// Verify
			args = args[1:]
			if !reflect.DeepEqual(args, tc.Expected) {
				t.Fatalf("expected args %#v but got %#v", tc.Expected, args)
			}
		})
	}
}

// verify that we output valid autocomplete results
func TestMain_autoComplete(t *testing.T) {
	// Restore original CLI args
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	t.Setenv("COMP_LINE", "tofu versio")

	// Run it!
	os.Args = []string{"tofu", "tofu", "versio"}
	exit := realMain()
	if exit != 0 {
		t.Fatalf("unexpected exit status %d; want 0", exit)
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
