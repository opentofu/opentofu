// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/mitchellh/cli"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	testing_command "github.com/opentofu/opentofu/internal/command/testing"
	"github.com/opentofu/opentofu/internal/command/views"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/terminal"
)

func TestTest(t *testing.T) {
	tcs := map[string]struct {
		override string
		args     []string
		expected string
		code     int
		skip     bool
	}{
		"function_call_in_variables": {
			expected: "1 passed, 0 failed.",
			code:     0,
		},
		"simple_pass": {
			expected: "1 passed, 0 failed.",
			code:     0,
		},
		"simple_pass_nested": {
			expected: "1 passed, 0 failed.",
			code:     0,
		},
		"simple_pass_nested_alternate": {
			args:     []string{"-test-directory", "other"},
			expected: "1 passed, 0 failed.",
			code:     0,
		},
		"simple_pass_very_nested": {
			args:     []string{"-test-directory", "tests/subdir"},
			expected: "1 passed, 0 failed.",
			code:     0,
		},
		"simple_pass_very_nested_alternate": {
			override: "simple_pass_very_nested",
			args:     []string{"-test-directory", "./tests/subdir"},
			expected: "1 passed, 0 failed.",
			code:     0,
		},
		"pass_with_locals": {
			expected: "1 passed, 0 failed.",
			code:     0,
		},
		"pass_with_outputs": {
			expected: "1 passed, 0 failed.",
			code:     0,
		},
		"pass_with_variables": {
			expected: "2 passed, 0 failed.",
			code:     0,
		},
		"plan_then_apply": {
			expected: "2 passed, 0 failed.",
			code:     0,
		},
		"expect_failures_checks": {
			expected: "1 passed, 0 failed.",
			code:     0,
		},
		"expect_failures_inputs": {
			expected: "1 passed, 0 failed.",
			code:     0,
		},
		"expect_failures_outputs": {
			expected: "1 passed, 0 failed.",
			code:     0,
		},
		"expect_runtime_check_fail": {
			expected: "0 passed, 1 failed.",
			code:     1,
		},
		"expect_runtime_check_pass_with_expect": {
			expected: "1 passed, 0 failed.",
			code:     0,
		},
		"expect_runtime_check_pass_command_plan_expected": {
			expected: "1 passed, 0 failed.",
			code:     0,
		},
		"expect_runtime_check_fail_command_plan": {
			expected: "0 passed, 1 failed.",
			code:     1,
		},
		"expect_failures_resources": {
			expected: "1 passed, 0 failed.",
			code:     0,
		},
		"multiple_files": {
			expected: "2 passed, 0 failed",
			code:     0,
		},
		"multiple_files_with_filter": {
			override: "multiple_files",
			args:     []string{"-filter=one.tftest.hcl"},
			expected: "1 passed, 0 failed",
			code:     0,
		},
		"variables": {
			expected: "2 passed, 0 failed",
			code:     0,
		},
		"variables_overridden": {
			override: "variables",
			args:     []string{"-var=input=foo"},
			expected: "1 passed, 1 failed",
			code:     1,
		},
		"simple_fail": {
			expected: "0 passed, 1 failed.",
			code:     1,
		},
		"custom_condition_checks": {
			expected: "0 passed, 1 failed.",
			code:     1,
		},
		"custom_condition_inputs": {
			expected: "0 passed, 1 failed.",
			code:     1,
		},
		"custom_condition_outputs": {
			expected: "0 passed, 1 failed.",
			code:     1,
		},
		"custom_condition_resources": {
			expected: "0 passed, 1 failed.",
			code:     1,
		},
		"no_providers_in_main": {
			expected: "1 passed, 0 failed",
			code:     0,
		},
		"default_variables": {
			expected: "1 passed, 0 failed.",
			code:     0,
		},
		"undefined_variables": {
			expected: "1 passed, 0 failed.",
			code:     0,
		},
		"refresh_only": {
			expected: "3 passed, 0 failed.",
			code:     0,
		},
		"null_output": {
			expected: "1 passed, 0 failed.",
			code:     0,
		},
		"pass_with_tests_dir_variables": {
			expected: "1 passed, 0 failed.",
			code:     0,
		},
		"override_with_tests_dir_variables": {
			expected: "1 passed, 0 failed.",
			code:     0,
		},
		// New variables introduced in the test file should error out
		"local_variables_in_provider_block": {
			code: 1,
		},
	}
	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			if tc.skip {
				t.Skip()
			}

			file := name
			if len(tc.override) > 0 {
				file = tc.override
			}

			td := t.TempDir()
			testCopyDir(t, testFixturePath(path.Join("test", file)), td)
			t.Chdir(td)

			provider := testing_command.NewProvider(nil)
			view, done := testView(t)

			c := &TestCommand{
				Meta: Meta{
					testingOverrides: metaOverridesForProvider(provider.Provider),
					View:             view,
				},
			}

			code := c.Run(tc.args)
			output := done(t)

			if code != tc.code {
				t.Errorf("expected status code %d but got %d", tc.code, code)
			}

			if !strings.Contains(output.Stdout(), tc.expected) {
				t.Errorf("output didn't contain expected string:\n\n%s", output.All())
			}

			if provider.ResourceCount() > 0 {
				t.Errorf("should have deleted all resources on completion but left %v", provider.ResourceString())
			}
		})
	}
}
func TestTest_Full_Output(t *testing.T) {
	tcs := map[string]struct {
		override string
		args     []string
		expected string
		code     int
		skip     bool
	}{
		"broken_no_valid_hcl": {
			expected: "Unsupported block type",
			code:     1,
		},
		"expect_runtime_check_fail_command_plan": {
			expected: "Check block assertion known after apply",
			code:     1,
		},
		"broken_wrong_block_resource": {
			expected: "Blocks of type \"resource\" are not expected here.",
			code:     1,
		},
		"broken_wrong_block_data": {
			expected: "Blocks of type \"data\" are not expected here.",
			code:     1,
		},
		"broken_wrong_block_output": {
			expected: "Blocks of type \"output\" are not expected here.",
			code:     1,
		},
		"broken_wrong_block_check": {
			expected: "Blocks of type \"check\" are not expected here.",
			code:     1,
		},
		"not_exists_output": {
			expected: "Error: Reference to undeclared output value",
			args:     []string{"-no-color"},
			code:     1,
		},
		"refresh_conflicting_config": {
			expected: "Incompatible plan options",
			code:     1,
		},
		"is_sorted": {
			expected: "1.tftest.hcl... pass\n  run \"a\"... pass\n2.tftest.hcl... pass\n  run \"b\"... pass\n3.tftest.hcl... pass\n  run \"c\"... pass",
			code:     0,
			args:     []string{"-no-color"},
		},
	}
	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			if tc.skip {
				t.Skip()
			}

			file := name
			if len(tc.override) > 0 {
				file = tc.override
			}

			td := t.TempDir()
			testCopyDir(t, testFixturePath(path.Join("test", file)), td)
			t.Chdir(td)

			provider := testing_command.NewProvider(nil)
			view, done := testView(t)

			c := &TestCommand{
				Meta: Meta{
					testingOverrides: metaOverridesForProvider(provider.Provider),
					View:             view,
				},
			}

			code := c.Run(tc.args)
			output := done(t)

			if code != tc.code {
				t.Errorf("expected status code %d but got %d", tc.code, code)
			}

			if !strings.Contains(output.All(), tc.expected) {
				t.Errorf("output didn't contain expected string:\n\n%s \n\n----\n\nexpected: %s", output.All(), tc.expected)
			}

			if provider.ResourceCount() > 0 {
				t.Errorf("should have deleted all resources on completion but left %v", provider.ResourceString())
			}
		})
	}
}

