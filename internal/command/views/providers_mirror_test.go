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

func TestProvidersMirrorView(t *testing.T) {
	tests := map[string]struct {
		viewCall   func(v ProvidersMirror)
		wantJson   []map[string]any
		wantStdout string
		wantStderr string
	}{
		"provider skipped": {
			viewCall: func(v ProvidersMirror) {
				v.ProviderSkipped("terraform.io/builtin/terraform")
			},
			wantStdout: withNewline("- Skipping terraform.io/builtin/terraform because it is built in to OpenTofu CLI"),
			wantStderr: "",
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Skipping terraform.io/builtin/terraform because it is built in to OpenTofu CLI",
					"@module":  "tofu.ui",
				},
			},
		},
		"mirroring provider": {
			viewCall: func(v ProvidersMirror) {
				v.MirroringProvider("registry.opentofu.org/test_ns/test_provider")
			},
			wantStdout: withNewline("- Mirroring registry.opentofu.org/test_ns/test_provider..."),
			wantStderr: "",
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Mirroring registry.opentofu.org/test_ns/test_provider...",
					"@module":  "tofu.ui",
				},
			},
		},
		"provider version selected to match lockfile": {
			viewCall: func(v ProvidersMirror) {
				v.ProviderVersionSelectedToMatchLockfile("registry.opentofu.org/test_ns/test_provider", "3.0.0")
			},
			wantStdout: withNewline("  - Selected v3.0.0 to match dependency lock file"),
			wantStderr: "",
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Selected registry.opentofu.org/test_ns/test_provider v3.0.0 to match dependency lock file",
					"@module":  "tofu.ui",
				},
			},
		},
		"provider version selected to match constraints": {
			viewCall: func(v ProvidersMirror) {
				v.ProviderVersionSelectedToMatchConstraints("registry.opentofu.org/test_ns/test_provider", "3.1.0", "~> 3.0")
			},
			wantStdout: withNewline("  - Selected v3.1.0 to meet constraints ~> 3.0"),
			wantStderr: "",
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Selected registry.opentofu.org/test_ns/test_provider v3.1.0 to meet constraints ~> 3.0",
					"@module":  "tofu.ui",
				},
			},
		},
		"provider version selected with no constraints": {
			viewCall: func(v ProvidersMirror) {
				v.ProviderVersionSelectedWithNoConstraints("registry.opentofu.org/test_ns/test_provider", "4.0.0")
			},
			wantStdout: withNewline("  - Selected v4.0.0 with no constraints"),
			wantStderr: "",
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Selected registry.opentofu.org/test_ns/test_provider v4.0.0 with no constraints",
					"@module":  "tofu.ui",
				},
			},
		},
		"downloading package for": {
			viewCall: func(v ProvidersMirror) {
				v.DownloadingPackageFor("registry.opentofu.org/test_ns/test_provider", "3.0.0", "linux_amd64")
			},
			wantStdout: withNewline("  - Downloading package for linux_amd64..."),
			wantStderr: "",
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Downloading registry.opentofu.org/test_ns/test_provider v3.0.0 package for linux_amd64...",
					"@module":  "tofu.ui",
				},
			},
		},
		"package authenticated": {
			viewCall: func(v ProvidersMirror) {
				v.PackageAuthenticated("registry.opentofu.org/test_ns/test_provider", "3.0.0", "linux_amd64", "signed by test_ns")
			},
			wantStdout: withNewline("  - Package authenticated: signed by test_ns"),
			wantStderr: "",
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Package registry.opentofu.org/test_ns/test_provider v3.0.0 for linux_amd64 authenticated: signed by test_ns",
					"@module":  "tofu.ui",
				},
			},
		},
		// Diagnostics
		"warning": {
			viewCall: func(v ProvidersMirror) {
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
			viewCall: func(v ProvidersMirror) {
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
			viewCall: func(v ProvidersMirror) {
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
			testProvidersMirrorHuman(t, tc.viewCall, tc.wantStdout, tc.wantStderr)
			testProvidersMirrorJson(t, tc.viewCall, tc.wantJson)
			testProvidersMirrorMulti(t, tc.viewCall, tc.wantStdout, tc.wantStderr, tc.wantJson)
		})
	}
}

func testProvidersMirrorHuman(t *testing.T, call func(v ProvidersMirror), wantStdout, wantStderr string) {
	view, done := testView(t)
	v := NewProvidersMirror(arguments.ViewOptions{ViewType: arguments.ViewHuman}, view)
	call(v)
	output := done(t)
	if diff := cmp.Diff(wantStderr, output.Stderr()); diff != "" {
		t.Errorf("invalid stderr (-want, +got):\n%s", diff)
	}
	if diff := cmp.Diff(wantStdout, output.Stdout()); diff != "" {
		t.Errorf("invalid stdout (-want, +got):\n%s", diff)
	}
}

func testProvidersMirrorJson(t *testing.T, call func(v ProvidersMirror), want []map[string]interface{}) {
	view, done := testView(t)
	v := NewProvidersMirror(arguments.ViewOptions{ViewType: arguments.ViewJSON}, view)
	call(v)
	output := done(t)
	if output.Stderr() != "" {
		t.Errorf("expected no stderr but got:\n%s", output.Stderr())
	}

	testJSONViewOutputEquals(t, output.Stdout(), want)
}

func testProvidersMirrorMulti(t *testing.T, call func(v ProvidersMirror), wantStdout string, wantStderr string, want []map[string]interface{}) {
	jsonInto, err := os.CreateTemp(t.TempDir(), "json-into-*")
	if err != nil {
		t.Fatalf("failed to create the file to write json content into: %s", err)
	}
	view, done := testView(t)
	v := NewProvidersMirror(arguments.ViewOptions{ViewType: arguments.ViewHuman, JSONInto: jsonInto}, view)
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
