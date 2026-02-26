// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tofu"
)

func TestImportViews(t *testing.T) {
	tests := map[string]struct {
		viewCall   func(v Import)
		wantJson   []map[string]any
		wantStdout string
		wantStderr string
	}{
		"invalid address reference": {
			viewCall: func(v Import) {
				v.InvalidAddressReference()
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "For information on valid syntax, see: https://opentofu.org/docs/cli/state/resource-addressing/",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline(`For information on valid syntax, see:
https://opentofu.org/docs/cli/state/resource-addressing/`),
		},
		"missing resource configuration": {
			viewCall: func(v Import) {
				v.MissingResourceConfiguration(addrs.AbsResourceInstance{
					Module: addrs.ModuleInstance{{Name: "mod"}},
					Resource: addrs.ResourceInstance{Resource: addrs.Resource{
						Mode: addrs.ManagedResourceMode,
						Type: "test",
						Name: "test_name",
					}},
				}, "./mod", "test", "test_name")
			},
			wantJson: []map[string]any{
				{
					"@level":   "error",
					"@message": `Resource address "module.mod.test.test_name" does not exist in the configuration. Before importing this resource, please create its configuration in ./mod`,
					"@module":  "tofu.ui",
				},
			},
			wantStderr: withNewline(`Error: resource address "module.mod.test.test_name" does not exist in the configuration.

Before importing this resource, please create its configuration in ./mod. For example:

resource "test" "test_name" {
  # (resource arguments)
}
`),
		},
		"success": {
			viewCall: func(v Import) {
				v.Success()
			},
			wantStdout: withNewline(`
Import successful!

The resources that were imported are shown above. These resources are now in
your OpenTofu state and will henceforth be managed by OpenTofu.
`),
			wantStderr: "",
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Import successful! The resources that were imported are shown above. These resources are now in your OpenTofu state and will henceforth be managed by OpenTofu",
					"@module":  "tofu.ui",
				},
			},
		},
		"unsupported local op": {
			viewCall: func(v Import) {
				v.UnsupportedLocalOp()
			},
			wantJson: []map[string]any{
				{
					"@level":   "error",
					"@message": `The configured backend doesn't support this operation. The "backend" in OpenTofu defines how OpenTofu operates. The default backend performs all operations locally on your machine. Your configuration is configured to use a non-local backend. This backend doesn't support this operation.`,
					"@module":  "tofu.ui",
				},
			},
			wantStderr: withNewline(errUnsupportedLocalOp),
		},
		// Diagnostics
		"warning": {
			viewCall: func(v Import) {
				diags := tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Warning, "A warning occurred", "foo bar"),
				}
				v.Diagnostics(diags)
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
			viewCall: func(v Import) {
				diags := tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Error, "An error occurred", "foo bar"),
				}
				v.Diagnostics(diags)
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
			viewCall: func(v Import) {
				diags := tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Warning, "A warning", "foo bar warning"),
					tfdiags.Sourceless(tfdiags.Error, "An error", "foo bar error"),
				}
				v.Diagnostics(diags)
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
		// hooks
		"random hook": {
			viewCall: func(v Import) {
				resAddr := addrs.AbsResourceInstance{
					Resource: addrs.ResourceInstance{
						Resource: addrs.Resource{
							Mode: addrs.EphemeralResourceMode,
							Type: "foo",
							Name: "bar",
						},
					},
				}
				for _, hook := range v.Hooks() {
					// called here a hook method that prints something both in human, and json, formats
					action, _ := hook.PreOpen(resAddr)
					if action != tofu.HookActionContinue {
						t.Errorf("invalid action returned by the PreImportState hook. wanted %d but got %d", tofu.HookActionContinue, action)
					}
				}
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "ephemeral.foo.bar: Opening...",
					"@module":  "tofu.ui",
					"hook": map[string]any{
						"Msg": "Opening...",
						"resource": map[string]any{
							"addr":             "ephemeral.foo.bar",
							"implied_provider": "foo",
							"module":           "",
							"resource":         "ephemeral.foo.bar",
							"resource_key":     nil,
							"resource_name":    "bar",
							"resource_type":    "foo",
						},
					},
					"type": "ephemeral_action_started",
				},
			},
			wantStdout: withNewline(`ephemeral.foo.bar: Opening...`),
		},
		// Operation
		"random operation": {
			viewCall: func(v Import) {
				v.Operation().Interrupted()
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "\nInterrupt received.\nPlease wait for OpenTofu to exit or data loss may occur.\nGracefully shutting down...\n",
					"@module":  "tofu.ui",
					"type":     "log",
				},
			},
			wantStdout: withNewline("\nInterrupt received.\nPlease wait for OpenTofu to exit or data loss may occur.\nGracefully shutting down...\n"),
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			testImportHuman(t, tc.viewCall, tc.wantStdout, tc.wantStderr)
			testImportJson(t, tc.viewCall, tc.wantJson)
			testImportMulti(t, tc.viewCall, tc.wantStdout, tc.wantStderr, tc.wantJson)
		})
	}
}

func testImportHuman(t *testing.T, call func(v Import), wantStdout, wantStderr string) {
	view, done := testView(t)
	v := NewImport(arguments.ViewOptions{ViewType: arguments.ViewHuman}, view)
	call(v)
	output := done(t)
	if diff := cmp.Diff(wantStderr, output.Stderr()); diff != "" {
		t.Errorf("invalid stderr (-want, +got):\n%s", diff)
	}
	if diff := cmp.Diff(wantStdout, output.Stdout()); diff != "" {
		t.Errorf("invalid stdout (-want, +got):\n%s", diff)
	}
}

func testImportJson(t *testing.T, call func(v Import), want []map[string]interface{}) {
	// New type just to assert the fields that we are interested in
	view, done := testView(t)
	v := NewImport(arguments.ViewOptions{ViewType: arguments.ViewJSON}, view)
	call(v)
	output := done(t)
	if output.Stderr() != "" {
		t.Errorf("expected no stderr but got:\n%s", output.Stderr())
	}

	testJSONViewOutputEquals(t, output.Stdout(), want)
}

func testImportMulti(t *testing.T, call func(v Import), wantStdout string, wantStderr string, want []map[string]interface{}) {
	jsonInto, err := os.CreateTemp(t.TempDir(), "json-into-*")
	if err != nil {
		t.Fatalf("failed to create the file to write json content into: %s", err)
	}
	view, done := testView(t)
	v := NewImport(arguments.ViewOptions{ViewType: arguments.ViewHuman, JSONInto: jsonInto}, view)
	call(v)
	{
		if err := jsonInto.Close(); err != nil {
			t.Fatalf("failed to close the jsonInto file: %s", err)
		}
		// check the fileInto content
		fileContent, err := os.ReadFile(jsonInto.Name())
		if err != nil {
			t.Fatalf("failed to read the file content with the json output: %s", err)
		}
		testJSONViewOutputEquals(t, string(fileContent), want)
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
