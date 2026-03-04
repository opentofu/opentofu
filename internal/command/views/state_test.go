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

func TestStateViews(t *testing.T) {
	tests := map[string]struct {
		viewCall   func(state State)
		wantJson   []map[string]any
		wantStdout string
		wantStderr string
	}{
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
			testStateJson(t, tc.viewCall, tc.wantJson)
			testStateMulti(t, tc.viewCall, tc.wantStdout, tc.wantStderr, tc.wantJson)
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

func testStateJson(t *testing.T, call func(state State), want []map[string]interface{}) {
	view, done := testView(t)
	stateView := NewState(arguments.ViewOptions{ViewType: arguments.ViewJSON}, view)
	call(stateView)
	output := done(t)
	if output.Stderr() != "" {
		t.Errorf("expected no stderr but got:\n%s", output.Stderr())
	}

	testJSONViewOutputEquals(t, output.Stdout(), want)
}

func testStateMulti(t *testing.T, call func(state State), wantStdout string, wantStderr string, want []map[string]interface{}) {
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
