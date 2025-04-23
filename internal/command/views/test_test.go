// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/moduletest"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/terminal"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestTestHuman_Conclusion(t *testing.T) {
	tcs := map[string]struct {
		Suite    *moduletest.Suite
		Expected string
	}{
		"no tests": {
			Suite:    &moduletest.Suite{},
			Expected: "\nExecuted 0 tests.\n",
		},

		"only skipped tests": {
			Suite: &moduletest.Suite{
				Status: moduletest.Skip,
				Files: map[string]*moduletest.File{
					"descriptive_test_name.tftest.hcl": {
						Name:   "descriptive_test_name.tftest.hcl",
						Status: moduletest.Skip,
						Runs: []*moduletest.Run{
							{
								Name:   "test_one",
								Status: moduletest.Skip,
							},
							{
								Name:   "test_two",
								Status: moduletest.Skip,
							},
							{
								Name:   "test_three",
								Status: moduletest.Skip,
							},
						},
					},
					"other_descriptive_test_name.tftest.hcl": {
						Name:   "other_descriptive_test_name.tftest.hcl",
						Status: moduletest.Skip,
						Runs: []*moduletest.Run{
							{
								Name:   "test_one",
								Status: moduletest.Skip,
							},
							{
								Name:   "test_two",
								Status: moduletest.Skip,
							},
							{
								Name:   "test_three",
								Status: moduletest.Skip,
							},
						},
					},
				},
			},
			Expected: "\nExecuted 0 tests, 6 skipped.\n",
		},

		"only passed tests": {
			Suite: &moduletest.Suite{
				Status: moduletest.Pass,
				Files: map[string]*moduletest.File{
					"descriptive_test_name.tftest.hcl": {
						Name:   "descriptive_test_name.tftest.hcl",
						Status: moduletest.Pass,
						Runs: []*moduletest.Run{
							{
								Name:   "test_one",
								Status: moduletest.Pass,
							},
							{
								Name:   "test_two",
								Status: moduletest.Pass,
							},
							{
								Name:   "test_three",
								Status: moduletest.Pass,
							},
						},
					},
					"other_descriptive_test_name.tftest.hcl": {
						Name:   "other_descriptive_test_name.tftest.hcl",
						Status: moduletest.Pass,
						Runs: []*moduletest.Run{
							{
								Name:   "test_one",
								Status: moduletest.Pass,
							},
							{
								Name:   "test_two",
								Status: moduletest.Pass,
							},
							{
								Name:   "test_three",
								Status: moduletest.Pass,
							},
						},
					},
				},
			},
			Expected: "\nSuccess! 6 passed, 0 failed.\n",
		},

		"passed and skipped tests": {
			Suite: &moduletest.Suite{
				Status: moduletest.Pass,
				Files: map[string]*moduletest.File{
					"descriptive_test_name.tftest.hcl": {
						Name:   "descriptive_test_name.tftest.hcl",
						Status: moduletest.Pass,
						Runs: []*moduletest.Run{
							{
								Name:   "test_one",
								Status: moduletest.Pass,
							},
							{
								Name:   "test_two",
								Status: moduletest.Skip,
							},
							{
								Name:   "test_three",
								Status: moduletest.Pass,
							},
						},
					},
					"other_descriptive_test_name.tftest.hcl": {
						Name:   "other_descriptive_test_name.tftest.hcl",
						Status: moduletest.Pass,
						Runs: []*moduletest.Run{
							{
								Name:   "test_one",
								Status: moduletest.Skip,
							},
							{
								Name:   "test_two",
								Status: moduletest.Pass,
							},
							{
								Name:   "test_three",
								Status: moduletest.Pass,
							},
						},
					},
				},
			},
			Expected: "\nSuccess! 4 passed, 0 failed, 2 skipped.\n",
		},

		"only failed tests": {
			Suite: &moduletest.Suite{
				Status: moduletest.Fail,
				Files: map[string]*moduletest.File{
					"descriptive_test_name.tftest.hcl": {
						Name:   "descriptive_test_name.tftest.hcl",
						Status: moduletest.Fail,
						Runs: []*moduletest.Run{
							{
								Name:   "test_one",
								Status: moduletest.Fail,
							},
							{
								Name:   "test_two",
								Status: moduletest.Fail,
							},
							{
								Name:   "test_three",
								Status: moduletest.Fail,
							},
						},
					},
					"other_descriptive_test_name.tftest.hcl": {
						Name:   "other_descriptive_test_name.tftest.hcl",
						Status: moduletest.Fail,
						Runs: []*moduletest.Run{
							{
								Name:   "test_one",
								Status: moduletest.Fail,
							},
							{
								Name:   "test_two",
								Status: moduletest.Fail,
							},
							{
								Name:   "test_three",
								Status: moduletest.Fail,
							},
						},
					},
				},
			},
			Expected: "\nFailure! 0 passed, 6 failed.\n",
		},

		"failed and skipped tests": {
			Suite: &moduletest.Suite{
				Status: moduletest.Fail,
				Files: map[string]*moduletest.File{
					"descriptive_test_name.tftest.hcl": {
						Name:   "descriptive_test_name.tftest.hcl",
						Status: moduletest.Fail,
						Runs: []*moduletest.Run{
							{
								Name:   "test_one",
								Status: moduletest.Fail,
							},
							{
								Name:   "test_two",
								Status: moduletest.Skip,
							},
							{
								Name:   "test_three",
								Status: moduletest.Fail,
							},
						},
					},
					"other_descriptive_test_name.tftest.hcl": {
						Name:   "other_descriptive_test_name.tftest.hcl",
						Status: moduletest.Fail,
						Runs: []*moduletest.Run{
							{
								Name:   "test_one",
								Status: moduletest.Fail,
							},
							{
								Name:   "test_two",
								Status: moduletest.Fail,
							},
							{
								Name:   "test_three",
								Status: moduletest.Skip,
							},
						},
					},
				},
			},
			Expected: "\nFailure! 0 passed, 4 failed, 2 skipped.\n",
		},

		"failed, passed and skipped tests": {
			Suite: &moduletest.Suite{
				Status: moduletest.Fail,
				Files: map[string]*moduletest.File{
					"descriptive_test_name.tftest.hcl": {
						Name:   "descriptive_test_name.tftest.hcl",
						Status: moduletest.Fail,
						Runs: []*moduletest.Run{
							{
								Name:   "test_one",
								Status: moduletest.Fail,
							},
							{
								Name:   "test_two",
								Status: moduletest.Pass,
							},
							{
								Name:   "test_three",
								Status: moduletest.Skip,
							},
						},
					},
					"other_descriptive_test_name.tftest.hcl": {
						Name:   "other_descriptive_test_name.tftest.hcl",
						Status: moduletest.Fail,
						Runs: []*moduletest.Run{
							{
								Name:   "test_one",
								Status: moduletest.Skip,
							},
							{
								Name:   "test_two",
								Status: moduletest.Fail,
							},
							{
								Name:   "test_three",
								Status: moduletest.Pass,
							},
						},
					},
				},
			},
			Expected: "\nFailure! 2 passed, 2 failed, 2 skipped.\n",
		},

		"failed and errored tests": {
			Suite: &moduletest.Suite{
				Status: moduletest.Error,
				Files: map[string]*moduletest.File{
					"descriptive_test_name.tftest.hcl": {
						Name:   "descriptive_test_name.tftest.hcl",
						Status: moduletest.Error,
						Runs: []*moduletest.Run{
							{
								Name:   "test_one",
								Status: moduletest.Fail,
							},
							{
								Name:   "test_two",
								Status: moduletest.Error,
							},
							{
								Name:   "test_three",
								Status: moduletest.Fail,
							},
						},
					},
					"other_descriptive_test_name.tftest.hcl": {
						Name:   "other_descriptive_test_name.tftest.hcl",
						Status: moduletest.Error,
						Runs: []*moduletest.Run{
							{
								Name:   "test_one",
								Status: moduletest.Fail,
							},
							{
								Name:   "test_two",
								Status: moduletest.Error,
							},
							{
								Name:   "test_three",
								Status: moduletest.Error,
							},
						},
					},
				},
			},
			Expected: "\nFailure! 0 passed, 6 failed.\n",
		},

		"failed, errored, passed, and skipped tests": {
			Suite: &moduletest.Suite{
				Status: moduletest.Error,
				Files: map[string]*moduletest.File{
					"descriptive_test_name.tftest.hcl": {
						Name:   "descriptive_test_name.tftest.hcl",
						Status: moduletest.Fail,
						Runs: []*moduletest.Run{
							{
								Name:   "test_one",
								Status: moduletest.Pass,
							},
							{
								Name:   "test_two",
								Status: moduletest.Pass,
							},
							{
								Name:   "test_three",
								Status: moduletest.Fail,
							},
						},
					},
					"other_descriptive_test_name.tftest.hcl": {
						Name:   "other_descriptive_test_name.tftest.hcl",
						Status: moduletest.Error,
						Runs: []*moduletest.Run{
							{
								Name:   "test_one",
								Status: moduletest.Error,
							},
							{
								Name:   "test_two",
								Status: moduletest.Skip,
							},
							{
								Name:   "test_three",
								Status: moduletest.Skip,
							},
						},
					},
				},
			},
			Expected: "\nFailure! 2 passed, 2 failed, 2 skipped.\n",
		},
	}
	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {

			streams, done := terminal.StreamsForTesting(t)
			view := NewTest(arguments.ViewHuman, NewView(streams))

			view.Conclusion(tc.Suite)

			actual := done(t).Stdout()
			expected := tc.Expected
			if diff := cmp.Diff(expected, actual); len(diff) > 0 {
				t.Fatalf("expected:\n%s\nactual:\n%s\ndiff:\n%s", expected, actual, diff)
			}
		})
	}
}

func TestTestHuman_File(t *testing.T) {
	tcs := map[string]struct {
		File     *moduletest.File
		Expected string
	}{
		"pass": {
			File:     &moduletest.File{Name: "main.tf", Status: moduletest.Pass},
			Expected: "main.tf... pass\n",
		},

		"pending": {
			File:     &moduletest.File{Name: "main.tf", Status: moduletest.Pending},
			Expected: "main.tf... pending\n",
		},

		"skip": {
			File:     &moduletest.File{Name: "main.tf", Status: moduletest.Skip},
			Expected: "main.tf... skip\n",
		},

		"fail": {
			File:     &moduletest.File{Name: "main.tf", Status: moduletest.Fail},
			Expected: "main.tf... fail\n",
		},

		"error": {
			File:     &moduletest.File{Name: "main.tf", Status: moduletest.Error},
			Expected: "main.tf... fail\n",
		},
	}
	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {

			streams, done := terminal.StreamsForTesting(t)
			view := NewTest(arguments.ViewHuman, NewView(streams))

			view.File(tc.File)

			actual := done(t).Stdout()
			expected := tc.Expected
			if diff := cmp.Diff(expected, actual); len(diff) > 0 {
				t.Fatalf("expected:\n%s\nactual:\n%s\ndiff:\n%s", expected, actual, diff)
			}
		})
	}
}

