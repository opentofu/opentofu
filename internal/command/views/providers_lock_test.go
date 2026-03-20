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

func TestProvidersLockView(t *testing.T) {
	tests := map[string]struct {
		viewCall   func(v ProvidersLock)
		wantJson   []map[string]any
		wantStdout string
		wantStderr string
	}{
		"installation fetching": {
			viewCall: func(v ProvidersLock) {
				v.InstallationFetching("registry.opentofu.org/test_ns/test_provider", "3.0.0", "linux_amd64")
			},
			wantStdout: withNewline("- Fetching registry.opentofu.org/test_ns/test_provider 3.0.0 for linux_amd64..."),
			wantStderr: "",
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Fetching registry.opentofu.org/test_ns/test_provider 3.0.0 for linux_amd64...",
					"@module":  "tofu.ui",
				},
			},
		},
		"fetch package success with keyID": {
			viewCall: func(v ProvidersLock) {
				v.FetchPackageSuccess("1234ABCD", "registry.opentofu.org/test_ns/test_provider", "3.0.0", "linux_amd64", "signed by test_ns")
			},
			wantStdout: withNewline("- Retrieved registry.opentofu.org/test_ns/test_provider 3.0.0 for linux_amd64 (signed by test_ns, key ID 1234ABCD)"),
			wantStderr: "",
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Retrieved registry.opentofu.org/test_ns/test_provider 3.0.0 for linux_amd64 (signed by test_ns, key ID 1234ABCD)",
					"@module":  "tofu.ui",
				},
			},
		},
		"fetch package success without keyID": {
			viewCall: func(v ProvidersLock) {
				v.FetchPackageSuccess("", "registry.opentofu.org/test_ns2/test_provider2", "3.1.0", "linux_amd64", "unsigned")
			},
			wantStdout: withNewline("- Retrieved registry.opentofu.org/test_ns2/test_provider2 3.1.0 for linux_amd64 (unsigned)"),
			wantStderr: "",
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Retrieved registry.opentofu.org/test_ns2/test_provider2 3.1.0 for linux_amd64 (unsigned)",
					"@module":  "tofu.ui",
				},
			},
		},
		"lock update new provider": {
			viewCall: func(v ProvidersLock) {
				v.LockUpdateNewProvider("registry.opentofu.org/test_ns/test_provider", "linux_amd64")
			},
			wantStdout: withNewline("- Obtained registry.opentofu.org/test_ns/test_provider checksums for linux_amd64; This was a new provider and the checksums for this platform are now tracked in the lock file"),
			wantStderr: "",
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Obtained registry.opentofu.org/test_ns/test_provider checksums for linux_amd64; This was a new provider and the checksums for this platform are now tracked in the lock file",
					"@module":  "tofu.ui",
				},
			},
		},
		"lock update with new hash for provider": {
			viewCall: func(v ProvidersLock) {
				v.LockUpdateNewHashForProvider("registry.opentofu.org/test_ns/test_provider", "darwin_arm64")
			},
			wantStdout: withNewline("- Obtained registry.opentofu.org/test_ns/test_provider checksums for darwin_arm64; Additional checksums for this platform are now tracked in the lock file"),
			wantStderr: "",
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Obtained registry.opentofu.org/test_ns/test_provider checksums for darwin_arm64; Additional checksums for this platform are now tracked in the lock file",
					"@module":  "tofu.ui",
				},
			},
		},
		"lock update - no changes needed": {
			viewCall: func(v ProvidersLock) {
				v.LockUpdateNoChange("registry.opentofu.org/test_ns2/test_provider2", "linux_amd64")
			},
			wantStdout: withNewline("- Obtained registry.opentofu.org/test_ns2/test_provider2 checksums for linux_amd64; All checksums for this platform were already tracked in the lock file"),
			wantStderr: "",
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Obtained registry.opentofu.org/test_ns2/test_provider2 checksums for linux_amd64; All checksums for this platform were already tracked in the lock file",
					"@module":  "tofu.ui",
				},
			},
		},
		"updated successfully with changes": {
			viewCall: func(v ProvidersLock) {
				v.UpdatedSuccessfully(true)
			},
			wantStdout: withNewline("\nSuccess! OpenTofu has updated the lock file.\n\nReview the changes in .terraform.lock.hcl and then commit to your\nversion control system to retain the new checksums.\n"),
			wantStderr: "",
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Success! OpenTofu has updated the lock file. Review the changes in .terraform.lock.hcl and then commit to your version control system to retain the new checksums",
					"@module":  "tofu.ui",
				},
			},
		},
		"updated successfully no changes": {
			viewCall: func(v ProvidersLock) {
				v.UpdatedSuccessfully(false)
			},
			wantStdout: withNewline("\nSuccess! OpenTofu has validated the lock file and found no need for changes."),
			wantStderr: "",
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Success! OpenTofu has validated the lock file and found no need for changes.",
					"@module":  "tofu.ui",
				},
			},
		},
		// Diagnostics
		"warning": {
			viewCall: func(v ProvidersLock) {
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
			viewCall: func(v ProvidersLock) {
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
			viewCall: func(v ProvidersLock) {
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
			testProvidersLockHuman(t, tc.viewCall, tc.wantStdout, tc.wantStderr)
			testProvidersLockJson(t, tc.viewCall, tc.wantJson)
			testProvidersLockMulti(t, tc.viewCall, tc.wantStdout, tc.wantStderr, tc.wantJson)
		})
	}
}

func testProvidersLockHuman(t *testing.T, call func(v ProvidersLock), wantStdout, wantStderr string) {
	view, done := testView(t)
	v := NewProvidersLock(arguments.ViewOptions{ViewType: arguments.ViewHuman}, view)
	call(v)
	output := done(t)
	if diff := cmp.Diff(wantStderr, output.Stderr()); diff != "" {
		t.Errorf("invalid stderr (-want, +got):\n%s", diff)
	}
	if diff := cmp.Diff(wantStdout, output.Stdout()); diff != "" {
		t.Errorf("invalid stdout (-want, +got):\n%s", diff)
	}
}

func testProvidersLockJson(t *testing.T, call func(v ProvidersLock), want []map[string]interface{}) {
	view, done := testView(t)
	v := NewProvidersLock(arguments.ViewOptions{ViewType: arguments.ViewJSON}, view)
	call(v)
	output := done(t)
	if output.Stderr() != "" {
		t.Errorf("expected no stderr but got:\n%s", output.Stderr())
	}

	testJSONViewOutputEquals(t, output.Stdout(), want)
}

func testProvidersLockMulti(t *testing.T, call func(v ProvidersLock), wantStdout string, wantStderr string, want []map[string]interface{}) {
	jsonInto, err := os.CreateTemp(t.TempDir(), "json-into-*")
	if err != nil {
		t.Fatalf("failed to create the file to write json content into: %s", err)
	}
	view, done := testView(t)
	v := NewProvidersLock(arguments.ViewOptions{ViewType: arguments.ViewHuman, JSONInto: jsonInto}, view)
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