func TestTest_Interrupt(t *testing.T) {
	td := t.TempDir()
	testCopyDir(t, testFixturePath(path.Join("test", "with_interrupt")), td)
	t.Chdir(td)

	provider := testing_command.NewProvider(nil)
	view, done := testView(t)

	interrupt := make(chan struct{})
	provider.Interrupt = interrupt

	c := &TestCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(provider.Provider),
			View:             view,
			ShutdownCh:       interrupt,
		},
	}

	c.Run(nil)
	output := done(t).All()

	if !strings.Contains(output, "Interrupt received") {
		t.Errorf("output didn't produce the right output:\n\n%s", output)
	}

	if provider.ResourceCount() > 0 {
		// we asked for a nice stop in this one, so it should still have tidied everything up.
		t.Errorf("should have deleted all resources on completion but left %v", provider.ResourceString())
	}
}

func TestTest_DoubleInterrupt(t *testing.T) {
	td := t.TempDir()
	testCopyDir(t, testFixturePath(path.Join("test", "with_double_interrupt")), td)
	t.Chdir(td)

	provider := testing_command.NewProvider(nil)
	view, done := testView(t)

	interrupt := make(chan struct{})
	provider.Interrupt = interrupt

	c := &TestCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(provider.Provider),
			View:             view,
			ShutdownCh:       interrupt,
		},
	}

	c.Run(nil)
	output := done(t).All()

	if !strings.Contains(output, "Two interrupts received") {
		t.Errorf("output didn't produce the right output:\n\n%s", output)
	}

	cleanupMessage := `OpenTofu was interrupted while executing main.tftest.hcl, and may not have
performed the expected cleanup operations.

OpenTofu has already created the following resources from the module under
test:
  - test_resource.primary
  - test_resource.secondary
  - test_resource.tertiary`

	// It's really important that the above message is printed, so we're testing
	// for it specifically and making sure it contains all the resources.
	if !strings.Contains(output, cleanupMessage) {
		t.Errorf("output didn't produce the right output:\n\n%s", output)
	}

	// This time the test command shouldn't have cleaned up the resource because
	// of the hard interrupt.
	if provider.ResourceCount() != 3 {
		// we asked for a nice stop in this one, so it should still have tidied everything up.
		t.Errorf("should not have deleted all resources on completion but left %v", provider.ResourceString())
	}
}

func TestTest_ProviderAlias(t *testing.T) {
	td := t.TempDir()
	testCopyDir(t, testFixturePath(path.Join("test", "with_provider_alias")), td)
	t.Chdir(td)

	provider := testing_command.NewProvider(nil)

	providerSource, close := newMockProviderSource(t, map[string][]string{
		"test": {"1.0.0"},
	})
	defer close()

	streams, done := terminal.StreamsForTesting(t)
	view := views.NewView(streams)
	ui := new(cli.MockUi)

	meta := Meta{
		testingOverrides: metaOverridesForProvider(provider.Provider),
		Ui:               ui,
		View:             view,
		Streams:          streams,
		ProviderSource:   providerSource,
	}

	init := &InitCommand{
		Meta: meta,
	}

	if code := init.Run(nil); code != 0 {
		t.Fatalf("expected status code 0 but got %d: %s", code, ui.ErrorWriter)
	}

	command := &TestCommand{
		Meta: meta,
	}

	code := command.Run(nil)
	output := done(t)

	printedOutput := false

	if code != 0 {
		printedOutput = true
		t.Errorf("expected status code 0 but got %d: %s", code, output.All())
	}

	if provider.ResourceCount() > 0 {
		if !printedOutput {
			t.Errorf("should have deleted all resources on completion but left %s\n\n%s", provider.ResourceString(), output.All())
		} else {
			t.Errorf("should have deleted all resources on completion but left %s", provider.ResourceString())
		}
	}
}

