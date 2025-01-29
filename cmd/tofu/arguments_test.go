// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"fmt"
	"os"
	"reflect"
	"slices"
	"testing"

	"github.com/mitchellh/cli"
)

func TestExpandFileBasedArgs(t *testing.T) {
	fixtureDir := "testdata/args-from-files"
	tests := map[string]struct {
		WantExpanded []string
		WantError    string
	}{
		"empty.txt": {
			WantExpanded: []string{},
		},
		"escapes.txt": {
			WantExpanded: []string{"can", "escape spaces", "and\nnewlines", "and", `"double`, `quotes"`, "and", "'single", "quotes'!"},
		},
		"unquoted.txt": {
			WantExpanded: []string{"-foo", "bar", "baz"},
		},
		"multiline.txt": {
			WantExpanded: []string{"bar", "baz", "beep", "boop"},
		},
		"backtick.txt": {
			WantError: `failed to expand "@testdata/args-from-files/backtick.txt" argument: invalid command line string`,
		},
		"cmdsubst.txt": {
			WantError: `failed to expand "@testdata/args-from-files/cmdsubst.txt" argument: invalid command line string`,
		},
		"doublequote.txt": {
			WantExpanded: []string{"beep boop", "foo", "blah blah"},
		},
		"doublequote-envvar-like.txt": {
			WantExpanded: []string{"no $INTERPOLATION inside double quotes"},
		},
		"doublequote-multiline.txt": {
			WantExpanded: []string{"hello\nworld"},
		},
		"doublequote-spaces.txt": {
			WantExpanded: []string{},
		},
		"doublequote-unclosed.txt": {
			WantError: `failed to expand "@testdata/args-from-files/doublequote-unclosed.txt" argument: invalid command line string`,
		},
		"envvar-like.txt": {
			WantExpanded: []string{"we", "$DONT", "expand", "things", "that", "look", "like", "${ENVVAR}interpolations"},
		},
		"parens.txt": {
			// https://github.com/mattn/go-shellwords/issues/54
			WantError: `failed to expand "@testdata/args-from-files/parens.txt" argument: invalid command line string`,
		},
		"parens-quoted.txt": {
			WantExpanded: []string{"it's okay to have (parens) in double quotes", "and (also) in single quotes"},
		},
		"singlequote.txt": {
			// This fails because the upstream library seems to mishandle double quotes inside single quotes
			WantExpanded: []string{"-var=something=foo bar"},
		},
		"singlequote-multiline.txt": {
			WantExpanded: []string{"hello\nworld"},
		},
		"singlequote-spaces.txt": {
			WantExpanded: []string{},
		},
		"singlequote-unclosed.txt": {
			WantError: `failed to expand "@testdata/args-from-files/singlequote-unclosed.txt" argument: invalid command line string`,
		},
		"nonexisting.txt": { // This intentionally refers to a filename that doesn't exist under "testdata/args-from-files"
			// Apparent reference to nonexisting file is taken literally instead
			WantExpanded: []string{"@testdata/args-from-files/nonexisting.txt"},
		},
		"unix-style-path.txt": {
			WantExpanded: []string{"-var-file=../bar/baz.tfvars"},
		},
		"windows-style-path.txt": {
			// The parsing library we use follows Unix-shell-style conventions and so backslashes
			// are treated as escape characters unless in single quotes or escaped by doubling up.
			WantExpanded: []string{
				"-var-file=..barbaz.tfvars",
				`-var-file=..\bar\baz.tfvars`,
				`-var-file=..\bar\baz.tfvars`,
			},
		},
		"metachars.txt": {
			// This test fails because of https://github.com/mattn/go-shellwords/issues/57
			// It currently seems to just halt parsing at the >, which is strange. I'd
			// expect it to either take all of the metacharacters literally or return
			// an error saying that this isn't valid syntax.
			WantExpanded: []string{"not", "a", "shell,", "so", ">we", "take", "<redirection", "metacharacters", "|", "literally", "2>&but", "still", "accept", "them;", "blah"},
		},
		"metachars-singlequote.txt": {
			WantExpanded: []string{"in quotes >we take <redirection metacharacters | literally 2>&and as a normal; part of the overall string"},
		},
	}

	for basename, test := range tests {
		t.Run(basename, func(t *testing.T) {
			// NOTE: Intentionally not filepath.Join here because that would use backslashes
			// on Windows but some of our WantExpanded entries want these strings to be
			// taken literally and need them to be normal slashes.
			filename := fixtureDir + "/" + basename

			// We'll try a few different variations with the expansion argument in
			// different positions with respect to other non-expanding arguments.
			makeTest := func(transform func([]string) []string) func(t *testing.T) {
				return func(t *testing.T) {
					input := transform([]string{"@" + filename})
					got, err := expandFileBasedArgs(input)
					if test.WantError != "" {
						if err == nil {
							t.Errorf("unexpected success; want error: %s", test.WantError)
						} else if gotMsg, wantMsg := err.Error(), test.WantError; gotMsg != wantMsg {
							t.Errorf("wrong error\ngot:  %s\nwant: %s", gotMsg, wantMsg)
						}
						return
					}
					want := make([]string, len(test.WantExpanded))
					copy(want, test.WantExpanded) // copy so that the transform function can't inadvertenly modify the original backing array
					want = transform(want)
					if err != nil {
						t.Fatalf("unexpected error: %s", err)
					}
					if !slices.Equal(got, want) {
						t.Errorf("wrong result\ngot:  %#v\nwant: %#v", got, want)
					}
				}
			}
			t.Run("alone", makeTest(func(s []string) []string {
				return s // just the input alone, without any other arguments
			}))
			t.Run("before", makeTest(func(s []string) []string {
				return append(s, "...")
			}))
			t.Run("after", makeTest(func(s []string) []string {
				return append([]string{"..."}, s...)
			}))
			t.Run("between", makeTest(func(s []string) []string {
				return append(append([]string{"..."}, s...), "...")
			}))
			t.Run("twice around", makeTest(func(s []string) []string {
				return append(append(s, "..."), s...)
			}))
		})
	}

	// We'll also make sure we have a table entry for every file that's in
	// the testdata directory, because it would be unfortunate if we had
	// a test case in there that just got silently ignored.
	allTestFixtures, err := os.ReadDir(fixtureDir)
	if err != nil {
		t.Fatalf("failed to enumerate all test fixture files: %s", err)
	}
	for _, entry := range allTestFixtures {
		if _, ok := tests[entry.Name()]; !ok {
			t.Errorf("no test case for fixture file %q", entry.Name())
		}
	}
}

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