func TestTestHuman_Run(t *testing.T) {
	tcs := map[string]struct {
		Run    *moduletest.Run
		StdOut string
		StdErr string
	}{
		"pass": {
			Run:    &moduletest.Run{Name: "run_block", Status: moduletest.Pass},
			StdOut: "  run \"run_block\"... pass\n",
		},

		"pass_with_diags": {
			Run: &moduletest.Run{
				Name:        "run_block",
				Status:      moduletest.Pass,
				Diagnostics: tfdiags.Diagnostics{tfdiags.Sourceless(tfdiags.Warning, "a warning occurred", "some warning happened during this test")},
			},
			StdOut: `  run "run_block"... pass

Warning: a warning occurred

some warning happened during this test
`,
		},

		"pending": {
			Run:    &moduletest.Run{Name: "run_block", Status: moduletest.Pending},
			StdOut: "  run \"run_block\"... pending\n",
		},

		"skip": {
			Run:    &moduletest.Run{Name: "run_block", Status: moduletest.Skip},
			StdOut: "  run \"run_block\"... skip\n",
		},

		"fail": {
			Run:    &moduletest.Run{Name: "run_block", Status: moduletest.Fail},
			StdOut: "  run \"run_block\"... fail\n",
		},

		"fail_with_diags": {
			Run: &moduletest.Run{
				Name:   "run_block",
				Status: moduletest.Fail,
				Diagnostics: tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Error, "a comparison failed", "details details details"),
					tfdiags.Sourceless(tfdiags.Error, "a second comparison failed", "other details"),
				},
			},
			StdOut: "  run \"run_block\"... fail\n",
			StdErr: `
Error: a comparison failed

details details details

Error: a second comparison failed

other details
`,
		},

		"error": {
			Run:    &moduletest.Run{Name: "run_block", Status: moduletest.Error},
			StdOut: "  run \"run_block\"... fail\n",
		},

		"error_with_diags": {
			Run: &moduletest.Run{
				Name:        "run_block",
				Status:      moduletest.Error,
				Diagnostics: tfdiags.Diagnostics{tfdiags.Sourceless(tfdiags.Error, "an error occurred", "something bad happened during this test")},
			},
			StdOut: "  run \"run_block\"... fail\n",
			StdErr: `
Error: an error occurred

something bad happened during this test
`,
		},
		"verbose_plan": {
			Run: &moduletest.Run{
				Name:   "run_block",
				Status: moduletest.Pass,
				Config: &configs.TestRun{
					Command: configs.PlanTestCommand,
				},
				Verbose: &moduletest.Verbose{
					Plan: &plans.Plan{
						Changes: &plans.Changes{
							Resources: []*plans.ResourceInstanceChangeSrc{
								{
									Addr: addrs.AbsResourceInstance{
										Module: addrs.RootModuleInstance,
										Resource: addrs.ResourceInstance{
											Resource: addrs.Resource{
												Mode: addrs.ManagedResourceMode,
												Type: "test_resource",
												Name: "creating",
											},
										},
									},
									PrevRunAddr: addrs.AbsResourceInstance{
										Module: addrs.RootModuleInstance,
										Resource: addrs.ResourceInstance{
											Resource: addrs.Resource{
												Mode: addrs.ManagedResourceMode,
												Type: "test_resource",
												Name: "creating",
											},
										},
									},
									ProviderAddr: addrs.AbsProviderConfig{
										Module: addrs.RootModule,
										Provider: addrs.Provider{
											Hostname:  addrs.DefaultProviderRegistryHost,
											Namespace: "hashicorp",
											Type:      "test",
										},
									},
									ChangeSrc: plans.ChangeSrc{
										Action: plans.Create,
										After: dynamicValue(
											t,
											cty.ObjectVal(map[string]cty.Value{
												"value": cty.StringVal("Hello, world!"),
											}),
											cty.Object(map[string]cty.Type{
												"value": cty.String,
											})),
									},
								},
							},
						},
					},
					State:  states.NewState(), // empty state
					Config: &configs.Config{},
					Providers: map[addrs.Provider]providers.ProviderSchema{
						addrs.Provider{
							Hostname:  addrs.DefaultProviderRegistryHost,
							Namespace: "hashicorp",
							Type:      "test",
						}: {
							ResourceTypes: map[string]providers.Schema{
								"test_resource": {
									Block: &configschema.Block{
										Attributes: map[string]*configschema.Attribute{
											"value": {
												Type: cty.String,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			StdOut: `  run "run_block"... pass

OpenTofu used the selected providers to generate the following execution
plan. Resource actions are indicated with the following symbols:
  + create

OpenTofu will perform the following actions:

  # test_resource.creating will be created
  + resource "test_resource" "creating" {
      + value = "Hello, world!"
    }

Plan: 1 to add, 0 to change, 0 to destroy.
`,
		},
		"verbose_apply": {
			Run: &moduletest.Run{
				Name:   "run_block",
				Status: moduletest.Pass,
				Config: &configs.TestRun{
					Command: configs.ApplyTestCommand,
				},
				Verbose: &moduletest.Verbose{
					Plan: &plans.Plan{}, // empty plan
					State: states.BuildState(func(state *states.SyncState) {
						state.SetResourceInstanceCurrent(
							addrs.AbsResourceInstance{
								Module: addrs.RootModuleInstance,
								Resource: addrs.ResourceInstance{
									Resource: addrs.Resource{
										Mode: addrs.ManagedResourceMode,
										Type: "test_resource",
										Name: "creating",
									},
								},
							},
							&states.ResourceInstanceObjectSrc{
								AttrsJSON: []byte(`{"value":"foobar"}`),
							},
							addrs.AbsProviderConfig{
								Module: addrs.RootModule,
								Provider: addrs.Provider{
									Hostname:  addrs.DefaultProviderRegistryHost,
									Namespace: "hashicorp",
									Type:      "test",
								},
							}, addrs.NoKey)
					}),
					Config: &configs.Config{},
					Providers: map[addrs.Provider]providers.ProviderSchema{
						addrs.Provider{
							Hostname:  addrs.DefaultProviderRegistryHost,
							Namespace: "hashicorp",
							Type:      "test",
						}: {
							ResourceTypes: map[string]providers.Schema{
								"test_resource": {
									Block: &configschema.Block{
										Attributes: map[string]*configschema.Attribute{
											"value": {
												Type: cty.String,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			StdOut: `  run "run_block"... pass
# test_resource.creating:
resource "test_resource" "creating" {
    value = "foobar"
}
`,
		},
	}
	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			file := &moduletest.File{
				Name: "main.tftest.hcl",
			}

			streams, done := terminal.StreamsForTesting(t)
			view := NewTest(arguments.ViewHuman, NewView(streams))

			view.Run(tc.Run, file)

			output := done(t)
			actual, expected := output.Stdout(), tc.StdOut
			if diff := cmp.Diff(expected, actual); len(diff) > 0 {
				t.Errorf("expected:\n%s\nactual:\n%s\ndiff:\n%s", expected, actual, diff)
			}

			actual, expected = output.Stderr(), tc.StdErr
			if diff := cmp.Diff(expected, actual); len(diff) > 0 {
				t.Errorf("expected:\n%s\nactual:\n%s\ndiff:\n%s", expected, actual, diff)
			}
		})
	}
}

func TestTestHuman_DestroySummary(t *testing.T) {
	tcs := map[string]struct {
		diags  tfdiags.Diagnostics
		run    *moduletest.Run
		file   *moduletest.File
		state  *states.State
		stdout string
		stderr string
	}{
		"empty": {
			diags: nil,
			file:  &moduletest.File{Name: "main.tftest.hcl"},
			state: states.NewState(),
		},
		"empty_state_only_warnings": {
			diags: tfdiags.Diagnostics{
				tfdiags.Sourceless(tfdiags.Warning, "first warning", "some thing not very bad happened"),
				tfdiags.Sourceless(tfdiags.Warning, "second warning", "some thing not very bad happened again"),
			},
			file:  &moduletest.File{Name: "main.tftest.hcl"},
			state: states.NewState(),
			stdout: `
Warning: first warning

some thing not very bad happened

Warning: second warning

some thing not very bad happened again
`,
		},
		"empty_state_with_errors": {
			diags: tfdiags.Diagnostics{
				tfdiags.Sourceless(tfdiags.Warning, "first warning", "some thing not very bad happened"),
				tfdiags.Sourceless(tfdiags.Warning, "second warning", "some thing not very bad happened again"),
				tfdiags.Sourceless(tfdiags.Error, "first error", "this time it is very bad"),
			},
			file:  &moduletest.File{Name: "main.tftest.hcl"},
			state: states.NewState(),
			stdout: `
Warning: first warning

some thing not very bad happened

Warning: second warning

some thing not very bad happened again
`,
			stderr: `OpenTofu encountered an error destroying resources created while executing
main.tftest.hcl.

Error: first error

this time it is very bad
`,
		},
		"error_from_run": {
			diags: tfdiags.Diagnostics{
				tfdiags.Sourceless(tfdiags.Error, "first error", "this time it is very bad"),
			},
			run:   &moduletest.Run{Name: "run_block"},
			file:  &moduletest.File{Name: "main.tftest.hcl"},
			state: states.NewState(),
			stderr: `OpenTofu encountered an error destroying resources created while executing
main.tftest.hcl/run_block.

Error: first error

this time it is very bad
`,
		},
		"state_only_warnings": {
			diags: tfdiags.Diagnostics{
				tfdiags.Sourceless(tfdiags.Warning, "first warning", "some thing not very bad happened"),
				tfdiags.Sourceless(tfdiags.Warning, "second warning", "some thing not very bad happened again"),
			},
			file: &moduletest.File{Name: "main.tftest.hcl"},
			state: states.BuildState(func(state *states.SyncState) {
				state.SetResourceInstanceCurrent(
					addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "test",
						Name: "foo",
					}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
					&states.ResourceInstanceObjectSrc{
						Status: states.ObjectReady,
					},
					addrs.AbsProviderConfig{
						Module:   addrs.RootModule,
						Provider: addrs.NewDefaultProvider("test"),
					}, addrs.NoKey)
				state.SetResourceInstanceCurrent(
					addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "test",
						Name: "bar",
					}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
					&states.ResourceInstanceObjectSrc{
						Status: states.ObjectReady,
					},
					addrs.AbsProviderConfig{
						Module:   addrs.RootModule,
						Provider: addrs.NewDefaultProvider("test"),
					}, addrs.NoKey)
				state.SetResourceInstanceDeposed(
					addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "test",
						Name: "bar",
					}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
					"0fcb640a",
					&states.ResourceInstanceObjectSrc{
						Status: states.ObjectReady,
					},
					addrs.AbsProviderConfig{
						Module:   addrs.RootModule,
						Provider: addrs.NewDefaultProvider("test"),
					}, addrs.NoKey)
			}),
			stdout: `
Warning: first warning

some thing not very bad happened

Warning: second warning

some thing not very bad happened again
`,
			stderr: `
OpenTofu left the following resources in state after executing
main.tftest.hcl, these left-over resources can be viewed by reading the
statefile written to disk(errored_test.tfstate) and they need to be cleaned
up manually:
  - test.bar
  - test.bar (0fcb640a)
  - test.foo
`,
		},
		"state_with_errors": {
			diags: tfdiags.Diagnostics{
				tfdiags.Sourceless(tfdiags.Warning, "first warning", "some thing not very bad happened"),
				tfdiags.Sourceless(tfdiags.Warning, "second warning", "some thing not very bad happened again"),
				tfdiags.Sourceless(tfdiags.Error, "first error", "this time it is very bad"),
			},
			file: &moduletest.File{Name: "main.tftest.hcl"},
			state: states.BuildState(func(state *states.SyncState) {
				state.SetResourceInstanceCurrent(
					addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "test",
						Name: "foo",
					}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
					&states.ResourceInstanceObjectSrc{
						Status: states.ObjectReady,
					},
					addrs.AbsProviderConfig{
						Module:   addrs.RootModule,
						Provider: addrs.NewDefaultProvider("test"),
					}, addrs.NoKey)
				state.SetResourceInstanceCurrent(
					addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "test",
						Name: "bar",
					}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
					&states.ResourceInstanceObjectSrc{
						Status: states.ObjectReady,
					},
					addrs.AbsProviderConfig{
						Module:   addrs.RootModule,
						Provider: addrs.NewDefaultProvider("test"),
					}, addrs.NoKey)
				state.SetResourceInstanceDeposed(
					addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "test",
						Name: "bar",
					}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
					"0fcb640a",
					&states.ResourceInstanceObjectSrc{
						Status: states.ObjectReady,
					},
					addrs.AbsProviderConfig{
						Module:   addrs.RootModule,
						Provider: addrs.NewDefaultProvider("test"),
					}, addrs.NoKey)
			}),
			stdout: `
Warning: first warning

some thing not very bad happened

Warning: second warning

some thing not very bad happened again
`,
			stderr: `OpenTofu encountered an error destroying resources created while executing
main.tftest.hcl.

Error: first error

this time it is very bad

OpenTofu left the following resources in state after executing
main.tftest.hcl, these left-over resources can be viewed by reading the
statefile written to disk(errored_test.tfstate) and they need to be cleaned
up manually:
  - test.bar
  - test.bar (0fcb640a)
  - test.foo
`,
		},
		"state_null_resource_with_errors": {
			diags: tfdiags.Diagnostics{
				tfdiags.Sourceless(tfdiags.Warning, "first warning", "some thing not very bad happened"),
				tfdiags.Sourceless(tfdiags.Warning, "second warning", "some thing not very bad happened again"),
				tfdiags.Sourceless(tfdiags.Error, "first error", "this time it is very bad"),
			},
			file: &moduletest.File{Name: "main.tftest.hcl"},
			state: states.BuildState(func(state *states.SyncState) {
				state.SetResourceInstanceCurrent(
					addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "null_resource",
						Name: "failing_will_depend_on_me",
					}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
					&states.ResourceInstanceObjectSrc{
						Status: states.ObjectReady,
					},
					addrs.AbsProviderConfig{
						Module:   addrs.RootModule,
						Provider: addrs.NewDefaultProvider("null"),
					}, addrs.NoKey)
				state.SetResourceInstanceCurrent(
					addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "null_resource",
						Name: "failing",
					}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
					&states.ResourceInstanceObjectSrc{
						Status: states.ObjectReady,
						Dependencies: []addrs.ConfigResource{
							{
								Module: []string{},
								Resource: addrs.Resource{
									Mode: addrs.ManagedResourceMode,
									Type: "null_resource",
									Name: "failing_will_depend_on_me",
								},
							},
						},
						CreateBeforeDestroy: false,
					},
					addrs.AbsProviderConfig{
						Module:   addrs.RootModule,
						Provider: addrs.NewDefaultProvider("null"),
					}, addrs.NoKey)
			}),
			stdout: `
Warning: first warning

some thing not very bad happened

Warning: second warning

some thing not very bad happened again
`,
			stderr: `OpenTofu encountered an error destroying resources created while executing
main.tftest.hcl.

Error: first error

this time it is very bad

OpenTofu left the following resources in state after executing
main.tftest.hcl, these left-over resources can be viewed by reading the
statefile written to disk(errored_test.tfstate) and they need to be cleaned
up manually:
  - null_resource.failing
  - null_resource.failing_will_depend_on_me
`,
		},
	}
	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			streams, done := terminal.StreamsForTesting(t)
			view := NewTest(arguments.ViewHuman, NewView(streams))

			view.DestroySummary(tc.diags, tc.run, tc.file, tc.state)

			output := done(t)
			actual, expected := output.Stdout(), tc.stdout
			if diff := cmp.Diff(expected, actual); len(diff) > 0 {
				t.Errorf("expected:\n%s\nactual:\n%s\ndiff:\n%s", expected, actual, diff)
			}

			actual, expected = output.Stderr(), tc.stderr
			if diff := cmp.Diff(expected, actual); len(diff) > 0 {
				t.Errorf("expected:\n%s\nactual:\n%s\ndiff:\n%s", expected, actual, diff)
			}
		})
	}
}

func TestTestHuman_FatalInterruptSummary(t *testing.T) {
	tcs := map[string]struct {
		states  map[*moduletest.Run]*states.State
		run     *moduletest.Run
		created []*plans.ResourceInstanceChangeSrc
		want    string
	}{
		"no_state_only_plan": {
			states: make(map[*moduletest.Run]*states.State),
			run: &moduletest.Run{
				Config: &configs.TestRun{},
				Name:   "run_block",
			},
			created: []*plans.ResourceInstanceChangeSrc{
				{
					Addr: addrs.AbsResourceInstance{
						Module: addrs.RootModuleInstance,
						Resource: addrs.ResourceInstance{
							Resource: addrs.Resource{
								Mode: addrs.ManagedResourceMode,
								Type: "test_instance",
								Name: "one",
							},
						},
					},
					ChangeSrc: plans.ChangeSrc{
						Action: plans.Create,
					},
				},
				{
					Addr: addrs.AbsResourceInstance{
						Module: addrs.RootModuleInstance,
						Resource: addrs.ResourceInstance{
							Resource: addrs.Resource{
								Mode: addrs.ManagedResourceMode,
								Type: "test_instance",
								Name: "two",
							},
						},
					},
					ChangeSrc: plans.ChangeSrc{
						Action: plans.Create,
					},
				},
			},
			want: `
OpenTofu was interrupted while executing main.tftest.hcl, and may not have
performed the expected cleanup operations.

OpenTofu was in the process of creating the following resources for
"run_block" from the module under test, and they may not have been destroyed:
  - test_instance.one
  - test_instance.two
`,
		},
		"file_state_no_plan": {
			states: map[*moduletest.Run]*states.State{
				nil: states.BuildState(func(state *states.SyncState) {
					state.SetResourceInstanceCurrent(
						addrs.AbsResourceInstance{
							Module: addrs.RootModuleInstance,
							Resource: addrs.ResourceInstance{
								Resource: addrs.Resource{
									Mode: addrs.ManagedResourceMode,
									Type: "test_instance",
									Name: "one",
								},
							},
						},
						&states.ResourceInstanceObjectSrc{},
						addrs.AbsProviderConfig{}, addrs.NoKey)

					state.SetResourceInstanceCurrent(
						addrs.AbsResourceInstance{
							Module: addrs.RootModuleInstance,
							Resource: addrs.ResourceInstance{
								Resource: addrs.Resource{
									Mode: addrs.ManagedResourceMode,
									Type: "test_instance",
									Name: "two",
								},
							},
						},
						&states.ResourceInstanceObjectSrc{},
						addrs.AbsProviderConfig{}, addrs.NoKey)
				}),
			},
			created: nil,
			want: `
OpenTofu was interrupted while executing main.tftest.hcl, and may not have
performed the expected cleanup operations.

OpenTofu has already created the following resources from the module under
test:
  - test_instance.one
  - test_instance.two
`,
		},
		"run_states_no_plan": {
			states: map[*moduletest.Run]*states.State{
				&moduletest.Run{
					Name: "setup_block",
					Config: &configs.TestRun{
						Module: &configs.TestRunModuleCall{
							Source: addrs.ModuleSourceLocal("../setup"),
						},
					},
				}: states.BuildState(func(state *states.SyncState) {
					state.SetResourceInstanceCurrent(
						addrs.AbsResourceInstance{
							Module: addrs.RootModuleInstance,
							Resource: addrs.ResourceInstance{
								Resource: addrs.Resource{
									Mode: addrs.ManagedResourceMode,
									Type: "test_instance",
									Name: "one",
								},
							},
						},
						&states.ResourceInstanceObjectSrc{},
						addrs.AbsProviderConfig{}, addrs.NoKey)

					state.SetResourceInstanceCurrent(
						addrs.AbsResourceInstance{
							Module: addrs.RootModuleInstance,
							Resource: addrs.ResourceInstance{
								Resource: addrs.Resource{
									Mode: addrs.ManagedResourceMode,
									Type: "test_instance",
									Name: "two",
								},
							},
						},
						&states.ResourceInstanceObjectSrc{},
						addrs.AbsProviderConfig{}, addrs.NoKey)
				}),
			},
			created: nil,
			want: `
OpenTofu was interrupted while executing main.tftest.hcl, and may not have
performed the expected cleanup operations.

OpenTofu has already created the following resources for "setup_block" from
"../setup":
  - test_instance.one
  - test_instance.two
`,
		},
		"all_states_with_plan": {
			states: map[*moduletest.Run]*states.State{
				&moduletest.Run{
					Name: "setup_block",
					Config: &configs.TestRun{
						Module: &configs.TestRunModuleCall{
							Source: addrs.ModuleSourceLocal("../setup"),
						},
					},
				}: states.BuildState(func(state *states.SyncState) {
					state.SetResourceInstanceCurrent(
						addrs.AbsResourceInstance{
							Module: addrs.RootModuleInstance,
							Resource: addrs.ResourceInstance{
								Resource: addrs.Resource{
									Mode: addrs.ManagedResourceMode,
									Type: "test_instance",
									Name: "setup_one",
								},
							},
						},
						&states.ResourceInstanceObjectSrc{},
						addrs.AbsProviderConfig{}, addrs.NoKey)

					state.SetResourceInstanceCurrent(
						addrs.AbsResourceInstance{
							Module: addrs.RootModuleInstance,
							Resource: addrs.ResourceInstance{
								Resource: addrs.Resource{
									Mode: addrs.ManagedResourceMode,
									Type: "test_instance",
									Name: "setup_two",
								},
							},
						},
						&states.ResourceInstanceObjectSrc{},
						addrs.AbsProviderConfig{}, addrs.NoKey)
				}),
				nil: states.BuildState(func(state *states.SyncState) {
					state.SetResourceInstanceCurrent(
						addrs.AbsResourceInstance{
							Module: addrs.RootModuleInstance,
							Resource: addrs.ResourceInstance{
								Resource: addrs.Resource{
									Mode: addrs.ManagedResourceMode,
									Type: "test_instance",
									Name: "one",
								},
							},
						},
						&states.ResourceInstanceObjectSrc{},
						addrs.AbsProviderConfig{}, addrs.NoKey)

					state.SetResourceInstanceCurrent(
						addrs.AbsResourceInstance{
							Module: addrs.RootModuleInstance,
							Resource: addrs.ResourceInstance{
								Resource: addrs.Resource{
									Mode: addrs.ManagedResourceMode,
									Type: "test_instance",
									Name: "two",
								},
							},
						},
						&states.ResourceInstanceObjectSrc{},
						addrs.AbsProviderConfig{}, addrs.NoKey)
				}),
			},
			created: []*plans.ResourceInstanceChangeSrc{
				{
					Addr: addrs.AbsResourceInstance{
						Module: addrs.RootModuleInstance,
						Resource: addrs.ResourceInstance{
							Resource: addrs.Resource{
								Mode: addrs.ManagedResourceMode,
								Type: "test_instance",
								Name: "new_one",
							},
						},
					},
					ChangeSrc: plans.ChangeSrc{
						Action: plans.Create,
					},
				},
				{
					Addr: addrs.AbsResourceInstance{
						Module: addrs.RootModuleInstance,
						Resource: addrs.ResourceInstance{
							Resource: addrs.Resource{
								Mode: addrs.ManagedResourceMode,
								Type: "test_instance",
								Name: "new_two",
							},
						},
					},
					ChangeSrc: plans.ChangeSrc{
						Action: plans.Create,
					},
				},
			},
			run: &moduletest.Run{
				Config: &configs.TestRun{},
				Name:   "run_block",
			},
			want: `
OpenTofu was interrupted while executing main.tftest.hcl, and may not have
performed the expected cleanup operations.

OpenTofu has already created the following resources from the module under
test:
  - test_instance.one
  - test_instance.two

OpenTofu has already created the following resources for "setup_block" from
"../setup":
  - test_instance.setup_one
  - test_instance.setup_two

OpenTofu was in the process of creating the following resources for
"run_block" from the module under test, and they may not have been destroyed:
  - test_instance.new_one
  - test_instance.new_two
`,
		},
	}
	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			streams, done := terminal.StreamsForTesting(t)
			view := NewTest(arguments.ViewHuman, NewView(streams))

			file := &moduletest.File{
				Name: "main.tftest.hcl",
				Runs: func() []*moduletest.Run {
					var runs []*moduletest.Run
					for run := range tc.states {
						if run != nil {
							runs = append(runs, run)
						}
					}
					return runs
				}(),
			}

			view.FatalInterruptSummary(tc.run, file, tc.states, tc.created)
			actual, expected := done(t).Stderr(), tc.want
			if diff := cmp.Diff(expected, actual); len(diff) > 0 {
				t.Errorf("expected:\n%s\nactual:\n%s\ndiff:\n%s", expected, actual, diff)
			}
		})
	}
}