func TestTest_ModuleDependencies(t *testing.T) {
	td := t.TempDir()
	testCopyDir(t, testFixturePath(path.Join("test", "with_setup_module")), td)
	t.Chdir(td)

	// Our two providers will share a common set of values to make things
	// easier.
	store := &testing_command.ResourceStore{
		Data: make(map[string]cty.Value),
	}

	// We set it up so the module provider will update the data sources
	// available to the core mock provider.
	test := testing_command.NewProvider(store)
	setup := testing_command.NewProvider(store)

	test.SetDataPrefix("data")
	test.SetResourcePrefix("resource")

	// Let's make the setup provider write into the data for test provider.
	setup.SetResourcePrefix("data")

	providerSource, close := newMockProviderSource(t, map[string][]string{
		"test":  {"1.0.0"},
		"setup": {"1.0.0"},
	})
	defer close()

	streams, done := terminal.StreamsForTesting(t)
	view := views.NewView(streams)
	ui := new(cli.MockUi)

	meta := Meta{
		testingOverrides: &testingOverrides{
			Providers: map[addrs.Provider]providers.Factory{
				addrs.NewDefaultProvider("test"):  providers.FactoryFixed(test.Provider),
				addrs.NewDefaultProvider("setup"): providers.FactoryFixed(setup.Provider),
			},
		},
		Ui:             ui,
		View:           view,
		Streams:        streams,
		ProviderSource: providerSource,
	}

	init := &InitCommand{
		Meta: meta,
	}

	if code := init.Run(nil); code != 0 {
		t.Fatalf("expected status code 0 but got %d: %s", code, ui.ErrorWriter)
	}

	command := &TestCommand{
		Meta: meta,
	}

	code := command.Run(nil)
	output := done(t)

	printedOutput := false

	if code != 0 {
		printedOutput = true
		t.Errorf("expected status code 0 but got %d: %s", code, output.All())
	}

	if test.ResourceCount() > 0 {
		if !printedOutput {
			printedOutput = true
			t.Errorf("should have deleted all resources on completion but left %s\n\n%s", test.ResourceString(), output.All())
		} else {
			t.Errorf("should have deleted all resources on completion but left %s", test.ResourceString())
		}
	}

	if setup.ResourceCount() > 0 {
		if !printedOutput {
			t.Errorf("should have deleted all resources on completion but left %s\n\n%s", setup.ResourceString(), output.All())
		} else {
			t.Errorf("should have deleted all resources on completion but left %s", setup.ResourceString())
		}
	}
}

func TestTest_CatchesErrorsBeforeDestroy(t *testing.T) {
	td := t.TempDir()
	testCopyDir(t, testFixturePath(path.Join("test", "invalid_default_state")), td)
	t.Chdir(td)

	provider := testing_command.NewProvider(nil)
	view, done := testView(t)

	c := &TestCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(provider.Provider),
			View:             view,
		},
	}

	code := c.Run([]string{"-no-color"})
	output := done(t)

	if code != 1 {
		t.Errorf("expected status code 0 but got %d", code)
	}

	expectedOut := `main.tftest.hcl... fail
  run "test"... fail

Failure! 0 passed, 1 failed.
`

	expectedErr := `
Error: No value for required variable

  on main.tf line 2:
   2: variable "input" {

The root module input variable "input" is not set, and has no default value.
Use a -var or -var-file command line argument to provide a value for this
variable.
`

	actualOut := output.Stdout()
	actualErr := output.Stderr()

	if diff := cmp.Diff(actualOut, expectedOut); len(diff) > 0 {
		t.Errorf("std out didn't match expected:\nexpected:\n%s\nactual:\n%s\ndiff:\n%s", expectedOut, actualOut, diff)
	}

	if diff := cmp.Diff(actualErr, expectedErr); len(diff) > 0 {
		t.Errorf("std err didn't match expected:\nexpected:\n%s\nactual:\n%s\ndiff:\n%s", expectedErr, actualErr, diff)
	}

	if provider.ResourceCount() > 0 {
		t.Errorf("should have deleted all resources on completion but left %v", provider.ResourceString())
	}
}

func TestTest_Verbose(t *testing.T) {
	td := t.TempDir()
	testCopyDir(t, testFixturePath(path.Join("test", "plan_then_apply")), td)
	t.Chdir(td)

	provider := testing_command.NewProvider(nil)
	view, done := testView(t)

	c := &TestCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(provider.Provider),
			View:             view,
		},
	}

	code := c.Run([]string{"-verbose", "-no-color"})
	output := done(t)

	if code != 0 {
		t.Errorf("expected status code 0 but got %d", code)
	}

	expected := `main.tftest.hcl... pass
  run "validate_test_resource"... pass

OpenTofu used the selected providers to generate the following execution
plan. Resource actions are indicated with the following symbols:
  + create

OpenTofu will perform the following actions:

  # test_resource.foo will be created
  + resource "test_resource" "foo" {
      + id    = "constant_value"
      + value = "bar"
    }

Plan: 1 to add, 0 to change, 0 to destroy.
  run "validate_test_resource"... pass
# test_resource.foo:
resource "test_resource" "foo" {
    id    = "constant_value"
    value = "bar"
}

Success! 2 passed, 0 failed.
`

	actual := output.All()

	if diff := cmp.Diff(actual, expected); len(diff) > 0 {
		t.Errorf("output didn't match expected:\nexpected:\n%s\nactual:\n%s\ndiff:\n%s", expected, actual, diff)
	}

	if provider.ResourceCount() > 0 {
		t.Errorf("should have deleted all resources on completion but left %v", provider.ResourceString())
	}
}

