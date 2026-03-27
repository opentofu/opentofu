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
)

func TestBackendViews(t *testing.T) {
	tests := map[string]struct {
		viewCall   func(view Backend)
		wantJson   []map[string]any
		wantStdout string
		wantStderr string
	}{
		"initializingCloudBackend": {
			viewCall: func(view Backend) {
				view.InitializingCloudBackend()
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Initializing cloud backend...",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: `
Initializing cloud backend...
`,
		},
		"initializingBackend": {
			viewCall: func(view Backend) {
				view.InitializingBackend()
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Initializing the backend...",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: `
Initializing the backend...
`,
		},
		"backendTypeAlias": {
			viewCall: func(view Backend) {
				view.BackendTypeAlias("s3", "aws_s3")
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "\"s3\" is an alias for backend type \"aws_s3\"",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: `- "s3" is an alias for backend type "aws_s3"
`,
		},
		"migratingFromCloudToLocal": {
			viewCall: func(view Backend) {
				view.MigratingFromCloudToLocal()
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Migrating from cloud backend to local state.",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: `Migrating from cloud backend to local state.
`,
		},
		"unconfiguringBackendType": {
			viewCall: func(view Backend) {
				view.UnconfiguringBackendType("s3")
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "OpenTofu has detected you're unconfiguring your previously set \"s3\" backend",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: `OpenTofu has detected you're unconfiguring your previously set "s3" backend.
`,
		},
		"backendTypeUnset": {
			viewCall: func(view Backend) {
				view.BackendTypeUnset("s3")
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Successfully unset the backend \"s3\". OpenTofu will now operate locally",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: `

Successfully unset the backend "s3". OpenTofu will now operate locally.
`,
		},
		"backendTypeSet": {
			viewCall: func(view Backend) {
				view.BackendTypeSet("s3")
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Successfully configured the backend \"s3\"! OpenTofu will automatically use this backend unless the backend configuration changes",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: `
Successfully configured the backend "s3"! OpenTofu will automatically
use this backend unless the backend configuration changes.
`,
		},
		"cloudBackendUpdated": {
			viewCall: func(view Backend) {
				view.CloudBackendUpdated()
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Cloud backend configuration has changed",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: `Cloud backend configuration has changed.
`,
		},
		"migratingLocalTypeToCloud": {
			viewCall: func(view Backend) {
				view.MigratingLocalTypeToCloud("s3")
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Migrating from backend \"s3\" to cloud backend",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: `Migrating from backend "s3" to cloud backend.
`,
		},
		"migratingCloudToLocalType": {
			viewCall: func(view Backend) {
				view.MigratingCloudToLocalType("s3")
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Migrating from cloud backend to backend \"s3\"",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: `Migrating from cloud backend to backend "s3".
`,
		},
		"backendTypeChanged": {
			viewCall: func(view Backend) {
				view.BackendTypeChanged("s3", "gcs")
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "OpenTofu detected that the backend type changed from \"s3\" to \"gcs\".",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: `
OpenTofu detected that the backend type changed from "s3" to "gcs".
`,
		},
		"backendReconfigured": {
			viewCall: func(view Backend) {
				view.BackendReconfigured()
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Backend configuration changed! OpenTofu has detected that the configuration specified for the backend has changed. OpenTofu will now check for existing state in the backends",
					"@module":  "tofu.ui",
				},
			},
			wantStdout: `Backend configuration changed!

OpenTofu has detected that the configuration specified for the backend
has changed. OpenTofu will now check for existing state in the backends.
`,
		},
		"migrationCompleted": {
			viewCall: func(view Backend) {
				view.MigrationCompleted([]string{"default", "staging"}, "default")
			},
			wantJson: []map[string]any{
				{
					"@level":            "info",
					"@message":          "Migration complete",
					"@module":           "tofu.ui",
					"workspaces":        []any{"default", "staging"},
					"current_workspace": "default",
				},
			},
			wantStdout: `Migration complete! Your workspaces are as follows:
* default
  staging

`,
		},
		// this validates that the [Backend] and [StateLocker] are initialised correctly
		// and both write to the same output [os.File].
		"same output used for multiple views": {
			viewCall: func(view Backend) {
				view.InitializingCloudBackend()
				slv := view.StateLocker()
				slv.Locking()
				slv.Unlocking()
				view.InitializingCloudBackend()
			},
			wantJson: []map[string]any{
				{
					"@level":   "info",
					"@message": "Initializing cloud backend...",
					"@module":  "tofu.ui",
				},
				{
					"@level":   "info",
					"@message": "Acquiring state lock. This may take a few moments...",
					"@module":  "tofu.ui",
					"type":     "state_lock_acquire",
				},
				{
					"@level":   "info",
					"@message": "Releasing state lock. This may take a few moments...",
					"@module":  "tofu.ui",
					"type":     "state_lock_release",
				},
				{
					"@level":   "info",
					"@message": "Initializing cloud backend...",
					"@module":  "tofu.ui",
				},
				{},
			},
			wantStdout: `
Initializing cloud backend...
Acquiring state lock. This may take a few moments...
Releasing state lock. This may take a few moments...

Initializing cloud backend...
`,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			testBackendHuman(t, tc.viewCall, tc.wantStdout, tc.wantStderr)
			testBackendJson(t, tc.viewCall, tc.wantJson)
			testBackendMulti(t, tc.viewCall, tc.wantStdout, tc.wantStderr, tc.wantJson)
		})
	}
}

func testBackendHuman(t *testing.T, call func(view Backend), wantStdout, wantStderr string) {
	view, done := testView(t)
	initView := NewBackend(arguments.ViewOptions{ViewType: arguments.ViewHuman}, view)
	call(initView)
	output := done(t)
	if diff := cmp.Diff(wantStderr, output.Stderr()); diff != "" {
		t.Errorf("invalid stderr (-want, +got):\n%s", diff)
	}
	if diff := cmp.Diff(wantStdout, output.Stdout()); diff != "" {
		t.Errorf("invalid stdout (-want, +got):\n%s", diff)
	}
}

func testBackendJson(t *testing.T, call func(view Backend), want []map[string]any) {
	// New type just to assert the fields that we are interested in
	view, done := testView(t)
	initView := NewBackend(arguments.ViewOptions{ViewType: arguments.ViewJSON}, view)
	call(initView)
	output := done(t)
	if output.Stderr() != "" {
		t.Errorf("expected no stderr but got:\n%s", output.Stderr())
	}

	testJSONViewOutputEquals(t, output.Stdout(), want)
}

func testBackendMulti(t *testing.T, call func(view Backend), wantStdout string, wantStderr string, want []map[string]any) {
	jsonInto, err := os.CreateTemp(t.TempDir(), "json-into-*")
	if err != nil {
		t.Fatalf("failed to create the file to write json content into: %s", err)
	}
	view, done := testView(t)
	initView := NewBackend(arguments.ViewOptions{ViewType: arguments.ViewHuman, JSONInto: jsonInto}, view)
	call(initView)
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