func TestTestJSON_Abstract(t *testing.T) {
	tcs := map[string]struct {
		suite *moduletest.Suite
		want  []map[string]interface{}
	}{
		"single": {
			suite: &moduletest.Suite{
				Files: map[string]*moduletest.File{
					"main.tftest.hcl": {
						Runs: []*moduletest.Run{
							{
								Name: "setup",
							},
						},
					},
				},
			},
			want: []map[string]interface{}{
				{
					"@level":   "info",
					"@message": "Found 1 file and 1 run block",
					"@module":  "tofu.ui",
					"test_abstract": map[string]interface{}{
						"main.tftest.hcl": []interface{}{
							"setup",
						},
					},
					"type": "test_abstract",
				},
			},
		},
		"plural": {
			suite: &moduletest.Suite{
				Files: map[string]*moduletest.File{
					"main.tftest.hcl": {
						Runs: []*moduletest.Run{
							{
								Name: "setup",
							},
							{
								Name: "test",
							},
						},
					},
					"other.tftest.hcl": {
						Runs: []*moduletest.Run{
							{
								Name: "test",
							},
						},
					},
				},
			},
			want: []map[string]interface{}{
				{
					"@level":   "info",
					"@message": "Found 2 files and 3 run blocks",
					"@module":  "tofu.ui",
					"test_abstract": map[string]interface{}{
						"main.tftest.hcl": []interface{}{
							"setup",
							"test",
						},
						"other.tftest.hcl": []interface{}{
							"test",
						},
					},
					"type": "test_abstract",
				},
			},
		},
	}
	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			streams, done := terminal.StreamsForTesting(t)
			view := NewTest(arguments.ViewJSON, NewView(streams))

			view.Abstract(tc.suite)
			testJSONViewOutputEquals(t, done(t).All(), tc.want)
		})
	}
}