func TestTest_ValidatesBeforeExecution(t *testing.T) {
	tcs := map[string]struct {
		expectedOut string
		expectedErr string
	}{
		"invalid": {
			expectedOut: `main.tftest.hcl... fail
  run "invalid"... fail

Failure! 0 passed, 1 failed.
`,
			expectedErr: `
Error: Invalid ` + "`expect_failures`" + ` reference

  on main.tftest.hcl line 5, in run "invalid":
   5:         local.my_value,

You cannot expect failures from local.my_value. You can only expect failures
from checkable objects such as input variables, output values, check blocks,
managed resources and data sources.
`,
		},
		"invalid-module": {
			expectedOut: `main.tftest.hcl... fail
  run "invalid"... fail
  run "test"... skip

Failure! 0 passed, 1 failed, 1 skipped.
`,
			expectedErr: fmt.Sprintf(`
Error: Reference to undeclared input variable

  on %s line 3, in resource "test_resource" "setup":
   3:     value = var.not_real // Oh no!

An input variable with the name "not_real" has not been declared. This
variable can be declared with a variable "not_real" {} block.
`, filepath.FromSlash("setup/main.tf")),
		},
		"missing-provider": {
			expectedOut: `main.tftest.hcl... fail
  run "passes_validation"... fail

Failure! 0 passed, 1 failed.
`,
			expectedErr: `
Error: Provider configuration not present

To work with test_resource.secondary its original provider configuration at
provider["registry.opentofu.org/hashicorp/test"].secondary is required, but
it has been removed. This occurs when a provider configuration is removed
while objects created by that provider still exist in the state. Re-add the
provider configuration to destroy test_resource.secondary, after which you
can remove the provider configuration again.
`,
		},
		"missing-provider-in-run-block": {
			expectedOut: `main.tftest.hcl... fail
  run "passes_validation"... fail

Failure! 0 passed, 1 failed.
`,
			expectedErr: `
Error: Provider configuration not present

To work with test_resource.secondary its original provider configuration at
provider["registry.opentofu.org/hashicorp/test"].secondary is required, but
it has been removed. This occurs when a provider configuration is removed
while objects created by that provider still exist in the state. Re-add the
provider configuration to destroy test_resource.secondary, after which you
can remove the provider configuration again.
`,
		},
		"missing-provider-in-test-module": {
			expectedOut: `main.tftest.hcl... fail
  run "passes_validation_primary"... pass
  run "passes_validation_secondary"... fail

Failure! 1 passed, 1 failed.
`,
			expectedErr: `
Error: Provider configuration not present

To work with test_resource.secondary its original provider configuration at
provider["registry.opentofu.org/hashicorp/test"].secondary is required, but
it has been removed. This occurs when a provider configuration is removed
while objects created by that provider still exist in the state. Re-add the
provider configuration to destroy test_resource.secondary, after which you
can remove the provider configuration again.
`,
		},
	}

	for file, tc := range tcs {
		t.Run(file, func(t *testing.T) {

			td := t.TempDir()
			testCopyDir(t, testFixturePath(path.Join("test", file)), td)
			t.Chdir(td)

			provider := testing_command.NewProvider(nil)

			providerSource, close := newMockProviderSource(t, map[string][]string{
				"test": {"1.0.0"},
			})
			defer close()

			streams, done := terminal.StreamsForTesting(t)
			view := views.NewView(streams)
			ui := new(cli.MockUi)

			meta := Meta{
				testingOverrides: metaOverridesForProvider(provider.Provider),
				Ui:               ui,
				View:             view,
				Streams:          streams,
				ProviderSource:   providerSource,
			}

			init := &InitCommand{
				Meta: meta,
			}

			if code := init.Run(nil); code != 0 {
				t.Fatalf("expected status code 0 but got %d: %s", code, ui.ErrorWriter)
			}

			c := &TestCommand{
				Meta: meta,
			}

			code := c.Run([]string{"-no-color"})
			output := done(t)

			if code != 1 {
				t.Errorf("expected status code 1 but got %d", code)
			}

			actualOut, expectedOut := output.Stdout(), tc.expectedOut
			actualErr, expectedErr := output.Stderr(), tc.expectedErr

			if diff := cmp.Diff(actualOut, expectedOut); len(diff) > 0 {
				t.Errorf("output didn't match expected:\nexpected:\n%s\nactual:\n%s\ndiff:\n%s", expectedOut, actualOut, diff)
			}

			if diff := cmp.Diff(actualErr, expectedErr); len(diff) > 0 {
				t.Errorf("error didn't match expected:\nexpected:\n%s\nactual:\n%s\ndiff:\n%s", expectedErr, actualErr, diff)
			}

			if provider.ResourceCount() > 0 {
				t.Errorf("should have deleted all resources on completion but left %v", provider.ResourceString())
			}
		})
	}
}

