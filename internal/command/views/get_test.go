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

func TestGetViews(t *testing.T) {
	tests := map[string]struct {
		viewCall   func(get Get)
		wantJson   []map[string]any
		wantStdout string
		wantStderr string
	}{
		// Diagnostics
		"warning": {
			viewCall: func(get Get) {
				diags := tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Warning, "A warning occurred", "foo bar"),
				}
				get.Diagnostics(diags)
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
			viewCall: func(get Get) {
				diags := tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Error, "An error occurred", "foo bar"),
				}
				get.Diagnostics(diags)
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
			viewCall: func(get Get) {
				diags := tfdiags.Diagnostics{
					tfdiags.Sourceless(tfdiags.Warning, "A warning", "foo bar warning"),
					tfdiags.Sourceless(tfdiags.Error, "An error", "foo bar error"),
				}
				get.Diagnostics(diags)
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
		// Miscs
		"help prompt": {
			viewCall: func(get Get) {
				get.HelpPrompt()
			},
			wantStdout: "",
			wantStderr: withNewline("\nFor more help on using this command, run:\n  tofu get -help"),
			wantJson:   []map[string]any{{}},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			testGetHuman(t, tc.viewCall, tc.wantStdout, tc.wantStderr)
			testGetJson(t, tc.viewCall, tc.wantJson)
			testGetMulti(t, tc.viewCall, tc.wantStdout, tc.wantStderr, tc.wantJson)
		})
	}
}

func TestGetViews_Hooks(t *testing.T) {
	t.Run("hooks_human", func(t *testing.T) {
		view, _ := testView(t)
		getView := NewGet(arguments.ViewOptions{ViewType: arguments.ViewHuman}, view)
		hooks := getView.Hooks(false)

		if hooks == nil {
			t.Fatal("expected hooks to be non-nil")
		}

		_, ok := hooks.(*moduleInstallationHookHuman)
		if !ok {
			t.Errorf("expected *moduleInstallationHookHuman, got %T", hooks)
		}
	})

	t.Run("hooks_json", func(t *testing.T) {
		view, _ := testView(t)
		getView := NewGet(arguments.ViewOptions{ViewType: arguments.ViewJSON}, view)
		hooks := getView.Hooks(true)

		if hooks == nil {
			t.Fatal("expected hooks to be non-nil")
		}

		_, ok := hooks.(*moduleInstallationHookJSON)
		if !ok {
			t.Errorf("expected *moduleInstallationHookJSON, got %T", hooks)
		}
	})

	t.Run("hooks_multi", func(t *testing.T) {
		jsonInto, err := os.CreateTemp(t.TempDir(), "json-into-*")
		if err != nil {
			t.Fatalf("failed to create temp file: %s", err)
		}
		defer func() { _ = jsonInto.Close() }()

		view, _ := testView(t)
		getView := NewGet(arguments.ViewOptions{ViewType: arguments.ViewHuman, JSONInto: jsonInto}, view)
		hooks := getView.Hooks(true)

		if hooks == nil {
			t.Fatal("expected hooks to be non-nil")
		}

		_, ok := hooks.(moduleInstallationHookMulti)
		if !ok {
			t.Errorf("expected moduleInstallationHookMulti, got %T", hooks)
		}
	})
}

func testGetHuman(t *testing.T, call func(get Get), wantStdout, wantStderr string) {
	view, done := testView(t)
	getView := NewGet(arguments.ViewOptions{ViewType: arguments.ViewHuman}, view)
	call(getView)
	output := done(t)
	if diff := cmp.Diff(wantStderr, output.Stderr()); diff != "" {
		t.Errorf("invalid stderr (-want, +got):\n%s", diff)
	}
	if diff := cmp.Diff(wantStdout, output.Stdout()); diff != "" {
		t.Errorf("invalid stdout (-want, +got):\n%s", diff)
	}
}

func testGetJson(t *testing.T, call func(get Get), want []map[string]interface{}) {
	view, done := testView(t)
	getView := NewGet(arguments.ViewOptions{ViewType: arguments.ViewJSON}, view)
	call(getView)
	output := done(t)
	if output.Stderr() != "" {
		t.Errorf("expected no stderr but got:\n%s", output.Stderr())
	}

	testJSONViewOutputEquals(t, output.Stdout(), want)
}

func testGetMulti(t *testing.T, call func(get Get), wantStdout string, wantStderr string, want []map[string]interface{}) {
	jsonInto, err := os.CreateTemp(t.TempDir(), "json-into-*")
	if err != nil {
		t.Fatalf("failed to create the file to write json content into: %s", err)
	}
	view, done := testView(t)
	getView := NewGet(arguments.ViewOptions{ViewType: arguments.ViewHuman, JSONInto: jsonInto}, view)
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
