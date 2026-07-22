// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Test represents the command-line arguments for the test command.
type Test struct {
	// Filter contains a list of test files to execute. If empty, all test files
	// will be executed.
	Filter []string

	// TestDirectory allows the user to override the directory that the test
	// command will use to discover test files, defaults to "tests". Regardless
	// of the value here, test files within the configuration directory will
	// always be discovered.
	TestDirectory string

	// ViewOptions specifies which view options to use
	ViewOptions ViewOptions

	// You can specify common variables for all tests from the command line.
	Vars *Vars

	// Verbose tells the test command to print out the plan either in
	// human-readable format or JSON for each run step depending on the
	// ViewType.
	Verbose bool
}

// BindTest registers CLI arguments, returning a Test value and it's corresponding hooks.
func BindTest(flags Flags) (*Test, Hooks) {
	var test Test
	var hooks Hooks

	test.ViewOptions.bind(flags, false)
	hooks = append(hooks, test.ViewOptions.ParseHook())

	test.Vars = &Vars{}
	test.Vars.bind(flags)

	flags.StringArrayVar(&test.Filter, "filter", nil, "If specified, OpenTofu will only execute the test files specified by this flag. You can use this option multiple times to execute more than one test file. The path should be relative to the current working directory, even if -test-directory is set.").SetDisplay("=testfile")
	flags.StringVar(&test.TestDirectory, "test-directory", "tests", `Set the OpenTofu test directory, defaults to "tests". When set, the test command will search for test files in the current directory and in the one specified by the flag.`).SetDisplay("=path")
	flags.BoolVar(&test.Verbose, "verbose", false, "Print the plan or state for each test run block as it executes.")

	return &test, hooks
}

func ParseTest(args []string) (*Test, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	flags := Flags{}
	test, hooks := BindTest(flags)

	cmdFlags := defaultFlagSet("test", flags)

	if err := cmdFlags.Parse(args); err != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to parse command-line flags",
			err.Error()))
	}

	return test, func() { hooks.Post() }, diags.Append(hooks.Pre())
}