func TestTest_Modules(t *testing.T) {
	tcs := map[string]struct {
		expected                      string
		code                          int
		skip                          bool
		expectedProviderConfigRequest *providers.ConfigureProviderRequest
	}{
		"pass_module_with_no_resource": {
			expected: "main.tftest.hcl... pass\n  run \"run\"... pass\n\nSuccess! 1 passed, 0 failed.\n",
			code:     0,
		},
		"with_nested_setup_modules": {
			expected: "main.tftest.hcl... pass\n  run \"load_module\"... pass\n\nSuccess! 1 passed, 0 failed.\n",
			code:     0,
		},
		"with_verify_module": {
			expected: "main.tftest.hcl... pass\n  run \"test\"... pass\n  run \"verify\"... pass\n\nSuccess! 2 passed, 0 failed.\n",
			code:     0,
		},
		"only_modules": {
			expected: "main.tftest.hcl... pass\n  run \"first\"... pass\n  run \"second\"... pass\n\nSuccess! 2 passed, 0 failed.\n",
			code:     0,
		},
		"variables_reference": {
			expected: "main.tftest.hcl... pass\n  run \"setup\"... pass\n  run \"test\"... pass\n\nSuccess! 2 passed, 0 failed.\n",
			code:     0,
		},
		"destroyed_mod_outputs": {
			expected: "main.tftest.hcl... pass\n  run \"first_apply\"... pass\n  run \"second_apply\"... pass\n\nSuccess! 2 passed, 0 failed.\n",
			code:     0,
		},
		"run_mod_output_in_provider": {
			expected: "main.tftest.hcl... pass\n  run \"setup\"... pass\n  run \"validate\"... pass\n\nSuccess! 2 passed, 0 failed.\n",
			code:     0,
			expectedProviderConfigRequest: &providers.ConfigureProviderRequest{
				Config: cty.ObjectVal(map[string]cty.Value{
					"password":        cty.StringVal("p"),
					"username":        cty.StringVal("test_user"),
					"data_prefix":     cty.StringVal("test"),
					"resource_prefix": cty.StringVal("test"),
					"block_single": cty.NullVal(cty.Object(map[string]cty.Type{
						"string_attr": cty.String,
					})),
				}),
			},
		},
		"run_mod_output_in_provider_complex": {
			expected: "main.tftest.hcl... pass\n  run \"setup\"... pass\n  run \"validate\"... pass\n\nSuccess! 2 passed, 0 failed.\n",
			code:     0,
			expectedProviderConfigRequest: &providers.ConfigureProviderRequest{
				Config: cty.ObjectVal(map[string]cty.Value{
					"password":        cty.StringVal("Password"),
					"username":        cty.StringVal("test_user@d"),
					"data_prefix":     cty.StringVal("test"),
					"resource_prefix": cty.StringVal("test"),
					"block_single": cty.NullVal(cty.Object(map[string]cty.Type{
						"string_attr": cty.String,
					})),
				}),
			},
		},
		"run_mod_output_in_provider_with_blocks": {
			expected: "main.tftest.hcl... pass\n  run \"setup\"... pass\n  run \"validate\"... pass\n\nSuccess! 2 passed, 0 failed.\n",
			code:     0,
			expectedProviderConfigRequest: &providers.ConfigureProviderRequest{
				Config: cty.ObjectVal(map[string]cty.Value{
					"password":        cty.StringVal("p"),
					"username":        cty.StringVal("test_user"),
					"data_prefix":     cty.StringVal("test"),
					"resource_prefix": cty.StringVal("test"),
					"block_single": cty.ObjectVal(map[string]cty.Value{
						"string_attr": cty.StringVal("r"),
					}),
				}),
			},
		},
		"run_mod_output_in_provider_undefined_ref": {
			code: 1,
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			if tc.skip {
				t.Skip()
			}

			file := name

			td := t.TempDir()
			testCopyDir(t, testFixturePath(path.Join("test", file)), td)
			t.Chdir(td)

			provider := testing_command.NewProvider(nil)
			providerSource, close := newMockProviderSource(t, map[string][]string{
				"test": {"1.0.0"},
			})
			defer close()

			streams, done := terminal.StreamsForTesting(t)
			view := views.NewView(streams)
			ui := new(cli.MockUi)
			meta := Meta{
				testingOverrides: metaOverridesForProvider(provider.Provider),
				Ui:               ui,
				View:             view,
				Streams:          streams,
				ProviderSource:   providerSource,
			}

			init := &InitCommand{
				Meta: meta,
			}

			if code := init.Run(nil); code != 0 {
				t.Fatalf("expected status code 0 but got %d: %s", code, ui.ErrorWriter)
			}

			command := &TestCommand{
				Meta: meta,
			}

			code := command.Run([]string{"-no-color"})
			output := done(t)
			printedOutput := false

			if code != tc.code {
				printedOutput = true
				t.Errorf("expected status code %d but got %d: %s", tc.code, code, output.All())
			}

			// If we're not expecting a failure, we can compare the output.
			if code != 1 {
				actual := output.All()
				if diff := cmp.Diff(actual, tc.expected); len(diff) > 0 {
					t.Errorf("output didn't match expected:\nexpected:\n%s\nactual:\n%s\ndiff:\n%s", tc.expected, actual, diff)
				}
			}

			if tc.expectedProviderConfigRequest != nil {
				if !provider.Provider.ConfigureProviderRequest.Config.Equals(tc.expectedProviderConfigRequest.Config).True() {
					t.Errorf("expected provider config request to equal %+v but got %+v", tc.expectedProviderConfigRequest.Config, provider.Provider.ConfigureProviderRequest.Config)
				}
			}

			if provider.ResourceCount() > 0 {
				if !printedOutput {
					t.Errorf("should have deleted all resources on completion but left %s\n\n%s", provider.ResourceString(), output.All())
				} else {
					t.Errorf("should have deleted all resources on completion but left %s", provider.ResourceString())
				}
			}

			if provider.DataSourceCount() > 0 {
				if !printedOutput {
					t.Errorf("should have deleted all data sources on completion but left %s\n\n%s", provider.DataSourceString(), output.All())
				} else {
					t.Errorf("should have deleted all data sources on completion but left %s", provider.DataSourceString())
				}
			}
		})
	}
}

