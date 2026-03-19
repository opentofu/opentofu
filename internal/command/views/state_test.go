// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"context"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/states/statefile"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tofu"
	regaddr "github.com/opentofu/registry-address/v2"
	"github.com/zclconf/go-cty/cty"
)

func TestStateViews(t *testing.T) {
	tests := map[string]struct {
		viewCall        func(state State)
		wantJson        []map[string]any
		wantStdout      string
		wantStderr      string
		ignoreTimestamp bool
	}{
		"stateNotFound": {
			viewCall: func(state State) {
				state.StateNotFound()
			},
			wantJson: []map[string]any{
				{
					"@level":   "error",
					"@message": "No state file was found! State management commands require a state file. Run this command in a directory where OpenTofu has been run or use the -state flag to point the command to a specific state location.",
					"@module":  "tofu.ui",
				},
			},
			wantStderr: withNewline("No state file was found!\n\nState management commands require a state file. Run this command\nin a directory where OpenTofu has been run or use the -state flag\nto point the command to a specific state location."),
		},
		"stateLoadingFailure": {
			viewCall: func(state State) {
				state.StateLoadingFailure("failed to read state file")
			},
			wantJson: []map[string]any{
				{
					"@level":   "error",
					"@message": "Error loading the state: failed to read state file. Please ensure that your OpenTofu state exists and that you've configured it properly. You can use the \"-state\" flag to point OpenTofu at another state file.",
					"@module":  "tofu.ui",
				},
			},
			wantStderr: withNewline("Error loading the state: failed to read state file\n\nPlease ensure that your OpenTofu state exists and that you've\nconfigured it properly. You can use the \"-state\" flag to point\nOpenTofu at another state file."),
		},
		"stateSavingError": {
			viewCall: func(state State) {
				state.StateSavingError("failed to save state file")
			},
			wantJson: []map[string]any{
				{
					"@level":   "error",
					"@message": "Error saving the state: failed to save state file. The state was not saved. No items were removed from the persisted state. No backup was created since no modification occurred. Please resolve the issue above and try again.",
					"@module":  "tofu.ui",
				},
			},
			wantStderr: withNewline("Error saving the state: failed to save state file\n\nThe state was not saved. No items were removed from the persisted\nstate. No backup was created since no modification occurred. Please\nresolve the issue above and try again."),
		},
		"stateListAddr": {
			viewCall: func(state State) {
				addr, diags := addrs.ParseAbsResourceInstanceStr("null_resource.example[0]")
				if diags.HasErrors() {
					panic(diags.Err())
				}
				state.StateListAddr(addr)
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "null_resource.example[0]",
					"@module":  "tofu.ui",
					"type":     "resource_address",
				},
			},
			wantStdout: withNewline("null_resource.example[0]"),
		},
		"errorMovingToAlreadyExistingDst": {
			viewCall: func(state State) {
				state.ErrorMovingToAlreadyExistingDst()
			},
			wantJson: []map[string]any{
				{
					"@level":   "error",
					"@message": "Error moving state: destination module already exists. Please ensure your addresses and state paths are valid. No state was persisted. Your existing states are untouched.",
					"@module":  "tofu.ui",
				},
			},
			wantStderr: withNewline("Error moving state: destination module already exists.\n\nPlease ensure your addresses and state paths are valid. No\nstate was persisted. Your existing states are untouched."),
		},
		"resourceMoveStatus with dryRun=true": {
			viewCall: func(state State) {
				state.ResourceMoveStatus(true, "test_res.name1", "test_res.name2")
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": `Would move "test_res.name1" to "test_res.name2"`,
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline(`Would move "test_res.name1" to "test_res.name2"`),
		},
		"resourceMoveStatus with dryRun=false": {
			viewCall: func(state State) {
				state.ResourceMoveStatus(false, "test_res.name1", "test_res.name2")
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": `Move "test_res.name1" to "test_res.name2"`,
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline(`Move "test_res.name1" to "test_res.name2"`),
		},
		"dryRunMovedStatus with 0 resources": {
			viewCall: func(state State) {
				state.DryRunMovedStatus(0)
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": `Would have moved nothing`,
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline(`Would have moved nothing.`),
		},
		"dryRunMovedStatus with >0 resources": {
			viewCall: func(state State) {
				state.DryRunMovedStatus(1)
			},
			wantJson: []map[string]any{
				{},
			},
			wantStdout: "",
		},
		"moveFinalStatus with 0 resources": {
			viewCall: func(state State) {
				state.MoveFinalStatus(0)
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": `No matching objects found`,
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline(`No matching objects found.`),
		},
		"moveFinalStatus with >0 resources": {
			viewCall: func(state State) {
				state.MoveFinalStatus(1)
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Successfully moved 1 object(s)",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline("Successfully moved 1 object(s)."),
		},
		"printPulledState": {
			viewCall: func(state State) {
				state.PrintPulledState(`{"version":4,"terraform_version":"1.11.5","serial":9,"lineage":"9ba8c556-ae6c-20ee-f6ed-b57c7cc04dcd","outputs":{},"resources":[]}`)
			},
			wantJson: []map[string]any{
				{
					"@level":   "error",
					"@message": "printing the pulled state is not available in the JSON view. The `tofu state pull` should not be configured with the `-json` flag",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline(`{"version":4,"terraform_version":"1.11.5","serial":9,"lineage":"9ba8c556-ae6c-20ee-f6ed-b57c7cc04dcd","outputs":{},"resources":[]}`),
		},
		"noMatchingResourcesForProviderReplacement": {
			viewCall: func(state State) {
				state.NoMatchingResourcesForProviderReplacement()
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "No matching resources found",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline(`No matching resources found.`),
		},
		"replaceProviderOverview": {
			viewCall: func(state State) {
				state.ReplaceProviderOverview(
					regaddr.NewProvider("registry1.org", "ns", "prov"),
					regaddr.NewProvider("registry2.org", "ns", "prov"),
					[]*states.Resource{
						{
							Addr: addrs.AbsResource{Resource: addrs.Resource{
								Mode: addrs.ManagedResourceMode,
								Type: "res",
								Name: "foo",
							}},
						},
					},
				)
			},
			wantJson: []map[string]any{
				{
					"@level":    "info",
					"@message":  "OpenTofu will replace provider from registry1.org/ns/prov to registry2.org/ns/prov for 1 resources",
					"resources": []any{"res.foo"},
					"@module":   "tofu.ui",
					"type":      "replace_provider",
					"from":      "registry1.org/ns/prov",
					"to":        "registry2.org/ns/prov",
				},
			},
			wantStdout: `OpenTofu will perform the following actions:

  ~ Updating provider:
    - registry1.org/ns/prov
    + registry2.org/ns/prov

Changing 1 resources:

  res.foo
`,
		},
		"replaceProviderCancelled": {
			viewCall: func(state State) {
				state.ReplaceProviderCancelled()
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Cancelled replacing providers",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline(`Cancelled replacing providers.`),
		},
		"providerReplaced": {
			viewCall: func(state State) {
				state.ProviderReplaced(2)
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Successfully replaced provider for 2 resources",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline(`Successfully replaced provider for 2 resources.`),
		},
		"resourceRemoveStatus with dryRun=true": {
			viewCall: func(state State) {
				state.ResourceRemoveStatus(true, "test_res.name1")
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Would remove test_res.name1",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline("Would remove test_res.name1"),
		},
		"resourceRemoveStatus with dryRun=false": {
			viewCall: func(state State) {
				state.ResourceRemoveStatus(false, "test_res.name1")
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Removed test_res.name1",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline("Removed test_res.name1"),
		},
		"dryRunRemovedStatus with 0 resources": {
			viewCall: func(state State) {
				state.DryRunRemovedStatus(0)
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Would have removed nothing",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline("Would have removed nothing."),
		},
		"dryRunRemovedStatus with >0 resources": {
			viewCall: func(state State) {
				state.DryRunRemovedStatus(1)
			},
			wantJson: []map[string]any{
				{},
			},
			wantStdout: "",
		},
		"removeFinalStatus with 0 resources": {
			viewCall: func(state State) {
				state.RemoveFinalStatus(0)
			},
			wantJson: []map[string]any{
				{},
			},
			wantStdout: "",
		},
		"removeFinalStatus with >0 resources": {
			viewCall: func(state State) {
				state.RemoveFinalStatus(2)
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Successfully removed 2 resource instance(s)",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline("Successfully removed 2 resource instance(s)."),
		},
		"unsupportedLocalOp": {
			viewCall: func(state State) {
				state.UnsupportedLocalOp()
			},
			wantJson: []map[string]any{
				{
					"@level":   "error",
					"@message": "The configured backend doesn't support this operation. The 'backend' in OpenTofu defines how OpenTofu operates. The default backend performs all operations locally on your machine. Your configuration is configured to use a non-local backend. This backend doesn't support this operation",
					"@module":  "tofu.ui",
				},
			},
			wantStderr: withNewline(errUnsupportedLocalOp),
		},
		"addressParsingError": {
			viewCall: func(state State) {
				state.AddressParsingError("aws_instance.example")
			},
			wantJson: []map[string]any{
				{
					"@level":   "error",
					"@message": "Error parsing instance address: aws_instance.example. This command requires that the address references one specific instance. To view the available instances, use \"tofu state list\". Please modify the address to reference a specific instance.",
					"@module":  "tofu.ui",
				},
			},
			wantStderr: withNewline("Error parsing instance address: aws_instance.example\n\nThis command requires that the address references one specific instance.\nTo view the available instances, use \"tofu state list\". Please modify \nthe address to reference a specific instance."),
		},
		"noInstanceFoundError": {
			viewCall: func(state State) {
				state.NoInstanceFoundError()
			},
			wantJson: []map[string]any{
				{
					"@level":   "error",
					"@message": "No instance found for the given address! This command requires that the address references one specific instance. To view the available instances, use \"tofu state list\". Please modify the address to reference a specific instance.",
					"@module":  "tofu.ui",
				},
			},
			wantStderr: withNewline("No instance found for the given address!\n\nThis command requires that the address references one specific instance.\nTo view the available instances, use \"tofu state list\". Please modify \nthe address to reference a specific instance."),
		},
		// ShowResourceState for success cases has its own dedicated test because in that situation the json output
		// is in a raw format and not adhere to the way "informative" messages are shown.
		"showResourceState with nil state": {
			viewCall: func(state State) {
				state.ShowResourceState(context.Background(), nil, nil)
			},
			wantStdout: withNewline("No state."),
			wantStderr: "",
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "no state",
					"@module":  "tofu.ui",
				},
			},
		},
		"showResourceState with proper state": {
			ignoreTimestamp: true,
			viewCall: func(state State) {
				stateFile := states.BuildState(func(s *states.SyncState) {
					s.SetResourceInstanceCurrent(
						addrs.Resource{
							Mode: addrs.ManagedResourceMode,
							Type: "test_resource",
							Name: "foo",
						}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
						&states.ResourceInstanceObjectSrc{
							AttrsJSON: []byte(`{"id":"bar","foo":"value"}`),
							Status:    states.ObjectReady,
						},
						addrs.AbsProviderConfig{
							Provider: addrs.NewDefaultProvider("test"),
							Module:   addrs.RootModule,
						},
						addrs.NoKey,
					)
				})
				resState := statefile.New(stateFile, "", 0)
				schema := tofu.Schemas{
					Providers: map[addrs.Provider]providers.ProviderSchema{
						addrs.NewDefaultProvider("test"): {
							ResourceTypes: map[string]providers.Schema{
								"test_resource": {
									Block: &configschema.Block{
										Attributes: map[string]*configschema.Attribute{
											"id": {
												Type:     cty.String,
												Computed: true,
											},
											"foo": {
												Type: cty.String,
											},
										},
									},
								},
							},
						},
					},
				}
				state.ShowResourceState(context.Background(), resState, &schema)
			},
			wantStdout: withNewline(`# test_resource.foo:
resource "test_resource" "foo" {
    foo = "value"
    id  = "bar"
}`),
			wantStderr: "",
			wantJson: []map[string]any{
				{
					"format_version":    "1.0",
					"terraform_version": "1.12.0",
					"values": map[string]any{
						"root_module": map[string]any{
							"resources": []any{
								map[string]any{
									"address":        "test_resource.foo",
									"mode":           "managed",
									"type":           "test_resource",
									"name":           "foo",
									"provider_name":  "registry.opentofu.org/hashicorp/test",
									"schema_version": float64(0),
									"values": map[string]any{
										"foo": "value",
										"id":  "bar",
									},
									"sensitive_values": map[string]any{},
								},
							},
						},
					},
				},
			},
		},
		// Diagnostics
		"warning": {
			viewCall: func(state State) {
				diags := tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Warning, "A warning occurred", "foo bar"),
				}
				state.Diagnostics(diags)
			},
			wantStdout: withNewline("\nWarning: A warning occurred\n\nfoo bar"),
			wantStderr: "",
			wantJson: []map[string]any{
				{
					"@level":   "warn",
					"@message": "Warning: A warning occurred",
					"@module":  "tofu.ui",
					"diagnostic": map[string]any{
						"detail":   "foo bar",
						"severity": "warning",
						"summary":  "A warning occurred",
					},
					"type": "diagnostic",
				},
			},
		},
		"error": {
			viewCall: func(state State) {
				diags := tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Error, "An error occurred", "foo bar"),
				}
				state.Diagnostics(diags)
			},
			wantStdout: "",
			wantStderr: withNewline("\nError: An error occurred\n\nfoo bar"),
			wantJson: []map[string]any{
				{
					"@level":   "error",
					"@message": "Error: An error occurred",
					"@module":  "tofu.ui",
					"diagnostic": map[string]any{
						"detail":   "foo bar",
						"severity": "error",
						"summary":  "An error occurred",
					},
					"type": "diagnostic",
				},
			},
		},
		"multiple_diagnostics": {
			viewCall: func(state State) {
				diags := tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Warning, "A warning", "foo bar warning"),
					tfdiags.Sourceless(tfdiags.Error, "An error", "foo bar error"),
				}
				state.Diagnostics(diags)
			},
			wantStdout: withNewline("\nWarning: A warning\n\nfoo bar warning"),
			wantStderr: withNewline("\nError: An error\n\nfoo bar error"),
			wantJson: []map[string]any{
				{
					"@level":   "warn",
					"@message": "Warning: A warning",
					"@module":  "tofu.ui",
					"diagnostic": map[string]any{
						"detail":   "foo bar warning",
						"severity": "warning",
						"summary":  "A warning",
					},
					"type": "diagnostic",
				},
				{
					"@level":   "error",
					"@message": "Error: An error",
					"@module":  "tofu.ui",
					"diagnostic": map[string]any{
						"detail":   "foo bar error",
						"severity": "error",
						"summary":  "An error",
					},
					"type": "diagnostic",
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			testStateHuman(t, tc.viewCall, tc.wantStdout, tc.wantStderr)
			testStateJson(t, tc.viewCall, tc.wantJson, !tc.ignoreTimestamp)
			testStateMulti(t, tc.viewCall, tc.wantStdout, tc.wantStderr, tc.wantJson, !tc.ignoreTimestamp)
		})
	}
}

func testStateHuman(t *testing.T, call func(state State), wantStdout, wantStderr string) {
	view, done := testView(t)
	stateView := NewState(arguments.ViewOptions{ViewType: arguments.ViewHuman}, view)
	call(stateView)
	output := done(t)
	if diff := cmp.Diff(wantStderr, output.Stderr()); diff != "" {
		t.Errorf("invalid stderr (-want, +got):\n%s", diff)
	}
	if diff := cmp.Diff(wantStdout, output.Stdout()); diff != "" {
		t.Errorf("invalid stdout (-want, +got):\n%s", diff)
	}
}

func testStateJson(t *testing.T, call func(state State), want []map[string]interface{}, withTimestamp bool) {
	view, done := testView(t)
	stateView := NewState(arguments.ViewOptions{ViewType: arguments.ViewJSON}, view)
	call(stateView)
	output := done(t)
	if output.Stderr() != "" {
		t.Errorf("expected no stderr but got:\n%s", output.Stderr())
	}

	if withTimestamp {
		testJSONViewOutputEquals(t, output.Stdout(), want)
		return
	}
	testJSONViewOutputEqualsIgnoringTimestamp(t, output.Stdout(), want)
}

func testStateMulti(t *testing.T, call func(state State), wantStdout string, wantStderr string, want []map[string]interface{}, withTimestamp bool) {
	jsonInto, err := os.CreateTemp(t.TempDir(), "json-into-*")
	if err != nil {
		t.Fatalf("failed to create the file to write json content into: %s", err)
	}
	view, done := testView(t)
	stateView := NewState(arguments.ViewOptions{ViewType: arguments.ViewHuman, JSONInto: jsonInto}, view)
	call(stateView)
	{
		if err := jsonInto.Close(); err != nil {
			t.Fatalf("failed to close the jsonInto file: %s", err)
		}
		// check the fileInto content
		fileContent, err := os.ReadFile(jsonInto.Name())
		if err != nil {
			t.Fatalf("failed to read the file content with the json output: %s", err)
		}
		if withTimestamp {
			testJSONViewOutputEquals(t, string(fileContent), want)
			return
		}
		testJSONViewOutputEqualsIgnoringTimestamp(t, string(fileContent), want)
	}
	{
		// check the human output
		output := done(t)
		if diff := cmp.Diff(wantStderr, output.Stderr()); diff != "" {
			t.Errorf("invalid stderr (-want, +got):\n%s", diff)
		}
		if diff := cmp.Diff(wantStdout, output.Stdout()); diff != "" {
			t.Errorf("invalid stdout (-want, +got):\n%s", diff)
		}
	}
}