func TestTestJSON_Conclusion(t *testing.T) {
	tcs := map[string]struct {
		suite *moduletest.Suite
		want  []map[string]interface{}
	}{
		"no tests": {
			suite: &moduletest.Suite{},
			want: []map[string]interface{}{
				{
					"@level":   "info",
					"@message": "Executed 0 tests.",
					"@module":  "tofu.ui",
					"test_summary": map[string]interface{}{
						"status":  "pending",
						"errored": 0.0,
						"failed":  0.0,
						"passed":  0.0,
						"skipped": 0.0,
					},
					"type": "test_summary",
				},
			},
		},

		"only skipped tests": {
			suite: &moduletest.Suite{
				Status: moduletest.Skip,
				Files: map[string]*moduletest.File{
					"descriptive_test_name.tftest.hcl": {
						Name:   "descriptive_test_name.tftest.hcl",
						Status: moduletest.Skip,
						Runs: []*moduletest.Run{
							{
								Name:   "test_one",
								Status: moduletest.Skip,
							},
							{
								Name:   "test_two",
								Status: moduletest.Skip,
							},
							{
								Name:   "test_three",
								Status: moduletest.Skip,
							},
						},
					},
					"other_descriptive_test_name.tftest.hcl": {
						Name:   "other_descriptive_test_name.tftest.hcl",
						Status: moduletest.Skip,
						Runs: []*moduletest.Run{
							{
								Name:   "test_one",
								Status: moduletest.Skip,
							},
							{
								Name:   "test_two",
								Status: moduletest.Skip,
							},
							{
								Name:   "test_three",
								Status: moduletest.Skip,
							},
						},
					},
				},
			},
			want: []map[string]interface{}{
				{
					"@level":   "info",
					"@message": "Executed 0 tests, 6 skipped.",
					"@module":  "tofu.ui",
					"test_summary": map[string]interface{}{
						"status":  "skip",
						"errored": 0.0,
						"failed":  0.0,
						"passed":  0.0,
						"skipped": 6.0,
					},
					"type": "test_summary",
				},
			},
		},

		"only passed tests": {
			suite: &moduletest.Suite{
				Status: moduletest.Pass,
				Files: map[string]*moduletest.File{
					"descriptive_test_name.tftest.hcl": {
						Name:   "descriptive_test_name.tftest.hcl",
						Status: moduletest.Pass,
						Runs: []*moduletest.Run{
							{
								Name:   "test_one",
								Status: moduletest.Pass,
							},
							{
								Name:   "test_two",
								Status: moduletest.Pass,
							},
							{
								Name:   "test_three",
								Status: moduletest.Pass,
							},
						},
					},
					"other_descriptive_test_name.tftest.hcl": {
						Name:   "other_descriptive_test_name.tftest.hcl",
						Status: moduletest.Pass,
						Runs: []*moduletest.Run{
							{
								Name:   "test_one",
								Status: moduletest.Pass,
							},
							{
								Name:   "test_two",
								Status: moduletest.Pass,
							},
							{
								Name:   "test_three",
								Status: moduletest.Pass,
							},
						},
					},
				},
			},
			want: []map[string]interface{}{
				{
					"@level":   "info",
					"@message": "Success! 6 passed, 0 failed.",
					"@module":  "tofu.ui",
					"test_summary": map[string]interface{}{
						"status":  "pass",
						"errored": 0.0,
						"failed":  0.0,
						"passed":  6.0,
						"skipped": 0.0,
					},
					"type": "test_summary",
				},
			},
		},

		"passed and skipped tests": {
			suite: &moduletest.Suite{
				Status: moduletest.Pass,
				Files: map[string]*moduletest.File{
					"descriptive_test_name.tftest.hcl": {
						Name:   "descriptive_test_name.tftest.hcl",
						Status: moduletest.Pass,
						Runs: []*moduletest.Run{
							{
								Name:   "test_one",
								Status: moduletest.Pass,
							},
							{
								Name:   "test_two",
								Status: moduletest.Skip,
							},
							{
								Name:   "test_three",
								Status: moduletest.Pass,
							},
						},
					},
					"other_descriptive_test_name.tftest.hcl": {
						Name:   "other_descriptive_test_name.tftest.hcl",
						Status: moduletest.Pass,
						Runs: []*moduletest.Run{
							{
								Name:   "test_one",
								Status: moduletest.Skip,
							},
							{
								Name:   "test_two",
								Status: moduletest.Pass,
							},
							{
								Name:   "test_three",
								Status: moduletest.Pass,
							},
						},
					},
				},
			},
			want: []map[string]interface{}{
				{
					"@level":   "info",
					"@message": "Success! 4 passed, 0 failed, 2 skipped.",
					"@module":  "tofu.ui",
					"test_summary": map[string]interface{}{
						"status":  "pass",
						"errored": 0.0,
						"failed":  0.0,
						"passed":  4.0,
						"skipped": 2.0,
					},
					"type": "test_summary",
				},
			},
		},

		"only failed tests": {
			suite: &moduletest.Suite{
				Status: moduletest.Fail,
				Files: map[string]*moduletest.File{
					"descriptive_test_name.tftest.hcl": {
						Name:   "descriptive_test_name.tftest.hcl",
						Status: moduletest.Fail,
						Runs: []*moduletest.Run{
							{
								Name:   "test_one",
								Status: moduletest.Fail,
							},
							{
								Name:   "test_two",
								Status: moduletest.Fail,
							},
							{
								Name:   "test_three",
								Status: moduletest.Fail,
							},
						},
					},
					"other_descriptive_test_name.tftest.hcl": {
						Name:   "other_descriptive_test_name.tftest.hcl",
						Status: moduletest.Fail,
						Runs: []*moduletest.Run{
							{
								Name:   "test_one",
								Status: moduletest.Fail,
							},
							{
								Name:   "test_two",
								Status: moduletest.Fail,
							},
							{
								Name:   "test_three",
								Status: moduletest.Fail,
							},
						},
					},
				},
			},
			want: []map[string]interface{}{
				{
					"@level":   "info",
					"@message": "Failure! 0 passed, 6 failed.",
					"@module":  "tofu.ui",
					"test_summary": map[string]interface{}{
						"status":  "fail",
						"errored": 0.0,
						"failed":  6.0,
						"passed":  0.0,
						"skipped": 0.0,
					},
					"type": "test_summary",
				},
			},
		},

		"failed and skipped tests": {
			suite: &moduletest.Suite{
				Status: moduletest.Fail,
				Files: map[string]*moduletest.File{
					"descriptive_test_name.tftest.hcl": {
						Name:   "descriptive_test_name.tftest.hcl",
						Status: moduletest.Fail,
						Runs: []*moduletest.Run{
							{
								Name:   "test_one",
								Status: moduletest.Fail,
							},
							{
								Name:   "test_two",
								Status: moduletest.Skip,
							},
							{
								Name:   "test_three",
								Status: moduletest.Fail,
							},
						},
					},
					"other_descriptive_test_name.tftest.hcl": {
						Name:   "other_descriptive_test_name.tftest.hcl",
						Status: moduletest.Fail,
						Runs: []*moduletest.Run{
							{
								Name:   "test_one",
								Status: moduletest.Fail,
							},
							{
								Name:   "test_two",
								Status: moduletest.Fail,
							},
							{
								Name:   "test_three",
								Status: moduletest.Skip,
							},
						},
					},
				},
			},
			want: []map[string]interface{}{
				{
					"@level":   "info",
					"@message": "Failure! 0 passed, 4 failed, 2 skipped.",
					"@module":  "tofu.ui",
					"test_summary": map[string]interface{}{
						"status":  "fail",
						"errored": 0.0,
						"failed":  4.0,
						"passed":  0.0,
						"skipped": 2.0,
					},
					"type": "test_summary",
				},
			},
		},

		"failed, passed and skipped tests": {
			suite: &moduletest.Suite{
				Status: moduletest.Fail,
				Files: map[string]*moduletest.File{
					"descriptive_test_name.tftest.hcl": {
						Name:   "descriptive_test_name.tftest.hcl",
						Status: moduletest.Fail,
						Runs: []*moduletest.Run{
							{
								Name:   "test_one",
								Status: moduletest.Fail,
							},
							{
								Name:   "test_two",
								Status: moduletest.Pass,
							},
							{
								Name:   "test_three",
								Status: moduletest.Skip,
							},
						},
					},
					"other_descriptive_test_name.tftest.hcl": {
						Name:   "other_descriptive_test_name.tftest.hcl",
						Status: moduletest.Fail,
						Runs: []*moduletest.Run{
							{
								Name:   "test_one",
								Status: moduletest.Skip,
							},
							{
								Name:   "test_two",
								Status: moduletest.Fail,
							},
							{
								Name:   "test_three",
								Status: moduletest.Pass,
							},
						},
					},
				},
			},
			want: []map[string]interface{}{
				{
					"@level":   "info",
					"@message": "Failure! 2 passed, 2 failed, 2 skipped.",
					"@module":  "tofu.ui",
					"test_summary": map[string]interface{}{
						"status":  "fail",
						"errored": 0.0,
						"failed":  2.0,
						"passed":  2.0,
						"skipped": 2.0,
					},
					"type": "test_summary",
				},
			},
		},

		"failed and errored tests": {
			suite: &moduletest.Suite{
				Status: moduletest.Error,
				Files: map[string]*moduletest.File{
					"descriptive_test_name.tftest.hcl": {
						Name:   "descriptive_test_name.tftest.hcl",
						Status: moduletest.Error,
						Runs: []*moduletest.Run{
							{
								Name:   "test_one",
								Status: moduletest.Fail,
							},
							{
								Name:   "test_two",
								Status: moduletest.Error,
							},
							{
								Name:   "test_three",
								Status: moduletest.Fail,
							},
						},
					},
					"other_descriptive_test_name.tftest.hcl": {
						Name:   "other_descriptive_test_name.tftest.hcl",
						Status: moduletest.Error,
						Runs: []*moduletest.Run{
							{
								Name:   "test_one",
								Status: moduletest.Fail,
							},
							{
								Name:   "test_two",
								Status: moduletest.Error,
							},
							{
								Name:   "test_three",
								Status: moduletest.Error,
							},
						},
					},
				},
			},
			want: []map[string]interface{}{
				{
					"@level":   "info",
					"@message": "Failure! 0 passed, 6 failed.",
					"@module":  "tofu.ui",
					"test_summary": map[string]interface{}{
						"status":  "error",
						"errored": 3.0,
						"failed":  3.0,
						"passed":  0.0,
						"skipped": 0.0,
					},
					"type": "test_summary",
				},
			},
		},

		"failed, errored, passed, and skipped tests": {
			suite: &moduletest.Suite{
				Status: moduletest.Error,
				Files: map[string]*moduletest.File{
					"descriptive_test_name.tftest.hcl": {
						Name:   "descriptive_test_name.tftest.hcl",
						Status: moduletest.Fail,
						Runs: []*moduletest.Run{
							{
								Name:   "test_one",
								Status: moduletest.Pass,
							},
							{
								Name:   "test_two",
								Status: moduletest.Pass,
							},
							{
								Name:   "test_three",
								Status: moduletest.Fail,
							},
						},
					},
					"other_descriptive_test_name.tftest.hcl": {
						Name:   "other_descriptive_test_name.tftest.hcl",
						Status: moduletest.Error,
						Runs: []*moduletest.Run{
							{
								Name:   "test_one",
								Status: moduletest.Error,
							},
							{
								Name:   "test_two",
								Status: moduletest.Skip,
							},
							{
								Name:   "test_three",
								Status: moduletest.Skip,
							},
						},
					},
				},
			},
			want: []map[string]interface{}{
				{
					"@level":   "info",
					"@message": "Failure! 2 passed, 2 failed, 2 skipped.",
					"@module":  "tofu.ui",
					"test_summary": map[string]interface{}{
						"status":  "error",
						"errored": 1.0,
						"failed":  1.0,
						"passed":  2.0,
						"skipped": 2.0,
					},
					"type": "test_summary",
				},
			},
		},
	}
	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			streams, done := terminal.StreamsForTesting(t)
			view := NewTest(arguments.ViewJSON, NewView(streams))

			view.Conclusion(tc.suite)
			testJSONViewOutputEquals(t, done(t).All(), tc.want)
		})
	}
}