func TestTest_StatePropagation(t *testing.T) {
	td := t.TempDir()
	testCopyDir(t, testFixturePath(path.Join("test", "state_propagation")), td)
	t.Chdir(td)

	provider := testing_command.NewProvider(nil)

	providerSource, close := newMockProviderSource(t, map[string][]string{
		"test": {"1.0.0"},
	})
	defer close()

	streams, done := terminal.StreamsForTesting(t)
	view := views.NewView(streams)
	ui := new(cli.MockUi)

	meta := Meta{
		testingOverrides: metaOverridesForProvider(provider.Provider),
		Ui:               ui,
		View:             view,
		Streams:          streams,
		ProviderSource:   providerSource,
	}

	init := &InitCommand{
		Meta: meta,
	}

	if code := init.Run(nil); code != 0 {
		t.Fatalf("expected status code 0 but got %d: %s", code, ui.ErrorWriter)
	}

	c := &TestCommand{
		Meta: meta,
	}

	code := c.Run([]string{"-verbose", "-no-color"})
	output := done(t)

	if code != 0 {
		t.Errorf("expected status code 0 but got %d", code)
	}

	expected := `main.tftest.hcl... pass
  run "initial_apply_example"... pass
# test_resource.module_resource:
resource "test_resource" "module_resource" {
    id    = "df6h8as9"
    value = "start"
}
  run "initial_apply"... pass
# test_resource.resource:
resource "test_resource" "resource" {
    id    = "598318e0"
    value = "start"
}
  run "plan_second_example"... pass

OpenTofu used the selected providers to generate the following execution
plan. Resource actions are indicated with the following symbols:
  + create

OpenTofu will perform the following actions:

  # test_resource.second_module_resource will be created
  + resource "test_resource" "second_module_resource" {
      + id    = "b6a1d8cb"
      + value = "start"
    }

Plan: 1 to add, 0 to change, 0 to destroy.
  run "plan_update"... pass

OpenTofu used the selected providers to generate the following execution
plan. Resource actions are indicated with the following symbols:
  ~ update in-place (current -> planned)

OpenTofu will perform the following actions:

  # test_resource.resource will be updated in-place
  ~ resource "test_resource" "resource" {
        id    = "598318e0"
      ~ value = "start" -> "update"
    }

Plan: 0 to add, 1 to change, 0 to destroy.
  run "plan_update_example"... pass

OpenTofu used the selected providers to generate the following execution
plan. Resource actions are indicated with the following symbols:
  ~ update in-place (current -> planned)

OpenTofu will perform the following actions:

  # test_resource.module_resource will be updated in-place
  ~ resource "test_resource" "module_resource" {
        id    = "df6h8as9"
      ~ value = "start" -> "update"
    }

Plan: 0 to add, 1 to change, 0 to destroy.

Success! 5 passed, 0 failed.
`

	actual := output.All()

	if diff := cmp.Diff(actual, expected); len(diff) > 0 {
		t.Errorf("output didn't match expected:\nexpected:\n%s\nactual:\n%s\ndiff:\n%s", expected, actual, diff)
	}

	if provider.ResourceCount() > 0 {
		t.Errorf("should have deleted all resources on completion but left %v", provider.ResourceString())
	}
}

func TestTest_PartialUpdates(t *testing.T) {
	tcs := map[string]struct {
		expectedOut  string
		expectedErr  string
		expectedCode int
	}{
		"partial_updates": {
			expectedOut: `main.tftest.hcl... pass
  run "first"... pass

Warning: Resource targeting is in effect

You are creating a plan with either the -target option or the -exclude
option, which means that the result of this plan may not represent all of the
changes requested by the current configuration.

The -target and -exclude options are not for routine use, and are provided
only for exceptional situations such as recovering from errors or mistakes,
or when OpenTofu specifically suggests to use it as part of an error message.

Warning: Applied changes may be incomplete

The plan was created with the -target or the -exclude option in effect, so
some changes requested in the configuration may have been ignored and the
output values may not be fully updated. Run the following command to verify
that no other changes are pending:
    tofu plan
	
Note that the -target and -exclude options are not suitable for routine use,
and are provided only for exceptional situations such as recovering from
errors or mistakes, or when OpenTofu specifically suggests to use it as part
of an error message.
  run "second"... pass

Success! 2 passed, 0 failed.
`,
			expectedCode: 0,
		},
		"partial_update_failure": {
			expectedOut: `main.tftest.hcl... fail
  run "partial"... fail

Warning: Resource targeting is in effect

You are creating a plan with either the -target option or the -exclude
option, which means that the result of this plan may not represent all of the
changes requested by the current configuration.

The -target and -exclude options are not for routine use, and are provided
only for exceptional situations such as recovering from errors or mistakes,
or when OpenTofu specifically suggests to use it as part of an error message.

Warning: Applied changes may be incomplete

The plan was created with the -target or the -exclude option in effect, so
some changes requested in the configuration may have been ignored and the
output values may not be fully updated. Run the following command to verify
that no other changes are pending:
    tofu plan
	
Note that the -target and -exclude options are not suitable for routine use,
and are provided only for exceptional situations such as recovering from
errors or mistakes, or when OpenTofu specifically suggests to use it as part
of an error message.

Failure! 0 passed, 1 failed.
`,
			expectedErr: `
Error: Unknown condition run

  on main.tftest.hcl line 7, in run "partial":
   7:     condition = test_resource.bar.value == "bar"

Condition expression could not be evaluated at this time.
`,
			expectedCode: 1,
		},
	}

	for file, tc := range tcs {
		t.Run(file, func(t *testing.T) {
			td := t.TempDir()
			testCopyDir(t, testFixturePath(path.Join("test", file)), td)
			t.Chdir(td)

			provider := testing_command.NewProvider(nil)
			view, done := testView(t)

			c := &TestCommand{
				Meta: Meta{
					testingOverrides: metaOverridesForProvider(provider.Provider),
					View:             view,
				},
			}

			code := c.Run([]string{"-no-color"})
			output := done(t)

			actualOut, expectedOut := output.Stdout(), tc.expectedOut
			actualErr, expectedErr := output.Stderr(), tc.expectedErr
			expectedCode := tc.expectedCode

			if code != expectedCode {
				t.Errorf("expected status code %d but got %d", expectedCode, code)
			}

			if diff := cmp.Diff(actualOut, expectedOut); len(diff) > 0 {
				t.Errorf("output didn't match expected:\nexpected:\n%s\nactual:\n%s\ndiff:\n%s", expectedOut, actualOut, diff)
			}

			if diff := cmp.Diff(actualErr, expectedErr); len(diff) > 0 {
				t.Errorf("error didn't match expected:\nexpected:\n%s\nactual:\n%s\ndiff:\n%s", expectedErr, actualErr, diff)
			}

			if provider.ResourceCount() > 0 {
				t.Errorf("should have deleted all resources on completion but left %v", provider.ResourceString())
			}
		})
	}
}

