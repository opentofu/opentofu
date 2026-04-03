// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestConsoleViews(t *testing.T) {
	tests := map[string]struct {
		viewCall   func(console Console)
		wantJson   []map[string]any
		wantStdout string
		wantStderr string
	}{
		"unsupportedLocalOp": {
			viewCall: func(console Console) {
				console.UnsupportedLocalOp()
			},
			wantJson: []map[string]any{
				{
					"@level":   "error",
					"@message": "Error: The configured backend doesn't support this operation",
					"@module":  "tofu.ui",
					"diagnostic": map[string]any{
						"detail":   `The "backend" in OpenTofu defines how OpenTofu operates. The default backend performs all operations locally on your machine. Your configuration is configured to use a non-local backend. This backend doesn't support this operation.`,
						"severity": "error",
						"summary":  "The configured backend doesn't support this operation",
					},
					"type": "diagnostic",
				},
			},
			wantStderr: `
Error: The configured backend doesn't support this operation

The "backend" in OpenTofu defines how OpenTofu operates. The default backend
performs all operations locally on your machine. Your configuration is
configured to use a non-local backend. This backend doesn't support this
operation.
`,
		},
		// Diagnostics
		"warning": {
			viewCall: func(console Console) {
				diags := tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Warning, "A warning occurred", "foo bar"),
				}
				console.Diagnostics(diags)
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
			viewCall: func(console Console) {
				diags := tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Error, "An error occurred", "foo bar"),
				}
				console.Diagnostics(diags)
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
			viewCall: func(console Console) {
				diags := tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Warning, "A warning", "foo bar warning"),
					tfdiags.Sourceless(tfdiags.Error, "An error", "foo bar error"),
				}
				console.Diagnostics(diags)
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
			testConsoleHuman(t, tc.viewCall, tc.wantStdout, tc.wantStderr)
			testConsoleJson(t, tc.viewCall, tc.wantJson)
			testConsoleMulti(t, tc.viewCall, tc.wantStdout, tc.wantStderr, tc.wantJson)
		})
	}
}

func testConsoleHuman(t *testing.T, call func(console Console), wantStdout, wantStderr string) {
	view, done := testView(t)
	consoleView := NewConsole(arguments.ViewOptions{ViewType: arguments.ViewHuman}, view)
	call(consoleView)
	output := done(t)
	if diff := cmp.Diff(wantStderr, output.Stderr()); diff != "" {
		t.Errorf("invalid stderr (-want, +got):\n%s", diff)
	}
	if diff := cmp.Diff(wantStdout, output.Stdout()); diff != "" {
		t.Errorf("invalid stdout (-want, +got):\n%s", diff)
	}
}

func testConsoleJson(t *testing.T, call func(console Console), want []map[string]interface{}) {
	view, done := testView(t)
	consoleView := NewConsole(arguments.ViewOptions{ViewType: arguments.ViewJSON}, view)
	call(consoleView)
	output := done(t)
	if output.Stderr() != "" {
		t.Errorf("expected no stderr but got:\n%s", output.Stderr())
	}

	testJSONViewOutputEquals(t, output.Stdout(), want)
}

func testConsoleMulti(t *testing.T, call func(console Console), wantStdout string, wantStderr string, want []map[string]interface{}) {
	jsonInto, err := os.CreateTemp(t.TempDir(), "json-into-*")
	if err != nil {
		t.Fatalf("failed to create the file to write json content into: %s", err)
	}
	view, done := testView(t)
	consoleView := NewConsole(arguments.ViewOptions{ViewType: arguments.ViewHuman, JSONInto: jsonInto}, view)
	call(consoleView)
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
