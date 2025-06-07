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
	"testing"

	"github.com/mitchellh/cli"
)

func TestMain_cliArgsFromEnv(t *testing.T) {
	// Set up the state. This test really messes with the environment and
	// global state so we set things up to be restored.

	// Restore original CLI args
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	// Set up test command and restore that
	commands = make(map[string]cli.CommandFactory)
	defer func() {
		commands = nil
	}()
	testCommandName := "unit-test-cli-args"
	testCommand := &testCommandCLI{}
	commands[testCommandName] = func() (cli.Command, error) {
		return testCommand, nil
	}

	cases := []struct {
		Name     string
		Args     []string
		Value    string
		Expected []string
		Err      bool
	}{
		{
			"no env",
			[]string{testCommandName, "foo", "bar"},
			"",
			[]string{"foo", "bar"},
			false,
		},

		{
			"both env var and CLI",
			[]string{testCommandName, "foo", "bar"},
			"-foo baz",
			[]string{"-foo", "baz", "foo", "bar"},
			false,
		},

		{
			"only env var",
			[]string{testCommandName},
			"-foo bar",
			[]string{"-foo", "bar"},
			false,
		},

		{
			"cli string has blank values",
			[]string{testCommandName, "bar", "", "baz"},
			"-foo bar",
			[]string{"-foo", "bar", "bar", "", "baz"},
			false,
		},

		{
			"cli string has blank values before the command",
			[]string{"", testCommandName, "bar"},
			"-foo bar",
			[]string{"-foo", "bar", "bar"},
			false,
		},

		{
			// this should fail gracefully, this is just testing
			// that we don't panic with our slice arithmetic
			"no command",
			[]string{},
			"-foo bar",
			nil,
			true,
		},

		{
			"single quoted strings",
			[]string{testCommandName, "foo"},
			"-foo 'bar baz'",
			[]string{"-foo", "bar baz", "foo"},
			false,
		},

		{
			"double quoted strings",
			[]string{testCommandName, "foo"},
			`-foo "bar baz"`,
			[]string{"-foo", "bar baz", "foo"},
			false,
		},

		{
			"double quoted single quoted strings",
			[]string{testCommandName, "foo"},
			`-foo "'bar baz'"`,
			[]string{"-foo", "'bar baz'", "foo"},
			false,
		},
	}

	for i, tc := range cases {
		t.Run(fmt.Sprintf("%d-%s", i, tc.Name), func(t *testing.T) {
			// Set the env var value
			if tc.Value != "" {
				t.Setenv(EnvCLI, tc.Value)
			}

			// Set up the args
			args := make([]string, len(tc.Args)+1)
			args[0] = oldArgs[0] // process name
			copy(args[1:], tc.Args)

			// Run it!
			os.Args = args
			testCommand.Args = nil
			exit := realMain()
			if (exit != 0) != tc.Err {
				t.Fatalf("bad: %d", exit)
			}
			if tc.Err {
				return
			}

			// Verify
			if !reflect.DeepEqual(testCommand.Args, tc.Expected) {
				t.Fatalf("expected args %#v but got %#v", tc.Expected, testCommand.Args)
			}
		})
	}
}

// This test just has more options than the test above. Use this for
// more control over behavior at the expense of more complex test structures.
func TestMain_cliArgsFromEnvAdvanced(t *testing.T) {
	// Restore original CLI args
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	// Set up test command and restore that
	commands = make(map[string]cli.CommandFactory)
	defer func() {
		commands = nil
	}()

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
			"targeted to another command",
			"command",
			EnvCLI + "_foo",
			[]string{"command", "foo", "bar"},
			"-flag",
			[]string{"foo", "bar"},
			false,
		},

		{
			"targeted to this command",
			"command",
			EnvCLI + "_command",
			[]string{"command", "foo", "bar"},
			"-flag",
			[]string{"-flag", "foo", "bar"},
			false,
		},

		{
			"targeted to a command with a hyphen",
			"command-name",
			EnvCLI + "_command_name",
			[]string{"command-name", "foo", "bar"},
			"-flag",
			[]string{"-flag", "foo", "bar"},
			false,
		},

		{
			"targeted to a command with a space",
			"command name",
			EnvCLI + "_command_name",
			[]string{"command", "name", "foo", "bar"},
			"-flag",
			[]string{"-flag", "foo", "bar"},
			false,
		},
	}

	for i, tc := range cases {
		t.Run(fmt.Sprintf("%d-%s", i, tc.Name), func(t *testing.T) {
			// Set up test command and restore that
			testCommandName := tc.Command
			testCommand := &testCommandCLI{}
			defer func() { delete(commands, testCommandName) }()
			commands[testCommandName] = func() (cli.Command, error) {
				return testCommand, nil
			}

			// Set the env var value
			if tc.Value != "" {
				t.Setenv(tc.EnvVar, tc.Value)
			}

			// Set up the args
			args := make([]string, len(tc.Args)+1)
			args[0] = oldArgs[0] // process name
			copy(args[1:], tc.Args)

			// Run it!
			os.Args = args
			testCommand.Args = nil
			exit := realMain()
			if (exit != 0) != tc.Err {
				t.Fatalf("unexpected exit status %d; want 0", exit)
			}
			if tc.Err {
				return
			}

			// Verify
			if !reflect.DeepEqual(testCommand.Args, tc.Expected) {
				t.Fatalf("bad: %#v", testCommand.Args)
			}
		})
	}
}

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
