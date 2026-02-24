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

func TestLogoutViews(t *testing.T) {
	tests := map[string]struct {
		viewCall   func(logout Logout)
		wantJson   []map[string]any
		wantStdout string
		wantStderr string
	}{
		"noCredentialsStored": {
			viewCall: func(logout Logout) {
				logout.NoCredentialsStored("app.example.com")
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "No credentials for app.example.com are stored",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline("No credentials for app.example.com are stored.\n"),
		},
		"removingCredentialsFromHelper": {
			viewCall: func(logout Logout) {
				logout.RemovingCredentialsFromHelper("app.example.com", "osxkeychain")
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Removing the stored credentials for app.example.com from the configured \"osxkeychain\" credentials helper",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: "Removing the stored credentials for app.example.com from the configured\n\"osxkeychain\" credentials helper.\n\n",
		},
		"removingCredentialsFromFile": {
			viewCall: func(logout Logout) {
				logout.RemovingCredentialsFromFile("app.example.com", "/home/user/.terraform.d/credentials.tfrc.json")
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Removing the stored credentials for app.example.com from the following file: /home/user/.terraform.d/credentials.tfrc.json",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: "Removing the stored credentials for app.example.com from the following file:\n    /home/user/.terraform.d/credentials.tfrc.json\n\n",
		},
		"logoutSuccess": {
			viewCall: func(logout Logout) {
				logout.LogoutSuccess("app.example.com")
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Success! OpenTofu has removed the stored API token for app.example.com",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: "Success! OpenTofu has removed the stored API token for app.example.com.\n\n",
		},
		// Diagnostics
		"warning": {
			viewCall: func(logout Logout) {
				diags := tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Warning, "A warning occurred", "foo bar"),
				}
				logout.Diagnostics(diags)
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
			viewCall: func(logout Logout) {
				diags := tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Error, "An error occurred", "foo bar"),
				}
				logout.Diagnostics(diags)
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
			viewCall: func(logout Logout) {
				diags := tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Warning, "A warning", "foo bar warning"),
					tfdiags.Sourceless(tfdiags.Error, "An error", "foo bar error"),
				}
				logout.Diagnostics(diags)
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
			testLogoutHuman(t, tc.viewCall, tc.wantStdout, tc.wantStderr)
			testLogoutJson(t, tc.viewCall, tc.wantJson)
			testLogoutMulti(t, tc.viewCall, tc.wantStdout, tc.wantStderr, tc.wantJson)
		})
	}
}

func testLogoutHuman(t *testing.T, call func(logout Logout), wantStdout, wantStderr string) {
	view, done := testView(t)
	logoutView := NewLogout(arguments.ViewOptions{ViewType: arguments.ViewHuman}, view)
	call(logoutView)
	output := done(t)
	if diff := cmp.Diff(wantStderr, output.Stderr()); diff != "" {
		t.Errorf("invalid stderr (-want, +got):\n%s", diff)
	}
	if diff := cmp.Diff(wantStdout, output.Stdout()); diff != "" {
		t.Errorf("invalid stdout (-want, +got):\n%s", diff)
	}
}

func testLogoutJson(t *testing.T, call func(logout Logout), want []map[string]interface{}) {
	view, done := testView(t)
	logoutView := NewLogout(arguments.ViewOptions{ViewType: arguments.ViewJSON}, view)
	call(logoutView)
	output := done(t)
	if output.Stderr() != "" {
		t.Errorf("expected no stderr but got:\n%s", output.Stderr())
	}

	testJSONViewOutputEquals(t, output.Stdout(), want)
}

func testLogoutMulti(t *testing.T, call func(logout Logout), wantStdout string, wantStderr string, want []map[string]interface{}) {
	jsonInto, err := os.CreateTemp(t.TempDir(), "json-into-*")
	if err != nil {
		t.Fatalf("failed to create the file to write json content into: %s", err)
	}
	view, done := testView(t)
	logoutView := NewLogout(arguments.ViewOptions{ViewType: arguments.ViewHuman, JSONInto: jsonInto}, view)
	call(logoutView)
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
