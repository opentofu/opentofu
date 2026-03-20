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

func TestUnlockViews(t *testing.T) {
	tests := map[string]struct {
		viewCall   func(v Unlock)
		wantJson   []map[string]any
		wantStdout string
		wantStderr string
	}{
		"locking disabled for backend": {
			viewCall: func(v Unlock) {
				v.LockingDisabledForBackend()
			},
			wantStdout: "",
			wantStderr: withNewline("Locking is disabled for this backend"),
			wantJson: []map[string]any{
				{
					"@level":   "error",
					"@message": "Locking is disabled for this backend",
					"@module":  "tofu.ui",
				},
			},
		},
		"cannot unlock by another process": {
			viewCall: func(v Unlock) {
				v.CannotUnlockByAnotherProcess()
			},
			wantStdout: "",
			wantStderr: withNewline("Local state cannot be unlocked by another process"),
			wantJson: []map[string]any{
				{
					"@level":   "error",
					"@message": "Local state cannot be unlocked by another process",
					"@module":  "tofu.ui",
				},
			},
		},
		"force unlock cancelled": {
			viewCall: func(v Unlock) {
				v.ForceUnlockCancelled()
			},
			wantStdout: withNewline("force-unlock cancelled."),
			wantStderr: "",
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "force-unlock cancelled",
					"@module":  "tofu.ui",
				},
			},
		},
		"force unlock succeeded": {
			viewCall: func(v Unlock) {
				v.ForceUnlockSucceeded()
			},
			wantStdout: withNewline("OpenTofu state has been successfully unlocked!\n\nThe state has been unlocked, and OpenTofu commands should now be able to\nobtain a new lock on the remote state."),
			wantStderr: "",
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "The state has been unlocked, and OpenTofu commands should now be able to obtain a new lock on the remote state.",
					"@module":  "tofu.ui",
				},
			},
		},
		// Diagnostics
		"warning": {
			viewCall: func(v Unlock) {
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
			viewCall: func(v Unlock) {
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
			viewCall: func(v Unlock) {
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
			testUnlockHuman(t, tc.viewCall, tc.wantStdout, tc.wantStderr)
			testUnlockJson(t, tc.viewCall, tc.wantJson)
			testUnlockMulti(t, tc.viewCall, tc.wantStdout, tc.wantStderr, tc.wantJson)
		})
	}
}

func testUnlockHuman(t *testing.T, call func(v Unlock), wantStdout, wantStderr string) {
	view, done := testView(t)
	getView := NewUnlock(arguments.ViewOptions{ViewType: arguments.ViewHuman}, view)
	call(getView)
	output := done(t)
	if diff := cmp.Diff(wantStderr, output.Stderr()); diff != "" {
		t.Errorf("invalid stderr (-want, +got):\n%s", diff)
	}
	if diff := cmp.Diff(wantStdout, output.Stdout()); diff != "" {
		t.Errorf("invalid stdout (-want, +got):\n%s", diff)
	}
}

func testUnlockJson(t *testing.T, call func(v Unlock), want []map[string]interface{}) {
	view, done := testView(t)
	getView := NewUnlock(arguments.ViewOptions{ViewType: arguments.ViewJSON}, view)
	call(getView)
	output := done(t)
	if output.Stderr() != "" {
		t.Errorf("expected no stderr but got:\n%s", output.Stderr())
	}

	testJSONViewOutputEquals(t, output.Stdout(), want)
}

func testUnlockMulti(t *testing.T, call func(v Unlock), wantStdout string, wantStderr string, want []map[string]interface{}) {
	jsonInto, err := os.CreateTemp(t.TempDir(), "json-into-*")
	if err != nil {
		t.Fatalf("failed to create the file to write json content into: %s", err)
	}
	view, done := testView(t)
	getView := NewUnlock(arguments.ViewOptions{ViewType: arguments.ViewHuman, JSONInto: jsonInto}, view)
	call(getView)
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