func TestTestJSON_DestroySummary(t *testing.T) {
	tcs := map[string]struct {
		file  *moduletest.File
		run   *moduletest.Run
		state *states.State
		diags tfdiags.Diagnostics
		want  []map[string]interface{}
	}{
		"empty_state_only_warnings": {
			diags: tfdiags.Diagnostics{
				tfdiags.Sourceless(tfdiags.Warning, "first warning", "something not very bad happened"),
				tfdiags.Sourceless(tfdiags.Warning, "second warning", "something not very bad happened again"),
			},
			file:  &moduletest.File{Name: "main.tftest.hcl"},
			state: states.NewState(),
			want: []map[string]interface{}{
				{
					"@level":    "warn",
					"@message":  "Warning: first warning",
					"@module":   "tofu.ui",
					"@testfile": "main.tftest.hcl",
					"diagnostic": map[string]interface{}{
						"detail":   "something not very bad happened",
						"severity": "warning",
						"summary":  "first warning",
					},
					"type": "diagnostic",
				},
				{
					"@level":    "warn",
					"@message":  "Warning: second warning",
					"@module":   "tofu.ui",
					"@testfile": "main.tftest.hcl",
					"diagnostic": map[string]interface{}{
						"detail":   "something not very bad happened again",
						"severity": "warning",
						"summary":  "second warning",
					},
					"type": "diagnostic",
				},
			},
		},
		"empty_state_with_errors": {
			diags: tfdiags.Diagnostics{
				tfdiags.Sourceless(tfdiags.Warning, "first warning", "something not very bad happened"),
				tfdiags.Sourceless(tfdiags.Warning, "second warning", "something not very bad happened again"),
				tfdiags.Sourceless(tfdiags.Error, "first error", "this time it is very bad"),
			},
			file:  &moduletest.File{Name: "main.tftest.hcl"},
			state: states.NewState(),
			want: []map[string]interface{}{
				{
					"@level":    "warn",
					"@message":  "Warning: first warning",
					"@module":   "tofu.ui",
					"@testfile": "main.tftest.hcl",
					"diagnostic": map[string]interface{}{
						"detail":   "something not very bad happened",
						"severity": "warning",
						"summary":  "first warning",
					},
					"type": "diagnostic",
				},
				{
					"@level":    "warn",
					"@message":  "Warning: second warning",
					"@module":   "tofu.ui",
					"@testfile": "main.tftest.hcl",
					"diagnostic": map[string]interface{}{
						"detail":   "something not very bad happened again",
						"severity": "warning",
						"summary":  "second warning",
					},
					"type": "diagnostic",
				},
				{
					"@level":    "error",
					"@message":  "Error: first error",
					"@module":   "tofu.ui",
					"@testfile": "main.tftest.hcl",
					"diagnostic": map[string]interface{}{
						"detail":   "this time it is very bad",
						"severity": "error",
						"summary":  "first error",
					},
					"type": "diagnostic",
				},
			},
		},
		"state_from_run": {
			file: &moduletest.File{Name: "main.tftest.hcl"},
			run:  &moduletest.Run{Name: "run_block"},
			state: states.BuildState(func(state *states.SyncState) {
				state.SetResourceInstanceCurrent(
					addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "test",
						Name: "foo",
					}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
					&states.ResourceInstanceObjectSrc{
						Status: states.ObjectReady,
					},
					addrs.AbsProviderConfig{
						Module:   addrs.RootModule,
						Provider: addrs.NewDefaultProvider("test"),
					}, addrs.NoKey)
			}),
			want: []map[string]interface{}{
				{
					"@level":    "error",
					"@message":  "OpenTofu left some resources in state after executing main.tftest.hcl/run_block, these left-over resources can be viewed by reading the statefile written to disk(errored_test.tfstate) and they need to be cleaned up manually:",
					"@module":   "tofu.ui",
					"@testfile": "main.tftest.hcl",
					"@testrun":  "run_block",
					"test_cleanup": map[string]interface{}{
						"failed_resources": []interface{}{
							map[string]interface{}{
								"instance": "test.foo",
							},
						},
					},
					"type": "test_cleanup",
				},
			},
		},
		"state_only_warnings": {
			diags: tfdiags.Diagnostics{
				tfdiags.Sourceless(tfdiags.Warning, "first warning", "something not very bad happened"),
				tfdiags.Sourceless(tfdiags.Warning, "second warning", "something not very bad happened again"),
			},
			file: &moduletest.File{Name: "main.tftest.hcl"},
			state: states.BuildState(func(state *states.SyncState) {
				state.SetResourceInstanceCurrent(
					addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "test",
						Name: "foo",
					}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
					&states.ResourceInstanceObjectSrc{
						Status: states.ObjectReady,
					},
					addrs.AbsProviderConfig{
						Module:   addrs.RootModule,
						Provider: addrs.NewDefaultProvider("test"),
					}, addrs.NoKey)
				state.SetResourceInstanceCurrent(
					addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "test",
						Name: "bar",
					}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
					&states.ResourceInstanceObjectSrc{
						Status: states.ObjectReady,
					},
					addrs.AbsProviderConfig{
						Module:   addrs.RootModule,
						Provider: addrs.NewDefaultProvider("test"),
					}, addrs.NoKey)
				state.SetResourceInstanceDeposed(
					addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "test",
						Name: "bar",
					}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
					"0fcb640a",
					&states.ResourceInstanceObjectSrc{
						Status: states.ObjectReady,
					},
					addrs.AbsProviderConfig{
						Module:   addrs.RootModule,
						Provider: addrs.NewDefaultProvider("test"),
					}, addrs.NoKey)
			}),
			want: []map[string]interface{}{
				{
					"@level":    "error",
					"@message":  "OpenTofu left some resources in state after executing main.tftest.hcl, these left-over resources can be viewed by reading the statefile written to disk(errored_test.tfstate) and they need to be cleaned up manually:",
					"@module":   "tofu.ui",
					"@testfile": "main.tftest.hcl",
					"test_cleanup": map[string]interface{}{
						"failed_resources": []interface{}{
							map[string]interface{}{
								"instance": "test.bar",
							},
							map[string]interface{}{
								"instance":    "test.bar",
								"deposed_key": "0fcb640a",
							},
							map[string]interface{}{
								"instance": "test.foo",
							},
						},
					},
					"type": "test_cleanup",
				},
				{
					"@level":    "warn",
					"@message":  "Warning: first warning",
					"@module":   "tofu.ui",
					"@testfile": "main.tftest.hcl",
					"diagnostic": map[string]interface{}{
						"detail":   "something not very bad happened",
						"severity": "warning",
						"summary":  "first warning",
					},
					"type": "diagnostic",
				},
				{
					"@level":    "warn",
					"@message":  "Warning: second warning",
					"@module":   "tofu.ui",
					"@testfile": "main.tftest.hcl",
					"diagnostic": map[string]interface{}{
						"detail":   "something not very bad happened again",
						"severity": "warning",
						"summary":  "second warning",
					},
					"type": "diagnostic",
				},
			},
		},
		"state_with_errors": {
			diags: tfdiags.Diagnostics{
				tfdiags.Sourceless(tfdiags.Warning, "first warning", "something not very bad happened"),
				tfdiags.Sourceless(tfdiags.Warning, "second warning", "something not very bad happened again"),
				tfdiags.Sourceless(tfdiags.Error, "first error", "this time it is very bad"),
			},
			file: &moduletest.File{Name: "main.tftest.hcl"},
			state: states.BuildState(func(state *states.SyncState) {
				state.SetResourceInstanceCurrent(
					addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "test",
						Name: "foo",
					}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
					&states.ResourceInstanceObjectSrc{
						Status: states.ObjectReady,
					},
					addrs.AbsProviderConfig{
						Module:   addrs.RootModule,
						Provider: addrs.NewDefaultProvider("test"),
					}, addrs.NoKey)
				state.SetResourceInstanceCurrent(
					addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "test",
						Name: "bar",
					}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
					&states.ResourceInstanceObjectSrc{
						Status: states.ObjectReady,
					},
					addrs.AbsProviderConfig{
						Module:   addrs.RootModule,
						Provider: addrs.NewDefaultProvider("test"),
					}, addrs.NoKey)
				state.SetResourceInstanceDeposed(
					addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "test",
						Name: "bar",
					}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
					"0fcb640a",
					&states.ResourceInstanceObjectSrc{
						Status: states.ObjectReady,
					},
					addrs.AbsProviderConfig{
						Module:   addrs.RootModule,
						Provider: addrs.NewDefaultProvider("test"),
					}, addrs.NoKey)
			}),
			want: []map[string]interface{}{
				{
					"@level":    "error",
					"@message":  "OpenTofu left some resources in state after executing main.tftest.hcl, these left-over resources can be viewed by reading the statefile written to disk(errored_test.tfstate) and they need to be cleaned up manually:",
					"@module":   "tofu.ui",
					"@testfile": "main.tftest.hcl",
					"test_cleanup": map[string]interface{}{
						"failed_resources": []interface{}{
							map[string]interface{}{
								"instance": "test.bar",
							},
							map[string]interface{}{
								"instance":    "test.bar",
								"deposed_key": "0fcb640a",
							},
							map[string]interface{}{
								"instance": "test.foo",
							},
						},
					},
					"type": "test_cleanup",
				},
				{
					"@level":    "warn",
					"@message":  "Warning: first warning",
					"@module":   "tofu.ui",
					"@testfile": "main.tftest.hcl",
					"diagnostic": map[string]interface{}{
						"detail":   "something not very bad happened",
						"severity": "warning",
						"summary":  "first warning",
					},
					"type": "diagnostic",
				},
				{
					"@level":    "warn",
					"@message":  "Warning: second warning",
					"@module":   "tofu.ui",
					"@testfile": "main.tftest.hcl",
					"diagnostic": map[string]interface{}{
						"detail":   "something not very bad happened again",
						"severity": "warning",
						"summary":  "second warning",
					},
					"type": "diagnostic",
				},
				{
					"@level":    "error",
					"@message":  "Error: first error",
					"@module":   "tofu.ui",
					"@testfile": "main.tftest.hcl",
					"diagnostic": map[string]interface{}{
						"detail":   "this time it is very bad",
						"severity": "error",
						"summary":  "first error",
					},
					"type": "diagnostic",
				},
			},
		},
		"state_null_resource_with_errors": {
			diags: tfdiags.Diagnostics{
				tfdiags.Sourceless(tfdiags.Warning, "first warning", "something not very bad happened"),
				tfdiags.Sourceless(tfdiags.Warning, "second warning", "something not very bad happened again"),
				tfdiags.Sourceless(tfdiags.Error, "first error", "this time it is very bad"),
			},
			file: &moduletest.File{Name: "main.tftest.hcl"},
			state: states.BuildState(func(state *states.SyncState) {
				state.SetResourceInstanceCurrent(
					addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "null_resource",
						Name: "failing_will_depend_on_me",
					}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
					&states.ResourceInstanceObjectSrc{
						Status: states.ObjectReady,
					},
					addrs.AbsProviderConfig{
						Module:   addrs.RootModule,
						Provider: addrs.NewDefaultProvider("null"),
					}, addrs.NoKey)
				state.SetResourceInstanceCurrent(
					addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "null_resource",
						Name: "failing",
					}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
					&states.ResourceInstanceObjectSrc{
						Status: states.ObjectReady,
						Dependencies: []addrs.ConfigResource{
							{
								Module: []string{},
								Resource: addrs.Resource{
									Mode: addrs.ManagedResourceMode,
									Type: "null_resource",
									Name: "failing_will_depend_on_me",
								},
							},
						},
						CreateBeforeDestroy: false,
					},
					addrs.AbsProviderConfig{
						Module:   addrs.RootModule,
						Provider: addrs.NewDefaultProvider("null"),
					}, addrs.NoKey)
			}), want: []map[string]interface{}{
				{
					"@level":    "error",
					"@message":  "OpenTofu left some resources in state after executing main.tftest.hcl, these left-over resources can be viewed by reading the statefile written to disk(errored_test.tfstate) and they need to be cleaned up manually:",
					"@module":   "tofu.ui",
					"@testfile": "main.tftest.hcl",
					"test_cleanup": map[string]interface{}{
						"failed_resources": []interface{}{
							map[string]interface{}{
								"instance": "null_resource.failing",
							},
							map[string]interface{}{
								"instance": "null_resource.failing_will_depend_on_me",
							},
						},
					},
					"type": "test_cleanup",
				},
				{
					"@level":    "warn",
					"@message":  "Warning: first warning",
					"@module":   "tofu.ui",
					"@testfile": "main.tftest.hcl",
					"diagnostic": map[string]interface{}{
						"detail":   "something not very bad happened",
						"severity": "warning",
						"summary":  "first warning",
					},
					"type": "diagnostic",
				},
				{
					"@level":    "warn",
					"@message":  "Warning: second warning",
					"@module":   "tofu.ui",
					"@testfile": "main.tftest.hcl",
					"diagnostic": map[string]interface{}{
						"detail":   "something not very bad happened again",
						"severity": "warning",
						"summary":  "second warning",
					},
					"type": "diagnostic",
				},
				{
					"@level":    "error",
					"@message":  "Error: first error",
					"@module":   "tofu.ui",
					"@testfile": "main.tftest.hcl",
					"diagnostic": map[string]interface{}{
						"detail":   "this time it is very bad",
						"severity": "error",
						"summary":  "first error",
					},
					"type": "diagnostic",
				},
			},
		},
	}
	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			streams, done := terminal.StreamsForTesting(t)
			view := NewTest(arguments.ViewJSON, NewView(streams))

			view.DestroySummary(tc.diags, tc.run, tc.file, tc.state)
			testJSONViewOutputEquals(t, done(t).All(), tc.want)
		})
	}
}