func TestTest_LocalVariables(t *testing.T) {
	tcs := map[string]struct {
		expected string
		code     int
		skip     bool
	}{
		"pass_with_local_variable": {
			expected: fmt.Sprintf(`%s... pass
  run "first"... pass


Outputs:

foo = "bar"
  run "second"... pass

No changes. Your infrastructure matches the configuration.

OpenTofu has compared your real infrastructure against your configuration and
found no differences, so no changes are needed.

Success! 2 passed, 0 failed.
`, filepath.FromSlash("tests/test.tftest.hcl")),
			code: 0,
		},
		"pass_var_inside_variables": {
			expected: `main.tftest.hcl... pass
  run "first"... pass


Outputs:

sss = "false"

Success! 1 passed, 0 failed.
`,
			code: 0,
		},
		"pass_var_with_default_value_inside_variables": {
			expected: `main.tftest.hcl... pass
  run "first"... pass


Outputs:

sss = "true"

Success! 1 passed, 0 failed.
`,
			code: 0,
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			if tc.skip {
				t.Skip()
			}

			file := name

			td := t.TempDir()
			testCopyDir(t, testFixturePath(path.Join("test", file)), td)
			t.Chdir(td)

			provider := testing_command.NewProvider(nil)
			providerSource, providerClose := newMockProviderSource(t, map[string][]string{
				"test": {"1.0.0"},
			})
			defer providerClose()

			streams, done := terminal.StreamsForTesting(t)
			view := views.NewView(streams)
			ui := new(cli.MockUi)
			meta := Meta{
				testingOverrides: metaOverridesForProvider(provider.Provider),
				Ui:               ui,
				View:             view,
				Streams:          streams,
				ProviderSource:   providerSource,
			}

			init := &InitCommand{
				Meta: meta,
			}

			if code := init.Run(nil); code != 0 {
				t.Fatalf("expected status code 0 but got %d: %s", code, ui.ErrorWriter)
			}

			command := &TestCommand{
				Meta: meta,
			}

			code := command.Run([]string{"-verbose", "-no-color"})
			output := done(t)

			if code != tc.code {
				t.Errorf("expected status code %d but got %d: %s", tc.code, code, output.All())
			}

			actual := output.All()

			if diff := cmp.Diff(actual, tc.expected); len(diff) > 0 {
				t.Errorf("output didn't match expected:\nexpected:\n%s\nactual:\n%s\ndiff:\n%s", tc.expected, actual, diff)
			}
		})
	}
}

func TestTest_InvalidLocalVariables(t *testing.T) {
	tcs := map[string]struct {
		contains    []string
		notContains []string
		code        int
		skip        bool
	}{
		"invalid_variable_warning_expected": {
			contains: []string{
				"Warning: Invalid Variable in test file",
				"Error: Invalid value for variable",
				"This was checked by the validation rule at main.tf:5,3-13.",
				"This was checked by the validation rule at main.tf:14,3-13.",
			},
			code: 1,
		},
		"invalid_variable_warning_no_expected": {
			contains: []string{
				"Error: Invalid value for variable",
				"This was checked by the validation rule at main.tf:5,3-13.",
				"This was checked by the validation rule at main.tf:14,3-13.",
			},
			notContains: []string{
				"Warning: Invalid Variable in test file",
			},
			code: 1,
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			if tc.skip {
				t.Skip()
			}

			file := name

			td := t.TempDir()
			testCopyDir(t, testFixturePath(path.Join("test", file)), td)
			t.Chdir(td)

			provider := testing_command.NewProvider(nil)
			providerSource, providerClose := newMockProviderSource(t, map[string][]string{
				"test": {"1.0.0"},
			})
			defer providerClose()

			streams, done := terminal.StreamsForTesting(t)
			view := views.NewView(streams)
			ui := new(cli.MockUi)
			meta := Meta{
				testingOverrides: metaOverridesForProvider(provider.Provider),
				Ui:               ui,
				View:             view,
				Streams:          streams,
				ProviderSource:   providerSource,
			}

			init := &InitCommand{
				Meta: meta,
			}

			if code := init.Run(nil); code != 0 {
				t.Fatalf("expected status code 0 but got %d: %s", code, ui.ErrorWriter)
			}

			command := &TestCommand{
				Meta: meta,
			}

			code := command.Run([]string{"-verbose", "-no-color"})
			output := done(t)

			if code != tc.code {
				t.Errorf("expected status code %d but got %d: %s", tc.code, code, output.All())
			}

			actual := output.All()

			for _, containsString := range tc.contains {
				if !strings.Contains(actual, containsString) {
					t.Errorf("expected '%s' in output but didn't find it: \n%s", containsString, output.All())
				}
			}

			for _, notContainsString := range tc.notContains {
				if strings.Contains(actual, notContainsString) {
					t.Errorf("expected not to find '%s' in output: \n%s", notContainsString, output.All())
				}
			}
		})
	}
}

