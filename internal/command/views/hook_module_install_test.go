// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package views

import (
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/go-version"
	"github.com/opentofu/opentofu/internal/initwd"
)

func TestModuleInstallationHooks(t *testing.T) {
	tests := map[string]struct {
		viewCall       func(hook initwd.ModuleInstallHooks)
		showLocalPaths bool
		wantJson       []map[string]any
		wantStdout     string
		wantStderr     string
	}{
		"download_with_version": {
			viewCall: func(hook initwd.ModuleInstallHooks) {
				hook.Download("root.networking", "git::https://example.com/module.git", version.Must(version.NewVersion("2.5.3")))
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Downloading git::https://example.com/module.git 2.5.3 for root.networking...",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline("Downloading git::https://example.com/module.git 2.5.3 for root.networking..."),
		},
		"download_without_version": {
			viewCall: func(hook initwd.ModuleInstallHooks) {
				hook.Download("root.storage", "git::https://example.com/storage.git", nil)
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Downloading git::https://example.com/storage.git for root.storage...",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline("Downloading git::https://example.com/storage.git for root.storage..."),
		},
		"install_without_local_path": {
			viewCall: func(hook initwd.ModuleInstallHooks) {
				hook.Install("root.networking", version.Must(version.NewVersion("2.5.3")), "/path/to/.terraform/modules/networking")
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "installing root.networking",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline("- root.networking"),
		},
		"install_with_local_path": {
			viewCall: func(hook initwd.ModuleInstallHooks) {
				hook.Install("root.networking", version.Must(version.NewVersion("2.5.3")), "/path/to/.terraform/modules/networking")
			},
			showLocalPaths: true,
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "installing root.networking in /path/to/.terraform/modules/networking",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: withNewline("- root.networking in /path/to/.terraform/modules/networking"),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			testModuleInstallationHookHuman(t, tc.viewCall, tc.showLocalPaths, tc.wantStdout, tc.wantStderr)
			testModuleInstallationHookJson(t, tc.viewCall, tc.showLocalPaths, tc.wantJson)
			testModuleInstallationHookMulti(t, tc.viewCall, tc.showLocalPaths, tc.wantStdout, tc.wantStderr, tc.wantJson)
		})
	}
}

func testModuleInstallationHookHuman(t *testing.T, call func(init initwd.ModuleInstallHooks), showLocalPaths bool, wantStdout, wantStderr string) {
	view, done := testView(t)
	moduleInstallationViewCall := moduleInstallationHookHuman{v: view, showLocalPaths: showLocalPaths}
	call(moduleInstallationViewCall)
	output := done(t)
	if diff := cmp.Diff(wantStderr, output.Stderr()); diff != "" {
		t.Errorf("invalid stderr (-want, +got):\n%s", diff)
	}
	if diff := cmp.Diff(wantStdout, output.Stdout()); diff != "" {
		t.Errorf("invalid stdout (-want, +got):\n%s", diff)
	}
}

func testModuleInstallationHookJson(t *testing.T, call func(init initwd.ModuleInstallHooks), showLocalPaths bool, want []map[string]interface{}) {
	// New type just to assert the fields that we are interested in
	view, done := testView(t)
	moduleInstallationViewCall := moduleInstallationHookJSON{v: NewJSONView(view, nil), showLocalPaths: showLocalPaths}
	call(moduleInstallationViewCall)
	output := done(t)
	if output.Stderr() != "" {
		t.Errorf("expected no stderr but got:\n%s", output.Stderr())
	}

	testJSONViewOutputEquals(t, output.Stdout(), want)
}

func testModuleInstallationHookMulti(t *testing.T, call func(init initwd.ModuleInstallHooks), showLocalPaths bool, wantStdout string, wantStderr string, want []map[string]interface{}) {
	jsonInto, err := os.CreateTemp(t.TempDir(), "json-into-*")
	if err != nil {
		t.Fatalf("failed to create the file to write json content into: %s", err)
	}
	view, done := testView(t)
	moduleInstallationViewCall := moduleInstallationHookMulti{
		moduleInstallationHookHuman{v: view, showLocalPaths: showLocalPaths},
		moduleInstallationHookJSON{v: NewJSONView(view, jsonInto), showLocalPaths: showLocalPaths},
	}
	call(moduleInstallationViewCall)
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