func TestTestJSON_File(t *testing.T) {
	tcs := map[string]struct {
		file *moduletest.File
		want []map[string]interface{}
	}{
		"pass": {
			file: &moduletest.File{Name: "main.tf", Status: moduletest.Pass},
			want: []map[string]interface{}{
				{
					"@level":    "info",
					"@message":  "main.tf... pass",
					"@module":   "tofu.ui",
					"@testfile": "main.tf",
					"test_file": map[string]interface{}{
						"path":   "main.tf",
						"status": "pass",
					},
					"type": "test_file",
				},
			},
		},

		"pending": {
			file: &moduletest.File{Name: "main.tf", Status: moduletest.Pending},
			want: []map[string]interface{}{
				{
					"@level":    "info",
					"@message":  "main.tf... pending",
					"@module":   "tofu.ui",
					"@testfile": "main.tf",
					"test_file": map[string]interface{}{
						"path":   "main.tf",
						"status": "pending",
					},
					"type": "test_file",
				},
			},
		},

		"skip": {
			file: &moduletest.File{Name: "main.tf", Status: moduletest.Skip},
			want: []map[string]interface{}{
				{
					"@level":    "info",
					"@message":  "main.tf... skip",
					"@module":   "tofu.ui",
					"@testfile": "main.tf",
					"test_file": map[string]interface{}{
						"path":   "main.tf",
						"status": "skip",
					},
					"type": "test_file",
				},
			},
		},

		"fail": {
			file: &moduletest.File{Name: "main.tf", Status: moduletest.Fail},
			want: []map[string]interface{}{
				{
					"@level":    "info",
					"@message":  "main.tf... fail",
					"@module":   "tofu.ui",
					"@testfile": "main.tf",
					"test_file": map[string]interface{}{
						"path":   "main.tf",
						"status": "fail",
					},
					"type": "test_file",
				},
			},
		},

		"error": {
			file: &moduletest.File{Name: "main.tf", Status: moduletest.Error},
			want: []map[string]interface{}{
				{
					"@level":    "info",
					"@message":  "main.tf... fail",
					"@module":   "tofu.ui",
					"@testfile": "main.tf",
					"test_file": map[string]interface{}{
						"path":   "main.tf",
						"status": "error",
					},
					"type": "test_file",
				},
			},
		},
	}
	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			streams, done := terminal.StreamsForTesting(t)
			view := NewTest(arguments.ViewJSON, NewView(streams))

			view.File(tc.file)
			testJSONViewOutputEquals(t, done(t).All(), tc.want)
		})
	}
}