func TestTest_RunBlock(t *testing.T) {
	tcs := map[string]struct {
		expected string
		code     int
		skip     bool
	}{
		"invalid_run_block_name": {
			expected: `
Error: Invalid run block name

  on tests/main.tftest.hcl line 1, in run "sample run":
   1: run "sample run" {

A name must start with a letter or underscore and may contain only letters,
digits, underscores, and dashes.
`,
			code: 1,
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			if tc.skip {
				t.Skip()
			}

			file := name

			td := t.TempDir()
			testCopyDir(t, testFixturePath(path.Join("test", file)), td)
			t.Chdir(td)

			provider := testing_command.NewProvider(nil)
			providerSource, close := newMockProviderSource(t, map[string][]string{
				"test": {"1.0.0"},
			})
			defer close()

			streams, _ := terminal.StreamsForTesting(t)
			view := views.NewView(streams)
			ui := new(cli.MockUi)
			meta := Meta{
				testingOverrides: metaOverridesForProvider(provider.Provider),
				Ui:               ui,
				View:             view,
				Streams:          streams,
				ProviderSource:   providerSource,
			}

			init := &InitCommand{
				Meta: meta,
			}

			if code := init.Run(nil); code != tc.code {
				t.Fatalf("expected status code 0 but got %d: %s", code, ui.ErrorWriter)
			}
		})
	}
}

// TestTest_MockProviderValidation checks if tofu test runs proper validation for
// mock_provider. Even if provider schema has required fields, tofu test should
// ignore it completely, because the provider is mocked.
func TestTest_MockProviderValidation(t *testing.T) {
	td := t.TempDir()
	testCopyDir(t, testFixturePath("test/mock_provider_validation"), td)
	t.Chdir(td)

	provider := testing_command.NewProvider(nil)
	providerSource, closePS := newMockProviderSource(t, map[string][]string{
		"test": {"1.0.0"},
	})
	defer closePS()

	provider.Provider.GetProviderSchemaResponse = &providers.GetProviderSchemaResponse{
		Provider: providers.Schema{
			Block: &configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"required_field": {
						Type:     cty.String,
						Required: true,
					},
				},
			},
		},
		ResourceTypes: map[string]providers.Schema{
			"test_resource": {
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"value": {
							Type:     cty.String,
							Optional: true,
						},
						"object_attr": {
							Computed: true,
							NestedType: &configschema.Object{
								Nesting: configschema.NestingSingle,
								Attributes: map[string]*configschema.Attribute{
									"string_attr": {
										Type:     cty.String,
										Computed: true,
									},
								},
							},
						},
						"computed_value": {
							Type:     cty.String,
							Computed: true,
						},
					},
				},
			},
		},
	}

	view, done := testView(t)
	ui := new(cli.MockUi)
	meta := Meta{
		testingOverrides: metaOverridesForProvider(provider.Provider),
		Ui:               ui,
		View:             view,
		ProviderSource:   providerSource,
	}

	testCmd := &TestCommand{
		Meta: meta,
	}

	code := testCmd.Run(nil)
	output := done(t)
	if code != 0 {
		t.Fatalf("expected status code 0 but got %d: %s", code, output.All())
	}
}

// TestTest_MockProviderValidationForEach checks if tofu test runs proper validation for
// mock_provider with for_each. Even if provider schema has required fields, tofu test should
// ignore it completely, because the provider is mocked.
func TestTest_MockProviderValidationForEach(t *testing.T) {
	td := t.TempDir()
	testCopyDir(t, testFixturePath("test/mock_provider_validation_for_each"), td)
	t.Chdir(td)

	provider := testing_command.NewProvider(nil)
	providerSource, closePS := newMockProviderSource(t, map[string][]string{
		"test": {"1.0.0"},
	})
	defer closePS()

	provider.Provider.ConfigureProviderCalled = true
	provider.Provider.GetProviderSchemaResponse = &providers.GetProviderSchemaResponse{
		ResourceTypes: map[string]providers.Schema{
			"test_resource": {
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"value": {
							Type:     cty.String,
							Optional: true,
						},
						"object_attr": {
							Computed: true,
							NestedType: &configschema.Object{
								Nesting: configschema.NestingSingle,
								Attributes: map[string]*configschema.Attribute{
									"string_attr": {
										Type:     cty.String,
										Computed: true,
									},
								},
							},
						},
						"computed_value": {
							Type:     cty.String,
							Computed: true,
						},
					},
				},
			},
		},
	}

	view, done := testView(t)
	ui := new(cli.MockUi)
	meta := Meta{
		testingOverrides: metaOverridesForProvider(provider.Provider),
		Ui:               ui,
		View:             view,
		ProviderSource:   providerSource,
	}

	testCmd := &TestCommand{
		Meta: meta,
	}

	code := testCmd.Run(nil)
	output := done(t)
	if code != 0 {
		t.Fatalf("expected status code 0 but got %d: %s", code, output.All())
	}
}

// See https://github.com/opentofu/opentofu/issues/3246
func TestTest_DeprecatedOutputs(t *testing.T) {
	td := t.TempDir()
	testCopyDir(t, testFixturePath("test/deprecated_outputs"), td)
	t.Chdir(td)

	view, done := testView(t)
	ui := new(cli.MockUi)
	meta := Meta{
		Ui:   ui,
		View: view,
	}

	testCmd := &TestCommand{
		Meta: meta,
	}

	code := testCmd.Run(nil)
	output := done(t)
	if code != 0 {
		t.Fatalf("expected status code 0 but got %d: %s", code, output.All())
	}
}
