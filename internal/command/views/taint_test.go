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
)

func TestTaintView(t *testing.T) {
	tests := map[string]struct {
		viewCall   func(v Taint)
		wantJson   []map[string]any
		wantStdout string
		wantStderr string
	}{
		"tainted successfully": {
			viewCall: func(v Taint) {
				addr := addrs.Resource{
					Mode: addrs.ManagedResourceMode,
					Type: "test_instance",
					Name: "foo",
				}.Instance(addrs.NoKey).Absolute(
					addrs.RootModuleInstance,
				)
				v.TaintedSuccessfully(addr)
			},
			wantStdout: withNewline("Resource instance test_instance.foo has been marked as tainted."),
			wantStderr: "",
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Resource instance test_instance.foo has been marked as tainted.",
					"@module":  "tofu.ui",
				},
			},
		},
		// Diagnostics
		"warning": {
			viewCall: func(v Taint) {
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
			viewCall: func(v Taint) {
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
		"multiple diagnostics": {
			viewCall: func(v Taint) {
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
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			testTaintHuman(t, tc.viewCall, tc.wantStdout, tc.wantStderr)
			testTaintJson(t, tc.viewCall, tc.wantJson)
			testTaintMulti(t, tc.viewCall, tc.wantStdout, tc.wantStderr, tc.wantJson)
		})
	}
}

func testTaintHuman(t *testing.T, call func(v Taint), wantStdout, wantStderr string) {
	view, done := testView(t)
	v := NewTaint(arguments.ViewOptions{ViewType: arguments.ViewHuman}, view)
	call(v)
	output := done(t)
	if diff := cmp.Diff(wantStderr, output.Stderr()); diff != "" {
		t.Errorf("invalid stderr (-want, +got):\n%s", diff)
	}
	if diff := cmp.Diff(wantStdout, output.Stdout()); diff != "" {
		t.Errorf("invalid stdout (-want, +got):\n%s", diff)
	}
}

func testTaintJson(t *testing.T, call func(v Taint), want []map[string]interface{}) {
	view, done := testView(t)
	v := NewTaint(arguments.ViewOptions{ViewType: arguments.ViewJSON}, view)
	call(v)
	output := done(t)
	if output.Stderr() != "" {
		t.Errorf("expected no stderr but got:\n%s", output.Stderr())
	}

	testJSONViewOutputEquals(t, output.Stdout(), want)
}

func testTaintMulti(t *testing.T, call func(v Taint), wantStdout string, wantStderr string, want []map[string]interface{}) {
	jsonInto, err := os.CreateTemp(t.TempDir(), "json-into-*")
	if err != nil {
		t.Fatalf("failed to create the file to write json content into: %s", err)
	}
	view, done := testView(t)
	v := NewTaint(arguments.ViewOptions{ViewType: arguments.ViewHuman, JSONInto: jsonInto}, view)
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
		output := done(t)
		if diff := cmp.Diff(wantStderr, output.Stderr()); diff != "" {
			t.Errorf("invalid stderr (-want, +got):\n%s", diff)
		}
		if diff := cmp.Diff(wantStdout, output.Stdout()); diff != "" {
			t.Errorf("invalid stdout (-want, +got):\n%s", diff)
		}
	}
}