func TestTestJSON_Run(t *testing.T) {
	tcs := map[string]struct {
		run  *moduletest.Run
		want []map[string]interface{}
	}{
		"pass": {
			run: &moduletest.Run{Name: "run_block", Status: moduletest.Pass},
			want: []map[string]interface{}{
				{
					"@level":    "info",
					"@message":  "  \"run_block\"... pass",
					"@module":   "tofu.ui",
					"@testfile": "main.tftest.hcl",
					"@testrun":  "run_block",
					"test_run": map[string]interface{}{
						"path":   "main.tftest.hcl",
						"run":    "run_block",
						"status": "pass",
					},
					"type": "test_run",
				},
			},
		},

		"pass_with_diags": {
			run: &moduletest.Run{
				Name:        "run_block",
				Status:      moduletest.Pass,
				Diagnostics: tfdiags.Diagnostics{tfdiags.Sourceless(tfdiags.Warning, "a warning occurred", "some warning happened during this test")},
			},
			want: []map[string]interface{}{
				{
					"@level":    "info",
					"@message":  "  \"run_block\"... pass",
					"@module":   "tofu.ui",
					"@testfile": "main.tftest.hcl",
					"@testrun":  "run_block",
					"test_run": map[string]interface{}{
						"path":   "main.tftest.hcl",
						"run":    "run_block",
						"status": "pass",
					},
					"type": "test_run",
				},
				{
					"@level":    "warn",
					"@message":  "Warning: a warning occurred",
					"@module":   "tofu.ui",
					"@testfile": "main.tftest.hcl",
					"@testrun":  "run_block",
					"diagnostic": map[string]interface{}{
						"detail":   "some warning happened during this test",
						"severity": "warning",
						"summary":  "a warning occurred",
					},
					"type": "diagnostic",
				},
			},
		},

		"pending": {
			run: &moduletest.Run{Name: "run_block", Status: moduletest.Pending},
			want: []map[string]interface{}{
				{
					"@level":    "info",
					"@message":  "  \"run_block\"... pending",
					"@module":   "tofu.ui",
					"@testfile": "main.tftest.hcl",
					"@testrun":  "run_block",
					"test_run": map[string]interface{}{
						"path":   "main.tftest.hcl",
						"run":    "run_block",
						"status": "pending",
					},
					"type": "test_run",
				},
			},
		},

		"skip": {
			run: &moduletest.Run{Name: "run_block", Status: moduletest.Skip},
			want: []map[string]interface{}{
				{
					"@level":    "info",
					"@message":  "  \"run_block\"... skip",
					"@module":   "tofu.ui",
					"@testfile": "main.tftest.hcl",
					"@testrun":  "run_block",
					"test_run": map[string]interface{}{
						"path":   "main.tftest.hcl",
						"run":    "run_block",
						"status": "skip",
					},
					"type": "test_run",
				},
			},
		},

		"fail": {
			run: &moduletest.Run{Name: "run_block", Status: moduletest.Fail},
			want: []map[string]interface{}{
				{
					"@level":    "info",
					"@message":  "  \"run_block\"... fail",
					"@module":   "tofu.ui",
					"@testfile": "main.tftest.hcl",
					"@testrun":  "run_block",
					"test_run": map[string]interface{}{
						"path":   "main.tftest.hcl",
						"run":    "run_block",
						"status": "fail",
					},
					"type": "test_run",
				},
			},
		},

		"fail_with_diags": {
			run: &moduletest.Run{
				Name:   "run_block",
				Status: moduletest.Fail,
				Diagnostics: tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Error, "a comparison failed", "details details details"),
					tfdiags.Sourceless(tfdiags.Error, "a second comparison failed", "other details"),
				},
			},
			want: []map[string]interface{}{
				{
					"@level":    "info",
					"@message":  "  \"run_block\"... fail",
					"@module":   "tofu.ui",
					"@testfile": "main.tftest.hcl",
					"@testrun":  "run_block",
					"test_run": map[string]interface{}{
						"path":   "main.tftest.hcl",
						"run":    "run_block",
						"status": "fail",
					},
					"type": "test_run",
				},
				{
					"@level":    "error",
					"@message":  "Error: a comparison failed",
					"@module":   "tofu.ui",
					"@testfile": "main.tftest.hcl",
					"@testrun":  "run_block",
					"diagnostic": map[string]interface{}{
						"detail":   "details details details",
						"severity": "error",
						"summary":  "a comparison failed",
					},
					"type": "diagnostic",
				},
				{
					"@level":    "error",
					"@message":  "Error: a second comparison failed",
					"@module":   "tofu.ui",
					"@testfile": "main.tftest.hcl",
					"@testrun":  "run_block",
					"diagnostic": map[string]interface{}{
						"detail":   "other details",
						"severity": "error",
						"summary":  "a second comparison failed",
					},
					"type": "diagnostic",
				},
			},
		},

		"error": {
			run: &moduletest.Run{Name: "run_block", Status: moduletest.Error},
			want: []map[string]interface{}{
				{
					"@level":    "info",
					"@message":  "  \"run_block\"... fail",
					"@module":   "tofu.ui",
					"@testfile": "main.tftest.hcl",
					"@testrun":  "run_block",
					"test_run": map[string]interface{}{
						"path":   "main.tftest.hcl",
						"run":    "run_block",
						"status": "error",
					},
					"type": "test_run",
				},
			},
		},

		"error_with_diags": {
			run: &moduletest.Run{
				Name:        "run_block",
				Status:      moduletest.Error,
				Diagnostics: tfdiags.Diagnostics{tfdiags.Sourceless(tfdiags.Error, "an error occurred", "something bad happened during this test")},
			},
			want: []map[string]interface{}{
				{
					"@level":    "info",
					"@message":  "  \"run_block\"... fail",
					"@module":   "tofu.ui",
					"@testfile": "main.tftest.hcl",
					"@testrun":  "run_block",
					"test_run": map[string]interface{}{
						"path":   "main.tftest.hcl",
						"run":    "run_block",
						"status": "error",
					},
					"type": "test_run",
				},
				{
					"@level":    "error",
					"@message":  "Error: an error occurred",
					"@module":   "tofu.ui",
					"@testfile": "main.tftest.hcl",
					"@testrun":  "run_block",
					"diagnostic": map[string]interface{}{
						"detail":   "something bad happened during this test",
						"severity": "error",
						"summary":  "an error occurred",
					},
					"type": "diagnostic",
				},
			},
		},

		"verbose_plan": {
			run: &moduletest.Run{
				Name:   "run_block",
				Status: moduletest.Pass,
				Config: &configs.TestRun{
					Command: configs.PlanTestCommand,
				},
				Verbose: &moduletest.Verbose{
					Plan: &plans.Plan{
						Changes: &plans.Changes{
							Resources: []*plans.ResourceInstanceChangeSrc{
								{
									Addr: addrs.AbsResourceInstance{
										Module: addrs.RootModuleInstance,
										Resource: addrs.ResourceInstance{
											Resource: addrs.Resource{
												Mode: addrs.ManagedResourceMode,
												Type: "test_resource",
												Name: "creating",
											},
										},
									},
									PrevRunAddr: addrs.AbsResourceInstance{
										Module: addrs.RootModuleInstance,
										Resource: addrs.ResourceInstance{
											Resource: addrs.Resource{
												Mode: addrs.ManagedResourceMode,
												Type: "test_resource",
												Name: "creating",
											},
										},
									},
									ProviderAddr: addrs.AbsProviderConfig{
										Module: addrs.RootModule,
										Provider: addrs.Provider{
											Hostname:  addrs.DefaultProviderRegistryHost,
											Namespace: "hashicorp",
											Type:      "test",
										},
									},
									ChangeSrc: plans.ChangeSrc{
										Action: plans.Create,
										After: dynamicValue(
											t,
											cty.ObjectVal(map[string]cty.Value{
												"value": cty.StringVal("foobar"),
											}),
											cty.Object(map[string]cty.Type{
												"value": cty.String,
											})),
									},
								},
							},
						},
					},
					State: states.NewState(), // empty state
					Config: &configs.Config{
						Module: &configs.Module{
							ProviderRequirements: &configs.RequiredProviders{},
						},
					},
					Providers: map[addrs.Provider]providers.ProviderSchema{
						addrs.Provider{
							Hostname:  addrs.DefaultProviderRegistryHost,
							Namespace: "hashicorp",
							Type:      "test",
						}: {
							ResourceTypes: map[string]providers.Schema{
								"test_resource": {
									Block: &configschema.Block{
										Attributes: map[string]*configschema.Attribute{
											"value": {
												Type: cty.String,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want: []map[string]interface{}{
				{
					"@level":    "info",
					"@message":  "  \"run_block\"... pass",
					"@module":   "tofu.ui",
					"@testfile": "main.tftest.hcl",
					"@testrun":  "run_block",
					"test_run": map[string]interface{}{
						"path":   "main.tftest.hcl",
						"run":    "run_block",
						"status": "pass",
					},
					"type": "test_run",
				},
				{
					"@level":    "info",
					"@message":  "-verbose flag enabled, printing plan",
					"@module":   "tofu.ui",
					"@testfile": "main.tftest.hcl",
					"@testrun":  "run_block",
					"test_plan": map[string]interface{}{
						"configuration": map[string]interface{}{
							"root_module": map[string]interface{}{},
						},
						"errored": false,
						"planned_values": map[string]interface{}{
							"root_module": map[string]interface{}{
								"resources": []interface{}{
									map[string]interface{}{
										"address":          "test_resource.creating",
										"mode":             "managed",
										"name":             "creating",
										"provider_name":    "registry.opentofu.org/hashicorp/test",
										"schema_version":   0.0,
										"sensitive_values": map[string]interface{}{},
										"type":             "test_resource",
										"values": map[string]interface{}{
											"value": "foobar",
										},
									},
								},
							},
						},
						"resource_changes": []interface{}{
							map[string]interface{}{
								"address": "test_resource.creating",
								"change": map[string]interface{}{
									"actions": []interface{}{"create"},
									"after": map[string]interface{}{
										"value": "foobar",
									},
									"after_sensitive":  map[string]interface{}{},
									"after_unknown":    map[string]interface{}{},
									"before":           nil,
									"before_sensitive": false,
								},
								"mode":          "managed",
								"name":          "creating",
								"provider_name": "registry.opentofu.org/hashicorp/test",
								"type":          "test_resource",
							},
						},
					},
					"type": "test_plan",
				},
			},
		},
		"verbose_apply": {
			run: &moduletest.Run{
				Name:   "run_block",
				Status: moduletest.Pass,
				Config: &configs.TestRun{
					Command: configs.ApplyTestCommand,
				},
				Verbose: &moduletest.Verbose{
					Plan: &plans.Plan{}, // empty plan
					State: states.BuildState(func(state *states.SyncState) {
						state.SetResourceInstanceCurrent(
							addrs.AbsResourceInstance{
								Module: addrs.RootModuleInstance,
								Resource: addrs.ResourceInstance{
									Resource: addrs.Resource{
										Mode: addrs.ManagedResourceMode,
										Type: "test_resource",
										Name: "creating",
									},
								},
							},
							&states.ResourceInstanceObjectSrc{
								AttrsJSON: []byte(`{"value":"foobar"}`),
							},
							addrs.AbsProviderConfig{
								Module: addrs.RootModule,
								Provider: addrs.Provider{
									Hostname:  addrs.DefaultProviderRegistryHost,
									Namespace: "hashicorp",
									Type:      "test",
								},
							}, addrs.NoKey)
					}),
					Config: &configs.Config{
						Module: &configs.Module{},
					},
					Providers: map[addrs.Provider]providers.ProviderSchema{
						addrs.Provider{
							Hostname:  addrs.DefaultProviderRegistryHost,
							Namespace: "hashicorp",
							Type:      "test",
						}: {
							ResourceTypes: map[string]providers.Schema{
								"test_resource": {
									Block: &configschema.Block{
										Attributes: map[string]*configschema.Attribute{
											"value": {
												Type: cty.String,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want: []map[string]interface{}{
				{
					"@level":    "info",
					"@message":  "  \"run_block\"... pass",
					"@module":   "tofu.ui",
					"@testfile": "main.tftest.hcl",
					"@testrun":  "run_block",
					"test_run": map[string]interface{}{
						"path":   "main.tftest.hcl",
						"run":    "run_block",
						"status": "pass",
					},
					"type": "test_run",
				},
				{
					"@level":    "info",
					"@message":  "-verbose flag enabled, printing state",
					"@module":   "tofu.ui",
					"@testfile": "main.tftest.hcl",
					"@testrun":  "run_block",
					"test_state": map[string]interface{}{
						"values": map[string]interface{}{
							"root_module": map[string]interface{}{
								"resources": []interface{}{
									map[string]interface{}{
										"address":          "test_resource.creating",
										"mode":             "managed",
										"name":             "creating",
										"provider_name":    "registry.opentofu.org/hashicorp/test",
										"schema_version":   0.0,
										"sensitive_values": map[string]interface{}{},
										"type":             "test_resource",
										"values": map[string]interface{}{
											"value": "foobar",
										},
									},
								},
							},
						},
					},
					"type": "test_state",
				},
			},
		},
	}
	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			streams, done := terminal.StreamsForTesting(t)
			view := NewTest(arguments.ViewJSON, NewView(streams))

			file := &moduletest.File{Name: "main.tftest.hcl"}

			view.Run(tc.run, file)
			testJSONViewOutputEquals(t, done(t).All(), tc.want, cmp.FilterPath(func(path cmp.Path) bool {
				return strings.Contains(path.Last().String(), "version") || strings.Contains(path.Last().String(), "timestamp")
			}, cmp.Ignore()))
		})
	}
}

func TestTestJSON_FatalInterruptSummary(t *testing.T) {
	tcs := map[string]struct {
		states  map[*moduletest.Run]*states.State
		changes []*plans.ResourceInstanceChangeSrc
		want    []map[string]interface{}
	}{
		"no_state_only_plan": {
			states: make(map[*moduletest.Run]*states.State),
			changes: []*plans.ResourceInstanceChangeSrc{
				{
					Addr: addrs.AbsResourceInstance{
						Module: addrs.RootModuleInstance,
						Resource: addrs.ResourceInstance{
							Resource: addrs.Resource{
								Mode: addrs.ManagedResourceMode,
								Type: "test_instance",
								Name: "one",
							},
						},
					},
					ChangeSrc: plans.ChangeSrc{
						Action: plans.Create,
					},
				},
				{
					Addr: addrs.AbsResourceInstance{
						Module: addrs.RootModuleInstance,
						Resource: addrs.ResourceInstance{
							Resource: addrs.Resource{
								Mode: addrs.ManagedResourceMode,
								Type: "test_instance",
								Name: "two",
							},
						},
					},
					ChangeSrc: plans.ChangeSrc{
						Action: plans.Create,
					},
				},
			},
			want: []map[string]interface{}{
				{
					"@level":    "error",
					"@message":  "OpenTofu was interrupted during test execution, and may not have performed the expected cleanup operations.",
					"@module":   "tofu.ui",
					"@testfile": "main.tftest.hcl",
					"test_interrupt": map[string]interface{}{
						"planned": []interface{}{
							"test_instance.one",
							"test_instance.two",
						},
					},
					"type": "test_interrupt",
				},
			},
		},
		"file_state_no_plan": {
			states: map[*moduletest.Run]*states.State{
				nil: states.BuildState(func(state *states.SyncState) {
					state.SetResourceInstanceCurrent(
						addrs.AbsResourceInstance{
							Module: addrs.RootModuleInstance,
							Resource: addrs.ResourceInstance{
								Resource: addrs.Resource{
									Mode: addrs.ManagedResourceMode,
									Type: "test_instance",
									Name: "one",
								},
							},
						},
						&states.ResourceInstanceObjectSrc{},
						addrs.AbsProviderConfig{}, addrs.NoKey)

					state.SetResourceInstanceCurrent(
						addrs.AbsResourceInstance{
							Module: addrs.RootModuleInstance,
							Resource: addrs.ResourceInstance{
								Resource: addrs.Resource{
									Mode: addrs.ManagedResourceMode,
									Type: "test_instance",
									Name: "two",
								},
							},
						},
						&states.ResourceInstanceObjectSrc{},
						addrs.AbsProviderConfig{}, addrs.NoKey)
				}),
			},
			changes: nil,
			want: []map[string]interface{}{
				{
					"@level":    "error",
					"@message":  "OpenTofu was interrupted during test execution, and may not have performed the expected cleanup operations.",
					"@module":   "tofu.ui",
					"@testfile": "main.tftest.hcl",
					"test_interrupt": map[string]interface{}{
						"state": []interface{}{
							map[string]interface{}{
								"instance": "test_instance.one",
							},
							map[string]interface{}{
								"instance": "test_instance.two",
							},
						},
					},
					"type": "test_interrupt",
				},
			},
		},
		"run_states_no_plan": {
			states: map[*moduletest.Run]*states.State{
				&moduletest.Run{Name: "setup_block"}: states.BuildState(func(state *states.SyncState) {
					state.SetResourceInstanceCurrent(
						addrs.AbsResourceInstance{
							Module: addrs.RootModuleInstance,
							Resource: addrs.ResourceInstance{
								Resource: addrs.Resource{
									Mode: addrs.ManagedResourceMode,
									Type: "test_instance",
									Name: "one",
								},
							},
						},
						&states.ResourceInstanceObjectSrc{},
						addrs.AbsProviderConfig{}, addrs.NoKey)

					state.SetResourceInstanceCurrent(
						addrs.AbsResourceInstance{
							Module: addrs.RootModuleInstance,
							Resource: addrs.ResourceInstance{
								Resource: addrs.Resource{
									Mode: addrs.ManagedResourceMode,
									Type: "test_instance",
									Name: "two",
								},
							},
						},
						&states.ResourceInstanceObjectSrc{},
						addrs.AbsProviderConfig{}, addrs.NoKey)
				}),
			},
			changes: nil,
			want: []map[string]interface{}{
				{
					"@level":    "error",
					"@message":  "OpenTofu was interrupted during test execution, and may not have performed the expected cleanup operations.",
					"@module":   "tofu.ui",
					"@testfile": "main.tftest.hcl",
					"test_interrupt": map[string]interface{}{
						"states": map[string]interface{}{
							"setup_block": []interface{}{
								map[string]interface{}{
									"instance": "test_instance.one",
								},
								map[string]interface{}{
									"instance": "test_instance.two",
								},
							},
						},
					},
					"type": "test_interrupt",
				},
			},
		},
		"all_states_with_plan": {
			states: map[*moduletest.Run]*states.State{
				&moduletest.Run{Name: "setup_block"}: states.BuildState(func(state *states.SyncState) {
					state.SetResourceInstanceCurrent(
						addrs.AbsResourceInstance{
							Module: addrs.RootModuleInstance,
							Resource: addrs.ResourceInstance{
								Resource: addrs.Resource{
									Mode: addrs.ManagedResourceMode,
									Type: "test_instance",
									Name: "setup_one",
								},
							},
						},
						&states.ResourceInstanceObjectSrc{},
						addrs.AbsProviderConfig{}, addrs.NoKey)

					state.SetResourceInstanceCurrent(
						addrs.AbsResourceInstance{
							Module: addrs.RootModuleInstance,
							Resource: addrs.ResourceInstance{
								Resource: addrs.Resource{
									Mode: addrs.ManagedResourceMode,
									Type: "test_instance",
									Name: "setup_two",
								},
							},
						},
						&states.ResourceInstanceObjectSrc{},
						addrs.AbsProviderConfig{}, addrs.NoKey)
				}),
				nil: states.BuildState(func(state *states.SyncState) {
					state.SetResourceInstanceCurrent(
						addrs.AbsResourceInstance{
							Module: addrs.RootModuleInstance,
							Resource: addrs.ResourceInstance{
								Resource: addrs.Resource{
									Mode: addrs.ManagedResourceMode,
									Type: "test_instance",
									Name: "one",
								},
							},
						},
						&states.ResourceInstanceObjectSrc{},
						addrs.AbsProviderConfig{}, addrs.NoKey)

					state.SetResourceInstanceCurrent(
						addrs.AbsResourceInstance{
							Module: addrs.RootModuleInstance,
							Resource: addrs.ResourceInstance{
								Resource: addrs.Resource{
									Mode: addrs.ManagedResourceMode,
									Type: "test_instance",
									Name: "two",
								},
							},
						},
						&states.ResourceInstanceObjectSrc{},
						addrs.AbsProviderConfig{}, addrs.NoKey)
				}),
			},
			changes: []*plans.ResourceInstanceChangeSrc{
				{
					Addr: addrs.AbsResourceInstance{
						Module: addrs.RootModuleInstance,
						Resource: addrs.ResourceInstance{
							Resource: addrs.Resource{
								Mode: addrs.ManagedResourceMode,
								Type: "test_instance",
								Name: "new_one",
							},
						},
					},
					ChangeSrc: plans.ChangeSrc{
						Action: plans.Create,
					},
				},
				{
					Addr: addrs.AbsResourceInstance{
						Module: addrs.RootModuleInstance,
						Resource: addrs.ResourceInstance{
							Resource: addrs.Resource{
								Mode: addrs.ManagedResourceMode,
								Type: "test_instance",
								Name: "new_two",
							},
						},
					},
					ChangeSrc: plans.ChangeSrc{
						Action: plans.Create,
					},
				},
			},
			want: []map[string]interface{}{
				{
					"@level":    "error",
					"@message":  "OpenTofu was interrupted during test execution, and may not have performed the expected cleanup operations.",
					"@module":   "tofu.ui",
					"@testfile": "main.tftest.hcl",
					"test_interrupt": map[string]interface{}{
						"state": []interface{}{
							map[string]interface{}{
								"instance": "test_instance.one",
							},
							map[string]interface{}{
								"instance": "test_instance.two",
							},
						},
						"states": map[string]interface{}{
							"setup_block": []interface{}{
								map[string]interface{}{
									"instance": "test_instance.setup_one",
								},
								map[string]interface{}{
									"instance": "test_instance.setup_two",
								},
							},
						},
						"planned": []interface{}{
							"test_instance.new_one",
							"test_instance.new_two",
						},
					},
					"type": "test_interrupt",
				},
			},
		},
	}
	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			streams, done := terminal.StreamsForTesting(t)
			view := NewTest(arguments.ViewJSON, NewView(streams))

			file := &moduletest.File{Name: "main.tftest.hcl"}
			run := &moduletest.Run{Name: "run_block"}

			view.FatalInterruptSummary(run, file, tc.states, tc.changes)
			testJSONViewOutputEquals(t, done(t).All(), tc.want)
		})
	}
}

func TestSaveErroredStateFile(t *testing.T) {
	tcsHuman := map[string]struct {
		state  *states.State
		run    *moduletest.Run
		file   *moduletest.File
		stderr string
		want   interface{}
	}{
		"state_foo_bar_human": {
			file: &moduletest.File{Name: "main.tftest.hcl"},
			state: states.BuildState(func(state *states.SyncState) {
				state.SetResourceInstanceCurrent(
					addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "test",
						Name: "foo",
					}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
					&states.ResourceInstanceObjectSrc{
						Status: states.ObjectReady,
					},
					addrs.AbsProviderConfig{
						Module:   addrs.RootModule,
						Provider: addrs.NewDefaultProvider("test"),
					}, addrs.NoKey)
				state.SetResourceInstanceCurrent(
					addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "test",
						Name: "bar",
					}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
					&states.ResourceInstanceObjectSrc{
						Status: states.ObjectReady,
					},
					addrs.AbsProviderConfig{
						Module:   addrs.RootModule,
						Provider: addrs.NewDefaultProvider("test"),
					}, addrs.NoKey)
				state.SetResourceInstanceDeposed(
					addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "test",
						Name: "bar",
					}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
					"0fcb640a",
					&states.ResourceInstanceObjectSrc{
						Status: states.ObjectReady,
					},
					addrs.AbsProviderConfig{
						Module:   addrs.RootModule,
						Provider: addrs.NewDefaultProvider("test"),
					}, addrs.NoKey)
			}),
			stderr: `
Writing state to file: errored_test.tfstate
`,
			want: nil,
		},
		"state_null_resource_human": {
			file: &moduletest.File{Name: "main.tftest.hcl"},
			state: states.BuildState(func(state *states.SyncState) {
				state.SetResourceInstanceCurrent(
					addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "null_resource",
						Name: "failing_will_depend_on_me",
					}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
					&states.ResourceInstanceObjectSrc{
						Status: states.ObjectReady,
					},
					addrs.AbsProviderConfig{
						Module:   addrs.RootModule,
						Provider: addrs.NewDefaultProvider("null"),
					}, addrs.NoKey)
				state.SetResourceInstanceCurrent(
					addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "null_resource",
						Name: "failing",
					}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
					&states.ResourceInstanceObjectSrc{
						Status: states.ObjectReady,
						Dependencies: []addrs.ConfigResource{
							{
								Module: []string{},
								Resource: addrs.Resource{
									Mode: addrs.ManagedResourceMode,
									Type: "null_resource",
									Name: "failing_will_depend_on_me",
								},
							},
						},
						CreateBeforeDestroy: false,
					},
					addrs.AbsProviderConfig{
						Module:   addrs.RootModule,
						Provider: addrs.NewDefaultProvider("null"),
					}, addrs.NoKey)
			}),
			stderr: `
Writing state to file: errored_test.tfstate
`,
			want: nil,
		},
	}

	tcsJson := map[string]struct {
		state  *states.State
		run    *moduletest.Run
		file   *moduletest.File
		stderr string
		want   interface{}
	}{
		"state_with_run_json": {
			file: &moduletest.File{Name: "main.tftest.hcl"},
			run:  &moduletest.Run{Name: "run_block"},
			state: states.BuildState(func(state *states.SyncState) {
				state.SetResourceInstanceCurrent(
					addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "test",
						Name: "foo",
					}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
					&states.ResourceInstanceObjectSrc{
						Status: states.ObjectReady,
					},
					addrs.AbsProviderConfig{
						Module:   addrs.RootModule,
						Provider: addrs.NewDefaultProvider("test"),
					}, addrs.NoKey)
			}),
			stderr: "",
			want: []map[string]interface{}{
				{
					"@level":   "info",
					"@message": "Writing state to file: errored_test.tfstate",
					"@module":  string("tofu.ui"),
				},
			},
		},
		"state_foo_bar_json": {
			file: &moduletest.File{Name: "main.tftest.hcl"},
			state: states.BuildState(func(state *states.SyncState) {
				state.SetResourceInstanceCurrent(
					addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "test",
						Name: "foo",
					}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
					&states.ResourceInstanceObjectSrc{
						Status: states.ObjectReady,
					},
					addrs.AbsProviderConfig{
						Module:   addrs.RootModule,
						Provider: addrs.NewDefaultProvider("test"),
					}, addrs.NoKey)
				state.SetResourceInstanceCurrent(
					addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "test",
						Name: "bar",
					}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
					&states.ResourceInstanceObjectSrc{
						Status: states.ObjectReady,
					},
					addrs.AbsProviderConfig{
						Module:   addrs.RootModule,
						Provider: addrs.NewDefaultProvider("test"),
					}, addrs.NoKey)
				state.SetResourceInstanceDeposed(
					addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "test",
						Name: "bar",
					}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
					"0fcb640a",
					&states.ResourceInstanceObjectSrc{
						Status: states.ObjectReady,
					},
					addrs.AbsProviderConfig{
						Module:   addrs.RootModule,
						Provider: addrs.NewDefaultProvider("test"),
					}, addrs.NoKey)
			}),
			stderr: "",
			want: []map[string]interface{}{
				{
					"@level":   "info",
					"@message": "Writing state to file: errored_test.tfstate",
					"@module":  "tofu.ui",
				},
			},
		},
		"state_null_resource_with_errors": {
			file: &moduletest.File{Name: "main.tftest.hcl"},
			state: states.BuildState(func(state *states.SyncState) {
				state.SetResourceInstanceCurrent(
					addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "null_resource",
						Name: "failing_will_depend_on_me",
					}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
					&states.ResourceInstanceObjectSrc{
						Status: states.ObjectReady,
					},
					addrs.AbsProviderConfig{
						Module:   addrs.RootModule,
						Provider: addrs.NewDefaultProvider("null"),
					}, addrs.NoKey)
				state.SetResourceInstanceCurrent(
					addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "null_resource",
						Name: "failing",
					}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
					&states.ResourceInstanceObjectSrc{
						Status: states.ObjectReady,
						Dependencies: []addrs.ConfigResource{
							{
								Module: []string{},
								Resource: addrs.Resource{
									Mode: addrs.ManagedResourceMode,
									Type: "null_resource",
									Name: "failing_will_depend_on_me",
								},
							},
						},
						CreateBeforeDestroy: false,
					},
					addrs.AbsProviderConfig{
						Module:   addrs.RootModule,
						Provider: addrs.NewDefaultProvider("null"),
					}, addrs.NoKey)
			}),
			stderr: "",
			want: []map[string]interface{}{
				{
					"@level":   "info",
					"@message": "Writing state to file: errored_test.tfstate",
					"@module":  "tofu.ui",
				},
			},
		},
	}
	// Run tests for Human view
	runTestSaveErroredStateFile(t, tcsHuman, arguments.ViewHuman)

	// Run tests for JSON view
	runTestSaveErroredStateFile(t, tcsJson, arguments.ViewJSON)
}

func runTestSaveErroredStateFile(t *testing.T, tc map[string]struct {
	state  *states.State
	run    *moduletest.Run
	file   *moduletest.File
	stderr string
	want   interface{}
}, viewType arguments.ViewType) {
	for name, data := range tc {
		t.Run(name, func(t *testing.T) {
			// Create a temporary directory
			tempDir := t.TempDir()

			// Modify the state file path to use the temporary directory
			tempStateFilePath := filepath.Clean(filepath.Join(tempDir, "errored_test.tfstate"))

			// Change the working directory to the temporary directory
			t.Chdir(tempDir)

			streams, done := terminal.StreamsForTesting(t)

			if viewType == arguments.ViewHuman {
				view := NewTest(arguments.ViewHuman, NewView(streams))
				SaveErroredTestStateFile(data.state, data.run, data.file, view)
				output := done(t)

				actual, expected := output.Stderr(), data.stderr
				if diff := cmp.Diff(expected, actual); len(diff) > 0 {
					t.Errorf("expected:\n%s\nactual:\n%s\ndiff:\n%s", expected, actual, diff)
				}
			} else if viewType == arguments.ViewJSON {
				view := NewTest(arguments.ViewJSON, NewView(streams))
				SaveErroredTestStateFile(data.state, data.run, data.file, view)
				want, ok := data.want.([]map[string]interface{})
				if !ok {
					t.Fatalf("Failed to assert want as []map[string]interface{}")
				}
				testJSONViewOutputEquals(t, done(t).All(), want)
			} else {
				t.Fatalf("Unsupported view type: %v", viewType)
			}

			// Check if the state file exists
			if _, err := os.Stat(tempStateFilePath); os.IsNotExist(err) {
				// File does not exist
				t.Errorf("Expected state file 'errored_test.tfstate' to exist in: %s, but it does not.", tempDir)
			}
			// Trigger garbage collection to ensure that all open file handles are closed.
			// This prevents TempDir RemoveAll cleanup errors on Windows.
			if runtime.GOOS == "windows" {
				runtime.GC()
			}
		})
	}
}

func dynamicValue(t *testing.T, value cty.Value, typ cty.Type) plans.DynamicValue {
	d, err := plans.NewDynamicValue(value, typ)
	if err != nil {
		t.Fatalf("failed to create dynamic value: %s", err)
	}
	return d
}
